package bucky

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/kronk"
)

// Version contains the current version of the bucky SDK package.
const Version = kronk.Version

// =============================================================================

// Bucky provides a concurrently safe API for using whisper.cpp.
// Each Bucky owns one model.Model (which in turn owns one
// whisper.Context). The whisper context is single-stream so
// concurrent transcribes are bounded by a per-handle semaphore sized
// at construction time from Config.NSeqMax * Config.QueueDepth.
type Bucky struct {
	cfg           model.Config
	model         *model.Model
	sem           chan struct{}
	activeStreams atomic.Int32
	shutdown      sync.Mutex
	shutdownFlag  bool
	modelInfo     model.ModelInfo
}

// New provides the ability to use a whisper model in a concurrently
// safe way.
func New(opts ...model.Option) (*Bucky, error) {
	return NewWithContext(context.Background(), opts...)
}

// NewWithContext provides the ability to use a whisper model in a
// concurrently safe way. The context is used to support logging trace
// ids during model loading.
func NewWithContext(ctx context.Context, opts ...model.Option) (*Bucky, error) {
	if libraryLocation == "" {
		return nil, fmt.Errorf("new: the Init() function has not been called")
	}

	// -------------------------------------------------------------------------

	cfg := model.NewConfig(opts...)

	mdl, err := model.NewModel(ctx, cfg)
	if err != nil {
		return nil, err
	}

	resolved := mdl.Config()

	// Whisper has no batch engine, so the outer semaphore is sized
	// 1:1 with the model's state pool. This matches sdk/kronk's
	// rule for embedding / rerank models (semCapacity = NSeqMax),
	// not the text-generation rule (NSeqMax * QueueDepth).
	semCapacity := max(resolved.NSeqMax, 1)

	b := Bucky{
		cfg:       resolved,
		model:     mdl,
		sem:       make(chan struct{}, semCapacity),
		modelInfo: mdl.ModelInfo(),
	}

	return &b, nil
}

// ModelConfig returns a copy of the resolved configuration being
// used.
func (b *Bucky) ModelConfig() model.Config {
	return b.cfg
}

// ModelInfo returns the static information about the loaded model.
func (b *Bucky) ModelInfo() model.ModelInfo {
	return b.modelInfo
}

// ActiveStreams returns the number of in-flight transcribe calls.
func (b *Bucky) ActiveStreams() int {
	return int(b.activeStreams.Load())
}

// SystemInfo returns the whisper.cpp system info string parsed into a
// key/value map for observability output.
func (b *Bucky) SystemInfo() map[string]string {
	return model.SystemInfo()
}

// Unload will close down the loaded model. You should call this only
// when you are completely done using Bucky.
func (b *Bucky) Unload(ctx context.Context) error {
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// -------------------------------------------------------------------------

	err := func() error {
		b.shutdown.Lock()
		defer b.shutdown.Unlock()

		if b.shutdownFlag {
			return fmt.Errorf("unload: already unloaded")
		}

		for b.activeStreams.Load() > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("unload: cannot unload, too many active-streams[%d]: %w", b.activeStreams.Load(), ctx.Err())

			case <-time.After(100 * time.Millisecond):
			}
		}

		b.shutdownFlag = true
		return nil
	}()

	if err != nil {
		return err
	}

	// -------------------------------------------------------------------------

	if err := b.model.Unload(ctx); err != nil {
		return fmt.Errorf("unload: failed to unload model, model-id[%s]: %w", b.model.ModelInfo().ID, err)
	}

	return nil
}

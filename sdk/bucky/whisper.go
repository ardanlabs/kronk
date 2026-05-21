package bucky

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

// Version contains the current version of the bucky SDK package.
const Version = "0.1.0"

// =============================================================================

// Whisper provides a concurrently safe API for using whisper.cpp.
// Each Whisper owns one model.Model (which in turn owns one
// whisper.Context). The whisper context is single-stream so
// concurrent transcribes are bounded by a per-handle semaphore sized
// at construction time from Config.NSeqMax * Config.QueueDepth.
type Whisper struct {
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
func New(opts ...model.Option) (*Whisper, error) {
	return NewWithContext(context.Background(), opts...)
}

// NewWithContext provides the ability to use a whisper model in a
// concurrently safe way. The context is used to support logging trace
// ids during model loading.
func NewWithContext(ctx context.Context, opts ...model.Option) (*Whisper, error) {
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

	w := Whisper{
		cfg:       resolved,
		model:     mdl,
		sem:       make(chan struct{}, semCapacity),
		modelInfo: mdl.ModelInfo(),
	}

	return &w, nil
}

// ModelConfig returns a copy of the resolved configuration being
// used.
func (w *Whisper) ModelConfig() model.Config {
	return w.cfg
}

// ModelInfo returns the static information about the loaded model.
func (w *Whisper) ModelInfo() model.ModelInfo {
	return w.modelInfo
}

// ActiveStreams returns the number of in-flight transcribe calls.
func (w *Whisper) ActiveStreams() int {
	return int(w.activeStreams.Load())
}

// SystemInfo returns the whisper.cpp system info string parsed into a
// key/value map for observability output.
func (w *Whisper) SystemInfo() map[string]string {
	return model.SystemInfo()
}

// Unload will close down the loaded model. You should call this only
// when you are completely done using Whisper.
func (w *Whisper) Unload(ctx context.Context) error {
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// -------------------------------------------------------------------------

	err := func() error {
		w.shutdown.Lock()
		defer w.shutdown.Unlock()

		if w.shutdownFlag {
			return fmt.Errorf("unload: already unloaded")
		}

		for w.activeStreams.Load() > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("unload: cannot unload, too many active-streams[%d]: %w", w.activeStreams.Load(), ctx.Err())

			case <-time.After(100 * time.Millisecond):
			}
		}

		w.shutdownFlag = true
		return nil
	}()

	if err != nil {
		return err
	}

	// -------------------------------------------------------------------------

	if err := w.model.Unload(ctx); err != nil {
		return fmt.Errorf("unload: failed to unload model, model-id[%s]: %w", w.model.ModelInfo().ID, err)
	}

	return nil
}

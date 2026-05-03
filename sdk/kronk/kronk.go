// Package kronk provides support for working with models using llama.cpp via yzma.
package kronk

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// TODO: Verify latest version of llama.cpp and update default.

// Version contains the current version of the kronk package.
const Version = "1.24.1"

// =============================================================================

// Kronk provides a concurrently safe api for using llama.cpp to access models.
type Kronk struct {
	cfg           model.Config
	model         *model.Model
	sem           chan struct{}
	activeStreams atomic.Int32
	shutdown      sync.Mutex
	shutdownFlag  bool
	modelInfo     model.ModelInfo
}

// New provides the ability to use models in a concurrently safe way.
func New(opts ...model.Option) (*Kronk, error) {
	return NewWithContext(context.Background(), opts...)
}

// NewWithContext provides the ability to use models in a concurrently safe way.
// The context is used to support logging trace ids during model loading.
func NewWithContext(ctx context.Context, opts ...model.Option) (*Kronk, error) {
	if libraryLocation == "" {
		return nil, fmt.Errorf("new: the Init() function has not been called")
	}

	// Ensure the processor-plugin registry has the built-in processors
	// wired in before the model loads. Idempotent — callers that
	// pre-registered a custom processor before this call keep precedence
	// (selectProcessor walks registrations in order).
	registerDefaultProcessors()

	// -------------------------------------------------------------------------

	cfg := model.NewConfig(opts...)

	// -------------------------------------------------------------------------

	mdl, err := model.NewModel(ctx, cfg)
	if err != nil {
		return nil, err
	}

	mi := mdl.ModelInfo()

	// -------------------------------------------------------------------------
	// Embed/rerank models use an internal context pool for parallelism.
	// Text/vision/audio models use the batch engine with queue depth.

	queueDepth := cfg.QueueDepth()
	if queueDepth == 0 {
		queueDepth = 2
	}

	var semCapacity int

	switch {
	case mi.IsEmbedModel || mi.IsRerankModel:
		semCapacity = max(cfg.NSeqMax(), 1)

	default:
		semCapacity = max(cfg.NSeqMax(), 1) * queueDepth
	}

	// -------------------------------------------------------------------------

	krn := Kronk{
		cfg:       mdl.Config(),
		model:     mdl,
		sem:       make(chan struct{}, semCapacity),
		modelInfo: mi,
	}

	return &krn, nil
}

// ModelConfig returns a copy of the configuration being used. This may be
// different from the configuration passed to New() if the model has
// overridden any of the settings.
func (krn *Kronk) ModelConfig() model.Config {
	return krn.cfg
}

// SystemInfo returns system information.
func (krn *Kronk) SystemInfo() map[string]string {
	result := make(map[string]string)

	for part := range strings.SplitSeq(llama.PrintSystemInfo(), "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Remove the "= 1" or similar suffix
		if idx := strings.Index(part, "="); idx != -1 {
			part = strings.TrimSpace(part[:idx])
		}

		// Check for "Key : Value" pattern
		switch kv := strings.SplitN(part, ":", 2); len(kv) {
		case 2:
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			result[key] = value
		default:
			result[part] = "on"
		}
	}

	return result
}

// ModelInfo returns the model information.
func (krn *Kronk) ModelInfo() model.ModelInfo {
	return krn.modelInfo
}

// ActiveStreams returns the number of active streams.
func (krn *Kronk) ActiveStreams() int {
	return int(krn.activeStreams.Load())
}

// Unload will close down the loaded model. You should call this only when you
// are completely done using Kronk.
func (krn *Kronk) Unload(ctx context.Context) error {
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// -------------------------------------------------------------------------

	err := func() error {
		krn.shutdown.Lock()
		defer krn.shutdown.Unlock()

		if krn.shutdownFlag {
			return fmt.Errorf("unload: already unloaded")
		}

		for krn.activeStreams.Load() > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("unload: cannot unload, too many active-streams[%d]: %w", krn.activeStreams.Load(), ctx.Err())

			case <-time.After(100 * time.Millisecond):
			}
		}

		krn.shutdownFlag = true
		return nil
	}()

	if err != nil {
		return err
	}

	// -------------------------------------------------------------------------

	if err := krn.model.Unload(ctx); err != nil {
		return fmt.Errorf("unload: failed to unload model, model-id[%s]: %w", krn.model.ModelInfo().ID, err)
	}

	return nil
}

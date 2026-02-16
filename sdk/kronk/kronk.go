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
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Version contains the current version of the kronk package.
const Version = "1.19.2"

// =============================================================================

type options struct {
	cataloger  model.Cataloger
	ctx        context.Context
	queueDepth int
}

// Option represents options for configuring Kronk.
type Option func(*options)

// WithCataloger sets a custom catalog for model config and template retrieval.
// If not set, a default catalog will be created.
func WithCataloger(cataloger model.Cataloger) Option {
	return func(o *options) {
		o.cataloger = cataloger
	}
}

// WithContext sets a context into the call to support logging trace ids.
func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.ctx = ctx
	}
}

// WithQueueDepth sets the multiplier for semaphore capacity when using the
// batch engine (NSeqMax > 1). This controls how many requests can queue while
// the current batch is processing. Default is 2, meaning NSeqMax * 2 requests
// can be in-flight. Only applies to text inference models.
func WithQueueDepth(multiplier int) Option {
	return func(o *options) {
		if multiplier > 0 {
			o.queueDepth = multiplier
		}
	}
}

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
func New(cfg model.Config, opts ...Option) (*Kronk, error) {
	if libraryLocation == "" {
		return nil, fmt.Errorf("new: the Init() function has not been called")
	}

	// -------------------------------------------------------------------------

	o := options{
		queueDepth: 2,
	}

	for _, opt := range opts {
		opt(&o)
	}

	if o.cataloger == nil {
		cataloger, err := catalog.New()
		if err != nil {
			return nil, fmt.Errorf("new: unable to create cataloger: %w", err)
		}

		o.cataloger = cataloger
	}

	ctx := context.Background()
	if o.ctx != nil {
		ctx = o.ctx
	}

	// -------------------------------------------------------------------------

	mdl, err := model.NewModel(ctx, o.cataloger, cfg)
	if err != nil {
		return nil, err
	}

	mi := mdl.ModelInfo()

	// -------------------------------------------------------------------------
	// Embed/rerank models use an internal context pool for parallelism.
	// Text/vision/audio models use the batch engine with queue depth.

	var semCapacity int

	switch {
	case mi.IsEmbedModel || mi.IsRerankModel:
		semCapacity = max(cfg.NSeqMax, 1)

	default:
		semCapacity = max(cfg.NSeqMax, 1) * o.queueDepth
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

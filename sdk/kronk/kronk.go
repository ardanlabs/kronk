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
	"github.com/ardanlabs/kronk/sdk/tools/templates"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Version contains the current version of the kronk package.
const Version = "1.15.1"

// =============================================================================

type options struct {
	tr         model.TemplateRetriever
	ctx        context.Context
	queueDepth int
}

// Option represents options for configuring Kronk.
type Option func(*options)

// WithTemplateRetriever sets a custom Github repo for templates.
// If not set, the default repo will be used.
func WithTemplateRetriever(templates model.TemplateRetriever) Option {
	return func(o *options) {
		o.tr = templates
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
	models        []*model.Model
	pool          chan *model.Model
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

	if o.tr == nil {
		templs, err := templates.New()
		if err != nil {
			return nil, fmt.Errorf("new: unable to create template: %w", err)
		}

		o.tr = templs
	}

	ctx := context.Background()
	if o.ctx != nil {
		ctx = o.ctx
	}

	// -------------------------------------------------------------------------
	// Determine if this is a sequential model (embed/rerank/vision) that
	// benefits from instance pooling rather than batch parallelism.

	// We need to check model info, so create the first instance.
	firstModel, err := model.NewModel(ctx, o.tr, cfg)
	if err != nil {
		return nil, err
	}

	isSingleFlight := cfg.ProjFile != ""

	mi := firstModel.ModelInfo()
	if mi.IsEmbedModel || mi.IsRerankModel {
		isSingleFlight = true
	}

	// -------------------------------------------------------------------------
	// For sequential models with NSeqMax > 1, create a pool of model instances.
	// For text models, NSeqMax controls batch parallelism within a single instance.

	var (
		models      = []*model.Model{firstModel}
		pool        chan *model.Model
		semCapacity int
	)

	switch {
	case isSingleFlight:
		numInstances := max(cfg.NSeqMax, 1)
		semCapacity = numInstances

		if numInstances > 1 {
			pool = make(chan *model.Model, numInstances)
			pool <- firstModel

			for range numInstances - 1 {
				m, err := model.NewModel(ctx, o.tr, cfg)
				if err != nil {
					for _, mdl := range models {
						mdl.Unload(ctx)
					}
					return nil, err
				}
				models = append(models, m)
				pool <- m
			}
		}

	default:
		semCapacity = max(cfg.NSeqMax, 1) * o.queueDepth
	}

	// -------------------------------------------------------------------------

	krn := Kronk{
		cfg:       firstModel.Config(),
		models:    models,
		pool:      pool,
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

	var errs []error
	for _, m := range krn.models {
		if err := m.Unload(ctx); err != nil {
			errs = append(errs, fmt.Errorf("unload: failed to unload model, model-id[%s]: %w", m.ModelInfo().ID, err))
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

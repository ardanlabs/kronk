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
const Version = "1.12.6"

// =============================================================================

type options struct {
	tr  model.TemplateRetriever
	ctx context.Context
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

// =============================================================================

// Kronk provides a concurrently safe api for using llama.cpp to access models.
type Kronk struct {
	cfg           model.Config
	models        chan *model.Model
	activeStreams atomic.Int32
	shutdown      sync.Mutex
	shutdownFlag  bool
	modelInfo     model.ModelInfo
}

// New provides the ability to use models in a concurrently safe way.
//
// modelInstances represents the number of instances of the model to create. Unless
// you have more than 1 GPU, the recommended number of instances is 1.
func New(modelInstances int, cfg model.Config, opts ...Option) (*Kronk, error) {
	if libraryLocation == "" {
		return nil, fmt.Errorf("the Init() function has not been called")
	}

	if modelInstances <= 0 {
		return nil, fmt.Errorf("instances must be > 0, got %d", modelInstances)
	}

	// -------------------------------------------------------------------------

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	if o.tr == nil {
		templs, err := templates.New()
		if err != nil {
			return nil, fmt.Errorf("template new: %w", err)
		}

		o.tr = templs
	}

	ctx := context.Background()
	if o.ctx != nil {
		ctx = o.ctx
	}

	// -------------------------------------------------------------------------

	models := make(chan *model.Model, modelInstances)
	var firstModel *model.Model

	for range modelInstances {
		m, err := model.NewModel(ctx, o.tr, cfg)
		if err != nil {
			close(models)
			for mdl := range models {
				mdl.Unload(ctx)
			}

			return nil, err
		}

		models <- m

		if firstModel == nil {
			firstModel = m
		}
	}

	if firstModel == nil {
		return nil, fmt.Errorf("no models loaded")
	}

	krn := Kronk{
		cfg:       firstModel.Config(),
		models:    models,
		modelInfo: firstModel.ModelInfo(),
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

// Unload will close down all loaded models. You should call this only when you
// are completely done using the group.
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
			return fmt.Errorf("unload:already unloaded")
		}

		for krn.activeStreams.Load() > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("unload:cannot unload: %d active streams: %w", krn.activeStreams.Load(), ctx.Err())

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

	var sb strings.Builder

	close(krn.models)
	for mdl := range krn.models {
		if err := mdl.Unload(ctx); err != nil {
			sb.WriteString(fmt.Sprintf("unload:failed to unload model: %s: %v\n", mdl.ModelInfo().ID, err))
		}
	}

	if sb.Len() > 0 {
		return fmt.Errorf("%s", sb.String())
	}

	return nil
}

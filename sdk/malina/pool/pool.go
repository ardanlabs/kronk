// Package pool manages pooled Malina stable-diffusion model handles.
package pool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	malinasdk "github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/pool/engine"
	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

// ErrServerBusy reports that no idle model can be evicted for a new load.
var ErrServerBusy = engine.ErrServerBusy

// ErrNoCapacity reports that a model cannot fit within the memory budget.
var ErrNoCapacity = errors.New("malina pool: insufficient memory budget")

const defaultModelsInPool = 10

// Config configures the Malina model pool.
type Config struct {
	Log          applog.Logger
	Models       *malinamodels.Models
	Resman       *resman.Manager
	ModelsInPool int
	TTL          time.Duration
}

// Pool manages stable-diffusion model handles backed by the generic pool
// engine and a shared resource manager.
type Pool struct {
	engine *engine.Pool[*malinasdk.Malina]
	loader *StableDiffusion
	models *malinamodels.Models
	resman *resman.Manager
}

// New constructs a Malina model pool.
func New(cfg Config) (*Pool, error) {
	if cfg.Log == nil {
		return nil, errors.New("new: log is required")
	}
	if cfg.Models == nil {
		return nil, errors.New("new: models is required")
	}
	if cfg.Resman == nil {
		return nil, errors.New("new: resman is required")
	}
	if cfg.ModelsInPool <= 0 {
		cfg.ModelsInPool = defaultModelsInPool
	}
	if cfg.TTL < 0 {
		return nil, errors.New("new: ttl must be >= 0")
	}

	ml := newStableDiffusion(cfg.Log, cfg.Models, cfg.Resman)
	core, err := engine.New(engine.Config{
		Log:      cfg.Log,
		Resman:   cfg.Resman,
		MaxItems: cfg.ModelsInPool,
		TTL:      cfg.TTL,
	}, ml)
	if err != nil {
		return nil, fmt.Errorf("new: constructing pool engine: %w", err)
	}

	p := Pool{
		engine: core,
		loader: ml,
		models: cfg.Models,
		resman: cfg.Resman,
	}

	return &p, nil
}

// ResourceManager returns the shared resource manager.
func (p *Pool) ResourceManager() *resman.Manager {
	return p.resman
}

// AcquireModel returns the named model, loading and reserving it when needed.
func (p *Pool) AcquireModel(ctx context.Context, modelID string) (*malinasdk.Malina, error) {
	handle, err := p.engine.Acquire(ctx, loader.LoadRequest{ModelID: modelID, Key: modelID})
	if err != nil {
		if errors.Is(err, resman.ErrNoCapacity) {
			return nil, errors.Join(ErrNoCapacity, err)
		}
		return nil, err
	}

	return handle, nil
}

// AquireModel returns the named model. Deprecated: use AcquireModel.
func (p *Pool) AquireModel(ctx context.Context, modelID string) (*malinasdk.Malina, error) {
	return p.AcquireModel(ctx, modelID)
}

// GetExisting returns a loaded handle without loading it.
func (p *Pool) GetExisting(key string) (*malinasdk.Malina, bool) {
	return p.engine.GetExisting(key)
}

// Invalidate removes a model asynchronously.
func (p *Pool) Invalidate(key string) {
	p.engine.Invalidate(key)
}

// InvalidateSync removes a model and waits for its reservation to be released.
func (p *Pool) InvalidateSync(ctx context.Context, key string) error {
	return p.engine.InvalidateSync(ctx, key)
}

// Shutdown unloads every model and releases every reservation.
func (p *Pool) Shutdown(ctx context.Context) error {
	return p.engine.Shutdown(ctx)
}

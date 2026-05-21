// Package pool manages a pool of bucky APIs for specific whisper models.
// Used by the model server to manage the number of models that are
// maintained in memory at any given time.
//
// The pool reuses the same resman.Manager instance as the llama pool
// so VRAM and RAM accounting is unified across every backend running
// on the host.
package pool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/pool/engine"
	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// ErrServerBusy is returned when all model slots are occupied with
// active streams. It aliases the core sentinel so errors.Is works
// across both packages.
var ErrServerBusy = engine.ErrServerBusy

// Config represents settings for the bucky (whisper) pool.
//
// Models is the pre-built whisper catalog the pool consults for path
// resolution. Required.
//
// Resman is the shared resource manager. Building it outside the pool
// lets every backend (kronk, bucky, …) charge the same byte budget.
// Required.
//
// ModelsInPool and TTL fall back to defaults when zero.
type Config struct {
	Log          applog.Logger
	Models       *buckymodels.Models
	Resman       *resman.Manager
	ModelsInPool int
	TTL          time.Duration
}

// Default config values applied when the corresponding field is zero.
const (
	defaultModelsInPool = 10
	defaultTTL          = 5 * time.Minute
)

func validateConfig(cfg Config) (Config, error) {
	if cfg.Log == nil {
		return Config{}, errors.New("log is required")
	}
	if cfg.Models == nil {
		return Config{}, errors.New("models is required")
	}
	if cfg.Resman == nil {
		return Config{}, errors.New("resman is required")
	}

	if cfg.ModelsInPool <= 0 {
		cfg.ModelsInPool = defaultModelsInPool
	}
	if cfg.TTL <= 0 {
		cfg.TTL = defaultTTL
	}

	return cfg, nil
}

// =============================================================================

// Pool manages a set of *bucky.Bucky handles. It maintains a cache of
// these handles and unloads them on TTL or capacity overflow.
type Pool struct {
	engine *engine.Pool[*bucky.Bucky]
	loader *Whisper
	models *buckymodels.Models
	resman *resman.Manager
}

// New constructs the bucky pool for use.
func New(cfg Config) (*Pool, error) {
	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
	}

	wl := newWhisper(cfg.Log, cfg.Models, cfg.Resman)

	c, err := engine.New(engine.Config{
		Log:      cfg.Log,
		Resman:   cfg.Resman,
		MaxItems: cfg.ModelsInPool,
		TTL:      cfg.TTL,
	}, wl)
	if err != nil {
		return nil, fmt.Errorf("new: constructing pool core: %w", err)
	}

	p := Pool{
		engine: c,
		loader: wl,
		models: cfg.Models,
		resman: cfg.Resman,
	}

	return &p, nil
}

// ResourceManager returns the pool's underlying resource manager.
func (p *Pool) ResourceManager() *resman.Manager {
	return p.resman
}

// Shutdown releases all handles from the pool and performs a proper
// unloading.
func (p *Pool) Shutdown(ctx context.Context) error {
	return p.engine.Shutdown(ctx)
}

// AquireModel will provide a bucky handle for the specified model. If
// the model is not in the pool, a handle for the model will be created.
func (p *Pool) AquireModel(ctx context.Context, modelID string) (*bucky.Bucky, error) {
	return p.engine.Acquire(ctx, loader.LoadRequest{
		ModelID: modelID,
		Key:     modelID,
	})
}

// GetExisting returns a pooled handle if it exists, without creating
// one.
func (p *Pool) GetExisting(key string) (*bucky.Bucky, bool) {
	return p.engine.GetExisting(key)
}

// Invalidate removes a single entry from the pool, triggering unload
// asynchronously.
func (p *Pool) Invalidate(key string) {
	p.engine.Invalidate(key)
}

// InvalidateSync invalidates a cache entry and waits for the eviction
// callback to release the underlying resource manager reservation.
func (p *Pool) InvalidateSync(ctx context.Context, key string) error {
	return p.engine.InvalidateSync(ctx, key)
}

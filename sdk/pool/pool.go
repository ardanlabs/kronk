// Package pool manages a pool of kronk APIs for specific llama models.
// Used by the model server to manage the number of models that are
// maintained in memory at any given time.
//
// The pool is a thin, llama-typed wrapper around the generic engine in
// sdk/pool/internal/core. The cache, eviction policy, budget
// reservations, and concurrent-load deduplication live in the core;
// the llama-specific planning, loading, and display logic lives in
// sdk/kronk/poolloader. When additional backends (whisper, …) ship,
// they will provide a sibling wrapper of this file around the same
// core engine.
package pool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/poolloader"
	"github.com/ardanlabs/kronk/sdk/pool/internal/core"
	"github.com/ardanlabs/kronk/sdk/pool/loader"
	"github.com/ardanlabs/kronk/sdk/pool/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// ErrServerBusy is returned when all model slots are occupied with
// active streams. It aliases the core sentinel so errors.Is works
// across both packages.
var ErrServerBusy = core.ErrServerBusy

// HumanBytes formats a byte count using decimal (SI) units. It aliases
// the core helper so existing callers of pool.HumanBytes keep working.
func HumanBytes(n int64) string {
	return core.HumanBytes(n)
}

// Config represents settings for the pool.
//
// BudgetPercent: Percentage (1..100) of detected GPU VRAM and system
// RAM that the pool's resource manager is allowed to commit to loaded
// models. Defaults to defaultBudgetPercent (80) when zero. This is the
// primary admission knob.
//
// ModelsInPool: Safety-net cap on the number of distinct entries the
// pool will keep, independent of the byte budget. Defaults to 10 when
// zero.
//
// TTL: Defines the time an existing model can live in the pool without
// being used. Defaults to 5 minutes if the value is 0.
//
// Snapshot: Optional resource snapshot used to construct the resource
// manager. When nil the pool calls devices.List() at construction
// time. Tests use this to inject a deterministic device topology.
//
// InsecureLogging: When true, logs potentially sensitive data such as
// message content and detailed model configuration.
type Config struct {
	Log             kronk.Logger
	BasePath        string
	ModelConfigFile string
	ModelsInPool    int
	BudgetPercent   int
	TTL             time.Duration
	Snapshot        *resman.Snapshot
	InsecureLogging bool
}

// Default config values applied when the corresponding field is zero.
const (
	defaultBudgetPercent = 80
	defaultModelsInPool  = 10
	defaultTTL           = 5 * time.Minute
)

func validateConfig(cfg Config) Config {
	if cfg.BudgetPercent <= 0 {
		cfg.BudgetPercent = defaultBudgetPercent
	}

	if cfg.ModelsInPool <= 0 {
		cfg.ModelsInPool = defaultModelsInPool
	}

	if cfg.TTL <= 0 {
		cfg.TTL = defaultTTL
	}

	return cfg
}

// =============================================================================

// Pool manages a set of Kronk APIs for use. It maintains a pool of
// these APIs and will unload them over time if not in use.
type Pool struct {
	core   *core.Core[*kronk.Kronk]
	llama  *poolloader.Llama
	models *models.Models
	resman *resman.Manager
}

// New constructs the pool for use.
func New(cfg Config) (*Pool, error) {
	cfg = validateConfig(cfg)

	mdls, err := models.NewWithPaths(cfg.BasePath)
	if err != nil {
		return nil, fmt.Errorf("new: creating models system: %w", err)
	}

	var mc map[string]models.ModelConfig
	if cfg.ModelConfigFile != "" {
		mc, err = models.LoadModelConfig(cfg.ModelConfigFile)
		if err != nil {
			return nil, fmt.Errorf("new: loading model config: %w", err)
		}
	}
	if mc == nil {
		mc = map[string]models.ModelConfig{}
	}

	var snap resman.Snapshot
	switch {
	case cfg.Snapshot != nil:
		snap = *cfg.Snapshot
	default:
		snap = resman.FromDevices(devices.List())
	}

	rm, err := resman.New(resman.Config{
		Snapshot:      snap,
		BudgetPercent: cfg.BudgetPercent,
	})
	if err != nil {
		return nil, fmt.Errorf("new: constructing resource manager: %w", err)
	}

	llama := poolloader.New(cfg.Log, mdls, mc, rm, cfg.InsecureLogging)

	c, err := core.New(core.Config{
		Log:      cfg.Log,
		Resman:   rm,
		MaxItems: cfg.ModelsInPool,
		TTL:      cfg.TTL,
	}, llama)
	if err != nil {
		return nil, fmt.Errorf("new: constructing pool core: %w", err)
	}

	p := Pool{
		core:   c,
		llama:  llama,
		models: mdls,
		resman: rm,
	}

	c.LogResmanInit(context.Background())

	return &p, nil
}

// ResourceManager returns the pool's underlying resource manager.
func (p *Pool) ResourceManager() *resman.Manager {
	return p.resman
}

// Shutdown releases all apis from the pool and performs a proper
// unloading.
func (p *Pool) Shutdown(ctx context.Context) error {
	return p.core.Shutdown(ctx)
}

// AquireModel will provide a kronk API for the specified model. If
// the model is not in the pool, an API for the model will be created.
func (p *Pool) AquireModel(ctx context.Context, modelID string) (*kronk.Kronk, error) {
	return p.core.Acquire(ctx, loader.LoadRequest{
		ModelID: modelID,
		Key:     modelID,
	})
}

// AquireCustom will provide a kronk API for a model using a pre-built
// config. This bypasses the normal catalog resolution path. The key
// should use format <modelID>/playground/<session_id> so that
// ModelStatus can still match playground sessions to locally installed
// models.
func (p *Pool) AquireCustom(ctx context.Context, key string, cfg model.Config) (*kronk.Kronk, error) {
	modelID, _, _ := strings.Cut(key, "/")
	return p.core.Acquire(ctx, loader.LoadRequest{
		ModelID: modelID,
		Key:     key,
		Custom:  cfg,
	})
}

// ModelConfig returns the loaded per-model configuration overrides.
func (p *Pool) ModelConfig() map[string]models.ModelConfig {
	return p.llama.ModelConfig()
}

// GetExisting returns a pooled model if it exists, without creating
// one.
func (p *Pool) GetExisting(key string) (*kronk.Kronk, bool) {
	return p.core.GetExisting(key)
}

// Invalidate removes a single entry from the pool, triggering unload.
//
// This is fire-and-forget: the eviction callback runs asynchronously,
// so the resource manager's reservation may not be released by the
// time this returns. Callers that need a consistent post-eviction view
// of the pool should use InvalidateSync instead.
func (p *Pool) Invalidate(key string) {
	p.core.Invalidate(key)
}

// InvalidateSync invalidates a cache entry and waits for the eviction
// callback to release the underlying resource manager reservation.
func (p *Pool) InvalidateSync(ctx context.Context, key string) error {
	return p.core.InvalidateSync(ctx, key)
}

// =============================================================================

// ModelStatus returns information about the current models in the
// pool.
//
// The result includes both fully loaded models (entries currently in
// the cache) and in-flight loads (memory reservations that have not
// yet completed their GGUF read). The latter are returned with
// Status=ModelStatusLoading so BUI/observability can show them as
// occupying budget while still being unavailable to serve requests.
func (p *Pool) ModelStatus() ([]ModelDetail, error) {
	list, err := p.models.Files()
	if err != nil {
		return nil, err
	}

	ps := make([]ModelDetail, 0)
	loadedKeys := make(map[string]struct{})

entries:
	for entry := range p.core.Coldest() {
		cacheID, _, _ := strings.Cut(entry.Key, "/")

		for _, mi := range list {
			if mi.ID != cacheID {
				continue
			}

			krn := entry.Value
			disp := p.llama.Display(krn, cacheID)

			ps = append(ps, ModelDetail{
				ID:            entry.Key,
				OwnedBy:       mi.OwnedBy,
				ModelFamily:   mi.ModelFamily,
				Size:          mi.Size,
				VRAMTotal:     disp.VRAMTotal,
				KVCache:       disp.KVCache,
				Slots:         max(disp.Slots, 1),
				ExpiresAt:     entry.ExpiresAt(),
				ActiveStreams: krn.ActiveStreams(),
				Status:        ModelStatusLoaded,
			})
			loadedKeys[entry.Key] = struct{}{}
			continue entries
		}
	}

	// Surface any in-flight reservations (memory accounted for by the
	// resource manager but the underlying kronk.Kronk has not finished
	// loading and is not yet in the cache). Without this, the
	// "Active Reservations" panel and the "Running Models" grid
	// disagree during the SHA-verify + GGUF-read window.
	for _, r := range p.resman.Usage().Reservations {
		if _, ok := loadedKeys[r.Key]; ok {
			continue
		}

		cacheID, _, _ := strings.Cut(r.Key, "/")

		var ownedBy, modelFamily string
		var size int64
		for _, mi := range list {
			if mi.ID == cacheID {
				ownedBy = mi.OwnedBy
				modelFamily = mi.ModelFamily
				size = mi.Size
				break
			}
		}

		ps = append(ps, ModelDetail{
			ID:          r.Key,
			OwnedBy:     ownedBy,
			ModelFamily: modelFamily,
			Size:        size,
			VRAMTotal:   r.VRAMBytes + r.RAMBytes,
			Status:      ModelStatusLoading,
		})
	}

	return ps, nil
}

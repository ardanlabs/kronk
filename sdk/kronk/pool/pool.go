// Package pool manages a pool of kronk APIs for specific llama models.
// Used by the model server to manage the number of models that are
// maintained in memory at any given time.
//
// The pool is a thin, llama-typed wrapper around the generic engine in
// sdk/pool. The cache, eviction policy, budget reservations, and
// concurrent-load deduplication live in the core; the llama-specific
// planning, loading, and display logic lives in this package's
// llama.go (which implements loader.Loader[*kronk.Kronk]). Sibling
// wrappers in sdk/bucky/pool (whisper) follow the same shape and
// share the resman.Manager passed in via Config.Resman so VRAM/RAM
// budgeting is unified across every backend.
package pool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/pool/engine"
	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// ErrServerBusy is returned when all model slots are occupied with
// active streams. It aliases the core sentinel so errors.Is works
// across both packages.
var ErrServerBusy = engine.ErrServerBusy

// HumanBytes formats a byte count using decimal (SI) units. It aliases
// the core helper so existing callers of pool.HumanBytes keep working.
func HumanBytes(n int64) string {
	return engine.HumanBytes(n)
}

// Config represents settings for the kronk (llama) pool.
//
// Models is the pre-built catalog the pool consults for path / size
// resolution. Required.
//
// Resman is the shared resource manager. Building it outside the pool
// lets every backend (kronk, bucky, …) charge the same byte budget.
// Required.
//
// ModelConfigFile is the optional per-model override file. Empty means
// no overrides.
//
// ModelsInPool is the safety-net cap on the number of distinct entries
// the pool keeps, independent of the byte budget. Defaults to 10 when
// zero.
//
// TTL is the time an existing model can live in the pool without being
// used. Defaults to 5 minutes when zero.
//
// InsecureLogging, when true, logs potentially sensitive data such as
// message content and detailed model configuration.
type Config struct {
	Log             kronk.Logger
	Models          *models.Models
	Resman          *resman.Manager
	ModelConfigFile string
	ModelsInPool    int
	TTL             time.Duration
	InsecureLogging bool
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

// Pool manages a set of Kronk APIs for use. It maintains a pool of
// these APIs and will unload them over time if not in use.
type Pool struct {
	engine *engine.Pool[*kronk.Kronk]
	llama  *Llama
	models *models.Models
	resman *resman.Manager
}

// New constructs the pool for use.
func New(cfg Config) (*Pool, error) {
	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
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

	llama := newLlama(cfg.Log, cfg.Models, mc, cfg.Resman, cfg.InsecureLogging)

	c, err := engine.New(engine.Config{
		Log:      cfg.Log,
		Resman:   cfg.Resman,
		MaxItems: cfg.ModelsInPool,
		TTL:      cfg.TTL,
	}, llama)
	if err != nil {
		return nil, fmt.Errorf("new: constructing pool core: %w", err)
	}

	p := Pool{
		engine: c,
		llama:  llama,
		models: cfg.Models,
		resman: cfg.Resman,
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
	return p.engine.Shutdown(ctx)
}

// AquireModel will provide a kronk API for the specified model. If
// the model is not in the pool, an API for the model will be created.
func (p *Pool) AquireModel(ctx context.Context, modelID string) (*kronk.Kronk, error) {
	return p.engine.Acquire(ctx, loader.LoadRequest{
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
	return p.engine.Acquire(ctx, loader.LoadRequest{
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
	return p.engine.GetExisting(key)
}

// Invalidate removes a single entry from the pool, triggering unload.
//
// This is fire-and-forget: the eviction callback runs asynchronously,
// so the resource manager's reservation may not be released by the
// time this returns. Callers that need a consistent post-eviction view
// of the pool should use InvalidateSync instead.
func (p *Pool) Invalidate(key string) {
	p.engine.Invalidate(key)
}

// InvalidateSync invalidates a cache entry and waits for the eviction
// callback to release the underlying resource manager reservation.
func (p *Pool) InvalidateSync(ctx context.Context, key string) error {
	return p.engine.InvalidateSync(ctx, key)
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
//
// Cache keys may be the bare catalog ID, or any of the variants
// accepted by Models.LookupFile (e.g. "<org>/<model>",
// "<model>/<variant>", "<org>/<model>/<variant>"), so the catalog
// resolver is used to recover the row metadata rather than splitting
// the key here.
func (p *Pool) ModelStatus() ([]ModelDetail, error) {
	ps := make([]ModelDetail, 0)
	loadedKeys := make(map[string]struct{})

	for entry := range p.engine.Coldest() {
		mi, ok := p.models.LookupFile(entry.Key)
		if !ok {
			continue
		}

		krn := entry.Value
		disp := p.llama.Display(krn, mi.ID)

		ps = append(ps, ModelDetail{
			ID:            entry.Key,
			Backend:       "kronk",
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
	}

	// Surface any in-flight reservations (memory accounted for by the
	// resource manager but the underlying kronk.Kronk has not finished
	// loading and is not yet in the cache). Without this, the
	// "Active Reservations" panel and the "Running Models" grid
	// disagree during the SHA-verify + GGUF-read window.
	//
	// The resman is shared across backends, so filter by
	// p.engine.HasTicket to only surface kronk's own in-flight loads.
	// Without this guard, bucky reservations leak in as fake "loading"
	// kronk entries with no size/owner/family populated.
	for _, r := range p.resman.Usage().Reservations {
		if _, ok := loadedKeys[r.Key]; ok {
			continue
		}
		if !p.engine.HasTicket(r.Key) {
			continue
		}

		var ownedBy, modelFamily string
		var size int64
		if mi, ok := p.models.LookupFile(r.Key); ok {
			ownedBy = mi.OwnedBy
			modelFamily = mi.ModelFamily
			size = mi.Size
		}

		ps = append(ps, ModelDetail{
			ID:          r.Key,
			Backend:     "kronk",
			OwnedBy:     ownedBy,
			ModelFamily: modelFamily,
			Size:        size,
			VRAMTotal:   r.VRAMBytes + r.RAMBytes,
			Status:      ModelStatusLoading,
		})
	}

	return ps, nil
}

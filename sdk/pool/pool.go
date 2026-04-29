// Package pool manages a pool of kronk APIs for specific models. Used by
// the model server to manage the number of models that are maintained in
// memory at any given time.
package pool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/maypok86/otter/v2"
	"golang.org/x/sync/singleflight"
)

// ErrServerBusy is returned when all model slots are occupied with active streams.
var ErrServerBusy = errors.New("server busy: all model slots have active requests")

// Config represents setting for the kronk manager.
//
// ModelsInCache: Defines the maximum number of unique models that will be
// available at a time. Defaults to 2 if the value is 0.
//
// CacheTTL: Defines the time an existing model can live in the pool without
// being used. Defaults to 5 minutes if the value is 0.
//
// InsecureLogging: When true, logs potentially sensitive data such as message
// content and detailed model configuration.
type Config struct {
	Log             kronk.Logger
	BasePath        string
	ModelConfigFile string
	ModelsInCache   int
	CacheTTL        time.Duration
	InsecureLogging bool
}

func validateConfig(cfg Config) (Config, error) {
	if cfg.ModelsInCache <= 0 {
		cfg.ModelsInCache = 2
	}

	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}

	return cfg, nil
}

// =============================================================================

// Pool manages a set of Kronk APIs for use. It maintains a pool of these
// APIs and will unload over time if not in use.
type Pool struct {
	log              kronk.Logger
	modelConfig      map[string]models.ModelConfig
	cache            *otter.Cache[string, *kronk.Kronk]
	itemsInCache     atomic.Int32
	maxModelsInCache int
	models           *models.Models
	insecureLogging  bool
	loadGroup        singleflight.Group
}

// New constructs the manager for use.
func New(cfg Config) (*Pool, error) {
	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}

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

	p := Pool{
		log:              cfg.Log,
		modelConfig:      mc,
		maxModelsInCache: cfg.ModelsInCache,
		models:           mdls,
		insecureLogging:  cfg.InsecureLogging,
	}

	opt := otter.Options[string, *kronk.Kronk]{
		MaximumSize:      cfg.ModelsInCache,
		ExpiryCalculator: otter.ExpiryAccessing[string, *kronk.Kronk](cfg.CacheTTL),
		OnDeletion:       p.eviction,
	}

	cache, err := otter.New(&opt)
	if err != nil {
		return nil, fmt.Errorf("new: constructing cache: %w", err)
	}

	p.cache = cache

	return &p, nil
}

// Shutdown releases all apis from the pool and performs a proper unloading.
func (p *Pool) Shutdown(ctx context.Context) error {
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}

	p.cache.InvalidateAll()

	for p.itemsInCache.Load() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-time.NewTimer(100 * time.Millisecond).C:
		}
	}

	return nil
}

// ModelStatus returns information about the current models in the pool.
func (p *Pool) ModelStatus() ([]ModelDetail, error) {

	// Extract the entries currently in the pool.
	var entries []otter.Entry[string, *kronk.Kronk]
	for entry := range p.cache.Coldest() {
		entries = append(entries, entry)
	}

	// Retrieve the models installed locally.
	list, err := p.models.Files()
	if err != nil {
		return nil, err
	}

	// Match the model in the pool with a locally stored model
	// so we can get information about that model.
	ps := make([]ModelDetail, 0, len(entries))
ids:
	for _, model := range entries {
		cacheID, _, _ := strings.Cut(model.Key, "/")
		for _, mi := range list {
			if mi.ID == cacheID {
				krn := model.Value
				ps = append(ps, ModelDetail{
					ID:            model.Key,
					OwnedBy:       mi.OwnedBy,
					ModelFamily:   mi.ModelFamily,
					Size:          mi.Size,
					VRAMTotal:     krn.ModelInfo().VRAMTotal,
					KVCache:       krn.ModelInfo().SlotMemory,
					Slots:         max(krn.ModelConfig().NSeqMax(), 1),
					ExpiresAt:     model.ExpiresAt(),
					ActiveStreams: krn.ActiveStreams(),
				})
				continue ids
			}
		}
	}

	return ps, nil
}

// AquireModel will provide a kronk API for the specified model. If the model
// is not in the pool, an API for the model will be created.
func (p *Pool) AquireModel(ctx context.Context, modelID string) (*kronk.Kronk, error) {
	if entry, exists := p.cache.GetEntry(modelID); exists {
		p.log(ctx, "acquire-model",
			"status", "cache-hit",
			"key", modelID,
			"ttl-reset", true,
			"expires-at", entry.ExpiresAt(),
			"active-streams", entry.Value.ActiveStreams(),
		)
		return entry.Value, nil
	}

	p.log(ctx, "acquire-model",
		"status", "cache-miss",
		"key", modelID,
	)

	if p.allSlotsActive() {
		return nil, ErrServerBusy
	}

	// Use singleflight to prevent concurrent loads of the same model.
	// This ensures only one goroutine loads a model while others wait.
	result, err, _ := p.loadGroup.Do(modelID, func() (any, error) {

		// Double-check pool after acquiring the singleflight lock.
		if krn, exists := p.cache.GetIfPresent(modelID); exists {
			return krn, nil
		}

		cfg, err := p.models.KronkResolvedConfig(modelID, p.modelConfig)
		if err != nil {
			return nil, fmt.Errorf("acquire-model: unable to retrieve model config: %w", err)
		}

		if p.insecureLogging {
			cfg.PtrInsecureLogging = new(true)
		}

		cfg.Log = p.log

		// Free a slot up-front (if needed) so the new model is not loaded
		// while an old one is still resident in memory.
		if err := p.evictForCapacity(ctx, modelID); err != nil {
			return nil, fmt.Errorf("acquire-model: %w", err)
		}

		krn, err := kronk.NewWithContext(ctx,
			model.WithConfig(cfg),
		)

		if err != nil {
			return nil, fmt.Errorf("acquire-model: unable to create inference model: %w", err)
		}

		p.cache.Set(modelID, krn)
		p.itemsInCache.Add(1)

		if entry, ok := p.cache.GetEntryQuietly(modelID); ok {
			p.log(ctx, "acquire-model",
				"status", "cache-set",
				"key", modelID,
				"expires-at", entry.ExpiresAt(),
				"ttl", entry.ExpiresAfter(),
			)
		}

		totalEntries := len(krn.SystemInfo())*2 + (5 * 2)
		info := make([]any, 0, totalEntries)
		for k, v := range krn.SystemInfo() {
			info = append(info, k)
			info = append(info, v)
		}

		info = append(info, "status")
		info = append(info, "load new model")
		info = append(info, "model-name")
		info = append(info, modelID)
		info = append(info, "contextWindow")
		info = append(info, krn.ModelConfig().ContextWindow())
		info = append(info, "isGPTModel")
		info = append(info, krn.ModelInfo().IsGPTModel)
		info = append(info, "isEmbedModel")
		info = append(info, krn.ModelInfo().IsEmbedModel)
		info = append(info, "isRerankModel")
		info = append(info, krn.ModelInfo().IsRerankModel)

		p.log(ctx, "acquire-model", info...)

		return krn, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*kronk.Kronk), nil
}

// AquireCustom will provide a kronk API for a model using a pre-built config.
// This bypasses the normal catalog resolution path. The key should use format
// <modelID>/playground/<session_id> so that ModelStatus() can still match
// playground sessions to locally installed models.
func (p *Pool) AquireCustom(ctx context.Context, key string, cfg model.Config) (*kronk.Kronk, error) {
	if entry, exists := p.cache.GetEntry(key); exists {
		p.log(ctx, "acquire-custom",
			"status", "cache-hit",
			"key", key,
			"ttl-reset", true,
			"expires-at", entry.ExpiresAt(),
			"active-streams", entry.Value.ActiveStreams(),
		)
		return entry.Value, nil
	}

	p.log(ctx, "acquire-custom",
		"status", "cache-miss",
		"key", key,
	)

	if p.allSlotsActive() {
		return nil, ErrServerBusy
	}

	result, err, _ := p.loadGroup.Do(key, func() (any, error) {
		if krn, exists := p.cache.GetIfPresent(key); exists {
			return krn, nil
		}

		if p.insecureLogging {
			cfg.PtrInsecureLogging = new(true)
		}

		cfg.Log = p.log

		// Free a slot up-front (if needed) so the new model is not loaded
		// while an old one is still resident in memory.
		if err := p.evictForCapacity(ctx, key); err != nil {
			return nil, fmt.Errorf("acquire-custom: %w", err)
		}

		krn, err := kronk.NewWithContext(ctx,
			model.WithConfig(cfg),
		)

		if err != nil {
			return nil, fmt.Errorf("acquire-custom: unable to create inference model: %w", err)
		}

		p.cache.Set(key, krn)
		p.itemsInCache.Add(1)

		if entry, ok := p.cache.GetEntryQuietly(key); ok {
			p.log(ctx, "acquire-custom",
				"status", "cache-set",
				"key", key,
				"expires-at", entry.ExpiresAt(),
				"ttl", entry.ExpiresAfter(),
			)
		}

		p.log(ctx, "acquire-custom", "status", "load new model", "key", key, "contextWindow", krn.ModelConfig().ContextWindow())

		return krn, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*kronk.Kronk), nil
}

// ModelConfig returns the loaded per-model configuration overrides.
func (p *Pool) ModelConfig() map[string]models.ModelConfig {
	return p.modelConfig
}

// GetExisting returns a pooled model if it exists, without creating one.
func (p *Pool) GetExisting(key string) (*kronk.Kronk, bool) {
	krn, exists := p.cache.GetIfPresent(key)
	if !exists {
		return nil, false
	}
	return krn, true
}

// Invalidate removes a single entry from the pool, triggering unload.
func (p *Pool) Invalidate(key string) {
	p.cache.Invalidate(key)
}

// evictForCapacity makes room for a new model by evicting an idle model from
// the pool before the new one is loaded. This avoids a temporary peak where
// both models are resident in memory at the same time. It blocks until the
// victim's unload has completed.
//
// The newKey parameter is the key being loaded; it is excluded from victim
// selection (it shouldn't normally be in the pool, but this is defensive).
func (p *Pool) evictForCapacity(ctx context.Context, newKey string) error {
	const pollInterval = 25 * time.Millisecond
	const maxWait = 60 * time.Second

	if int(p.itemsInCache.Load()) < p.maxModelsInCache {
		return nil
	}

	// Pick the coldest entry that has no active streams as the victim.
	var victim string
	for entry := range p.cache.Coldest() {
		if entry.Key == newKey {
			continue
		}
		if entry.Value.ActiveStreams() == 0 {
			victim = entry.Key
			break
		}
	}

	if victim == "" {
		return ErrServerBusy
	}

	p.log(ctx, "acquire-model",
		"status", "evict-before-load",
		"victim", victim,
		"items-in-cache", p.itemsInCache.Load(),
		"max-models-in-cache", p.maxModelsInCache,
	)

	p.cache.Invalidate(victim)

	deadline := time.Now().Add(maxWait)
	for int(p.itemsInCache.Load()) >= p.maxModelsInCache {
		if time.Now().After(deadline) {
			return fmt.Errorf("evict-for-capacity: timeout waiting for victim[%s] to unload", victim)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	p.log(ctx, "acquire-model",
		"status", "evict-before-load-complete",
		"victim", victim,
		"items-in-cache", p.itemsInCache.Load(),
	)

	return nil
}

// allSlotsActive returns true if all model slots are occupied and every
// pooled model has at least one active stream.
func (p *Pool) allSlotsActive() bool {
	count := 0
	for entry := range p.cache.Hottest() {
		count++
		if entry.Value.ActiveStreams() == 0 {
			return false
		}
	}

	return count >= p.maxModelsInCache
}

func (p *Pool) eviction(event otter.DeletionEvent[string, *kronk.Kronk]) {
	const unloadTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), unloadTimeout)
	defer cancel()

	p.log(ctx, "kronk pool eviction", "key", event.Key, "cause", event.Cause.String(), "cause-code", int(event.Cause), "was-evicted", event.WasEvicted(), "active-streams", event.Value.ActiveStreams())

	// If there are active streams and this was an automatic eviction (not a replacement
	// from our own Set call below), re-insert the model to prevent eviction.
	// WasEvicted() returns false for CauseReplacement and CauseInvalidation.
	if event.Value.ActiveStreams() > 0 && event.WasEvicted() {
		p.log(ctx, "kronk pool eviction prevented", "key", event.Key, "active-streams", event.Value.ActiveStreams())
		p.cache.Set(event.Key, event.Value)
		return
	}

	// If this is a replacement event (from our Set above) and there are still active
	// streams, just return without unloading - the model is still in the pool.
	// For invalidation (shutdown), we still need to unload since the pool is being cleared.
	if event.Value.ActiveStreams() > 0 && event.Cause != otter.CauseInvalidation {
		p.log(ctx, "kronk pool eviction skipped (replacement with active streams)", "key", event.Key, "active-streams", event.Value.ActiveStreams())
		return
	}

	p.log(ctx, "kronk pool eviction", "key", event.Key, "status", "unload-started", "active-streams", event.Value.ActiveStreams())

	if err := event.Value.Unload(ctx); err != nil {
		p.log(ctx, "kronk pool eviction", "key", event.Key, "ERROR", err)
	}

	p.log(ctx, "kronk pool eviction", "key", event.Key, "status", "unload-finished")

	metrics.ClearVRAM(event.Key)

	p.itemsInCache.Add(-1)
}

// Package cache manages a cache of kronk APIs for specific models. Used by
// the model server to manage the number of models that are maintained in
// memory at any given time.
package cache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
	"github.com/maypok86/otter/v2"
)

// ErrServerBusy is returned when all model slots are occupied with active streams.
var ErrServerBusy = errors.New("server busy: all model slots have active requests")

// Config represents setting for the kronk manager.
//
// CatalogRepo represents the Github repo for where the catalog is. If left empty
// then api.github.com/repos/ardanlabs/kronk_catalogs/contents/catalogs is used.
//
// TemplateRepo represents the Github repo for where the templates are. If left empty
// then api.github.com/repos/ardanlabs/kronk_catalogs/contents/templates is used.
//
// MaxInCache: Defines the maximum number of unique models will be available at a
// time. Defaults to 3 if the value is 0.
//
// ModelInstances: Defines how many instances of the same model should be
// loaded. Defaults to 1 if the value is 0.
//
// ContextWindow: Sets the global context window for all models. Defaults to
// what is in the model metadata if set to 0. If no metadata is found, 4096
// is the default.
//
// CacheTTL: Defines the time an existing model can live in the cache without
// being used.
type Config struct {
	Log                  model.Logger
	BasePath             string
	Templates            *templates.Templates
	ModelsInCache        int
	CacheTTL             time.Duration
	IgnoreIntegrityCheck bool
}

func validateConfig(cfg Config) (Config, error) {
	if cfg.Templates == nil {
		templates, err := templates.New()
		if err != nil {
			return Config{}, err
		}

		cfg.Templates = templates
	}

	if cfg.ModelsInCache <= 0 {
		cfg.ModelsInCache = 3
	}

	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}

	return cfg, nil
}

// =============================================================================

// Cache manages a set of Kronk APIs for use. It maintains a cache of these
// APIs and will unload over time if not in use.
type Cache struct {
	log                  model.Logger
	templates            *templates.Templates
	cache                *otter.Cache[string, *kronk.Kronk]
	itemsInCache         atomic.Int32
	maxModelsInCache     int
	models               *models.Models
	ignoreIntegrityCheck bool
}

// New constructs the manager for use.
func New(cfg Config) (*Cache, error) {
	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}

	models, err := models.NewWithPaths(cfg.BasePath)
	if err != nil {
		return nil, fmt.Errorf("new: creating models system: %w", err)
	}

	c := Cache{
		log:                  cfg.Log,
		templates:            cfg.Templates,
		maxModelsInCache:     cfg.ModelsInCache,
		models:               models,
		ignoreIntegrityCheck: cfg.IgnoreIntegrityCheck,
	}

	opt := otter.Options[string, *kronk.Kronk]{
		MaximumSize:      cfg.ModelsInCache,
		ExpiryCalculator: otter.ExpiryAccessing[string, *kronk.Kronk](cfg.CacheTTL),
		OnDeletion:       c.eviction,
	}

	cache, err := otter.New(&opt)
	if err != nil {
		return nil, fmt.Errorf("new: constructing cache: %w", err)
	}

	c.cache = cache

	return &c, nil
}

// Shutdown releases all apis from the cache and performs a proper unloading.
func (c *Cache) Shutdown(ctx context.Context) error {
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
	}

	c.cache.InvalidateAll()

	for c.itemsInCache.Load() > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-time.NewTimer(100 * time.Millisecond).C:
		}
	}

	return nil
}

// ModelStatus returns information about the current models in the cache.
func (c *Cache) ModelStatus() ([]ModelDetail, error) {

	// Extract the entries currently in the cache.
	var entries []otter.Entry[string, *kronk.Kronk]
	for entry := range c.cache.Coldest() {
		entries = append(entries, entry)
	}

	// Retrieve the models installed locally.
	list, err := c.models.RetrieveFiles()
	if err != nil {
		return nil, err
	}

	// Match the model in the cache with a locally stored model
	// so we can get information about that model.
	ps := make([]ModelDetail, 0, len(entries))
ids:
	for _, model := range entries {
		for _, mi := range list {
			modelKey := strings.ToLower(model.Key) // Everything on disk is lowercase.
			if mi.ID == modelKey {
				ps = append(ps, ModelDetail{
					ID:            model.Key,
					OwnedBy:       mi.OwnedBy,
					ModelFamily:   mi.ModelFamily,
					Size:          mi.Size,
					ExpiresAt:     model.ExpiresAt(),
					ActiveStreams: model.Value.ActiveStreams(),
				})
				continue ids
			}
		}
	}

	return ps, nil
}

// AquireModel will provide a kronk API for the specified model. If the model
// is not in the cache, an API for the model will be created.
func (c *Cache) AquireModel(ctx context.Context, modelID string) (*kronk.Kronk, error) {
	krn, exists := c.cache.GetIfPresent(modelID)
	if exists {
		return krn, nil
	}

	if c.allSlotsActive() {
		return nil, ErrServerBusy
	}

	cfg, err := c.templates.Catalog().RetrieveModelConfig(modelID)
	if err != nil {
		return nil, fmt.Errorf("acquire-model: unable to retrieve model config: %w", err)
	}

	if c.ignoreIntegrityCheck {
		cfg.IgnoreIntegrityCheck = true
	}

	cfg.Log = c.log

	c.log(ctx, "model config settings", "model-id", modelID, "mc", cfg.String())

	krn, err = kronk.New(cfg,
		kronk.WithTemplateRetriever(c.templates),
		kronk.WithContext(ctx),
	)

	if err != nil {
		return nil, fmt.Errorf("acquire-model: unable to create inference model: %w", err)
	}

	modelID, _, _ = strings.Cut(modelID, "/")

	c.cache.Set(modelID, krn)
	c.itemsInCache.Add(1)

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
	info = append(info, krn.ModelConfig().ContextWindow)
	info = append(info, "isGPTModel")
	info = append(info, krn.ModelInfo().IsGPTModel)
	info = append(info, "isEmbedModel")
	info = append(info, krn.ModelInfo().IsEmbedModel)
	info = append(info, "isRerankModel")
	info = append(info, krn.ModelInfo().IsRerankModel)

	c.log(ctx, "acquire-model", info...)

	return krn, nil
}

// allSlotsActive returns true if all model slots are occupied and every
// cached model has at least one active stream.
func (c *Cache) allSlotsActive() bool {
	count := 0
	for entry := range c.cache.Hottest() {
		count++
		if entry.Value.ActiveStreams() == 0 {
			return false
		}
	}

	return count >= c.maxModelsInCache
}

func (c *Cache) eviction(event otter.DeletionEvent[string, *kronk.Kronk]) {
	const unloadTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), unloadTimeout)
	defer cancel()

	c.log(ctx, "kronk cache eviction", "key", event.Key, "cause", event.Cause, "was-evicted", event.WasEvicted(), "active-streams", event.Value.ActiveStreams())

	// If there are active streams and this was an automatic eviction (not a replacement
	// from our own Set call below), re-insert the model to prevent eviction.
	// WasEvicted() returns false for CauseReplacement and CauseInvalidation.
	if event.Value.ActiveStreams() > 0 && event.WasEvicted() {
		c.log(ctx, "kronk cache eviction prevented", "key", event.Key, "active-streams", event.Value.ActiveStreams())
		c.cache.Set(event.Key, event.Value)
		return
	}

	// If this is a replacement event (from our Set above) and there are still active
	// streams, just return without unloading - the model is still in the cache.
	// For invalidation (shutdown), we still need to unload since the cache is being cleared.
	if event.Value.ActiveStreams() > 0 && event.Cause != otter.CauseInvalidation {
		c.log(ctx, "kronk cache eviction skipped (replacement with active streams)", "key", event.Key, "active-streams", event.Value.ActiveStreams())
		return
	}

	c.log(ctx, "kronk cache eviction", "key", event.Key, "status", "unload-started", "active-streams", event.Value.ActiveStreams())

	if err := event.Value.Unload(ctx); err != nil {
		c.log(ctx, "kronk cache eviction", "key", event.Key, "ERROR", err)
	}

	c.log(ctx, "kronk cache eviction", "key", event.Key, "status", "unload-finished")

	c.itemsInCache.Add(-1)
}

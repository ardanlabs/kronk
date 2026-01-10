// Package cache manages a cache of kronk APIs for specific models. Used by
// the model server to manage the number of models that are maintained in
// memory at any given time.
package cache

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
	"github.com/hybridgroup/yzma/pkg/download"
	"github.com/maypok86/otter/v2"
)

// Config represents setting for the kronk manager.
//
// CatalogRepo represents the Github repo for where the catalog is. If left empty
// then api.github.com/repos/ardanlabs/kronk_catalogs/contents/catalogs is used.
//
// TemplateRepo represents the Github repo for where the templates are. If left empty
// then api.github.com/repos/ardanlabs/kronk_catalogs/contents/templates is used.
//
// Device: Specify a specific device. To see the list of devices run this command:
// $HOME/kronk/libraries/llama-bench --list-devices
// Leave empty for the system to pick the device.
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
	Arch                 download.Arch
	OS                   download.OS
	Processor            download.Processor
	Device               string
	ContextWindow        int
	NBatch               int
	NUBatch              int
	NThreads             int
	NThreadsBatch        int
	CacheTypeK           model.GGMLType
	CacheTypeV           model.GGMLType
	FlashAttention       model.FlashAttentionType
	DefragThold          float32
	IgnoreIntegrityCheck bool
	MaxInCache           int
	ModelInstances       int
	CacheTTL             time.Duration
}

func validateConfig(cfg Config) (Config, error) {
	if cfg.Templates == nil {
		templates, err := templates.New()
		if err != nil {
			return Config{}, err
		}

		cfg.Templates = templates
	}

	if cfg.MaxInCache <= 0 {
		cfg.MaxInCache = 3
	}

	if cfg.ModelInstances <= 0 {
		cfg.ModelInstances = 1
	}

	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 5 * time.Minute
	}

	return cfg, nil
}

// Cache manages a set of Kronk APIs for use. It maintains a cache of these
// APIs and will unload over time if not in use.
type Cache struct {
	log                  model.Logger
	templates            *templates.Templates
	arch                 download.Arch
	os                   download.OS
	processor            download.Processor
	device               string
	contextWindow        int
	nBatch               int
	nUBatch              int
	nThreads             int
	nThreadsBatch        int
	cacheTypeK           model.GGMLType
	cacheTypeV           model.GGMLType
	flashAttention       model.FlashAttentionType
	defragThold          float32
	ignoreIntegrityCheck bool
	instances            int
	cache                *otter.Cache[string, *kronk.Kronk]
	itemsInCache         atomic.Int32
	models               *models.Models
}

// NewCache constructs the manager for use.
func NewCache(cfg Config) (*Cache, error) {
	cfg, err := validateConfig(cfg)
	if err != nil {
		return nil, err
	}

	models, err := models.NewWithPaths(cfg.BasePath)
	if err != nil {
		return nil, fmt.Errorf("creating models system: %w", err)
	}

	c := Cache{
		log:                  cfg.Log,
		templates:            cfg.Templates,
		arch:                 cfg.Arch,
		os:                   cfg.OS,
		processor:            cfg.Processor,
		device:               cfg.Device,
		instances:            cfg.ModelInstances,
		contextWindow:        cfg.ContextWindow,
		ignoreIntegrityCheck: cfg.IgnoreIntegrityCheck,
		models:               models,
	}

	opt := otter.Options[string, *kronk.Kronk]{
		MaximumSize:      cfg.MaxInCache,
		ExpiryCalculator: otter.ExpiryWriting[string, *kronk.Kronk](cfg.CacheTTL),
		OnDeletion:       c.eviction,
	}

	cache, err := otter.New(&opt)
	if err != nil {
		return nil, fmt.Errorf("constructing cache: %w", err)
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

// Arch returns the hardware being used.
func (c *Cache) Arch() download.Arch {
	return c.arch
}

// OS returns the operating system being used.
func (c *Cache) OS() download.OS {
	return c.os
}

// Processor returns the processor being used.
func (c *Cache) Processor() download.Processor {
	return c.processor
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
			id := strings.ToLower(mi.ID)

			if id == model.Key {
				ps = append(ps, ModelDetail{
					ID:            mi.ID,
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
	modelID = strings.ToLower(modelID)

	krn, exists := c.cache.GetIfPresent(modelID)
	if exists {
		return krn, nil
	}

	fi, err := c.models.RetrievePath(modelID)
	if err != nil {
		return nil, fmt.Errorf("aquire-model: %w", err)
	}

	cfg := model.Config{
		Log:                  c.log,
		ModelFiles:           fi.ModelFiles,
		ProjFile:             fi.ProjFile,
		Device:               c.device,
		ContextWindow:        c.contextWindow,
		NBatch:               c.nBatch,
		NUBatch:              c.nUBatch,
		NThreads:             c.nThreads,
		NThreadsBatch:        c.nThreadsBatch,
		CacheTypeK:           c.cacheTypeK,
		CacheTypeV:           c.cacheTypeV,
		FlashAttention:       c.flashAttention,
		DefragThold:          c.defragThold,
		IgnoreIntegrityCheck: c.ignoreIntegrityCheck,
	}

	krn, err = kronk.New(c.instances, cfg,
		kronk.WithTemplateRetriever(c.templates),
		kronk.WithContext(ctx),
	)

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	c.cache.Set(modelID, krn)
	c.itemsInCache.Add(1)

	totalEntries := len(krn.SystemInfo())*2 + (5 * 2)
	info := make([]any, 0, totalEntries)
	for k, v := range krn.SystemInfo() {
		info = append(info, k)
		info = append(info, v)
	}

	info = append(info, "status")
	info = append(info, "kronk cache add")
	info = append(info, "model-name")
	info = append(info, modelID)
	info = append(info, "contextWindow")
	info = append(info, krn.ModelConfig().ContextWindow)
	info = append(info, "isGPTModel")
	info = append(info, krn.ModelInfo().IsGPTModel)
	info = append(info, "isEmbedModel")
	info = append(info, krn.ModelInfo().IsEmbedModel)

	c.log(ctx, "acquire-model", info...)

	return krn, nil
}

func (c *Cache) eviction(event otter.DeletionEvent[string, *kronk.Kronk]) {
	const unloadTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), unloadTimeout)
	defer cancel()

	c.log(ctx, "kronk cache eviction", "key", event.Key, "cause", event.Cause, "was-evicted", event.WasEvicted())
	if err := event.Value.Unload(ctx); err != nil {
		c.log(ctx, "kronk cache eviction", "key", event.Key, "ERROR", err)
	}

	c.itemsInCache.Add(-1)
}

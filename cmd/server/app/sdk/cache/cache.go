// Package cache manages a cache of kronk APIs for specific models. Used by
// the model server to manage the number of models that are maintained in
// memory at any given time.
package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
	"github.com/maypok86/otter/v2"
	"gopkg.in/yaml.v3"
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
	ModelConfigFile      string
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

type defaultContext struct {
	Temperature     *float32 `yaml:"temperature"`
	TopK            *int32   `yaml:"top_k"`
	TopP            *float32 `yaml:"top_p"`
	MinP            *float32 `yaml:"min_p"`
	MaxTokens       *int     `yaml:"max_tokens"`
	RepeatPenalty   *float32 `yaml:"repeat_penalty"`
	RepeatLastN     *int32   `yaml:"repeat_last_n"`
	DryMultiplier   *float32 `yaml:"dry_multiplier"`
	DryBase         *float32 `yaml:"dry_base"`
	DryAllowedLen   *int32   `yaml:"dry_allowed_length"`
	DryPenaltyLast  *int32   `yaml:"dry_penalty_last_n"`
	XtcProbability  *float32 `yaml:"xtc_probability"`
	XtcThreshold    *float32 `yaml:"xtc_threshold"`
	XtcMinKeep      *uint32  `yaml:"xtc_min_keep"`
	Thinking        *string  `yaml:"enable_thinking"`
	ReasoningEffort *string  `yaml:"reasoning_effort"`
	ReturnPrompt    *bool    `yaml:"return_prompt"`
	IncludeUsage    *bool    `yaml:"include_usage"`
	Logprobs        *bool    `yaml:"logprobs"`
	TopLogprobs     *int     `yaml:"top_logprobs"`
	Stream          *bool    `yaml:"stream"`
}

type modelConfig struct {
	Device               string                   `yaml:"device"`
	ContextWindow        int                      `yaml:"context-window"`
	NBatch               int                      `yaml:"nbatch"`
	NUBatch              int                      `yaml:"nubatch"`
	NThreads             int                      `yaml:"nthreads"`
	NThreadsBatch        int                      `yaml:"nthreads-batch"`
	CacheTypeK           model.GGMLType           `yaml:"cache-type-k"`
	CacheTypeV           model.GGMLType           `yaml:"cache-type-v"`
	UseDirectIO          bool                     `yaml:"use-direct-io"`
	FlashAttention       model.FlashAttentionType `yaml:"flash-attention"`
	IgnoreIntegrityCheck bool                     `yaml:"ignore-integrity-check"`
	NSeqMax              int                      `yaml:"nseq-max"`
	OffloadKQV           *bool                    `yaml:"offload-kqv"`
	OpOffload            *bool                    `yaml:"op-offload"`
	NGpuLayers           *int32                   `yaml:"ngpu-layers"`
	SplitMode            model.SplitMode          `yaml:"split-mode"`
	SystemPromptCache    bool                     `yaml:"system-prompt-cache"`
	FirstMessageCache    bool                     `yaml:"first-message-cache"`
	CacheMinTokens       int                      `yaml:"cache-min-tokens"`
	DefaultContext       defaultContext           `yaml:"default-context"`
}

func (mc modelConfig) String() string {
	formatBoolPtr := func(p *bool) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprintf("%t", *p)
	}

	formatInt32Ptr := func(p *int32) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprintf("%d", *p)
	}

	return fmt.Sprintf("{Device:%q ContextWindow:%d NBatch:%d NUBatch:%d NThreads:%d NThreadsBatch:%d CacheTypeK:%d CacheTypeV:%d UseDirectIO:%t FlashAttention:%d IgnoreIntegrityCheck:%t NSeqMax:%d OffloadKQV:%s OpOffload:%s NGpuLayers:%s SplitMode:%d SystemPromptCache:%t FirstMessageCache:%t CacheMinTokens:%d}",
		mc.Device, mc.ContextWindow, mc.NBatch, mc.NUBatch, mc.NThreads, mc.NThreadsBatch,
		mc.CacheTypeK, mc.CacheTypeV, mc.UseDirectIO, mc.FlashAttention, mc.IgnoreIntegrityCheck,
		mc.NSeqMax, formatBoolPtr(mc.OffloadKQV), formatBoolPtr(mc.OpOffload),
		formatInt32Ptr(mc.NGpuLayers), mc.SplitMode, mc.SystemPromptCache, mc.FirstMessageCache, mc.CacheMinTokens)
}

// Cache manages a set of Kronk APIs for use. It maintains a cache of these
// APIs and will unload over time if not in use.
type Cache struct {
	log                  model.Logger
	templates            *templates.Templates
	cache                *otter.Cache[string, *kronk.Kronk]
	itemsInCache         atomic.Int32
	maxModelsInCache     int
	models               *models.Models
	catalog              *catalog.Catalog
	ignoreIntegrityCheck bool
	modelConfig          map[string]modelConfig
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

	cat, err := catalog.New(catalog.WithBasePath(cfg.BasePath))
	if err != nil {
		return nil, fmt.Errorf("new: creating catalog system: %w", err)
	}

	var mc map[string]modelConfig
	if cfg.ModelConfigFile != "" {
		mc, err = loadModelConfig(cfg.ModelConfigFile)
		if err != nil {
			return nil, fmt.Errorf("new: loading model config: %w", err)
		}
	}

	c := Cache{
		log:                  cfg.Log,
		templates:            cfg.Templates,
		maxModelsInCache:     cfg.ModelsInCache,
		models:               models,
		catalog:              cat,
		ignoreIntegrityCheck: cfg.IgnoreIntegrityCheck,
		modelConfig:          mc,
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

	if c.allSlotsActive() {
		return nil, ErrServerBusy
	}

	fi, err := c.models.RetrievePath(modelID)
	if err != nil {
		return nil, fmt.Errorf("acquire-model: unable to retrieve path: %w", err)
	}

	c.log(ctx, "model config lookup", "modelID", modelID, "available-keys", fmt.Sprintf("%v", func() []string {
		keys := make([]string, 0, len(c.modelConfig))
		for k := range c.modelConfig {
			keys = append(keys, k)
		}
		return keys
	}()))

	mc, found := c.modelConfig[strings.ToLower(modelID)]
	c.log(ctx, "model config result", "found", found, "mc", mc.String())

	if c.ignoreIntegrityCheck {
		mc.IgnoreIntegrityCheck = true
	}

	cfg := model.Config{
		Log:                  c.log,
		ModelFiles:           fi.ModelFiles,
		ProjFile:             fi.ProjFile,
		Device:               mc.Device,
		ContextWindow:        mc.ContextWindow,
		NBatch:               mc.NBatch,
		NUBatch:              mc.NUBatch,
		NThreads:             mc.NThreads,
		NThreadsBatch:        mc.NThreadsBatch,
		CacheTypeK:           mc.CacheTypeK,
		CacheTypeV:           mc.CacheTypeV,
		FlashAttention:       mc.FlashAttention,
		UseDirectIO:          mc.UseDirectIO,
		IgnoreIntegrityCheck: mc.IgnoreIntegrityCheck,
		NSeqMax:              mc.NSeqMax,
		OffloadKQV:           mc.OffloadKQV,
		OpOffload:            mc.OpOffload,
		NGpuLayers:           mc.NGpuLayers,
		SplitMode:            mc.SplitMode,
		SystemPromptCache:    mc.SystemPromptCache,
		FirstMessageCache:    mc.FirstMessageCache,
		CacheMinTokens:       mc.CacheMinTokens,
	}

	krn, err = kronk.New(cfg,
		kronk.WithTemplateRetriever(c.templates),
		kronk.WithContext(ctx),
	)

	if err != nil {
		return nil, fmt.Errorf("acquire-model: unable to create inference model: %w", err)
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

func (c *Cache) ApplyDefaults(ctx context.Context, modelID string, d model.D) model.D {
	mc, found := c.modelConfig[strings.ToLower(modelID)]
	if !found {
		return d
	}

	catModel, _ := c.catalog.RetrieveModelDetails(modelID)
	catDC := catModel.DefaultContext

	dc := mc.DefaultContext

	var overrides []any

	if _, exists := d["temperature"]; !exists && dc.Temperature != nil {
		if catDC.Temperature != nil && *catDC.Temperature != *dc.Temperature {
			overrides = append(overrides, "temperature", fmt.Sprintf("catalog:%v->config:%v", *catDC.Temperature, *dc.Temperature))
		}
		d["temperature"] = *dc.Temperature
	}
	if _, exists := d["top_k"]; !exists && dc.TopK != nil {
		if catDC.TopK != nil && *catDC.TopK != *dc.TopK {
			overrides = append(overrides, "top_k", fmt.Sprintf("catalog:%v->config:%v", *catDC.TopK, *dc.TopK))
		}
		d["top_k"] = *dc.TopK
	}
	if _, exists := d["top_p"]; !exists && dc.TopP != nil {
		if catDC.TopP != nil && *catDC.TopP != *dc.TopP {
			overrides = append(overrides, "top_p", fmt.Sprintf("catalog:%v->config:%v", *catDC.TopP, *dc.TopP))
		}
		d["top_p"] = *dc.TopP
	}
	if _, exists := d["min_p"]; !exists && dc.MinP != nil {
		if catDC.MinP != nil && *catDC.MinP != *dc.MinP {
			overrides = append(overrides, "min_p", fmt.Sprintf("catalog:%v->config:%v", *catDC.MinP, *dc.MinP))
		}
		d["min_p"] = *dc.MinP
	}
	if _, exists := d["max_tokens"]; !exists && dc.MaxTokens != nil {
		if catDC.MaxTokens != nil && *catDC.MaxTokens != *dc.MaxTokens {
			overrides = append(overrides, "max_tokens", fmt.Sprintf("catalog:%v->config:%v", *catDC.MaxTokens, *dc.MaxTokens))
		}
		d["max_tokens"] = *dc.MaxTokens
	}
	if _, exists := d["repeat_penalty"]; !exists && dc.RepeatPenalty != nil {
		if catDC.RepeatPenalty != nil && *catDC.RepeatPenalty != *dc.RepeatPenalty {
			overrides = append(overrides, "repeat_penalty", fmt.Sprintf("catalog:%v->config:%v", *catDC.RepeatPenalty, *dc.RepeatPenalty))
		}
		d["repeat_penalty"] = *dc.RepeatPenalty
	}
	if _, exists := d["repeat_last_n"]; !exists && dc.RepeatLastN != nil {
		if catDC.RepeatLastN != nil && *catDC.RepeatLastN != *dc.RepeatLastN {
			overrides = append(overrides, "repeat_last_n", fmt.Sprintf("catalog:%v->config:%v", *catDC.RepeatLastN, *dc.RepeatLastN))
		}
		d["repeat_last_n"] = *dc.RepeatLastN
	}
	if _, exists := d["dry_multiplier"]; !exists && dc.DryMultiplier != nil {
		if catDC.DryMultiplier != nil && *catDC.DryMultiplier != *dc.DryMultiplier {
			overrides = append(overrides, "dry_multiplier", fmt.Sprintf("catalog:%v->config:%v", *catDC.DryMultiplier, *dc.DryMultiplier))
		}
		d["dry_multiplier"] = *dc.DryMultiplier
	}
	if _, exists := d["dry_base"]; !exists && dc.DryBase != nil {
		if catDC.DryBase != nil && *catDC.DryBase != *dc.DryBase {
			overrides = append(overrides, "dry_base", fmt.Sprintf("catalog:%v->config:%v", *catDC.DryBase, *dc.DryBase))
		}
		d["dry_base"] = *dc.DryBase
	}
	if _, exists := d["dry_allowed_length"]; !exists && dc.DryAllowedLen != nil {
		if catDC.DryAllowedLen != nil && *catDC.DryAllowedLen != *dc.DryAllowedLen {
			overrides = append(overrides, "dry_allowed_length", fmt.Sprintf("catalog:%v->config:%v", *catDC.DryAllowedLen, *dc.DryAllowedLen))
		}
		d["dry_allowed_length"] = *dc.DryAllowedLen
	}
	if _, exists := d["dry_penalty_last_n"]; !exists && dc.DryPenaltyLast != nil {
		if catDC.DryPenaltyLast != nil && *catDC.DryPenaltyLast != *dc.DryPenaltyLast {
			overrides = append(overrides, "dry_penalty_last_n", fmt.Sprintf("catalog:%v->config:%v", *catDC.DryPenaltyLast, *dc.DryPenaltyLast))
		}
		d["dry_penalty_last_n"] = *dc.DryPenaltyLast
	}
	if _, exists := d["xtc_probability"]; !exists && dc.XtcProbability != nil {
		if catDC.XtcProbability != nil && *catDC.XtcProbability != *dc.XtcProbability {
			overrides = append(overrides, "xtc_probability", fmt.Sprintf("catalog:%v->config:%v", *catDC.XtcProbability, *dc.XtcProbability))
		}
		d["xtc_probability"] = *dc.XtcProbability
	}
	if _, exists := d["xtc_threshold"]; !exists && dc.XtcThreshold != nil {
		if catDC.XtcThreshold != nil && *catDC.XtcThreshold != *dc.XtcThreshold {
			overrides = append(overrides, "xtc_threshold", fmt.Sprintf("catalog:%v->config:%v", *catDC.XtcThreshold, *dc.XtcThreshold))
		}
		d["xtc_threshold"] = *dc.XtcThreshold
	}
	if _, exists := d["xtc_min_keep"]; !exists && dc.XtcMinKeep != nil {
		if catDC.XtcMinKeep != nil && *catDC.XtcMinKeep != *dc.XtcMinKeep {
			overrides = append(overrides, "xtc_min_keep", fmt.Sprintf("catalog:%v->config:%v", *catDC.XtcMinKeep, *dc.XtcMinKeep))
		}
		d["xtc_min_keep"] = *dc.XtcMinKeep
	}
	if _, exists := d["enable_thinking"]; !exists && dc.Thinking != nil {
		if catDC.Thinking != nil && *catDC.Thinking != *dc.Thinking {
			overrides = append(overrides, "enable_thinking", fmt.Sprintf("catalog:%v->config:%v", *catDC.Thinking, *dc.Thinking))
		}
		d["enable_thinking"] = *dc.Thinking
	}
	if _, exists := d["reasoning_effort"]; !exists && dc.ReasoningEffort != nil {
		if catDC.ReasoningEffort != nil && *catDC.ReasoningEffort != *dc.ReasoningEffort {
			overrides = append(overrides, "reasoning_effort", fmt.Sprintf("catalog:%v->config:%v", *catDC.ReasoningEffort, *dc.ReasoningEffort))
		}
		d["reasoning_effort"] = *dc.ReasoningEffort
	}
	if _, exists := d["return_prompt"]; !exists && dc.ReturnPrompt != nil {
		if catDC.ReturnPrompt != nil && *catDC.ReturnPrompt != *dc.ReturnPrompt {
			overrides = append(overrides, "return_prompt", fmt.Sprintf("catalog:%v->config:%v", *catDC.ReturnPrompt, *dc.ReturnPrompt))
		}
		d["return_prompt"] = *dc.ReturnPrompt
	}
	if _, exists := d["include_usage"]; !exists && dc.IncludeUsage != nil {
		if catDC.IncludeUsage != nil && *catDC.IncludeUsage != *dc.IncludeUsage {
			overrides = append(overrides, "include_usage", fmt.Sprintf("catalog:%v->config:%v", *catDC.IncludeUsage, *dc.IncludeUsage))
		}
		d["include_usage"] = *dc.IncludeUsage
	}
	if _, exists := d["logprobs"]; !exists && dc.Logprobs != nil {
		if catDC.Logprobs != nil && *catDC.Logprobs != *dc.Logprobs {
			overrides = append(overrides, "logprobs", fmt.Sprintf("catalog:%v->config:%v", *catDC.Logprobs, *dc.Logprobs))
		}
		d["logprobs"] = *dc.Logprobs
	}
	if _, exists := d["top_logprobs"]; !exists && dc.TopLogprobs != nil {
		if catDC.TopLogprobs != nil && *catDC.TopLogprobs != *dc.TopLogprobs {
			overrides = append(overrides, "top_logprobs", fmt.Sprintf("catalog:%v->config:%v", *catDC.TopLogprobs, *dc.TopLogprobs))
		}
		d["top_logprobs"] = *dc.TopLogprobs
	}
	if _, exists := d["stream"]; !exists && dc.Stream != nil {
		if catDC.Stream != nil && *catDC.Stream != *dc.Stream {
			overrides = append(overrides, "stream", fmt.Sprintf("catalog:%v->config:%v", *catDC.Stream, *dc.Stream))
		}
		d["stream"] = *dc.Stream
	}

	if len(overrides) > 0 {
		logArgs := []any{"model", modelID}
		logArgs = append(logArgs, overrides...)
		c.log(ctx, "model_config.yaml overrides catalog defaults", logArgs...)
	}

	return d
}

// ApplyCatalogDefaults applies default context values from the catalog for the given model.
// These are lower priority than model_config.yaml defaults.
func (c *Cache) ApplyCatalogDefaults(modelID string, d model.D) model.D {
	catModel, err := c.catalog.RetrieveModelDetails(modelID)
	if err != nil {
		return d
	}

	return model.D(catModel.DefaultContext.ApplyDefaults(d))
}

// ResolvedDefaults represents the resolved sampling defaults for a model.
type ResolvedDefaults struct {
	Temperature     *float32 `json:"temperature,omitempty"`
	TopK            *int32   `json:"top_k,omitempty"`
	TopP            *float32 `json:"top_p,omitempty"`
	MinP            *float32 `json:"min_p,omitempty"`
	MaxTokens       *int     `json:"max_tokens,omitempty"`
	RepeatPenalty   *float32 `json:"repeat_penalty,omitempty"`
	RepeatLastN     *int32   `json:"repeat_last_n,omitempty"`
	DryMultiplier   *float32 `json:"dry_multiplier,omitempty"`
	DryBase         *float32 `json:"dry_base,omitempty"`
	DryAllowedLen   *int32   `json:"dry_allowed_length,omitempty"`
	DryPenaltyLast  *int32   `json:"dry_penalty_last_n,omitempty"`
	XtcProbability  *float32 `json:"xtc_probability,omitempty"`
	XtcThreshold    *float32 `json:"xtc_threshold,omitempty"`
	XtcMinKeep      *uint32  `json:"xtc_min_keep,omitempty"`
	Thinking        *string  `json:"enable_thinking,omitempty"`
	ReasoningEffort *string  `json:"reasoning_effort,omitempty"`
	ReturnPrompt    *bool    `json:"return_prompt,omitempty"`
	IncludeUsage    *bool    `json:"include_usage,omitempty"`
	Logprobs        *bool    `json:"logprobs,omitempty"`
	TopLogprobs     *int     `json:"top_logprobs,omitempty"`
	Stream          *bool    `json:"stream,omitempty"`
}

// GetResolvedDefaults returns the resolved sampling defaults for a model.
// It merges catalog defaults with model_config.yaml overrides.
func (c *Cache) GetResolvedDefaults(ctx context.Context, modelID string) ResolvedDefaults {
	var rd ResolvedDefaults

	catModel, err := c.catalog.RetrieveModelDetails(modelID)
	if err == nil {
		dc := catModel.DefaultContext
		rd.Temperature = dc.Temperature
		rd.TopK = dc.TopK
		rd.TopP = dc.TopP
		rd.MinP = dc.MinP
		rd.MaxTokens = dc.MaxTokens
		rd.RepeatPenalty = dc.RepeatPenalty
		rd.RepeatLastN = dc.RepeatLastN
		rd.DryMultiplier = dc.DryMultiplier
		rd.DryBase = dc.DryBase
		rd.DryAllowedLen = dc.DryAllowedLen
		rd.DryPenaltyLast = dc.DryPenaltyLast
		rd.XtcProbability = dc.XtcProbability
		rd.XtcThreshold = dc.XtcThreshold
		rd.XtcMinKeep = dc.XtcMinKeep
		rd.Thinking = dc.Thinking
		rd.ReasoningEffort = dc.ReasoningEffort
		rd.ReturnPrompt = dc.ReturnPrompt
		rd.IncludeUsage = dc.IncludeUsage
		rd.Logprobs = dc.Logprobs
		rd.TopLogprobs = dc.TopLogprobs
		rd.Stream = dc.Stream
	}

	mc, found := c.modelConfig[strings.ToLower(modelID)]
	if found {
		dc := mc.DefaultContext
		if dc.Temperature != nil {
			rd.Temperature = dc.Temperature
		}
		if dc.TopK != nil {
			rd.TopK = dc.TopK
		}
		if dc.TopP != nil {
			rd.TopP = dc.TopP
		}
		if dc.MinP != nil {
			rd.MinP = dc.MinP
		}
		if dc.MaxTokens != nil {
			rd.MaxTokens = dc.MaxTokens
		}
		if dc.RepeatPenalty != nil {
			rd.RepeatPenalty = dc.RepeatPenalty
		}
		if dc.RepeatLastN != nil {
			rd.RepeatLastN = dc.RepeatLastN
		}
		if dc.DryMultiplier != nil {
			rd.DryMultiplier = dc.DryMultiplier
		}
		if dc.DryBase != nil {
			rd.DryBase = dc.DryBase
		}
		if dc.DryAllowedLen != nil {
			rd.DryAllowedLen = dc.DryAllowedLen
		}
		if dc.DryPenaltyLast != nil {
			rd.DryPenaltyLast = dc.DryPenaltyLast
		}
		if dc.XtcProbability != nil {
			rd.XtcProbability = dc.XtcProbability
		}
		if dc.XtcThreshold != nil {
			rd.XtcThreshold = dc.XtcThreshold
		}
		if dc.XtcMinKeep != nil {
			rd.XtcMinKeep = dc.XtcMinKeep
		}
		if dc.Thinking != nil {
			rd.Thinking = dc.Thinking
		}
		if dc.ReasoningEffort != nil {
			rd.ReasoningEffort = dc.ReasoningEffort
		}
		if dc.ReturnPrompt != nil {
			rd.ReturnPrompt = dc.ReturnPrompt
		}
		if dc.IncludeUsage != nil {
			rd.IncludeUsage = dc.IncludeUsage
		}
		if dc.Logprobs != nil {
			rd.Logprobs = dc.Logprobs
		}
		if dc.TopLogprobs != nil {
			rd.TopLogprobs = dc.TopLogprobs
		}
		if dc.Stream != nil {
			rd.Stream = dc.Stream
		}
	}

	return rd
}

func loadModelConfig(modelConfigFile string) (map[string]modelConfig, error) {
	data, err := os.ReadFile(modelConfigFile)
	if err != nil {
		return nil, fmt.Errorf("load-model-config: reading model config file: %w", err)
	}

	var configs map[string]modelConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("load-model-config: unmarshaling model config: %w", err)
	}

	// Normalize keys to lowercase for case-insensitive lookup.
	normalized := make(map[string]modelConfig, len(configs))
	for k, v := range configs {
		normalized[strings.ToLower(k)] = v
	}

	return normalized, nil
}

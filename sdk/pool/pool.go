// Package pool manages a pool of kronk APIs for specific models. Used by
// the model server to manage the number of models that are maintained in
// memory at any given time.
package pool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/maypok86/otter/v2"
	"golang.org/x/sync/singleflight"
)

// ErrServerBusy is returned when all model slots are occupied with active streams.
var ErrServerBusy = errors.New("server busy: all model slots have active requests")

// Config represents setting for the kronk manager.
//
// BudgetPercent: Percentage (1..100) of detected GPU VRAM and system RAM
// that the pool's resource manager is allowed to commit to loaded models.
// Defaults to defaultBudgetPercent (80) when zero. This is the primary
// admission knob.
//
// ModelsInCache: A hard upper bound on the number of distinct entries the
// pool will keep, regardless of budget. Used as a safety net to keep TTL
// eviction predictable; defaults to 32 when zero.
//
// CacheTTL: Defines the time an existing model can live in the pool without
// being used. Defaults to 5 minutes if the value is 0.
//
// Snapshot: Optional resource snapshot used to construct the resource
// manager. When nil the pool calls devices.List() at construction time.
// Tests use this to inject a deterministic device topology.
//
// InsecureLogging: When true, logs potentially sensitive data such as message
// content and detailed model configuration.
type Config struct {
	Log             kronk.Logger
	BasePath        string
	ModelConfigFile string
	ModelsInCache   int
	BudgetPercent   int
	CacheTTL        time.Duration
	Snapshot        *resman.Snapshot
	InsecureLogging bool
}

// Default config values applied when the corresponding field is zero.
const (
	defaultBudgetPercent = 80
	defaultModelsInCache = 32
	defaultCacheTTL      = 5 * time.Minute
)

func validateConfig(cfg Config) (Config, error) {
	if cfg.BudgetPercent <= 0 {
		cfg.BudgetPercent = defaultBudgetPercent
	}

	if cfg.ModelsInCache <= 0 {
		cfg.ModelsInCache = defaultModelsInCache
	}

	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
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

	resman    *resman.Manager
	ticketsMu sync.Mutex
	tickets   map[string]resman.Ticket
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

	p := Pool{
		log:              cfg.Log,
		modelConfig:      mc,
		maxModelsInCache: cfg.ModelsInCache,
		models:           mdls,
		insecureLogging:  cfg.InsecureLogging,
		resman:           rm,
		tickets:          make(map[string]resman.Ticket),
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

	p.logResmanInit(context.Background())

	return &p, nil
}

// ResourceManager returns the pool's underlying resource manager. Useful
// for surfacing budget/usage data via observability endpoints.
func (p *Pool) ResourceManager() *resman.Manager {
	return p.resman
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
				kvCache, vramTotal := p.modelDisplayMemory(krn, cacheID)
				ps = append(ps, ModelDetail{
					ID:            model.Key,
					OwnedBy:       mi.OwnedBy,
					ModelFamily:   mi.ModelFamily,
					Size:          mi.Size,
					VRAMTotal:     vramTotal,
					KVCache:       kvCache,
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

// modelDisplayMemory returns the KV cache and total VRAM values to surface
// in BUI/observability output for a loaded model. It prefers the
// resman-side calculation (models.CalculateVRAM) over whatever the SDK
// stored in ModelInfo.
//
// Defense-in-depth: as of the sdk/kronk/gguf consolidation both sides
// share the array-aware metadata parser, so the SDK's own calculation now
// reports correct slot memory for hybrid architectures (notably
// gemma3/gemma4, whose attention.head_count_kv is a per-layer array that
// llama.cpp's gguf_kv_to_str silently drops). This overlay is kept in
// place because it costs essentially nothing and protects against any
// future ARRAY-key regression in another architecture. Falls back to the
// SDK's stored values when the resman calculation cannot run.
func (p *Pool) modelDisplayMemory(krn *kronk.Kronk, modelID string) (kvCache int64, vramTotal int64) {
	cfg := krn.ModelConfig()
	mi := krn.ModelInfo()

	ctxWin := int64(cfg.ContextWindow())
	if ctxWin <= 0 {
		ctxWin = int64(vram.ContextWindow4K)
	}

	nseq := int64(cfg.NSeqMax())
	if nseq <= 0 {
		nseq = 1
	}

	vramCfg := vram.Config{
		ContextWindow:   ctxWin,
		BytesPerElement: bytesPerElement(cfg.CacheTypeK),
		Slots:           nseq,
	}

	if v, err := p.models.CalculateVRAM(modelID, vramCfg); err == nil {
		return v.SlotMemory, v.TotalVRAM
	}

	return mi.SlotMemory, mi.VRAMTotal
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

		// Reserve the model's predicted memory footprint with the resource
		// manager, evicting idle entries if needed to make room.
		planReq, err := p.planRequest(ctx, modelID, modelID, cfg)
		if err != nil {
			return nil, fmt.Errorf("acquire-model: %w", err)
		}

		ticket, plan, err := p.reserveWithEviction(ctx, modelID, planReq)
		if err != nil {
			return nil, fmt.Errorf("acquire-model: %w", err)
		}

		reservedArgs := append([]any{
			"status", "reserved",
			"key", modelID,
		}, describePlan(plan)...)
		p.log(ctx, "acquire-model", reservedArgs...)
		p.logResmanUsage(ctx, "post-reserve", "key", modelID)

		krn, err := kronk.NewWithContext(ctx,
			model.WithConfig(cfg),
		)

		if err != nil {
			p.resman.Release(ticket)
			p.log(ctx, "acquire-model",
				"status", "load-failed-reservation-released",
				"key", modelID,
				"ERROR", err,
			)
			p.logResmanUsage(ctx, "post-failed-load", "key", modelID)
			return nil, fmt.Errorf("acquire-model: unable to create inference model: %w", err)
		}

		p.storeTicket(modelID, ticket)
		p.cache.Set(modelID, krn)
		p.itemsInCache.Add(1)

		if entry, ok := p.cache.GetEntryQuietly(modelID); ok {
			p.log(ctx, "acquire-model",
				"status", "cache-set",
				"key", modelID,
				"expires-at", entry.ExpiresAt(),
				"ttl", entry.ExpiresAfter().String(),
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

	result, err, _ := p.loadGroup.Do(key, func() (any, error) {
		if krn, exists := p.cache.GetIfPresent(key); exists {
			return krn, nil
		}

		if p.insecureLogging {
			cfg.PtrInsecureLogging = new(true)
		}

		cfg.Log = p.log

		// Reserve the model's predicted memory footprint with the resource
		// manager, evicting idle entries if needed to make room. The modelID
		// for the VRAM calculation is the first segment of the key (per the
		// <modelID>/playground/<session_id> convention).
		modelID, _, _ := strings.Cut(key, "/")
		planReq, err := p.planRequest(ctx, modelID, key, cfg)
		if err != nil {
			return nil, fmt.Errorf("acquire-custom: %w", err)
		}

		ticket, plan, err := p.reserveWithEviction(ctx, key, planReq)
		if err != nil {
			return nil, fmt.Errorf("acquire-custom: %w", err)
		}

		reservedArgs := append([]any{
			"status", "reserved",
			"key", key,
		}, describePlan(plan)...)
		p.log(ctx, "acquire-custom", reservedArgs...)
		p.logResmanUsage(ctx, "post-reserve", "key", key)

		krn, err := kronk.NewWithContext(ctx,
			model.WithConfig(cfg),
		)

		if err != nil {
			p.resman.Release(ticket)
			p.log(ctx, "acquire-custom",
				"status", "load-failed-reservation-released",
				"key", key,
				"ERROR", err,
			)
			p.logResmanUsage(ctx, "post-failed-load", "key", key)
			return nil, fmt.Errorf("acquire-custom: unable to create inference model: %w", err)
		}

		p.storeTicket(key, ticket)
		p.cache.Set(key, krn)
		p.itemsInCache.Add(1)

		if entry, ok := p.cache.GetEntryQuietly(key); ok {
			p.log(ctx, "acquire-custom",
				"status", "cache-set",
				"key", key,
				"expires-at", entry.ExpiresAt(),
				"ttl", entry.ExpiresAfter().String(),
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

// reserveWithEviction reserves the model's memory footprint with the
// resource manager, evicting idle entries to free either the budget or the
// items-in-cache cap when necessary.
//
// On success it returns the ticket and the resolved plan. On failure it
// returns ErrServerBusy if no idle victims remain, or a wrapped error from
// the resource manager / context.
func (p *Pool) reserveWithEviction(ctx context.Context, newKey string, req resman.PlanRequest) (resman.Ticket, resman.LoadPlan, error) {
	const maxAttempts = 64

	p.log(ctx, "reserve",
		"status", "begin",
		"key", newKey,
		"vram", humanBytes(req.VRAMBytes),
		"ram", humanBytes(req.RAMBytes),
		"devices", req.Devices,
		"items-in-cache", p.itemsInCache.Load(),
		"max-models-in-cache", p.maxModelsInCache,
	)

	for attempt := range maxAttempts {

		// Enforce the items-in-cache cap before attempting to reserve. Even
		// when budget allows, the cap bounds how many distinct entries we
		// keep in memory.
		if int(p.itemsInCache.Load()) >= p.maxModelsInCache {
			p.log(ctx, "reserve",
				"status", "cap-hit",
				"key", newKey,
				"attempt", attempt,
				"items-in-cache", p.itemsInCache.Load(),
				"max-models-in-cache", p.maxModelsInCache,
			)
			if err := p.evictOneIdle(ctx, newKey, "cap"); err != nil {
				p.log(ctx, "reserve",
					"status", "cap-evict-failed",
					"key", newKey,
					"ERROR", err,
				)
				return resman.Ticket{}, resman.LoadPlan{}, err
			}
			continue
		}

		ticket, plan, err := p.resman.Reserve(req)
		if err == nil {
			p.log(ctx, "reserve",
				"status", "success",
				"key", newKey,
				"attempt", attempt,
			)
			return ticket, plan, nil
		}

		// Only ErrNoCapacity is recoverable via eviction.
		if !errors.Is(err, resman.ErrNoCapacity) {
			p.log(ctx, "reserve",
				"status", "fatal",
				"key", newKey,
				"attempt", attempt,
				"ERROR", err,
			)
			return resman.Ticket{}, resman.LoadPlan{}, fmt.Errorf("reserve: %w", err)
		}

		p.log(ctx, "reserve",
			"status", "no-capacity",
			"key", newKey,
			"attempt", attempt,
			"ERROR", err,
		)
		p.logResmanUsage(ctx, "no-capacity", "key", newKey)

		if err := p.evictOneIdle(ctx, newKey, "budget"); err != nil {
			p.log(ctx, "reserve",
				"status", "budget-evict-failed",
				"key", newKey,
				"ERROR", err,
			)
			return resman.Ticket{}, resman.LoadPlan{}, err
		}
	}

	p.log(ctx, "reserve",
		"status", "gave-up",
		"key", newKey,
		"max-attempts", maxAttempts,
	)
	return resman.Ticket{}, resman.LoadPlan{}, fmt.Errorf("reserve: gave up after %d eviction attempts", maxAttempts)
}

// evictOneIdle picks the coldest pooled entry without active streams,
// invalidates it, and waits for the eviction callback to release the
// reservation. Returns ErrServerBusy when no idle victim is available.
func (p *Pool) evictOneIdle(ctx context.Context, newKey, reason string) error {
	const pollInterval = 25 * time.Millisecond
	const maxWait = 60 * time.Second

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

	p.log(ctx, "acquire",
		"status", "evict-before-load",
		"reason", reason,
		"victim", victim,
		"items-in-cache", p.itemsInCache.Load(),
		"max-models-in-cache", p.maxModelsInCache,
	)

	p.cache.Invalidate(victim)

	deadline := time.Now().Add(maxWait)
	for {
		if !p.hasTicket(victim) && int(p.itemsInCache.Load()) < p.maxModelsInCache+1 {
			// The eviction callback has run (ticket released) and the
			// counter has been decremented or is at its previous level.
			// We use cap+1 to allow the counter check to succeed even if
			// another acquire raced; the loop in reserveWithEviction will
			// recheck on the next iteration.
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("evict-one-idle: timeout waiting for victim[%s] to unload", victim)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	p.log(ctx, "acquire",
		"status", "evict-before-load-complete",
		"victim", victim,
		"items-in-cache", p.itemsInCache.Load(),
	)

	return nil
}

// storeTicket records a successful reservation so the eviction callback can
// release it when the model is unloaded.
func (p *Pool) storeTicket(key string, t resman.Ticket) {
	p.ticketsMu.Lock()
	defer p.ticketsMu.Unlock()
	p.tickets[key] = t
}

// takeTicket removes and returns a stored ticket. The second return value
// is false if no ticket was found for the key.
func (p *Pool) takeTicket(key string) (resman.Ticket, bool) {
	p.ticketsMu.Lock()
	defer p.ticketsMu.Unlock()
	t, ok := p.tickets[key]
	if ok {
		delete(p.tickets, key)
	}
	return t, ok
}

// hasTicket reports whether a ticket is still tracked for key.
func (p *Pool) hasTicket(key string) bool {
	p.ticketsMu.Lock()
	defer p.ticketsMu.Unlock()
	_, ok := p.tickets[key]
	return ok
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

	if ticket, ok := p.takeTicket(event.Key); ok {
		p.resman.Release(ticket)
		p.log(ctx, "kronk pool eviction",
			"status", "reservation-released",
			"key", event.Key,
		)
		p.logResmanUsage(ctx, "post-release", "key", event.Key)
	}

	p.itemsInCache.Add(-1)
}

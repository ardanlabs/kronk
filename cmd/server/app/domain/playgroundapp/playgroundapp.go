// Package playgroundapp provides endpoints for the model playground.
package playgroundapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
)

type sessionEntry struct {
	cacheKey      string
	custom        bool
	pendingDelete bool
}

type app struct {
	log      *logger.Logger
	cache    *cache.Cache
	catalog  *catalog.Catalog
	mu       sync.Mutex
	sessions map[string]sessionEntry // session_id -> session entry
	done     chan struct{}
}

func newApp(cfg Config) *app {
	a := &app{
		log:      cfg.Log,
		cache:    cfg.Cache,
		catalog:  cfg.Catalog,
		sessions: make(map[string]sessionEntry),
		done:     make(chan struct{}),
	}

	go a.cleanupLoop()

	return a
}

func (a *app) listTemplates(ctx context.Context, r *http.Request) web.Encoder {
	list, err := a.catalog.ListTemplates()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	templates := make([]TemplateInfo, len(list))
	for i, t := range list {
		templates[i] = TemplateInfo{
			Name: t.Name,
			Size: t.Size,
		}
	}

	return TemplateListResponse{Templates: templates}
}

func (a *app) getTemplate(ctx context.Context, r *http.Request) web.Encoder {
	name := web.Param(r, "name")
	if err := validateTemplateName(name); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	content, err := a.catalog.ReadTemplate(name)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return TemplateContentResponse{
		Name:   name,
		Script: content,
	}
}

func (a *app) saveTemplate(ctx context.Context, r *http.Request) web.Encoder {
	var req TemplateSaveRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if err := a.catalog.SaveTemplate(req.Name, req.Script); err != nil {
		return errs.New(errs.Internal, err)
	}

	return TemplateSaveResponse{Status: "saved"}
}

func (a *app) createSession(ctx context.Context, r *http.Request) web.Encoder {
	var req SessionRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if req.ModelID == "" {
		return errs.Errorf(errs.InvalidArgument, "missing model_id")
	}

	baseCfg, err := a.catalog.KronkResolvedModelConfig(req.ModelID)
	if err != nil {
		return errs.New(errs.Internal, fmt.Errorf("resolving model config: %w", err))
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return errs.New(errs.Internal, fmt.Errorf("generating session id: %w", err))
	}

	cfg := req.Config.ApplyTo(baseCfg)

	if cfg.NUBatch > cfg.NBatch {
		return errs.Errorf(errs.InvalidArgument, "nubatch (%d) must not exceed nbatch (%d)", cfg.NUBatch, cfg.NBatch)
	}

	cat := &playgroundCataloger{
		modelID:        req.ModelID,
		templateMode:   req.TemplateMode,
		templateName:   req.TemplateName,
		templateScript: req.TemplateScript,
		catalog:        a.catalog,
	}

	var (
		cacheKey string
		krn      *kronk.Kronk
	)

	switch {
	case req.HasOverrides():
		cacheKey = fmt.Sprintf("%s/playground/%s", req.ModelID, sessionID)
		krn, err = a.cache.AquireCustom(ctx, cacheKey, cfg, cat)
	default:
		cacheKey = req.ModelID
		krn, err = a.cache.AquireModel(ctx, req.ModelID)
	}
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	a.log.Info(ctx, "playground-session",
		"session-id", sessionID,
		"model-id", req.ModelID,
		"cache-key", cacheKey,
		"shared", !req.HasOverrides(),
		"krn-ptr", fmt.Sprintf("%p", krn),
		"context-window", krn.ModelConfig().ContextWindow,
		"nbatch", krn.ModelConfig().NBatch,
		"nubatch", krn.ModelConfig().NUBatch,
		"flash-attention", krn.ModelConfig().FlashAttention.String(),
		"cache-type-k", krn.ModelConfig().CacheTypeK.String(),
		"cache-type-v", krn.ModelConfig().CacheTypeV.String(),
		"nseq-max", krn.ModelConfig().NSeqMax,
	)

	a.mu.Lock()
	a.sessions[sessionID] = sessionEntry{
		cacheKey: cacheKey,
		custom:   req.HasOverrides(),
	}
	a.mu.Unlock()

	effectiveConfig := map[string]any{
		"context_window":      krn.ModelConfig().ContextWindow,
		"nbatch":              krn.ModelConfig().NBatch,
		"nubatch":             krn.ModelConfig().NUBatch,
		"nseq_max":            krn.ModelConfig().NSeqMax,
		"flash_attention":     krn.ModelConfig().FlashAttention.String(),
		"cache_type_k":        krn.ModelConfig().CacheTypeK.String(),
		"cache_type_v":        krn.ModelConfig().CacheTypeV.String(),
		"system_prompt_cache": krn.ModelConfig().SystemPromptCache,
		"incremental_cache":   krn.ModelConfig().IncrementalCache,
		"split_mode":          krn.ModelConfig().SplitMode.String(),
		"model_type":          krn.ModelInfo().Type.String(),
		"is_gpt_model":        krn.ModelInfo().IsGPTModel,
	}

	return SessionResponse{
		SessionID:       sessionID,
		Status:          "loaded",
		EffectiveConfig: effectiveConfig,
	}
}

func (a *app) deleteSession(ctx context.Context, r *http.Request) web.Encoder {
	id := web.Param(r, "id")
	if id == "" {
		return errs.Errorf(errs.InvalidArgument, "missing session id")
	}

	a.mu.Lock()
	entry, exists := a.sessions[id]
	if !exists {
		a.mu.Unlock()
		return errs.Errorf(errs.NotFound, "session not found: %s", id)
	}

	if !entry.custom {
		delete(a.sessions, id)
		a.mu.Unlock()
		return SessionDeleteResponse{Status: "unloaded"}
	}

	a.mu.Unlock()

	if krn, found := a.cache.GetExisting(entry.cacheKey); found && krn.ActiveStreams() > 0 {
		a.mu.Lock()
		entry.pendingDelete = true
		a.sessions[id] = entry
		a.mu.Unlock()
		a.log.Info(ctx, "playground-delete-deferred", "session-id", id, "cache-key", entry.cacheKey, "active-streams", krn.ActiveStreams())
		return SessionDeleteResponse{Status: "unloaded"}
	}

	a.cache.Invalidate(entry.cacheKey)

	a.mu.Lock()
	delete(a.sessions, id)
	a.mu.Unlock()

	return SessionDeleteResponse{Status: "unloaded"}
}

func (a *app) chatCompletions(ctx context.Context, r *http.Request) web.Encoder {
	var raw model.D
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	sessionIDRaw, exists := raw["session_id"]
	if !exists {
		return errs.Errorf(errs.InvalidArgument, "missing session_id field")
	}

	sessionID, ok := sessionIDRaw.(string)
	if !ok {
		return errs.Errorf(errs.InvalidArgument, "session_id must be a string")
	}

	a.mu.Lock()
	entry, exists := a.sessions[sessionID]
	a.mu.Unlock()

	if !exists {
		return errs.Errorf(errs.NotFound, "session not found or expired: %s", sessionID)
	}

	if entry.pendingDelete {
		return errs.Errorf(errs.InvalidArgument, "session is being deleted")
	}

	krn, found := a.cache.GetExisting(entry.cacheKey)
	if !found {
		a.mu.Lock()
		delete(a.sessions, sessionID)
		a.mu.Unlock()
		return errs.Errorf(errs.NotFound, "session expired: %s", sessionID)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	d := model.MapToModelD(raw)

	if _, err := krn.ChatStreamingHTTP(ctx, web.GetWriter(ctx), d); err != nil {
		return errs.New(errs.Internal, err)
	}

	return web.NewNoResponse()
}

func generateSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pg-" + hex.EncodeToString(b), nil
}

func (a *app) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.done:
			return
		case <-ticker.C:
			a.mu.Lock()
			var pending []struct {
				id       string
				cacheKey string
			}
			for id, entry := range a.sessions {
				if entry.pendingDelete && entry.custom {
					pending = append(pending, struct {
						id       string
						cacheKey string
					}{id: id, cacheKey: entry.cacheKey})
				}
			}
			a.mu.Unlock()

			for _, p := range pending {
				krn, found := a.cache.GetExisting(p.cacheKey)
				if !found || krn.ActiveStreams() == 0 {
					a.cache.Invalidate(p.cacheKey)
					a.mu.Lock()
					delete(a.sessions, p.id)
					a.mu.Unlock()
				}
			}

			// Prune sessions whose cache entry has been evicted.
			// Collect candidates under lock, then check cache without holding it.
			type candidate struct {
				id       string
				cacheKey string
			}
			var stale []candidate
			a.mu.Lock()
			for id, entry := range a.sessions {
				if !entry.pendingDelete {
					stale = append(stale, candidate{id: id, cacheKey: entry.cacheKey})
				}
			}
			a.mu.Unlock()

			for _, c := range stale {
				if _, found := a.cache.GetExisting(c.cacheKey); !found {
					a.mu.Lock()
					delete(a.sessions, c.id)
					a.mu.Unlock()
				}
			}
		}
	}
}

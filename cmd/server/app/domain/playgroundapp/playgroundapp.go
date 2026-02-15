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
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

type app struct {
	log      *logger.Logger
	cache    *cache.Cache
	models   *models.Models
	catalog  *catalog.Catalog
	mu       sync.Mutex
	sessions map[string]string // session_id -> cache_key
}

func newApp(cfg Config) *app {
	return &app{
		log:      cfg.Log,
		cache:    cfg.Cache,
		models:   cfg.Models,
		catalog:  cfg.Catalog,
		sessions: make(map[string]string),
	}
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
	if name == "" {
		return errs.Errorf(errs.InvalidArgument, "missing template name")
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

	if req.Name == "" {
		return errs.Errorf(errs.InvalidArgument, "missing template name")
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

	fp, err := a.models.FullPath(req.ModelID)
	if err != nil {
		return errs.Errorf(errs.InvalidArgument, "model not found: %s", req.ModelID)
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return errs.New(errs.Internal, fmt.Errorf("generating session id: %w", err))
	}

	cfg := req.Config.ToModelConfig()
	cfg.ModelFiles = fp.ModelFiles
	cfg.ProjFile = fp.ProjFile

	cat := &playgroundCataloger{
		modelID:        req.ModelID,
		templateMode:   req.TemplateMode,
		templateName:   req.TemplateName,
		templateScript: req.TemplateScript,
		catalog:        a.catalog,
	}

	cacheKey := fmt.Sprintf("%s/playground/%s", req.ModelID, sessionID)

	krn, err := a.cache.AquireCustom(ctx, cacheKey, cfg, cat)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	a.mu.Lock()
	a.sessions[sessionID] = cacheKey
	a.mu.Unlock()

	effectiveConfig := map[string]any{
		"context_window":      krn.ModelConfig().ContextWindow,
		"nbatch":              krn.ModelConfig().NBatch,
		"nubatch":             krn.ModelConfig().NUBatch,
		"nseq_max":            krn.ModelConfig().NSeqMax,
		"flash_attention":     krn.ModelConfig().FlashAttention.String(),
		"system_prompt_cache": krn.ModelConfig().SystemPromptCache,
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
	cacheKey, exists := a.sessions[id]
	if exists {
		delete(a.sessions, id)
	}
	a.mu.Unlock()

	if !exists {
		return errs.Errorf(errs.InvalidArgument, "session not found: %s", id)
	}

	a.cache.Invalidate(cacheKey)

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
	cacheKey, exists := a.sessions[sessionID]
	a.mu.Unlock()

	if !exists {
		return errs.Errorf(errs.InvalidArgument, "session not found or expired: %s", sessionID)
	}

	krn, found := a.cache.GetExisting(cacheKey)
	if !found {
		a.mu.Lock()
		delete(a.sessions, sessionID)
		a.mu.Unlock()
		return errs.Errorf(errs.InvalidArgument, "session expired: %s", sessionID)
	}

	ctx, cancel := context.WithTimeout(ctx, 180*time.Minute)
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

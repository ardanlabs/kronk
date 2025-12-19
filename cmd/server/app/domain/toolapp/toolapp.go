// Package toolapp provides endpoints to handle tool management.
package toolapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/cache"
	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/security"
)

type app struct {
	log      *logger.Logger
	cache    *cache.Cache
	security *security.Security
}

func newApp(log *logger.Logger, cache *cache.Cache, security *security.Security) *app {
	return &app{
		log:      log,
		cache:    cache,
		security: security,
	}
}

func (a *app) pullLibs(ctx context.Context, r *http.Request) web.Encoder {
	w := web.GetWriter(ctx)

	f, ok := w.(http.Flusher)
	if !ok {
		return errs.Errorf(errs.Internal, "streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	// -------------------------------------------------------------------------

	cfg := libs.Config{
		LibPath:      a.cache.LibPath(),
		Arch:         a.cache.Arch(),
		OS:           a.cache.OS(),
		AllowUpgrade: true,
		Processor:    a.cache.Processor(),
	}

	logger := func(ctx context.Context, msg string, args ...any) {
		var sb strings.Builder
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				sb.WriteString(fmt.Sprintf(" %v[%v]", args[i], args[i+1]))
			}
		}

		status := fmt.Sprintf("%s:%s\n", msg, sb.String())
		ver := toAppVersion(status, libs.VersionTag{})

		a.log.Info(ctx, "pull-libs", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
	}

	vi, err := libs.Download(ctx, logger, cfg)
	if err != nil {
		ver := toAppVersion(err.Error(), libs.VersionTag{})

		a.log.Info(ctx, "pull-libs", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()

		return errs.Errorf(errs.Internal, "unable to install llama.cpp: %s", err)
	}

	ver := toAppVersion("downloaded", vi)

	a.log.Info(ctx, "pull-libs", "info", ver[:len(ver)-1])
	fmt.Fprint(w, ver)
	f.Flush()

	return web.NewNoResponse()
}

func (a *app) listModels(ctx context.Context, r *http.Request) web.Encoder {
	modelPath := a.cache.ModelPath()

	models, err := models.RetrieveFiles(modelPath)
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve model list: %s", err)
	}

	return toListModelsInfo(models)
}

func (a *app) pullModels(ctx context.Context, r *http.Request) web.Encoder {
	var req PullRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if _, err := url.ParseRequestURI(req.ModelURL); err != nil {
		return errs.Errorf(errs.InvalidArgument, "invalid model URL: %s", req.ModelURL)
	}

	if req.ProjURL != "" {
		if _, err := url.ParseRequestURI(req.ProjURL); err != nil {
			return errs.Errorf(errs.InvalidArgument, "invalid project URL: %s", req.ProjURL)
		}
	}

	// -------------------------------------------------------------------------

	w := web.GetWriter(ctx)

	f, ok := w.(http.Flusher)
	if !ok {
		return errs.Errorf(errs.Internal, "streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	// -------------------------------------------------------------------------

	modelPath := a.cache.ModelPath()

	logger := func(ctx context.Context, msg string, args ...any) {
		var sb strings.Builder
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				sb.WriteString(fmt.Sprintf(" %v[%v]", args[i], args[i+1]))
			}
		}

		status := fmt.Sprintf("%s:%s\n", msg, sb.String())
		ver := toAppPull(status, models.Path{})

		a.log.Info(ctx, "pull-model", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
	}

	mp, err := models.Download(ctx, logger, req.ModelURL, req.ProjURL, modelPath)
	if err != nil {
		ver := toAppPull(err.Error(), models.Path{})

		a.log.Info(ctx, "pull-model", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()

		return errs.Errorf(errs.Internal, "unable to install model: %s", err)
	}

	ver := toAppPull("downloaded", mp)

	a.log.Info(ctx, "pull-model", "info", ver[:len(ver)-1])
	fmt.Fprint(w, ver)
	f.Flush()

	return web.NewNoResponse()
}

func (a *app) removeModel(ctx context.Context, r *http.Request) web.Encoder {
	modelPath := a.cache.ModelPath()
	modelName := web.Param(r, "model")

	a.log.Info(ctx, "tool-remove", "modelName", modelName)

	mp, err := models.RetrievePath(modelPath, modelName)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if err := models.Remove(mp); err != nil {
		return errs.Errorf(errs.Internal, "failed to remove model: %s", err)
	}

	return nil
}

func (a *app) showModel(ctx context.Context, r *http.Request) web.Encoder {
	libPath := a.cache.LibPath()
	modelPath := a.cache.ModelPath()
	modelName := web.Param(r, "model")

	mi, err := models.RetrieveInfo(libPath, modelPath, modelName)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toModelInfo(mi)
}

func (a *app) modelPS(ctx context.Context, r *http.Request) web.Encoder {
	models, err := a.cache.ModelStatus()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	a.log.Info(ctx, "models", "len", len(models))

	return toModelDetails(models)
}

func (a *app) listCatalog(ctx context.Context, r *http.Request) web.Encoder {
	basePath := defaults.BaseDir("")
	filterCategory := web.Param(r, "filter")

	list, err := catalog.CatalogModelList(basePath, filterCategory)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toCatalogModelsResponse(list)
}

func (a *app) pullCatalog(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	basePath := defaults.BaseDir("")

	model, err := catalog.RetrieveModelDetails(basePath, modelID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	// -------------------------------------------------------------------------

	w := web.GetWriter(ctx)

	f, ok := w.(http.Flusher)
	if !ok {
		return errs.Errorf(errs.Internal, "streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	// -------------------------------------------------------------------------

	modelPath := a.cache.ModelPath()

	logger := func(ctx context.Context, msg string, args ...any) {
		var sb strings.Builder
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				sb.WriteString(fmt.Sprintf(" %v[%v]", args[i], args[i+1]))
			}
		}

		status := fmt.Sprintf("%s:%s\n", msg, sb.String())
		ver := toAppPull(status, models.Path{})

		a.log.Info(ctx, "pull-model", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
	}

	mp, err := models.Download(ctx, logger, model.Files.Model.URL, model.Files.Proj.URL, modelPath)
	if err != nil {
		ver := toAppPull(err.Error(), models.Path{})

		a.log.Info(ctx, "pull-model", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()

		return errs.Errorf(errs.Internal, "unable to install model: %s", err)
	}

	ver := toAppPull("downloaded", mp)

	a.log.Info(ctx, "pull-model", "info", ver[:len(ver)-1])
	fmt.Fprint(w, ver)
	f.Flush()

	return web.NewNoResponse()
}

func (a *app) showCatalogModel(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")
	basePath := defaults.BaseDir("")

	model, err := catalog.RetrieveModelDetails(basePath, modelID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toCatalogModelResponse(model)
}

func (a *app) listKeys(ctx context.Context, r *http.Request) web.Encoder {
	keys, err := a.security.ListKeys()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toKeys(keys)
}

func (a *app) createToken(ctx context.Context, r *http.Request) web.Encoder {
	var req TokenRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	endpoints := make(map[string]bool)
	for _, endpoint := range req.Endpoints {
		endpoints[endpoint] = true
	}

	token, err := a.security.GenerateToken(req.UserName, req.Admin, endpoints, req.Duration)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return TokenResponse{
		Token: token,
	}
}

func (a *app) addKey(ctx context.Context, r *http.Request) web.Encoder {
	if err := a.security.AddPrivateKey(); err != nil {
		return errs.New(errs.Internal, err)
	}

	return nil
}

func (a *app) removeKey(ctx context.Context, r *http.Request) web.Encoder {
	keyID := web.Param(r, "keyid")
	if keyID == "" {
		return errs.Errorf(errs.InvalidArgument, "missing key id")
	}

	if err := a.security.DeletePrivateKey(keyID); err != nil {
		return errs.New(errs.Internal, err)
	}

	return nil
}

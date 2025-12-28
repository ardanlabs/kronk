// Package toolapp provides endpoints to handle tool management.
package toolapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
	"google.golang.org/protobuf/proto"
)

type app struct {
	log        *logger.Logger
	cache      *cache.Cache
	authClient *authclient.Client
	libs       *libs.Libs
	models     *models.Models
	catalog    *catalog.Catalog
	templates  *templates.Templates
}

func newApp(cfg Config) *app {
	return &app{
		log:        cfg.Log,
		cache:      cfg.Cache,
		authClient: cfg.AuthClient,
		libs:       cfg.Libs,
		models:     cfg.Models,
		catalog:    cfg.Catalog,
		templates:  cfg.Templates,
	}
}

func (a *app) listLibs(ctx context.Context, r *http.Request) web.Encoder {
	versionTag, err := a.libs.VersionInformation()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toAppVersionTag("retrieve", versionTag)
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

	vi, err := a.libs.Download(ctx, logger)
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

func (a *app) indexModels(ctx context.Context, r *http.Request) web.Encoder {
	if err := a.models.BuildIndex(); err != nil {
		return errs.Errorf(errs.Internal, "unable to build model index: %s", err)
	}

	return nil
}

func (a *app) listModels(ctx context.Context, r *http.Request) web.Encoder {
	models, err := a.models.RetrieveFiles()
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

	mp, err := a.models.Download(ctx, logger, req.ModelURL, req.ProjURL)
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
	modelName := web.Param(r, "model")

	a.log.Info(ctx, "tool-remove", "modelName", modelName)

	mp, err := a.models.RetrievePath(modelName)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if err := a.models.Remove(mp); err != nil {
		return errs.Errorf(errs.Internal, "failed to remove model: %s", err)
	}

	return nil
}

func (a *app) missingModel(ctx context.Context, r *http.Request) web.Encoder {
	return errs.New(errs.InvalidArgument, fmt.Errorf("model parameter is required"))
}

func (a *app) showModel(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	mi, err := a.models.RetrieveInfo(modelID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	krn, err := a.cache.AquireModel(ctx, mi.ID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toModelInfo(mi, krn.ModelInfo())
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
	filterCategory := web.Param(r, "filter")

	list, err := a.catalog.CatalogModelList(filterCategory)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toCatalogModelsResponse(list)
}

func (a *app) pullCatalog(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	model, err := a.catalog.RetrieveModelDetails(modelID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	if model.GatedModel {
		if os.Getenv("KRONK_HF_TOKEN") == "" {
			return errs.Errorf(errs.FailedPrecondition, "gated model requires KRONK_HF_TOKEN to be set with HF token")
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

	modelURLs := model.Files.ToURLS()

	mp, err := a.models.DownloadShards(ctx, logger, modelURLs, model.Files.Proj.URL)
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

	model, err := a.catalog.RetrieveModelDetails(modelID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toCatalogModelResponse(model)
}

func (a *app) listKeys(ctx context.Context, r *http.Request) web.Encoder {
	bearerToken := r.Header.Get("Authorization")

	resp, err := a.authClient.ListKeys(ctx, bearerToken)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toKeys(resp.Keys)
}

func (a *app) createToken(ctx context.Context, r *http.Request) web.Encoder {
	var req TokenRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	bearerToken := r.Header.Get("Authorization")

	endpoints := make(map[string]*authapp.RateLimit)
	for name, rl := range req.Endpoints {
		window := string(rl.Window)
		endpoints[name] = authapp.RateLimit_builder{
			Limit:  proto.Int32(int32(rl.Limit)),
			Window: proto.String(window),
		}.Build()
	}

	resp, err := a.authClient.CreateToken(ctx, bearerToken, req.Admin, endpoints, req.Duration)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return TokenResponse{
		Token: resp.Token,
	}
}

func (a *app) addKey(ctx context.Context, r *http.Request) web.Encoder {
	bearerToken := r.Header.Get("Authorization")

	if err := a.authClient.AddKey(ctx, bearerToken); err != nil {
		return errs.New(errs.Internal, err)
	}

	return nil
}

func (a *app) removeKey(ctx context.Context, r *http.Request) web.Encoder {
	keyID := web.Param(r, "keyid")
	if keyID == "" {
		return errs.Errorf(errs.InvalidArgument, "missing key id")
	}

	bearerToken := r.Header.Get("Authorization")

	if err := a.authClient.RemoveKey(ctx, bearerToken, keyID); err != nil {
		return errs.New(errs.Internal, err)
	}

	return nil
}

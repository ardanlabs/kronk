// Package toolapp provides endpoints to handle tool management.
package toolapp

import (
	"context"
	"fmt"
	"net/http"
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
)

type app struct {
	log        *logger.Logger
	cache      *cache.Cache
	authClient *authclient.Client
	libs       *libs.Libs
	models     *models.Models
	catalog    *catalog.Catalog
}

func newApp(cfg Config) *app {
	return &app{
		log:        cfg.Log,
		cache:      cfg.Cache,
		authClient: cfg.AuthClient,
		libs:       cfg.Libs,
		models:     cfg.Models,
		catalog:    cfg.Catalog,
	}
}

func (a *app) listLibs(ctx context.Context, r *http.Request) web.Encoder {
	versionTag, err := a.libs.VersionInformation()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toAppVersionTag("retrieve", versionTag, a.libs.AllowUpgrade)
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
		ver := toAppVersion(status, libs.VersionTag{}, a.libs.AllowUpgrade)

		a.log.Info(ctx, "pull-libs", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
	}

	// I know this is a hack and a race condition. I expect this situation
	// to only exist for a few people and in a single tenant mode.
	if !a.libs.AllowUpgrade {
		if r.URL.Query().Get("allow-upgrade") != "" {
			a.log.Info(ctx, "pull-libs", "status", "allowing libs upgrade")
			a.libs.AllowUpgrade = true
			defer func() {
				a.libs.AllowUpgrade = false
			}()
		}
	}

	vi, err := a.libs.Download(ctx, logger)
	if err != nil {
		ver := toAppVersion(err.Error(), libs.VersionTag{}, a.libs.AllowUpgrade)

		a.log.Info(ctx, "pull-libs", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()

		return errs.Errorf(errs.Internal, "unable to install llama.cpp: %s", err)
	}

	ver := toAppVersion("downloaded", vi, a.libs.AllowUpgrade)

	a.log.Info(ctx, "pull-libs", "info", ver[:len(ver)-1])
	fmt.Fprint(w, ver)
	f.Flush()

	return web.NewNoResponse()
}

func (a *app) indexModels(ctx context.Context, r *http.Request) web.Encoder {
	if err := a.models.BuildIndex(a.log.Info); err != nil {
		return errs.Errorf(errs.Internal, "unable to build model index: %s", err)
	}

	return nil
}

func (a *app) listModels(ctx context.Context, r *http.Request) web.Encoder {
	modelFiles, err := a.models.Files()
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve model list: %s", err)
	}

	// Build a map of existing models for quick lookup.
	existing := make(map[string]models.File)
	for _, mf := range modelFiles {
		existing[mf.ID] = mf
	}

	// Add extension models from the model config that aren't already present.
	// Extension models use "/" in their ID (e.g., "model/FMC") and inherit
	// from a base model.
	modelConfig := a.catalog.ModelConfig()
	for modelID := range modelConfig {
		if _, exists := existing[modelID]; exists {
			continue
		}

		// Check if this is an extension model (contains "/").
		before, _, ok := strings.Cut(modelID, "/")
		if !ok {
			continue
		}

		// Extract the base model ID and check if it exists.
		baseModelID := before
		baseModel, exists := existing[baseModelID]
		if !exists {
			continue
		}

		// Create a new File entry for the extension model using the base model's info.
		extModel := models.File{
			ID:          modelID,
			OwnedBy:     baseModel.OwnedBy,
			ModelFamily: baseModel.ModelFamily,
			Size:        baseModel.Size,
			Modified:    baseModel.Modified,
			Validated:   baseModel.Validated,
		}

		modelFiles = append(modelFiles, extModel)
	}

	extendedConfig := r.URL.Query().Get("extended-config") == "true"

	// Build resolved configs so the BUI sees the same sampling values
	// the engine will use (catalog defaults + model_config overrides + SDK defaults).
	var resolvedConfigs map[string]catalog.ModelConfig
	if extendedConfig {
		resolvedConfigs = make(map[string]catalog.ModelConfig, len(modelFiles))
		for _, mf := range modelFiles {
			rmc := a.catalog.ResolvedModelConfig(mf.ID)
			rmc.Sampling = rmc.Sampling.WithDefaults()
			resolvedConfigs[mf.ID] = rmc
		}
	}

	return toListModelsInfo(modelFiles, resolvedConfigs, extendedConfig)
}

func (a *app) pullModels(ctx context.Context, r *http.Request) web.Encoder {
	var req PullRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	a.log.Info(ctx, "pull-models", "model", req.ModelURL, "proj", req.ProjURL)

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

func (a *app) calculateVRAM(ctx context.Context, r *http.Request) web.Encoder {
	var req VRAMRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	slots := max(req.Slots, 1)

	// SPC uses a dedicated cache sequence, adding one extra sequence.
	if req.CacheType == "spc" {
		slots++
	}

	contextWindow := req.ContextWindow

	cfg := models.VRAMConfig{
		ContextWindow:   contextWindow,
		BytesPerElement: req.BytesPerElement,
		Slots:           slots,
	}

	vram, err := models.CalculateVRAMFromHuggingFace(ctx, req.ModelURL, cfg)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return VRAMResponse{
		Input: VRAMInput{
			ModelSizeBytes:  vram.Input.ModelSizeBytes,
			ContextWindow:   vram.Input.ContextWindow,
			BlockCount:      vram.Input.BlockCount,
			HeadCountKV:     vram.Input.HeadCountKV,
			KeyLength:       vram.Input.KeyLength,
			ValueLength:     vram.Input.ValueLength,
			BytesPerElement: vram.Input.BytesPerElement,
			Slots:           vram.Input.Slots,
		},
		KVPerTokenPerLayer: vram.KVPerTokenPerLayer,
		KVPerSlot:          vram.KVPerSlot,
		SlotMemory:         vram.SlotMemory,
		TotalVRAM:          vram.TotalVRAM,
	}
}

func (a *app) removeModel(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	a.log.Info(ctx, "tool-remove", "modelName", modelID)

	mp, err := a.models.FullPath(modelID)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if err := a.models.Remove(mp, a.log.Info); err != nil {
		return errs.Errorf(errs.Internal, "failed to remove model: %s", err)
	}

	return nil
}

func (a *app) missingModel(ctx context.Context, r *http.Request) web.Encoder {
	return errs.New(errs.InvalidArgument, fmt.Errorf("model parameter is required"))
}

func (a *app) showModel(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	fi, err := a.models.FileInformation(modelID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	mi, err := a.models.ModelInformation(modelID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	rmc := a.catalog.ResolvedModelConfig(modelID)

	vram, err := a.catalog.CalculateVRAM(modelID, rmc)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toModelInfo(fi, mi, rmc, vram)
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

	list, err := a.catalog.ModelList(filterCategory)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toCatalogModelsResponse(list)
}

func (a *app) pullCatalog(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	model, err := a.catalog.Details(modelID)
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

	modelURLs := model.Files.ToModelURLS()

	mp, err := a.models.DownloadSplits(ctx, logger, modelURLs, model.Files.Proj.URL)
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

	catDetails, err := a.catalog.Details(modelID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	tmpl, err := a.catalog.RetrieveTemplate(modelID)
	if err == nil && tmpl.FileName != "" {
		catDetails.Template = tmpl.FileName
	}

	metadata := make(map[string]string)

	switch catDetails.Validated {
	case true:
		mi, err := a.models.ModelInformation(modelID)
		if err == nil {
			metadata = mi.Metadata
		}

	default:
		md, _, err := models.FetchGGUFMetadata(ctx, catDetails.Files.Models[0].URL)
		if err == nil {
			metadata = md
		}
	}

	rmc := a.catalog.ResolvedModelConfig(modelID)

	var vram *models.VRAM
	vramTmp, err := a.catalog.CalculateVRAM(modelID, rmc)
	if err == nil {
		vram = &vramTmp
	}

	return toCatalogModelResponse(catDetails, &rmc, metadata, vram)
}

func (a *app) lookupHuggingFace(ctx context.Context, r *http.Request) web.Encoder {
	var req HFLookupRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	result, err := catalog.LookupHuggingFace(ctx, req.Input)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toHFLookupResponse(result)
}

func (a *app) saveCatalogModel(ctx context.Context, r *http.Request) web.Encoder {
	var req SaveCatalogRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if err := a.catalog.SaveModel(req.toModelDetails(), req.CatalogFile); err != nil {
		return errs.New(errs.Internal, err)
	}

	return SaveCatalogResponse{Status: "saved", ID: req.ID}
}

func (a *app) deleteCatalogModel(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	if err := a.catalog.DeleteModel(modelID); err != nil {
		return errs.New(errs.Internal, err)
	}

	return SaveCatalogResponse{Status: "deleted", ID: modelID}
}

func (a *app) publishCatalogModel(ctx context.Context, r *http.Request) web.Encoder {
	var req PublishCatalogRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if err := a.catalog.PublishModel(req.CatalogFile); err != nil {
		return errs.New(errs.Internal, err)
	}

	return PublishCatalogResponse{Status: "published"}
}

func (a *app) catalogRepoPath(ctx context.Context, r *http.Request) web.Encoder {
	return RepoPathResponse{RepoPath: a.catalog.RepoPath()}
}

func (a *app) listCatalogFiles(ctx context.Context, r *http.Request) web.Encoder {
	files, err := a.catalog.ListCatalogFiles()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toCatalogFilesResponse(files)
}

func (a *app) listGrammars(ctx context.Context, r *http.Request) web.Encoder {
	files, err := a.catalog.GrammarFiles()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toGrammarFilesResponse(files)
}

func (a *app) showGrammar(ctx context.Context, r *http.Request) web.Encoder {
	name := web.Param(r, "name")

	content, err := a.catalog.GrammarContent(name)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return GrammarContentResponse{Content: content}
}

func (a *app) listTemplates(ctx context.Context, r *http.Request) web.Encoder {
	files, err := a.catalog.TemplateFiles()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toTemplateFilesResponse(files)
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
			Limit:  new(int32(rl.Limit)),
			Window: &window,
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

package toolapp

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/hf"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

var (
	reDownloadMeta     = regexp.MustCompile(`download-model: model-url\[([^\]]*)\] proj-url\[([^\]]*)\] model-id\[([^\]]*)\] file\[(\d+)/(\d+)\]`)
	reDownloadProgress = regexp.MustCompile(`download-model: Downloading ([^ ]+)\.\.\. (\d+) MB of (\d+) MB \(([\d.]+) MB/s\)`)
)

func (a *app) indexModels(ctx context.Context, r *http.Request) web.Encoder {
	if err := a.models.BuildIndex(a.log.Info, true); err != nil {
		return errs.Errorf(errs.Internal, "unable to build model index: %s", err)
	}

	if err := a.models.ReconcileCatalog(ctx, a.log.Info); err != nil {
		return errs.Errorf(errs.Internal, "unable to reconcile catalog: %s", err)
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
	modelConfig := a.pool.ModelConfig()
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
			ID:                   modelID,
			OwnedBy:              baseModel.OwnedBy,
			ModelFamily:          baseModel.ModelFamily,
			TokenizerFingerprint: baseModel.TokenizerFingerprint,
			Size:                 baseModel.Size,
			Modified:             baseModel.Modified,
			Validated:            baseModel.Validated,
			HasProjection:        baseModel.HasProjection,
		}

		modelFiles = append(modelFiles, extModel)
	}

	extendedConfig := r.URL.Query().Get("extended-config") == "true"

	// Build resolved configs so the BUI sees the same sampling values
	// the engine will use (analysis defaults + model_config overrides + SDK defaults).
	var resolvedConfigs map[string]models.ModelConfig
	if extendedConfig {
		resolvedConfigs = make(map[string]models.ModelConfig, len(modelFiles))
		for _, mf := range modelFiles {
			a.log.Info(ctx, "resolved-model-config", "id", mf.ID)
			rmc := a.resolvedModelConfig(mf.ID)
			rmc.Sampling = rmc.Sampling.WithDefaults()
			resolvedConfigs[mf.ID] = rmc
		}
	}

	return toListModelsInfo(modelFiles, resolvedConfigs, extendedConfig)
}

// resolvedModelConfig assembles the analysis-derived defaults overlaid
// with the user-supplied model_config.yaml entry for the given model.
func (a *app) resolvedModelConfig(modelID string) models.ModelConfig {
	cfg := a.models.AnalysisDefaults(modelID)

	if override, ok := a.pool.ModelConfig()[modelID]; ok {
		models.MergeModelConfig(&cfg, override)
	}

	return cfg
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

	// Extend the per-connection write deadline so large model downloads
	// are not killed by the server-wide WriteTimeout.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(6 * time.Hour)); err != nil {
		a.log.Info(ctx, "pull-models", "set-write-deadline", "ERROR", err)
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
				fmt.Fprintf(&sb, " %v[%v]", args[i], args[i+1])
			}
		}

		cleanMsg := strings.TrimPrefix(msg, "\r\x1b[K")

		clean := cleanMsg
		if sb.Len() > 0 {
			clean = fmt.Sprintf("%s:%s", cleanMsg, sb.String())
		}

		var ver string

		switch {
		case reDownloadMeta.MatchString(clean):
			m := reDownloadMeta.FindStringSubmatch(clean)
			fileIdx, _ := strconv.Atoi(m[4])
			fileTotal, _ := strconv.Atoi(m[5])
			ver = toAppPullResponse(PullResponse{
				Status: clean,
				Meta: &PullMeta{
					ModelURL:  m[1],
					ProjURL:   m[2],
					ModelID:   m[3],
					FileIndex: fileIdx,
					FileTotal: fileTotal,
				},
			})

		case reDownloadProgress.MatchString(clean):
			m := reDownloadProgress.FindStringSubmatch(clean)
			cur, _ := strconv.ParseInt(m[2], 10, 64)
			total, _ := strconv.ParseInt(m[3], 10, 64)
			mbps, _ := strconv.ParseFloat(m[4], 64)
			ver = toAppPullResponse(PullResponse{
				Status: clean,
				Progress: &PullProgress{
					Src:          m[1],
					CurrentBytes: cur * 1000 * 1000,
					TotalBytes:   total * 1000 * 1000,
					MBPerSec:     mbps,
					Complete:     total > 0 && cur >= total,
				},
			})

		default:
			ver = toAppPullResponse(PullResponse{Status: clean})
		}

		a.log.Info(ctx, "pull-model", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
	}

	// Download handles both direct URLs and catalog ids (bare or
	// canonical). Catalog ids are resolved through ~/.kronk/catalog.yaml
	// and the configured HuggingFace provider list. When DownloadServer
	// is set, the resolved HuggingFace URLs are rewritten to point at a
	// peer Kronk server on the local network.
	var mp models.Path
	var err error
	switch {
	case req.DownloadServer != "":
		mp, err = a.downloadFromPeer(ctx, logger, req)
	case req.ProjURL != "":
		mp, err = a.models.DownloadURLs(ctx, logger, []string{req.ModelURL}, req.ProjURL)
	default:
		mp, err = a.models.Download(ctx, logger, req.ModelURL)
	}
	if err != nil {
		ver := toAppPull(err.Error(), models.Path{})

		a.log.Info(ctx, "pull-model", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()

		return web.NewNoResponse()
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

	if req.ModelURL == "" && req.ModelID == "" {
		return errs.Errorf(errs.InvalidArgument, "either model_url or model_id is required")
	}

	slots := max(req.Slots, 1)

	cfg := vram.Config{
		ContextWindow:     req.ContextWindow,
		BytesPerElement:   req.BytesPerElement,
		Slots:             slots,
		GPULayers:         req.GPULayers,
		ExpertLayersOnGPU: req.ExpertLayersOnGPU,
		KVCacheOnCPU:      req.KVCacheOnCPU,
	}

	v, err := a.computeVRAM(ctx, req, cfg)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	// Auto-fit: re-run with the search loop using the supplied hardware
	// constraints, then keep the resulting offload values for the
	// response so the caller can apply them as new control values.
	if req.AutoFit {
		ngl, ext, fitted := vram.AutoFit(v.Input, vram.FitConstraints{
			DeviceCount:    req.DeviceCount,
			GPUFreeBytes:   req.GPUFreeBytes,
			SystemRAMBytes: req.SystemRAMBytes,
			TensorSplit:    req.TensorSplit,
			KVCacheOnCPU:   req.KVCacheOnCPU,
		})
		v = fitted
		v.Input.GPULayers = ngl
		v.Input.ExpertLayersOnGPU = ext
	}

	// Per-device split when the caller asked for one.
	if req.DeviceCount > 0 {
		v.PerDevice = vram.CalculatePerDevice(
			v.ModelWeightsGPU, v.KVVRAMBytes, v.ComputeBufferEst,
			req.DeviceCount, req.TensorSplit, nil, 0,
		)
	}

	// Only fetch repo file list on the initial (non-auto-fit / non-incremental)
	// call when the caller supplies a ModelURL — keeps recompute calls fast.
	var repoFiles []HFRepoFile
	if req.ModelURL != "" && !req.AutoFit && req.GPULayers == 0 && req.ExpertLayersOnGPU == 0 && !req.KVCacheOnCPU && req.DeviceCount == 0 {
		repoFiles = fetchVRAMRepoFiles(ctx, req.ModelURL)
	}

	return toVRAMResponse(v, repoFiles)
}

// computeVRAM dispatches to the local-model or HuggingFace path based on
// which identifier the caller provided.
func (a *app) computeVRAM(ctx context.Context, req VRAMRequest, cfg vram.Config) (vram.Result, error) {
	if req.ModelID != "" {
		return a.models.CalculateVRAM(req.ModelID, cfg)
	}
	return vram.FromHuggingFace(ctx, req.ModelURL, cfg)
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

	rmc := a.resolvedModelConfig(modelID)
	rmc.Sampling = rmc.Sampling.WithDefaults()

	var vramResp *VRAMResponse
	if v, err := a.models.CalculateVRAM(modelID, vramConfigFromRMC(rmc)); err == nil {
		vr := toVRAMResponse(v, nil)
		vramResp = &vr
	} else {
		a.log.Info(ctx, "show-model: calculate-vram", "ERROR", err)
	}

	return toModelInfo(fi, mi, rmc, vramResp)
}

func (a *app) modelPS(ctx context.Context, r *http.Request) web.Encoder {
	models, err := a.pool.ModelStatus()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	a.log.Info(ctx, "models", "len", len(models))

	return toModelDetails(models)
}

func (a *app) poolBudget(ctx context.Context, r *http.Request) web.Encoder {
	rm := a.pool.ResourceManager()
	if rm == nil {
		return errs.Errorf(errs.Internal, "resource manager not available")
	}

	usage := rm.Usage()

	a.log.Info(ctx, "pool-budget",
		"budget-percent", usage.BudgetPercent,
		"headroom", pool.HumanBytes(usage.HeadroomBytes),
		"devices", len(usage.Devices),
		"reservations", len(usage.Reservations),
		"ram-used", pool.HumanBytes(usage.RAMUsed),
		"ram-budget", pool.HumanBytes(usage.RAMBudget),
	)

	return toPoolBudget(usage)
}

func (a *app) unloadModel(ctx context.Context, r *http.Request) web.Encoder {
	var req UnloadRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	a.log.Info(ctx, "tool-unload", "modelID", req.ID)

	krn, exists := a.pool.GetExisting(req.ID)
	if !exists {
		return errs.Errorf(errs.NotFound, "model %q is not loaded", req.ID)
	}

	if n := krn.ActiveStreams(); n > 0 {
		return errs.Errorf(errs.FailedPrecondition, "model has %d active stream(s); cannot unload", n)
	}

	// Wait for the eviction callback to release the resource manager
	// reservation before returning. Otherwise the BUI's follow-up
	// /pool/budget refresh races the async unload and the user sees
	// stale "Used" / "Free in Budget" numbers until they manually hit
	// the Refresh button.
	if err := a.pool.InvalidateSync(ctx, req.ID); err != nil {
		return errs.Errorf(errs.Internal, "unload: %s", err)
	}

	return UnloadResponse{Status: "unloaded", ID: req.ID}
}

// =============================================================================

// fetchVRAMRepoFiles extracts the owner/repo from a model URL and fetches
// the list of GGUF files available in that HuggingFace repository. This is
// best-effort: if parsing or fetching fails, an empty slice is returned.
func fetchVRAMRepoFiles(ctx context.Context, modelURL string) []HFRepoFile {
	owner, repo, _, err := hf.ParseInput(modelURL)
	if err != nil || owner == "" || repo == "" {
		return nil
	}

	allFiles, err := hf.RepoFiles(ctx, owner, repo, "main", "", true)
	if err != nil {
		return nil
	}

	var ggufFiles []HFRepoFile
	for _, f := range allFiles {
		if strings.HasSuffix(strings.ToLower(f.Filename), ".gguf") {
			ggufFiles = append(ggufFiles, HFRepoFile{
				Filename: f.Filename,
				Size:     f.Size,
				SizeStr:  f.SizeStr,
			})
		}
	}

	return ggufFiles
}

// downloadFromPeer pulls a model from a peer Kronk server on the local
// network. The HuggingFace URLs produced by the resolver are rewritten
// to point at the peer's /download/ endpoint before the file transfer
// begins. SHA pointer files are fetched the same way (the peer's
// /download/{path...} handler serves both /resolve/main/ and
// /raw/main/).
func (a *app) downloadFromPeer(ctx context.Context, log kronk.Logger, req PullRequest) (models.Path, error) {
	modelURLs, projURL, err := a.resolvePeerURLs(ctx, req.ModelURL, req.ProjURL)
	if err != nil {
		return models.Path{}, fmt.Errorf("download-from-peer: resolve %q: %w", req.ModelURL, err)
	}

	for i, u := range modelURLs {
		modelURLs[i] = toDownloadServerURL(req.DownloadServer, u)
	}
	if projURL != "" {
		projURL = toDownloadServerURL(req.DownloadServer, projURL)
	}

	return a.models.DownloadURLs(ctx, log, modelURLs, projURL)
}

// resolvePeerURLs returns the HuggingFace download URLs for the given
// model source. A direct URL passes through unchanged. Anything else
// (bare or canonical catalog id, owner/repo/file.gguf short form) is
// resolved through the resolver so multi-file (split) models and
// companion mmproj files come back with all of their URLs.
func (a *app) resolvePeerURLs(ctx context.Context, modelSource, projSource string) ([]string, string, error) {
	if strings.HasPrefix(modelSource, "https://") || strings.HasPrefix(modelSource, "http://") {
		return []string{modelSource}, projSource, nil
	}

	rfile, err := defaults.CatalogFile("", a.models.BasePath())
	if err != nil {
		return nil, "", fmt.Errorf("resolver-file: %w", err)
	}

	res, err := models.NewResolver(a.models, rfile).Resolve(ctx, modelSource)
	if err != nil {
		return nil, "", fmt.Errorf("resolve: %w", err)
	}

	if len(res.DownloadURLs) == 0 {
		return nil, "", fmt.Errorf("resolver returned no download URLs for %q", modelSource)
	}

	proj := res.DownloadProj
	if projSource != "" {
		proj = projSource
	}

	return res.DownloadURLs, proj, nil
}

// toDownloadServerURL rewrites a HuggingFace download URL (or short-form
// owner/repo/file.gguf path) to point at a peer Kronk server's
// /download endpoint.
//
//	https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf
//	→ http://192.168.0.246:11435/download/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf
func toDownloadServerURL(server, rawURL string) string {
	const hfPrefix = "https://huggingface.co/"

	normalized := hf.NormalizeDownloadURL(rawURL)
	path := strings.TrimPrefix(normalized, hfPrefix)

	return fmt.Sprintf("http://%s/download/%s", server, path)
}

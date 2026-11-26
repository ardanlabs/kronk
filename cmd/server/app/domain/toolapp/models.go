package toolapp

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/hf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

var (
	reDownloadMeta = regexp.MustCompile(`download-model: model-url\[([^\]]*)\] proj-url\[([^\]]*)\] draft-url\[([^\]]*)\] draft-method\[([^\]]*)\] model-id\[([^\]]*)\] file\[(\d+)/(\d+)\]`)

	// Bucky model downloads still report progress through logger text.
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

// listModelsOpenAI returns the OpenAI-compatible model list served at
// GET /v1/models. Apps like OpenWebUI call this endpoint to discover the
// available models. The native, Kronk-specific listing (with sizes,
// sampling config, etc.) lives at GET /v1/kronk/models.
func (a *app) listModelsOpenAI(ctx context.Context, r *http.Request) web.Encoder {
	modelFiles, err := a.collectModelFiles()
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve model list: %s", err)
	}

	return toOpenAIModels(modelFiles)
}

// retrieveModelOpenAI returns the OpenAI-compatible model object served at
// GET /v1/models/{model}.
func (a *app) retrieveModelOpenAI(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	modelFiles, err := a.collectModelFiles()
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve model list: %s", err)
	}

	for _, mf := range modelFiles {
		if mf.ID == modelID {
			return toOpenAIModel(mf)
		}
	}

	return errs.FromSDK(fmt.Errorf("%w: %q", models.ErrModelNotFound, modelID))
}

func (a *app) listModels(ctx context.Context, r *http.Request) web.Encoder {
	modelFiles, err := a.collectModelFiles()
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve model list: %s", err)
	}

	extendedConfig := r.URL.Query().Get("extended-config") == "true"

	// Build resolved configs so the BUI sees the same sampling values the engine
	// will use (analysis defaults + model_config overrides + GGUF + SDK defaults).
	var resolvedConfigs map[string]models.ModelConfig
	if extendedConfig {
		resolvedConfigs = make(map[string]models.ModelConfig, len(modelFiles))
		for _, mf := range modelFiles {
			a.log.Info(ctx, "resolved-model-config", "id", mf.ID)
			rmc, err := a.pool.Kronk.ResolvedModelConfig(mf.ID)
			if err != nil {
				a.log.Info(ctx, "resolved-model-config", "id", mf.ID, "ERROR", err)
				continue
			}
			if mi, err := a.models.ModelInformation(mf.ID); err == nil {
				rmc.Sampling = rmc.Sampling.WithMetadataDefaults(mi.Metadata)
			} else {
				rmc.Sampling = rmc.Sampling.WithDefaults()
			}
			resolvedConfigs[mf.ID] = rmc
		}
	}

	return toListModelsInfo(modelFiles, resolvedConfigs, extendedConfig)
}

func (a *app) listModelsIntegrity(ctx context.Context, r *http.Request) web.Encoder {
	integrity, err := a.models.Integrity()
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve model integrity: %s", err)
	}

	return toModelIntegrity(integrity)
}

func (a *app) retrieveModelIntegrity(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	integrity, err := a.models.IntegrityFor(modelID)
	if err != nil {
		return errs.FromSDK(err)
	}

	return toModelIntegrityDetail(integrity)
}

// collectModelFiles returns all on-disk model files plus any profiles declared
// in the model config that inherit from a base model.
func (a *app) collectModelFiles() ([]models.File, error) {
	modelFiles, err := a.models.Files()
	if err != nil {
		return nil, err
	}

	existing := make(map[string]models.File, len(modelFiles))
	for i, mf := range modelFiles {
		mf.ID = mf.OwnedBy + "/" + mf.ID
		modelFiles[i] = mf
		existing[mf.ID] = mf
	}

	modelConfig := a.pool.Kronk.ModelConfig()
	for modelID := range modelConfig {
		if _, exists := existing[modelID]; exists {
			continue
		}

		parsed, err := models.ParseModelID(modelID)
		if err != nil || parsed.Profile == "" {
			continue
		}

		baseModel, exists := existing[parsed.Base()]
		if !exists {
			continue
		}

		baseModel.ID = modelID
		modelFiles = append(modelFiles, baseModel)
	}

	return modelFiles, nil
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

	emit := func(pr PullResponse) {
		ver := toAppPullResponse(pr)
		a.log.Info(ctx, "pull-model", "info", strings.TrimSpace(ver))
		fmt.Fprint(w, ver)
		f.Flush()
	}

	logger := func(ctx context.Context, msg string, args ...any) {
		var sb strings.Builder
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				fmt.Fprintf(&sb, " %v[%v]", args[i], args[i+1])
			}
		}

		clean := msg
		if sb.Len() > 0 {
			clean = fmt.Sprintf("%s:%s", msg, sb.String())
		}

		pr := PullResponse{Status: clean}

		if reDownloadMeta.MatchString(clean) {
			m := reDownloadMeta.FindStringSubmatch(clean)
			fileIdx, _ := strconv.Atoi(m[6])
			fileTotal, _ := strconv.Atoi(m[7])
			pr.Meta = &PullMeta{
				ModelURL:  m[1],
				ProjURL:   m[2],
				ModelID:   m[5],
				FileIndex: fileIdx,
				FileTotal: fileTotal,
			}
			if m[3] != "" {
				method, err := models.ParseDraftMethod(m[4])
				if err != nil {
					a.log.Info(ctx, "pull-model", "draft-method", m[4], "ERROR", err)
				} else {
					pr.Meta.Draft = models.CatalogDraft{Method: method, URL: m[3]}
				}
			}
		}

		emit(pr)
	}

	progress := func(progress models.DownloadProgress) {
		emit(toPullProgress(progress))
	}

	// Download handles both direct URLs and canonical catalog ids. Catalog
	// ids are resolved through ~/.kronk/catalog/catalog.yaml. When DownloadServer
	// is set, the resolved HuggingFace URLs are rewritten to point at a
	// peer Kronk server on the local network.
	//
	// When ProjURL or Draft is set the request must reach DownloadDraftURLsWithProgress
	// with fully qualified HuggingFace URLs. If the caller passed an id
	// (bare or canonical) instead, run ResolveSource first so the BUI can
	// keep sending a single shape regardless of whether a projection or
	// draft companion override is in play.
	var mp models.Path
	var err error
	switch {
	case req.DownloadServer != "":
		mp, err = a.downloadFromPeer(ctx, logger, req, progress)
	case req.ProjURL != "" || req.Draft.URL != "":
		modelURLs, projURL, draft, rerr := a.resolvePullURLs(ctx, req.ModelURL, req.ProjURL, req.Draft)
		if rerr != nil {
			err = rerr
			break
		}
		mp, err = a.models.DownloadDraftURLsWithProgress(ctx, logger, modelURLs, projURL, draft, progress)
	default:
		mp, err = a.models.DownloadWithProgress(ctx, logger, req.ModelURL, progress)
	}
	if err != nil {
		ver := toAppPull(err.Error(), models.Path{})

		a.log.Info(ctx, "pull-model", "info", strings.TrimSpace(ver))
		fmt.Fprint(w, ver)
		f.Flush()

		return web.NewNoResponse()
	}

	ver := toAppPull("downloaded", mp)

	a.log.Info(ctx, "pull-model", "info", strings.TrimSpace(ver))
	fmt.Fprint(w, ver)
	f.Flush()

	return web.NewNoResponse()
}

func toPullProgress(progress models.DownloadProgress) PullResponse {
	return PullResponse{
		Status: fmt.Sprintf("download-model: Downloading %s... %d MB of %d MB (%.2f MB/s)",
			progress.Src,
			progress.CurrentBytes/(1000*1000),
			progress.TotalBytes/(1000*1000),
			progress.MBPerSec,
		),
		Progress: &PullProgress{
			Src:          progress.Src,
			CurrentBytes: progress.CurrentBytes,
			TotalBytes:   progress.TotalBytes,
			MBPerSec:     progress.MBPerSec,
			Complete:     progress.Complete,
		},
	}
}

func (a *app) calculateVRAM(ctx context.Context, r *http.Request) web.Encoder {
	var req VRAMRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if req.ModelURL == "" && len(req.ModelURLs) == 0 && req.ModelID == "" {
		return errs.Errorf(errs.InvalidArgument, "model_url, model_urls, or model_id is required")
	}

	if req.MoECacheSize < 0 {
		return errs.Errorf(errs.InvalidArgument, "moe_cache_size must be >= 0")
	}

	if share := req.MoECacheCPUShare; math.IsNaN(float64(share)) || math.IsInf(float64(share), 0) || share < 0 || share > 1 {
		return errs.Errorf(errs.InvalidArgument, "moe_cache_cpu_share must be finite and between 0 and 1")
	}

	slots := max(req.Slots, 1)
	var configuredSWAFull *bool
	var nUBatch int64
	kronkConfig := model.Config{}
	if req.ModelID != "" {
		modelConfig, err := a.pool.Kronk.ResolvedModelConfig(req.ModelID)
		if err != nil {
			return errs.FromSDK(fmt.Errorf("resolve model config: %w", err))
		}
		configuredSWAFull = modelConfig.PtrSWAFull
		kronkConfig = modelConfig.ToKronkConfig()
		nUBatch = int64(kronkConfig.BatchBaseSize())
	}
	swaFull := resolveSWAFull(req.SWAFull, configuredSWAFull)
	if req.KVCacheMode != nil {
		kronkConfig.PtrKVCacheMode = req.KVCacheMode
	}

	cfg := vram.Config{
		WeightLoading:     kronkConfig.VRAMWeightLoading(devices.List(devices.WithIncludeMemory(false))),
		ComputeBackend:    devices.Backend(),
		ContextWindow:     req.ContextWindow,
		BytesPerElement:   req.BytesPerElement,
		Slots:             slots,
		NUBatch:           nUBatch,
		GPULayers:         req.GPULayers,
		ExpertLayersOnGPU: req.ExpertLayersOnGPU,
		MoECacheSize:      req.MoECacheSize,
		KVCacheOnCPU:      req.KVCacheOnCPU,
		KVUnified:         kronkConfig.KVCacheMode() == model.KVCacheShared,
		SWAFull:           swaFull,
	}

	v, err := a.computeVRAM(ctx, req, cfg)
	if err != nil {
		return errs.FromSDK(err)
	}

	// Auto-fit: re-run with the search loop using the supplied hardware
	// constraints, then keep the resulting offload values for the
	// response so the caller can apply them as new control values.
	var autoFitSucceeded *bool
	if req.AutoFit {
		ngl, ext, fitted, fit := vram.AutoFit(v.Input, vram.FitConstraints{
			DeviceCount:       req.DeviceCount,
			GPUFreeBytes:      req.GPUFreeBytes,
			CombinedFreeBytes: req.GPUCapacity,
			SystemRAMBytes:    req.SystemRAMBytes,
			TensorSplit:       req.TensorSplit,
			KVCacheOnCPU:      req.KVCacheOnCPU,
			UnifiedMemory:     req.UnifiedMemory,
		})
		v = fitted
		v.Input.GPULayers = ngl
		v.Input.ExpertLayersOnGPU = ext
		autoFitSucceeded = &fit
	}

	// Per-device split when the caller asked for one.
	if req.DeviceCount > 0 {
		v.PerDevice = v.CalculatePerDevice(req.DeviceCount, req.TensorSplit, nil, 0)
	}

	// Only fetch repo file list on the initial (non-auto-fit / non-incremental)
	// call when the caller supplies a ModelURL — keeps recompute calls fast.
	var repoFiles []HFRepoFile
	if req.ModelURL != "" && !req.AutoFit && req.GPULayers == 0 && req.ExpertLayersOnGPU == 0 && !req.KVCacheOnCPU && req.DeviceCount == 0 {
		repoFiles = fetchVRAMRepoFiles(ctx, req.ModelURL)
	}

	resp := toVRAMResponse(v, repoFiles)
	// CPU share is execution tuning for config export, not a memory discount.
	resp.Input.MoECacheCPUShare = req.MoECacheCPUShare
	resp.AutoFitSucceeded = autoFitSucceeded
	if req.GPUCapacity > 0 || len(req.GPUFreeBytes) > 0 || req.SystemRAMBytes > 0 {
		assessment := toFitAssessment(vram.AssessFit(v, vram.FitConstraints{
			DeviceCount:       req.DeviceCount,
			GPUFreeBytes:      req.GPUFreeBytes,
			CombinedFreeBytes: req.GPUCapacity,
			SystemRAMBytes:    req.SystemRAMBytes,
			TensorSplit:       req.TensorSplit,
			UnifiedMemory:     req.UnifiedMemory,
		}))
		resp.FitAssessment = &assessment
	}

	return resp
}

func (a *app) autoTuneModel(ctx context.Context, r *http.Request) web.Encoder {
	var req AutoTuneRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	req.ModelID = strings.TrimSpace(req.ModelID)
	req.CatalogID = strings.TrimSpace(req.CatalogID)
	if (req.ModelID == "") == (req.CatalogID == "") {
		return errs.Errorf(errs.InvalidArgument, "exactly one of model_id or catalog_id is required")
	}

	if req.ModelID != "" {
		constraints := a.pool.Kronk.ModelConfig()[req.ModelID]
		analysis, err := a.models.ModelAnalysisWithConfig(req.ModelID, constraints)
		if err != nil {
			return errs.Errorf(errs.Internal, "auto-tune model %q: %s", req.ModelID, err)
		}

		return AutoTuneResponse{Analysis: analysis}
	}

	entry, ok, err := a.models.CatalogEntry(req.CatalogID)
	if err != nil {
		return errs.Errorf(errs.Internal, "lookup catalog entry: %s", err)
	}
	if !ok {
		return errs.Errorf(errs.NotFound, "catalog entry %q not found", req.CatalogID)
	}

	data, err := a.models.GGUFHead(ctx, entry)
	if err != nil {
		return errs.FromSDK(fmt.Errorf("read catalog GGUF header: %w", err))
	}

	metadata, err := gguf.ParseMetadata(data)
	if err != nil {
		return errs.Errorf(errs.Internal, "parse catalog GGUF header: %s", err)
	}

	var totalSize uint64
	for _, size := range entry.FileSizes {
		if size > 0 {
			totalSize += uint64(size)
		}
	}

	info := models.ModelInfoFromMetadata(req.CatalogID, metadata, totalSize, entry.MMProj != "", entry.Draft.Method == models.DraftMethodMTP && entry.Draft.File != "")
	analysis, err := models.Analyze(info, devices.List())
	if err != nil {
		return errs.Errorf(errs.Internal, "auto-tune catalog model %q: %s", req.CatalogID, err)
	}

	return AutoTuneResponse{Analysis: analysis}
}

// computeVRAM dispatches to the local-model or HuggingFace path based on
// which identifier the caller provided.
func (a *app) computeVRAM(ctx context.Context, req VRAMRequest, cfg vram.Config) (vram.Result, error) {
	if req.ModelID != "" {
		return a.models.CalculateVRAM(req.ModelID, cfg)
	}
	if len(req.ModelURLs) > 0 {
		return vram.FromHuggingFaceFiles(ctx, req.ModelURLs, cfg)
	}
	return vram.FromHuggingFace(ctx, req.ModelURL, cfg)
}

func (a *app) removeModel(ctx context.Context, r *http.Request) web.Encoder {
	modelID := web.Param(r, "model")

	a.log.Info(ctx, "tool-remove", "modelName", modelID)

	mp, err := a.models.FullPath(modelID)
	if err != nil {
		return errs.FromSDK(err)
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
		return errs.FromSDK(err)
	}

	mi, err := a.models.ModelInformation(modelID)
	if err != nil {
		return errs.FromSDK(err)
	}

	rmc, err := a.pool.Kronk.ResolvedModelConfig(modelID)
	if err != nil {
		return errs.FromSDK(err)
	}
	rmc.Sampling = rmc.Sampling.WithMetadataDefaults(mi.Metadata)

	var vramResp *VRAMResponse
	if v, err := a.models.CalculateVRAM(modelID, vramConfigFromRMC(rmc)); err == nil {
		vr := toVRAMResponse(v, nil)
		vr.Input.MoECacheCPUShare = rmc.ToKronkConfig().MoECacheCPUShare()
		vramResp = &vr
	} else {
		a.log.Info(ctx, "show-model: calculate-vram", "ERROR", err)
	}

	resp := toModelInfo(fi, mi, rmc, vramResp)
	summary := toModelSummary(mi, &rmc)
	resp.Summary = &summary
	return resp
}

func (a *app) modelPS(ctx context.Context, r *http.Request) web.Encoder {
	kronkModels, err := a.pool.Kronk.ModelStatus()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	resp := toModelDetails(kronkModels)
	buckyCount := 0
	malinaCount := 0

	if a.pool.Bucky != nil {
		buckyModels, err := a.pool.Bucky.ModelStatus()
		if err != nil {
			return errs.New(errs.Internal, err)
		}
		resp = append(resp, fromBuckyDetails(buckyModels)...)
		buckyCount = len(buckyModels)
	}

	if a.pool.Malina != nil {
		malinaModels, err := a.pool.Malina.ModelStatus()
		if err != nil {
			return errs.New(errs.Internal, err)
		}
		resp = append(resp, fromMalinaDetails(malinaModels)...)
		malinaCount = len(malinaModels)
	}

	a.log.Info(ctx, "models", "len", len(resp), "kronk", len(kronkModels), "bucky", buckyCount, "malina", malinaCount)

	return resp
}

func (a *app) imcSessions(ctx context.Context, r *http.Request) web.Encoder {
	sessions := a.pool.Kronk.IMCSessions()

	a.log.Info(ctx, "imc-sessions", "len", len(sessions))

	return toIMCSessions(sessions)
}

func (a *app) imcSystemCaches(ctx context.Context, r *http.Request) web.Encoder {
	caches := a.pool.Kronk.IMCSystemCaches()

	a.log.Info(ctx, "imc-system-caches", "len", len(caches))

	return toIMCSystemCaches(caches)
}

func (a *app) batchEngineSlots(ctx context.Context, r *http.Request) web.Encoder {
	snapshots := a.pool.Kronk.BatchEngineSnapshots()

	a.log.Info(ctx, "batch-engine-slots", "models", len(snapshots))

	return toBatchEngineSnapshots(snapshots)
}

func (a *app) poolBudget(ctx context.Context, r *http.Request) web.Encoder {
	rm := a.pool.Resman
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

	// Look in the kronk pool first, then bucky. The two pools never
	// share a cache key in practice (whisper short names like
	// "ggml-tiny.bin" don't collide with llama model ids), but checking
	// kronk first matches the historical behavior of this endpoint.
	if krn, exists := a.pool.Kronk.GetExisting(req.ID); exists {
		if n := krn.ActiveStreams(); n > 0 {
			return errs.Errorf(errs.FailedPrecondition, "model has %d active stream(s); cannot unload", n)
		}

		// Wait for the eviction callback to release the resource
		// manager reservation before returning. Otherwise the BUI's
		// follow-up /pool/budget refresh races the async unload and
		// the user sees stale "Used" / "Free in Budget" numbers
		// until they manually hit the Refresh button.
		if err := a.pool.Kronk.InvalidateSync(ctx, req.ID); err != nil {
			return errs.FromSDK(fmt.Errorf("unload: %w", err))
		}

		return UnloadResponse{Status: "unloaded", ID: req.ID}
	}

	if a.pool.Bucky != nil {
		if b, exists := a.pool.Bucky.GetExisting(req.ID); exists {
			if n := b.ActiveStreams(); n > 0 {
				return errs.Errorf(errs.FailedPrecondition, "model has %d active stream(s); cannot unload", n)
			}

			if err := a.pool.Bucky.InvalidateSync(ctx, req.ID); err != nil {
				return errs.FromSDK(fmt.Errorf("unload: %w", err))
			}

			return UnloadResponse{Status: "unloaded", ID: req.ID}
		}
	}

	if a.pool.Malina != nil {
		if m, exists := a.pool.Malina.GetExisting(req.ID); exists {
			if n := m.ActiveGenerations(); n > 0 {
				return errs.Errorf(errs.FailedPrecondition, "model has %d active generation(s); cannot unload", n)
			}

			if err := a.pool.Malina.InvalidateSync(ctx, req.ID); err != nil {
				return errs.FromSDK(fmt.Errorf("unload: %w", err))
			}

			return UnloadResponse{Status: "unloaded", ID: req.ID}
		}
	}

	return errs.Errorf(errs.NotFound, "model %q is not loaded", req.ID)
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
func (a *app) downloadFromPeer(ctx context.Context, log kronk.Logger, req PullRequest, progress models.DownloadProgressFunc) (models.Path, error) {
	modelURLs, projURL, draft, err := a.resolvePullURLs(ctx, req.ModelURL, req.ProjURL, req.Draft)
	if err != nil {
		return models.Path{}, fmt.Errorf("download-from-peer: resolve %q: %w", req.ModelURL, err)
	}

	for i, u := range modelURLs {
		modelURLs[i] = toDownloadServerURL(req.DownloadServer, u)
	}
	if projURL != "" {
		projURL = toDownloadServerURL(req.DownloadServer, projURL)
	}
	if draft.URL != "" {
		draft.URL = toDownloadServerURL(req.DownloadServer, draft.URL)
	}

	return a.models.DownloadDraftURLsWithProgress(ctx, log, modelURLs, projURL, draft, progress)
}

// resolvePullURLs returns the HuggingFace download URLs for the given
// model source. A direct URL passes through unchanged. Anything else
// (bare or canonical catalog id, owner/repo/file.gguf short form) is
// resolved through the resolver so multi-file (split) models and
// companion mmproj files come back with all of their URLs.
func (a *app) resolvePullURLs(ctx context.Context, modelSource, projSource string, override models.CatalogDraft) ([]string, string, models.CatalogDraft, error) {
	if strings.HasPrefix(modelSource, "https://") || strings.HasPrefix(modelSource, "http://") {
		return []string{modelSource}, projSource, override, nil
	}

	res, err := a.models.ResolveSource(ctx, modelSource)
	if err != nil {
		return nil, "", models.CatalogDraft{}, fmt.Errorf("resolve: %w", err)
	}

	if len(res.DownloadURLs) == 0 {
		return nil, "", models.CatalogDraft{}, fmt.Errorf("resolver returned no download URLs for %q", modelSource)
	}

	proj := res.DownloadProj
	if projSource != "" {
		proj = projSource
	}

	draft := res.Draft
	draft.URL = res.DownloadDraft
	if override.URL != "" {
		draft = override
	}

	return res.DownloadURLs, proj, draft, nil
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

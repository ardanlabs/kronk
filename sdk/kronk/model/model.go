// Package model provides the low-level api for working with models.
package model

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/jinja"
	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// modelLoadMu serializes model loading to prevent concurrent mutation of
// process-level environment variables (e.g., GGML_OP_OFFLOAD_MIN_BATCH).
var modelLoadMu sync.Mutex

// compiledTemplate holds a pre-compiled Jinja template for reuse across requests.
type compiledTemplate struct {
	tmpl *jinja.Template
	err  error
}

// imcSession holds the state for a single IMC (Incremental Message Cache) session.
// Currently sessions are slot-indexed (one per physical slot). Future phases will
// externalize KV state to RAM and decouple sessions from slots.
//
// Transitional: slotID, seqID, and pending are retained until IMC scheduling
// no longer uses slot routing (Phase 4/5).
type imcSession struct {
	slotID            int           // Slot index this session is bound to (transitional: removed in Phase 4)
	seqID             llama.SeqId   // KV cache sequence ID (transitional: removed in Phase 4)
	cachedMsgsHash    string        // Hash of all cached messages
	cachedTokens      []llama.Token // Full token sequence in KV cache (immutable; replaced, never mutated)
	totalTokensCached int           // Total KV positions cached (includes text + media tokens)
	cachedMsgCount    int           // Number of messages cached
	kvState           []byte        // Externalized KV cache state (RAM buffer); restored into any slot via StateSeqSetData
	kvStateBytes      int           // Size of kvState in bytes (for byte-budgeted LRU eviction)
	lastUsed          time.Time     // Last access time (for eviction)
	pending           bool          // True when a build/extend is in-flight (transitional: removed in Phase 4)
	hasMedia          bool          // True if the cached content includes media tokens (image/audio)
	useMRoPE          bool          // True if the cached media used M-RoPE 4D positional encoding
	mediaKVCounts     []int         // KV positions consumed per media chunk (image/audio); used for text-only extend math
	sysPromptHash     string        // Hash of the system prompt message (messages[0] when role="system")
	sysPromptTokens   int           // Token count of the system prompt in the KV cache
}

// draftModel holds resources for the draft model used in speculative decoding.
// The draft model is a smaller, faster model that generates candidate tokens
// for the target model to verify in a single forward pass.
type draftModel struct {
	model        llama.Model
	vocab        llama.Vocab
	lctx         llama.Context
	mem          llama.Memory
	sampler      llama.Sampler
	batch        llama.Batch
	prefillBatch llama.Batch // Reusable batch for prefill decoding (sized to nBatch)
	nDraft       int
	promptBuf    []llama.Token // Reusable buffer for assembling draft prompt tokens
	draftBuf     []llama.Token // Reusable buffer for generateDraftTokens output

	// Pre-allocated buffers for speculative sampling to avoid per-round
	// allocations of vocab-sized slices (~600KB each for 152k vocab).
	draftProbs  [][]float32 // nDraft reusable buffers for draft probability distributions
	targetProbs []float32   // Reusable buffer for target probability distribution
	adjusted    []float32   // Reusable buffer for sampleAdjusted computation
	sortIndices []int       // Reusable buffer for applySamplerFilters top-K indices
	filterBuf   filterState // Reusable buffers for applySamplerFilters heap/rawProbs

	// registeredSampler tracks the sampler currently registered on the draft
	// context via SetSampler for backend (GPU-side) sampling. This avoids
	// redundant set_sampler calls that trigger scheduler re-reservation.
	registeredSampler llama.Sampler
	registeredSeqID   llama.SeqId
}

// Model represents a model and provides a low-level API for working with it.
type Model struct {
	cfg               Config
	log               applog.Logger
	model             llama.Model
	vocab             llama.Vocab
	ctxParams         llama.ContextParams
	lctx              llama.Context
	mem               llama.Memory
	batch             *batchEngine
	template          Template
	compiledTmpl      *compiledTemplate
	templateOnce      sync.Once
	projFile          string
	modelInfo         ModelInfo
	activeStreams     atomic.Int32
	unloaded          atomic.Bool
	decodeMu          sync.Mutex
	cacheMu           sync.RWMutex
	cacheCond         *sync.Cond    // Signaled when an IMC slot becomes available (transitional: removed in Phase 4)
	imcSessions       []*imcSession // IMC sessions, one per slot (transitional: becomes session pool in Phase 4)
	addBOSToken       bool          // Whether to add BOS token (from model metadata)
	mediaMarkerTokens int           // Token count for the media marker string; computed once via mediaMarkerOnce
	mediaMarkerOnce   sync.Once     // Guards one-time computation of mediaMarkerTokens
	pool              *contextPool  // Context pool for parallel embed/rerank
	draft             *draftModel   // Draft model for speculative decoding
}

func NewModel(ctx context.Context, cfg Config) (*Model, error) {
	l := cfg.Log
	if cfg.Log == nil {
		l = func(ctx context.Context, msg string, args ...any) {}
	}

	if len(cfg.ModelFiles) == 0 {
		return nil, fmt.Errorf("model required")
	}

	if err := validateConfig(ctx, cfg, l); err != nil {
		return nil, fmt.Errorf("validate-config: unable to validate config: %w", err)
	}

	mParams := llama.ModelDefaultParams()

	deviceNames := cfg.Devices

	var devicesBuf []llama.GGMLBackendDevice
	if len(deviceNames) > 0 {
		resolved, err := resolveBackendDevices(deviceNames)
		if err != nil {
			return nil, fmt.Errorf("resolve-devices: %w", err)
		}
		if err := mParams.SetDevices(resolved); err != nil {
			return nil, fmt.Errorf("set-devices: %w", err)
		}
		devicesBuf = resolved
	}

	// llama.cpp has a -1 default for loading all layers into the GPU
	// However, we want to make it convenient to write the configuration.
	// So, we default to invert these two values after loading them.
	switch {
	case cfg.PtrNGpuLayers == nil:
		mParams.NGpuLayers = -1
	case *cfg.PtrNGpuLayers == 0:
		mParams.NGpuLayers = -1
	case *cfg.PtrNGpuLayers == -1:
		mParams.NGpuLayers = 0
	default:
		mParams.NGpuLayers = int32(*cfg.PtrNGpuLayers)
	}

	// Set split mode for multi-GPU and tensor parallelism (expert-parallel for MoE).
	// Default to SplitModeRow (tensor parallelism) when not explicitly configured,
	// as it provides the best performance for MoE models and works well for dense models.
	switch cfg.PtrSplitMode {
	case nil:
		mParams.SplitMode = SplitModeRow.ToYZMAType()
	default:
		mParams.SplitMode = (*cfg.PtrSplitMode).ToYZMAType()
	}

	if cfg.PtrMainGPU != nil {
		mParams.MainGpu = int32(*cfg.PtrMainGPU)
	}

	// TensorSplit: proportional distribution of layers across multiple GPUs.
	var tensorSplitBuf []float32
	if len(cfg.TensorSplit) > 0 {
		tensorSplitBuf = make([]float32, len(cfg.TensorSplit))
		copy(tensorSplitBuf, cfg.TensorSplit)
		mParams.TensorSplit = &tensorSplitBuf[0]
	}

	// Compile MoEConfig into TensorBuftOverrides if applicable.
	// Explicit TensorBuftOverrides take highest precedence.
	if cfg.MoE != nil && len(cfg.TensorBuftOverrides) == 0 {
		switch cfg.MoE.Mode {
		case MoEModeExpertsCPU:
			cfg.TensorBuftOverrides = []string{"moe-experts"}
		case MoEModeKeepTopN:
			if cfg.MoE.PtrKeepExpertsOnGPUForTopNLayers != nil {
				topN := *cfg.MoE.PtrKeepExpertsOnGPUForTopNLayers
				// To keep top N on GPU, we offload all layers EXCEPT the top N.
				// We need block_count from model metadata, which isn't available yet.
				// Use the "moe-experts" shortcut for now; per-layer targeting requires
				// model metadata which is available after loading.
				// For initial implementation: offload all experts, then in Phase E
				// we can add per-layer granularity.
				if topN == 0 {
					cfg.TensorBuftOverrides = []string{"moe-experts"}
				}
				// topN > 0: we can't generate per-block overrides without knowing
				// block_count from the model. Leave overrides empty and let
				// llama.cpp handle it. Log the intention.
				if topN > 0 {
					l(ctx, "MOE-CONFIG", "mode", "keep_top_n", "top_n", topN, "note", "per-layer expert placement requires model metadata; using auto-fit")
				}
			}
		case MoEModeExpertsGPU, MoEModeAuto, MoEModeCustom, "":
			// No overrides needed
		}
	}

	// TensorBuftOverrides: force specific tensors to run on CPU.
	var tensorBuftBuf []llama.TensorBuftOverride
	if len(cfg.TensorBuftOverrides) > 0 {
		overrides, err := parseTensorBuftOverrides(cfg.TensorBuftOverrides)
		if err != nil {
			return nil, fmt.Errorf("tensor-buft-overrides: %w", err)
		}
		if err := mParams.SetTensorBufOverrides(overrides); err != nil {
			return nil, fmt.Errorf("set-tensor-buft-overrides: %w", err)
		}
		tensorBuftBuf = overrides
	}

	// UseMMap: controls mmap for model loading.
	// When nil, use llama.cpp default (mmap enabled). UseDirectIO takes precedence.
	if cfg.PtrUseMMap != nil {
		if *cfg.PtrUseMMap {
			mParams.UseMmap = 1
		} else {
			mParams.UseMmap = 0
		}
	}

	// NUMA strategy: must be called once before model loading.
	if cfg.NUMA != "" {
		var numaStrategy llama.NumaStrategy
		switch cfg.NUMA {
		case NUMADistribute:
			numaStrategy = llama.NumaStrategyDistribute
		case NUMAIsolate:
			numaStrategy = llama.NumaStrategyIsolate
		case NUMANumactl:
			numaStrategy = llama.NumaStrategyNumactl
		case NUMAMirror:
			numaStrategy = llama.NumaStrategyMirror
		}
		llama.NumaInit(numaStrategy)
		l(ctx, "NUMA", "strategy", cfg.NUMA)
	}

	// -------------------------------------------------------------------------

	// Set/unset GGML_OP_OFFLOAD_MIN_BATCH before model load.
	// This env var is read by the llama.cpp C library at load time.
	// Use a mutex to prevent concurrent model loads from racing on the env var.
	// Save and restore the previous value so subsequent loads (e.g., draft model)
	// are not unintentionally affected.
	modelLoadMu.Lock()

	prevOffloadMinBatch, hadOffloadMinBatch := os.LookupEnv("GGML_OP_OFFLOAD_MIN_BATCH")
	if cfg.OpOffloadMinBatch() > 0 {
		os.Setenv("GGML_OP_OFFLOAD_MIN_BATCH", strconv.Itoa(*cfg.PtrOpOffloadMinBatch))
		l(ctx, "OP-OFFLOAD-MIN-BATCH", "value", *cfg.PtrOpOffloadMinBatch)
	} else {
		os.Unsetenv("GGML_OP_OFFLOAD_MIN_BATCH")
	}

	loadStart := time.Now()

	mdl, err := loadModelFromFiles(ctx, l, cfg.ModelFiles, mParams)
	runtime.KeepAlive(devicesBuf)
	runtime.KeepAlive(tensorSplitBuf)
	runtime.KeepAlive(tensorBuftBuf)

	if hadOffloadMinBatch {
		os.Setenv("GGML_OP_OFFLOAD_MIN_BATCH", prevOffloadMinBatch)
	} else {
		os.Unsetenv("GGML_OP_OFFLOAD_MIN_BATCH")
	}
	modelLoadMu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("load-model-from-files: unable to load model: %w", err)
	}

	loadDuration := time.Since(loadStart)

	cfg = adjustConfig(cfg, mdl)
	modelInfo := toModelInfo(cfg, mdl)

	metrics.AddModelFileLoadTime(modelInfo.ID, loadDuration)

	// -------------------------------------------------------------------------

	modelInfo.VRAMTotal, modelInfo.SlotMemory = calculateVRAM(cfg, modelInfo)

	metrics.SetVRAM(modelInfo.ID, modelInfo.VRAMTotal, modelInfo.SlotMemory)

	template, err := retrieveTemplate(cfg, mdl)
	if err != nil {
		return nil, fmt.Errorf("retrieve-template: failed to retrieve model template: %w", err)
	}

	modelInfo.Template = template

	// Check if model metadata specifies to add BOS token.
	// Default to true for backward compatibility with models that don't specify.
	addBOSToken := true
	if v, ok := modelInfo.Metadata["tokenizer.ggml.add_bos_token"]; ok && v == "false" {
		addBOSToken = false
	}

	// -------------------------------------------------------------------------

	ctxParams := modelCtxParams(cfg, modelInfo)

	l(ctx, "MODEL-INFO", "values", modelInfo.String(), "addBOSToken", addBOSToken)

	l(ctx, "MODEL-CONFIG", "values", cfg.String())

	// Log effective MoE configuration for debugging.
	if cfg.MoE != nil && cfg.MoE.Mode != "" && cfg.MoE.Mode != MoEModeAuto {
		topN := 0
		if cfg.MoE.PtrKeepExpertsOnGPUForTopNLayers != nil {
			topN = *cfg.MoE.PtrKeepExpertsOnGPUForTopNLayers
		}

		overrides := cfg.TensorBuftOverrides
		if overrides == nil {
			overrides = []string{}
		}

		l(ctx, "MOE-CONFIG",
			"mode", string(cfg.MoE.Mode),
			"experts_on_gpu_layers", topN,
			"overrides_applied", fmt.Sprintf("%v", overrides),
		)
	}

	faName := "unknown"
	switch ctxParams.FlashAttentionType {
	case llama.FlashAttentionTypeAuto:
		faName = "auto"
	case llama.FlashAttentionTypeDisabled:
		faName = "disabled"
	case llama.FlashAttentionTypeEnabled:
		faName = "enabled"
	}

	typeKName := GGMLTypeFromYZMA(ctxParams.TypeK).String()
	typeVName := GGMLTypeFromYZMA(ctxParams.TypeV).String()

	l(ctx, "LLAMA-CONTEXT-PARAMS", "values", fmt.Sprintf("\nEmbeddings[%d]\nFlashAttentionType[%s]\nNBatch[%d]\nNCtx[%d]\nNSeqMax[%d]\nNThreads[%d]\nNThreadsBatch[%d]\nNUBatch[%d]\nOffloadKQV[%d]\nOpOffload[%d]\nPoolingType[%d]\nRopeFreqBase[%g]\nRopeFreqScale[%g]\nRopeScalingType[%d]\nSwaFull[%d]\nTypeK[%s]\nTypeV[%s]\nYarnAttnFactor[%g]\nYarnBetaFast[%g]\nYarnBetaSlow[%g]\nYarnExtFactor[%g]\nYarnOrigCtx[%d]\n",
		ctxParams.Embeddings, faName, ctxParams.NBatch, ctxParams.NCtx,
		ctxParams.NSeqMax, ctxParams.NThreads, ctxParams.NThreadsBatch, ctxParams.NUbatch,
		ctxParams.Offload_kqv, ctxParams.OpOffload, ctxParams.PoolingType,
		ctxParams.RopeFreqBase, ctxParams.RopeFreqScale, ctxParams.RopeScalingType,
		ctxParams.SwaFull, typeKName, typeVName, ctxParams.YarnAttnFactor, ctxParams.YarnBetaFast,
		ctxParams.YarnBetaSlow, ctxParams.YarnExtFactor, ctxParams.YarnOrigCtx))

	// -------------------------------------------------------------------------

	m := Model{
		cfg:         cfg,
		log:         l,
		model:       mdl,
		vocab:       llama.ModelGetVocab(mdl),
		ctxParams:   ctxParams,
		template:    template,
		projFile:    cfg.ProjFile,
		modelInfo:   modelInfo,
		addBOSToken: addBOSToken,
	}

	// Initialize either context pool (for embed/rerank) or batch engine (for generation).
	// Embed/rerank models use a pool of contexts for parallel processing.
	// Generation models use the batch engine with a primary context.
	nSlots := max(cfg.NSeqMax(), 1)

	switch {
	case modelInfo.IsEmbedModel || modelInfo.IsRerankModel:
		pool, err := newContextPool(ctx, mdl, ctxParams, l, nSlots)
		if err != nil {
			llama.ModelFree(mdl)
			return nil, fmt.Errorf("new-context-pool: unable to create context pool: %w", err)
		}
		m.pool = pool

	default:
		// Generation models need a primary context for the batch engine.
		lctx, err := llama.InitFromModel(mdl, ctxParams)
		if err != nil {
			llama.ModelFree(mdl)
			return nil, fmt.Errorf("init-from-model: unable to init context: %w", err)
		}

		mem, err := llama.GetMemory(lctx)
		if err != nil {
			llama.Free(lctx)
			llama.ModelFree(mdl)
			return nil, fmt.Errorf("get-memory: unable to get memory: %w", err)
		}

		llama.MemoryClear(mem, true)

		m.lctx = lctx
		m.mem = mem

		// Initialize IMC sessions (one per slot). Transitional: sessions are
		// currently slot-indexed. Future phases will decouple from slots.
		if cfg.IncrementalCache() {
			m.imcSessions = make([]*imcSession, nSlots)
			for i := range nSlots {
				m.imcSessions[i] = &imcSession{
					slotID: i,
					seqID:  llama.SeqId(i),
				}
			}
			m.cacheCond = sync.NewCond(&m.cacheMu)
		}

		m.batch = newBatchEngine(&m, nSlots)
		m.batch.start(ctx)

		// Initialize draft model for speculative decoding if configured.
		if cfg.DraftModel != nil {
			draft, err := loadDraftModel(ctx, l, cfg, mdl, ctxParams)
			if err != nil {
				m.batch.stop(ctx)
				m.batch.freeBatch()
				llama.Free(lctx)
				llama.ModelFree(mdl)
				return nil, fmt.Errorf("load-draft-model: %w", err)
			}
			m.draft = draft
			l(ctx, "draft-model", "status", "loaded",
				"nDraft", draft.nDraft, "devices", cfg.DraftModel.Devices,
				"nCtx", llama.NCtx(draft.lctx))
		}
	}

	return &m, nil
}

// loadDraftModel loads the draft model for speculative decoding. It creates
// a separate model, context, and greedy sampler. The draft model uses the
// same context window as the target to support long prompts.
func loadDraftModel(ctx context.Context, log applog.Logger, cfg Config, targetModel llama.Model, targetCtxParams llama.ContextParams) (*draftModel, error) {
	dCfg := cfg.DraftModel

	// Load draft model.
	mParams := llama.ModelDefaultParams()
	switch {
	case dCfg.PtrNGpuLayers == nil:
		mParams.NGpuLayers = -1
	case *dCfg.PtrNGpuLayers == 0:
		mParams.NGpuLayers = -1
	case *dCfg.PtrNGpuLayers == -1:
		mParams.NGpuLayers = 0
	default:
		mParams.NGpuLayers = int32(*dCfg.PtrNGpuLayers)
	}

	var draftDevicesBuf []llama.GGMLBackendDevice
	if len(dCfg.Devices) > 0 {
		resolved, err := resolveBackendDevices(dCfg.Devices)
		if err != nil {
			return nil, fmt.Errorf("draft-resolve-devices: %w", err)
		}
		if err := mParams.SetDevices(resolved); err != nil {
			return nil, fmt.Errorf("draft-set-devices: %w", err)
		}
		draftDevicesBuf = resolved
	}

	if dCfg.PtrMainGPU != nil {
		mParams.MainGpu = int32(*dCfg.PtrMainGPU)
	}

	var draftTensorSplitBuf []float32
	if len(dCfg.TensorSplit) > 0 {
		draftTensorSplitBuf = make([]float32, len(dCfg.TensorSplit))
		copy(draftTensorSplitBuf, dCfg.TensorSplit)
		mParams.TensorSplit = &draftTensorSplitBuf[0]
	}

	log(ctx, "draft-model", "status", "loading",
		"files", fmt.Sprintf("%v", dCfg.ModelFiles),
		"devices", fmt.Sprintf("%v", dCfg.Devices),
		"nDraft", dCfg.NDraft,
		"gpu_layers", mParams.NGpuLayers)

	dModel, err := loadModelFromFiles(ctx, log, dCfg.ModelFiles, mParams)
	runtime.KeepAlive(draftDevicesBuf)
	runtime.KeepAlive(draftTensorSplitBuf)
	if err != nil {
		return nil, fmt.Errorf("unable to load draft model: %w", err)
	}

	// Validate vocabulary compatibility.
	dVocab := llama.ModelGetVocab(dModel)
	targetVocab := llama.ModelGetVocab(targetModel)
	targetVocabSize := llama.VocabNTokens(targetVocab)
	draftVocabSize := llama.VocabNTokens(dVocab)

	log(ctx, "draft-model", "status", "vocab-check",
		"target_vocab", targetVocabSize, "draft_vocab", draftVocabSize)

	if draftVocabSize != targetVocabSize {
		llama.ModelFree(dModel)
		return nil, fmt.Errorf("vocabulary mismatch: target has %d tokens, draft has %d tokens",
			targetVocabSize, draftVocabSize)
	}

	// Create draft context with same context window as target.
	dCtxParams := llama.ContextDefaultParams()
	dCtxParams.NCtx = targetCtxParams.NCtx
	dCtxParams.NBatch = targetCtxParams.NBatch
	dCtxParams.NUbatch = targetCtxParams.NUbatch
	dCtxParams.NSeqMax = 1
	dCtxParams.FlashAttentionType = targetCtxParams.FlashAttentionType
	dCtxParams.NThreads = targetCtxParams.NThreads
	dCtxParams.NThreadsBatch = targetCtxParams.NThreadsBatch

	dLctx, err := llama.InitFromModel(dModel, dCtxParams)
	if err != nil {
		llama.ModelFree(dModel)
		return nil, fmt.Errorf("unable to init draft context: %w", err)
	}

	dMem, err := llama.GetMemory(dLctx)
	if err != nil {
		llama.Free(dLctx)
		llama.ModelFree(dModel)
		return nil, fmt.Errorf("unable to get draft memory: %w", err)
	}

	llama.MemoryClear(dMem, true)

	// Create greedy sampler for draft model (temperature=0 for speed).
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	llama.SamplerChainAdd(sampler, llama.SamplerInitGreedy())

	// Create reusable batch for drafting (1 token at a time).
	batch := llama.BatchInit(1, 0, 1)

	// Create reusable batch for prefill decoding (sized to nBatch).
	prefillBatch := llama.BatchInit(int32(dCtxParams.NBatch), 0, 1)

	// Pre-allocate reusable buffers for speculative sampling.
	nVocab := int(llama.VocabNTokens(dVocab))
	draftProbs := make([][]float32, dCfg.NDraft)
	for i := range draftProbs {
		draftProbs[i] = make([]float32, nVocab)
	}

	return &draftModel{
		model:        dModel,
		vocab:        dVocab,
		lctx:         dLctx,
		mem:          dMem,
		sampler:      sampler,
		batch:        batch,
		prefillBatch: prefillBatch,
		nDraft:       dCfg.NDraft,
		draftBuf:     make([]llama.Token, 0, dCfg.NDraft),
		draftProbs:   draftProbs,
		targetProbs:  make([]float32, nVocab),
		adjusted:     make([]float32, nVocab),
	}, nil
}

// buildDraftSampler creates a sampler chain for draft token generation that
// matches the request's sampling parameters. This ensures the draft model's
// proposal distribution q(x) is consistent with the request's temperature,
// top-k, and other settings.
func buildDraftSampler(params Params) llama.Sampler {
	chain := llama.SamplerChainInit(llama.SamplerChainDefaultParams())

	// Build chain in the standard order: truncation → temperature → dist.
	llama.SamplerChainAdd(chain, llama.SamplerInitTopK(params.TopK))
	llama.SamplerChainAdd(chain, llama.SamplerInitTopP(params.TopP, 0))
	llama.SamplerChainAdd(chain, llama.SamplerInitMinP(params.MinP, 0))
	llama.SamplerChainAdd(chain, llama.SamplerInitTempExt(params.Temperature, 0, 1.0))
	llama.SamplerChainAdd(chain, llama.SamplerInitDist(llama.DefaultSeed))

	return chain
}

func loadModelFromFiles(ctx context.Context, log applog.Logger, modelFiles []string, params llama.ModelParams) (llama.Model, error) {
	baseModelFile := path.Base(modelFiles[0])

	log(ctx, "loading model from file", "status", "started", "model", baseModelFile)
	defer log(ctx, "loading model from file", "status", "completed", "model", baseModelFile)

	_, span := otel.AddSpan(ctx, "model-file-load-time",
		attribute.String("model-file", baseModelFile),
	)
	defer span.End()

	var err error
	var mdl llama.Model

	switch len(modelFiles) {
	case 1:
		mdl, err = llama.ModelLoadFromFile(modelFiles[0], params)
		if err != nil {
			return 0, fmt.Errorf("model-load-from-file: unable to load model: %w", err)
		}

	default:
		mdl, err = llama.ModelLoadFromSplits(modelFiles, params)
		if err != nil {
			return 0, fmt.Errorf("model-load-from-splits: unable to load model from split: %w", err)
		}
	}

	return mdl, nil
}

func retrieveTemplate(cfg Config, mdl llama.Model) (Template, error) {
	if cfg.JinjaFile != "" {
		data, err := readJinjaTemplate(cfg.JinjaFile)
		if err != nil {
			return Template{}, fmt.Errorf("read-jinja-template: failed to read jinja template: %w", err)
		}

		if data == "" {
			return Template{}, fmt.Errorf("read-jinja-template: jinja template is empty")
		}

		return Template{
			FileName: cfg.JinjaFile,
			Script:   data,
		}, nil
	}

	data := llama.ModelChatTemplate(mdl, "")
	if data == "" {
		data, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
	}

	return Template{
		FileName: "tokenizer.chat_template",
		Script:   data,
	}, nil
}

func (m *Model) Unload(ctx context.Context) error {
	if !m.unloaded.CompareAndSwap(false, true) {
		return nil // Already unloaded
	}

	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// Stop the batch engine if running.
	hasBatch := m.batch != nil
	if hasBatch {
		m.batch.stop(ctx)
	}

	m.log(ctx, "unload", "status", "waiting-for-streams", "active", m.activeStreams.Load())

	for m.activeStreams.Load() > 0 {
		select {
		case <-ctx.Done():
			return fmt.Errorf("unload: cannot unload %d active streams: %w", m.activeStreams.Load(), ctx.Err())

		case <-time.After(100 * time.Millisecond):
		}
	}

	m.log(ctx, "unload", "status", "streams-drained")

	// Free draft model resources if loaded.
	if m.draft != nil {
		if m.draft.registeredSampler != 0 {
			llama.SetSampler(m.draft.lctx, m.draft.registeredSeqID, 0)
			m.draft.registeredSampler = 0
		}
		llama.SamplerFree(m.draft.sampler)
		llama.BatchFree(m.draft.batch)
		llama.BatchFree(m.draft.prefillBatch)
		llama.Free(m.draft.lctx)
		llama.ModelFree(m.draft.model)
		m.draft = nil
		m.log(ctx, "unload", "status", "draft-model-freed")
	}

	// Free batch buffer before context (batch references context internals).
	if hasBatch {
		m.batch.freeBatch()
	}

	// Close the context pool if running (embed/rerank models).
	if m.pool != nil {
		m.pool.close()
	}

	// Free primary context if it exists (generation models only).
	if m.lctx != 0 {
		llama.Synchronize(m.lctx)
		llama.Free(m.lctx)
	}

	llama.ModelFree(m.model)

	return nil
}

func (m *Model) Config() Config {
	return m.cfg
}

func (m *Model) ModelInfo() ModelInfo {
	return m.modelInfo
}

func (m *Model) resetContext() {
	llama.Synchronize(m.lctx)

	mem, err := llama.GetMemory(m.lctx)
	if err == nil {
		llama.MemoryClear(mem, true)
	}

	m.clearCaches()
}

func (m *Model) isUnnecessaryCRLF(reasonFlag int, completionFlag int, content string) bool {
	// We just started reasoning or tool calling so remove leading CR.
	if reasonFlag == 1 && content == "\x0A" {
		return true
	}

	// We just started completion so remove leading CR.
	if completionFlag == 1 && (content == "\x0A\x0A" || content == "\x0A") {
		return true
	}

	return false
}

func (m *Model) sendDeltaResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, choiceIndex int, prompt string, content string, reasonFlag int, outputTokens int, logprob *ContentLogprob) error {
	if outputTokens%500 == 0 {
		m.log(ctx, "chat-completion", "status", "delta", "id", id, "tokens", outputTokens, "object", object, "reasoning", reasonFlag, "content", len(content))
	}

	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.ID, choiceIndex, prompt, ctx.Err(), Usage{}):
		default:
		}

		return ctx.Err()

	case ch <- chatResponseDelta(id, object, m.modelInfo.ID, choiceIndex, content, reasonFlag > 0, logprob):
	}

	return nil
}

func (m *Model) sendFinalResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, choiceIndex int, prompt string, finalContent *strings.Builder, finalReasoning *strings.Builder, respToolCalls []ResponseToolCall, logprobsData []ContentLogprob, streaming bool, usage Usage) {
	args := []any{"status", "final", "id", id, "tokens", usage.OutputTokens, "object", object, "tooling", len(respToolCalls) > 0, "reasoning", finalReasoning.Len(), "content", finalContent.Len()}
	if usage.DraftTokens > 0 {
		args = append(args, "draft_tokens", usage.DraftTokens, "draft_accepted_tokens", usage.DraftAcceptedTokens, "acceptance_rate", fmt.Sprintf("%.2f", usage.DraftAcceptanceRate))
	}
	m.log(ctx, "chat-completion", args...)

	// For streaming responses, logprobs were already sent per-delta chunk.
	// Only include accumulated logprobs for non-streaming requests.
	finalLogprobs := logprobsData
	if streaming {
		finalLogprobs = nil
	}

	select {
	case <-ctx.Done():
		select {
		case ch <- ChatResponseErr(id, object, m.modelInfo.ID, choiceIndex, prompt, ctx.Err(), usage):
		default:
		}

	case ch <- chatResponseFinal(id, object, m.modelInfo.ID, choiceIndex, prompt,
		finalContent.String(),
		finalReasoning.String(),
		respToolCalls,
		finalLogprobs,
		usage):
	}

	contextTokens := usage.PromptTokens + usage.CompletionTokens
	contextWindow := m.cfg.ContextWindow()
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	m.log(ctx, "chat-completion (send final response)", "prompt", usage.PromptTokens, "output", usage.OutputTokens,
		"context", contextTokens, "down", fmt.Sprintf("(%.0f%% of %.0fK) TPS: %.2f", percentage, of, usage.TokensPerSecond))
}

func (m *Model) sendErrorResponse(ctx context.Context, ch chan<- ChatResponse, id string, object string, choiceIndex int, prompt string, err error, usage Usage) {
	m.log(ctx, "chat-completion", "status", "ERROR", "msg", err, "id", id, "object", object)

	select {
	case <-ctx.Done():

	case ch <- ChatResponseErr(id, object, m.modelInfo.ID, choiceIndex, prompt,
		err,
		usage):
	}
}

func calculateVRAM(cfg Config, mi ModelInfo) (vramTotal int64, slotMemory int64) {
	arch := mi.Metadata["general.architecture"]
	if arch == "" {
		return int64(mi.Size), 0
	}

	blockCount, err := strconv.ParseInt(mi.Metadata[arch+".block_count"], 10, 64)
	if err != nil {
		return int64(mi.Size), 0
	}

	headCountKV, err := parseMetadataInt64OrArrayAvg(mi.Metadata, arch+".attention.head_count_kv")
	if err != nil {
		return int64(mi.Size), 0
	}

	keyLength, valueLength, err := resolveKVLengths(mi.Metadata, arch)
	if err != nil {
		return int64(mi.Size), 0
	}

	bytesPerElement := ggmlTypeToBytes(cfg.CacheTypeK, cfg.CacheTypeV)

	nSeqMax := int64(max(cfg.NSeqMax(), 1))

	contextWindow := int64(cfg.ContextWindow())

	kvPerTokenPerLayer := headCountKV * (keyLength + valueLength) * bytesPerElement
	kvPerSlot := contextWindow * blockCount * kvPerTokenPerLayer
	slotMemory = nSeqMax * kvPerSlot
	vramTotal = int64(mi.Size) + slotMemory

	return vramTotal, slotMemory
}

// resolveKVLengths returns key_length and value_length for VRAM calculation.
// It first checks for explicit metadata keys. When those are missing (e.g.
// audio models like Qwen2-Audio), it falls back to embedding_length / head_count
// which is the same default llama.cpp uses internally.
func resolveKVLengths(metadata map[string]string, arch string) (keyLen int64, valLen int64, err error) {
	keyLen, keyErr := strconv.ParseInt(metadata[arch+".attention.key_length"], 10, 64)
	valLen, valErr := strconv.ParseInt(metadata[arch+".attention.value_length"], 10, 64)

	if keyErr == nil && valErr == nil {
		return keyLen, valLen, nil
	}

	embLen, err := strconv.ParseInt(metadata[arch+".embedding_length"], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("resolve-kv-lengths: key_length and embedding_length both missing")
	}

	headCount, err := strconv.ParseInt(metadata[arch+".attention.head_count"], 10, 64)
	if err != nil || headCount == 0 {
		return 0, 0, fmt.Errorf("resolve-kv-lengths: key_length and head_count both missing")
	}

	derived := embLen / headCount

	if keyErr != nil {
		keyLen = derived
	}
	if valErr != nil {
		valLen = derived
	}

	return keyLen, valLen, nil
}

// parseMetadataInt64OrArrayAvg parses a metadata value that may be either a
// single integer (e.g. "8") or a per-layer array. Arrays produced by our own
// GGUF parser are space-separated ("[0 0 8 ...]") while arrays surfaced by
// llama.cpp's gguf_kv_to_str are comma-separated ("[0, 0, 8, ...]"); both are
// accepted. For arrays, the average of all elements is returned. This handles
// hybrid architectures like LFM2 and MoE Gemma where head_count_kv varies per
// layer.
func parseMetadataInt64OrArrayAvg(metadata map[string]string, key string) (int64, error) {
	val, ok := metadata[key]
	if !ok {
		return 0, fmt.Errorf("parse-metadata-int64: metadata key %q not found", key)
	}

	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		return n, nil
	}

	trimmed := strings.TrimSpace(val)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return 0, fmt.Errorf("parse-metadata-int64: unable to parse %q for key %q", val, key)
	}

	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return 0, fmt.Errorf("parse-metadata-int64: empty array for key %q", key)
	}

	splitFn := func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}
	fields := strings.FieldsFunc(inner, splitFn)

	var sum int64
	var count int64
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		n, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse-metadata-int64: unable to parse array element %q for key %q: %w", f, key, err)
		}
		sum += n
		count++
	}

	if count == 0 {
		return 0, fmt.Errorf("parse-metadata-int64: empty array for key %q", key)
	}

	return sum / count, nil
}

func ggmlTypeToBytes(typeK, typeV GGMLType) int64 {
	bytesK := ggmlBytes(typeK)
	bytesV := ggmlBytes(typeV)

	if bytesK > bytesV {
		return bytesK
	}
	return bytesV
}

func ggmlBytes(t GGMLType) int64 {
	switch t {
	case GGMLTypeF32:
		return 4
	case GGMLTypeF16, GGMLTypeBF16:
		return 2
	case GGMLTypeQ8_0:
		return 1
	case GGMLTypeQ4_0, GGMLTypeQ4_1, GGMLTypeQ5_0, GGMLTypeQ5_1:
		return 1
	default:
		return 2
	}
}

// resolveBackendDevice maps a user-facing device name to the ggml backend
// device handle. ROCm libraries register under the "hip" backend name in
// llama.cpp, so "rocm" is treated as an alias for "hip".
func resolveBackendDevice(name string) llama.GGMLBackendDevice {
	candidates := []string{name}

	if strings.EqualFold(name, "rocm") {
		candidates = []string{"hip", "HIP", name}
	}

	for _, c := range candidates {
		if dev := llama.GGMLBackendDeviceByName(c); dev != 0 {
			return dev
		}
	}

	return 0
}

// resolveBackendDevices resolves a list of device names to ggml backend device
// handles. The returned slice is NULL-terminated as required by llama.cpp.
// Returns an error if any device name cannot be resolved.
func resolveBackendDevices(names []string) ([]llama.GGMLBackendDevice, error) {
	devices := make([]llama.GGMLBackendDevice, 0, len(names)+1)
	for _, name := range names {
		dev := resolveBackendDevice(name)
		if dev == 0 {
			return nil, fmt.Errorf("unknown device: %s", name)
		}
		devices = append(devices, dev)
	}
	devices = append(devices, 0) // NULL terminator
	return devices, nil
}

// parseTensorBuftOverrides converts config string patterns into yzma
// TensorBuftOverride values. The returned slice is sentinel-terminated
// (last element has Pattern == nil) as required by llama.cpp.
// Supports shortcuts:
//   - "all-ffn": offload all FFN expression tensors to CPU
//   - "block:N": offload FFN tensors for block N to CPU
//   - any other string: treated as a raw regex pattern
func parseTensorBuftOverrides(patterns []string) ([]llama.TensorBuftOverride, error) {
	overrides := make([]llama.TensorBuftOverride, 0, len(patterns)+1)
	for _, p := range patterns {
		var o llama.TensorBuftOverride
		switch {
		case p == "moe-experts":
			o = llama.NewTensorBuftAllFFNExprsOverride()
		case strings.HasPrefix(p, "moe-experts:block:"):
			idx, err := strconv.Atoi(strings.TrimPrefix(p, "moe-experts:block:"))
			if err != nil {
				return nil, fmt.Errorf("invalid block index in %q: %w", p, err)
			}
			o = llama.NewTensorBuftBlockOverride(idx)
		case p == "all-ffn":
			o = llama.NewTensorBuftAllFFNExprsOverride()
		case strings.HasPrefix(p, "block:"):
			idx, err := strconv.Atoi(strings.TrimPrefix(p, "block:"))
			if err != nil {
				return nil, fmt.Errorf("invalid block index in %q: %w", p, err)
			}
			o = llama.NewTensorBuftBlockOverride(idx)
		default:
			o = llama.NewTensorBuftOverride(p)
		}
		overrides = append(overrides, o)
	}
	overrides = append(overrides, llama.TensorBuftOverride{}) // sentinel
	return overrides, nil
}

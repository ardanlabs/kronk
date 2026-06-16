// Package model provides the low-level api for working with models.
package model

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/jinja"
	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// modelLoadMu serializes model loading to prevent concurrent mutation of
// process-level environment variables (e.g., GGML_OP_OFFLOAD_MIN_BATCH).
var modelLoadMu sync.Mutex

// compiledTemplate holds a pre-compiled Jinja template. Compiled once per
// Model via Model.templateOnce / Model.compiledTmpl in applyJinjaTemplate.
type compiledTemplate struct {
	tmpl *jinja.Template
	err  error
}

// imcSessionMultiplier is the per-slot multiplier for the IMC session
// pool. With NSeqMax execution slots the pool holds NSeqMax *
// imcSessionMultiplier cache identities, which decouples how many
// distinct conversation prefixes the server can keep warm from how
// many requests can decode in parallel. 3x matches the realistic
// agentic shape of a driver loop plus a handful of sub-agents plus
// the occasional side conversation. Session structs cost only a few
// hundred bytes when idle; the SessionStore backing buffer is
// allocated lazily on first use, so unused sessions cost essentially
// nothing.
const imcSessionMultiplier = 3

// imcSeqIDUnbound marks an IMC session that is not currently resident
// in any execution slot's KV sequence. A session's seqID is set when
// startSlot binds it to a slot and reset to imcSeqIDUnbound in
// finishSlot after the slot's sequence is cleared. The KV-pressure
// eviction path uses this sentinel to skip MemorySeqRm calls for
// sessions whose bytes only live in host RAM (via SessionStore).
const imcSeqIDUnbound llama.SeqId = -1

// imcSession holds the state for a single IMC (Incremental Message Cache)
// session. Sessions are session-pool entries, sized at NSeqMax *
// imcSessionMultiplier so the cache identity count is larger than the
// execution slot count. Each session externalizes its cached KV state
// via SessionStore between requests; the scheduler binds a session to
// whichever execution slot is free at startSlot time. The seqID field
// is dynamic: it holds the bound slot's KV sequence id while a
// request is in flight, and imcSeqIDUnbound otherwise.
type imcSession struct {
	id                int           // Stable session-pool index. Used by imcClearPending lookup and for log correlation; not related to execution slot identity.
	seqID             llama.SeqId   // KV sequence id the session is currently bound to, or imcSeqIDUnbound when externalized to RAM only.
	cachedMsgsHash    string        // Hash of all cached messages
	cachedTokens      []llama.Token // Full token sequence in KV cache (immutable; replaced, never mutated)
	totalTokensCached int           // Total KV positions cached (includes text + media tokens)
	cachedMsgCount    int           // Number of messages cached
	kvState           SessionStore  // Externalized KV cache state, accessed via the pluggable SessionStore interface. The default RAM impl (kvstorage/ram.Store) restores into any slot via StateSeqSetData with lazy-grow / never-shrink semantics: backing storage is retained across snapshots and session rebinds to eliminate per-turn allocation churn.
	draftKVState      SessionStore  // Externalized MTP draft seq KV state. Nil unless the model has an MTP drafter (allocated post-draft-load in initGenerationRuntime). Captured alongside kvState during cache build so a cache hit on the next request can restore the draft seq in lock-step with the target seq and MTP can keep running for IMC-cache-hit requests.
	pendingH          []float32     // Copy of the slot's pre-norm hidden row taken at cache-build snapshot time (one row of nEmbd floats). Restored into slot.pendingH on cache hit so the very first MTP draft round can condition correctly on the cached prefix's last position. Empty when the session has no MTP draft snapshot.
	lastUsed          time.Time     // Last access time (for eviction)
	pending           bool          // True when a build/extend is in-flight on this session — protects kvState from concurrent writers.
	hasMedia          bool          // True if the cached content includes media tokens (image/audio)
	useMRoPE          bool          // True if the cached media used M-RoPE 4D positional encoding
	mediaKVCounts     []int         // KV positions consumed per media chunk (image/audio); used for text-only extend math
	sysPromptHash     string        // Hash of the system prompt message (messages[0] when role="system")
	sysPromptTokens   int           // Token count of the system prompt in the KV cache

	// cachedRenderInputHash is the imcRenderFingerprint of the inputs that
	// produced the currently-cached prefix (template, add_generation_prompt,
	// preserve_thinking, cacheable messages, top-level tools). It is set by
	// imcCommitSession and consumed by the pure-hit snapshot-skip predicate
	// in startSlot. Empty string means "do not skip" — pre-rollout sessions
	// and sessions for which fingerprinting failed naturally disqualify.
	cachedRenderInputHash string
}

// draftModel holds resources for the draft model used in speculative decoding.
// The draft model is a smaller, faster model that generates candidate tokens
// for the target model to verify in a single forward pass.
//
// Two operating modes are supported:
//
//   - Separate-GGUF draft (mtp == false): a distinct, smaller GGUF loaded
//     into its own llama_model + context. Token-only decode loop (no
//     hidden-state plumbing). Uses llama.DraftGenerate.
//
//   - MTP draft (mtp == true): an MTP (multi-token-prediction) head living
//     inside the TARGET GGUF (Qwen3.5 / Qwen3.6 architecture qwen35). The
//     llama_model pointer is SHARED with the target — there is no extra
//     file. The MTP head takes (token_id, pre_norm_hidden_state) per
//     position, so every target llama_decode must be mirrored into the
//     draft context with batch.embd populated from
//     llama_get_embeddings_pre_norm. See loadDraftModelMTP and the
//     batch_mtp.go mirroring helpers.
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

	// MTP-only fields. When mtp == true the draft context shares its
	// llama_model with the target (Unload must skip the ModelFree).
	mtp   bool
	nEmbd int // model embedding width; size of one pre-norm hidden row

	// MTP batches carry pre-norm hidden-state vectors alongside token
	// ids. llama_batch_init allocates EITHER the token buffer OR the
	// embd buffer (depending on the embd arg) — never both. MTP needs
	// both, so we call BatchInit(N, 0, 1) to get a token-only batch
	// and then attach a Go-allocated embd buffer below.
	//
	//   draftBatchMTP   : AR per-step draft decode (capacity 1 token).
	//   mirrorBatchMTP  : mirror a target batch into the draft KV
	//                     (capacity nBatch).
	//
	// The embd buffers are Go-owned ([]float32) and pinned via
	// runtime.Pinner so the GC can't relocate them while the C side
	// reads through Batch.Embd. Pins are released and Batch.Embd is
	// nilled out before BatchFree (which would otherwise free() Go
	// memory and crash).
	draftBatchMTP   llama.Batch
	mirrorBatchMTP  llama.Batch
	draftEmbdSlice  []float32      // Go-owned backing for draftBatchMTP.Embd  (size 1*nEmbd)
	mirrorEmbdSlice []float32      // Go-owned backing for mirrorBatchMTP.Embd (size NBatch*nEmbd)
	draftEmbdPin    runtime.Pinner // Keeps draftEmbdSlice[0] address stable while the batch is alive.
	mirrorEmbdPin   runtime.Pinner // Keeps mirrorEmbdSlice[0] address stable while the batch is alive.

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
	cfg          Config
	log          applog.Logger
	model        llama.Model
	vocab        llama.Vocab
	ctxParams    llama.ContextParams
	lctx         llama.Context
	mem          llama.Memory
	batch        *batchEngine
	template     Template
	compiledTmpl *compiledTemplate // Long-lived compiled jinja template (one-time init via templateOnce).
	templateOnce sync.Once         // Guards one-time compile of compiledTmpl.
	projFile     string
	// mtmdMetaCtx is a single, long-lived multimodal projector context
	// loaded in NewModel and freed in Unload. It is used ONLY for
	// read-only metadata checks (SupportVision/SupportAudio) by chat
	// handlers — never for Tokenize/Encode/Decode of actual requests.
	// Per-request slots create their own short-lived mtmd context (via
	// mtmd.InitFromFile in startSlot, freed in freeSlotResources) so
	// that any internal accumulation inside an mtmd context is bounded
	// to a single request and cannot bleed across requests.
	// Zero when projFile == "" (text-only models) or for embed/rerank
	// models.
	mtmdMetaCtx       mtmd.Context
	modelInfo         ModelInfo
	activeStreams     atomic.Int32
	unloaded          atomic.Bool
	decodeMu          sync.Mutex
	cacheMu           sync.RWMutex
	cacheCond         *sync.Cond    // Signaled when an IMC session's pending flag is cleared (a build/extend has finished).
	imcSessions       []*imcSession // IMC session pool, sized NSeqMax * imcSessionMultiplier; sessions migrate freely between slots via SessionStore. Idle sessions cost only the struct itself — the SessionStore buffer is allocated lazily on first use.
	addBOSToken       bool          // Whether to add BOS token (from model metadata)
	mediaMarkerTokens int           // Token count for the media marker string; computed once via mediaMarkerOnce
	mediaMarkerOnce   sync.Once     // Guards one-time computation of mediaMarkerTokens
	pool              *contextPool  // Context pool for parallel embed/rerank
	parser            Parser        // Selected via selectParser at load time; nil for embed/rerank.
	draft             *draftModel   // Draft model for speculative decoding
}

// NewModel loads a model from the GGUF files specified in cfg and returns
// a *Model ready to serve requests. It validates the configuration, builds
// llama.cpp model parameters, applies NUMA settings, performs the actual
// GGUF load (serialized via a process-wide mutex to guard the
// GGML_OP_OFFLOAD_MIN_BATCH env var), computes VRAM/KV diagnostics,
// retrieves the chat template, and initializes the per-model runtime —
// either a context pool for embed/rerank models or a batch engine plus
// parser plugin and optional draft model for generation models.
//
// The returned *Model owns the underlying llama.Model, llama.Context, KV
// memory, batch engine, and (when configured) draft model; release them
// via Model.Unload when finished.
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

	// -------------------------------------------------------------------------

	mParams, ka, err := buildModelParams(ctx, &cfg, l)
	if err != nil {
		return nil, err
	}

	applyNUMA(ctx, cfg, l)

	// -------------------------------------------------------------------------

	mdl, loadDuration, err := loadModelWithEnvGuard(ctx, l, cfg, mParams, ka)
	if err != nil {
		return nil, err
	}

	cfg = adjustConfig(cfg, mdl)
	modelInfo := toModelInfo(cfg, mdl)

	metrics.AddModelFileLoadTime(modelInfo.ID, loadDuration)

	// -------------------------------------------------------------------------

	var vramDiag string
	modelInfo.VRAMTotal, modelInfo.SlotMemory, vramDiag = calculateVRAMDiag(cfg, modelInfo)

	// The SDK-side calculation can return slot-memory=0 for architectures
	// whose head_count_kv is stored as a per-layer ARRAY (notably
	// gemma3/gemma4). llama.cpp's gguf_kv_to_str returns false for ARRAY
	// values so those keys never make it into modelInfo.Metadata. The pool
	// (and the BUI display path it feeds) overlays a more accurate value
	// using sdk/tools/models.CalculateVRAM in that case.
	l(ctx, "calculate-vram",
		"vram-total", humanBytes(modelInfo.VRAMTotal),
		"slot-memory", humanBytes(modelInfo.SlotMemory),
		"diag", vramDiag,
	)

	metrics.SetVRAM(modelInfo.ID, modelInfo.VRAMTotal, modelInfo.SlotMemory)

	template, err := retrieveTemplate(cfg, modelInfo.ID, mdl)
	if err != nil {
		llama.ModelFree(mdl)
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

	ctxParams := modelCtxParams(cfg, modelInfo, mdl)

	// Reflect the KV cache types that llama.cpp will actually use back into
	// cfg. When the user leaves CacheTypeK/V as GGMLTypeAuto, modelCtxParams
	// lets llama_context_default_params() pick (typically f16). Surfacing
	// those resolved values via ModelConfig() keeps user-facing diagnostics
	// honest — krn.ModelConfig().CacheTypeK now reports "f16" instead of
	// "auto" when no explicit value was set.
	cfg.CacheTypeK = GGMLTypeFromYZMA(ctxParams.TypeK)
	cfg.CacheTypeV = GGMLTypeFromYZMA(ctxParams.TypeV)

	l(ctx, "MODEL-INFO", "values", modelInfo.String(), "addBOSToken", addBOSToken)
	l(ctx, "MODEL-CONFIG", "values", cfg.String())

	logMoEConfig(ctx, cfg, l)
	logContextParamsTrace(ctx, ctxParams, l)

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

	// Load a single, long-lived mtmd context used ONLY for metadata
	// reads (SupportVision/SupportAudio) by chat handlers. Per-request
	// processing contexts are created by each slot in startSlot and
	// freed in freeSlotResources — see Model.mtmdMetaCtx for the
	// rationale.
	isGenerationModel := !(modelInfo.IsEmbedModel || modelInfo.IsRerankModel)
	if isGenerationModel && m.projFile != "" {
		l(ctx, "loading-prof-file", "status", "started", "proj", path.Base(m.projFile))

		start := time.Now()

		mtmdCtx, err := mtmd.InitFromFile(m.projFile, m.model, mtmdContextParams(cfg))
		if err != nil {
			llama.ModelFree(mdl)
			return nil, fmt.Errorf("init-mtmd-meta-context: %w", err)
		}
		m.mtmdMetaCtx = mtmdCtx

		metrics.AddProjFileLoadTime(m.modelInfo.ID, time.Since(start))

		l(ctx, "loading-prof-file", "status", "completed", "proj", path.Base(m.projFile))
	}

	switch {
	case !isGenerationModel:
		pool, err := newContextPool(ctx, mdl, ctxParams, l, nSlots)
		if err != nil {
			llama.ModelFree(mdl)
			return nil, fmt.Errorf("new-context-pool: unable to create context pool: %w", err)
		}
		m.pool = pool

	default:
		if err := initGenerationRuntime(ctx, &m, nSlots); err != nil {
			if m.mtmdMetaCtx != 0 {
				mtmd.Free(m.mtmdMetaCtx)
			}
			llama.ModelFree(mdl)
			return nil, err
		}
	}

	return &m, nil
}

// modelParamsKeepalive holds backing buffers that the C side of llama.cpp
// reads via pointer during ModelLoadFromFile. They must outlive the load
// call; the caller passes this to loadModelWithEnvGuard which holds the
// references via runtime.KeepAlive across the load.
type modelParamsKeepalive struct {
	devices     []llama.GGMLBackendDevice
	tensorSplit []float32
	tensorBuft  []llama.TensorBuftOverride
}

// buildModelParams translates Config into llama.ModelParams. It mutates cfg
// (passed by pointer so the changes are visible to the caller): it fills
// cfg.TensorBuftOverrides when MoE compilation produces an implicit override
// list, and writes the resolved cfg.PtrSplitMode back when none was set so
// ModelConfig() reports the effective device-aware split mode.
func buildModelParams(ctx context.Context, cfg *Config, l applog.Logger) (llama.ModelParams, modelParamsKeepalive, error) {
	mParams := llama.ModelDefaultParams()
	var ka modelParamsKeepalive

	if len(cfg.Devices) > 0 {
		resolved, err := resolveBackendDevices(cfg.Devices)
		if err != nil {
			return mParams, ka, fmt.Errorf("resolve-devices: %w", err)
		}
		if err := mParams.SetDevices(resolved); err != nil {
			return mParams, ka, fmt.Errorf("set-devices: %w", err)
		}
		ka.devices = resolved
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
	// When not explicitly configured, fall back to a device-count aware default
	// (DefaultSplitMode): SplitModeRow only with multiple GPUs, otherwise
	// SplitModeLayer. This is the universal floor every load path shares, so it
	// protects callers (including direct SDK users) that never run the hardware
	// analysis. Tensor parallelism on a single GPU is a no-op that performs
	// worse and crashes MoE models with view tensors (e.g. gemma4).
	switch cfg.PtrSplitMode {
	case nil:
		split := DefaultSplitMode(gpuDeviceCount(cfg))
		mParams.SplitMode = split.ToYZMAType()
		// Surface the resolved split mode back into cfg so ModelConfig() reports
		// the effective value ("layer"/"row") instead of nil. This keeps
		// diagnostics honest whether or not auto-tune ran, and confirms tensor
		// parallelism is never silently used on a single GPU.
		cfg.PtrSplitMode = &split
		l(ctx, "BUILD-MODEL-PARAMS", "split_mode", "resolved", split.String())
	default:
		mParams.SplitMode = (*cfg.PtrSplitMode).ToYZMAType()
	}

	if cfg.PtrMainGPU != nil {
		mParams.MainGpu = int32(*cfg.PtrMainGPU)
	}

	// TensorSplit: proportional distribution of layers across multiple GPUs.
	if len(cfg.TensorSplit) > 0 {
		ka.tensorSplit = make([]float32, len(cfg.TensorSplit))
		copy(ka.tensorSplit, cfg.TensorSplit)
		mParams.TensorSplit = &ka.tensorSplit[0]
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
	if len(cfg.TensorBuftOverrides) > 0 {
		overrides, err := parseTensorBuftOverrides(cfg.TensorBuftOverrides)
		if err != nil {
			return mParams, ka, fmt.Errorf("tensor-buft-overrides: %w", err)
		}
		if err := mParams.SetTensorBufOverrides(overrides); err != nil {
			return mParams, ka, fmt.Errorf("set-tensor-buft-overrides: %w", err)
		}
		ka.tensorBuft = overrides
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

	return mParams, ka, nil
}

// applyNUMA initializes the llama.cpp NUMA strategy. NUMA init must happen
// once before any model load.
func applyNUMA(ctx context.Context, cfg Config, l applog.Logger) {
	if cfg.NUMA == "" {
		return
	}

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

// loadModelWithEnvGuard performs the actual GGUF load while serializing the
// process-level GGML_OP_OFFLOAD_MIN_BATCH env var so concurrent loads (e.g.
// target + draft) do not race. The previous env value is saved and restored
// so unrelated callers are unaffected. The keepalive struct keeps the
// param-backing buffers alive across the C call.
func loadModelWithEnvGuard(ctx context.Context, l applog.Logger, cfg Config, mParams llama.ModelParams, ka modelParamsKeepalive) (llama.Model, time.Duration, error) {
	modelLoadMu.Lock()
	defer modelLoadMu.Unlock()

	prevOffloadMinBatch, hadOffloadMinBatch := os.LookupEnv("GGML_OP_OFFLOAD_MIN_BATCH")
	if cfg.OpOffloadMinBatch() > 0 {
		os.Setenv("GGML_OP_OFFLOAD_MIN_BATCH", strconv.Itoa(*cfg.PtrOpOffloadMinBatch))
		l(ctx, "OP-OFFLOAD-MIN-BATCH", "value", *cfg.PtrOpOffloadMinBatch)
	} else {
		os.Unsetenv("GGML_OP_OFFLOAD_MIN_BATCH")
	}
	defer func() {
		if hadOffloadMinBatch {
			os.Setenv("GGML_OP_OFFLOAD_MIN_BATCH", prevOffloadMinBatch)
		} else {
			os.Unsetenv("GGML_OP_OFFLOAD_MIN_BATCH")
		}
	}()

	loadStart := time.Now()
	mdl, err := loadModelFromFiles(ctx, l, cfg.ModelFiles, mParams)
	runtime.KeepAlive(ka.devices)
	runtime.KeepAlive(ka.tensorSplit)
	runtime.KeepAlive(ka.tensorBuft)

	if err != nil {
		return 0, 0, fmt.Errorf("load-model-from-files: unable to load model: %w", err)
	}

	return mdl, time.Since(loadStart), nil
}

// logMoEConfig emits a single MOE-CONFIG line summarizing the effective MoE
// settings. Skipped for unconfigured/auto modes since there is nothing
// meaningful to report.
func logMoEConfig(ctx context.Context, cfg Config, l applog.Logger) {
	if cfg.MoE == nil || cfg.MoE.Mode == "" || cfg.MoE.Mode == MoEModeAuto {
		return
	}

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

// logContextParamsTrace emits the multi-line LLAMA-CONTEXT-PARAMS dump used
// for post-load diagnostics.
func logContextParamsTrace(ctx context.Context, ctxParams llama.ContextParams, l applog.Logger) {
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

	l(ctx, "LLAMA-CONTEXT-PARAMS", "values", fmt.Sprintf("\nEmbeddings[%d]\nFlashAttentionType[%s]\nNBatch[%d]\nNCtx[%d]\nNOutputsMax[%d]\nNSeqMax[%d]\nNThreads[%d]\nNThreadsBatch[%d]\nNUBatch[%d]\nOffloadKQV[%d]\nOpOffload[%d]\nPoolingType[%d]\nRopeFreqBase[%g]\nRopeFreqScale[%g]\nRopeScalingType[%d]\nSwaFull[%d]\nTypeK[%s]\nTypeV[%s]\nYarnAttnFactor[%g]\nYarnBetaFast[%g]\nYarnBetaSlow[%g]\nYarnExtFactor[%g]\nYarnOrigCtx[%d]\n",
		ctxParams.Embeddings, faName, ctxParams.NBatch, ctxParams.NCtx, ctxParams.NOutputsMax,
		ctxParams.NSeqMax, ctxParams.NThreads, ctxParams.NThreadsBatch, ctxParams.NUbatch,
		ctxParams.Offload_kqv, ctxParams.OpOffload, ctxParams.PoolingType,
		ctxParams.RopeFreqBase, ctxParams.RopeFreqScale, ctxParams.RopeScalingType,
		ctxParams.SwaFull, typeKName, typeVName, ctxParams.YarnAttnFactor, ctxParams.YarnBetaFast,
		ctxParams.YarnBetaSlow, ctxParams.YarnExtFactor, ctxParams.YarnOrigCtx))
}

// initGenerationRuntime wires up the generation-only runtime: primary llama
// context, KV memory, IMC sessions, family plugin, batch engine, and the
// optional draft model for speculative decoding. On error the helper frees
// any partial state it created, but leaves m.model for the caller to free.
func initGenerationRuntime(ctx context.Context, m *Model, nSlots int) error {
	lctx, err := llama.InitFromModel(m.model, m.ctxParams)
	if err != nil {
		return fmt.Errorf("init-from-model: unable to init context: %w", err)
	}

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		llama.Free(lctx)
		return fmt.Errorf("get-memory: unable to get memory: %w", err)
	}

	llama.MemoryClear(mem, true)

	m.lctx = lctx
	m.mem = mem

	// Initialize the IMC session pool. Sized at nSlots *
	// imcSessionMultiplier so the cache identity count is larger than
	// the execution slot count: multi-agent workloads keep N distinct
	// prefixes warm without LRU thrashing, while actual decode
	// concurrency stays capped at nSlots in the batch engine. Sessions
	// externalize their KV state via SessionStore so any session can
	// run on any free slot. The SessionStore backing buffer is
	// allocated lazily on first use, so unused sessions cost only the
	// imcSession struct itself.
	if m.cfg.IncrementalCache() {
		nSessions := nSlots * imcSessionMultiplier
		m.imcSessions = make([]*imcSession, nSessions)
		for i := range nSessions {
			store, err := newSessionStore(m.cfg)
			if err != nil {
				return fmt.Errorf("init-generation-runtime: session-store: %w", err)
			}
			m.imcSessions[i] = &imcSession{
				id:      i,
				seqID:   imcSeqIDUnbound,
				kvState: store,
			}
		}
		m.cacheCond = sync.NewCond(&m.cacheMu)
	}

	// Select the parser plugin once at load time. The selected
	// parser lives on *Model for the lifetime of the model and
	// provides the per-slot state machine and tool-call parser used by
	// the batch engine. Embed/rerank models do not reach this branch,
	// so m.parser stays nil for them.
	fp := Fingerprint{
		ChatTemplate: m.template.Script,
		Architecture: m.modelInfo.Metadata["general.architecture"],
		ModelName:    m.modelInfo.ID,
	}

	m.log(ctx, "select-parser",
		"status", "fingerprint",
		"model", fp.ModelName,
		"arch", fp.Architecture,
		"template-len", len(fp.ChatTemplate),
	)

	m.parser = selectParser(fp)

	if m.parser == nil {
		llama.Free(lctx)
		return fmt.Errorf("select-parser: no parser registered for %q (call kronk.registerDefaultParsers or model.RegisterParser(standard.New) at bootstrap)", m.modelInfo.ID)
	}

	m.log(ctx, "select-parser",
		"status", "selected",
		"model", fp.ModelName,
		"parser", m.parser.Name(),
	)

	m.batch = newBatchEngine(m, nSlots)
	m.batch.start(ctx)

	// Initialize draft model for speculative decoding. selectAndLoadDraft
	// picks between an explicit separate-GGUF draft (cfg.DraftModel) and
	// an auto-detected MTP head living inside the target GGUF
	// (nextn_predict_layers > 0, qwen35 architecture). Returns (nil, nil)
	// when no draft applies.
	draft, err := selectAndLoadDraft(ctx, m.log, m.cfg, lctx, m.model, m.ctxParams)
	if err != nil {
		m.batch.stop(ctx)
		m.batch.freeBatch()
		llama.Free(lctx)
		return fmt.Errorf("load-draft-model: %w", err)
	}
	m.draft = draft

	// Initialize per-session draft KV state stores once we know MTP is
	// enabled. The stores externalize the MTP draft seq state alongside
	// the target's kvState during IMC cache build, so cache hits on
	// later requests restore both seqs in lock-step and MTP can keep
	// running. Non-MTP / no-draft / non-IMC paths leave draftKVState
	// nil, which the snapshot/restore code paths skip.
	if m.cfg.IncrementalCache() && m.draft != nil && m.draft.mtp {
		for _, sess := range m.imcSessions {
			store, err := newSessionStore(m.cfg)
			if err != nil {
				m.batch.stop(ctx)
				m.batch.freeBatch()
				llama.Free(lctx)
				return fmt.Errorf("init-generation-runtime: draft session-store: %w", err)
			}
			sess.draftKVState = store
		}
	}

	return nil
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
	// HEAP-CORRUPTION WORKAROUND: do NOT call SamplerChainDefaultParams
	// (yzma FFI return-type mismatch overruns Go heap by 7 bytes). See
	// detailed comment in toSampler in params.go.
	// TODO: fix yzma's ffiSamplerChainParams type registration upstream.
	sampler := llama.SamplerChainInit(llama.SamplerChainParams{NoPerf: 1})
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
	// HEAP-CORRUPTION WORKAROUND: do NOT call SamplerChainDefaultParams
	// (yzma FFI return-type mismatch overruns Go heap by 7 bytes). See
	// detailed comment in toSampler in params.go.
	// TODO: fix yzma's ffiSamplerChainParams type registration upstream.
	chain := llama.SamplerChainInit(llama.SamplerChainParams{NoPerf: 1})

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

// retrieveTemplate resolves the Jinja chat template for a model. The
// resolution order, from highest to lowest priority:
//
//  1. cfg.JinjaFile — explicit "template:" entry in model_config.yaml.
//  2. <jinjaDir>/<modelID>.jinja — exact match on the model id (e.g.
//     "Qwopus3.5-4B-Coder.Q8_0.jinja"), letting operators target a
//     specific quant.
//  3. <jinjaDir>/<stripQuantSuffix(modelID)>.jinja — match against the
//     base model name with any trailing quant tag removed (e.g.
//     "Qwopus3.5-4B-Coder.jinja"), so one file covers every quant of
//     the same model.
//  4. The GGUF-embedded "tokenizer.chat_template" — original fallback.
//
// Auto-discovery (steps 2-3) lets users drop a .jinja file into
// <basePath>/jinja/ and have it applied without editing model_config.yaml.
func retrieveTemplate(cfg Config, modelID string, mdl llama.Model) (Template, error) {
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

	if tmpl, ok := autoDiscoverTemplate(modelID); ok {
		return tmpl, nil
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

// autoDiscoverTemplate searches the per-user jinja directory for a template
// matching the model id. It tries the full id first, then the id with any
// trailing quant suffix stripped. Returns (Template{}, false) when nothing
// matches or the model id is empty.
func autoDiscoverTemplate(modelID string) (Template, bool) {
	if modelID == "" {
		return Template{}, false
	}

	jinjaDir := defaults.JinjaDir("")

	candidates := []string{modelID}
	if stripped := stripQuantSuffix(modelID); stripped != "" && stripped != modelID {
		candidates = append(candidates, stripped)
	}

	for _, name := range candidates {
		filePath := filepath.Join(jinjaDir, name+".jinja")

		data, err := readJinjaTemplate(filePath)
		if err != nil || data == "" {
			continue
		}

		return Template{
			FileName: filePath,
			Script:   data,
		}, true
	}

	return Template{}, false
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
		// MTP-specific batches; zero values are safe no-ops if the draft
		// was a separate-GGUF (mtp == false) and these were never alloc'd.
		// llama_batch_free unconditionally calls free() on Batch.Embd —
		// our MTP Embd pointers reference Go-owned slices (pinned via
		// runtime.Pinner), so detach them before BatchFree and unpin
		// after so the Go GC can reclaim the backing arrays.
		if m.draft.mtp {
			m.draft.draftBatchMTP.Embd = nil
			m.draft.mirrorBatchMTP.Embd = nil
			llama.BatchFree(m.draft.draftBatchMTP)
			llama.BatchFree(m.draft.mirrorBatchMTP)
			m.draft.draftEmbdPin.Unpin()
			m.draft.mirrorEmbdPin.Unpin()
		}
		llama.Free(m.draft.lctx)
		// MTP shares the target's llama_model — the target's Unload path
		// owns its lifetime. Skip ModelFree to avoid double-free.
		if !m.draft.mtp {
			llama.ModelFree(m.draft.model)
		}
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

	// Release per-session SessionStore resources (e.g. on-disk files).
	// The RAM impl is a no-op; the disk impl removes its file. Errors
	// are logged and otherwise ignored — the model is going away.
	for i, sess := range m.imcSessions {
		if sess == nil {
			continue
		}
		if sess.kvState != nil {
			if err := sess.kvState.Close(); err != nil {
				m.log(ctx, "unload", "status", "session-store-close-failed", "session", i, "err", err.Error())
			}
		}
		if sess.draftKVState != nil {
			if err := sess.draftKVState.Close(); err != nil {
				m.log(ctx, "unload", "status", "draft-session-store-close-failed", "session", i, "err", err.Error())
			}
		}
	}

	// Free the long-lived metadata mtmd context before the model. mtmd
	// holds references into the loaded llama model, so the order matters.
	// Per-request slot mtmd contexts are freed in freeSlotResources during
	// stop()/drainSlots; by the time we get here they are all gone.
	if m.mtmdMetaCtx != 0 {
		mtmd.Free(m.mtmdMetaCtx)
		m.mtmdMetaCtx = 0
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
	// When a draft model is configured, always emit draft metrics so the
	// log schema stays stable for scrapers/dashboards even when
	// speculation was disabled mid-request (collapsed acceptance EMA).
	// Models without a draft model omit the fields entirely.
	if m.draft != nil {
		args = append(args, "draft_tokens", usage.DraftTokens, "draft_accepted_tokens", usage.DraftAcceptedTokens, "acceptance_rate", fmt.Sprintf("%.2f", usage.DraftAcceptanceRate), "draft_coverage", fmt.Sprintf("%.2f", usage.DraftCoverage))
		if usage.DraftDisableReason != "" {
			args = append(args, "draft_disable_reason", usage.DraftDisableReason)
		}
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

// calculateVRAMDiag computes the predicted VRAM and KV-cache slot memory
// for a loaded model. The third return value is a short diagnostic
// describing which branch produced the result, intended for inclusion in
// the calling log line so unexpected zero-KV results can be traced.
//
// The whole calculation — file size, tensor parsing, MoE weight
// breakdown, KV cache, compute buffer — is delegated to
// sdk/kronk/vram.FromFiles so this function and the resman planner's
// tools/models.CalculateVRAM go through the exact same orchestrator.
// Reading metadata directly from the GGUF (rather than from the
// llama.cpp-populated ModelInfo.Metadata map) is what fixes hybrid
// architectures like gemma3/gemma4 whose attention.head_count_kv is
// stored as a per-layer ARRAY: llama.cpp's gguf_kv_to_str silently
// drops ARRAY values, so any path that reads from ModelInfo.Metadata
// will lose them. The SDK's own GGUF parser preserves them.
func calculateVRAMDiag(cfg Config, mi ModelInfo) (vramTotal int64, slotMemory int64, diag string) {
	if len(cfg.ModelFiles) == 0 {
		return int64(mi.Size), 0, "missing model files"
	}

	vramCfg := vram.Config{
		ContextWindow:     int64(cfg.ContextWindow()),
		BytesPerElement:   int64(gguf.MaxBytesPerElement(int32(cfg.CacheTypeK), int32(cfg.CacheTypeV))),
		Slots:             int64(max(cfg.NSeqMax(), 1)),
		ExpertLayersOnGPU: cfg.ExpertLayersOnGPU(),
	}

	result, err := vram.FromFiles(cfg.ModelFiles, vramCfg)
	if err != nil {
		return int64(mi.Size), 0, err.Error()
	}

	diag = fmt.Sprintf("ok arch[%s] block_count[%d] head_count_kv[%d] key_length[%d] value_length[%d] bytes_per_element[%d] context_window[%d] nseq[%d] swa_window[%d] swa_layers[%d]",
		mi.Metadata["general.architecture"],
		result.Input.BlockCount, result.Input.HeadCountKV,
		result.Input.KeyLength, result.Input.ValueLength,
		result.Input.BytesPerElement, result.Input.ContextWindow,
		result.Input.Slots, result.Input.SlidingWindow,
		result.Input.SlidingWindowLayers,
	)

	return result.TotalVRAM, result.SlotMemory, diag
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

// gpuDeviceCount reports how many GPU devices the model will load across. When
// the config pins an explicit device list, only its GPU entries are counted;
// otherwise llama.cpp uses every available device, so all enumerated GPUs are
// counted. It is used to pick the device-count aware split-mode default.
func gpuDeviceCount(cfg *Config) int {
	if len(cfg.Devices) > 0 {
		n := 0
		for _, name := range cfg.Devices {
			if !strings.EqualFold(strings.TrimSpace(name), "CPU") {
				n++
			}
		}
		return n
	}

	return devices.List(
		devices.WithIncludeCPU(false),
		devices.WithIncludeUnknown(false),
		devices.WithIncludeMemory(false),
	).GPUCount
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

// humanBytes formats a byte count using decimal (SI) units. Mirrors the
// helper in sdk/pool to keep log output consistent.
func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}

	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}

	suffixes := []string{"KB", "MB", "GB", "TB", "PB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}

	return fmt.Sprintf("%.1f%s", float64(n)/float64(div), suffixes[exp])
}

// mtmdContextParams returns the mtmd context parameters to use for the given
// model configuration.
func mtmdContextParams(cfg Config) mtmd.ContextParamsType {
	params := mtmd.ContextParamsDefault()

	if cfg.PtrProjOnCPU != nil {
		params.UseGPU = !*cfg.PtrProjOnCPU
	}

	return params
}

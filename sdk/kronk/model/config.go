package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

/*
Workload							NBatch		NUBatch		Rationale
Interactive chat (single user)		512–1024	512			Low latency; small batches
Long prompts/RAG					2048–4096	512–1024	Faster prompt ingestion
Batch inference (multiple prompts)	2048–4096	512			Higher throughput
Low VRAM (<8GB)						512			256–512		Avoid OOM
High VRAM (24GB+)					4096+		1024+		Maximize parallelism

Key principles:
- NUBatch ≤ NBatch always (you already enforce this at line 139)
- NUBatch primarily affects prompt processing speed; keep it ≤512 for stability on most consumer GPUs
- NBatch closer to ContextWindow improves throughput but uses more VRAM
- Powers of 2 are slightly more efficient on most hardware
*/

const (
	defContextWindow    = 8 * 1024
	defNBatch           = 2 * 1024
	defNUBatch          = 512
	defNUBatchVision    = 2 * 1024
	defMinCacheTokens   = 100
	defThreadZero       = 0
	defNSeqMax          = 1
	defNDraft           = 5
	defCacheSlotTimeout = 30
)

// Logger provides a function for logging messages from different APIs.
type Logger func(ctx context.Context, msg string, args ...any)

// =============================================================================

// Config represents model level configuration. These values if configured
// incorrectly can cause the system to panic. The defaults are used when these
// values are set to 0.
//
// CacheMinTokens sets the minimum token count required before caching. Messages
// shorter than this threshold are not cached, as the overhead of cache management
// may outweigh the prefill savings. When set to 0, defaults to 100 tokens.
//
// CacheSlotTimeout sets the maximum number of seconds used for two IMC timeout
// scenarios:
//
//  1. Wait for slot availability: When all IMC slots have cache builds
//     in-flight (pending), incoming requests wait up to this duration for a
//     slot to become available before returning a "server busy" error.
//
//  2. Slot preemption: When a request's target slot is busy generating,
//     the request is deferred. If the deferred job waits longer than this
//     duration (measured from batch queue entry, not HTTP arrival), the
//     busy slot is preempted so the waiting request can proceed.
//
// When set to 0, defaults to 30 seconds.
//
// CacheTypeK is the data type for the K (key) cache. This controls the precision
// of the key vectors in the KV cache. Lower precision types (like Q8_0 or Q4_0)
// reduce memory usage but may slightly affect quality. When left as the zero
// value (GGMLTypeAuto), the default llama.cpp value is used.
//
// CacheTypeV is the data type for the V (value) cache. This controls the precision
// of the value vectors in the KV cache. When left as the zero value (GGMLTypeAuto),
// the default llama.cpp value is used.
//
// ContextWindow (often referred to as context length) is the maximum number of
// tokens that a large language model can process and consider at one time when
// generating a response. It defines the model's effective "memory" for a single
// conversation or text generation task.
// When set to 0, the default value is 4096.
//
// DefaultParams contains the default sampling parameters for requests.
//
// Device is the device to use for the model. If not set, the default device
// will be used. To see what devices are available, run the following command
// which will be found where you installed llama.cpp.
// $ llama-bench --list-devices
//
// FlashAttention controls Flash Attention mode. Flash Attention reduces memory
// usage and speeds up attention computation, especially for large context windows.
// When left as zero value, FlashAttentionEnabled is used (default on).
// Set to FlashAttentionDisabled to disable, or FlashAttentionAuto to let llama.cpp decide.
//
// IgnoreIntegrityCheck is a boolean that determines if the system should ignore
// a model integrity check before trying to use it.
//
// IncrementalCache enables Incremental Message Caching (IMC) for agentic
// workflows. It caches all messages except the last one (which triggers
// generation) and extends the cache incrementally on each turn. This is ideal
// for agents like Cline or OpenCode where conversations grow monotonically.
// The cache is rebuilt from scratch when the message prefix changes (new thread).
//
// InsecureLogging enables logging of potentially sensitive data such as message
// content. This should only be enabled for debugging purposes in non-production
// environments.
//
// JinjaFile is the path to the jinja file. This is not required and can be
// used if you want to override the templated provided by the model metadata.
//
// Log is the logger to use for model operations.
//
// ModelFiles is the path to the model files. This is mandatory to provide.
//
// NBatch is the logical batch size or the maximum number of tokens that can be
// in a single forward pass through the model at any given time.  It defines
// the maximum capacity of the processing batch. If you are processing a very
// long prompt or multiple prompts simultaneously, the total number of tokens
// processed in one go will not exceed NBatch. Increasing n_batch can improve
// performance (throughput) if your hardware can handle it, as it better
// utilizes parallel computation. However, a very high n_batch can lead to
// out-of-memory errors on systems with limited VRAM.
// When set to 0, the default value is 2048.
//
// NGpuLayers is the number of model layers to offload to the GPU. When set to 0,
// all layers are offloaded (default). Set to -1 to keep all layers on CPU. Any
// positive value specifies the exact number of layers to offload.
//
// NSeqMax controls concurrency behavior based on model type. For text inference
// models (including vision/audio), it sets the maximum number of sequences
// processed in parallel within the batch engine. For embedding and reranking
// models, it sets the number of contexts in the internal pool for parallel
// request processing. When set to 0, a default of 1 is used.
//
// NThreads is the number of threads to use for generation. When set to 0, the
// default llama.cpp value is used.
//
// NThreadsBatch is the number of threads to use for batch processing. When set
// to 0, the default llama.cpp value is used.
//
// NUBatch is the physical batch size or the maximum number of tokens processed
// together during the initial prompt processing phase (also called "prompt
// ingestion") to populate the KV cache. It specifically optimizes the initial
// loading of prompt tokens into the KV cache. If a prompt is longer than
// NUBatch, it will be broken down and processed in chunks of n_ubatch tokens
// sequentially. This parameter is crucial for tuning performance on specific
// hardware (especially GPUs) because different values might yield better prompt
// processing times depending on the memory architecture.
// When set to 0, the default value is 512.
//
// OffloadKQV controls whether the KV cache is offloaded to the GPU. When nil or
// true, the KV cache is stored on the GPU (default behavior). Set to false to
// keep the KV cache on the CPU, which reduces VRAM usage but may slow inference.
//
// OpOffload controls whether host tensor operations are offloaded to the device
// (GPU). When nil or true, operations are offloaded (default behavior). Set to
// false to keep operations on the CPU.
//
// ProjFile is the path to the projection files. This is mandatory for media
// based models like vision and audio.
//
// RopeFreqBase overrides the RoPE base frequency. When nil, uses model default.
// Common values: 10000 (Llama), 1000000 (Qwen3).
//
// RopeFreqScale overrides the RoPE frequency scaling factor. When nil, uses
// model default or auto-calculates based on context extension ratio.
//
// RopeScaling controls the RoPE scaling method for extended context support.
// Set to RopeScalingYaRN to enable YaRN scaling for models like Qwen3 that
// support extended context (e.g., 32k training → 131k with YaRN).
//
// SplitMode controls how the model is split across multiple GPUs:
//   - SplitModeNone (0): single GPU
//   - SplitModeLayer (1): split layers and KV across GPUs
//   - SplitModeRow (2): split layers and KV across GPUs with tensor parallelism
//     (recommended for MoE models like Qwen3-MoE, Mixtral, DeepSeek)
//
// When not set, defaults to SplitModeRow for optimal MoE performance.
//
// SystemPromptCache enables caching of system prompt KV state. When enabled,
// the first message with role="system" is cached. The system prompt is evaluated
// once and its KV state is externalized to a byte buffer in RAM. On subsequent
// requests with the same system prompt, the KV state is restored into the
// slot's sequence via StateSeqSetData, avoiding redundant prefill computation.
// The cache is automatically invalidated and re-evaluated when the system
// prompt changes. No dedicated KV cache sequence is permanently occupied.
//
// SystemPromptCache and IncrementalCache are mutually exclusive. IncrementalCache
// includes the system prompt in its cached prefix, so enabling both is redundant
// and will return a validation error.
//
// UseDirectIO enables direct I/O for model loading.
//
// YarnAttnFactor sets the YaRN attention magnitude scaling factor. When nil,
// uses default of 1.0.
//
// YarnBetaFast sets the YaRN low correction dimension. When nil, uses default
// of 32.0.
//
// YarnBetaSlow sets the YaRN high correction dimension. When nil, uses default
// of 1.0.
//
// YarnExtFactor sets the YaRN extrapolation mix factor. When nil, auto-calculated
// from context scaling ratio. Set to 0 to disable extrapolation.
//
// YarnOrigCtx sets the original training context size for YaRN scaling. When nil
// or 0, uses the model's native training context length from metadata.

// DraftModelConfig configures a draft model for speculative decoding. A smaller,
// faster model generates candidate tokens that the target model verifies in a
// single forward pass. This can improve generation throughput when the draft
// model's predictions frequently match the target's.
//
// Requirements:
//   - Draft and target models must share the same vocabulary (same tokenizer)
//   - NSeqMax must be 1 (single-slot mode)
//   - Draft model should be significantly smaller than the target (e.g., 0.6B draft for 8B target)
type DraftModelConfig struct {
	ModelFiles []string // Path to the draft model GGUF file(s)
	NDraft     int      // Number of tokens to draft per step (default 5)
	NGpuLayers *int     // GPU layers for draft model (nil = all layers on GPU)
	Device     string   // Device for draft model (e.g., "GPU1") to pin to a specific GPU
}

type Config struct {
	CacheMinTokens       int
	CacheSlotTimeout     int
	CacheTypeK           GGMLType
	CacheTypeV           GGMLType
	ContextWindow        int
	DefaultParams        Params
	Device               string
	FlashAttention       FlashAttentionType
	IgnoreIntegrityCheck bool
	IncrementalCache     bool
	InsecureLogging      bool
	JinjaFile            string
	Log                  Logger
	ModelFiles           []string
	NBatch               int
	NGpuLayers           *int
	NSeqMax              int
	NThreads             int
	NThreadsBatch        int
	NUBatch              int
	OffloadKQV           *bool
	OpOffload            *bool
	ProjFile             string
	RopeFreqBase         *float32
	RopeFreqScale        *float32
	RopeScaling          RopeScalingType
	SplitMode            SplitMode
	SystemPromptCache    bool
	UseDirectIO          bool
	YarnAttnFactor       *float32
	YarnBetaFast         *float32
	YarnBetaSlow         *float32
	YarnExtFactor        *float32
	YarnOrigCtx          *int
	DraftModel           *DraftModelConfig
}

func (cfg Config) String() string {
	formatBoolPtr := func(p *bool) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprintf("%t", *p)
	}

	formatFloat32Ptr := func(p *float32) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprintf("%g", *p)
	}

	formatIntPtr := func(p *int) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprintf("%d", *p)
	}

	return fmt.Sprintf("\nCacheMinTokens[%d]\nCacheSlotTimeout[%d]\nCacheTypeK[%d]\nCacheTypeV[%d]\nContextWindow[%d]\nDevice[%s]\nFlashAttention[%d]\nIgnoreIntegrityCheck[%t]\nIncrementalCache[%t]\nInsecureLogging[%t]\nJinjaFile[%s]\nModelFiles[%v]\nNBatch[%d]\nNGpuLayers[%s]\nNSeqMax[%d]\nNThreads[%d]\nNThreadsBatch[%d]\nNUBatch[%d]\nOffloadKQV[%s]\nOpOffload[%s]\nProjFile[%s]\nRopeFreqBase[%s]\nRopeFreqScale[%s]\nRopeScaling[%s]\nSplitMode[%d]\nSystemPromptCache[%t]\nUseDirectIO[%t]\nYarnAttnFactor[%s]\nYarnBetaFast[%s]\nYarnBetaSlow[%s]\nYarnExtFactor[%s]\nYarnOrigCtx[%s]\nDraftModel[%v]\n",
		cfg.CacheMinTokens, cfg.CacheSlotTimeout, cfg.CacheTypeK, cfg.CacheTypeV, cfg.ContextWindow, cfg.Device, cfg.FlashAttention, cfg.IgnoreIntegrityCheck,
		cfg.IncrementalCache, cfg.InsecureLogging, cfg.JinjaFile, cfg.ModelFiles, cfg.NBatch,
		formatIntPtr(cfg.NGpuLayers), cfg.NSeqMax, cfg.NThreads, cfg.NThreadsBatch, cfg.NUBatch,
		formatBoolPtr(cfg.OffloadKQV), formatBoolPtr(cfg.OpOffload), cfg.ProjFile,
		formatFloat32Ptr(cfg.RopeFreqBase), formatFloat32Ptr(cfg.RopeFreqScale), cfg.RopeScaling, cfg.SplitMode,
		cfg.SystemPromptCache, cfg.UseDirectIO, formatFloat32Ptr(cfg.YarnAttnFactor),
		formatFloat32Ptr(cfg.YarnBetaFast), formatFloat32Ptr(cfg.YarnBetaSlow), formatFloat32Ptr(cfg.YarnExtFactor), formatIntPtr(cfg.YarnOrigCtx), cfg.DraftModel)
}

func validateConfig(ctx context.Context, cfg Config, log Logger) error {
	if len(cfg.ModelFiles) == 0 {
		return fmt.Errorf("validate-config: model file is required")
	}

	if cfg.SystemPromptCache && cfg.IncrementalCache {
		return fmt.Errorf("validate-config: cannot enable both SystemPromptCache and IncrementalCache; use IncrementalCache alone (it includes the system prompt)")
	}

	if cfg.DraftModel != nil {
		if len(cfg.DraftModel.ModelFiles) == 0 {
			return fmt.Errorf("validate-config: draft model requires model files")
		}
		if cfg.NSeqMax > 1 {
			return fmt.Errorf("validate-config: speculative decoding requires NSeqMax=1, got %d", cfg.NSeqMax)
		}
		if !cfg.IgnoreIntegrityCheck {
			for _, modelFile := range cfg.DraftModel.ModelFiles {
				log(ctx, "validate-config", "draft-model-file", modelFile)
				if err := CheckModel(modelFile, true); err != nil {
					return fmt.Errorf("validate-config: draft model: %w", err)
				}
			}
		}
	}

	if !cfg.IgnoreIntegrityCheck {
		for _, modelFile := range cfg.ModelFiles {
			log(ctx, "validate-config", "model-file", modelFile)

			if err := CheckModel(modelFile, true); err != nil {
				return fmt.Errorf("validate-config: %w", err)
			}
		}

		if cfg.ProjFile != "" {
			log(ctx, "validate-config", "model-file", cfg.ProjFile)

			if err := CheckModel(cfg.ProjFile, true); err != nil {
				return fmt.Errorf("validate-config: prog-file[%s]: %w", cfg.ProjFile, err)
			}
		}
	}

	return nil
}

func adjustConfig(cfg Config, model llama.Model) Config {
	cfg = adjustContextWindow(cfg, model)

	if cfg.NBatch <= 0 {
		cfg.NBatch = defNBatch
	}

	if cfg.NUBatch <= 0 {
		// Vision models require n_ubatch >= n_tokens for the image encoder's
		// non-causal attention. Use a larger default when ProjFile is set.
		switch cfg.ProjFile != "" {
		case true:
			cfg.NUBatch = defNUBatchVision
		case false:
			cfg.NUBatch = defNUBatch
		}
	}

	if cfg.NThreads < 0 {
		cfg.NThreads = defThreadZero
	}

	if cfg.NThreadsBatch < 0 {
		cfg.NThreadsBatch = defThreadZero
	}

	// NBatch is generally greater than or equal to NUBatch. The entire
	// NUBatch of tokens must fit into a physical batch for processing.
	if cfg.NUBatch > cfg.NBatch {
		cfg.NUBatch = cfg.NBatch
	}

	// This value must be 1 to properly configure the batch engine.
	if cfg.NSeqMax <= 0 {
		cfg.NSeqMax = defNSeqMax
	}

	// Default minimum tokens for caching.
	if (cfg.SystemPromptCache || cfg.IncrementalCache) && cfg.CacheMinTokens <= 0 {
		cfg.CacheMinTokens = defMinCacheTokens
	}

	// Default slot wait timeout for IMC.
	if cfg.IncrementalCache && cfg.CacheSlotTimeout <= 0 {
		cfg.CacheSlotTimeout = defCacheSlotTimeout
	}

	if cfg.DraftModel != nil && cfg.DraftModel.NDraft <= 0 {
		cfg.DraftModel.NDraft = defNDraft
	}

	return cfg
}

func applyCatalogConfig(user Config, cat Config) Config {
	if user.Device == "" {
		user.Device = cat.Device
	}
	if user.ContextWindow == 0 {
		user.ContextWindow = cat.ContextWindow
	}
	if user.NBatch == 0 {
		user.NBatch = cat.NBatch
	}
	if user.NUBatch == 0 {
		user.NUBatch = cat.NUBatch
	}
	if user.NThreads == 0 {
		user.NThreads = cat.NThreads
	}
	if user.NThreadsBatch == 0 {
		user.NThreadsBatch = cat.NThreadsBatch
	}
	if user.CacheTypeK == GGMLTypeAuto {
		user.CacheTypeK = cat.CacheTypeK
	}
	if user.CacheTypeV == GGMLTypeAuto {
		user.CacheTypeV = cat.CacheTypeV
	}
	if user.FlashAttention == FlashAttentionEnabled && cat.FlashAttention != FlashAttentionEnabled {
		user.FlashAttention = cat.FlashAttention
	}
	if !user.UseDirectIO {
		user.UseDirectIO = cat.UseDirectIO
	}
	if !user.IgnoreIntegrityCheck {
		user.IgnoreIntegrityCheck = cat.IgnoreIntegrityCheck
	}
	if user.NSeqMax == 0 {
		user.NSeqMax = cat.NSeqMax
	}
	if user.OffloadKQV == nil {
		user.OffloadKQV = cat.OffloadKQV
	}
	if user.OpOffload == nil {
		user.OpOffload = cat.OpOffload
	}
	if user.NGpuLayers == nil {
		user.NGpuLayers = cat.NGpuLayers
	}
	if user.SplitMode == SplitModeNone {
		user.SplitMode = cat.SplitMode
	}
	if !user.SystemPromptCache {
		user.SystemPromptCache = cat.SystemPromptCache
	}
	if !user.IncrementalCache {
		user.IncrementalCache = cat.IncrementalCache
	}
	if user.CacheMinTokens == 0 {
		user.CacheMinTokens = cat.CacheMinTokens
	}
	if user.CacheSlotTimeout == 0 {
		user.CacheSlotTimeout = cat.CacheSlotTimeout
	}
	if !user.InsecureLogging {
		user.InsecureLogging = cat.InsecureLogging
	}
	if user.RopeScaling == RopeScalingNone {
		user.RopeScaling = cat.RopeScaling
	}
	if user.RopeFreqBase == nil {
		user.RopeFreqBase = cat.RopeFreqBase
	}
	if user.RopeFreqScale == nil {
		user.RopeFreqScale = cat.RopeFreqScale
	}
	if user.YarnExtFactor == nil {
		user.YarnExtFactor = cat.YarnExtFactor
	}
	if user.YarnAttnFactor == nil {
		user.YarnAttnFactor = cat.YarnAttnFactor
	}
	if user.YarnBetaFast == nil {
		user.YarnBetaFast = cat.YarnBetaFast
	}
	if user.YarnBetaSlow == nil {
		user.YarnBetaSlow = cat.YarnBetaSlow
	}
	if user.YarnOrigCtx == nil {
		user.YarnOrigCtx = cat.YarnOrigCtx
	}
	if user.DraftModel == nil {
		user.DraftModel = cat.DraftModel
	}

	user.DefaultParams = applyCatalogSamplingParams(user.DefaultParams, cat.DefaultParams)

	return user
}

func applyCatalogSamplingParams(user Params, cat Params) Params {
	if user.Temperature == 0 {
		user.Temperature = cat.Temperature
	}
	if user.TopK == 0 {
		user.TopK = cat.TopK
	}
	if user.TopP == 0 {
		user.TopP = cat.TopP
	}
	if user.MinP == 0 {
		user.MinP = cat.MinP
	}
	if user.MaxTokens == 0 {
		user.MaxTokens = cat.MaxTokens
	}
	if user.RepeatPenalty == 0 {
		user.RepeatPenalty = cat.RepeatPenalty
	}
	if user.RepeatLastN == 0 {
		user.RepeatLastN = cat.RepeatLastN
	}
	if user.FrequencyPenalty == 0 {
		user.FrequencyPenalty = cat.FrequencyPenalty
	}
	if user.PresencePenalty == 0 {
		user.PresencePenalty = cat.PresencePenalty
	}
	if user.DryMultiplier == 0 {
		user.DryMultiplier = cat.DryMultiplier
	}
	if user.DryBase == 0 {
		user.DryBase = cat.DryBase
	}
	if user.DryAllowedLen == 0 {
		user.DryAllowedLen = cat.DryAllowedLen
	}
	if user.DryPenaltyLast == 0 {
		user.DryPenaltyLast = cat.DryPenaltyLast
	}
	if user.XtcProbability == 0 {
		user.XtcProbability = cat.XtcProbability
	}
	if user.XtcThreshold == 0 {
		user.XtcThreshold = cat.XtcThreshold
	}
	if user.XtcMinKeep == 0 {
		user.XtcMinKeep = cat.XtcMinKeep
	}
	if user.Thinking == "" {
		user.Thinking = cat.Thinking
	}
	if user.ReasoningEffort == "" {
		user.ReasoningEffort = cat.ReasoningEffort
	}
	if user.Grammar == "" {
		user.Grammar = cat.Grammar
	}

	return user
}

func adjustContextWindow(cfg Config, model llama.Model) Config {
	modelCW := defContextWindow
	v, found := searchModelMeta(model, "adjust-context-window: context_length")
	if found {
		ctxLen, err := strconv.Atoi(v)
		if err == nil {
			modelCW = ctxLen
		}
	}

	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = modelCW
	}

	return cfg
}

func modelCtxParams(cfg Config, mi ModelInfo) llama.ContextParams {
	ctxParams := llama.ContextDefaultParams()

	if mi.IsEmbedModel || mi.IsRerankModel {
		ctxParams.Embeddings = 1
	}

	if mi.IsRerankModel {
		ctxParams.PoolingType = llama.PoolingTypeRank
	}

	// SPC externalizes the KV state to a byte buffer after initial decode,
	// so it does not need an extra sequence. IMC uses dedicated slot/seq
	// binding — no separate cache sequences either.
	nSeqMax := max(cfg.NSeqMax, 1)
	totalSeqs := nSeqMax

	if cfg.ContextWindow > 0 {
		ctxParams.NBatch = uint32(cfg.NBatch)
		ctxParams.NUbatch = uint32(cfg.NUBatch)
		ctxParams.NThreads = int32(cfg.NThreads)
		ctxParams.NThreadsBatch = int32(cfg.NThreadsBatch)

		ctxParams.NCtx = uint32(cfg.ContextWindow)
	}

	if cfg.CacheTypeK != GGMLTypeAuto {
		ctxParams.TypeK = cfg.CacheTypeK.ToYZMAType()
	}

	if cfg.CacheTypeV != GGMLTypeAuto {
		ctxParams.TypeV = cfg.CacheTypeV.ToYZMAType()
	}

	switch {
	case mi.IsHybridModel:
		ctxParams.FlashAttentionType = llama.FlashAttentionTypeDisabled
	case cfg.FlashAttention == FlashAttentionDisabled:
		ctxParams.FlashAttentionType = llama.FlashAttentionTypeDisabled
	case cfg.FlashAttention == FlashAttentionAuto:
		ctxParams.FlashAttentionType = llama.FlashAttentionTypeAuto
	default:
		ctxParams.FlashAttentionType = llama.FlashAttentionTypeEnabled
	}

	ctxParams.NSeqMax = uint32(totalSeqs)

	// Offload KQV cache to CPU.
	// llama.cpp has this as default set to true
	ctxParams.Offload_kqv = 1
	if cfg.OffloadKQV != nil &&
		!*cfg.OffloadKQV {
		ctxParams.Offload_kqv = 0
	}

	// Offload host tensor operations to device.
	// llama.cpp has this as default set to true
	ctxParams.OpOffload = 1
	if cfg.OpOffload != nil && !*cfg.OpOffload {
		ctxParams.OpOffload = 0
	}

	// YaRN RoPE scaling for extended context windows.
	// Only set parameters when explicitly configured (non-nil).
	// llama.cpp uses special values (0, -1) to mean "use model defaults".
	if cfg.RopeScaling != RopeScalingNone {
		ctxParams.RopeScalingType = cfg.RopeScaling.ToYZMAType()
	}
	if cfg.RopeFreqBase != nil {
		ctxParams.RopeFreqBase = *cfg.RopeFreqBase
	}
	if cfg.RopeFreqScale != nil {
		ctxParams.RopeFreqScale = *cfg.RopeFreqScale
	}
	if cfg.YarnExtFactor != nil {
		ctxParams.YarnExtFactor = *cfg.YarnExtFactor
	}
	if cfg.YarnAttnFactor != nil {
		ctxParams.YarnAttnFactor = *cfg.YarnAttnFactor
	}
	if cfg.YarnBetaFast != nil {
		ctxParams.YarnBetaFast = *cfg.YarnBetaFast
	}
	if cfg.YarnBetaSlow != nil {
		ctxParams.YarnBetaSlow = *cfg.YarnBetaSlow
	}
	if cfg.YarnOrigCtx != nil {
		ctxParams.YarnOrigCtx = uint32(*cfg.YarnOrigCtx)
	}

	return ctxParams
}

func searchModelMeta(model llama.Model, find string) (string, bool) {
	count := llama.ModelMetaCount(model)

	for i := range count {
		key, ok := llama.ModelMetaKeyByIndex(model, i)
		if !ok {
			continue
		}

		if strings.Contains(key, find) {
			value, ok := llama.ModelMetaValStrByIndex(model, i)
			if !ok {
				continue
			}

			return value, true
		}
	}

	return "", false
}

// =============================================================================

// GGMLType represents a ggml data type for the KV cache.
// These values correspond to the ggml_type enum in llama.cpp.
type GGMLType int32

const (
	GGMLTypeAuto GGMLType = 0  // Use default from llama.cpp (zero value)
	GGMLTypeF32  GGMLType = 50 // 32-bit floating point
	GGMLTypeF16  GGMLType = 1  // 16-bit floating point
	GGMLTypeQ4_0 GGMLType = 2  // 4-bit quantization (type 0)
	GGMLTypeQ4_1 GGMLType = 3  // 4-bit quantization (type 1)
	GGMLTypeQ5_0 GGMLType = 6  // 5-bit quantization (type 0)
	GGMLTypeQ5_1 GGMLType = 7  // 5-bit quantization (type 1)
	GGMLTypeQ8_0 GGMLType = 8  // 8-bit quantization (type 0)
	GGMLTypeBF16 GGMLType = 30 // Brain floating point 16-bit
)

// String returns the string representation of a GGMLType.
func (t GGMLType) String() string {
	switch t {
	case GGMLTypeF32:
		return "f32"

	case GGMLTypeF16:
		return "f16"

	case GGMLTypeQ4_0:
		return "q4_0"

	case GGMLTypeQ4_1:
		return "q4_1"

	case GGMLTypeQ5_0:
		return "q5_0"

	case GGMLTypeQ5_1:
		return "q5_1"

	case GGMLTypeQ8_0:
		return "q8_0"

	case GGMLTypeBF16:
		return "bf16"

	case GGMLTypeAuto:
		return "auto"

	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func (t GGMLType) ToYZMAType() llama.GGMLType {
	switch t {
	case GGMLTypeAuto:
		return llama.GGMLType(-1)
	case GGMLTypeF32:
		return llama.GGMLType(0)
	default:
		return llama.GGMLType(t)
	}
}

func (t GGMLType) MarshalYAML() (any, error) {
	return t.String(), nil
}

func (t GGMLType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *GGMLType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	parsed, err := ParseGGMLType(s)
	if err != nil {
		return err
	}

	*t = parsed

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler to parse string values like "f16".
func (t *GGMLType) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	parsed, err := ParseGGMLType(s)
	if err != nil {
		return err
	}

	*t = parsed

	return nil
}

// ParseGGMLType parses a string into a GGMLType.
// Supported values: "f32", "f16", "q4_0", "q4_1", "q5_0", "q5_1", "q8_0", "bf16", "auto".
func ParseGGMLType(s string) (GGMLType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "f32", "fp32":
		return GGMLTypeF32, nil

	case "f16", "fp16":
		return GGMLTypeF16, nil

	case "q4_0", "q4":
		return GGMLTypeQ4_0, nil

	case "q4_1":
		return GGMLTypeQ4_1, nil

	case "q5_0", "q5":
		return GGMLTypeQ5_0, nil

	case "q5_1":
		return GGMLTypeQ5_1, nil

	case "f8", "q8_0", "q8":
		return GGMLTypeQ8_0, nil

	case "bf16", "bfloat16":
		return GGMLTypeBF16, nil

	case "auto", "":
		return GGMLTypeAuto, nil

	default:
		return GGMLTypeAuto, fmt.Errorf("unknown ggml type: %s", s)
	}
}

// =============================================================================

// FlashAttentionType controls when to enable Flash Attention.
// Flash Attention reduces memory usage and speeds up attention computation,
// especially beneficial for large context windows.
type FlashAttentionType int32

const (
	FlashAttentionEnabled  FlashAttentionType = 0 // Default: enable Flash Attention
	FlashAttentionDisabled FlashAttentionType = 1 // Disable Flash Attention
	FlashAttentionAuto     FlashAttentionType = 2 // Let llama.cpp decide
)

func (t FlashAttentionType) String() string {
	switch t {
	case FlashAttentionEnabled:
		return "enabled"
	case FlashAttentionDisabled:
		return "disabled"
	case FlashAttentionAuto:
		return "auto"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func (t FlashAttentionType) MarshalYAML() (any, error) {
	return t.String(), nil
}

func (t FlashAttentionType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *FlashAttentionType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(s)) {
	case "enabled", "on", "true", "1":
		*t = FlashAttentionEnabled
	case "disabled", "off", "false", "0":
		*t = FlashAttentionDisabled
	case "auto", "":
		*t = FlashAttentionAuto
	default:
		return fmt.Errorf("unknown flash attention type: %s", s)
	}

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler to parse string values.
func (t *FlashAttentionType) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(s)) {
	case "enabled", "on", "true", "1":
		*t = FlashAttentionEnabled

	case "disabled", "off", "false", "0":
		*t = FlashAttentionDisabled

	case "auto", "":
		*t = FlashAttentionAuto

	default:
		return fmt.Errorf("unmarshal-yaml: unknown flash attention type: %s", s)
	}

	return nil
}

// =============================================================================

// SplitMode controls how the model is split across multiple GPUs.
// This is particularly important for Mixture of Experts (MoE) models.
type SplitMode int32

const (
	// SplitModeNone uses a single GPU (default).
	SplitModeNone SplitMode = 0

	// SplitModeLayer splits layers and KV cache across GPUs.
	SplitModeLayer SplitMode = 1

	// SplitModeRow splits layers and KV across GPUs with tensor parallelism.
	// This enables expert-parallel execution for MoE models (Qwen3-MoE, Mixtral, DeepSeek).
	// Equivalent to vLLM's --enable-expert-parallel flag.
	SplitModeRow SplitMode = 2
)

// String returns the string representation of a SplitMode.
func (s SplitMode) String() string {
	switch s {
	case SplitModeNone:
		return "none"

	case SplitModeLayer:
		return "layer"

	case SplitModeRow:
		return "row"

	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// ToYZMAType converts to the yzma/llama.cpp SplitMode type.
func (s SplitMode) ToYZMAType() llama.SplitMode {
	return llama.SplitMode(s)
}

func (s SplitMode) MarshalYAML() (any, error) {
	return s.String(), nil
}

func (s SplitMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *SplitMode) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	parsed, err := ParseSplitMode(str)
	if err != nil {
		return err
	}

	*s = parsed

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler to parse string values.
func (s *SplitMode) UnmarshalYAML(unmarshal func(any) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}

	parsed, err := ParseSplitMode(str)
	if err != nil {
		return err
	}

	*s = parsed

	return nil
}

// ParseSplitMode parses a string into a SplitMode.
// Supported values: "none", "layer", "row", "expert-parallel", "tensor-parallel".
func ParseSplitMode(s string) (SplitMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "single", "0", "":
		return SplitModeNone, nil

	case "layer", "1":
		return SplitModeLayer, nil

	case "row", "tensor", "tensor-parallel", "expert-parallel", "2":
		return SplitModeRow, nil

	default:
		return SplitModeNone, fmt.Errorf("parse-split-mode: unknown split mode: %s (valid: none, layer, row, expert-parallel)", s)
	}
}

// =============================================================================

// RopeScalingType controls RoPE (Rotary Position Embedding) scaling method.
// This enables extended context windows beyond the model's native training length.
// For example, Qwen3 models trained on 32k can support 131k with YaRN scaling.
type RopeScalingType int32

const (
	// RopeScalingNone disables RoPE scaling (use native context length).
	RopeScalingNone RopeScalingType = 0

	// RopeScalingLinear uses linear interpolation scaling.
	// Simple but less effective for large extensions.
	RopeScalingLinear RopeScalingType = 1

	// RopeScalingYaRN uses YaRN (Yet another RoPE extensioN) scaling.
	// Recommended for extending context 2-4x beyond training length.
	// Applies frequency-dependent interpolation with attention scaling.
	RopeScalingYaRN RopeScalingType = 2
)

// String returns the string representation of a RopeScalingType.
func (r RopeScalingType) String() string {
	switch r {
	case RopeScalingNone:
		return "none"

	case RopeScalingLinear:
		return "linear"

	case RopeScalingYaRN:
		return "yarn"

	default:
		return fmt.Sprintf("unknown(%d)", r)
	}
}

// ToYZMAType converts to the yzma/llama.cpp RopeScalingType.
func (r RopeScalingType) ToYZMAType() llama.RopeScalingType {
	return llama.RopeScalingType(r)
}

func (r RopeScalingType) MarshalYAML() (any, error) {
	return r.String(), nil
}

func (r RopeScalingType) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *RopeScalingType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	parsed, err := ParseRopeScalingType(s)
	if err != nil {
		return err
	}

	*r = parsed

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler to parse string values.
func (r *RopeScalingType) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	parsed, err := ParseRopeScalingType(s)
	if err != nil {
		return err
	}

	*r = parsed

	return nil
}

// ParseRopeScalingType parses a string into a RopeScalingType.
// Supported values: "none", "linear", "yarn".
func ParseRopeScalingType(s string) (RopeScalingType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "0", "":
		return RopeScalingNone, nil

	case "linear", "1":
		return RopeScalingLinear, nil

	case "yarn", "2":
		return RopeScalingYaRN, nil

	default:
		return RopeScalingNone, fmt.Errorf("parse-rope-scaling-type: unknown type: %s (valid: none, linear, yarn)", s)
	}
}

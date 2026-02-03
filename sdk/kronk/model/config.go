package model

import (
	"context"
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
	defContextWindow  = 8 * 1024
	defNBatch         = 2 * 1024
	defNUBatch        = 512
	defNUBatchVision  = 2 * 1024
	defMinCacheTokens = 100
	defThreadZero     = 0
	defNSeqMax        = 1
	defMaxCacheSessions = 1
)

// Logger provides a function for logging messages from different APIs.
type Logger func(ctx context.Context, msg string, args ...any)

// =============================================================================

// Config represents model level configuration. These values if configured
// incorrectly can cause the system to panic. The defaults are used when these
// values are set to 0.
//
// ModelInstances is the number of instances of the model to create. Unless
// you have more than 1 GPU, the recommended number of instances is 1.
//
// ModelFiles is the path to the model files. This is mandatory to provide.
//
// ProjFiles is the path to the projection files. This is mandatory for media
// based models like vision and audio.
//
// JinjaFile is the path to the jinja file. This is not required and can be
// used if you want to override the templated provided by the model metadata.
//
// Device is the device to use for the model. If not set, the default device
// will be used. To see what devices are available, run the following command
// which will be found where you installed llama.cpp.
// $ llama-bench --list-devices
//
// ContextWindow (often referred to as context length) is the maximum number of
// tokens that a large language model can process and consider at one time when
// generating a response. It defines the model's effective "memory" for a single
// conversation or text generation task.
// When set to 0, the default value is 4096.
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
// NThreads is the number of threads to use for generation. When set to 0, the
// default llama.cpp value is used.
//
// NThreadsBatch is the number of threads to use for batch processing. When set
// to 0, the default llama.cpp value is used.
//
// CacheTypeK is the data type for the K (key) cache. This controls the precision
// of the key vectors in the KV cache. Lower precision types (like Q8_0 or Q4_0)
// reduce memory usage but may slightly affect quality. When set to GGMLTypeAuto
// or left as zero value, the default llama.cpp value (F16) is used.
//
// CacheTypeV is the data type for the V (value) cache. This controls the precision
// of the value vectors in the KV cache. When set to GGMLTypeAuto or left as zero
// value, the default llama.cpp value (F16) is used.
//
// FlashAttention controls Flash Attention mode. Flash Attention reduces memory
// usage and speeds up attention computation, especially for large context windows.
// When left as zero value, FlashAttentionEnabled is used (default on).
// Set to FlashAttentionDisabled to disable, or FlashAttentionAuto to let llama.cpp decide.
//
// IgnoreIntegrityCheck is a boolean that determines if the system should ignore
// a model integrity check before trying to use it.
//
// NSeqMax controls concurrency behavior based on model type. For text inference
// models, it sets the maximum number of sequences processed in parallel within
// a single model instance (batched inference). For sequential models (embeddings,
// reranking, vision, audio), it creates that many model instances in a pool for
// concurrent request handling. When set to 0, a default of 1 is used.
//
// OffloadKQV controls whether the KV cache is offloaded to the GPU. When nil or
// true, the KV cache is stored on the GPU (default behavior). Set to false to
// keep the KV cache on the CPU, which reduces VRAM usage but may slow inference.
//
// OpOffload controls whether host tensor operations are offloaded to the device
// (GPU). When nil or true, operations are offloaded (default behavior). Set to
// false to keep operations on the CPU.
//
// NGpuLayers is the number of model layers to offload to the GPU. When set to 0,
// all layers are offloaded (default). Set to -1 to keep all layers on CPU. Any
// positive value specifies the exact number of layers to offload.
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
// once and its KV cache is copied to all client sequences on subsequent requests
// with the same system prompt. This avoids redundant prefill computation for
// applications that use a consistent system prompt. The cache is automatically
// invalidated and re-evaluated when the system prompt changes.
//
// IncrementalCache enables Incremental Message Caching (IMC) for agentic
// workflows. It caches all messages except the last one (which triggers
// generation) and extends the cache incrementally on each turn. This is ideal
// for agents like Cline or OpenCode where conversations grow monotonically.
// The cache is rebuilt from scratch when the message prefix changes (new thread).
//
// MaxCacheSessions sets the maximum number of concurrent cache sessions (users).
// Each session gets its own dedicated cache sequence, identified by the cache_id
// parameter in requests. When set to 0, defaults to 1 session. If all sessions
// are in use, new requests without an available slot bypass caching gracefully.
// The input request should have `cache_id` with a unique session/user ID to
// activate caching. Each unique ID gets its own dedicated cache sequence (up to
// max-cache-sessions). If no cache id is passed, the "default" id is used. On a
// multi-user system that will cause problems.
//
// SystemPromptCache and IncrementalCache are mutually exclusive. IncrementalCache
// includes the system prompt in its cached prefix, so enabling both is redundant
// and will return a validation error.
//
// CacheMinTokens sets the minimum token count required before caching. Messages
// shorter than this threshold are not cached, as the overhead of cache management
// may outweigh the prefill savings. When set to 0, defaults to 100 tokens.
//
// InsecureLogging enables logging of potentially sensitive data such as message
// content. This should only be enabled for debugging purposes in non-production
// environments.
//
// RopeScaling controls the RoPE scaling method for extended context support.
// Set to RopeScalingYaRN to enable YaRN scaling for models like Qwen3 that
// support extended context (e.g., 32k training → 131k with YaRN).
//
// RopeFreqBase overrides the RoPE base frequency. When nil, uses model default.
// Common values: 10000 (Llama), 1000000 (Qwen3).
//
// RopeFreqScale overrides the RoPE frequency scaling factor. When nil, uses
// model default or auto-calculates based on context extension ratio.
//
// YarnExtFactor sets the YaRN extrapolation mix factor. When nil, auto-calculated
// from context scaling ratio. Set to 0 to disable extrapolation.
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
// YarnOrigCtx sets the original training context size for YaRN scaling. When nil
// or 0, uses the model's native training context length from metadata.
type Config struct {
	Log                  Logger
	ModelFiles           []string
	ProjFile             string
	JinjaFile            string
	Device               string
	ContextWindow        int
	NBatch               int
	NUBatch              int
	NThreads             int
	NThreadsBatch        int
	CacheTypeK           GGMLType
	CacheTypeV           GGMLType
	FlashAttention       FlashAttentionType
	UseDirectIO          bool
	IgnoreIntegrityCheck bool
	NSeqMax              int
	OffloadKQV           *bool
	OpOffload            *bool
	NGpuLayers           *int
	SplitMode            SplitMode
	SystemPromptCache    bool
	IncrementalCache     bool
	MaxCacheSessions     int
	CacheMinTokens       int
	InsecureLogging      bool
	RopeScaling          RopeScalingType
	RopeFreqBase         *float32
	RopeFreqScale        *float32
	YarnExtFactor        *float32
	YarnAttnFactor       *float32
	YarnBetaFast         *float32
	YarnBetaSlow         *float32
	YarnOrigCtx          *int
	DefaultParams        Params
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

	return fmt.Sprintf("\nModelFiles[%v]\nProjFile[%s]\nJinjaFile[%s]\nDevice[%s]\nContextWindow[%d]\nNBatch[%d]\nNUBatch[%d]\nNThreads[%d]\nNThreadsBatch[%d]\nCacheTypeK[%d]\nCacheTypeV[%d]\nUseDirectIO[%t]\nFlashAttention[%d]\nIgnoreIntegrityCheck[%t]\nNSeqMax[%d]\nOffloadKQV[%s]\nOpOffload[%s]\nNGpuLayers[%s]\nSplitMode[%d]\nSystemPromptCache[%t]\nIncrementalCache[%t]\nMaxCacheSessions[%d]\nCacheMinTokens[%d]\nInsecureLogging[%t]\nRopeScaling[%s]\nRopeFreqBase[%s]\nRopeFreqScale[%s]\nYarnExtFactor[%s]\nYarnAttnFactor[%s]\nYarnBetaFast[%s]\nYarnBetaSlow[%s]\nYarnOrigCtx[%s]\n",
		cfg.ModelFiles, cfg.ProjFile, cfg.JinjaFile, cfg.Device, cfg.ContextWindow, cfg.NBatch, cfg.NUBatch, cfg.NThreads, cfg.NThreadsBatch,
		cfg.CacheTypeK, cfg.CacheTypeV, cfg.UseDirectIO, cfg.FlashAttention, cfg.IgnoreIntegrityCheck,
		cfg.NSeqMax, formatBoolPtr(cfg.OffloadKQV), formatBoolPtr(cfg.OpOffload),
		formatIntPtr(cfg.NGpuLayers), cfg.SplitMode, cfg.SystemPromptCache, cfg.IncrementalCache, cfg.MaxCacheSessions, cfg.CacheMinTokens, cfg.InsecureLogging,
		cfg.RopeScaling, formatFloat32Ptr(cfg.RopeFreqBase), formatFloat32Ptr(cfg.RopeFreqScale),
		formatFloat32Ptr(cfg.YarnExtFactor), formatFloat32Ptr(cfg.YarnAttnFactor),
		formatFloat32Ptr(cfg.YarnBetaFast), formatFloat32Ptr(cfg.YarnBetaSlow), formatIntPtr(cfg.YarnOrigCtx))
}

func validateConfig(ctx context.Context, cfg Config, log Logger) error {
	if len(cfg.ModelFiles) == 0 {
		return fmt.Errorf("validate-config: model file is required")
	}

	if cfg.SystemPromptCache && cfg.IncrementalCache {
		return fmt.Errorf("validate-config: cannot enable both SystemPromptCache and IncrementalCache; use IncrementalCache alone (it includes the system prompt)")
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

	// Default MaxCacheSessions when caching is enabled.
	if (cfg.SystemPromptCache || cfg.IncrementalCache) && cfg.MaxCacheSessions <= 0 {
		cfg.MaxCacheSessions = defMaxCacheSessions
	}

	return cfg
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

	// Reserve sequences for caching based on enabled modes.
	// Both SPC and IMC use seqs 0 to MaxCacheSessions-1.
	// Remaining seqs: batch engine slots.
	nSeqMax := max(cfg.NSeqMax, 1)
	var cacheSeqs int
	switch {
	case cfg.SystemPromptCache:
		cacheSeqs = max(cfg.MaxCacheSessions, 1)
	case cfg.IncrementalCache:
		cacheSeqs = max(cfg.MaxCacheSessions, 1)
	}
	totalSeqs := nSeqMax + cacheSeqs

	if cfg.ContextWindow > 0 {
		ctxParams.NBatch = uint32(cfg.NBatch)
		ctxParams.NUbatch = uint32(cfg.NUBatch)
		ctxParams.NThreads = int32(cfg.NThreads)
		ctxParams.NThreadsBatch = int32(cfg.NThreadsBatch)

		// For IMC, scale NCtx by total sequences so users get their configured
		// context per slot. IMC caches the full conversation, so without scaling
		// the effective context per slot would be reduced. SPC only caches the
		// system prompt (typically small), so no scaling needed.
		switch {
		case cfg.IncrementalCache:
			ctxParams.NCtx = uint32(cfg.ContextWindow * totalSeqs)
		default:
			ctxParams.NCtx = uint32(cfg.ContextWindow)
		}
	}

	switch {
	case cfg.CacheTypeK > -2 && cfg.CacheTypeK < 41:
		ctxParams.TypeK = cfg.CacheTypeK.ToYZMAType()
	default:
		ctxParams.TypeK = GGMLTypeQ8_0.ToYZMAType()
	}

	switch {
	case cfg.CacheTypeV > -2 && cfg.CacheTypeV < 41:
		ctxParams.TypeV = cfg.CacheTypeV.ToYZMAType()
	default:
		ctxParams.TypeV = GGMLTypeQ8_0.ToYZMAType()
	}

	switch cfg.FlashAttention {
	case FlashAttentionDisabled:
		ctxParams.FlashAttentionType = llama.FlashAttentionTypeDisabled
	case FlashAttentionAuto:
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
	GGMLTypeAuto GGMLType = -1 // Use default from llama.cpp
	GGMLTypeF32  GGMLType = 0  // 32-bit floating point
	GGMLTypeF16  GGMLType = 1  // 16-bit floating point
	GGMLTypeQ4_0 GGMLType = 2  // 4-bit quantization (type 0)
	GGMLTypeQ4_1 GGMLType = 3  // 4-bit quantization (type 1)
	GGMLTypeQ5_0 GGMLType = 6  // 5-bit quantization (type 0)
	GGMLTypeQ5_1 GGMLType = 7  // 5-bit quantization (type 1)
	GGMLTypeQ8_0 GGMLType = 8  // 8-bit quantization (type 0) (default)
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
	return llama.GGMLType(t)
}

// UnmarshalYAML implements yaml.Unmarshaler to parse string values like "f16".
func (t *GGMLType) UnmarshalYAML(unmarshal func(interface{}) error) error {
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

// UnmarshalYAML implements yaml.Unmarshaler to parse string values.
func (t *FlashAttentionType) UnmarshalYAML(unmarshal func(interface{}) error) error {
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

// UnmarshalYAML implements yaml.Unmarshaler to parse string values.
func (s *SplitMode) UnmarshalYAML(unmarshal func(interface{}) error) error {
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

// UnmarshalYAML implements yaml.Unmarshaler to parse string values.
func (r *RopeScalingType) UnmarshalYAML(unmarshal func(interface{}) error) error {
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

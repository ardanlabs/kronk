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
// SystemPromptCache enables caching of system prompt KV state in sequence 0.
// When enabled, only messages with role="system" are cached. The system prompt
// is evaluated once and its KV cache is copied to all client sequences on subsequent
// requests with the same system prompt. This avoids redundant prefill computation
// for applications that use a consistent system prompt. The cache is automatically
// invalidated and re-evaluated when the system prompt changes.
//
// FirstMessageCache enables caching of the first user message's KV state. This
// supports clients like Cline that use a large first user message as context.
// Only messages with role="user" are cached. The cache is invalidated when the
// first user message changes.
//
// SystemPromptCache and FirstMessageCache are mutually exclusive - only one can
// be enabled at a time.
//
// CacheMinTokens sets the minimum token count required before caching. Messages
// shorter than this threshold are not cached, as the overhead of cache management
// may outweigh the prefill savings. When set to 0, defaults to 100 tokens.
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
	NGpuLayers           *int32
	SplitMode            SplitMode
	SystemPromptCache    bool
	FirstMessageCache    bool
	CacheMinTokens       int
}

func validateConfig(ctx context.Context, cfg Config, log Logger) error {
	if len(cfg.ModelFiles) == 0 {
		return fmt.Errorf("validate-config: model file is required")
	}

	if cfg.SystemPromptCache && cfg.FirstMessageCache {
		return fmt.Errorf("validate-config: SystemPromptCache and FirstMessageCache are mutually exclusive")
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
		if cfg.ProjFile != "" {
			cfg.NUBatch = defNUBatchVision
		} else {
			cfg.NUBatch = defNUBatch
		}
	}

	if cfg.NThreads < 0 {
		cfg.NThreads = 0
	}

	if cfg.NThreadsBatch < 0 {
		cfg.NThreadsBatch = 0
	}

	// NBatch is generally greater than or equal to NUBatch. The entire
	// NUBatch of tokens must fit into a physical batch for processing.
	if cfg.NUBatch > cfg.NBatch {
		cfg.NUBatch = cfg.NBatch
	}

	// This value must be 1 to properly configure the batch engine.
	if cfg.NSeqMax <= 0 {
		cfg.NSeqMax = 1
	}

	// Default minimum tokens for caching.
	if (cfg.SystemPromptCache || cfg.FirstMessageCache) && cfg.CacheMinTokens <= 0 {
		cfg.CacheMinTokens = defMinCacheTokens
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

	if cfg.ContextWindow > 0 {
		ctxParams.NBatch = uint32(cfg.NBatch)
		ctxParams.NUbatch = uint32(cfg.NUBatch)
		ctxParams.NCtx = uint32(cfg.ContextWindow)
		ctxParams.NThreads = int32(cfg.NThreads)
		ctxParams.NThreadsBatch = int32(cfg.NThreadsBatch)
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

	// +1 to allow seqIDs 1..NSeqMax (seqID 0 reserved).
	// When NSeqMax is 0, we still need at least 2 for single-slot batch engine.
	nSeqMax := max(cfg.NSeqMax, 1)
	ctxParams.NSeqMax = uint32(nSeqMax + 1)

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

package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// GGMLType represents a ggml data type for the KV cache.
// These values correspond to the ggml_type enum in llama.cpp.
type GGMLType int32

const (
	GGMLTypeF32  GGMLType = 0  // 32-bit floating point (default)
	GGMLTypeF16  GGMLType = 1  // 16-bit floating point
	GGMLTypeQ4_0 GGMLType = 2  // 4-bit quantization (type 0)
	GGMLTypeQ4_1 GGMLType = 3  // 4-bit quantization (type 1)
	GGMLTypeQ5_0 GGMLType = 6  // 5-bit quantization (type 0)
	GGMLTypeQ5_1 GGMLType = 7  // 5-bit quantization (type 1)
	GGMLTypeQ8_0 GGMLType = 8  // 8-bit quantization (type 0)
	GGMLTypeBF16 GGMLType = 30 // Brain floating point 16-bit
	GGMLTypeAuto GGMLType = -1 // Use default from llama.cpp
)

// FlashAttentionType controls when to enable Flash Attention.
// Flash Attention reduces memory usage and speeds up attention computation,
// especially beneficial for large context windows.
type FlashAttentionType int32

const (
	FlashAttentionAuto     FlashAttentionType = -1 // Automatically enable when appropriate
	FlashAttentionDisabled FlashAttentionType = 0  // Disable Flash Attention
	FlashAttentionEnabled  FlashAttentionType = 1  // Enable Flash Attention
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
	case "q8_0", "q8":
		return GGMLTypeQ8_0, nil
	case "bf16", "bfloat16":
		return GGMLTypeBF16, nil
	case "auto", "":
		return GGMLTypeAuto, nil
	default:
		return GGMLTypeAuto, fmt.Errorf("unknown ggml type: %s", s)
	}
}

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
	defContextWindow = 4 * 1024
	defNBatch        = 2 * 1024
	defNUBatch       = 512
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
// When left as zero value, FlashAttentionEnabled is used as the default.
// Set to FlashAttentionDisabled to explicitly disable, or FlashAttentionAuto to
// let llama.cpp decide.
//
// DefragThold is the KV cache defragmentation threshold. When the ratio of
// fragmented (holes) to total cache size exceeds this threshold, the cache is
// automatically defragmented. When left as zero value, 0.1 (10%) is used as
// the default. Set to a negative value to disable automatic defragmentation.
//
// IgnorelIntegrityCheck is a boolean that determines if the system should ignore
// a model integrity check before trying to use it.
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
	DefragThold          float32
	IgnoreIntegrityCheck bool
}

func validateConfig(cfg Config, log Logger) error {
	if len(cfg.ModelFiles) == 0 {
		return fmt.Errorf("validate-config: model file is required")
	}

	if !cfg.IgnoreIntegrityCheck {
		for _, modelFile := range cfg.ModelFiles {
			log(context.Background(), "checking-model-integrity", "model-file", modelFile)

			if err := CheckModel(modelFile, true); err != nil {
				return fmt.Errorf("validate-config: checking-model-integrity: %w", err)
			}
		}

		if cfg.ProjFile != "" {
			log(context.Background(), "checking-model-integrity", "model-file", cfg.ProjFile)

			if err := CheckModel(cfg.ProjFile, true); err != nil {
				return fmt.Errorf("validate-config: checking-model-integrity: %w", err)
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
		cfg.NUBatch = defNUBatch
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

	if mi.IsEmbedModel {
		ctxParams.Embeddings = 1
	}

	if cfg.ContextWindow > 0 {
		ctxParams.NBatch = uint32(cfg.NBatch)
		ctxParams.NUbatch = uint32(cfg.NUBatch)
		ctxParams.NCtx = uint32(cfg.ContextWindow)
		ctxParams.NThreads = int32(cfg.NThreads)
		ctxParams.NThreadsBatch = int32(cfg.NThreadsBatch)
	}

	if cfg.CacheTypeK > 0 {
		ctxParams.TypeK = int32(cfg.CacheTypeK)
	}

	if cfg.CacheTypeV > 0 {
		ctxParams.TypeV = int32(cfg.CacheTypeV)
	}

	switch cfg.FlashAttention {
	case FlashAttentionDisabled:
		ctxParams.FlashAttentionType = llama.FlashAttentionType(FlashAttentionDisabled)
	case FlashAttentionAuto:
		ctxParams.FlashAttentionType = llama.FlashAttentionType(FlashAttentionAuto)
	default:
		ctxParams.FlashAttentionType = llama.FlashAttentionType(FlashAttentionEnabled)
	}

	switch {
	case cfg.DefragThold < 0:
		// Negative value disables defragmentation
	case cfg.DefragThold > 0:
		ctxParams.DefragThold = cfg.DefragThold
	default:
		ctxParams.DefragThold = 0.1
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

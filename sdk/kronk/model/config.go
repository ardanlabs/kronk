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
// IgnorelIntegrityCheck is a boolean that determines if the system should ignore
// a model integrity check before trying to use it.
type Config struct {
	Log                   Logger
	ModelFiles            []string
	ProjFile              string
	JinjaFile             string
	Device                string
	ContextWindow         int
	NBatch                int
	NUBatch               int
	NThreads              int
	NThreadsBatch         int
	IgnorelIntegrityCheck bool
}

func validateConfig(cfg Config, log Logger) error {
	if len(cfg.ModelFiles) == 0 {
		return fmt.Errorf("validate-config: model file is required")
	}

	if !cfg.IgnorelIntegrityCheck {
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

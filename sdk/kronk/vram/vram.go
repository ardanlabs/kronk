// Package vram provides VRAM requirement calculation for GGUF models.
// It owns the pure-math estimator (Calculate), the configuration types
// the rest of Kronk passes around, and the HuggingFace-fetching helpers
// that drive the BUI's "before download" estimator.
package vram

import (
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

// Context window size constants (in tokens).
const (
	ContextWindow1K   int64 = 1024
	ContextWindow2K   int64 = 2048
	ContextWindow4K   int64 = 4096
	ContextWindow8K   int64 = 8192
	ContextWindow16K  int64 = 16384
	ContextWindow32K  int64 = 32768
	ContextWindow64K  int64 = 65536
	ContextWindow128K int64 = 131072
	ContextWindow256K int64 = 262144
)

// Bytes per element constants for KV cache types. Re-exported from
// sdk/kronk/gguf so callers don't need a second import.
const (
	BytesPerElementF32  = gguf.BytesPerElementF32
	BytesPerElementF16  = gguf.BytesPerElementF16
	BytesPerElementBF16 = gguf.BytesPerElementBF16
	BytesPerElementQ8_0 = gguf.BytesPerElementQ8_0
	BytesPerElementQ4_0 = gguf.BytesPerElementQ4_0
	BytesPerElementQ4_1 = gguf.BytesPerElementQ4_1
	BytesPerElementQ5_0 = gguf.BytesPerElementQ5_0
	BytesPerElementQ5_1 = gguf.BytesPerElementQ5_1
)

// Slot count constants.
const (
	Slots1 int64 = 1
	Slots2 int64 = 2
	Slots3 int64 = 3
	Slots4 int64 = 4
	Slots5 int64 = 5
)

// Config contains the user-provided parameters for VRAM calculation
// that cannot be extracted from the model file.
type Config struct {
	ContextWindow   int64 // n_ctx - context window size (e.g., 8192, 131072)
	BytesPerElement int64 // Depends on cache type: q8_0=1, f16=2
	Slots           int64 // n_seq_max - number of concurrent sequences
}

// Input contains all parameters needed to calculate VRAM requirements.
type Input struct {
	ModelSizeBytes    int64                 // Size of model weights in bytes
	ContextWindow     int64                 // n_ctx - context window size (e.g., 8192, 131072)
	BlockCount        int64                 // n_layers - number of transformer layers
	HeadCountKV       int64                 // Number of KV attention heads
	KeyLength         int64                 // K dimension per head (typically 128)
	ValueLength       int64                 // V dimension per head (typically 128)
	BytesPerElement   int64                 // Depends on cache type: q8_0=1, f16=2
	Slots             int64                 // n_seq_max - number of concurrent sequences
	EmbeddingLength   int64                 // needed for compute buffer estimate
	MoE               *gguf.MoEInfo         //
	Weights           *gguf.WeightBreakdown //
	GPULayers         int64                 // Number of layers on GPU (0 or -1 = all layers)
	ExpertLayersOnGPU int64                 // 0 = all experts on CPU
}

// Result contains the calculated VRAM requirements.
type Result struct {
	Input              Input // Input parameters used for calculation
	KVPerTokenPerLayer int64 // Bytes per token per layer
	KVPerSlot          int64 // Bytes per slot
	SlotMemory         int64 // Total KV cache memory in bytes
	TotalVRAM          int64 // Model size + slot memory in bytes
	MoE                *gguf.MoEInfo
	Weights            *gguf.WeightBreakdown
	ModelWeightsGPU    int64
	ModelWeightsCPU    int64
	ComputeBufferEst   int64
}

// Calculate computes the VRAM requirements for running a model based on
// the provided input parameters. The KV cache portion of the math is
// delegated to sdk/kronk/gguf.CalculateKVCache so the SDK and tools sides
// share a single implementation.
func Calculate(input Input) Result {
	kv := gguf.CalculateKVCache(gguf.KVCacheInput{
		ContextWindow:   input.ContextWindow,
		BlockCount:      input.BlockCount,
		HeadCountKV:     input.HeadCountKV,
		KeyLength:       input.KeyLength,
		ValueLength:     input.ValueLength,
		BytesPerElement: input.BytesPerElement,
		Slots:           input.Slots,
	})
	kvPerTokenPerLayer := kv.KVPerTokenPerLayer
	kvPerSlot := kv.KVPerSlot
	slotMemory := kv.SlotMemory

	gpuLayers := clampGPULayers(input.GPULayers, input.BlockCount)

	var modelWeightsGPU, modelWeightsCPU int64

	switch {
	case input.Weights != nil && input.MoE != nil && input.MoE.IsMoE:

		// Always-active weights are split proportionally by GPU layers.
		// When all layers are on GPU, all always-active weights stay on GPU.
		var alwaysActiveGPU, alwaysActiveCPU int64
		if gpuLayers >= input.BlockCount {
			alwaysActiveGPU = input.Weights.AlwaysActiveBytes
		} else {
			alwaysActiveGPU, alwaysActiveCPU = splitByGPULayers(input.Weights.AlwaysActiveBytes, gpuLayers, input.BlockCount)
		}

		// Expert weights are split by ExpertLayersOnGPU (expert offloading).
		var expertsGPU int64
		if input.ExpertLayersOnGPU > 0 && len(input.Weights.ExpertBytesByLayer) > 0 {
			blockCount := int64(len(input.Weights.ExpertBytesByLayer))
			startLayer := max(blockCount-input.ExpertLayersOnGPU, 0)
			for i := startLayer; i < blockCount; i++ {
				expertsGPU += input.Weights.ExpertBytesByLayer[i]
			}
		}

		modelWeightsGPU = alwaysActiveGPU + expertsGPU
		modelWeightsCPU = alwaysActiveCPU + max(0, input.Weights.ExpertBytesTotal-expertsGPU)

	default:

		// Dense models: split total model weights proportionally by GPU layers.
		if gpuLayers >= input.BlockCount {
			modelWeightsGPU = input.ModelSizeBytes
		} else {
			modelWeightsGPU, modelWeightsCPU = splitByGPULayers(input.ModelSizeBytes, gpuLayers, input.BlockCount)
		}
	}

	computeBufferEst := EstimateComputeBuffer(input)
	totalVRAM := modelWeightsGPU + slotMemory + computeBufferEst

	return Result{
		Input:              input,
		KVPerTokenPerLayer: kvPerTokenPerLayer,
		KVPerSlot:          kvPerSlot,
		SlotMemory:         slotMemory,
		TotalVRAM:          totalVRAM,
		MoE:                input.MoE,
		Weights:            input.Weights,
		ModelWeightsGPU:    modelWeightsGPU,
		ModelWeightsCPU:    modelWeightsCPU,
		ComputeBufferEst:   computeBufferEst,
	}
}

// =============================================================================

// clampGPULayers returns the effective number of GPU layers. A zero value
// (the default) or -1 means all layers on GPU, preserving backward
// compatibility with callers that don't set GPULayers.
func clampGPULayers(gpuLayers, blockCount int64) int64 {
	if gpuLayers <= 0 || gpuLayers > blockCount {
		return blockCount
	}

	return gpuLayers
}

// splitByGPULayers splits totalBytes proportionally between GPU and CPU based
// on how many layers are offloaded.
func splitByGPULayers(totalBytes, gpuLayers, blockCount int64) (gpu, cpu int64) {
	if blockCount <= 0 {
		return totalBytes, 0
	}

	gpu = (gpuLayers * totalBytes) / blockCount
	cpu = max(0, totalBytes-gpu)

	return gpu, cpu
}

// EstimateComputeBuffer provides a heuristic estimate of the compute buffer
// VRAM needed during inference. This is inherently approximate.
func EstimateComputeBuffer(input Input) int64 {
	const (
		baseBufferSmall = 256 * 1024 * 1024 // 256 MiB for models < 100B params
		baseBufferLarge = 512 * 1024 * 1024 // 512 MiB for models >= 100B params
		k               = 8                 // empirical multiplier
	)

	baseBuffer := int64(baseBufferSmall)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		baseBuffer = int64(baseBufferLarge)
	}

	var embeddingComponent int64
	if input.EmbeddingLength > 0 {
		nUBatch := int64(512)
		embeddingComponent = k * nUBatch * input.EmbeddingLength * 4
	}

	total := baseBuffer + embeddingComponent
	total = total + total/10

	return total
}

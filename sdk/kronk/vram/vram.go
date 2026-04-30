// Package vram provides VRAM requirement calculation for GGUF models.
// It owns the pure-math estimator (Calculate), the configuration types
// the rest of Kronk passes around, and the HuggingFace-fetching helpers
// that drive the BUI's "before download" estimator.
package vram

import (
	"fmt"

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
	ContextWindow     int64 // n_ctx - context window size (e.g., 8192, 131072)
	BytesPerElement   int64 // Depends on cache type: q8_0=1, f16=2
	Slots             int64 // n_seq_max - number of concurrent sequences
	GPULayers         int64 // Number of layers on GPU (0 or -1 = all layers).
	ExpertLayersOnGPU int64 // 0 = all experts on CPU.
	KVCacheOnCPU      bool  // Move KV cache off the GPU (offload-kqv: false).
}

// Input contains all parameters needed to calculate VRAM requirements.
type Input struct {
	ModelSizeBytes      int64                 // Size of model weights in bytes
	ContextWindow       int64                 // n_ctx - context window size (e.g., 8192, 131072)
	BlockCount          int64                 // n_layers - number of transformer layers
	HeadCountKV         int64                 // Number of KV attention heads
	KeyLength           int64                 // K dimension per head (typically 128)
	ValueLength         int64                 // V dimension per head (typically 128)
	BytesPerElement     int64                 // Depends on cache type: q8_0=1, f16=2
	Slots               int64                 // n_seq_max - number of concurrent sequences
	SlidingWindow       int64                 // Sliding-window size in tokens (0 = no SWA layers).
	SlidingWindowLayers int64                 // Layer count using SWA (0 = treat all BlockCount as full attention).
	EmbeddingLength     int64                 // needed for compute buffer estimate
	MoE                 *gguf.MoEInfo         //
	Weights             *gguf.WeightBreakdown //
	GPULayers           int64                 // Number of layers on GPU (0 or -1 = all layers)
	ExpertLayersOnGPU   int64                 // 0 = all experts on CPU
	KVCacheOnCPU        bool                  // Move KV cache off the GPU (offload-kqv: false)
}

// PerDeviceVRAM is the per-GPU breakdown of model weights, KV cache, and
// compute buffer when tensor_split is in effect. The first element is the
// main GPU; compute buffer is reported as fully on the main GPU.
type PerDeviceVRAM struct {
	Label        string
	WeightsBytes int64
	KVBytes      int64
	ComputeBytes int64
	TotalBytes   int64
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

	// MoE-specific weight breakdown (zero for dense models).
	AlwaysActiveGPUBytes int64
	AlwaysActiveCPUBytes int64
	ExpertGPUBytes       int64
	ExpertCPUBytes       int64

	// KV cache placement and total system RAM estimate. When
	// Input.KVCacheOnCPU is true, KVCPUBytes == SlotMemory and
	// KVVRAMBytes == 0; otherwise the inverse.
	KVVRAMBytes       int64
	KVCPUBytes        int64
	TotalSystemRAMEst int64

	// Per-device breakdown (populated by CalculatePerDevice when
	// tensor_split / device_count are passed by the caller).
	PerDevice []PerDeviceVRAM
}

// Calculate computes the VRAM requirements for running a model based on
// the provided input parameters. The KV cache portion of the math is
// delegated to sdk/kronk/gguf.CalculateKVCache so the SDK and tools sides
// share a single implementation.
func Calculate(input Input) Result {
	kv := gguf.CalculateKVCache(gguf.KVCacheInput{
		ContextWindow:       input.ContextWindow,
		BlockCount:          input.BlockCount,
		HeadCountKV:         input.HeadCountKV,
		KeyLength:           input.KeyLength,
		ValueLength:         input.ValueLength,
		BytesPerElement:     input.BytesPerElement,
		Slots:               input.Slots,
		SlidingWindow:       input.SlidingWindow,
		SlidingWindowLayers: input.SlidingWindowLayers,
	})
	kvPerTokenPerLayer := kv.KVPerTokenPerLayer
	kvPerSlot := kv.KVPerSlot
	slotMemory := kv.SlotMemory

	gpuLayers := clampGPULayers(input.GPULayers, input.BlockCount)

	var modelWeightsGPU, modelWeightsCPU int64
	var alwaysActiveGPU, alwaysActiveCPU, expertsGPU, expertsCPU int64

	switch {
	case input.Weights != nil && input.MoE != nil && input.MoE.IsMoE:

		// Always-active weights are split proportionally by GPU layers.
		// When all layers are on GPU, all always-active weights stay on GPU.
		if gpuLayers >= input.BlockCount {
			alwaysActiveGPU = input.Weights.AlwaysActiveBytes
		} else {
			alwaysActiveGPU, alwaysActiveCPU = splitByGPULayers(input.Weights.AlwaysActiveBytes, gpuLayers, input.BlockCount)
		}

		// Expert weights are split by ExpertLayersOnGPU (expert offloading).
		if input.ExpertLayersOnGPU > 0 && len(input.Weights.ExpertBytesByLayer) > 0 {
			blockCount := int64(len(input.Weights.ExpertBytesByLayer))
			startLayer := max(blockCount-input.ExpertLayersOnGPU, 0)
			for i := startLayer; i < blockCount; i++ {
				expertsGPU += input.Weights.ExpertBytesByLayer[i]
			}
		}
		expertsCPU = max(0, input.Weights.ExpertBytesTotal-expertsGPU)

		modelWeightsGPU = alwaysActiveGPU + expertsGPU
		modelWeightsCPU = alwaysActiveCPU + expertsCPU

	default:

		// Dense models: split total model weights proportionally by GPU layers.
		if gpuLayers >= input.BlockCount {
			modelWeightsGPU = input.ModelSizeBytes
		} else {
			modelWeightsGPU, modelWeightsCPU = splitByGPULayers(input.ModelSizeBytes, gpuLayers, input.BlockCount)
		}
	}

	computeBufferEst := EstimateComputeBuffer(input)

	var kvVRAMBytes, kvCPUBytes int64
	if input.KVCacheOnCPU {
		kvCPUBytes = slotMemory
	} else {
		kvVRAMBytes = slotMemory
	}

	totalVRAM := modelWeightsGPU + kvVRAMBytes + computeBufferEst
	totalSystemRAMEst := modelWeightsCPU + kvCPUBytes

	return Result{
		Input:                input,
		KVPerTokenPerLayer:   kvPerTokenPerLayer,
		KVPerSlot:            kvPerSlot,
		SlotMemory:           slotMemory,
		TotalVRAM:            totalVRAM,
		MoE:                  input.MoE,
		Weights:              input.Weights,
		ModelWeightsGPU:      modelWeightsGPU,
		ModelWeightsCPU:      modelWeightsCPU,
		ComputeBufferEst:     computeBufferEst,
		AlwaysActiveGPUBytes: alwaysActiveGPU,
		AlwaysActiveCPUBytes: alwaysActiveCPU,
		ExpertGPUBytes:       expertsGPU,
		ExpertCPUBytes:       expertsCPU,
		KVVRAMBytes:          kvVRAMBytes,
		KVCPUBytes:           kvCPUBytes,
		TotalSystemRAMEst:    totalSystemRAMEst,
	}
}

// CalculatePerDevice splits the GPU model weights, KV cache, and compute
// buffer across deviceCount GPUs based on tensorSplit fractions. The
// compute buffer is reported as fully allocated on mainGPUIndex (default
// 0). When deviceCount <= 1 a single entry is returned. deviceLabels
// override the default "GPU N" labels when provided.
func CalculatePerDevice(modelWeightsGPU, slotMemory, computeBufferEst, deviceCount int64, tensorSplit []float64, deviceLabels []string, mainGPUIndex int) []PerDeviceVRAM {
	label := func(i int, isMain bool) string {
		if i < len(deviceLabels) && deviceLabels[i] != "" {
			return deviceLabels[i]
		}
		if isMain {
			return fmt.Sprintf("GPU %d (main)", i)
		}
		return fmt.Sprintf("GPU %d", i)
	}

	if deviceCount <= 1 {
		return []PerDeviceVRAM{{
			Label:        label(0, true),
			WeightsBytes: modelWeightsGPU,
			KVBytes:      slotMemory,
			ComputeBytes: computeBufferEst,
			TotalBytes:   modelWeightsGPU + slotMemory + computeBufferEst,
		}}
	}

	fractions := make([]float64, deviceCount)
	if int64(len(tensorSplit)) == deviceCount {
		var sum float64
		for i, v := range tensorSplit {
			if v < 0 || v != v { // NaN check
				v = 0
			}
			fractions[i] = v
			sum += v
		}
		if sum > 0 {
			for i := range fractions {
				fractions[i] /= sum
			}
		} else {
			for i := range fractions {
				fractions[i] = 1.0 / float64(deviceCount)
			}
		}
	} else {
		for i := range fractions {
			fractions[i] = 1.0 / float64(deviceCount)
		}
	}

	out := make([]PerDeviceVRAM, 0, deviceCount)
	wRemaining := modelWeightsGPU
	kvRemaining := slotMemory

	for i := int64(0); i < deviceCount; i++ {
		isLast := i == deviceCount-1
		var w, kv int64
		if isLast {
			w = wRemaining
			kv = kvRemaining
		} else {
			w = int64(float64(modelWeightsGPU) * fractions[i])
			kv = int64(float64(slotMemory) * fractions[i])
		}
		wRemaining -= w
		kvRemaining -= kv

		var comp int64
		isMain := int(i) == mainGPUIndex
		if isMain {
			comp = computeBufferEst
		}

		out = append(out, PerDeviceVRAM{
			Label:        label(int(i), isMain),
			WeightsBytes: w,
			KVBytes:      kv,
			ComputeBytes: comp,
			TotalBytes:   w + kv + comp,
		})
	}

	return out
}

// FitConstraints describes the available hardware budget for AutoFit.
// GPUFreeBytes is the per-device free VRAM in bytes (length must equal
// DeviceCount when DeviceCount > 0). When GPUFreeBytes is empty,
// CombinedFreeBytes is used as a single aggregate capacity.
type FitConstraints struct {
	DeviceCount       int64
	GPUFreeBytes      []int64
	CombinedFreeBytes int64
	SystemRAMBytes    int64
	TensorSplit       []float64
	KVCacheOnCPU      bool
	// Threshold to consider a configuration "fits" (e.g., 0.95 means the
	// candidate must occupy at most 95% of available capacity).
	Threshold float64
}

// AutoFit searches for the largest GPU offload configuration that fits
// within the supplied hardware constraints. Returns the best gpuLayers
// and expertLayersOnGPU values along with the resulting Result.
//
// For MoE models, expert offloading is preferred (all layers on GPU,
// maximize expert layers) and falls back to layer offloading if the
// always-active weights alone don't fit. For dense models, the maximum
// gpuLayers value that fits is selected.
func AutoFit(input Input, constraints FitConstraints) (gpuLayers int64, expertLayersOnGPU int64, result Result) {
	threshold := constraints.Threshold
	if threshold <= 0 || threshold > 1 {
		threshold = 0.95
	}

	blockCount := input.BlockCount
	if blockCount <= 0 {
		// Nothing to fit; just compute with defaults.
		return 0, 0, Calculate(input)
	}

	deviceCount := constraints.DeviceCount
	if deviceCount < 1 {
		deviceCount = 1
	}

	hasPerGPU := int64(len(constraints.GPUFreeBytes)) == deviceCount && deviceCount > 0

	var combined int64
	if hasPerGPU {
		for _, b := range constraints.GPUFreeBytes {
			combined += b
		}
	} else {
		combined = constraints.CombinedFreeBytes
	}

	// If we have no GPU capacity info we can't auto-fit; return
	// "everything on GPU" defaults.
	if combined <= 0 {
		full := input
		full.GPULayers = blockCount
		if input.MoE != nil && input.MoE.IsMoE {
			full.ExpertLayersOnGPU = blockCount
		}
		return blockCount, full.ExpertLayersOnGPU, Calculate(full)
	}

	fits := func(v Result) bool {
		var fitsGPU bool
		if hasPerGPU && deviceCount > 1 {
			perDev := CalculatePerDevice(v.ModelWeightsGPU, v.KVVRAMBytes, v.ComputeBufferEst, deviceCount, constraints.TensorSplit, nil, 0)
			fitsGPU = true
			for i, dev := range perDev {
				cap := constraints.GPUFreeBytes[i]
				if cap <= 0 || dev.TotalBytes > int64(float64(cap)*threshold) {
					fitsGPU = false
					break
				}
			}
		} else {
			var singleCap int64
			if hasPerGPU {
				singleCap = constraints.GPUFreeBytes[0]
			} else {
				singleCap = combined
			}
			fitsGPU = singleCap > 0 && v.TotalVRAM <= int64(float64(singleCap)*threshold)
		}

		fitsRAM := true
		if constraints.SystemRAMBytes > 0 {
			fitsRAM = v.TotalSystemRAMEst <= int64(float64(constraints.SystemRAMBytes)*threshold)
		}

		return fitsGPU && fitsRAM
	}

	isMoE := input.MoE != nil && input.MoE.IsMoE && input.Weights != nil

	if isMoE {
		// Expert offloading first.
		bestExperts := int64(-1)
		for experts := blockCount; experts >= 0; experts-- {
			candidate := input
			candidate.GPULayers = blockCount
			candidate.ExpertLayersOnGPU = experts
			candidate.KVCacheOnCPU = constraints.KVCacheOnCPU
			v := Calculate(candidate)
			if fits(v) {
				bestExperts = experts
				result = v
				break
			}
		}
		if bestExperts >= 0 {
			return blockCount, bestExperts, result
		}

		// Fall back to layer offloading.
		for ngl := blockCount; ngl >= 0; ngl-- {
			candidate := input
			candidate.GPULayers = ngl
			candidate.ExpertLayersOnGPU = ngl
			candidate.KVCacheOnCPU = constraints.KVCacheOnCPU
			v := Calculate(candidate)
			if fits(v) {
				return ngl, ngl, v
			}
		}

		// Nothing fits — return zero offload.
		zero := input
		zero.GPULayers = 0
		zero.ExpertLayersOnGPU = 0
		zero.KVCacheOnCPU = constraints.KVCacheOnCPU
		return 0, 0, Calculate(zero)
	}

	// Dense: find max gpuLayers that fits.
	bestGPULayers := int64(0)
	var bestResult Result
	for ngl := int64(0); ngl <= blockCount; ngl++ {
		candidate := input
		candidate.GPULayers = ngl
		candidate.KVCacheOnCPU = constraints.KVCacheOnCPU
		v := Calculate(candidate)
		if fits(v) {
			bestGPULayers = ngl
			bestResult = v
		}
	}
	if bestResult.TotalVRAM == 0 {
		// Nothing fit; return zero-layer Calculate result for callers.
		zero := input
		zero.GPULayers = 0
		zero.KVCacheOnCPU = constraints.KVCacheOnCPU
		bestResult = Calculate(zero)
	}
	return bestGPULayers, 0, bestResult
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

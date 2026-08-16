// Package vram provides VRAM requirement calculation for GGUF models.
// It owns the pure-math estimator (Calculate), the configuration types
// the rest of Kronk passes around, and the HuggingFace-fetching helpers
// that drive the BUI's "before download" estimator.
package vram

import (
	"fmt"
	"math/big"

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

// DefaultNUBatch is the physical batch capacity used when a caller does not
// provide an effective NUBatch.
const DefaultNUBatch int64 = 2 * 1024

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
	ContextWindow          int64 // n_ctx - context window size (e.g., 8192, 131072)
	BytesPerElement        int64 // Depends on cache type: q8_0=1, f16=2
	TypeK                  int32 // Kronk GGML cache type ID (0 = use BytesPerElement).
	TypeV                  int32 // Kronk GGML cache type ID (0 = use BytesPerElement).
	Slots                  int64 // n_seq_max - number of concurrent sequences
	NUBatch                int64 // Effective physical batch size.
	GPULayers              int64 // Number of layers on GPU (0 = all, -1 = none).
	ExpertLayersOnGPU      int64 // 0 = all experts on CPU.
	KVCacheOnCPU           bool  // Move KV cache off the GPU (offload-kqv: false).
	SWAFull                bool  // Size SWA layers against the full context window.
	VTransposed            bool  // Size V using the model-wide transposed layout.
	RecurrentStateCopies   int64 // State copies for separate-model or companion-MTP speculation.
	EmbeddedMTPStateCopies int64 // State copies when the GGUF contains an embedded MTP layer.
}

// Input contains all parameters needed to calculate VRAM requirements.
type Input struct {
	ModelSizeBytes       int64                 // Size of model weights in bytes
	ContextWindow        int64                 // n_ctx - context window size (e.g., 8192, 131072)
	BlockCount           int64                 // n_layers - number of transformer layers
	HeadCountKV          int64                 // Number of KV attention heads
	KeyLength            int64                 // K dimension per head (typically 128)
	ValueLength          int64                 // V dimension per head (typically 128)
	SWAHeadCountKV       int64                 // Number of KV heads in SWA layers (0 = HeadCountKV)
	SWAKeyLength         int64                 // K dimension in SWA layers (0 = KeyLength)
	SWAValueLength       int64                 // V dimension in SWA layers (0 = ValueLength)
	HeadCountKVByLayer   []int64               // Exact per-layer KV head counts when available.
	BytesPerElement      int64                 // Depends on cache type: q8_0=1, f16=2
	TypeK                int32                 // Kronk GGML cache type ID (0 = use BytesPerElement).
	TypeV                int32                 // Kronk GGML cache type ID (0 = use BytesPerElement).
	Slots                int64                 // n_seq_max - number of concurrent sequences
	SlidingWindow        int64                 // Sliding-window size in tokens (0 = no SWA layers).
	SlidingWindowLayers  int64                 // Layer count using SWA (0 = treat all BlockCount as full attention).
	SharedKVLayers       int64                 // Trailing layers that reuse another layer's KV tensors.
	SWAPattern           []bool                // Per-layer SWA classification when available.
	RecurrentPattern     []bool                // Per-layer recurrent classification when available.
	NextNPredictLayers   int64                 // Appended MTP layers excluded from target context memory.
	RecurrentStateBytes  int64                 // State bytes per recurrent layer, sequence, and copy.
	RecurrentStateCopies int64                 // Current state plus speculative rollback copies.
	NUBatch              int64                 // Effective physical batch size.
	KVUnified            bool                  // Whether all sequence slots share one KV pool.
	EmbeddingLength      int64                 // needed for compute buffer estimate
	MoE                  *gguf.MoEInfo         //
	Weights              *gguf.WeightBreakdown //
	GPULayers            int64                 // Number of layers on GPU (0 = all, -1 = none)
	ExpertLayersOnGPU    int64                 // 0 = all experts on CPU
	KVCacheOnCPU         bool                  // Move KV cache off the GPU (offload-kqv: false)
	SWAFull              bool                  // Size SWA layers against the full context window.
	VTransposed          bool                  // Size V using the model-wide transposed layout.
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
		ContextWindow:        input.ContextWindow,
		BlockCount:           input.BlockCount,
		HeadCountKV:          input.HeadCountKV,
		KeyLength:            input.KeyLength,
		ValueLength:          input.ValueLength,
		SWAHeadCountKV:       input.SWAHeadCountKV,
		SWAKeyLength:         input.SWAKeyLength,
		SWAValueLength:       input.SWAValueLength,
		HeadCountKVByLayer:   input.HeadCountKVByLayer,
		BytesPerElement:      input.BytesPerElement,
		TypeK:                input.TypeK,
		TypeV:                input.TypeV,
		Slots:                input.Slots,
		SlidingWindow:        input.SlidingWindow,
		SlidingWindowLayers:  input.SlidingWindowLayers,
		SharedKVLayers:       input.SharedKVLayers,
		SWAPattern:           input.SWAPattern,
		RecurrentPattern:     input.RecurrentPattern,
		NextNPredictLayers:   input.NextNPredictLayers,
		RecurrentStateBytes:  input.RecurrentStateBytes,
		RecurrentStateCopies: input.RecurrentStateCopies,
		NUBatch:              input.NUBatch,
		KVUnified:            input.KVUnified,
		SWAFull:              input.SWAFull,
		VTransposed:          input.VTransposed,
	})
	kvPerTokenPerLayer := kv.KVPerTokenPerLayer
	kvPerSlot := kv.KVPerSlot
	slotMemory := kv.SlotMemory

	gpuLayers := clampGPULayers(input.GPULayers, input.BlockCount)

	var modelWeightsGPU, modelWeightsCPU int64
	var alwaysActiveGPU, alwaysActiveCPU, expertsGPU, expertsCPU int64

	switch {
	case input.Weights != nil && input.MoE != nil && input.MoE.IsMoE:

		// The GGUF analyzer's per-tensor byte accounting does not
		// survive some non-standard quantizations (notably the MXFP4
		// packing used by gpt-oss), so Weights.ExpertBytesTotal can
		// undercount what's actually on disk by an order of magnitude.
		// Fall back to "file size minus always-active" as the honest
		// expert footprint and rescale the per-layer breakdown so
		// expert-offload math still produces sensible numbers. For
		// well-analyzed models (most quants) the rescale factor is 1
		// and behavior is unchanged.
		expertsTotal := input.Weights.ExpertBytesTotal
		if t := input.ModelSizeBytes - input.Weights.AlwaysActiveBytes; t > expertsTotal {
			expertsTotal = t
		}
		perLayerExpert := input.Weights.ExpertBytesByLayer
		if expertsTotal != input.Weights.ExpertBytesTotal && len(perLayerExpert) > 0 {
			perLayerExpert = scaledPerLayer(perLayerExpert, input.Weights.ExpertBytesTotal, expertsTotal)
		}

		// Always-active weights are split proportionally by GPU layers.
		// When all layers are on GPU, all always-active weights stay on GPU.
		if gpuLayers >= input.BlockCount {
			alwaysActiveGPU = input.Weights.AlwaysActiveBytes
		} else {
			alwaysActiveGPU, alwaysActiveCPU = splitByGPULayers(input.Weights.AlwaysActiveBytes, gpuLayers, input.BlockCount)
		}

		// Expert weights are split by ExpertLayersOnGPU (expert offloading).
		if input.ExpertLayersOnGPU > 0 && len(perLayerExpert) > 0 {
			blockCount := int64(len(perLayerExpert))
			startLayer := max(blockCount-input.ExpertLayersOnGPU, 0)
			for i := startLayer; i < blockCount; i++ {
				expertsGPU += perLayerExpert[i]
			}
		}
		expertsCPU = max(0, expertsTotal-expertsGPU)

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
	gpuLayerStart := input.BlockCount - gpuLayers
	for layer, bytes := range kv.LayerMemory {
		if !input.KVCacheOnCPU && int64(layer) >= gpuLayerStart {
			kvVRAMBytes += bytes
		} else {
			kvCPUBytes += bytes
		}
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

	for i := range deviceCount {
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
	UnifiedMemory     bool
	// Threshold to consider a configuration "fits" (e.g., 0.95 means the
	// candidate must occupy at most 95% of available capacity).
	Threshold float64
}

// FitStatus describes how an estimated memory requirement compares with the
// configured capacity.
type FitStatus string

const (
	// FitStatusUnknown indicates that no capacity was supplied.
	FitStatusUnknown FitStatus = "unknown"
	// FitStatusFits indicates that the estimate fits with comfortable headroom.
	FitStatusFits FitStatus = "fits"
	// FitStatusTight indicates that the estimate fits but uses more than 80% of capacity.
	FitStatusTight FitStatus = "tight"
	// FitStatusDoesNotFit indicates that the estimate exceeds the fit threshold.
	FitStatusDoesNotFit FitStatus = "does_not_fit"
)

// CapacityAssessment describes one memory pool's required bytes, available
// capacity, remaining headroom, and fit status.
type CapacityAssessment struct {
	RequiredBytes int64
	CapacityBytes int64
	HeadroomBytes int64
	Status        FitStatus
}

// FitAssessment describes whether a calculated placement fits the supplied
// hardware constraints.
type FitAssessment struct {
	Fits      bool
	Status    FitStatus
	GPU       CapacityAssessment
	SystemRAM CapacityAssessment
	Unified   CapacityAssessment
}

// AssessFit compares a calculated placement with the supplied hardware
// constraints using the same thresholds as AutoFit.
func AssessFit(result Result, constraints FitConstraints) FitAssessment {
	threshold := constraints.Threshold
	if threshold <= 0 || threshold > 1 {
		threshold = 0.95
	}

	if constraints.UnifiedMemory {
		unified := assessCapacity(result.UnifiedFootprint(), constraints.SystemRAMBytes, threshold)
		return FitAssessment{
			Fits:      statusFits(unified.Status),
			Status:    unified.Status,
			GPU:       CapacityAssessment{Status: FitStatusUnknown},
			SystemRAM: CapacityAssessment{Status: FitStatusUnknown},
			Unified:   unified,
		}
	}

	deviceCount := max(constraints.DeviceCount, 1)
	hasPerGPU := int64(len(constraints.GPUFreeBytes)) == deviceCount

	var gpu CapacityAssessment
	if hasPerGPU && deviceCount > 1 {
		perDevice := CalculatePerDevice(result.ModelWeightsGPU, result.KVVRAMBytes, result.ComputeBufferEst, deviceCount, constraints.TensorSplit, nil, 0)
		gpu.RequiredBytes = result.TotalVRAM
		for _, capacity := range constraints.GPUFreeBytes {
			gpu.CapacityBytes += capacity
		}
		gpu.HeadroomBytes = gpu.CapacityBytes - gpu.RequiredBytes
		gpu.Status = FitStatusFits
		for i, device := range perDevice {
			assessment := assessCapacity(device.TotalBytes, constraints.GPUFreeBytes[i], threshold)
			if assessment.Status == FitStatusUnknown {
				gpu.Status = FitStatusUnknown
				break
			}
			gpu.Status = worseFitStatus(gpu.Status, assessment.Status)
		}
	} else {
		capacity := constraints.CombinedFreeBytes
		if hasPerGPU {
			capacity = constraints.GPUFreeBytes[0]
		}
		gpu = assessCapacity(result.TotalVRAM, capacity, threshold)
	}

	ram := CapacityAssessment{Status: FitStatusUnknown}
	if constraints.SystemRAMBytes > 0 {
		ram = assessCapacity(result.TotalSystemRAMEst, constraints.SystemRAMBytes, threshold)
	}

	status := gpu.Status
	if status != FitStatusUnknown && result.TotalSystemRAMEst > 0 {
		if ram.Status == FitStatusUnknown {
			status = FitStatusUnknown
		} else {
			status = worseFitStatus(status, ram.Status)
		}
	}

	return FitAssessment{
		Fits:      statusFits(status),
		Status:    status,
		GPU:       gpu,
		SystemRAM: ram,
		Unified:   CapacityAssessment{Status: FitStatusUnknown},
	}
}

// AutoFit searches for the largest GPU offload configuration that fits
// within the supplied hardware constraints. Returns the best gpuLayers
// and expertLayersOnGPU values along with the resulting Result.
//
// For MoE models, expert offloading is preferred (all layers on GPU,
// maximize expert layers) and falls back to layer offloading if the
// always-active weights alone don't fit. For dense models, the maximum
// gpuLayers value that fits is selected. The fit result reports whether
// any placement satisfies the supplied constraints.
func AutoFit(input Input, constraints FitConstraints) (gpuLayers int64, expertLayersOnGPU int64, result Result, fit bool) {
	blockCount := input.BlockCount
	if blockCount <= 0 {
		// Nothing to fit; just compute with defaults.
		return 0, 0, Calculate(input), false
	}

	if constraints.UnifiedMemory {
		full := input
		full.GPULayers = blockCount
		if input.MoE != nil && input.MoE.IsMoE {
			full.ExpertLayersOnGPU = blockCount
		}
		result := Calculate(full)
		return blockCount, full.ExpertLayersOnGPU, result, AssessFit(result, constraints).Fits
	}

	deviceCount := max(constraints.DeviceCount, 1)

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
		return blockCount, full.ExpertLayersOnGPU, Calculate(full), false
	}

	fits := func(v Result) bool { return AssessFit(v, constraints).Fits }

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
			return blockCount, bestExperts, result, true
		}

		// Fall back to layer offloading.
		for ngl := blockCount; ngl >= 0; ngl-- {
			candidate := input
			candidate.GPULayers = ngl
			if ngl == 0 {
				candidate.GPULayers = -1
			}
			candidate.ExpertLayersOnGPU = ngl
			candidate.KVCacheOnCPU = constraints.KVCacheOnCPU
			v := Calculate(candidate)
			if fits(v) {
				return candidate.GPULayers, ngl, v, true
			}
		}

		// Nothing fits — return zero offload.
		zero := input
		zero.GPULayers = -1
		zero.ExpertLayersOnGPU = 0
		zero.KVCacheOnCPU = constraints.KVCacheOnCPU
		return -1, 0, Calculate(zero), false
	}

	// Dense: find max gpuLayers that fits.
	bestGPULayers := int64(0)
	var bestResult Result
	var found bool
	for ngl := int64(0); ngl <= blockCount; ngl++ {
		candidate := input
		candidate.GPULayers = ngl
		if ngl == 0 {
			candidate.GPULayers = -1
		}
		candidate.KVCacheOnCPU = constraints.KVCacheOnCPU
		v := Calculate(candidate)
		if fits(v) {
			bestGPULayers = candidate.GPULayers
			bestResult = v
			found = true
		}
	}
	if !found {
		// Nothing fit; return zero-layer Calculate result for callers.
		zero := input
		zero.GPULayers = -1
		zero.KVCacheOnCPU = constraints.KVCacheOnCPU
		bestGPULayers = -1
		bestResult = Calculate(zero)
	}
	return bestGPULayers, 0, bestResult, found
}

// =============================================================================

// UnifiedFootprint returns the bytes a model occupies once every page of
// the GGUF is resident in unified memory. This is the value the resman
// reserves and the BUI displays on Apple Silicon (and any future
// unified-memory platform), where the MoE-aware GPU/CPU split that
// Calculate produces does not correspond to a real physical separation.
//
// The formula intentionally uses the raw model bytes (Input.ModelSizeBytes)
// rather than ModelWeightsGPU+ModelWeightsCPU so a model whose GGUF
// analyzer is missing the MoE expert breakdown still reserves the full
// file. SlotMemory and ComputeBufferEst round out the live footprint.
func (r Result) UnifiedFootprint() int64 {
	return r.Input.ModelSizeBytes + r.SlotMemory + r.ComputeBufferEst
}

func assessCapacity(required, capacity int64, threshold float64) CapacityAssessment {
	assessment := CapacityAssessment{
		RequiredBytes: required,
		CapacityBytes: capacity,
		HeadroomBytes: capacity - required,
		Status:        FitStatusUnknown,
	}
	if capacity <= 0 {
		return assessment
	}

	if required > int64(float64(capacity)*threshold) {
		assessment.Status = FitStatusDoesNotFit
	} else if required > int64(float64(capacity)*0.8) {
		assessment.Status = FitStatusTight
	} else {
		assessment.Status = FitStatusFits
	}

	return assessment
}

func statusFits(status FitStatus) bool {
	return status == FitStatusFits || status == FitStatusTight
}

func worseFitStatus(a, b FitStatus) FitStatus {
	rank := func(status FitStatus) int {
		switch status {
		case FitStatusDoesNotFit:
			return 3
		case FitStatusTight:
			return 2
		case FitStatusFits:
			return 1
		default:
			return 0
		}
	}

	if rank(b) > rank(a) {
		return b
	}
	return a
}

// clampGPULayers returns the effective number of GPU layers. A zero value
// preserves the default of all layers on GPU; -1 explicitly selects CPU-only.
func clampGPULayers(gpuLayers, blockCount int64) int64 {
	if gpuLayers == -1 {
		return 0
	}
	if gpuLayers <= 0 || gpuLayers > blockCount {
		return blockCount
	}

	return gpuLayers
}

// scaledPerLayer rescales perLayer so its sum equals newTotal. Used
// when the analyzer's per-tensor byte accounting undercounts the file
// (unrecognised quantizations) but the per-layer ratios are still
// meaningful. When origTotal is zero perLayer is returned unchanged so
// the caller can decide what to do.
//
// The intermediate b*newTotal product can exceed int64 for large MoE
// models (e.g. ~800 MiB per-layer * ~36 GB total ≈ 3e19), so the
// multiplication is performed with math/big to avoid silent overflow.
func scaledPerLayer(perLayer []int64, origTotal, newTotal int64) []int64 {
	if len(perLayer) == 0 || origTotal <= 0 || origTotal == newTotal {
		return perLayer
	}

	bigNew := big.NewInt(newTotal)
	bigOrig := big.NewInt(origTotal)
	tmp := new(big.Int)

	scaled := make([]int64, len(perLayer))
	for i, b := range perLayer {
		tmp.SetInt64(b)
		tmp.Mul(tmp, bigNew)
		tmp.Quo(tmp, bigOrig)
		scaled[i] = tmp.Int64()
	}

	return scaled
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
//
// The estimate scales with NSeqMax (Input.Slots) because llama.cpp grows
// its logical batch and per-sequence bookkeeping (KQ masks, position
// buffers, per-token logits) as more parallel slots are configured. The
// scaling is sub-linear; the multiplier 1 + 0.25*(slots-1) yields 1.0,
// 1.25, 1.5, 1.75, 2.0 for slots 1..5, matching observed llama-server
// growth at --parallel 1/2/4 within ~10% on the models we have measured.
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

	slots := max(input.Slots, 1)
	slotMultiplier := 1.0 + 0.25*float64(slots-1)
	nUBatch := input.NUBatch
	if nUBatch <= 0 {
		nUBatch = DefaultNUBatch
	}

	var embeddingComponent int64
	if input.EmbeddingLength > 0 {
		base := int64(k) * nUBatch * input.EmbeddingLength * 4
		embeddingComponent = int64(float64(base) * slotMultiplier)
	}

	total := baseBuffer + embeddingComponent
	total = total + total/10

	return total
}

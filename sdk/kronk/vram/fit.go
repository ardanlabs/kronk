package vram

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
		perDevice := result.CalculatePerDevice(deviceCount, constraints.TensorSplit, nil, 0)
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
		full.GPULayers = blockCount + 1
		if input.MoE != nil && input.MoE.IsMoE {
			full.ExpertLayersOnGPU = blockCount
		}
		result := Calculate(full)
		return full.GPULayers, full.ExpertLayersOnGPU, result, AssessFit(result, constraints).Fits
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
		full.GPULayers = blockCount + 1
		if input.MoE != nil && input.MoE.IsMoE {
			full.ExpertLayersOnGPU = blockCount
		}
		return full.GPULayers, full.ExpertLayersOnGPU, Calculate(full), false
	}

	fits := func(v Result) bool { return AssessFit(v, constraints).Fits }

	isMoE := input.MoE != nil && input.MoE.IsMoE && input.Weights != nil

	if isMoE {
		// Expert offloading first.
		bestExperts := int64(-1)
		for experts := blockCount; experts >= 0; experts-- {
			candidate := input
			candidate.GPULayers = blockCount + 1
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
			return blockCount + 1, bestExperts, result, true
		}

		// Fall back to layer offloading.
		for ngl := blockCount + 1; ngl >= 0; ngl-- {
			candidate := input
			candidate.GPULayers = ngl
			if ngl == 0 {
				candidate.GPULayers = -1
			}
			candidate.ExpertLayersOnGPU = max(ngl-1, 0)
			candidate.KVCacheOnCPU = constraints.KVCacheOnCPU
			v := Calculate(candidate)
			if fits(v) {
				return candidate.GPULayers, candidate.ExpertLayersOnGPU, v, true
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
	for ngl := int64(0); ngl <= blockCount+1; ngl++ {
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

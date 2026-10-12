package vram

import "fmt"

// PerDeviceVRAM is the per-GPU breakdown of model weights, KV cache, and
// compute buffer when tensor_split is in effect. The first element is the
// main GPU; compute buffer is reported as fully on the main GPU.
type PerDeviceVRAM struct {
	Label         string
	WeightsBytes  int64
	KVBytes       int64
	ComputeBytes  int64
	MoECacheBytes int64
	TotalBytes    int64
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

// CalculatePerDevice includes the expert-cache reservation in the per-GPU
// estimate. Like weights, the global cache budget is split proportionally;
// actual eligible layer placement and free-memory-based splitting may differ.
func (r Result) CalculatePerDevice(deviceCount int64, tensorSplit []float64, deviceLabels []string, mainGPUIndex int) []PerDeviceVRAM {
	devices := CalculatePerDevice(r.ModelWeightsGPU, r.KVVRAMBytes, r.ComputeBufferEst, deviceCount, tensorSplit, deviceLabels, mainGPUIndex)
	cache := CalculatePerDevice(r.MoECacheBytes, 0, 0, deviceCount, tensorSplit, nil, mainGPUIndex)
	for i := range devices {
		devices[i].MoECacheBytes = cache[i].WeightsBytes
		devices[i].TotalBytes += devices[i].MoECacheBytes
	}
	return devices
}

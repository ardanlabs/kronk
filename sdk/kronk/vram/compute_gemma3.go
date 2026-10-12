package vram

import "strings"

// gemma3ComputeMemory owns the non-SWA Gemma3 graph policy validated with RNJ.
// Keep the formulas local; neither Gemma4 nor generic tuning should alter it.
func gemma3ComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	gpu, cpu := gemma3Workspace(input)
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}

// gemma3Workspace estimates b11513's non-SWA attention graph. Its single host
// mask also has a GPU copy; unlike Gemma4, there is no second SWA mask. Metal
// reserves F16 K/V scratch even for F16 caches. Layers reuse this arena rather
// than multiplying the peak by layer count. Activation/runtime allowances
// approximate the remaining graph costs, not an exact tensor inventory.
func gemma3Workspace(input Input) (gpu, cpu int64) {
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	slots := max(input.Slots, 1)
	batch = min(batch, max(input.ContextWindow, 1)*slots)
	cells := ((max(input.ContextWindow, 1)*slots + 255) / 256) * 256
	streams := slots
	if input.KVUnified {
		streams = 1
	}
	cells = ((cells/streams + 255) / 256) * 256
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	mask := maskWidth * cells * batch
	attention := mask
	if strings.EqualFold(input.ComputeBackend, "metal") && !input.VTransposed && !input.KVCacheOnCPU {
		attention += gemma3MetalAttentionScratch(input, cells, streams)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	logits := 4 * input.VocabularySize * min(rows, batch)
	gpu = gemma3ActivationWorkspace(input, batch) + max(attention, logits)
	if input.VTransposed {
		gpu += 8 * cells * batch * max(input.HeadCount, 1)
	}
	// The host remainder is close to four F32 hidden-width batch slabs.
	// This is a leading-order allowance, not four proven live tensors.
	cpu = mask + 16*input.EmbeddingLength*batch
	return gpu, cpu
}

func gemma3ActivationWorkspace(input Input, batch int64) int64 {
	slotMultiplier := 1.0 + 0.25*float64(max(input.Slots, 1)-1)
	base := int64(8) * batch * input.EmbeddingLength * 4
	return int64(float64(base) * slotMultiplier)
}

// gemma3MetalAttentionScratch charges the largest offloaded layer's K/V
// extent. Both regions use F16 with independent 16-byte alignment regardless
// of the persistent cache type; non-unified streams multiply their extent.
func gemma3MetalAttentionScratch(input Input, cells, streams int64) int64 {
	gpuLayers := max(clampGPULayers(input.GPULayers, input.BlockCount)-1, 0)
	var peak int64
	for layer := input.BlockCount - gpuLayers; layer < input.BlockCount; layer++ {
		heads := input.HeadCountKV
		if layer < int64(len(input.HeadCountKVByLayer)) {
			heads = input.HeadCountKVByLayer[layer]
		}
		kBytes := ((2*input.KeyLength*heads*cells*streams + 15) / 16) * 16
		vBytes := ((2*input.ValueLength*heads*cells*streams + 15) / 16) * 16
		peak = max(peak, kBytes+vBytes)
	}
	return peak
}

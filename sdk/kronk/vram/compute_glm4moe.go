package vram

import "strings"

// glm4moeComputeMemory owns GLM4 MoE's activation, runtime, and margin policy.
// Keep it independent of other MoE architectures and the generic fallback.
func glm4moeComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	gpu, cpu := glm4moeWorkspace(input)
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}

// glm4moeWorkspace estimates b11513's ordinary GQA mask and Metal K/V scratch.
// GLM4 MoE has no recurrent or MLA-compressed state. Dense, routed, and shared
// FFN intermediates are covered approximately by the activation/runtime
// allowances; this is not an exact graph allocation inventory. Layers reuse
// the arena, so layer count does not multiply mask or scratch.
func glm4moeWorkspace(input Input) (gpu, cpu int64) {
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
		attention += glm4moeMetalAttentionScratch(input, cells, streams)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	logits := 4 * input.VocabularySize * min(rows, batch)
	gpu = glm4moeActivationWorkspace(input, batch) + max(attention, logits)
	if input.VTransposed {
		gpu += 8 * cells * batch * max(input.HeadCount, 1)
	}
	// A host mask plus four F32 hidden-width batch slabs matches the
	// leading observed host costs. The slab count is a heuristic.
	cpu = mask + 16*input.EmbeddingLength*batch
	return gpu, cpu
}

func glm4moeActivationWorkspace(input Input, batch int64) int64 {
	slotMultiplier := 1.0 + 0.25*float64(max(input.Slots, 1)-1)
	base := int64(8) * batch * input.EmbeddingLength * 4
	return int64(float64(base) * slotMultiplier)
}

// glm4moeMetalAttentionScratch reserves one layer's independently aligned F16
// K/V extent, irrespective of cache quantization. Ignore appended NextN layers:
// they are not loaded into this target's graph or an implicit draft context.
func glm4moeMetalAttentionScratch(input Input, cells, streams int64) int64 {
	gpuLayers := max(clampGPULayers(input.GPULayers, input.BlockCount)-1, 0)
	trunk := max(input.BlockCount-max(input.NextNPredictLayers, 0), 0)
	var peak int64
	for layer := input.BlockCount - gpuLayers; layer < trunk; layer++ {
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

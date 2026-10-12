package vram

import "strings"

// llamaComputeMemory owns Llama's activation, runtime, and margin policy.
// Keep these formulas local so tuning another architecture cannot change it.
func llamaComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	gpu, cpu := llamaWorkspace(input)
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}

// llamaWorkspace estimates b11513's full-attention GQA graph. A single host
// mask is copied to the GPU. Metal reserves F16 K/V scratch even with an F16
// cache. These buffers are shared across layers, not multiplied by layer count.
// The remaining graph and host-input storage use approximate allowances;
// persistent KV, model residency, and projector workspaces are sized separately.
func llamaWorkspace(input Input) (gpu, cpu int64) {
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
		attention += llamaMetalAttentionScratch(input, cells, streams)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	logits := 4 * input.VocabularySize * min(rows, batch)
	gpu = llamaActivationWorkspace(input, batch) + max(attention, logits)
	if input.VTransposed {
		gpu += 8 * cells * batch * max(input.HeadCount, 1)
	}
	// Four F32 hidden-width batch slabs match the leading observed host
	// remainder; the count is a heuristic, not an exact tensor inventory.
	cpu = mask + 16*input.EmbeddingLength*batch
	return gpu, cpu
}

func llamaActivationWorkspace(input Input, batch int64) int64 {
	slotMultiplier := 1.0 + 0.25*float64(max(input.Slots, 1)-1)
	base := int64(8) * batch * input.EmbeddingLength * 4
	return int64(float64(base) * slotMultiplier)
}

// llamaMetalAttentionScratch uses the largest offloaded layer's K/V extent.
// Independently aligned F16 K/V regions are reserved regardless of cache type;
// non-unified streams multiply the extent, unlike the shared attention mask.
func llamaMetalAttentionScratch(input Input, cells, streams int64) int64 {
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

package vram

import "strings"

// lfm2ComputeMemory owns LFM2's workspace and safety policies. Keep these
// allowances local: tuning another hybrid architecture must not affect LFM2.
func lfm2ComputeMemory(input Input) ComputeMemory {
	// Mask and attention scratch are explicit below. A smaller 64 MiB
	// graph/runtime allowance covers the remaining LFM2 graph approximately.
	const runtime int64 = 64 * 1024 * 1024
	gpu, cpu := lfm2Workspace(input)
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}

// lfm2Workspace estimates a shared attention mask, Metal K/V scratch, and
// short-convolution/hidden activations. Layers reuse the graph arena; layer
// count does not multiply these buffers. Persistent history belongs to cache
// sizing, not this workspace. Activation and host allowances are heuristics.
func lfm2Workspace(input Input) (gpu, cpu int64) {
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
	var scratch int64
	if strings.EqualFold(input.ComputeBackend, "metal") && !input.VTransposed && !input.KVCacheOnCPU {
		scratch = lfm2MetalAttentionScratch(input, cells, streams)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	logits := 4 * input.VocabularySize * min(rows, batch)
	gpu = lfm2ActivationWorkspace(input, batch) + max(mask+scratch, logits)
	if input.VTransposed {
		gpu += 8 * cells * batch * max(input.HeadCount, 1)
	}
	// The observed host remainder is close to four F32 hidden-width
	// batch slabs. This is an allowance, not an exact tensor inventory.
	cpu = mask + 16*input.EmbeddingLength*batch
	return gpu, cpu
}

func lfm2ActivationWorkspace(input Input, batch int64) int64 {
	slotMultiplier := 1.0 + 0.25*float64(max(input.Slots, 1)-1)
	base := int64(8) * batch * input.EmbeddingLength * 4
	return int64(float64(base) * slotMultiplier)
}

// lfm2MetalAttentionScratch uses the largest offloaded attention layer,
// skipping short-convolution layers. Metal reserves independently aligned
// F16 K/V scratch even when the persistent cache is F16 or quantized.
func lfm2MetalAttentionScratch(input Input, cells, streams int64) int64 {
	gpuLayers := max(clampGPULayers(input.GPULayers, input.BlockCount)-1, 0)
	var peak int64
	for layer := input.BlockCount - gpuLayers; layer < input.BlockCount; layer++ {
		if layer < int64(len(input.RecurrentPattern)) && input.RecurrentPattern[layer] {
			continue
		}
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

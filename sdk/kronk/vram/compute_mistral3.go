package vram

import "strings"

// mistral3ComputeMemory owns the Mistral3 activation, runtime, and margin
// policies. Keep them local so tuning this architecture cannot alter others.
func mistral3ComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	gpu, cpu := mistral3Workspace(input)
	memory := ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
	return memory
}

// mistral3Workspace estimates b11513's dense full-attention graph. One host
// causal mask is transferred to the GPU and reused across layers. Metal also
// reserves F16 K/V scratch regardless of cache type. Layers reuse the arena;
// neither mask nor scratch is multiplied by layer count. Activation/runtime
// allowances cover the remaining dense graph approximately, not exactly.
func mistral3Workspace(input Input) (gpu, cpu int64) {
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
		attention += mistral3MetalAttentionScratch(input, cells, streams)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	logits := 4 * input.VocabularySize * min(rows, batch)
	gpu = mistral3ActivationWorkspace(input) + max(attention, logits)
	if input.VTransposed {
		gpu += 8 * cells * batch * max(input.HeadCount, 1)
	}
	// Four F32 hidden-width batch slabs approximate host input and
	// scheduler storage. This is a leading-order allowance, not a claim
	// that exactly four such tensors are simultaneously resident.
	hidden := max(input.OutputEmbeddingLength, input.EmbeddingLength)
	cpu = mask + 16*hidden*batch
	return gpu, cpu
}

func mistral3ActivationWorkspace(input Input) int64 {
	slotMultiplier := 1.0 + 0.25*float64(max(input.Slots, 1)-1)
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	var embeddingComponent int64
	if input.EmbeddingLength > 0 {
		base := int64(8) * batch * input.EmbeddingLength * 4
		embeddingComponent = int64(float64(base) * slotMultiplier)
	}
	return embeddingComponent
}

// mistral3MetalAttentionScratch reserves the largest offloaded layer's K/V
// extent. F16 K and V scratch are each aligned to 16 bytes, independently of
// persistent cache quantization. Non-unified KV streams multiply the extent.
func mistral3MetalAttentionScratch(input Input, cells, streams int64) int64 {
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

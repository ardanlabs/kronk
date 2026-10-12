package vram

import "strings"

// gptossComputeMemory owns GPT-OSS activation, runtime, and margin policies.
// Keep these formulas local so tuning GPT-OSS cannot change other architectures.
func gptossComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	gpu, cpu := gptossWorkspace(input)
	memory := ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
	return memory
}

// gptossWorkspace estimates b11513's alternating full/SWA attention graph.
// Each host mask is also transferred to the GPU. They have no query-head
// multiplier and are shared across layers. Metal's F16 K/V scratch is reserved
// even with F16 caches. Use the larger attention/logits phase; activation and
// runtime allowances approximate routing, FA extras, and allocator overhead.
// Persistent KV and model residency are sized elsewhere.
func gptossWorkspace(input Input) (gpu, cpu int64) {
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
	swaCells := cells
	if input.SlidingWindow > 0 && !input.SWAFull {
		sequences := int64(1)
		if input.KVUnified {
			sequences = slots
		}
		swaCells = ((min(cells, input.SlidingWindow*sequences+batch) + 255) / 256) * 256
	}
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	masks := maskWidth * (cells + swaCells) * batch
	attention := masks
	if strings.EqualFold(input.ComputeBackend, "metal") && !input.VTransposed && !input.KVCacheOnCPU {
		attention += gptossMetalAttentionScratch(input, cells, swaCells, streams)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	logits := 4 * input.VocabularySize * min(rows, batch)
	gpu = gptossActivationWorkspace(input) + max(attention, logits)
	if input.VTransposed {
		gpu += 8 * cells * batch * max(input.HeadCount, 1)
	}
	// Four F32 hidden-width rows per token provide a leading-order host
	// input/routing allowance, not an exact inventory of the MoE schedule.
	hidden := max(input.OutputEmbeddingLength, input.EmbeddingLength)
	cpu = masks + 16*hidden*batch
	return gpu, cpu
}

func gptossActivationWorkspace(input Input) int64 {
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

// gptossMetalAttentionScratch charges the largest offloaded layer, not a
// sum over layers. K/V buffers are F16 and independently aligned to 16 bytes.
func gptossMetalAttentionScratch(input Input, cells, swaCells, streams int64) int64 {
	gpuLayers := max(clampGPULayers(input.GPULayers, input.BlockCount)-1, 0)
	var peak int64
	for layer := input.BlockCount - gpuLayers; layer < input.BlockCount; layer++ {
		swa := layer < input.SlidingWindowLayers
		if int64(len(input.SWAPattern)) == input.BlockCount {
			swa = input.SWAPattern[layer]
		}
		extent := cells
		if swa {
			extent = swaCells
		}
		heads := input.HeadCountKV
		if layer < int64(len(input.HeadCountKVByLayer)) {
			heads = input.HeadCountKVByLayer[layer]
		}
		kBytes := ((2*input.KeyLength*heads*extent*streams + 15) / 16) * 16
		vBytes := ((2*input.ValueLength*heads*extent*streams + 15) / 16) * 16
		peak = max(peak, kBytes+vBytes)
	}
	return peak
}

package vram

import "strings"

// gemma4ComputeMemory owns the Gemma4 activation, runtime-allowance, and margin
// policies. Matching fallback formulas are deliberately kept local so tuning
// another architecture or the generic estimate cannot change Gemma4 sizing.
func gemma4ComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	gpu, cpu := gemma4Workspace(input)
	memory := ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
	return memory
}

// gemma4Workspace estimates the b11513 Gemma4 target/assistant arenas.
// Shared KV does not share compute: both contexts create full and SWA masks.
// With one scheduler copy, transferred GPU masks can be reused by the later
// vocabulary projection. Metal attention additionally reserves F16 K/V scratch,
// even for F16 caches. Use the larger attention/logits phase plus Gemma4's
// activation allowance, not every graph tensor or a per-layer multiplier.
// Backend scheduling/fragmentation make this a heuristic, not an exact peak.
// Model residency and persistent slot state are not part of this workspace.
func gemma4Workspace(input Input) (gpu, cpu int64) {
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	batch = min(batch, max(input.ContextWindow, 1)*max(input.Slots, 1))
	cells := ((max(input.ContextWindow, 1) + 255) / 256) * 256
	swaCells := cells
	if input.SlidingWindow > 0 && !input.SWAFull {
		swaCells = ((min(cells, input.SlidingWindow+batch) + 255) / 256) * 256
	}
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	masks := maskWidth * (cells + swaCells) * batch
	logits := 4 * input.VocabularySize * batch
	attention := masks
	if strings.EqualFold(input.ComputeBackend, "metal") && !input.VTransposed && !input.KVCacheOnCPU {
		attention += gemma4MetalAttentionScratch(input, cells, swaCells)
	}
	gpu = gemma4ActivationWorkspace(input) + max(attention, logits)
	if input.VTransposed {
		// Ordinary attention materializes KQ and softmax, unlike FA.
		gpu += 8 * cells * batch * max(input.HeadCount, 1)
	}
	hidden := max(input.OutputEmbeddingLength, input.EmbeddingLength)
	cpu = masks + 4*hidden*batch
	return gpu, cpu
}

// gemma4ActivationWorkspace retains the current Gemma4 activation estimate.
// Use the configured batch capacity, not the graph's context-clamped query
// count. The slot multiplier remains a heuristic, independent of KV state.
func gemma4ActivationWorkspace(input Input) int64 {
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

// gemma4MetalAttentionScratch sizes b11513's always-reserved F16 K/V workspace
// (ggml-metal-ops.cpp:3358-3378). Layers reuse the arena, so only the largest
// offloaded layer matters. Shared-KV assistant layers still execute attention
// and reserve scratch; they must not be excluded with the cache-sharing layers.
func gemma4MetalAttentionScratch(input Input, cells, swaCells int64) int64 {
	streams := max(input.Slots, 1)
	if input.KVUnified {
		streams = 1
	}
	gpuLayers := max(clampGPULayers(input.GPULayers, input.BlockCount)-1, 0)
	var peak int64
	for layer := input.BlockCount - gpuLayers; layer < input.BlockCount; layer++ {
		swa := layer < input.SlidingWindowLayers
		if int64(len(input.SWAPattern)) == input.BlockCount {
			swa = input.SWAPattern[layer]
		}
		heads, key, value, extent := input.HeadCountKV, input.KeyLength, input.ValueLength, cells
		if swa && input.SlidingWindow > 0 {
			heads = input.SWAHeadCountKV
			if heads <= 0 {
				heads = input.HeadCountKV
			}
			key, value = input.SWAKeyLength, input.SWAValueLength
			if key <= 0 {
				key = input.KeyLength
			}
			if value <= 0 {
				value = input.ValueLength
			}
			extent = swaCells
		}
		if layer < int64(len(input.HeadCountKVByLayer)) && input.HeadCountKVByLayer[layer] > 0 {
			heads = input.HeadCountKVByLayer[layer]
		}
		// Each K and V scratch region is F16 and separately aligned to
		// 16 bytes, regardless of the configured cache quantization.
		kBytes := ((2*key*heads*extent*streams + 15) / 16) * 16
		vBytes := ((2*value*heads*extent*streams + 15) / 16) * 16
		peak = max(peak, kBytes+vBytes)
	}
	return peak
}

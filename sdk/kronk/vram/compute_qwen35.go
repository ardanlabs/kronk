package vram

import "strings"

// qwen35ComputeMemory owns the Qwen3.5 dense/MoE hybrid workspace policy.
// Activation, runtime, and margin formulas deliberately remain local: tuning
// another architecture or the generic fallback must not change Qwen3.5 sizing.
func qwen35ComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	gpu, cpu := qwen35Workspace(input)
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}

// qwen35Workspace sizes the leading b11513 attention tensors. The host's F16
// causal mask is also transferred to the GPU. Metal reserves F16 K/V scratch
// even with F16 caches; layers reuse that arena. Logits use Kronk's output-row
// cap, not every prefill token. Recurrent operations use chunked scratch rather
// than full-context masks; activation/runtime allowances cover that remaining
// workspace approximately. Persistent recurrent state belongs to slot memory.
func qwen35Workspace(input Input) (gpu, cpu int64) {
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	batch = min(batch, max(input.ContextWindow, 1)*max(input.Slots, 1))
	cells := ((max(input.ContextWindow, 1) + 255) / 256) * 256
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	mask := maskWidth * cells * batch
	attention := mask
	if strings.EqualFold(input.ComputeBackend, "metal") && !input.VTransposed && !input.KVCacheOnCPU {
		attention += qwen35MetalAttentionScratch(input, cells)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	logits := 4 * input.VocabularySize * min(rows, batch)
	gpu = qwen35ActivationWorkspace(input) + max(attention, logits)
	if input.VTransposed {
		gpu += 8 * cells * batch * max(input.HeadCount, 1)
	}
	// The target also carries recurrent/hidden input rows. MTP has token
	// and hidden inputs but no target recurrent graph. These are leading
	// host allowances, not an exact inventory of every scheduled tensor.
	hiddenRows := int64(4)
	if input.MTPContext {
		hiddenRows = 2
	}
	hidden := max(input.OutputEmbeddingLength, input.EmbeddingLength)
	cpu = mask + hiddenRows*4*hidden*batch
	return gpu, cpu
}

func qwen35ActivationWorkspace(input Input) int64 {
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

// qwen35MetalAttentionScratch charges the largest offloaded attention layer,
// excluding recurrent layers and selecting either the trunk or appended MTP.
// K and V scratch are independently aligned F16 buffers, regardless of the
// persistent cache's quantization. Parallel KV streams multiply their extent.
func qwen35MetalAttentionScratch(input Input, cells int64) int64 {
	streams := max(input.Slots, 1)
	if input.KVUnified {
		streams = 1
	}
	gpuLayers := max(clampGPULayers(input.GPULayers, input.BlockCount)-1, 0)
	trunk := max(input.BlockCount-input.NextNPredictLayers, 0)
	start, end := input.BlockCount-gpuLayers, trunk
	if input.MTPContext {
		start, end = max(start, trunk), input.BlockCount
	}
	var peak int64
	for layer := start; layer < end; layer++ {
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

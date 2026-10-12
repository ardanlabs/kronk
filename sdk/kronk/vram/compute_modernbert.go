package vram

import "strings"

// modernbertComputeMemory owns ModernBERT's encoder workspace and margins.
// Keep this policy independent of Clef and autoregressive architectures.
func modernbertComputeMemory(input Input) ComputeMemory {
	// This heuristic covers remaining graph/bookkeeping storage, including
	// Metal's temporary attention states and the optional decision head.
	// It is not a llama.cpp constant or a measured worst-case bound.
	const runtime int64 = 32 * 1024 * 1024
	gpu, cpu := modernbertWorkspace(input)
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}

// modernbertWorkspace sizes the no-cache encoder against its physical batch.
// Global and local attention share graph storage across layers, but both masks
// coexist. The local mask is still batch squared, not batch times SWA window.
// Hidden-state and host slabs are allowances, not an exact tensor inventory.
func modernbertWorkspace(input Input) (gpu, cpu int64) {
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	batch = min(batch, max(input.ContextWindow, 1)*max(input.Slots, 1))
	maskCount := int64(1)
	if input.SlidingWindow > 0 {
		maskCount = 2
	}
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	mask := maskCount * maskWidth * batch * batch
	attention := mask
	if strings.EqualFold(input.ComputeBackend, "metal") && !input.VTransposed {
		// Metal reserves F16 K/V scratch for the current batch even
		// when its graph K/V tensors are already F16. No cache is retained.
		kBytes := ((2*input.KeyLength*input.HeadCountKV*batch + 15) / 16) * 16
		vBytes := ((2*input.ValueLength*input.HeadCountKV*batch + 15) / 16) * 16
		attention += kBytes + vBytes
	}
	if input.VTransposed {
		attention += 8 * batch * batch * max(input.HeadCount, 1)
	}
	slotMultiplier := 1.0 + 0.25*float64(max(input.Slots, 1)-1)
	activation := int64(float64(32*batch*input.EmbeddingLength) * slotMultiplier)
	// ModernBERT returns embeddings or decision scores, not vocabulary
	// logits. Do not size an output projection against VocabularySize.
	gpu = activation + attention
	cpu = mask + 16*input.EmbeddingLength*batch
	return gpu, cpu
}

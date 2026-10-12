package vram

// qwen4expComputeMemory owns the Qwen4Exp activation, runtime-allowance, and
// margin policies. Matching fallback formulas are deliberately kept local so
// tuning another architecture or the generic estimate cannot change Qwen sizing.
func qwen4expComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	gpu, cpu := qwen4expWorkspace(input)
	memory := ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
	return memory
}

// qwen4expWorkspace estimates the dominant b11513 QSA tensors, not a
// per-layer arena: the scheduler reuses scratch storage between layers.
// Two dense selection/scatter masks and F32 pooled scores live on the GPU.
// Input causal/pool masks, pool indices, and hidden-state rows live on the host.
// Actual peaks depend on backend scheduling; qwen4expComputeMemory applies its
// compute margin to this leading-order tensor inventory.
// Model residency and persistent slot state are not part of this workspace.
func qwen4expWorkspace(input Input) (gpu, cpu int64) {
	gpu = qwen4expActivationWorkspace(input)
	if input.IndexerKeyLength <= 0 || input.IndexerCompression <= 0 {
		return gpu, 0
	}
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	cells := ((max(input.ContextWindow, 1) + 255) / 256) * 256
	ratio := input.IndexerCompression
	// Dense masks use one stream's extent; pools span all sequence streams.
	pools := max(64, ((cells/ratio*max(input.Slots, 1)+1+63)/64)*64)
	selected := min(pools, max(input.IndexerTopK, 0)/ratio)*ratio + ratio - 1
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	gpu += maskWidth*(2*cells+selected)*batch + 4*pools*batch
	if input.VTransposed {
		// Without flash attention, KQ and softmax are F32 and have a query
		// head dimension. Use the normalized query head count when known.
		gpu += 8 * cells * batch * max(input.HeadCount, 1)
	}
	hiddenWidth := max(input.OutputEmbeddingLength, input.EmbeddingLength)
	cpu = maskWidth*cells*batch + 2*pools*batch + 4*ratio*pools + 4*hiddenWidth*batch
	return gpu, cpu
}

// qwen4expActivationWorkspace retains the current Qwen4Exp activation estimate.
// The slot multiplier is an activation heuristic, separate from indexer and
// recurrent state, and can be tuned independently of other architectures.
func qwen4expActivationWorkspace(input Input) int64 {
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

package vram

// genericComputeMemory is the fallback for architectures without a validated
// compute model. Its policies are independent of the dedicated architecture
// estimates, even where the initial formulas match.
func genericComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	workspace := genericActivationWorkspace(input)
	memory := ComputeMemory{
		WorkspaceGPU:        workspace,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (workspace + runtime) / 10,
	}
	return memory
}

// genericActivationWorkspace approximates activation storage for unvalidated
// architectures. Its slot multiplier is a heuristic, not persistent slot state.
func genericActivationWorkspace(input Input) int64 {
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

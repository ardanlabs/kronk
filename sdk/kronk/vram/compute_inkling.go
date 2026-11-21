package vram

// inklingComputeMemory estimates b12606's relative-position attention graph.
// Gathering the bias and making its permuted layout contiguous requires two
// F32 heads × cache-cells × query tensors, even with flash attention. Layers
// reuse this arena: neither physical nor repeated loop layers multiply it.
// Masks and I32 relative-position inputs coexist for full and SWA caches.
// Other activations, scheduler reuse and fragmentation remain heuristic; this
// is a planning estimate, not a guarantee that a native decode will succeed.
func inklingComputeMemory(input Input) ComputeMemory {
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	slots := max(input.Slots, 1)
	batch = min(batch, max(input.ContextWindow, 1)*slots)
	capacity := max(input.ContextWindow, 1)
	sequences := int64(1)
	if input.KVUnified {
		capacity *= slots
		sequences = slots
	}
	cells := ((capacity + 255) / 256) * 256
	swaCells := cells
	if input.SlidingWindow > 0 && !input.SWAFull {
		swaCells = ((min(cells, input.SlidingWindow*sequences+batch) + 255) / 256) * 256
	}
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	inputs := (4 + maskWidth) * (cells + swaCells) * batch
	bias := 8 * cells * batch * max(input.HeadCount, 1)
	if input.VTransposed {
		// Non-flash attention also materializes KQ and softmax.
		bias += 8 * cells * batch * max(input.HeadCount, 1)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	logits := 4 * input.VocabularySize * min(rows, batch)
	// The remaining activation allowance is independent of the generic policy.
	activations := int64(float64(32*batch*input.EmbeddingLength) * (1 + 0.25*float64(slots-1)))
	gpu := activations + max(inputs+bias, logits)
	// Host masks are F32, relative positions I32, plus hidden-state inputs.
	cpu := 8*(cells+swaCells)*batch + 4*input.EmbeddingLength*batch
	runtime := int64(256 * 1024 * 1024)
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}

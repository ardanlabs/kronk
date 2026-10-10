package vram

// EstimateComputeBuffer provides a heuristic estimate of the compute buffer
// VRAM needed during inference. This is inherently approximate.
//
// The estimate scales with NSeqMax (Input.Slots) because llama.cpp grows
// its logical batch and per-sequence bookkeeping (KQ masks, position
// buffers, per-token logits) as more parallel slots are configured. The
// scaling is sub-linear; the multiplier 1 + 0.25*(slots-1) yields 1.0,
// 1.25, 1.5, 1.75, 2.0 for slots 1..5, matching observed llama-server
// growth at --parallel 1/2/4 within ~10% on the models we have measured.
func EstimateComputeBuffer(input Input) int64 {
	const (
		baseBufferSmall = 256 * 1024 * 1024 // 256 MiB for models < 100B params
		baseBufferLarge = 512 * 1024 * 1024 // 512 MiB for models >= 100B params
		k               = 8                 // empirical multiplier
	)

	baseBuffer := int64(baseBufferSmall)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		baseBuffer = int64(baseBufferLarge)
	}

	slots := max(input.Slots, 1)
	slotMultiplier := 1.0 + 0.25*float64(slots-1)
	nUBatch := input.NUBatch
	if nUBatch <= 0 {
		nUBatch = DefaultNUBatch
	}

	var embeddingComponent int64
	if input.EmbeddingLength > 0 {
		base := int64(k) * nUBatch * input.EmbeddingLength * 4
		embeddingComponent = int64(float64(base) * slotMultiplier)
	}

	total := baseBuffer + embeddingComponent
	total = total + total/10

	return total
}

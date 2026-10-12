package vram

// calculateModelResidency places weights independently of execution-slot state
// and workspaces. Architecture-specific weight facts come from normalized Input;
// dense and MoE models reuse the same placement rules.
func calculateModelResidency(input Input, offloadLayers int64) (activeGPU, activeCPU, expertsGPU, expertsCPU int64) {
	activeTotal := input.ModelSizeBytes
	if input.Weights != nil && input.MoE != nil && input.MoE.IsMoE {
		expertsGPU, expertsCPU = calculateExpertWeights(input, max(offloadLayers-1, 0))
		activeTotal = input.Weights.AlwaysActiveBytes
	}
	activeGPU, activeCPU = calculateActiveWeights(input, activeTotal, offloadLayers)
	return activeGPU, activeCPU, expertsGPU, expertsCPU
}

// calculateActiveWeights sizes all dense weights, or the always-active portion
// of MoE weights. Both layouts share input/output and block placement rules.
func calculateActiveWeights(input Input, activeTotal, offloadLayers int64) (gpu, cpu int64) {
	// Only block weights use the proportional approximation. Input tensors
	// stay on CPU; output tensors move as a unit at any positive offload.
	var inputBytes, outputBytes, tiedBytes int64

	if input.Weights != nil {
		inputBytes = input.Weights.InputBytes
		outputBytes = input.Weights.OutputBytes
		if input.TiedOutputFallback && !input.Weights.HasOutputWeight {
			tiedBytes = input.Weights.TokenEmbeddingBytes
		}
	}

	if inputBytes == 0 && outputBytes == 0 {
		// Metadata-only callers have no tensor placement facts. Treat
		// output as one of the N+1 weight groups, rather than free offload.
		outputBytes = activeTotal / max(input.BlockCount+1, 1)
	}

	blockBytes := max(activeTotal-inputBytes-outputBytes, 0)
	gpu, cpu = splitByGPULayers(blockBytes, max(offloadLayers-1, 0), input.BlockCount)
	cpu += inputBytes

	if offloadLayers > 0 {
		gpu += outputBytes + tiedBytes
	} else {
		// Tied output on the same CPU buffer aliases the input table.
		cpu += outputBytes
	}

	return gpu, cpu
}

// clampGPULayers counts output plus repeating layers, matching native n_gpu_layers.
// A zero value selects full offload; -1 explicitly selects CPU-only.
func clampGPULayers(gpuLayers, blockCount int64) int64 {
	if gpuLayers == -1 {
		return 0
	}
	if gpuLayers <= 0 || gpuLayers > blockCount+1 {
		return blockCount + 1
	}

	return gpuLayers
}

// splitByGPULayers splits totalBytes proportionally between GPU and CPU based
// on how many layers are offloaded.
func splitByGPULayers(totalBytes, gpuLayers, blockCount int64) (gpu, cpu int64) {
	if blockCount <= 0 {
		return totalBytes, 0
	}

	gpu = (gpuLayers * totalBytes) / blockCount
	cpu = max(0, totalBytes-gpu)

	return gpu, cpu
}

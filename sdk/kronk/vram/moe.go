package vram

import "math/big"

// calculateExpertWeights sizes routed MoE experts independently of the shared
// always-active weights. CPU expert overrides cannot put CPU-layer experts on GPU.
func calculateExpertWeights(input Input, gpuLayers int64) (gpu, cpu int64) {
	// The GGUF analyzer's per-tensor byte accounting does not
	// survive some non-standard quantizations (notably the MXFP4
	// packing used by gpt-oss), so Weights.ExpertBytesTotal can
	// undercount what's actually on disk by an order of magnitude.
	// Fall back to "file size minus always-active" as the honest
	// expert footprint and rescale the per-layer breakdown so
	// expert-offload math still produces sensible numbers. For
	// well-analyzed models (most quants) the rescale factor is 1
	// and behavior is unchanged.
	expertsTotal := input.Weights.ExpertBytesTotal
	if t := input.ModelSizeBytes - input.Weights.AlwaysActiveBytes; t > expertsTotal {
		expertsTotal = t
	}
	perLayerExpert := input.Weights.ExpertBytesByLayer
	if expertsTotal != input.Weights.ExpertBytesTotal && len(perLayerExpert) > 0 {
		perLayerExpert = scaledPerLayer(perLayerExpert, input.Weights.ExpertBytesTotal, expertsTotal)
	}

	if input.ExpertLayersOnGPU > 0 && len(perLayerExpert) > 0 {
		blockCount := int64(len(perLayerExpert))
		startLayer := max(blockCount-min(input.ExpertLayersOnGPU, gpuLayers), 0)
		for i := startLayer; i < blockCount; i++ {
			gpu += perLayerExpert[i]
		}
	}
	cpu = max(0, expertsTotal-gpu)

	return gpu, cpu
}

// scaledPerLayer rescales perLayer so its sum equals newTotal. Used
// when the analyzer's per-tensor byte accounting undercounts the file
// (unrecognised quantizations) but the per-layer ratios are still
// meaningful. When origTotal is zero perLayer is returned unchanged so
// the caller can decide what to do.
//
// The intermediate b*newTotal product can exceed int64 for large MoE
// models (e.g. ~800 MiB per-layer * ~36 GB total ≈ 3e19), so the
// multiplication is performed with math/big to avoid silent overflow.
func scaledPerLayer(perLayer []int64, origTotal, newTotal int64) []int64 {
	if len(perLayer) == 0 || origTotal <= 0 || origTotal == newTotal {
		return perLayer
	}

	bigNew := big.NewInt(newTotal)
	bigOrig := big.NewInt(origTotal)
	tmp := new(big.Int)

	scaled := make([]int64, len(perLayer))
	for i, b := range perLayer {
		tmp.SetInt64(b)
		tmp.Mul(tmp, bigNew)
		tmp.Quo(tmp, bigOrig)
		scaled[i] = tmp.Int64()
	}

	return scaled
}

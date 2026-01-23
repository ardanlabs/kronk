package models

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

var kvCacheBytesPerElement = map[string]float64{
	"fp32":    4.0,
	"fp16":    2.0,
	"bf16":    2.0,
	"fp8":     1.0,
	"q8_0":    1.0,
	"q8":      1.0,
	"q6_k":    0.75,
	"q5_1":    0.6875,
	"q5_0":    0.625,
	"q5_k":    0.625,
	"q4_1":    0.5625,
	"q4_0":    0.5,
	"q4_k":    0.5,
	"q4":      0.5,
	"q3_k":    0.4375,
	"q2_k":    0.3125,
	"iq4_nl":  0.5,
	"iq4_xs":  0.5,
	"iq3_xxs": 0.375,
	"iq3_s":   0.375,
	"iq2_xxs": 0.25,
	"iq2_xs":  0.25,
	"iq2_s":   0.25,
	"iq1_s":   0.1875,
	"iq1_m":   0.1875,
}

var llamaLikeModelArch = []string{
	"qwen", "qwen2", "qwen3", "qwen3moe", "mistral", "starcoder", "codellama",
}

func CalculateVRAM(metadata model.Metadata, contextLength uint64, kvCacheType string) uint64 {
	alignment := metadata.Alignment
	if alignment == 0 {
		alignment = 32
	}

	arch := metadata.Architecture

	switch arch {
	case model.ARCH_LLAMA:
		return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
	case model.ARCH_MPT:
		return calculateMPTVRAM(metadata, contextLength, alignment, kvCacheType)
	case model.ARCH_GPTNEOX:
		return calculateGPTNeoXVRAM(metadata, contextLength, alignment, kvCacheType)
	case model.ARCH_GPTJ:
		return calculateGPTJVRAM(metadata, contextLength, alignment, kvCacheType)
	case model.ARCH_GPT2:
		return calculateGPT2VRAM(metadata, contextLength, alignment, kvCacheType)
	case model.ARCH_BLOOM:
		return calculateBLOOMVRAM(metadata, contextLength, alignment, kvCacheType)
	case model.ARCH_FALCON:
		return calculateFALCONVRAM(metadata, contextLength, alignment, kvCacheType)
	case model.ARCH_MAMBA:
		return calculateMAMBAVRAM(metadata, contextLength, alignment)
	case model.ARCH_RWKV:
		return calculateRWKVVRAM(metadata, contextLength, alignment)
	default:
		archStr := string(arch)
		for _, prefix := range llamaLikeModelArch {
			if strings.HasPrefix(archStr, prefix) {
				return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
			}
		}

		return calculateDefaultVRAM(metadata, contextLength, alignment, kvCacheType)
	}
}

func calculateLlamaVRAM(metadata model.Metadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	nLayers := metadata.BlockCount
	nHeads := metadata.HeadCount
	nHeadsKV := metadata.HeadCountKV
	embd := metadata.EmbeddingLength

	if nHeadsKV == 0 {
		nHeadsKV = nHeads
	}

	var dHead uint64 = 128
	if nHeads > 0 && embd > 0 {
		dHead = embd / nHeads
	}

	modelWeightsVRAM := metadata.FileSize

	bytesPerElement, ok := kvCacheBytesPerElement[strings.ToLower(kvCacheType)]
	if !ok {
		bytesPerElement = 2.0 // default to FP16
	}

	// KV cache size: 2 (K and V) * nLayers * nHeadsKV * dHead * contextLength * bytesPerElement
	kvCacheBytes := uint64(float64(2*nLayers*nHeadsKV*dHead*contextLength) * bytesPerElement)

	overhead := modelWeightsVRAM / 10

	totalVRAM := modelWeightsVRAM + kvCacheBytes + overhead

	return alignVRAM(totalVRAM, uint64(alignment))
}

func calculateMPTVRAM(metadata model.Metadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateGPTNeoXVRAM(metadata model.Metadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateGPTJVRAM(metadata model.Metadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateGPT2VRAM(metadata model.Metadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateBLOOMVRAM(metadata model.Metadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateFALCONVRAM(metadata model.Metadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateMAMBAVRAM(metadata model.Metadata, contextLength uint64, alignment uint32) uint64 {
	// Mamba doesn't have traditional KV cache, just state
	modelWeightsVRAM := metadata.FileSize
	overhead := modelWeightsVRAM / 10

	// Mamba state is much smaller than KV cache
	nLayers := metadata.BlockCount
	stateSize := metadata.FeedForwardLength
	if stateSize == 0 {
		stateSize = 16
	}
	stateBytes := nLayers * stateSize * 2

	return alignVRAM(modelWeightsVRAM+stateBytes+overhead, uint64(alignment))
}

func calculateRWKVVRAM(metadata model.Metadata, contextLength uint64, alignment uint32) uint64 {
	modelWeightsVRAM := metadata.FileSize
	overhead := modelWeightsVRAM / 10

	// RWKV has fixed state regardless of context length
	nLayers := metadata.BlockCount
	embd := metadata.EmbeddingLength
	stateBytes := nLayers * embd * 5 * 2

	return alignVRAM(modelWeightsVRAM+stateBytes+overhead, uint64(alignment))
}

// calculateDefaultVRAM calculates VRAM for unknown architectures
func calculateDefaultVRAM(metadata model.Metadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func getBytesPerWeight(metadata model.Metadata) float64 {
	fileTypeBytes := map[uint32]float64{
		0:  4.0,    // ALL_F32
		1:  2.0,    // MOSTLY_F16
		2:  0.5,    // MOSTLY_Q4_0
		3:  0.5625, // MOSTLY_Q4_1
		7:  1.0,    // MOSTLY_Q8_0
		8:  0.625,  // MOSTLY_Q5_0
		9:  0.6875, // MOSTLY_Q5_1
		10: 0.3125, // MOSTLY_Q2_K
		11: 0.375,  // MOSTLY_Q3_K_S
		12: 0.4375, // MOSTLY_Q3_K_M
		13: 0.5,    // MOSTLY_Q3_K_L
		14: 0.5,    // MOSTLY_Q4_K_S
		15: 0.5625, // MOSTLY_Q4_K_M
		16: 0.625,  // MOSTLY_Q5_K_S
		17: 0.6875, // MOSTLY_Q5_K_M
		18: 0.8125, // MOSTLY_Q6_K
	}

	if bpw, exists := fileTypeBytes[metadata.FileType]; exists {
		return bpw
	}

	return 2.0
}

// alignVRAM aligns VRAM usage to the specified alignment
func alignVRAM(vram, alignment uint64) uint64 {
	if alignment == 0 {
		alignment = 32
	}
	return ((vram + alignment - 1) / alignment) * alignment
}

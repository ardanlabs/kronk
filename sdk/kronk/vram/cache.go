package vram

import (
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// calculateSlotState sizes persistent attention, indexer, and recurrent state
// using the shared GGUF allocator formulas and normalized architecture facts.
// This reports both per-slot and total state; it excludes model residency and
// shared execution workspace. Weight layout (dense or MoE) does not affect it.
func calculateSlotState(input Input) gguf.KVCache {
	if input.ComputeKind == modelprofile.ComputeModernBERT {
		// b11513 runs ModernBERT's encoder with no persistent memory
		// module. K/V tensors are temporary graph intermediates.
		return gguf.KVCache{}
	}
	if input.ComputeKind == modelprofile.ComputeClef {
		// b11513 creates no memory module for Clef. Its attention and
		// zero-initialized recurrent states live only in the compute graph.
		return gguf.KVCache{}
	}
	kvInput := gguf.KVCacheInput{
		ContextWindow:        input.ContextWindow,
		BlockCount:           input.BlockCount,
		HeadCountKV:          input.HeadCountKV,
		KeyLength:            input.KeyLength,
		ValueLength:          input.ValueLength,
		SWAHeadCountKV:       input.SWAHeadCountKV,
		SWAKeyLength:         input.SWAKeyLength,
		SWAValueLength:       input.SWAValueLength,
		HeadCountKVByLayer:   input.HeadCountKVByLayer,
		BytesPerElement:      input.BytesPerElement,
		TypeK:                input.TypeK,
		TypeV:                input.TypeV,
		Slots:                input.Slots,
		SlidingWindow:        input.SlidingWindow,
		SlidingWindowLayers:  input.SlidingWindowLayers,
		SharedKVLayers:       input.SharedKVLayers,
		SWAPattern:           input.SWAPattern,
		RecurrentPattern:     input.RecurrentPattern,
		NextNPredictLayers:   input.NextNPredictLayers,
		MTPContext:           input.MTPContext,
		IndexerKeyLength:     input.IndexerKeyLength,
		RecurrentStateBytes:  input.RecurrentStateBytes,
		RecurrentStateCopies: input.RecurrentStateCopies,
		NUBatch:              input.NUBatch,
		KVUnified:            input.KVUnified,
		SWAFull:              input.SWAFull,
		VTransposed:          input.VTransposed,
	}

	kv := gguf.CalculateKVCache(kvInput)

	if !input.MTPContext && input.ComputeContexts > 1 && input.NextNPredictLayers > 0 {
		// Embedded own-KV MTP allocates attention state for the appended
		// layers in its draft context, separately from the target trunk.
		draftInput := kvInput
		draftInput.MTPContext = true
		draft := gguf.CalculateKVCache(draftInput)
		for layer := max(input.BlockCount-input.NextNPredictLayers, 0); layer < int64(len(kv.LayerMemory)); layer++ {
			kv.LayerMemory[layer] = draft.LayerMemory[layer]
			kv.SlotMemory += draft.LayerMemory[layer]
		}
		kv.KVPerSlot = kv.SlotMemory / max(input.Slots, 1)
	}

	return kv
}

// placeCache assigns per-layer attention/recurrent state to the same device
// as its transformer block, unless all cache is explicitly kept on CPU.
func placeCache(input Input, kv gguf.KVCache, gpuLayers int64) (gpu, cpu int64) {
	gpuLayerStart := input.BlockCount - gpuLayers
	for layer, bytes := range kv.LayerMemory {
		if !input.KVCacheOnCPU && int64(layer) >= gpuLayerStart {
			gpu += bytes
		} else {
			cpu += bytes
		}
	}

	return gpu, cpu
}

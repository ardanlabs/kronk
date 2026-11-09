package vram

import "github.com/ardanlabs/kronk/sdk/kronk/gguf"

// kimiCache follows the K-only MLA allocator and cell-aligned indexer cache.
// IndexerSources names the owning layer; consumers reuse its selection and
// allocate no additional indexer keys. KDA state remains F32 at every KV type.
func kimiCache(input Input) gguf.KVCache {
	main := input
	main.ValueLength, main.SWAValueLength, main.IndexerKeyLength = 0, 0, 0
	main.VTransposed = false
	kv := genericCache(main)
	if input.IndexerKeyLength <= 0 {
		return kv
	}
	index := main
	index.BlockCount, index.HeadCountKV, index.KeyLength = 1, 1, input.IndexerKeyLength
	index.HeadCountKVByLayer, index.RecurrentPattern = nil, nil
	index.RecurrentStateBytes, index.NextNPredictLayers, index.SharedKVLayers = 0, 0, 0
	index.MTPContext = false
	indexKV := genericCache(index)
	for layer := range kv.LayerMemory {
		if layer < len(input.RecurrentPattern) && input.RecurrentPattern[layer] {
			continue
		}
		if len(input.IndexerSources) == len(kv.LayerMemory) && input.IndexerSources[layer] != int64(layer) {
			continue
		}
		kv.LayerMemory[layer] += indexKV.SlotMemory
		kv.SlotMemory += indexKV.SlotMemory
	}
	kv.KVPerSlot = kv.SlotMemory / max(input.Slots, 1)
	return kv
}

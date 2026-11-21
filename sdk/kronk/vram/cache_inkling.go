package vram

import "github.com/ardanlabs/kronk/sdk/kronk/gguf"

// inklingCache sizes independent loop slots with both attention and convolution
// history, while the physical block count continues to describe shared weights.
func inklingCache(input Input) gguf.KVCache {
	input.BlockCount = cacheBlockCount(input)
	kv := genericCache(input)
	// Convolution coexists with attention in every cache slot, unlike the
	// recurrent-only layers represented by RecurrentPattern in other models.
	state := input.RecurrentStateBytes * max(input.RecurrentStateCopies, 1) * max(input.Slots, 1)
	for layer := range kv.LayerMemory {
		kv.LayerMemory[layer] += state
		kv.SlotMemory += state
	}
	kv.KVPerSlot = kv.SlotMemory / max(input.Slots, 1)
	return kv
}

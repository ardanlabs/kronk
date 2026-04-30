package gguf

// KVCacheInput contains the parameters needed to size the per-slot and
// total KV cache footprint for a transformer model. It deliberately
// excludes MoE / weight-breakdown / compute-buffer concerns; those live
// in the higher-level VRAM calculator in sdk/tools/models.
type KVCacheInput struct {
	ContextWindow   int64 // n_ctx - context window size in tokens.
	BlockCount      int64 // n_layers - number of transformer layers.
	HeadCountKV     int64 // Number of KV attention heads (averaged for hybrid archs).
	KeyLength       int64 // K dimension per head (typically 128).
	ValueLength     int64 // V dimension per head (typically 128).
	BytesPerElement int64 // Per-element width of the KV cache type (q8_0=1, f16=2, ...).
	Slots           int64 // n_seq_max - number of concurrent sequences.
}

// KVCache holds the KV-cache sizing breakdown produced by CalculateKVCache.
type KVCache struct {
	KVPerTokenPerLayer int64 // Bytes per token per layer.
	KVPerSlot          int64 // Bytes per slot (full context for one sequence).
	SlotMemory         int64 // Total KV cache memory in bytes (KVPerSlot * Slots).
}

// CalculateKVCache returns the per-token, per-slot, and total KV cache
// memory footprint for a given input. This is the pure formula shared by
// the SDK's diagnostic VRAM estimator and the tools/models VRAM
// calculator; it has no I/O or hardware dependencies.
func CalculateKVCache(input KVCacheInput) KVCache {
	kvPerTokenPerLayer := input.HeadCountKV * (input.KeyLength + input.ValueLength) * input.BytesPerElement
	kvPerSlot := input.ContextWindow * input.BlockCount * kvPerTokenPerLayer
	slotMemory := input.Slots * kvPerSlot

	return KVCache{
		KVPerTokenPerLayer: kvPerTokenPerLayer,
		KVPerSlot:          kvPerSlot,
		SlotMemory:         slotMemory,
	}
}

package gguf

// KVCacheInput contains the parameters needed to size the per-slot and
// total KV cache footprint for a transformer model. It deliberately
// excludes MoE / weight-breakdown / compute-buffer concerns; those live
// in the higher-level VRAM calculator in sdk/tools/models.
//
// SWA-aware accounting (sliding-window attention) is opt-in via the two
// SlidingWindow* fields. Architectures like gemma3/gemma4 mix a small
// number of "global" full-context attention layers with a larger number
// of sliding-window layers whose KV state is bounded by SlidingWindow,
// not ContextWindow. Without these two fields, every layer is budgeted
// for full ContextWindow KV — a several-gigabyte over-estimate at long
// contexts and the cause of unloadable AGENT-style configs.
type KVCacheInput struct {
	ContextWindow       int64 // n_ctx - context window size in tokens.
	BlockCount          int64 // n_layers - number of transformer layers.
	HeadCountKV         int64 // Number of KV attention heads (averaged for hybrid archs).
	KeyLength           int64 // K dimension per head (typically 128).
	ValueLength         int64 // V dimension per head (typically 128).
	BytesPerElement     int64 // Per-element width of the KV cache type (q8_0=1, f16=2, ...).
	Slots               int64 // n_seq_max - number of concurrent sequences.
	SlidingWindow       int64 // Sliding-window size in tokens (0 = no SWA layers).
	SlidingWindowLayers int64 // Layer count that uses SWA (0 = treat all BlockCount as full attention).
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
//
// When SlidingWindow > 0 and 0 < SlidingWindowLayers <= BlockCount, the
// SWA layers are budgeted at min(SlidingWindow, ContextWindow) tokens
// while the remaining (BlockCount - SlidingWindowLayers) layers keep
// full ContextWindow KV. This matches how llama.cpp actually allocates
// the cache for gemma3/gemma4-style hybrid attention models.
func CalculateKVCache(input KVCacheInput) KVCache {
	kvPerTokenPerLayer := input.HeadCountKV * (input.KeyLength + input.ValueLength) * input.BytesPerElement

	swaLayers := input.SlidingWindowLayers
	if swaLayers < 0 {
		swaLayers = 0
	}
	if swaLayers > input.BlockCount {
		swaLayers = input.BlockCount
	}

	fullLayers := input.BlockCount - swaLayers

	swaCtx := input.SlidingWindow
	if swaCtx <= 0 || swaLayers == 0 {
		// No SWA → every layer pays full ContextWindow.
		swaCtx = 0
		swaLayers = 0
		fullLayers = input.BlockCount
	}
	if swaCtx > input.ContextWindow {
		swaCtx = input.ContextWindow
	}

	fullKVPerSlot := input.ContextWindow * fullLayers * kvPerTokenPerLayer
	swaKVPerSlot := swaCtx * swaLayers * kvPerTokenPerLayer
	kvPerSlot := fullKVPerSlot + swaKVPerSlot

	slotMemory := input.Slots * kvPerSlot

	return KVCache{
		KVPerTokenPerLayer: kvPerTokenPerLayer,
		KVPerSlot:          kvPerSlot,
		SlotMemory:         slotMemory,
	}
}

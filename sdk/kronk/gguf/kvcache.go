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
	ContextWindow        int64 // n_ctx - context window size in tokens.
	BlockCount           int64 // n_layers - number of transformer layers.
	HeadCountKV          int64 // Number of KV attention heads (averaged for hybrid archs).
	KeyLength            int64 // K dimension per head (typically 128).
	ValueLength          int64 // V dimension per head (typically 128).
	SWAHeadCountKV       int64 // Number of KV heads in SWA layers (0 = HeadCountKV).
	SWAKeyLength         int64 // K dimension in SWA layers (0 = KeyLength).
	SWAValueLength       int64 // V dimension in SWA layers (0 = ValueLength).
	BytesPerElement      int64 // Per-element width of the KV cache type (q8_0=1, f16=2, ...).
	TypeK                int32 // Kronk GGML cache type ID (0 = use BytesPerElement).
	TypeV                int32 // Kronk GGML cache type ID (0 = use BytesPerElement).
	HeadCountKVByLayer   []int64
	Slots                int64 // n_seq_max - number of concurrent sequences.
	SlidingWindow        int64 // Sliding-window size in tokens (0 = no SWA layers).
	SlidingWindowLayers  int64 // Layer count that uses SWA (0 = treat all BlockCount as full attention).
	SharedKVLayers       int64 // Trailing layers that reuse another layer's KV tensors.
	SWAPattern           []bool
	RecurrentPattern     []bool // Per-layer recurrent classification; recurrent layers do not allocate K/V.
	NextNPredictLayers   int64  // Appended MTP layers excluded from the target context memory.
	RecurrentStateBytes  int64  // F32 recurrent state bytes per recurrent layer, sequence, and state copy.
	RecurrentStateCopies int64  // Current state plus rollback snapshots retained per sequence.
	NUBatch              int64  // Effective physical batch size used by the SWA allocator.
	KVUnified            bool   // Whether all sequence slots share one KV pool.
	SWAFull              bool   // Whether SWA layers allocate the full context.
	VTransposed          bool   // Whether V uses the model-wide transposed layout.
}

// KVCache holds the KV-cache sizing breakdown produced by CalculateKVCache.
type KVCache struct {
	KVPerTokenPerLayer int64 // Legacy unquantized-width diagnostic.
	KVPerSlot          int64 // Average bytes per configured sequence slot.
	SlotMemory         int64 // Total allocated KV cache memory in bytes.
	LayerMemory        []int64
}

// CalculateKVCache returns the per-token, per-slot, and total KV cache
// memory footprint for a given input. This is the pure formula shared by
// the SDK's diagnostic VRAM estimator and the tools/models VRAM
// calculator; it has no I/O or hardware dependencies.
//
// The calculation follows llama.cpp's allocation layout: context and SWA
// cell padding, physical-batch headroom, unified/non-unified sequence pools,
// per-layer hybrid dimensions, GGML row sizes, transposed V layout, and shared
// KV layers.
func CalculateKVCache(input KVCacheInput) KVCache {
	kvPerTokenPerLayer := input.HeadCountKV * (input.KeyLength + input.ValueLength) * input.BytesPerElement
	swaHeadCountKV := input.SWAHeadCountKV
	if swaHeadCountKV <= 0 {
		swaHeadCountKV = input.HeadCountKV
	}
	swaKeyLength := input.SWAKeyLength
	if swaKeyLength <= 0 {
		swaKeyLength = input.KeyLength
	}
	swaValueLength := input.SWAValueLength
	if swaValueLength <= 0 {
		swaValueLength = input.ValueLength
	}
	blockCount := max(input.BlockCount, 0)
	allocatedLayers := blockCount - min(max(input.SharedKVLayers, 0), blockCount)
	pattern := layerPattern(input, blockCount)

	totalContext := pad(input.ContextWindow*input.Slots, 256)
	baseCells := totalContext
	streams := int64(1)
	if !input.KVUnified && input.Slots > 0 {
		baseCells = pad(totalContext/input.Slots, 256)
		streams = input.Slots
	}
	fullCells := baseCells * streams
	swaCells := fullCells
	if input.SlidingWindow > 0 && !input.SWAFull {
		sequences := int64(1)
		if input.KVUnified {
			sequences = input.Slots
		}
		swaCells = pad(min(baseCells, input.SlidingWindow*sequences+max(input.NUBatch, 0)), 256) * streams
	}
	maxValueWidth := maxKVValueWidth(input, pattern, allocatedLayers, swaHeadCountKV, swaValueLength)
	trunkLayers := max(blockCount-max(input.NextNPredictLayers, 0), 0)
	recurrentCopies := max(input.RecurrentStateCopies, 1)
	recurrentStreams := max(input.Slots, 1)

	layerMemory := make([]int64, allocatedLayers)
	var slotMemory int64
	for layer := range allocatedLayers {
		if layer >= trunkLayers {
			continue
		}
		if input.RecurrentStateBytes > 0 && isRecurrentLayer(input.RecurrentPattern, layer, blockCount) {
			layerMemory[layer] = input.RecurrentStateBytes * recurrentCopies * recurrentStreams
			slotMemory += layerMemory[layer]
			continue
		}

		headCountKV := layerHeadCount(input.HeadCountKVByLayer, layer, input.HeadCountKV)
		keyLength := input.KeyLength
		valueLength := input.ValueLength
		cells := fullCells
		if pattern[layer] && input.SlidingWindow > 0 {
			headCountKV = layerHeadCount(input.HeadCountKVByLayer, layer, swaHeadCountKV)
			keyLength = swaKeyLength
			valueLength = swaValueLength
			cells = swaCells
		}

		keyWidth := headCountKV * keyLength
		valueWidth := headCountKV * valueLength
		layerMemory[layer] = cacheBytes(input.TypeK, input.BytesPerElement, keyWidth, cells, false)
		if input.VTransposed {
			valueWidth = maxValueWidth
		}
		layerMemory[layer] += cacheBytes(input.TypeV, input.BytesPerElement, valueWidth, cells, input.VTransposed)
		slotMemory += layerMemory[layer]
	}

	kvPerSlot := slotMemory
	if input.Slots > 0 {
		kvPerSlot /= input.Slots
	}

	return KVCache{
		KVPerTokenPerLayer: kvPerTokenPerLayer,
		KVPerSlot:          kvPerSlot,
		SlotMemory:         slotMemory,
		LayerMemory:        layerMemory,
	}
}

func maxKVValueWidth(input KVCacheInput, pattern []bool, blockCount, swaHeadCountKV, swaValueLength int64) int64 {
	var maxWidth int64
	for layer := range blockCount {
		if layer >= max(input.BlockCount-max(input.NextNPredictLayers, 0), 0) ||
			input.RecurrentStateBytes > 0 && isRecurrentLayer(input.RecurrentPattern, layer, input.BlockCount) {
			continue
		}
		headCountKV := layerHeadCount(input.HeadCountKVByLayer, layer, input.HeadCountKV)
		valueLength := input.ValueLength
		if pattern[layer] && input.SlidingWindow > 0 {
			headCountKV = layerHeadCount(input.HeadCountKVByLayer, layer, swaHeadCountKV)
			valueLength = swaValueLength
		}
		maxWidth = max(maxWidth, headCountKV*valueLength)
	}
	return maxWidth
}

func isRecurrentLayer(pattern []bool, layer, blockCount int64) bool {
	return int64(len(pattern)) == blockCount && pattern[layer]
}

func layerHeadCount(values []int64, layer int64, fallback int64) int64 {
	if layer < int64(len(values)) && values[layer] > 0 {
		return values[layer]
	}
	return fallback
}

func cacheBytes(typeID int32, bytesPerElement, width, cells int64, transposed bool) int64 {
	if typeID == 0 {
		return width * cells * bytesPerElement
	}

	ggmlType := uint32(typeID)
	if typeID == 50 {
		ggmlType = 0
	}
	if transposed {
		return GGMLRowSize(ggmlType, cells) * width
	}
	return GGMLRowSize(ggmlType, width) * cells
}

func layerPattern(input KVCacheInput, blockCount int64) []bool {
	if int64(len(input.SWAPattern)) == blockCount {
		return input.SWAPattern
	}

	pattern := make([]bool, blockCount)
	swaLayers := min(max(input.SlidingWindowLayers, 0), blockCount)
	for layer := range swaLayers {
		pattern[layer] = true
	}
	return pattern
}

func pad(value, alignment int64) int64 {
	if value <= 0 || alignment <= 0 {
		return value
	}
	return ((value + alignment - 1) / alignment) * alignment
}

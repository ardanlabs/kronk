package models

import (
	"fmt"
	"strconv"
)

// VRAMConfig contains the user-provided parameters for VRAM calculation
// that cannot be extracted from the model file.
type VRAMConfig struct {
	ContextWindow   int64 // n_ctx - context window size (e.g., 8192, 131072)
	BytesPerElement int64 // Depends on cache type: q8_0=1, f16=2
	Slots           int64 // n_seq_max - number of concurrent sequences
	CacheSequences  int64 // Additional sequences for caching: 0=none, 1=FMC, 2=FMC+SPC
}

// VRAM contains the calculated VRAM requirements.
type VRAM struct {
	KVPerTokenPerLayer int64 // Bytes per token per layer
	KVPerSlot          int64 // Bytes per slot
	TotalSlots         int64 // Slots + CacheSequences
	SlotMemory         int64 // Total KV cache memory in bytes
	TotalVRAM          int64 // Model size + slot memory in bytes
}

// CalculateVRAM retrieves model metadata and computes the VRAM requirements.
func (m *Models) CalculateVRAM(modelID string, cfg VRAMConfig) (VRAM, error) {
	info, err := m.ModelInformation(modelID)
	if err != nil {
		return VRAM{}, fmt.Errorf("failed to retrieve model info: %w", err)
	}

	arch := detectArchitecture(info.Metadata)
	if arch == "" {
		return VRAM{}, fmt.Errorf("unable to detect model architecture")
	}

	blockCount, err := parseMetadataInt64(info.Metadata, arch+".block_count")
	if err != nil {
		return VRAM{}, fmt.Errorf("failed to parse block_count: %w", err)
	}

	headCountKV, err := parseMetadataInt64(info.Metadata, arch+".attention.head_count_kv")
	if err != nil {
		return VRAM{}, fmt.Errorf("failed to parse head_count_kv: %w", err)
	}

	keyLength, err := parseMetadataInt64(info.Metadata, arch+".attention.key_length")
	if err != nil {
		return VRAM{}, fmt.Errorf("failed to parse key_length: %w", err)
	}

	valueLength, err := parseMetadataInt64(info.Metadata, arch+".attention.value_length")
	if err != nil {
		return VRAM{}, fmt.Errorf("failed to parse value_length: %w", err)
	}

	input := VRAMInput{
		ModelSizeBytes:  int64(info.Size),
		ContextWindow:   cfg.ContextWindow,
		BlockCount:      blockCount,
		HeadCountKV:     headCountKV,
		KeyLength:       keyLength,
		ValueLength:     valueLength,
		BytesPerElement: cfg.BytesPerElement,
		Slots:           cfg.Slots,
		CacheSequences:  cfg.CacheSequences,
	}

	return CalculateVRAM(input), nil
}

func detectArchitecture(metadata map[string]string) string {
	if arch, ok := metadata["general.architecture"]; ok {
		return arch
	}
	return ""
}

// =============================================================================

// VRAMInput contains all parameters needed to calculate VRAM requirements.
type VRAMInput struct {
	ModelSizeBytes  int64 // Size of model weights in bytes
	ContextWindow   int64 // n_ctx - context window size (e.g., 8192, 131072)
	BlockCount      int64 // n_layers - number of transformer layers
	HeadCountKV     int64 // Number of KV attention heads
	KeyLength       int64 // K dimension per head (typically 128)
	ValueLength     int64 // V dimension per head (typically 128)
	BytesPerElement int64 // Depends on cache type: q8_0=1, f16=2
	Slots           int64 // n_seq_max - number of concurrent sequences
	CacheSequences  int64 // Additional sequences for caching: 0=none, 1=FMC, 2=FMC+SPC
}

// CalculateVRAM computes the VRAM requirements for running a model based on
// the provided input parameters.
func CalculateVRAM(input VRAMInput) VRAM {

	// Calculate bytes needed per token in each transformer layer.
	// Formula: head_count_kv × (key_length + value_length) × bytes_per_element
	// Example: 4 × (128 + 128) × 1 = 1024 bytes
	kvPerTokenPerLayer := input.HeadCountKV * (input.KeyLength + input.ValueLength) * input.BytesPerElement

	// Calculate total KV cache memory per slot (sequence).
	// Formula: n_ctx × n_layers × kv_per_token_per_layer
	// Example: 131072 × 48 × 1024 = ~6.4 GB
	kvPerSlot := input.ContextWindow * input.BlockCount * kvPerTokenPerLayer

	// Total sequences = user slots + cache sequences (FMC adds 1, FMC+SPC adds 2).
	totalSlots := input.Slots + input.CacheSequences

	// Total KV cache memory allocated at model load time.
	// Formula: total_slots × kv_per_slot
	// Example: 4 × 6.4GB = ~25.6 GB
	slotMemory := totalSlots * kvPerSlot

	// Total VRAM = model weights + KV cache memory.
	// Example: 36GB + 25.6GB = ~61.6 GB
	totalVRAM := input.ModelSizeBytes + slotMemory

	return VRAM{
		KVPerTokenPerLayer: kvPerTokenPerLayer,
		KVPerSlot:          kvPerSlot,
		TotalSlots:         totalSlots,
		SlotMemory:         slotMemory,
		TotalVRAM:          totalVRAM,
	}
}

// =============================================================================

func parseMetadataInt64(metadata map[string]string, key string) (int64, error) {
	val, ok := metadata[key]
	if !ok {
		return 0, fmt.Errorf("metadata key %q not found", key)
	}
	return strconv.ParseInt(val, 10, 64)
}

// =============================================================================
/*
	SLOT MEMORY AND TOTAL VRAM COST FORMULA

	These figures are for KV cache VRAM only (when offload-kqv: true).
	Model weights require additional VRAM: ~7GB (7B Q8) or ~70GB (70B Q8).
	Total VRAM = model weights + KV cache.

	Memory is statically allocated upfront when the model loads,
	based on n_ctx × n_seq_max. Reserving slots consumes memory whether or not
	they're actually used.

	Example Calculations:

	This is how you calculate the amount of KV memory you need per slot.

	KV_Per_Token_Per_Layer = head_count_kv × (key_length + value_length) × bytes_per_element
	KV_Per_Slot            = n_ctx × n_layers × KV_per_token_per_layer

	------------------------------------------------------------------------------
	So Given these values, this is what you are looking at:

	Model   Context_Window   KV_Per_Slot      NSeqMax (Slots)
	7B      8K               ~537 MB VRAM     2
	70B     8K               ~1.3 GB VRAM     2

	No Caching:
	Total sequences allocated: 2 (no cache)
	7B:  Slot Memory (2 × 537MB) ~1.07GB: Total VRAM: ~8.1GB
	70B: Slot Memory (2 × 1.3GB) ~2.6GB : Total VRAM: ~72.6GB

	First Memory Caching (FMC):
	Total sequences allocated: 2 + 1 = 3 (cache)
	7B:  Slot Memory (3 × 537MB) ~1.6GB: Total VRAM: ~8.6GB
	70B: Slot Memory (3 × 1.3GB) ~3.9GB: Total VRAM: ~73.9GB

	Both SPC and FMC:
	Total sequences allocated: 2 + 2 = 4 (cache)
	7B:  Slot Memory (4 × 537MB) ~2.15GB: Total VRAM: ~9.2GB
	70B: Slot Memory (4 × 1.3GB) ~5.2GB:  Total VRAM: ~75.2GB

	------------------------------------------------------------------------------
	Full Example With Real Model:

	Model                   : Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
	Size                    : 36.0GB
	Context Window          : 131072 (128k)
	cache-type-k            : q8_0 (1 byte per element), f16 (2 bytes)
	cache-type-v            : q8_0 (1 byte per element), f16 (2 bytes)
	block_count             : 48  (n_layers)
	attention.head_count_kv : 4   (KV heads)
	attention.key_length    : 128	(K dimension per head)
	attention.value_length  : 128	(V dimension per head)

	KV_per_token_per_layer = head_count_kv  ×  (key_length + value_length)  ×  bytes_per_element
	1024 bytes             =             4  ×  ( 128       +         128 )  ×  1

	KV_Per_Slot            =  n_ctx  ×  n_layers  ×  KV_per_token_per_layer
	~6.4 GB                =  131072 ×  48        ×  1024

	No Caching:
	Total sequences allocated: 2 : (no cache)
	Slot Memory (2 × 6.4GB) ~12.8GB: Total VRAM: ~48.8GB

	First Memory Caching (FMC):
	Total sequences allocated: 3 : (2 + 1) (1 cache sequence)
	Slot Memory (3 × 6.4GB) ~19.2GB: Total VRAM: ~55.2GB

	Both SPC and FMC:
	Total sequences allocated: 4 : (2 + 2) (2 cache sequences)
	Slot Memory (4 × 6.4GB) ~25.6GB: Total VRAM: ~61.6GB
*/

// Package vram provides VRAM requirement calculation for GGUF models.
// Calculate combines weight placement, cache/state memory, and compute
// buffers. Model-specific formulas stay separate from hardware fitting
// and the local/HuggingFace input helpers.
package vram

import "github.com/ardanlabs/kronk/sdk/kronk/gguf"

// Context window size constants (in tokens).
const (
	ContextWindow1K   int64 = 1024
	ContextWindow2K   int64 = 2048
	ContextWindow4K   int64 = 4096
	ContextWindow8K   int64 = 8192
	ContextWindow16K  int64 = 16384
	ContextWindow32K  int64 = 32768
	ContextWindow64K  int64 = 65536
	ContextWindow128K int64 = 131072
	ContextWindow256K int64 = 262144
)

// DefaultNUBatch is the physical batch capacity used when a caller does not
// provide an effective NUBatch.
const DefaultNUBatch int64 = 2 * 1024

// Bytes per element constants for KV cache types. Re-exported from
// sdk/kronk/gguf so callers don't need a second import.
const (
	BytesPerElementF32  = gguf.BytesPerElementF32
	BytesPerElementF16  = gguf.BytesPerElementF16
	BytesPerElementBF16 = gguf.BytesPerElementBF16
	BytesPerElementQ8_0 = gguf.BytesPerElementQ8_0
	BytesPerElementQ4_0 = gguf.BytesPerElementQ4_0
	BytesPerElementQ4_1 = gguf.BytesPerElementQ4_1
	BytesPerElementQ5_0 = gguf.BytesPerElementQ5_0
	BytesPerElementQ5_1 = gguf.BytesPerElementQ5_1
)

// Slot count constants.
const (
	Slots1 int64 = 1
	Slots2 int64 = 2
	Slots3 int64 = 3
	Slots4 int64 = 4
	Slots5 int64 = 5
)

// Config contains the user-provided parameters for VRAM calculation
// that cannot be extracted from the model file.
type Config struct {
	ContextWindow          int64 // n_ctx - context window size (e.g., 8192, 131072)
	BytesPerElement        int64 // Depends on cache type: q8_0=1, f16=2
	TypeK                  int32 // Kronk GGML cache type ID (0 = use BytesPerElement).
	TypeV                  int32 // Kronk GGML cache type ID (0 = use BytesPerElement).
	Slots                  int64 // n_seq_max - number of concurrent sequences
	NUBatch                int64 // Effective physical batch size.
	GPULayers              int64 // Native offload count: output first, then blocks (0 = all, -1 = none).
	ExpertLayersOnGPU      int64 // 0 = all experts on CPU.
	MoECacheSize           int64 // GPU expert-cache budget in bytes per context; ignored for dense models.
	KVCacheOnCPU           bool  // Move KV cache off the GPU (offload-kqv: false).
	SWAFull                bool  // Size SWA layers against the full context window.
	VTransposed            bool  // Size V using the model-wide transposed layout.
	RecurrentStateCopies   int64 // State copies for separate-model or companion-MTP speculation.
	EmbeddedMTPStateCopies int64 // State copies when the GGUF contains an embedded MTP layer.
	ComputeContexts        int64 // Native contexts that reserve independent compute buffers.
}

// Input contains all parameters needed to calculate VRAM requirements.
type Input struct {
	ModelSizeBytes       int64                 // Size of model weights in bytes
	ContextWindow        int64                 // n_ctx - context window size (e.g., 8192, 131072)
	BlockCount           int64                 // n_layers - number of transformer layers
	HeadCountKV          int64                 // Number of KV attention heads
	KeyLength            int64                 // K dimension per head (typically 128)
	ValueLength          int64                 // V dimension per head (typically 128)
	SWAHeadCountKV       int64                 // Number of KV heads in SWA layers (0 = HeadCountKV)
	SWAKeyLength         int64                 // K dimension in SWA layers (0 = KeyLength)
	SWAValueLength       int64                 // V dimension in SWA layers (0 = ValueLength)
	HeadCountKVByLayer   []int64               // Exact per-layer KV head counts when available.
	BytesPerElement      int64                 // Depends on cache type: q8_0=1, f16=2
	TypeK                int32                 // Kronk GGML cache type ID (0 = use BytesPerElement).
	TypeV                int32                 // Kronk GGML cache type ID (0 = use BytesPerElement).
	Slots                int64                 // n_seq_max - number of concurrent sequences
	SlidingWindow        int64                 // Sliding-window size in tokens (0 = no SWA layers).
	SlidingWindowLayers  int64                 // Layer count using SWA (0 = treat all BlockCount as full attention).
	SharedKVLayers       int64                 // Trailing layers that reuse another layer's KV tensors.
	SWAPattern           []bool                // Per-layer SWA classification when available.
	RecurrentPattern     []bool                // Per-layer recurrent classification when available.
	NextNPredictLayers   int64                 // Appended MTP layers excluded from target context memory.
	RecurrentStateBytes  int64                 // State bytes per recurrent layer, sequence, and copy.
	RecurrentStateCopies int64                 // Current state plus speculative rollback copies.
	NUBatch              int64                 // Effective physical batch size.
	KVUnified            bool                  // Whether all sequence slots share one KV pool.
	EmbeddingLength      int64                 // needed for compute buffer estimate
	ComputeContexts      int64                 // Native contexts that reserve independent compute buffers.
	MoE                  *gguf.MoEInfo         //
	Weights              *gguf.WeightBreakdown //
	TiedOutputFallback   bool                  // Architecture duplicates token embeddings for missing output weights.
	GPULayers            int64                 // Native offload count: output first, then blocks (0 = all, -1 = none).
	ExpertLayersOnGPU    int64                 // 0 = all experts on CPU
	MoECacheSize         int64                 // GPU expert-cache budget in bytes per context.
	KVCacheOnCPU         bool                  // Move KV cache off the GPU (offload-kqv: false)
	SWAFull              bool                  // Size SWA layers against the full context window.
	VTransposed          bool                  // Size V using the model-wide transposed layout.
}

// Result contains the calculated VRAM requirements.
type Result struct {
	Input              Input // Input parameters used for calculation
	KVPerTokenPerLayer int64 // Bytes per token per layer
	KVPerSlot          int64 // Bytes per slot
	SlotMemory         int64 // Total KV cache memory in bytes
	TotalVRAM          int64 // Model size + slot memory in bytes
	MoE                *gguf.MoEInfo
	Weights            *gguf.WeightBreakdown
	ModelWeightsGPU    int64
	ModelWeightsCPU    int64
	ComputeBufferEst   int64
	MoECacheBytes      int64 // Additional GPU expert-cache reservation; original host weights remain.

	// Always-active weights include all dense weights; expert fields are MoE-only.
	AlwaysActiveGPUBytes int64
	AlwaysActiveCPUBytes int64
	ExpertGPUBytes       int64
	ExpertCPUBytes       int64

	// KV cache placement and total system RAM estimate. When
	// Input.KVCacheOnCPU is true, KVCPUBytes == SlotMemory and
	// KVVRAMBytes == 0; otherwise the inverse.
	KVVRAMBytes       int64
	KVCPUBytes        int64
	TotalSystemRAMEst int64

	// Per-device breakdown (populated by CalculatePerDevice when
	// tensor_split / device_count are passed by the caller).
	PerDevice []PerDeviceVRAM
}

// Calculate computes the VRAM requirements for running a model based on
// the provided input parameters. The KV cache portion of the math is
// delegated to sdk/kronk/gguf.CalculateKVCache so the SDK and tools sides
// share a single implementation.
func Calculate(input Input) Result {
	kv := calculateCache(input)
	offloadLayers := clampGPULayers(input.GPULayers, input.BlockCount)

	// b11513 offloads output first; it has no attention/recurrent state.
	gpuLayers := max(offloadLayers-1, 0)

	var expertsGPU, expertsCPU int64
	activeTotal := input.ModelSizeBytes
	if input.Weights != nil && input.MoE != nil && input.MoE.IsMoE {
		expertsGPU, expertsCPU = calculateExpertWeights(input, gpuLayers)
		activeTotal = input.Weights.AlwaysActiveBytes
	}

	alwaysActiveGPU, alwaysActiveCPU := calculateActiveWeights(input, activeTotal, offloadLayers)
	modelWeightsGPU := alwaysActiveGPU + expertsGPU
	modelWeightsCPU := alwaysActiveCPU + expertsCPU

	computeContexts := max(input.ComputeContexts, 1)
	computeBufferEst := EstimateComputeBuffer(input) * computeContexts

	var moeCacheBytes int64
	if input.MoE != nil && input.MoE.IsMoE {
		moeCacheBytes = max(input.MoECacheSize, 0)
	} else {
		input.MoECacheSize = 0
	}

	kvVRAMBytes, kvCPUBytes := placeCache(input, kv, gpuLayers)

	totalVRAM := modelWeightsGPU + kvVRAMBytes + computeBufferEst + moeCacheBytes
	totalSystemRAMEst := modelWeightsCPU + kvCPUBytes

	return Result{
		Input:                input,
		KVPerTokenPerLayer:   kv.KVPerTokenPerLayer,
		KVPerSlot:            kv.KVPerSlot,
		SlotMemory:           kv.SlotMemory,
		TotalVRAM:            totalVRAM,
		MoE:                  input.MoE,
		Weights:              input.Weights,
		ModelWeightsGPU:      modelWeightsGPU,
		ModelWeightsCPU:      modelWeightsCPU,
		ComputeBufferEst:     computeBufferEst,
		MoECacheBytes:        moeCacheBytes,
		AlwaysActiveGPUBytes: alwaysActiveGPU,
		AlwaysActiveCPUBytes: alwaysActiveCPU,
		ExpertGPUBytes:       expertsGPU,
		ExpertCPUBytes:       expertsCPU,
		KVVRAMBytes:          kvVRAMBytes,
		KVCPUBytes:           kvCPUBytes,
		TotalSystemRAMEst:    totalSystemRAMEst,
	}
}

// UnifiedFootprint returns the bytes a model occupies once every page of
// the GGUF is resident in unified memory. This is the value the resman
// reserves and the BUI displays on Apple Silicon (and any future
// unified-memory platform), where the MoE-aware GPU/CPU split that
// Calculate produces does not correspond to a real physical separation.
//
// The formula intentionally uses the raw model bytes (Input.ModelSizeBytes)
// rather than ModelWeightsGPU+ModelWeightsCPU so a model whose GGUF
// analyzer is missing the MoE expert breakdown still reserves the full
// file. SlotMemory, ComputeBufferEst, and MoECacheBytes round out the footprint.
func (r Result) UnifiedFootprint() int64 {
	return r.Input.ModelSizeBytes + r.SlotMemory + r.ComputeBufferEst + r.MoECacheBytes
}

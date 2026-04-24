package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

/*
Workload							NBatch		NUBatch		Rationale
Interactive chat (single user)		512–1024	512			Low latency; small batches
Long prompts/RAG					2048–4096	512–1024	Faster prompt ingestion
Batch inference (multiple prompts)	2048–4096	512			Higher throughput
Low VRAM (<8GB)						512			256–512		Avoid OOM
High VRAM (24GB+)					4096+		1024+		Maximize parallelism

Key principles:
- NUBatch ≤ NBatch always (you already enforce this at line 139)
- NUBatch primarily affects prompt processing speed; keep it ≤512 for stability on most consumer GPUs
- NBatch closer to ContextWindow improves throughput but uses more VRAM
- Powers of 2 are slightly more efficient on most hardware
*/

const (
	NUMADisabled   = ""
	NUMADistribute = "distribute"
	NUMAIsolate    = "isolate"
	NUMANumactl    = "numactl"
	NUMAMirror     = "mirror"
)

const (
	defContextWindow    = 8 * 1024
	defNBatch           = 2 * 1024
	defNUBatch          = 512
	defNUBatchVision    = 2 * 1024
	defMinCacheTokens   = 100
	defThreadZero       = 0
	defNSeqMax          = 1
	defNDraft           = 5
	defCacheSlotTimeout = 30
)

// Logger provides a function for logging messages from different APIs.
type Logger func(ctx context.Context, msg string, args ...any)

// =============================================================================

// intOr returns the value pointed to by p, or def if p is nil.
func intOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// boolOr returns the value pointed to by p, or def if p is nil.
func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// float32Or returns the value pointed to by p, or def if p is nil.
func float32Or(p *float32, def float32) float32 {
	if p == nil {
		return def
	}
	return *p
}

// =============================================================================

// DraftModelConfig configures a draft model for speculative decoding. A smaller,
// faster model generates candidate tokens that the target model verifies in a
// single forward pass. This can improve generation throughput when the draft
// model's predictions frequently match the target's.
//
// Requirements:
//   - Draft and target models must share the same vocabulary (same tokenizer)
//   - NSeqMax must be 1 (single-slot mode)
//   - Draft model should be significantly smaller than the target (e.g., 0.6B draft for 8B target)
type DraftModelConfig struct {
	ModelFiles    []string  // Path to the draft model GGUF file(s)
	NDraft        int       // Number of tokens to draft per step (default 5)
	PtrNGpuLayers *int      // GPU layers for draft model (nil = all layers on GPU)
	Devices       []string  // Devices for draft model (e.g., ["CUDA0"])
	PtrMainGPU    *int      // Primary GPU index for draft model
	TensorSplit   []float32 // Per-device tensor split for draft model
}

func (d DraftModelConfig) NGpuLayers() int { return intOr(d.PtrNGpuLayers, 0) }
func (d DraftModelConfig) MainGPU() int    { return intOr(d.PtrMainGPU, 0) }

// Config represents model level configuration. These values if configured
// incorrectly can cause the system to panic. The defaults are used when these
// values are set to 0.
//
// CacheMinTokens sets the minimum token count required before caching. Messages
// shorter than this threshold are not cached, as the overhead of cache management
// may outweigh the prefill savings. When set to 0, defaults to 100 tokens.
//
// CacheSlotTimeout sets the maximum number of seconds used for two IMC timeout
// scenarios:
//
//  1. Wait for slot availability: When all IMC slots have cache builds
//     in-flight (pending), incoming requests wait up to this duration for a
//     slot to become available before returning a "server busy" error.
//
//  2. Slot preemption: When a request's target slot is busy generating,
//     the request is deferred. If the deferred job waits longer than this
//     duration (measured from batch queue entry, not HTTP arrival), the
//     busy slot is preempted so the waiting request can proceed.
//
// When set to 0, defaults to 30 seconds.
//
// CacheTypeK is the data type for the K (key) cache. This controls the precision
// of the key vectors in the KV cache. Lower precision types (like Q8_0 or Q4_0)
// reduce memory usage but may slightly affect quality. When left as the zero
// value (GGMLTypeAuto), the default llama.cpp value is used.
//
// CacheTypeV is the data type for the V (value) cache. This controls the precision
// of the value vectors in the KV cache. When left as the zero value (GGMLTypeAuto),
// the default llama.cpp value is used.
//
// Cataloger provides catalog config and template retrieval. If not set,
// a default catalog will be created by kronk.New.
//
// ContextWindow (often referred to as context length) is the maximum number of
// tokens that a large language model can process and consider at one time when
// generating a response. It defines the model's effective "memory" for a single
// conversation or text generation task.
// When set to 0, the default value is 4096.
//
// DefaultParams contains the default sampling parameters for requests.
//
// Devices is a list of device names to use for model execution. When multiple
// devices are specified, the model is distributed across them according to the
// SplitMode and TensorSplit configuration. Device names can be obtained from
// the output of llama-bench --list-devices (e.g., "CUDA0", "CUDA1", "Metal").
// When empty and Device is also empty, the default device selection is used.
//
// FlashAttention controls Flash Attention mode. Flash Attention reduces memory
// usage and speeds up attention computation, especially for large context windows.
// When left as zero value, FlashAttentionEnabled is used (default on).
// Set to FlashAttentionDisabled to disable, or FlashAttentionAuto to let llama.cpp decide.
//
// IncrementalCache enables Incremental Message Caching (IMC) for agentic
// workflows. It caches all messages except the last one (which triggers
// generation) and extends the cache incrementally on each turn. This is ideal
// for agents like Cline or OpenCode where conversations grow monotonically.
// The cache is rebuilt from scratch when the message prefix changes (new thread).
//
// InsecureLogging enables logging of potentially sensitive data such as message
// content. This should only be enabled for debugging purposes in non-production
// environments.
//
// JinjaFile is the path to the jinja file. This is not required and can be
// used if you want to override the templated provided by the model metadata.
//
// Log is the logger to use for model operations.
//
// ModelFiles is the path to the model files. This is mandatory to provide.
//
// NBatch is the logical batch size or the maximum number of tokens that can be
// in a single forward pass through the model at any given time.  It defines
// the maximum capacity of the processing batch. If you are processing a very
// long prompt or multiple prompts simultaneously, the total number of tokens
// processed in one go will not exceed NBatch. Increasing n_batch can improve
// performance (throughput) if your hardware can handle it, as it better
// utilizes parallel computation. However, a very high n_batch can lead to
// out-of-memory errors on systems with limited VRAM.
// When set to 0, the default value is 2048.
//
// MainGPU is the index of the GPU to use as the primary device when SplitMode
// is SplitModeNone. When nil, the default GPU (usually index 0) is used.
//
// NGpuLayers is the number of model layers to offload to the GPU. When set to 0,
// all layers are offloaded (default). Set to -1 to keep all layers on CPU. Any
// positive value specifies the exact number of layers to offload.
//
// NSeqMax controls concurrency behavior based on model type. For text inference
// models (including vision/audio), it sets the maximum number of sequences
// processed in parallel within the batch engine. For embedding and reranking
// models, it sets the number of contexts in the internal pool for parallel
// request processing. When set to 0, a default of 1 is used.
//
// NThreads is the number of threads to use for generation. When set to 0, the
// default llama.cpp value is used.
//
// NThreadsBatch is the number of threads to use for batch processing. When set
// to 0, the default llama.cpp value is used.
//
// NUBatch is the physical batch size or the maximum number of tokens processed
// together during the initial prompt processing phase (also called "prompt
// ingestion") to populate the KV cache. It specifically optimizes the initial
// loading of prompt tokens into the KV cache. If a prompt is longer than
// NUBatch, it will be broken down and processed in chunks of n_ubatch tokens
// sequentially. This parameter is crucial for tuning performance on specific
// hardware (especially GPUs) because different values might yield better prompt
// processing times depending on the memory architecture.
// When set to 0, the default value is 512.
//
// OffloadKQV controls whether the KV cache is offloaded to the GPU. When nil or
// true, the KV cache is stored on the GPU (default behavior). Set to false to
// keep the KV cache on the CPU, which reduces VRAM usage but may slow inference.
//
// OpOffload controls whether host tensor operations are offloaded to the device
// (GPU). When nil or true, operations are offloaded (default behavior). Set to
// false to keep operations on the CPU.
//
// ProjFile is the path to the projection files. This is mandatory for media
// based models like vision and audio.
//
// QueueDepth sets the multiplier for semaphore capacity when using the
// batch engine (NSeqMax > 1). This controls how many requests can queue while
// the current batch is processing. Default is 2, meaning NSeqMax * 2 requests
// can be in-flight. Only applies to text inference models.
//
// RopeFreqBase overrides the RoPE base frequency. When nil, uses model default.
// Common values: 10000 (Llama), 1000000 (Qwen3).
//
// RopeFreqScale overrides the RoPE frequency scaling factor. When nil, uses
// model default or auto-calculates based on context extension ratio.
//
// RopeScaling controls the RoPE scaling method for extended context support.
// Set to RopeScalingYaRN to enable YaRN scaling for models like Qwen3 that
// support extended context (e.g., 32k training → 131k with YaRN).
//
// SWAFull controls whether models with sliding window attention (SWA) use a
// full-size KV cache for SWA layers instead of the memory-efficient small
// cache. When nil (default), llama.cpp's default is used (currently true).
// When explicitly set to false, SWA layers only cache the last n_swa tokens,
// saving significant VRAM but limiting context caching and shifting. When
// true, SWA layers use the full context window for their KV cache, preserving
// accuracy at the cost of higher memory usage.
//
// SplitMode controls how the model is split across multiple GPUs:
//   - SplitModeNone (0): single GPU
//   - SplitModeLayer (1): split layers and KV across GPUs
//   - SplitModeRow (2): split layers and KV across GPUs with tensor parallelism
//     (recommended for MoE models like Qwen3-MoE, Mixtral, DeepSeek)
//
// When nil (not set), defaults to SplitModeRow for optimal MoE performance.
//
// TensorBuftOverrides is a list of tensor buffer type override patterns that
// force matching tensors to execute on CPU instead of GPU. This is an expert-level
// configuration useful for MoE models where certain FFN expert tensors don't fit
// in VRAM. Supported values:
//   - "all-ffn": offload all FFN expression tensors to CPU
//   - "block:N": offload FFN tensors for block N to CPU (e.g., "block:12")
//   - Any regex pattern matching tensor names (e.g., `blk\.12\.ffn_(up|down|gate)`)
//
// TensorSplit controls how model layers are proportionally distributed across
// multiple GPUs. Each element represents the fraction of the model assigned to
// the corresponding device. For example, [0.6, 0.4] splits 60%/40% across two
// GPUs. The length must match the number of devices. When empty, the split is
// determined automatically based on available VRAM.
//
// UseDirectIO enables direct I/O for model loading.
//
// UseMMap controls whether mmap is used for model loading. When nil, mmap is
// enabled by default (llama.cpp default). Set to false to disable mmap, which
// is recommended for multi-socket NUMA systems running MoE models with CPU
// experts — without mmap, tensor data is directly allocated and can be placed
// on the appropriate NUMA node. UseDirectIO takes precedence over UseMMap.
//
// NUMA controls the NUMA (Non-Uniform Memory Access) strategy. This matters
// most when expert tensors are on CPU and the system has multiple NUMA nodes.
// Valid values: "" (disabled), "distribute", "isolate", "numactl", "mirror".
// "distribute" is recommended for multi-socket MoE setups; without it,
// cross-socket memory access can cause significant bandwidth collapse.
//
// YarnAttnFactor sets the YaRN attention magnitude scaling factor. When nil,
// uses default of 1.0.
//
// YarnBetaFast sets the YaRN low correction dimension. When nil, uses default
// of 32.0.
//
// YarnBetaSlow sets the YaRN high correction dimension. When nil, uses default
// of 1.0.
//
// YarnExtFactor sets the YaRN extrapolation mix factor. When nil, auto-calculated
// from context scaling ratio. Set to 0 to disable extrapolation.
//
// YarnOrigCtx sets the original training context size for YaRN scaling. When nil
// or 0, uses the model's native training context length from metadata.
type Config struct {
	PtrCacheMinTokens    *int
	PtrCacheSlotTimeout  *int
	CacheTypeK           GGMLType
	CacheTypeV           GGMLType
	Cataloger            Cataloger
	PtrContextWindow     *int
	DefaultParams        Params
	DraftModel           *DraftModelConfig
	Devices              []string // Device names for model execution (e.g., ["CUDA0", "CUDA1"])
	FlashAttention       FlashAttentionType
	PtrIncrementalCache  *bool
	PtrInsecureLogging   *bool
	JinjaFile            string
	Log                  Logger
	PtrMainGPU           *int
	MoE                  *MoEConfig
	ModelFiles           []string
	PtrNBatch            *int
	PtrNGpuLayers        *int
	PtrNSeqMax           *int
	PtrNThreads          *int
	PtrNThreadsBatch     *int
	PtrNUBatch           *int
	NUMA                 string
	PtrOffloadKQV        *bool
	PtrOpOffload         *bool
	PtrOpOffloadMinBatch *int
	ProjFile             string
	PtrQueueDepth        *int
	PtrRopeFreqBase      *float32
	PtrRopeFreqScale     *float32
	RopeScaling          RopeScalingType
	PtrSplitMode         *SplitMode
	PtrSWAFull           *bool
	TensorBuftOverrides  []string
	TensorSplit          []float32
	PtrUseDirectIO       *bool
	PtrUseMMap           *bool
	PtrYarnAttnFactor    *float32
	PtrYarnBetaFast      *float32
	PtrYarnBetaSlow      *float32
	PtrYarnExtFactor     *float32
	PtrYarnOrigCtx       *int
}

func (cfg Config) QueueDepth() int         { return intOr(cfg.PtrQueueDepth, 0) }
func (cfg Config) ContextWindow() int      { return intOr(cfg.PtrContextWindow, 0) }
func (cfg Config) NBatch() int             { return intOr(cfg.PtrNBatch, 0) }
func (cfg Config) NUBatch() int            { return intOr(cfg.PtrNUBatch, 0) }
func (cfg Config) NSeqMax() int            { return intOr(cfg.PtrNSeqMax, 0) }
func (cfg Config) NThreads() int           { return intOr(cfg.PtrNThreads, 0) }
func (cfg Config) NThreadsBatch() int      { return intOr(cfg.PtrNThreadsBatch, 0) }
func (cfg Config) CacheMinTokens() int     { return intOr(cfg.PtrCacheMinTokens, 0) }
func (cfg Config) CacheSlotTimeout() int   { return intOr(cfg.PtrCacheSlotTimeout, 0) }
func (cfg Config) OpOffloadMinBatch() int  { return intOr(cfg.PtrOpOffloadMinBatch, 0) }
func (cfg Config) MainGPU() int            { return intOr(cfg.PtrMainGPU, 0) }
func (cfg Config) NGpuLayers() int         { return intOr(cfg.PtrNGpuLayers, 0) }
func (cfg Config) RopeFreqBase() float32   { return float32Or(cfg.PtrRopeFreqBase, 0) }
func (cfg Config) RopeFreqScale() float32  { return float32Or(cfg.PtrRopeFreqScale, 0) }
func (cfg Config) YarnAttnFactor() float32 { return float32Or(cfg.PtrYarnAttnFactor, 0) }
func (cfg Config) YarnBetaFast() float32   { return float32Or(cfg.PtrYarnBetaFast, 0) }
func (cfg Config) YarnBetaSlow() float32   { return float32Or(cfg.PtrYarnBetaSlow, 0) }
func (cfg Config) YarnExtFactor() float32  { return float32Or(cfg.PtrYarnExtFactor, 0) }
func (cfg Config) YarnOrigCtx() int        { return intOr(cfg.PtrYarnOrigCtx, 0) }
func (cfg Config) IncrementalCache() bool  { return boolOr(cfg.PtrIncrementalCache, false) }
func (cfg Config) InsecureLogging() bool   { return boolOr(cfg.PtrInsecureLogging, false) }
func (cfg Config) UseDirectIO() bool       { return boolOr(cfg.PtrUseDirectIO, false) }

func (cfg Config) String() string {
	formatBoolPtr := func(p *bool) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprintf("%t", *p)
	}

	formatFloat32Ptr := func(p *float32) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprintf("%g", *p)
	}

	formatIntPtr := func(p *int) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprintf("%d", *p)
	}

	formatSplitModePtr := func(p *SplitMode) string {
		if p == nil {
			return "nil"
		}
		return p.String()
	}

	formatMoEPtr := func(m *MoEConfig) string {
		if m == nil {
			return "nil"
		}
		topN := "nil"
		if m.PtrKeepExpertsOnGPUForTopNLayers != nil {
			topN = fmt.Sprintf("%d", *m.PtrKeepExpertsOnGPUForTopNLayers)
		}
		return fmt.Sprintf("{mode:%s top_n:%s}", m.Mode, topN)
	}

	return fmt.Sprintf("\nCacheMinTokens[%s]\nCacheSlotTimeout[%s]\nCacheTypeK[%s]\nCacheTypeV[%s]\nContextWindow[%s]\nDevices[%v]\nFlashAttention[%s]\nIncrementalCache[%s]\nInsecureLogging[%s]\nJinjaFile[%s]\nMainGPU[%s]\nMoE[%s]\nModelFiles[%v]\nNBatch[%s]\nNGpuLayers[%s]\nNSeqMax[%s]\nNThreads[%s]\nNThreadsBatch[%s]\nNUBatch[%s]\nNUMA[%s]\nOffloadKQV[%s]\nOpOffload[%s]\nOpOffloadMinBatch[%s]\nProjFile[%s]\nRopeFreqBase[%s]\nRopeFreqScale[%s]\nRopeScaling[%s]\nSplitMode[%s]\nSWAFull[%s]\nTensorBuftOverrides[%v]\nTensorSplit[%v]\nUseDirectIO[%s]\nUseMMap[%s]\nYarnAttnFactor[%s]\nYarnBetaFast[%s]\nYarnBetaSlow[%s]\nYarnExtFactor[%s]\nYarnOrigCtx[%s]\nDraftModel[%v]\n",
		formatIntPtr(cfg.PtrCacheMinTokens), formatIntPtr(cfg.PtrCacheSlotTimeout), cfg.CacheTypeK, cfg.CacheTypeV,
		formatIntPtr(cfg.PtrContextWindow), cfg.Devices, cfg.FlashAttention,
		formatBoolPtr(cfg.PtrIncrementalCache), formatBoolPtr(cfg.PtrInsecureLogging), cfg.JinjaFile,
		formatIntPtr(cfg.PtrMainGPU), formatMoEPtr(cfg.MoE), cfg.ModelFiles, formatIntPtr(cfg.PtrNBatch),
		formatIntPtr(cfg.PtrNGpuLayers), formatIntPtr(cfg.PtrNSeqMax), formatIntPtr(cfg.PtrNThreads), formatIntPtr(cfg.PtrNThreadsBatch), formatIntPtr(cfg.PtrNUBatch),
		cfg.NUMA,
		formatBoolPtr(cfg.PtrOffloadKQV), formatBoolPtr(cfg.PtrOpOffload), formatIntPtr(cfg.PtrOpOffloadMinBatch), cfg.ProjFile,
		formatFloat32Ptr(cfg.PtrRopeFreqBase), formatFloat32Ptr(cfg.PtrRopeFreqScale), cfg.RopeScaling,
		formatSplitModePtr(cfg.PtrSplitMode),
		formatBoolPtr(cfg.PtrSWAFull), cfg.TensorBuftOverrides, cfg.TensorSplit, formatBoolPtr(cfg.PtrUseDirectIO),
		formatBoolPtr(cfg.PtrUseMMap),
		formatFloat32Ptr(cfg.PtrYarnAttnFactor),
		formatFloat32Ptr(cfg.PtrYarnBetaFast), formatFloat32Ptr(cfg.PtrYarnBetaSlow), formatFloat32Ptr(cfg.PtrYarnExtFactor), formatIntPtr(cfg.PtrYarnOrigCtx), cfg.DraftModel)
}

func validateConfig(ctx context.Context, cfg Config, log Logger) error {
	if len(cfg.ModelFiles) == 0 {
		return fmt.Errorf("validate-config: model file is required")
	}

	if len(cfg.Devices) > 0 {
		return fmt.Errorf("validate-config: Device and Devices are mutually exclusive; use Devices only (Device is deprecated)")
	}

	if len(cfg.TensorSplit) > 0 && len(cfg.Devices) > 0 && len(cfg.TensorSplit) != len(cfg.Devices) {
		return fmt.Errorf("validate-config: TensorSplit length (%d) must match Devices length (%d)", len(cfg.TensorSplit), len(cfg.Devices))
	}

	switch cfg.NUMA {
	case NUMADisabled, NUMADistribute, NUMAIsolate, NUMANumactl, NUMAMirror:
		// valid
	default:
		return fmt.Errorf("validate-config: unknown NUMA strategy: %s (valid: distribute, isolate, numactl, mirror)", cfg.NUMA)
	}

	if cfg.DraftModel != nil {
		if len(cfg.DraftModel.ModelFiles) == 0 {
			return fmt.Errorf("validate-config: draft model requires model files")
		}
		if cfg.NSeqMax() > 1 {
			return fmt.Errorf("validate-config: speculative decoding requires NSeqMax=1, got %d", cfg.NSeqMax())
		}
		for _, modelFile := range cfg.DraftModel.ModelFiles {
			log(ctx, "validate-config", "draft-model-file", modelFile)
			if err := CheckModel(modelFile, true); err != nil {
				return fmt.Errorf("validate-config: draft model: %w", err)
			}
		}
	}

	if cfg.MoE != nil {
		switch cfg.MoE.Mode {
		case MoEModeAuto, MoEModeExpertsCPU, MoEModeExpertsGPU, MoEModeKeepTopN, MoEModeCustom, "":
			// valid
		default:
			return fmt.Errorf("validate-config: unknown MoE mode: %s (valid: auto, experts_cpu, experts_gpu, keep_top_n, custom)", cfg.MoE.Mode)
		}

		if cfg.MoE.Mode == MoEModeKeepTopN && cfg.MoE.PtrKeepExpertsOnGPUForTopNLayers == nil {
			return fmt.Errorf("validate-config: MoE mode keep_top_n requires KeepExpertsOnGPUForTopNLayers to be set")
		}

		if cfg.MoE.PtrKeepExpertsOnGPUForTopNLayers != nil && *cfg.MoE.PtrKeepExpertsOnGPUForTopNLayers < 0 {
			return fmt.Errorf("validate-config: MoE KeepExpertsOnGPUForTopNLayers must be >= 0, got %d", *cfg.MoE.PtrKeepExpertsOnGPUForTopNLayers)
		}

		if cfg.MoE.Mode != "" && cfg.MoE.Mode != MoEModeAuto && cfg.MoE.Mode != MoEModeCustom && len(cfg.TensorBuftOverrides) > 0 {
			return fmt.Errorf("validate-config: MoE mode %s and TensorBuftOverrides are mutually exclusive; use MoE mode 'custom' with TensorBuftOverrides", cfg.MoE.Mode)
		}
	}

	if cfg.OpOffloadMinBatch() < 0 {
		return fmt.Errorf("validate-config: OpOffloadMinBatch must be >= 0, got %d", cfg.OpOffloadMinBatch())
	}

	for _, modelFile := range cfg.ModelFiles {
		log(ctx, "validate-config", "model-file", modelFile)

		if err := CheckModel(modelFile, true); err != nil {
			return fmt.Errorf("validate-config: %w", err)
		}
	}

	if cfg.ProjFile != "" {
		log(ctx, "validate-config", "model-file", cfg.ProjFile)

		if err := CheckModel(cfg.ProjFile, true); err != nil {
			return fmt.Errorf("validate-config: prog-file[%s]: %w", cfg.ProjFile, err)
		}
	}

	return nil
}

func adjustConfig(cfg Config, model llama.Model) Config {
	cfg = adjustContextWindow(cfg, model)

	// MoE-optimized defaults: larger batch sizes for CPU expert offload.
	moeExperts := cfg.MoE != nil && (cfg.MoE.Mode == MoEModeExpertsCPU || cfg.MoE.Mode == MoEModeKeepTopN)
	if moeExperts {
		if cfg.NBatch() <= 0 {
			cfg.PtrNBatch = new(4096)
		}
		if cfg.NUBatch() <= 0 {
			cfg.PtrNUBatch = new(4096)
		}
	}

	if cfg.NBatch() <= 0 {
		cfg.PtrNBatch = new(defNBatch)
	}

	if cfg.NUBatch() <= 0 {
		// Vision models require n_ubatch >= n_tokens for the image encoder's
		// non-causal attention. Use a larger default when ProjFile is set.
		switch cfg.ProjFile != "" {
		case true:
			cfg.PtrNUBatch = new(defNUBatchVision)
		case false:
			cfg.PtrNUBatch = new(defNUBatch)
		}
	}

	if cfg.NThreads() < 0 {
		cfg.PtrNThreads = new(defThreadZero)
	}
	if cfg.PtrNThreads == nil {
		cfg.PtrNThreads = new(defThreadZero)
	}

	if cfg.NThreadsBatch() < 0 {
		cfg.PtrNThreadsBatch = new(defThreadZero)
	}
	if cfg.PtrNThreadsBatch == nil {
		cfg.PtrNThreadsBatch = new(defThreadZero)
	}

	// NBatch is generally greater than or equal to NUBatch. The entire
	// NUBatch of tokens must fit into a physical batch for processing.
	if cfg.NUBatch() > cfg.NBatch() {
		cfg.PtrNUBatch = new(cfg.NBatch())
	}

	// This value must be 1 to properly configure the batch engine.
	if cfg.NSeqMax() <= 0 {
		cfg.PtrNSeqMax = new(defNSeqMax)
	}

	// IMC is enabled by default.
	if cfg.PtrIncrementalCache == nil {
		cfg.PtrIncrementalCache = new(true)
	}

	// Default minimum tokens for caching.
	if cfg.IncrementalCache() && cfg.CacheMinTokens() <= 0 {
		cfg.PtrCacheMinTokens = new(defMinCacheTokens)
	}
	if cfg.PtrCacheMinTokens == nil {
		cfg.PtrCacheMinTokens = new(0)
	}

	// Default slot wait timeout for IMC.
	if cfg.IncrementalCache() && cfg.CacheSlotTimeout() <= 0 {
		cfg.PtrCacheSlotTimeout = new(defCacheSlotTimeout)
	}
	if cfg.PtrCacheSlotTimeout == nil {
		cfg.PtrCacheSlotTimeout = new(0)
	}

	if cfg.DraftModel != nil && cfg.DraftModel.NDraft <= 0 {
		cfg.DraftModel.NDraft = defNDraft
	}

	// Hybrid models (Attention + Recurrent) don't support flash attention,
	// and quantized KV caches require flash attention. Force f16 KV cache
	// in the config so downstream code and display reflect the actual values.
	if llama.ModelIsHybrid(model) {
		cfg.CacheTypeK = GGMLTypeF16
		cfg.CacheTypeV = GGMLTypeF16
		cfg.FlashAttention = FlashAttentionDisabled
	}

	// Ensure remaining pointer fields are non-nil after adjustment.
	if cfg.PtrInsecureLogging == nil {
		cfg.PtrInsecureLogging = new(false)
	}
	if cfg.PtrUseDirectIO == nil {
		cfg.PtrUseDirectIO = new(false)
	}
	if cfg.PtrOpOffloadMinBatch == nil {
		cfg.PtrOpOffloadMinBatch = new(0)
	}

	return cfg
}

func applyCatalogConfig(user Config, cat Config) Config {
	if len(user.Devices) == 0 {
		user.Devices = cat.Devices
	}
	if user.PtrContextWindow == nil {
		user.PtrContextWindow = cat.PtrContextWindow
	}
	if user.PtrNBatch == nil {
		user.PtrNBatch = cat.PtrNBatch
	}
	if user.PtrNUBatch == nil {
		user.PtrNUBatch = cat.PtrNUBatch
	}
	if user.PtrNThreads == nil {
		user.PtrNThreads = cat.PtrNThreads
	}
	if user.PtrNThreadsBatch == nil {
		user.PtrNThreadsBatch = cat.PtrNThreadsBatch
	}
	if user.CacheTypeK == GGMLTypeAuto {
		user.CacheTypeK = cat.CacheTypeK
	}
	if user.CacheTypeV == GGMLTypeAuto {
		user.CacheTypeV = cat.CacheTypeV
	}
	if user.PtrUseDirectIO == nil {
		user.PtrUseDirectIO = cat.PtrUseDirectIO
	}
	if user.PtrUseMMap == nil {
		user.PtrUseMMap = cat.PtrUseMMap
	}
	if user.NUMA == NUMADisabled {
		user.NUMA = cat.NUMA
	}
	if user.PtrNSeqMax == nil {
		user.PtrNSeqMax = cat.PtrNSeqMax
	}
	if user.PtrOffloadKQV == nil {
		user.PtrOffloadKQV = cat.PtrOffloadKQV
	}
	if user.PtrOpOffload == nil {
		user.PtrOpOffload = cat.PtrOpOffload
	}
	if user.PtrOpOffloadMinBatch == nil {
		user.PtrOpOffloadMinBatch = cat.PtrOpOffloadMinBatch
	}
	if user.PtrNGpuLayers == nil {
		user.PtrNGpuLayers = cat.PtrNGpuLayers
	}
	if user.PtrMainGPU == nil {
		user.PtrMainGPU = cat.PtrMainGPU
	}
	if user.PtrSplitMode == nil {
		user.PtrSplitMode = cat.PtrSplitMode
	}
	if len(user.TensorSplit) == 0 {
		user.TensorSplit = cat.TensorSplit
	}
	if len(user.TensorBuftOverrides) == 0 {
		user.TensorBuftOverrides = cat.TensorBuftOverrides
	}
	if user.PtrSWAFull == nil {
		user.PtrSWAFull = cat.PtrSWAFull
	}
	if user.PtrIncrementalCache == nil {
		user.PtrIncrementalCache = cat.PtrIncrementalCache
	}
	if user.PtrCacheMinTokens == nil {
		user.PtrCacheMinTokens = cat.PtrCacheMinTokens
	}
	if user.PtrCacheSlotTimeout == nil {
		user.PtrCacheSlotTimeout = cat.PtrCacheSlotTimeout
	}
	if user.PtrInsecureLogging == nil {
		user.PtrInsecureLogging = cat.PtrInsecureLogging
	}
	if user.RopeScaling == RopeScalingNone {
		user.RopeScaling = cat.RopeScaling
	}
	if user.PtrRopeFreqBase == nil {
		user.PtrRopeFreqBase = cat.PtrRopeFreqBase
	}
	if user.PtrRopeFreqScale == nil {
		user.PtrRopeFreqScale = cat.PtrRopeFreqScale
	}
	if user.PtrYarnExtFactor == nil {
		user.PtrYarnExtFactor = cat.PtrYarnExtFactor
	}
	if user.PtrYarnAttnFactor == nil {
		user.PtrYarnAttnFactor = cat.PtrYarnAttnFactor
	}
	if user.PtrYarnBetaFast == nil {
		user.PtrYarnBetaFast = cat.PtrYarnBetaFast
	}
	if user.PtrYarnBetaSlow == nil {
		user.PtrYarnBetaSlow = cat.PtrYarnBetaSlow
	}
	if user.PtrYarnOrigCtx == nil {
		user.PtrYarnOrigCtx = cat.PtrYarnOrigCtx
	}
	if user.MoE == nil {
		user.MoE = cat.MoE
	}
	if user.DraftModel == nil {
		user.DraftModel = cat.DraftModel
	}

	user.DefaultParams = applyCatalogSamplingParams(user.DefaultParams, cat.DefaultParams)

	return user
}

func applyCatalogSamplingParams(user Params, cat Params) Params {
	if user.Temperature == 0 {
		user.Temperature = cat.Temperature
	}
	if user.TopK == 0 {
		user.TopK = cat.TopK
	}
	if user.TopP == 0 {
		user.TopP = cat.TopP
	}
	if user.MinP == 0 {
		user.MinP = cat.MinP
	}
	if user.RepeatPenalty == 0 {
		user.RepeatPenalty = cat.RepeatPenalty
	}
	if user.RepeatLastN == 0 {
		user.RepeatLastN = cat.RepeatLastN
	}
	if user.FrequencyPenalty == 0 {
		user.FrequencyPenalty = cat.FrequencyPenalty
	}
	if user.PresencePenalty == 0 {
		user.PresencePenalty = cat.PresencePenalty
	}
	if user.DryMultiplier == 0 {
		user.DryMultiplier = cat.DryMultiplier
	}
	if user.DryBase == 0 {
		user.DryBase = cat.DryBase
	}
	if user.DryAllowedLen == 0 {
		user.DryAllowedLen = cat.DryAllowedLen
	}
	if user.DryPenaltyLast == 0 {
		user.DryPenaltyLast = cat.DryPenaltyLast
	}
	if user.XtcProbability == 0 {
		user.XtcProbability = cat.XtcProbability
	}
	if user.XtcThreshold == 0 {
		user.XtcThreshold = cat.XtcThreshold
	}
	if user.XtcMinKeep == 0 {
		user.XtcMinKeep = cat.XtcMinKeep
	}
	if user.Thinking == "" {
		user.Thinking = cat.Thinking
	}
	if user.ReasoningEffort == "" {
		user.ReasoningEffort = cat.ReasoningEffort
	}
	if user.Grammar == "" {
		user.Grammar = cat.Grammar
	}

	return user
}

func adjustContextWindow(cfg Config, model llama.Model) Config {
	modelCW := defContextWindow
	v, found := searchModelMeta(model, "context_length")
	if found {
		ctxLen, err := strconv.Atoi(v)
		if err == nil {
			modelCW = ctxLen
		}
	}

	if cfg.ContextWindow() <= 0 {
		cw := modelCW
		cfg.PtrContextWindow = &cw
	}

	return cfg
}

func modelCtxParams(cfg Config, mi ModelInfo) llama.ContextParams {
	ctxParams := llama.ContextDefaultParams()

	if mi.IsEmbedModel || mi.IsRerankModel {
		ctxParams.Embeddings = 1
	}

	if mi.IsRerankModel {
		ctxParams.PoolingType = llama.PoolingTypeRank
	}

	// IMC externalizes KV state to RAM after cache build, so it does not
	// need extra sequences beyond the configured NSeqMax.
	nSeqMax := max(cfg.NSeqMax(), 1)
	totalSeqs := nSeqMax

	if cfg.ContextWindow() > 0 {
		ctxParams.NBatch = uint32(cfg.NBatch())
		ctxParams.NUbatch = uint32(cfg.NUBatch())
		ctxParams.NThreads = int32(cfg.NThreads())
		ctxParams.NThreadsBatch = int32(cfg.NThreadsBatch())
		ctxParams.NCtx = uint32(cfg.ContextWindow() * nSeqMax)
	}

	if cfg.CacheTypeK != GGMLTypeAuto {
		ctxParams.TypeK = cfg.CacheTypeK.ToYZMAType()
	}

	if cfg.CacheTypeV != GGMLTypeAuto {
		ctxParams.TypeV = cfg.CacheTypeV.ToYZMAType()
	}

	switch {
	case cfg.FlashAttention == FlashAttentionDisabled:
		ctxParams.FlashAttentionType = llama.FlashAttentionTypeDisabled

	case cfg.FlashAttention == FlashAttentionAuto:
		ctxParams.FlashAttentionType = llama.FlashAttentionTypeAuto

	default:
		ctxParams.FlashAttentionType = llama.FlashAttentionTypeEnabled
	}

	ctxParams.NSeqMax = uint32(totalSeqs)

	// Enable unified KV cache so all sequences share one pool of
	// ContextWindow * NSeqMax tokens. Each slot is gated to ContextWindow
	// tokens by the batch engine (startSlotText / startSlotTextMRoPE /
	// startSlotMedia). With unified mode the pool is flexible: idle slots
	// don't reserve capacity, and KV pressure eviction can reclaim stale
	// slots when the shared pool gets tight.
	if nSeqMax > 1 {
		ctxParams.KVUnified = 1
	}

	// Offload KQV cache to CPU.
	// llama.cpp has this as default set to true
	ctxParams.Offload_kqv = 1
	if cfg.PtrOffloadKQV != nil &&
		!*cfg.PtrOffloadKQV {
		ctxParams.Offload_kqv = 0
	}

	// Offload host tensor operations to device.
	// llama.cpp has this as default set to true
	ctxParams.OpOffload = 1
	if cfg.PtrOpOffload != nil && !*cfg.PtrOpOffload {
		ctxParams.OpOffload = 0
	}

	// Use full-size SWA cache for sliding window attention models.
	// When enabled, SWA layers use the full context window for their KV
	// cache instead of the compact n_swa-sized cache, preserving accuracy
	// at the cost of higher memory usage.
	// When nil, llama.cpp's default (true/on) is used.
	if cfg.PtrSWAFull != nil {
		if *cfg.PtrSWAFull {
			ctxParams.SwaFull = 1
		} else {
			ctxParams.SwaFull = 0
		}
	}

	// YaRN RoPE scaling for extended context windows.
	// Only set parameters when explicitly configured (non-nil).
	// llama.cpp uses special values (0, -1) to mean "use model defaults".
	if cfg.RopeScaling != RopeScalingNone {
		ctxParams.RopeScalingType = cfg.RopeScaling.ToYZMAType()
	}
	if cfg.PtrRopeFreqBase != nil {
		ctxParams.RopeFreqBase = *cfg.PtrRopeFreqBase
	}
	if cfg.PtrRopeFreqScale != nil {
		ctxParams.RopeFreqScale = *cfg.PtrRopeFreqScale
	}
	if cfg.PtrYarnExtFactor != nil {
		ctxParams.YarnExtFactor = *cfg.PtrYarnExtFactor
	}
	if cfg.PtrYarnAttnFactor != nil {
		ctxParams.YarnAttnFactor = *cfg.PtrYarnAttnFactor
	}
	if cfg.PtrYarnBetaFast != nil {
		ctxParams.YarnBetaFast = *cfg.PtrYarnBetaFast
	}
	if cfg.PtrYarnBetaSlow != nil {
		ctxParams.YarnBetaSlow = *cfg.PtrYarnBetaSlow
	}
	if cfg.PtrYarnOrigCtx != nil {
		ctxParams.YarnOrigCtx = uint32(*cfg.PtrYarnOrigCtx)
	}

	return ctxParams
}

func searchModelMeta(model llama.Model, find string) (string, bool) {
	count := llama.ModelMetaCount(model)

	for i := range count {
		key, ok := llama.ModelMetaKeyByIndex(model, i)
		if !ok {
			continue
		}

		if strings.Contains(key, find) {
			value, ok := llama.ModelMetaValStrByIndex(model, i)
			if !ok {
				continue
			}

			return value, true
		}
	}

	return "", false
}

// =============================================================================

// GGMLType represents a ggml data type for the KV cache.
// These values correspond to the ggml_type enum in llama.cpp.
type GGMLType int32

const (
	GGMLTypeAuto GGMLType = 0  // Use default from llama.cpp (zero value)
	GGMLTypeF32  GGMLType = 50 // 32-bit floating point
	GGMLTypeF16  GGMLType = 1  // 16-bit floating point
	GGMLTypeQ4_0 GGMLType = 2  // 4-bit quantization (type 0)
	GGMLTypeQ4_1 GGMLType = 3  // 4-bit quantization (type 1)
	GGMLTypeQ5_0 GGMLType = 6  // 5-bit quantization (type 0)
	GGMLTypeQ5_1 GGMLType = 7  // 5-bit quantization (type 1)
	GGMLTypeQ8_0 GGMLType = 8  // 8-bit quantization (type 0)
	GGMLTypeBF16 GGMLType = 30 // Brain floating point 16-bit
)

// String returns the string representation of a GGMLType.
func (t GGMLType) String() string {
	switch t {
	case GGMLTypeF32:
		return "f32"

	case GGMLTypeF16:
		return "f16"

	case GGMLTypeQ4_0:
		return "q4_0"

	case GGMLTypeQ4_1:
		return "q4_1"

	case GGMLTypeQ5_0:
		return "q5_0"

	case GGMLTypeQ5_1:
		return "q5_1"

	case GGMLTypeQ8_0:
		return "q8_0"

	case GGMLTypeBF16:
		return "bf16"

	case GGMLTypeAuto:
		return "auto"

	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func (t GGMLType) ToYZMAType() llama.GGMLType {
	switch t {
	case GGMLTypeAuto:
		return llama.GGMLType(-1)
	case GGMLTypeF32:
		return llama.GGMLType(0)
	default:
		return llama.GGMLType(t)
	}
}

// GGMLTypeFromYZMA converts a llama.GGMLType back to a GGMLType.
func GGMLTypeFromYZMA(t llama.GGMLType) GGMLType {
	switch t {
	case llama.GGMLType(-1):
		return GGMLTypeAuto
	case llama.GGMLType(0):
		return GGMLTypeF32
	default:
		return GGMLType(t)
	}
}

func (t GGMLType) MarshalYAML() (any, error) {
	return t.String(), nil
}

func (t GGMLType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *GGMLType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	parsed, err := ParseGGMLType(s)
	if err != nil {
		return err
	}

	*t = parsed

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler to parse string values like "f16".
func (t *GGMLType) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	parsed, err := ParseGGMLType(s)
	if err != nil {
		return err
	}

	*t = parsed

	return nil
}

// ParseGGMLType parses a string into a GGMLType.
// Supported values: "f32", "f16", "q4_0", "q4_1", "q5_0", "q5_1", "q8_0", "bf16", "auto".
func ParseGGMLType(s string) (GGMLType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "f32", "fp32":
		return GGMLTypeF32, nil

	case "f16", "fp16":
		return GGMLTypeF16, nil

	case "q4_0", "q4":
		return GGMLTypeQ4_0, nil

	case "q4_1":
		return GGMLTypeQ4_1, nil

	case "q5_0", "q5":
		return GGMLTypeQ5_0, nil

	case "q5_1":
		return GGMLTypeQ5_1, nil

	case "f8", "q8_0", "q8":
		return GGMLTypeQ8_0, nil

	case "bf16", "bfloat16":
		return GGMLTypeBF16, nil

	case "auto", "":
		return GGMLTypeAuto, nil

	default:
		return GGMLTypeAuto, fmt.Errorf("unknown ggml type: %s", s)
	}
}

// =============================================================================

// FlashAttentionType controls when to enable Flash Attention.
// Flash Attention reduces memory usage and speeds up attention computation,
// especially beneficial for large context windows.
type FlashAttentionType int32

const (
	FlashAttentionEnabled  FlashAttentionType = 0 // Default: enable Flash Attention
	FlashAttentionDisabled FlashAttentionType = 1 // Disable Flash Attention
	FlashAttentionAuto     FlashAttentionType = 2 // Let llama.cpp decide
)

func (t FlashAttentionType) String() string {
	switch t {
	case FlashAttentionEnabled:
		return "enabled"
	case FlashAttentionDisabled:
		return "disabled"
	case FlashAttentionAuto:
		return "auto"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

func (t FlashAttentionType) MarshalYAML() (any, error) {
	return t.String(), nil
}

func (t FlashAttentionType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *FlashAttentionType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(s)) {
	case "enabled", "on", "true", "1":
		*t = FlashAttentionEnabled
	case "disabled", "off", "false", "0":
		*t = FlashAttentionDisabled
	case "auto", "":
		*t = FlashAttentionAuto
	default:
		return fmt.Errorf("unknown flash attention type: %s", s)
	}

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler to parse string values.
func (t *FlashAttentionType) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(s)) {
	case "enabled", "on", "true", "1":
		*t = FlashAttentionEnabled

	case "disabled", "off", "false", "0":
		*t = FlashAttentionDisabled

	case "auto", "":
		*t = FlashAttentionAuto

	default:
		return fmt.Errorf("unmarshal-yaml: unknown flash attention type: %s", s)
	}

	return nil
}

// DerefFlashAttention returns the value of a FlashAttentionType pointer,
// defaulting to FlashAttentionEnabled when nil.
func DerefFlashAttention(p *FlashAttentionType) FlashAttentionType {
	if p == nil {
		return FlashAttentionEnabled
	}
	return *p
}

// =============================================================================

// SplitMode controls how the model is split across multiple GPUs.
// This is particularly important for Mixture of Experts (MoE) models.
type SplitMode int32

const (
	// SplitModeNone uses a single GPU (default).
	SplitModeNone SplitMode = 0

	// SplitModeLayer splits layers and KV cache across GPUs.
	SplitModeLayer SplitMode = 1

	// SplitModeRow splits layers and KV across GPUs with tensor parallelism.
	// This enables expert-parallel execution for MoE models (Qwen3-MoE, Mixtral, DeepSeek).
	// Equivalent to vLLM's --enable-expert-parallel flag.
	SplitModeRow SplitMode = 2
)

// String returns the string representation of a SplitMode.
func (s SplitMode) String() string {
	switch s {
	case SplitModeNone:
		return "none"

	case SplitModeLayer:
		return "layer"

	case SplitModeRow:
		return "row"

	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// ToYZMAType converts to the yzma/llama.cpp SplitMode type.
func (s SplitMode) ToYZMAType() llama.SplitMode {
	return llama.SplitMode(s)
}

func (s SplitMode) MarshalYAML() (any, error) {
	return s.String(), nil
}

func (s SplitMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *SplitMode) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	parsed, err := ParseSplitMode(str)
	if err != nil {
		return err
	}

	*s = parsed

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler to parse string values.
func (s *SplitMode) UnmarshalYAML(unmarshal func(any) error) error {
	var str string
	if err := unmarshal(&str); err != nil {
		return err
	}

	parsed, err := ParseSplitMode(str)
	if err != nil {
		return err
	}

	*s = parsed

	return nil
}

// ParseSplitMode parses a string into a SplitMode.
// Supported values: "none", "layer", "row", "expert-parallel", "tensor-parallel".
func ParseSplitMode(s string) (SplitMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "single", "0", "":
		return SplitModeNone, nil

	case "layer", "1":
		return SplitModeLayer, nil

	case "row", "tensor", "tensor-parallel", "expert-parallel", "2":
		return SplitModeRow, nil

	default:
		return SplitModeNone, fmt.Errorf("parse-split-mode: unknown split mode: %s (valid: none, layer, row, expert-parallel)", s)
	}
}

// =============================================================================

// RopeScalingType controls RoPE (Rotary Position Embedding) scaling method.
// This enables extended context windows beyond the model's native training length.
// For example, Qwen3 models trained on 32k can support 131k with YaRN scaling.
type RopeScalingType int32

const (
	// RopeScalingNone disables RoPE scaling (use native context length).
	RopeScalingNone RopeScalingType = 0

	// RopeScalingLinear uses linear interpolation scaling.
	// Simple but less effective for large extensions.
	RopeScalingLinear RopeScalingType = 1

	// RopeScalingYaRN uses YaRN (Yet another RoPE extensioN) scaling.
	// Recommended for extending context 2-4x beyond training length.
	// Applies frequency-dependent interpolation with attention scaling.
	RopeScalingYaRN RopeScalingType = 2
)

// String returns the string representation of a RopeScalingType.
func (r RopeScalingType) String() string {
	switch r {
	case RopeScalingNone:
		return "none"

	case RopeScalingLinear:
		return "linear"

	case RopeScalingYaRN:
		return "yarn"

	default:
		return fmt.Sprintf("unknown(%d)", r)
	}
}

// ToYZMAType converts to the yzma/llama.cpp RopeScalingType.
func (r RopeScalingType) ToYZMAType() llama.RopeScalingType {
	return llama.RopeScalingType(r)
}

func (r RopeScalingType) MarshalYAML() (any, error) {
	return r.String(), nil
}

func (r RopeScalingType) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *RopeScalingType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	parsed, err := ParseRopeScalingType(s)
	if err != nil {
		return err
	}

	*r = parsed

	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler to parse string values.
func (r *RopeScalingType) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	parsed, err := ParseRopeScalingType(s)
	if err != nil {
		return err
	}

	*r = parsed

	return nil
}

// ParseRopeScalingType parses a string into a RopeScalingType.
// Supported values: "none", "linear", "yarn".
func ParseRopeScalingType(s string) (RopeScalingType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none", "0", "":
		return RopeScalingNone, nil

	case "linear", "1":
		return RopeScalingLinear, nil

	case "yarn", "2":
		return RopeScalingYaRN, nil

	default:
		return RopeScalingNone, fmt.Errorf("parse-rope-scaling-type: unknown type: %s (valid: none, linear, yarn)", s)
	}
}

// =============================================================================

// MoEMode controls expert placement strategy for Mixture of Experts models.
type MoEMode string

const (
	// MoEModeAuto uses catalog defaults.
	MoEModeAuto MoEMode = "auto"

	// MoEModeExpertsCPU places all routed expert tensors on CPU.
	// Recommended for VRAM-constrained setups.
	MoEModeExpertsCPU MoEMode = "experts_cpu"

	// MoEModeExpertsGPU keeps all expert tensors on GPU.
	// Requires sufficient VRAM for the full model.
	MoEModeExpertsGPU MoEMode = "experts_gpu"

	// MoEModeKeepTopN keeps routed experts on GPU for the top N layers.
	// All other expert layers go to CPU.
	MoEModeKeepTopN MoEMode = "keep_top_n"

	// MoEModeCustom defers to TensorBuftOverrides for expert placement.
	MoEModeCustom MoEMode = "custom"
)

// MoEConfig configures Mixture of Experts tensor placement.
// When nil, no MoE-specific behavior is applied.
type MoEConfig struct {
	// Mode controls expert placement strategy.
	Mode MoEMode `yaml:"mode,omitempty"`

	// PtrKeepExpertsOnGPUForTopNLayers keeps routed expert tensors on GPU for the
	// top N layers (highest-index layers). All other expert layers go to CPU.
	// Only used when Mode is MoEModeKeepTopN. 0 means all experts on CPU.
	// llama.cpp convention: "top" means highest-numbered layers.
	PtrKeepExpertsOnGPUForTopNLayers *int `yaml:"keep-experts-top-n,omitempty"`
}

func (m MoEConfig) KeepExpertsOnGPUForTopNLayers() int {
	return intOr(m.PtrKeepExpertsOnGPUForTopNLayers, 0)
}

// =============================================================================

// Option represents a functional option for configuring a Config.
type Option func(*Config)

// NewConfig creates a Config with the provided functional options applied.
func NewConfig(opts ...Option) Config {
	var cfg Config
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func WithConfig(src Config) Option                   { return func(c *Config) { *c = src } }
func WithCataloger(v Cataloger) Option               { return func(c *Config) { c.Cataloger = v } }
func WithCacheMinTokens(v int) Option                { return func(c *Config) { c.PtrCacheMinTokens = new(v) } }
func WithCacheSlotTimeout(v int) Option              { return func(c *Config) { c.PtrCacheSlotTimeout = new(v) } }
func WithCacheTypeK(v GGMLType) Option               { return func(c *Config) { c.CacheTypeK = v } }
func WithCacheTypeV(v GGMLType) Option               { return func(c *Config) { c.CacheTypeV = v } }
func WithContextWindow(v int) Option                 { return func(c *Config) { c.PtrContextWindow = new(v) } }
func WithDefaultParams(v Params) Option              { return func(c *Config) { c.DefaultParams = v } }
func WithDevices(v []string) Option                  { return func(c *Config) { c.Devices = v } }
func WithDraftModel(v *DraftModelConfig) Option      { return func(c *Config) { c.DraftModel = v } }
func WithFlashAttention(v FlashAttentionType) Option { return func(c *Config) { c.FlashAttention = v } }
func WithIncrementalCache(v bool) Option             { return func(c *Config) { c.PtrIncrementalCache = new(v) } }
func WithInsecureLogging(v bool) Option              { return func(c *Config) { c.PtrInsecureLogging = new(v) } }
func WithJinjaFile(v string) Option                  { return func(c *Config) { c.JinjaFile = v } }
func WithLog(v Logger) Option                        { return func(c *Config) { c.Log = v } }
func WithMainGPU(v int) Option                       { return func(c *Config) { c.PtrMainGPU = new(v) } }
func WithMoE(v *MoEConfig) Option                    { return func(c *Config) { c.MoE = v } }
func WithModelFiles(v []string) Option               { return func(c *Config) { c.ModelFiles = v } }
func WithNBatch(v int) Option                        { return func(c *Config) { c.PtrNBatch = new(v) } }
func WithNGpuLayers(v int) Option                    { return func(c *Config) { c.PtrNGpuLayers = new(v) } }
func WithNSeqMax(v int) Option                       { return func(c *Config) { c.PtrNSeqMax = new(v) } }
func WithNThreads(v int) Option                      { return func(c *Config) { c.PtrNThreads = new(v) } }
func WithNThreadsBatch(v int) Option                 { return func(c *Config) { c.PtrNThreadsBatch = new(v) } }
func WithNUBatch(v int) Option                       { return func(c *Config) { c.PtrNUBatch = new(v) } }
func WithNUMA(v string) Option                       { return func(c *Config) { c.NUMA = v } }
func WithOffloadKQV(v bool) Option                   { return func(c *Config) { c.PtrOffloadKQV = new(v) } }
func WithOpOffload(v bool) Option                    { return func(c *Config) { c.PtrOpOffload = new(v) } }
func WithOpOffloadMinBatch(v int) Option             { return func(c *Config) { c.PtrOpOffloadMinBatch = new(v) } }
func WithProjFile(v string) Option                   { return func(c *Config) { c.ProjFile = v } }
func WithRopeFreqBase(v float32) Option              { return func(c *Config) { c.PtrRopeFreqBase = new(v) } }
func WithRopeFreqScale(v float32) Option             { return func(c *Config) { c.PtrRopeFreqScale = new(v) } }
func WithRopeScaling(v RopeScalingType) Option       { return func(c *Config) { c.RopeScaling = v } }
func WithSplitMode(v SplitMode) Option               { return func(c *Config) { c.PtrSplitMode = new(v) } }
func WithSWAFull(v bool) Option                      { return func(c *Config) { c.PtrSWAFull = new(v) } }
func WithTensorBuftOverrides(v []string) Option      { return func(c *Config) { c.TensorBuftOverrides = v } }
func WithTensorSplit(v []float32) Option             { return func(c *Config) { c.TensorSplit = v } }
func WithUseDirectIO(v bool) Option                  { return func(c *Config) { c.PtrUseDirectIO = new(v) } }
func WithUseMMap(v bool) Option                      { return func(c *Config) { c.PtrUseMMap = new(v) } }
func WithYarnAttnFactor(v float32) Option            { return func(c *Config) { c.PtrYarnAttnFactor = new(v) } }
func WithYarnBetaFast(v float32) Option              { return func(c *Config) { c.PtrYarnBetaFast = new(v) } }
func WithYarnBetaSlow(v float32) Option              { return func(c *Config) { c.PtrYarnBetaSlow = new(v) } }
func WithYarnExtFactor(v float32) Option             { return func(c *Config) { c.PtrYarnExtFactor = new(v) } }
func WithYarnOrigCtx(v int) Option                   { return func(c *Config) { c.PtrYarnOrigCtx = new(v) } }
func WithQueueDepth(v int) Option {
	return func(c *Config) {
		if v > 0 {
			c.PtrQueueDepth = new(v)
		}
	}
}

package model

import (
	"context"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/hybridgroup/yzma/pkg/llama"
)

/*
Batch sizing defaults (applied in adjustConfig when left unset):

	Prefill = 2048
	Base    = max(Prefill, ImageBatchTokenCapacity, ImageTokenBudget)
	NBatch  = Base + generation reserve
	NUBatch = Base (non-MTP) or NBatch (MTP)

Non-causal image chunks need every row in one physical ubatch. Image capacity
and preprocessing budgets are explicit opt-ins; projector defaults are unchanged.

One slot owns ordinary prefill until it finishes. Generation rows from all
ready slots are staged first. Non-MTP reserves one row; MTP reserves 1 + nDraft
rows per configured slot. Non-MTP can split the logical tray into physical
ubatches. MTP preserves the existing single-physical-batch policy pending
validation of multi-ubatch hidden-state consumption.

Key principles:
- NUBatch ≤ NBatch always (enforced in adjustConfig)
- NUBatch primarily affects prompt processing speed and compute-buffer VRAM
*/

const (
	defContextWindow        = 8 * 1024
	defMinCacheTokens       = 100
	defNThreads             = 4
	defNSeqMax              = 1
	defNDraft               = 5
	defaultAdmissionTimeout = 3 * time.Minute
)

// DefaultPrefillBatchSize is the prompt-token capacity used when prefill batch
// size is not explicitly configured.
const DefaultPrefillBatchSize = int(vram.DefaultNUBatch)

// DefaultQueueDepth is the number of admitted request layers used when queue
// depth is not explicitly configured.
const DefaultQueueDepth = 2

// Logger provides a function for logging messages from different APIs.
type Logger = applog.Logger

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

// DraftModelConfig configures speculative decoding for a target model. It
// serves two purposes depending on whether ModelFiles is set:
//
//  1. Separate-GGUF draft (ModelFiles set): a smaller, faster model
//     generates candidate tokens that the target verifies in a single
//     forward pass. Requires NSeqMax == 1 (single-slot mode) and a draft
//     that shares the target's vocabulary (same tokenizer).
//
//  2. MTP nDraft override (ModelFiles empty): when the target GGUF ships
//     an auto-detected MTP head, this block sets the number of draft tokens
//     per round without supplying a separate model. NDraft defaults to
//     defMTPNDraft when left unset.
//
// A model can have at most one drafter. If ModelFiles is set, the
// separate-GGUF drafter wins even on a target that also has an MTP head.
type DraftModelConfig struct {
	ModelFiles    []string  // Path to the draft model GGUF file(s); empty means MTP nDraft override
	NDraft        int       // Number of tokens to draft per step (separate-GGUF default 5, MTP default 3)
	PtrNGpuLayers *int      // GPU layers for draft model (nil = all layers on GPU)
	Devices       []string  // Devices for draft model (e.g., ["CUDA0"])
	PtrMainGPU    *int      // Primary GPU index for draft model
	TensorSplit   []float32 // Per-device tensor split for draft model
}

func (d DraftModelConfig) NGpuLayers() int { return intOr(d.PtrNGpuLayers, 0) }
func (d DraftModelConfig) MainGPU() int    { return intOr(d.PtrMainGPU, 0) }

// IsSeparate reports whether this config points at a separate draft GGUF.
// When false, the config is an MTP nDraft override that carries no model
// files and only applies when the target ships an auto-detected MTP head.
func (d DraftModelConfig) IsSeparate() bool { return len(d.ModelFiles) > 0 }

// AdapterConfig configures a local llama.cpp-compatible LoRA adapter GGUF.
// Scale is fixed for the lifetime of the loaded model.
type AdapterConfig struct {
	Path  string
	Scale float32
}

// Config represents model level configuration. These values if configured
// incorrectly can cause the system to panic. The defaults are used when these
// values are set to 0.
//
// Adapters contains local llama.cpp-compatible LoRA adapter GGUF files to load
// with the model. Adapter scales are fixed for the lifetime of the model.
//
// ArtifactIntegrity contains expected artifact verification state keyed by
// model file path. It is used to avoid repeating completed verification work.
//
// AdmissionTimeout limits how long a request waits for an admission permit.
// The timeout applies only to admission, not request processing after a permit
// is acquired. When unset or set to 0, the default is 3 minutes.
//
// AutoTune, when true, asks kronk.New to run a hardware-aware analysis of the
// model (architecture, size, and available devices) and seed unset settings
// (context window, KV cache type, slots, flash attention, split mode, etc.)
// before loading. It is off by default for backwards compatibility, and any
// option the caller sets explicitly always wins over the analysis. It has no
// effect when using the low-level model package directly (only kronk.New
// applies it).
//
// AutoTuned records that an upstream owner such as the Kronk Model Server has
// already applied AutoTune. When AutoTune and AutoTuned are both true,
// kronk.New preserves the enabled state for diagnostics without repeating the
// hardware analysis.
//
// BasePath sets the root for automatic chat-template discovery in <BasePath>/jinja.
// When empty, KRONK_BASE_PATH is used, falling back to ~/.kronk. An explicit
// path takes precedence over the environment and does not search other roots.
// It does not change model-file paths or separately configured library managers.
//
// CacheMinTokens sets the minimum token count required before caching. Messages
// shorter than this threshold are not cached, as the overhead of cache management
// may outweigh the prefill savings. When set to 0, defaults to 100 tokens.
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
// ContextWindow (often referred to as context length) is the maximum number of
// tokens that a large language model can process and consider at one time when
// generating a response. It defines the model's effective "memory" for a single
// conversation or text generation task.
// When set to 0, the default value is 4096.
//
// DefaultParams contains the default sampling parameters for requests.
//
// DecisionProtocol optionally overrides automatic decision-protocol detection.
// It is useful when a decision model has been renamed and its GGUF metadata
// does not identify the protocol.
//
// ChatTemplateKwargs contains model-level defaults passed only to the Jinja
// chat template. Request-level chat_template_kwargs override matching keys.
// Resolved first-class request parameters remain top-level template values.
//
// PtrDraftModel configures a separate speculative-decoding draft model or an
// nDraft override for an auto-detected MTP head.
//
// Devices is a list of device names to use for model execution. When multiple
// devices are specified, the model is distributed across them according to the
// SplitMode and TensorSplit configuration. Device names can be obtained from
// the output of llama-bench --list-devices (e.g., "CUDA0", "CUDA1", "Metal").
// When empty, the default device selection is used.
//
// PtrFlashAttention controls Flash Attention mode. Flash Attention reduces
// memory usage and speeds up attention computation, especially for large
// context windows. When nil, FlashAttentionAuto is used so llama.cpp can decide
// whether the active backend supports it. Set to FlashAttentionEnabled to force
// it on, or FlashAttentionDisabled to force it off.
//
// IMCSessionCapacity sets the number of reusable IMC session identities. When
// left unset or set to 0, generation models default to NSeqMax * QueueDepth.
// An explicit value must be at least that large so every admitted generation
// request can reserve a session.
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
// JinjaScript is an in-memory jinja template override. It takes precedence
// over JinjaFile and is intended for request-scoped model configurations.
//
// LazyMode controls on-demand reading of tensors marked eligible by llama.cpp.
// The default is LazyModeAuto. It applies to target and draft models. Effective
// lazy tables use a working-set allowance in the calculator's reservation.
// On Metal, mmap may bring an eligible table into RAM despite lazy reading;
// LoadModeNone can avoid that overlap.
//
// LoadMode controls how model weights are loaded. The default is LoadModeAuto,
// which uses mmap when every selected device supports it and otherwise uses
// ordinary loading. LoadModeNone disables mmap, which can improve tensor
// placement on multi-socket NUMA systems running MoE models with CPU experts.
// LoadModeMLock requests resident pages without forcing mmap, LoadModeMMapMLock
// combines mmap and mlock, and LoadModeDirectIO bypasses the page cache where
// the platform and filesystem support it.
//
// Log is the logger to use for model operations.
//
// MainGPU is the index of the GPU to use as the primary device when SplitMode
// is SplitModeNone. When nil, the default GPU (usually index 0) is used.
//
// PtrMoE controls expert-tensor placement for Mixture of Experts models.
//
// ModelFiles is the path to the model files. This is mandatory to provide.
//
// PrefillBatchSize is the maximum number of prompt tokens one prefill owner can
// contribute to a decode iteration. The default is 2048. Larger values can
// reduce the number of decode calls needed to reach generation, but each call
// takes longer before already-generating slots can run again and requires
// larger compute buffers. Kronk derives llama.cpp's logical and physical batch
// capacities from this value, the configured slot count, and the generation
// mode, with ImageBatchTokenCapacity providing an independent media floor.
//
// PhysicalBatchSize requests the physical compute capacity independently of
// the prompt contribution. When unset, it follows the legacy prefill capacity
// unless AutoTune selects a value. Image floors can raise it, logical capacity
// caps it, and active MTP and specialized evaluation retain their sizing policies.
// EffectiveNUBatch reports the resolved capacity; the request is preserved.
//
// ImageBatchTokenCapacity provides an independent image-capacity floor for
// native batches without increasing the text prefill contribution or changing
// image preprocessing. The default is zero (no extra floor).
// Oversized non-causal image chunks return an error instead of being split.
//
// ImageTokenBudget explicitly sets the preprocessing token budget for supported
// projectors. Lower values can reduce image resolution and detail. When unset,
// the projector's default is preserved. Explicit values must be positive and
// also contribute to native batch capacity. Not all projectors honor the budget,
// and final chunk sizes are still checked before decoding.
//
// NGpuLayers is the number of model layers to offload to the GPU. When set to 0,
// all layers are offloaded (default). Set to -1 to keep all layers on CPU. Any
// positive value specifies the exact number of layers to offload.
//
// NSeqMax controls concurrency behavior based on model type. For text inference
// models (including vision/audio), it sets the maximum number of generation
// slots. For supported embedding and reranking architectures, it sets the
// maximum sequence width of the sequence-batch engine. Other embedding and
// reranking architectures use it as the context-pool size. For decision
// models, it sets the maximum number of work items evaluated together on one
// context. When set to 0, a default of 1 is used.
//
// NThreads is the number of threads to use for generation. When unset or set
// to 0, it defaults to the greater of 4 and runtime.NumCPU().
//
// NThreadsBatch is the number of threads to use for batch processing. When
// unset or set to 0, it defaults to the greater of 4 and runtime.NumCPU().
//
// NUMA controls the NUMA (Non-Uniform Memory Access) strategy. This matters
// most when expert tensors are on CPU and the system has multiple NUMA nodes.
// Valid values: "" (disabled), "distribute", "isolate", "numactl", "mirror".
// "distribute" is recommended for multi-socket MoE setups; without it,
// cross-socket memory access can cause significant bandwidth collapse.
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
// MTPDrafterFile is the path to a separate-file MTP drafter GGUF that ships
// alongside the main model. Supported files are Gemma assistant heads that
// share target KV and Qwen35 heads that own their draft KV. It is not the main
// model or a vocab-matched classic draft model. The catalog wires this field
// when it downloads a compatible companion from the model repository.
//
// ProjOnCPU forces the multimodal projector (mmproj) to run on the CPU. When
// nil or false, the projector runs on whichever device llama.cpp picks by
// default (GPU when available). Set to true to keep the projector on the CPU
// — equivalent to llama-mtmd-cli's --no-mmproj-offload. The LLM itself is
// unaffected and still runs on whatever device WithNGpuLayers selects.
//
// ProjDevice names the backend device used by the multimodal projector
// (mmproj), such as "CUDA1" or "MTL0". When empty, llama.cpp selects the
// projector device automatically. It cannot be combined with ProjOnCPU=true.
// The LLM device selection is unaffected.
//
// QueueDepth sets the number of admitted request layers for generation and
// decision models.
// The admission capacity is NSeqMax * QueueDepth. A depth of 1 admits only the
// requests that can occupy execution slots; the default depth of 2 admits one
// additional waiting layer. QueueDepth also determines the default IMC session
// capacity for text inference models.
//
// RopeFreqBase overrides the RoPE base frequency. When nil, uses model default.
// Common values: 10000 (Llama), 1000000 (Qwen3).
//
// RopeFreqScale overrides the raw RoPE frequency multiplier. When nil, uses
// the value from model metadata. Kronk does not derive this value from
// ContextWindow; an N-times extension generally uses 1/N when the model's
// documentation requires explicit scaling.
//
// RecordArtifactVerification persists updated verification state after Kronk
// verifies a model artifact. When nil, verification state is not persisted.
//
// RopeScaling controls the RoPE scaling method for extended context support.
// Set to RopeScalingYaRN only when the model supports YaRN and configure the
// frequency scale required by that model.
//
// SessionStoreFactory constructs session stores for direct SDK use.
// Kronk invokes it separately for every working session store it needs and
// closes every successfully returned store. The factory must return a new,
// independent store on each call. The factory may capture shared,
// concurrency-safe resources such as a connection pool or process-wide quota.
// It receives the context for the operation that needs the store. Store read,
// prepare, commit, and reset operations also receive that context and return
// errors. System prompt preloads always remain in RAM. When nil, Kronk uses the
// built-in RAM factory. Backend-specific constructor parameters belong to the
// backend package and are captured by the injected factory.
//
// SplitMode controls how the model is split across multiple GPUs:
//   - SplitModeNone (0): single GPU
//   - SplitModeLayer (1): split layers and KV across GPUs
//   - SplitModeRow (2): legacy row-split parallelism
//   - SplitModeTensor (3): tensor parallelism
//
// When nil (not set), the default is SplitModeLayer, matching llama.cpp. Layer
// mode distributes a single GGUF across multiple GPUs without requiring the
// backend-specific support used by row and tensor modes.
//
// SWAFull controls whether models with sliding window attention (SWA) use a
// full-size KV cache for SWA layers instead of the memory-efficient small
// cache. When nil (default), llama.cpp's default is used.
// When explicitly set to false, SWA layers only cache the last n_swa tokens,
// saving significant VRAM but limiting context caching and shifting. When
// true, SWA layers use the full context window for their KV cache, preserving
// accuracy at the cost of higher memory usage.
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
// YarnAttnFactor sets the YaRN attention magnitude scaling factor. When nil,
// uses the model or llama.cpp default.
//
// YarnBetaFast sets the YaRN low correction dimension. When nil, uses the
// model or llama.cpp default.
//
// YarnBetaSlow sets the YaRN high correction dimension. When nil, uses the
// model or llama.cpp default.
//
// YarnExtFactor sets the YaRN extrapolation mix factor. When nil, uses the
// model or llama.cpp default. Set to 0 to disable extrapolation.
//
// YarnOrigCtx sets the original training context size for YaRN scaling. When nil
// or 0, uses the model's native training context length from metadata.
type Config struct {
	Adapters                   []AdapterConfig
	ArtifactIntegrity          map[string]ArtifactIntegrity
	PtrAdmissionTimeout        *time.Duration
	AutoTune                   bool
	AutoTuned                  bool
	BasePath                   string
	PtrCacheMinTokens          *int
	CacheTypeK                 GGMLType
	CacheTypeV                 GGMLType
	PtrContextWindow           *int
	DefaultParams              Params
	DecisionProtocol           DecisionProtocol
	ChatTemplateKwargs         D
	PtrDraftModel              *DraftModelConfig
	Devices                    []string // Device names for model execution (e.g., ["CUDA0", "CUDA1"])
	PtrFlashAttention          *FlashAttentionType
	PtrIMCSessionCapacity      *int
	PtrImageBatchTokenCapacity *int
	PtrImageTokenBudget        *int
	PtrIncrementalCache        *bool
	PtrInsecureLogging         *bool
	JinjaFile                  string
	JinjaScript                string
	PtrKVCacheMode             *KVCacheMode
	LazyMode                   LazyMode
	LoadMode                   LoadMode
	Log                        applog.Logger
	PtrMainGPU                 *int
	PtrMoE                     *MoEConfig
	ModelFiles                 []string
	PtrNGpuLayers              *int
	PtrNSeqMax                 *int
	PtrNThreads                *int
	PtrNThreadsBatch           *int
	PtrPrefillBatchSize        *int
	PtrPhysicalBatchSize       *int
	NUMA                       NUMAStrategy
	PtrOffloadKQV              *bool
	PtrOpOffload               *bool
	ProjFile                   string
	MTPDrafterFile             string
	PtrProjOnCPU               *bool
	ProjDevice                 string
	PtrQueueDepth              *int
	ResponseModelID            string
	PtrRopeFreqBase            *float32
	PtrRopeFreqScale           *float32
	RecordArtifactVerification ArtifactVerificationRecorder
	RopeScaling                RopeScalingType
	SessionStoreFactory        SessionStoreFactory
	Speculation                SpeculationMode
	PtrSplitMode               *SplitMode
	PtrSWAFull                 *bool
	TensorBuftOverrides        []string
	TensorSplit                []float32
	PtrYarnAttnFactor          *float32
	PtrYarnBetaFast            *float32
	PtrYarnBetaSlow            *float32
	PtrYarnExtFactor           *float32
	PtrYarnOrigCtx             *int
	nBatch                     int
	nUBatch                    int
	speculationPlan            speculationPlan
}

func (cfg Config) AdmissionTimeout() time.Duration {
	if cfg.PtrAdmissionTimeout == nil {
		return 0
	}
	return *cfg.PtrAdmissionTimeout
}
func (cfg Config) QueueDepth() int         { return intOr(cfg.PtrQueueDepth, 0) }
func (cfg Config) IMCSessionCapacity() int { return intOr(cfg.PtrIMCSessionCapacity, 0) }
func (cfg Config) ContextWindow() int      { return intOr(cfg.PtrContextWindow, 0) }
func (cfg Config) PrefillBatchSize() int   { return intOr(cfg.PtrPrefillBatchSize, 0) }

// PhysicalBatchSize returns the requested physical capacity, or zero when unset.
func (cfg Config) PhysicalBatchSize() int { return intOr(cfg.PtrPhysicalBatchSize, 0) }

// KVCacheMode returns the cache layout, defaulting to partitioned.
func (cfg Config) KVCacheMode() KVCacheMode {
	if cfg.PtrKVCacheMode == nil {
		return KVCachePartitioned
	}
	return *cfg.PtrKVCacheMode
}

// ImageBatchTokenCapacity returns the explicit image-capacity floor, or zero.
func (cfg Config) ImageBatchTokenCapacity() int { return intOr(cfg.PtrImageBatchTokenCapacity, 0) }

// ImageTokenBudget returns the explicit preprocessing budget, or zero when unset.
func (cfg Config) ImageTokenBudget() int { return intOr(cfg.PtrImageTokenBudget, 0) }

// BatchBaseSize returns the native capacity base without the generation reserve.
// The text scheduler continues to use PrefillBatchSize independently.
func (cfg Config) BatchBaseSize() int {
	prefill := cfg.PrefillBatchSize()
	if prefill <= 0 {
		prefill = DefaultPrefillBatchSize
	}
	return max(prefill, cfg.ImageBatchTokenCapacity(), cfg.ImageTokenBudget())
}

// EffectiveNBatch returns the derived llama.cpp logical batch capacity.
func (cfg Config) EffectiveNBatch() int { return cfg.nBatch }

// EffectiveNUBatch returns the derived llama.cpp physical batch capacity.
func (cfg Config) EffectiveNUBatch() int { return cfg.nUBatch }

func (cfg Config) NSeqMax() int            { return intOr(cfg.PtrNSeqMax, 0) }
func (cfg Config) NThreads() int           { return intOr(cfg.PtrNThreads, 0) }
func (cfg Config) NThreadsBatch() int      { return intOr(cfg.PtrNThreadsBatch, 0) }
func (cfg Config) CacheMinTokens() int     { return intOr(cfg.PtrCacheMinTokens, 0) }
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

// SpeculationMode returns the selected speculative-decoding implementation.
// The zero value preserves automatic selection.
func (cfg Config) SpeculationMode() SpeculationMode {
	if cfg.Speculation.IsZero() {
		return SpeculationAuto
	}
	return cfg.Speculation
}

// FlashAttention returns the configured flash attention mode. An unset value
// defaults to auto so llama.cpp can select the supported mode.
func (cfg Config) FlashAttention() FlashAttentionType {
	return DerefFlashAttention(cfg.PtrFlashAttention)
}

// SWAFull reports whether a full-size sliding-window attention cache was
// explicitly requested. Callers that need the effective value must check
// PtrSWAFull first because nil leaves the choice to llama.cpp.
func (cfg Config) SWAFull() bool { return boolOr(cfg.PtrSWAFull, false) }

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

	formatDurationPtr := func(p *time.Duration) string {
		if p == nil {
			return "nil"
		}
		return p.String()
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

	return fmt.Sprintf("\nPhysicalBatchSize[%s]", formatIntPtr(cfg.PtrPhysicalBatchSize)) + fmt.Sprintf("\nAdapters[%v]\nAdmissionTimeout[%s]\nAutoTune[%t]\nCacheMinTokens[%s]\nCacheTypeK[%s]\nCacheTypeV[%s]\nContextWindow[%s]\nDefaultParams[%s]\nDecisionProtocol[%s]\nChatTemplateKwargs[%s]\nDevices[%v]\nFlashAttention[%s]\nIMCSessionCapacity[%d]\nIncrementalCache[%s]\nInsecureLogging[%s]\nJinjaFile[%s]\nLazyMode[%s]\nLoadMode[%s]\nMainGPU[%s]\nMoE[%s]\nModelFiles[%v]\nNGpuLayers[%s]\nNSeqMax[%s]\nNThreads[%s]\nNThreadsBatch[%s]\nPrefillBatchSize[%s]\nEffectiveNBatch[%d]\nEffectiveNUBatch[%d]\nNUMA[%s]\nOffloadKQV[%s]\nOpOffload[%s]\nProjFile[%s]\nMTPDrafterFile[%s]\nProjOnCPU[%s]\nProjDevice[%s]\nQueueDepth[%d]\nRopeFreqBase[%s]\nRopeFreqScale[%s]\nRopeScaling[%s]\nSessionStoreFactory[%t]\nSpeculation[%s]\nSplitMode[%s]\nSWAFull[%s]\nTensorBuftOverrides[%v]\nTensorSplit[%v]\nYarnAttnFactor[%s]\nYarnBetaFast[%s]\nYarnBetaSlow[%s]\nYarnExtFactor[%s]\nYarnOrigCtx[%s]\nDraftModel[%v]\n",
		cfg.Adapters, formatDurationPtr(cfg.PtrAdmissionTimeout), cfg.AutoTune, formatIntPtr(cfg.PtrCacheMinTokens), cfg.CacheTypeK, cfg.CacheTypeV,
		formatIntPtr(cfg.PtrContextWindow), cfg.DefaultParams.String(), cfg.DecisionProtocol, chatTemplateKwargsSummary(cfg.ChatTemplateKwargs), cfg.Devices, cfg.FlashAttention(),
		cfg.IMCSessionCapacity(), formatBoolPtr(cfg.PtrIncrementalCache), formatBoolPtr(cfg.PtrInsecureLogging), cfg.JinjaFile,
		cfg.LazyMode, cfg.LoadMode, formatIntPtr(cfg.PtrMainGPU), formatMoEPtr(cfg.PtrMoE), cfg.ModelFiles,
		formatIntPtr(cfg.PtrNGpuLayers), formatIntPtr(cfg.PtrNSeqMax), formatIntPtr(cfg.PtrNThreads), formatIntPtr(cfg.PtrNThreadsBatch), formatIntPtr(cfg.PtrPrefillBatchSize), cfg.EffectiveNBatch(), cfg.EffectiveNUBatch(),
		cfg.NUMA,
		formatBoolPtr(cfg.PtrOffloadKQV), formatBoolPtr(cfg.PtrOpOffload), cfg.ProjFile, cfg.MTPDrafterFile, formatBoolPtr(cfg.PtrProjOnCPU), cfg.ProjDevice, cfg.QueueDepth(),
		formatFloat32Ptr(cfg.PtrRopeFreqBase), formatFloat32Ptr(cfg.PtrRopeFreqScale), cfg.RopeScaling,
		cfg.SessionStoreFactory != nil, cfg.SpeculationMode(),
		formatSplitModePtr(cfg.PtrSplitMode),
		formatBoolPtr(cfg.PtrSWAFull), cfg.TensorBuftOverrides, cfg.TensorSplit,
		formatFloat32Ptr(cfg.PtrYarnAttnFactor),
		formatFloat32Ptr(cfg.PtrYarnBetaFast), formatFloat32Ptr(cfg.PtrYarnBetaSlow), formatFloat32Ptr(cfg.PtrYarnExtFactor), formatIntPtr(cfg.PtrYarnOrigCtx), cfg.PtrDraftModel)
}

func chatTemplateKwargsSummary(kwargs D) string {
	if len(kwargs) == 0 {
		return "none"
	}

	keys := slices.Sorted(maps.Keys(kwargs))
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		value := "configured"
		if boolean, ok := kwargs[key].(bool); ok {
			value = strconv.FormatBool(boolean)
		}
		values = append(values, key+"="+value)
	}

	return strings.Join(values, " ")
}

func validateConfig(ctx context.Context, cfg Config, log applog.Logger) error {
	if len(cfg.ModelFiles) == 0 {
		return fmt.Errorf("validate-config: model file is required")
	}

	if _, err := ParseKVCacheMode(cfg.KVCacheMode().String()); err != nil {
		return fmt.Errorf("validate-config: %w", err)
	}

	switch cfg.DecisionProtocol {
	case DecisionProtocol{}, DecisionProtocolOpenJEV, DecisionProtocolJevStyle, DecisionProtocolLaya, DecisionProtocolLev, DecisionProtocolKev, DecisionProtocolNimble, DecisionProtocolClef, DecisionProtocolPplxDecider, DecisionProtocolLfm2D1, DecisionProtocolVllmDecision2, DecisionProtocolVllmD3:
	default:
		return fmt.Errorf("validate-config: unsupported decision protocol %q", cfg.DecisionProtocol)
	}

	if cfg.QueueDepth() < 0 {
		return fmt.Errorf("validate-config: queue depth must be >= 0, got %d", cfg.QueueDepth())
	}

	if cfg.IMCSessionCapacity() < 0 {
		return fmt.Errorf("validate-config: IMC session capacity must be >= 0, got %d", cfg.IMCSessionCapacity())
	}

	if cfg.IMCSessionCapacity() > 0 && !isEmbedOrRerankConfig(cfg) {
		nSeqMax := max(cfg.NSeqMax(), defNSeqMax)
		queueDepth := cfg.QueueDepth()
		if queueDepth == 0 {
			queueDepth = DefaultQueueDepth
		}
		admissionCapacity := nSeqMax * queueDepth
		if cfg.IMCSessionCapacity() < admissionCapacity {
			return fmt.Errorf("validate-config: IMC session capacity must be >= admission capacity %d, got %d", admissionCapacity, cfg.IMCSessionCapacity())
		}
	}

	if cfg.AdmissionTimeout() < 0 {
		return fmt.Errorf("validate-config: admission timeout must be >= 0, got %s", cfg.AdmissionTimeout())
	}

	if cfg.PtrPrefillBatchSize != nil && cfg.PrefillBatchSize() <= 0 {
		return fmt.Errorf("validate-config: prefill batch size must be > 0, got %d", cfg.PrefillBatchSize())
	}

	if cfg.PtrPhysicalBatchSize != nil && (cfg.PhysicalBatchSize() <= 0 || cfg.PhysicalBatchSize() > math.MaxInt32) {
		return fmt.Errorf("validate-config: physical batch size must be between 1 and %d, got %d", math.MaxInt32, cfg.PhysicalBatchSize())
	}

	if cfg.ImageBatchTokenCapacity() < 0 {
		return fmt.Errorf("validate-config: image batch token capacity must be >= 0, got %d", cfg.ImageBatchTokenCapacity())
	}

	if cfg.PtrImageTokenBudget != nil && (cfg.ImageTokenBudget() <= 0 || cfg.ImageTokenBudget() > math.MaxInt32) {
		return fmt.Errorf("validate-config: image token budget must be between 1 and %d, got %d", math.MaxInt32, cfg.ImageTokenBudget())
	}

	if cfg.ProjDevice != "" && cfg.PtrProjOnCPU != nil && *cfg.PtrProjOnCPU {
		return fmt.Errorf("validate-config: projector device cannot be combined with proj-on-cpu=true")
	}

	if cfg.MoECacheSize() < 0 {
		return fmt.Errorf("validate-config: moe cache-size must be >= 0")
	}

	switch cfg.CacheTypeV {
	case GGMLTypeQ4_0, GGMLTypeQ4_1, GGMLTypeQ5_0, GGMLTypeQ5_1, GGMLTypeQ8_0:
		if cfg.FlashAttention() == FlashAttentionDisabled {
			return fmt.Errorf("validate-config: quantized V cache type %s requires Flash Attention to be enabled or auto", cfg.CacheTypeV)
		}
	}

	seenAdapters := make(map[string]struct{}, len(cfg.Adapters))
	for i, adapter := range cfg.Adapters {
		if !filepath.IsAbs(adapter.Path) {
			return fmt.Errorf("validate-config: adapter[%d] path must be absolute: %q", i, adapter.Path)
		}
		if !strings.EqualFold(filepath.Ext(adapter.Path), ".gguf") {
			return fmt.Errorf("validate-config: adapter[%d] path must have a .gguf extension: %q", i, adapter.Path)
		}
		if adapter.Scale < 0 || math.IsNaN(float64(adapter.Scale)) || math.IsInf(float64(adapter.Scale), 0) {
			return fmt.Errorf("validate-config: adapter[%d] scale must be finite and >= 0, got %g", i, adapter.Scale)
		}

		adapterPath := filepath.Clean(adapter.Path)
		if _, exists := seenAdapters[adapterPath]; exists {
			return fmt.Errorf("validate-config: duplicate adapter path: %q", adapterPath)
		}
		seenAdapters[adapterPath] = struct{}{}

		fi, err := os.Stat(adapterPath)
		if err != nil {
			return fmt.Errorf("validate-config: adapter[%d] path %q: %w", i, adapterPath, err)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("validate-config: adapter[%d] path is not a regular file: %q", i, adapterPath)
		}
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

	switch cfg.LazyMode {
	case LazyMode{}, LazyModeAuto, LazyModeOff, LazyModeOn:
		// valid
	default:
		return fmt.Errorf("validate-config: unknown lazy mode: %s (valid: auto, off, on)", cfg.LazyMode)
	}

	switch cfg.LoadMode {
	case LoadMode{}, LoadModeAuto, LoadModeMMap, LoadModeNone, LoadModeMLock, LoadModeMMapMLock, LoadModeDirectIO:
		// valid
	default:
		return fmt.Errorf("validate-config: unknown load mode: %s (valid: auto, mmap, none, mlock, mmap+mlock, direct-io)", cfg.LoadMode)
	}

	switch cfg.SpeculationMode() {
	case SpeculationAuto, SpeculationDisabled, SpeculationClassic, SpeculationMTP:
		// valid
	default:
		return fmt.Errorf("validate-config: unknown speculation mode %q (valid: auto, disabled, classic, mtp)", cfg.Speculation)
	}

	if cfg.SpeculationMode() != SpeculationDisabled && cfg.PtrDraftModel != nil {
		if cfg.PtrDraftModel.IsSeparate() {
			// Separate-GGUF drafter: requires single-slot mode and a
			// readable draft GGUF that shares the target's tokenizer.
			if cfg.NSeqMax() > 1 {
				return fmt.Errorf("validate-config: speculative decoding requires NSeqMax=1, got %d", cfg.NSeqMax())
			}
			for _, modelFile := range cfg.PtrDraftModel.ModelFiles {
				log(ctx, "validate-config", "draft-model-file", modelFile)
				if err := verifyConfiguredArtifact(ctx, cfg, log, modelFile); err != nil {
					return fmt.Errorf("validate-config: draft model: %w", err)
				}
			}
		} else {
			// MTP nDraft override (no model files): only the draft token
			// count is configurable here. It applies when the target ships
			// an auto-detected MTP head; on a target without one it is a
			// harmless no-op.
			if cfg.PtrDraftModel.NDraft < 0 {
				return fmt.Errorf("validate-config: draft model ndraft must be >= 0, got %d", cfg.PtrDraftModel.NDraft)
			}
		}
	}

	if cfg.PtrMoE != nil {
		switch cfg.PtrMoE.Mode {
		case MoEMode{}, MoEModeAuto, MoEModeExpertsCPU, MoEModeExpertsGPU, MoEModeKeepTopN, MoEModeCustom:
			// valid
		default:
			return fmt.Errorf("validate-config: unknown MoE mode: %s (valid: auto, experts_cpu, experts_gpu, keep_top_n, custom)", cfg.PtrMoE.Mode)
		}

		if cfg.PtrMoE.Mode == MoEModeKeepTopN && cfg.PtrMoE.PtrKeepExpertsOnGPUForTopNLayers == nil {
			return fmt.Errorf("validate-config: MoE mode keep_top_n requires KeepExpertsOnGPUForTopNLayers to be set")
		}

		if cfg.PtrMoE.PtrKeepExpertsOnGPUForTopNLayers != nil && *cfg.PtrMoE.PtrKeepExpertsOnGPUForTopNLayers < 0 {
			return fmt.Errorf("validate-config: MoE KeepExpertsOnGPUForTopNLayers must be >= 0, got %d", *cfg.PtrMoE.PtrKeepExpertsOnGPUForTopNLayers)
		}

		if !cfg.PtrMoE.Mode.IsZero() && cfg.PtrMoE.Mode != MoEModeAuto && cfg.PtrMoE.Mode != MoEModeCustom && len(cfg.TensorBuftOverrides) > 0 {
			return fmt.Errorf("validate-config: MoE mode %s and TensorBuftOverrides are mutually exclusive; use MoE mode 'custom' with TensorBuftOverrides", cfg.PtrMoE.Mode)
		}
	}

	for _, modelFile := range cfg.ModelFiles {
		log(ctx, "validate-config", "model-file", modelFile)

		if err := verifyConfiguredArtifact(ctx, cfg, log, modelFile); err != nil {
			return fmt.Errorf("validate-config: %w", err)
		}
	}

	if cfg.ProjFile != "" {
		log(ctx, "validate-config", "model-file", cfg.ProjFile)

		if err := verifyConfiguredArtifact(ctx, cfg, log, cfg.ProjFile); err != nil {
			return fmt.Errorf("validate-config: prog-file[%s]: %w", cfg.ProjFile, err)
		}
	}

	if cfg.SpeculationMode() != SpeculationDisabled && cfg.SpeculationMode() != SpeculationClassic && cfg.MTPDrafterFile != "" {
		log(ctx, "validate-config", "model-file", cfg.MTPDrafterFile)

		if err := verifyConfiguredArtifact(ctx, cfg, log, cfg.MTPDrafterFile); err != nil {
			return fmt.Errorf("validate-config: mtp-drafter-file[%s]: %w", cfg.MTPDrafterFile, err)
		}
	}

	return nil
}

func verifyConfiguredArtifact(ctx context.Context, cfg Config, log applog.Logger, modelFile string) error {
	integrity, exists := cfg.ArtifactIntegrity[modelFile]
	if !exists {
		return nil
	}

	verification, changed, err := VerifyArtifact(modelFile, integrity.Digest, integrity.Verification, true)
	if err != nil {
		return err
	}
	if changed && cfg.RecordArtifactVerification != nil {
		if err := cfg.RecordArtifactVerification(modelFile, verification); err != nil {
			log(ctx, "record-artifact-verification", "model-file", modelFile, "ERROR", err)
		}
	}

	return nil
}

func adjustQueueDepth(cfg Config) Config {
	if cfg.QueueDepth() == 0 {
		cfg.PtrQueueDepth = new(DefaultQueueDepth)
	}

	return cfg
}

func adjustAdmissionTimeout(cfg Config) Config {
	if cfg.AdmissionTimeout() == 0 {
		cfg.PtrAdmissionTimeout = new(defaultAdmissionTimeout)
	}

	return cfg
}

func adjustIMCSessionCapacity(cfg Config) Config {
	if cfg.IMCSessionCapacity() == 0 && !isEmbedOrRerankConfig(cfg) {
		capacity := imcSessionCapacity(cfg.NSeqMax(), cfg.QueueDepth())
		cfg.PtrIMCSessionCapacity = new(capacity)
	}

	return cfg
}

func adjustConfig(cfg Config, model llama.Model) Config {
	cfg = adjustQueueDepth(cfg)
	cfg = adjustAdmissionTimeout(cfg)

	cfg = adjustContextWindow(cfg, model)

	// Resolve the number of parallel sequence slots first; the batch sizing
	// below derives n_batch from it.
	if cfg.NSeqMax() <= 0 {
		cfg.PtrNSeqMax = new(defNSeqMax)
	}
	cfg = adjustIMCSessionCapacity(cfg)

	// Default the scheduler's text contribution independently of native media
	// capacity. Generation sizing adds the image floor and per-slot reserve.
	if cfg.PrefillBatchSize() <= 0 {
		cfg.PtrPrefillBatchSize = new(DefaultPrefillBatchSize)
	}

	// Logical batch size (n_batch) is the shared "tray" the batch engine fills.
	// Generation models reserve output rows after one full prefill contribution.
	// The scheduler serves one prefill owner at a time, so the prefill capacity
	// is not multiplied by NSeqMax; only the per-slot reserve is. Non-MTP
	// leaves n_ubatch at the prefill capacity and may split the logical tray.
	// MTP sizing is finalized after model loading, when MTP availability is
	// known. Pooled embedding and reranking evaluation use n_ubatch directly
	// because they do not mix generation and prefill rows.
	if isEmbedOrRerankConfig(cfg) {
		cfg.nBatch = cfg.PrefillBatchSize()
		cfg.nUBatch = cfg.PrefillBatchSize()
	} else {
		cfg = adjustGenerationBatch(cfg, 1, false)
	}

	nThreads := max(defNThreads, runtime.NumCPU())
	if cfg.NThreads() <= 0 {
		cfg.PtrNThreads = new(nThreads)
	}

	if cfg.NThreadsBatch() <= 0 {
		cfg.PtrNThreadsBatch = new(nThreads)
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

	if cfg.PtrDraftModel != nil && cfg.PtrDraftModel.NDraft <= 0 {
		draft := *cfg.PtrDraftModel
		cfg.PtrDraftModel = &draft
		// Separate-GGUF drafts default to defNDraft; MTP nDraft overrides
		// (no model files) default to defMTPNDraft.
		if cfg.PtrDraftModel.IsSeparate() {
			cfg.PtrDraftModel.NDraft = defNDraft
		} else {
			cfg.PtrDraftModel.NDraft = defMTPNDraft
		}
	}

	// Ensure remaining pointer fields are non-nil after adjustment.
	if cfg.PtrInsecureLogging == nil {
		cfg.PtrInsecureLogging = new(false)
	}
	return cfg
}

// adjustGenerationBatch derives llama.cpp's logical and physical generation
// batch capacities from the user-facing prefill size and generation reserve.
func adjustGenerationBatch(cfg Config, generationRowsPerSlot int, singlePhysicalBatch bool) Config {
	cfg.nBatch, cfg.nUBatch = cfg.GenerationBatchSizes(generationRowsPerSlot, singlePhysicalBatch)
	return cfg
}

// GenerationBatchSizes derives native batch capacities for both memory planning
// and runtime configuration. Requests do not change the logical generation reserve.
// MTP preserves one physical batch including rollback rows. A zero row count
// selects the configured/default MTP reserve when singlePhysicalBatch is true.
func (cfg Config) GenerationBatchSizes(generationRowsPerSlot int, singlePhysicalBatch bool) (int, int) {
	if isEmbedOrRerankConfig(cfg) || !cfg.DecisionProtocol.IsZero() {
		base := cfg.BatchBaseSize()
		return base, base
	}
	if singlePhysicalBatch && generationRowsPerSlot <= 0 {
		generationRowsPerSlot = 1 + mtpNDraft(cfg)
	}
	base := cfg.BatchBaseSize()
	nBatch := base + max(cfg.NSeqMax(), 1)*max(generationRowsPerSlot, 1)
	nUBatch := base
	if cfg.PtrPhysicalBatchSize != nil {
		nUBatch = min(nBatch, max(cfg.PhysicalBatchSize(), cfg.ImageBatchTokenCapacity(), cfg.ImageTokenBudget()))
	}
	if singlePhysicalBatch {
		nUBatch = nBatch
	}
	return nBatch, nUBatch
}

func isEmbedOrRerankConfig(cfg Config) bool {
	if len(cfg.ModelFiles) == 0 {
		return false
	}

	isEmbed, isRerank := detectEmbedRerank(modelIDFromFiles(cfg.ModelFiles))

	return isEmbed || isRerank
}

func batchSeqTokenLimit(nBatch, nUBatch int) int {
	switch {
	case nBatch <= 0:
		return max(nUBatch, 0)
	case nUBatch <= 0:
		return nBatch
	default:
		return min(nBatch, nUBatch)
	}
}

func adjustContextWindow(cfg Config, model llama.Model) Config {

	// User explicitly set the context window — honor it as-is.
	if cfg.ContextWindow() > 0 {
		return cfg
	}

	// Discover the model's max trained context window from GGUF metadata.
	// Fall back to the package default if the key is missing or malformed.
	modelCW := defContextWindow
	v, found := searchModelMeta(model, "context_length")
	if found {
		ctxLen, err := strconv.Atoi(v)
		if err == nil {
			modelCW = ctxLen
		}
	}

	// Cap the auto-picked context window at defContextWindow (8K). Many
	// modern models advertise 128K–1M context in their GGUF metadata, but
	// allocating that much KV by default tanks TPS as the conversation
	// grows and bloats first-token latency. Users who actually want a
	// large context can opt in via WithContextWindow.
	pickedCW := min(modelCW, defContextWindow)

	cfg.PtrContextWindow = &pickedCW

	if cfg.Log != nil {
		cfg.Log(context.Background(), "adjust-context-window",
			"model-max", modelCW,
			"default-cap", defContextWindow,
			"picked", pickedCW,
			"note", "auto-selected; override with WithContextWindow",
		)
	}

	return cfg
}

func modelCtxParams(cfg Config, mi ModelInfo) llama.ContextParams {
	ctxParams := llama.ContextDefaultParams()

	if mi.profile.MoE.IsMoE && cfg.MoECacheSize() > 0 {
		ctxParams.MoeCacheSize = uint64(cfg.MoECacheSize())
	}

	if mi.IsEmbedModel || mi.IsRerankModel {
		ctxParams.Embeddings = 1
	}

	if mi.IsRerankModel {
		ctxParams.PoolingType = llama.PoolingTypeRank
	}

	// IMC externalizes KV state after cache build, so it does not
	// need extra sequences beyond the configured NSeqMax.
	nSeqMax := max(cfg.NSeqMax(), 1)
	ctxParams = contextTopologyParams(ctxParams, cfg.ContextWindow(), nSeqMax, cfg.KVCacheMode())

	if cfg.ContextWindow() > 0 {
		ctxParams.NBatch = uint32(cfg.EffectiveNBatch())
		ctxParams.NUbatch = uint32(cfg.EffectiveNUBatch())
		if cfg.NThreads() > 0 {
			ctxParams.NThreads = int32(cfg.NThreads())
		}
		if cfg.NThreadsBatch() > 0 {
			ctxParams.NThreadsBatch = int32(cfg.NThreadsBatch())
		}
	}

	if !cfg.CacheTypeK.Equal(GGMLTypeAuto) {
		ctxParams.TypeK = cfg.CacheTypeK.ToYZMAType()
	}

	if !cfg.CacheTypeV.Equal(GGMLTypeAuto) {
		ctxParams.TypeV = cfg.CacheTypeV.ToYZMAType()
	}

	ctxParams.FlashAttentionType = cfg.FlashAttention().toYZMAType()

	// NOutputsMax caps how many logits-flagged rows llama.cpp reserves
	// per ubatch. NOutputsMaxPerSeq gives graph reservation the matching
	// per-sequence bound instead of letting one sequence claim the total.
	// The defaults reserve n_batch rows, which is wasteful for server-style
	// decoding where only a handful of tokens per slot ever need logits.
	// Capping at the actual upper bound frees significant VRAM at large
	// -ub / MTP. See ggml-org/llama.cpp PR 23861 (1.2 GB saved at -ub 2048
	// with MTP).
	//
	// Embed / rerank models pool logits across the whole prompt, so leave
	// both limits at 0 (= n_batch) to avoid clipping the pooled outputs.
	if !mi.IsEmbedModel && !mi.IsRerankModel {
		// Plain decode and IMC prefill mark logits on at most one token
		// per slot per ubatch (last prompt token during prefill, sampled
		// token during decode).
		perSlot := 1
		rollbackDepth := 0

		if cfg.speculationPlan.Active() {
			perSlot = cfg.speculationPlan.RowsPerSequence()
			rollbackDepth = cfg.speculationPlan.NDraft
		}

		ctxParams.NOutputsMax = uint32(nSeqMax * perSlot)
		ctxParams.NOutputsMaxPerSeq = uint32(perSlot)
		if mi.Type == ModelTypeHybrid {
			ctxParams.NRsSeq = uint32(rollbackDepth)
		}
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
	// When nil, preserve the value returned by llama.cpp's context defaults.
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
	if !cfg.RopeScaling.Equal(RopeScalingNone) {
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

func contextTopologyParams(params llama.ContextParams, contextWindow, nSeqMax int, mode KVCacheMode) llama.ContextParams {
	if contextWindow > 0 {
		params.NCtx = uint32(contextWindow * nSeqMax)
	}

	params.NSeqMax = uint32(nSeqMax)
	params.KVUnified = 0
	if mode == KVCacheShared {
		params.KVUnified = 1
	}

	return params
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

	// PtrCacheSize sets the experimental GPU expert-cache budget in bytes per
	// context, shared across GPUs. Zero uses llama.cpp's default (disabled).
	// Original host weights remain resident. Ignored for non-MoE models.
	PtrCacheSize *int64 `yaml:"cache-size,omitempty"`
}

func (m MoEConfig) KeepExpertsOnGPUForTopNLayers() int {
	return intOr(m.PtrKeepExpertsOnGPUForTopNLayers, 0)
}

// MoECacheSize returns the requested GPU expert-cache budget in bytes.
func (cfg Config) MoECacheSize() int64 {
	if cfg.PtrMoE == nil || cfg.PtrMoE.PtrCacheSize == nil {
		return 0
	}
	return *cfg.PtrMoE.PtrCacheSize
}

// ExpertsAllOnGPU is the sentinel value used for vram.Config.ExpertLayersOnGPU
// to request that every routed-expert layer be charged against GPU VRAM.
// The vram package treats any value greater than or equal to the model's
// block count as "all layers on GPU", so a large constant works
// regardless of model depth and avoids a metadata round-trip just to
// discover it.
const ExpertsAllOnGPU = int64(math.MaxInt32)

// ExpertLayersOnGPU translates the model's MoE configuration into the
// value the vram calculator expects so every prediction site (resman
// planner, post-load model logging, BUI display) reflects what
// llama.cpp will actually do at runtime. With no MoE override we mirror
// llama.cpp's default behavior: experts follow the layer they belong to,
// and full GPU offload puts every expert on the GPU. Without this
// resolution the calculator defaults experts to CPU even when the
// runtime puts them on GPU, producing the inverse of the placement
// that's actually loaded and silently under-accounting expert weight
// memory in the BUI VRAM column.
func (cfg Config) ExpertLayersOnGPU() int64 {
	if cfg.PtrMoE == nil {
		return ExpertsAllOnGPU
	}
	switch cfg.PtrMoE.Mode {
	case MoEModeExpertsCPU:
		return 0
	case MoEModeKeepTopN:
		return int64(cfg.PtrMoE.KeepExpertsOnGPUForTopNLayers())
	default:
		// MoEModeExpertsGPU, MoEModeAuto, MoEModeCustom, or zero value.
		return ExpertsAllOnGPU
	}
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

func WithConfig(src Config) Option {
	return func(c *Config) {
		*c = src
	}
}

func WithAdapters(v []AdapterConfig) Option { return func(c *Config) { c.Adapters = v } }

func WithAdmissionTimeout(v time.Duration) Option {
	return func(c *Config) { c.PtrAdmissionTimeout = new(v) }
}

func WithAutoTune(v bool) Option        { return func(c *Config) { c.AutoTune = v } }
func WithBasePath(v string) Option      { return func(c *Config) { c.BasePath = v } }
func WithCacheMinTokens(v int) Option   { return func(c *Config) { c.PtrCacheMinTokens = new(v) } }
func WithCacheTypeK(v GGMLType) Option  { return func(c *Config) { c.CacheTypeK = v } }
func WithCacheTypeV(v GGMLType) Option  { return func(c *Config) { c.CacheTypeV = v } }
func WithContextWindow(v int) Option    { return func(c *Config) { c.PtrContextWindow = new(v) } }
func WithDefaultParams(v Params) Option { return func(c *Config) { c.DefaultParams = v } }

// WithDecisionProtocol overrides automatic decision-protocol detection.
func WithDecisionProtocol(v DecisionProtocol) Option {
	return func(c *Config) { c.DecisionProtocol = v }
}

func WithChatTemplateKwargs(v D) Option {
	return func(c *Config) { c.ChatTemplateKwargs = v.Clone() }
}

func WithDevices(v []string) Option             { return func(c *Config) { c.Devices = v } }
func WithDraftModel(v *DraftModelConfig) Option { return func(c *Config) { c.PtrDraftModel = v } }

func WithFlashAttention(v FlashAttentionType) Option {
	return func(c *Config) { c.PtrFlashAttention = new(v) }
}
func WithIMCSessionCapacity(v int) Option {
	return func(c *Config) { c.PtrIMCSessionCapacity = new(v) }
}

// WithImageBatchTokenCapacity sets native image capacity without changing image detail.
func WithImageBatchTokenCapacity(v int) Option {
	return func(c *Config) { c.PtrImageBatchTokenCapacity = new(v) }
}

// WithImageTokenBudget explicitly sets the projector's image preprocessing budget.
func WithImageTokenBudget(v int) Option {
	return func(c *Config) { c.PtrImageTokenBudget = new(v) }
}
func WithIncrementalCache(v bool) Option       { return func(c *Config) { c.PtrIncrementalCache = new(v) } }
func WithInsecureLogging(v bool) Option        { return func(c *Config) { c.PtrInsecureLogging = new(v) } }
func WithJinjaFile(v string) Option            { return func(c *Config) { c.JinjaFile = v } }
func WithJinjaScript(v string) Option          { return func(c *Config) { c.JinjaScript = v } }
func WithKVCacheMode(v KVCacheMode) Option     { return func(c *Config) { c.PtrKVCacheMode = new(v) } }
func WithLazyMode(v LazyMode) Option           { return func(c *Config) { c.LazyMode = v } }
func WithLoadMode(v LoadMode) Option           { return func(c *Config) { c.LoadMode = v } }
func WithLog(v applog.Logger) Option           { return func(c *Config) { c.Log = v } }
func WithMainGPU(v int) Option                 { return func(c *Config) { c.PtrMainGPU = new(v) } }
func WithMoE(v *MoEConfig) Option              { return func(c *Config) { c.PtrMoE = v } }
func WithModelFiles(v []string) Option         { return func(c *Config) { c.ModelFiles = v } }
func WithNGpuLayers(v int) Option              { return func(c *Config) { c.PtrNGpuLayers = new(v) } }
func WithNSeqMax(v int) Option                 { return func(c *Config) { c.PtrNSeqMax = new(v) } }
func WithNThreads(v int) Option                { return func(c *Config) { c.PtrNThreads = new(v) } }
func WithNThreadsBatch(v int) Option           { return func(c *Config) { c.PtrNThreadsBatch = new(v) } }
func WithPrefillBatchSize(v int) Option        { return func(c *Config) { c.PtrPrefillBatchSize = new(v) } }
func WithPhysicalBatchSize(v int) Option       { return func(c *Config) { c.PtrPhysicalBatchSize = new(v) } }
func WithNUMA(v NUMAStrategy) Option           { return func(c *Config) { c.NUMA = v } }
func WithOffloadKQV(v bool) Option             { return func(c *Config) { c.PtrOffloadKQV = new(v) } }
func WithOpOffload(v bool) Option              { return func(c *Config) { c.PtrOpOffload = new(v) } }
func WithProjFile(v string) Option             { return func(c *Config) { c.ProjFile = v } }
func WithMTPDrafterFile(v string) Option       { return func(c *Config) { c.MTPDrafterFile = v } }
func WithProjOnCPU(v bool) Option              { return func(c *Config) { c.PtrProjOnCPU = new(v) } }
func WithProjDevice(v string) Option           { return func(c *Config) { c.ProjDevice = v } }
func WithRopeFreqBase(v float32) Option        { return func(c *Config) { c.PtrRopeFreqBase = new(v) } }
func WithRopeFreqScale(v float32) Option       { return func(c *Config) { c.PtrRopeFreqScale = new(v) } }
func WithRopeScaling(v RopeScalingType) Option { return func(c *Config) { c.RopeScaling = v } }

func WithSpeculationMode(v SpeculationMode) Option {
	return func(c *Config) { c.Speculation = v }
}

func WithSplitMode(v SplitMode) Option          { return func(c *Config) { c.PtrSplitMode = new(v) } }
func WithSWAFull(v bool) Option                 { return func(c *Config) { c.PtrSWAFull = new(v) } }
func WithTensorBuftOverrides(v []string) Option { return func(c *Config) { c.TensorBuftOverrides = v } }
func WithTensorSplit(v []float32) Option        { return func(c *Config) { c.TensorSplit = v } }
func WithYarnAttnFactor(v float32) Option       { return func(c *Config) { c.PtrYarnAttnFactor = new(v) } }
func WithYarnBetaFast(v float32) Option         { return func(c *Config) { c.PtrYarnBetaFast = new(v) } }
func WithYarnBetaSlow(v float32) Option         { return func(c *Config) { c.PtrYarnBetaSlow = new(v) } }
func WithYarnExtFactor(v float32) Option        { return func(c *Config) { c.PtrYarnExtFactor = new(v) } }
func WithYarnOrigCtx(v int) Option              { return func(c *Config) { c.PtrYarnOrigCtx = new(v) } }

func WithQueueDepth(v int) Option {
	return func(c *Config) {
		c.PtrQueueDepth = new(v)
	}
}

func WithSessionStoreFactory(v SessionStoreFactory) Option {
	return func(c *Config) { c.SessionStoreFactory = v }
}

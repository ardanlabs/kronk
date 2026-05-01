package models

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

// =============================================================================
// Analysis types

// Analysis is the result of analyzing a local GGUF model file. It contains
// parsed model facts, system hardware information, memory estimates, and
// recommended runtime settings.
type Analysis struct {
	Model       ModelFacts              `json:"model"`
	System      SystemFacts             `json:"system"`
	Memory      MemoryEstimate          `json:"memory"`
	Recommended RuntimeRecommendation   `json:"recommended"`
	Profiles    []RuntimeRecommendation `json:"profiles,omitempty"`
	Warnings    []string                `json:"warnings,omitempty"`
}

// ModelFacts contains information extracted from the GGUF metadata.
type ModelFacts struct {
	ID              string           `json:"id"`
	Name            string           `json:"name,omitempty"`
	Architecture    string           `json:"architecture"`
	Class           string           `json:"class"`
	Quantization    string           `json:"quantization,omitempty"`
	FileType        int64            `json:"file_type,omitempty"`
	SizeBytes       int64            `json:"size_bytes"`
	TrainingContext int64            `json:"training_context,omitempty"`
	BlockCount      int64            `json:"block_count"`
	HeadCount       int64            `json:"head_count,omitempty"`
	HeadCountKV     int64            `json:"head_count_kv,omitempty"`
	KeyLength       int64            `json:"key_length,omitempty"`
	ValueLength     int64            `json:"value_length,omitempty"`
	EmbeddingLength int64            `json:"embedding_length,omitempty"`
	FeedForward     int64            `json:"feed_forward_length,omitempty"`
	VocabSize       int64            `json:"vocab_size,omitempty"`
	HasProjection   bool             `json:"has_projection"`
	MoE             *MoEInfo         `json:"moe,omitempty"`
	Weights         *WeightBreakdown `json:"weights,omitempty"`
	Rope            RopeFacts        `json:"rope"`
	Attention       AttentionFacts   `json:"attention"`
}

// RopeFacts contains RoPE (Rotary Position Embedding) configuration.
type RopeFacts struct {
	FreqBase    float64 `json:"freq_base,omitempty"`
	FreqScale   float64 `json:"freq_scale,omitempty"`
	ScalingType string  `json:"scaling_type,omitempty"`
	OriginalCtx int64   `json:"original_context,omitempty"`
	DimCount    int64   `json:"dimension_count,omitempty"`
}

// AttentionFacts contains attention-specific metadata.
type AttentionFacts struct {
	SlidingWindow       int64   `json:"sliding_window,omitempty"`
	SlidingWindowLayers int64   `json:"sliding_window_layers,omitempty"`
	FullAttentionLayers int64   `json:"full_attention_layers,omitempty"`
	LogitSoftcapping    float64 `json:"logit_softcapping,omitempty"`
}

// SystemFacts contains information about the host system hardware.
type SystemFacts struct {
	GPUName            string `json:"gpu_name,omitempty"`
	GPUType            string `json:"gpu_type,omitempty"`
	GPUFreeBytes       uint64 `json:"gpu_free_bytes"`
	GPUTotalBytes      uint64 `json:"gpu_total_bytes"`
	SystemRAMBytes     uint64 `json:"system_ram_bytes"`
	SupportsGPUOffload bool   `json:"supports_gpu_offload"`
}

// MemoryEstimate contains memory sizing information independent of any
// particular runtime configuration.
type MemoryEstimate struct {
	KVBytesPerTokenF16 int64 `json:"kv_bytes_per_token_f16"`
	KVBytesPerTokenQ8  int64 `json:"kv_bytes_per_token_q8_0"`
	FullGPUFit         bool  `json:"full_gpu_fit"`
}

// RuntimeRecommendation is a recommended set of runtime parameters.
type RuntimeRecommendation struct {
	Name               string `json:"name"`
	ContextWindow      int64  `json:"context_window"`
	NSeqMax            int64  `json:"nseq_max"`
	CacheTypeK         string `json:"cache_type_k"`
	CacheTypeV         string `json:"cache_type_v"`
	FlashAttention     string `json:"flash_attention"`
	NGPULayers         int64  `json:"ngpu_layers"`
	EstimatedVRAMBytes int64  `json:"estimated_vram_bytes"`
	Fits               bool   `json:"fits"`
	Reason             string `json:"reason,omitempty"`
}

// ToModelConfig converts a RuntimeRecommendation into a model.Config suitable
// for use with kronk.New. Fields not covered by the recommendation (like
// ModelFiles, ProjFile, Log) must be set by the caller.
func (r RuntimeRecommendation) ToModelConfig() model.Config {
	cfg := model.Config{
		PtrContextWindow: new(int(r.ContextWindow)),
		PtrNSeqMax:       new(int(r.NSeqMax)),
		PtrNBatch:        new(defNBatch),
		PtrNUBatch:       new(defNUBatch),
	}

	switch r.CacheTypeK {
	case "f16":
		cfg.CacheTypeK = model.GGMLTypeF16
	case "q8_0":
		cfg.CacheTypeK = model.GGMLTypeQ8_0
	}

	switch r.CacheTypeV {
	case "f16":
		cfg.CacheTypeV = model.GGMLTypeF16
	case "q8_0":
		cfg.CacheTypeV = model.GGMLTypeQ8_0
	}

	switch r.FlashAttention {
	case "auto":
		cfg.FlashAttention = model.FlashAttentionAuto
	case "disabled":
		cfg.FlashAttention = model.FlashAttentionDisabled
	default:
		cfg.FlashAttention = model.FlashAttentionEnabled
	}

	// model.Config: PtrNGpuLayers nil = all on GPU, 0 = all on GPU, -1 = all on CPU.
	// Only set when we explicitly want CPU-only.
	if r.NGPULayers < 0 {
		n := int(r.NGPULayers)
		cfg.PtrNGpuLayers = &n
	}

	return cfg
}

// Default batch sizes matching model package defaults.
const (
	defNBatch  = 2048
	defNUBatch = 512
)

// =============================================================================
// Public API

// ModelAnalysis reads a GGUF model file and produces an analysis with
// recommended runtime settings based on the model's architecture and
// the available system hardware.
func (m *Models) ModelAnalysis(modelID string) (Analysis, error) {
	info, err := m.ModelInformation(modelID)
	if err != nil {
		return Analysis{}, fmt.Errorf("model-analysis: %w", err)
	}

	devs := devices.List()

	return analyzeModel(info, devs)
}

// =============================================================================
// Core analysis (pure, testable)

// analyzeModel performs the analysis given parsed model info and hardware.
func analyzeModel(info ModelInfo, devs devices.Devices) (Analysis, error) {
	md := info.Metadata

	arch := gguf.DetectArchitecture(md)
	if arch == "" {
		return Analysis{}, fmt.Errorf("model-analysis: unable to detect architecture")
	}

	// -------------------------------------------------------------------------
	// Parse model facts.

	blockCount, err := gguf.ParseInt64WithFallback(md, arch+".block_count", ".block_count")
	if err != nil {
		return Analysis{}, fmt.Errorf("model-analysis: block_count: %w", err)
	}

	headCount, _ := gguf.ParseInt64(md, arch+".attention.head_count")
	headCountKV, _ := gguf.ParseInt64OrArrayAvg(md, arch+".attention.head_count_kv")
	keyLength, valueLength, _ := gguf.ResolveKVLengths(md, arch)
	embeddingLength, _ := gguf.ParseInt64WithFallback(md, arch+".embedding_length", ".embedding_length")
	feedForward, _ := gguf.ParseInt64WithFallback(md, arch+".feed_forward_length", ".feed_forward_length")
	trainingCtx, _ := gguf.ParseInt64WithFallback(md, arch+".context_length", ".context_length")
	vocabSize, _ := gguf.ParseInt64WithFallback(md, arch+".vocab_size", "tokenizer.ggml.tokens")

	fileType, _ := gguf.ParseInt64(md, "general.file_type")
	quantName := gguf.FileTypeName(fileType)

	moeInfo := detectMoE(md)
	class := classifyModel(info, moeInfo, arch)

	rope := ropeFactsFromGGUF(gguf.ParseRopeFacts(md, arch))
	attn := attentionFactsFromGGUF(gguf.ParseAttentionFacts(md, arch, blockCount))

	mf := ModelFacts{
		ID:              info.ID,
		Name:            info.Desc,
		Architecture:    arch,
		Class:           class,
		Quantization:    quantName,
		FileType:        fileType,
		SizeBytes:       int64(info.Size),
		TrainingContext: trainingCtx,
		BlockCount:      blockCount,
		HeadCount:       headCount,
		HeadCountKV:     headCountKV,
		KeyLength:       keyLength,
		ValueLength:     valueLength,
		EmbeddingLength: embeddingLength,
		FeedForward:     feedForward,
		VocabSize:       vocabSize,
		HasProjection:   info.HasProjection,
		Rope:            rope,
		Attention:       attn,
	}

	if moeInfo.IsMoE {
		mf.MoE = &moeInfo
	}

	// -------------------------------------------------------------------------
	// System facts.

	sf := buildSystemFacts(devs)

	// -------------------------------------------------------------------------
	// Memory estimates.

	kvBytesF16 := headCountKV * (keyLength + valueLength) * vram.BytesPerElementF16
	kvBytesQ8 := headCountKV * (keyLength + valueLength) * vram.BytesPerElementQ8_0

	// Use 85% of free GPU as the budget.
	gpuBudget := int64(float64(sf.GPUFreeBytes) * 0.85)

	modelSize := int64(info.Size)
	computeBuf := vram.EstimateComputeBuffer(vram.Input{
		ModelSizeBytes:  modelSize,
		EmbeddingLength: embeddingLength,
	})

	fullGPUFit := sf.SupportsGPUOffload && (modelSize+computeBuf) < gpuBudget

	mem := MemoryEstimate{
		KVBytesPerTokenF16: kvBytesF16,
		KVBytesPerTokenQ8:  kvBytesQ8,
		FullGPUFit:         fullGPUFit,
	}

	// -------------------------------------------------------------------------
	// Build profiles.

	profileInput := profileInput{
		modelSize:   modelSize,
		blockCount:  blockCount,
		headCountKV: headCountKV,
		keyLength:   keyLength,
		valueLength: valueLength,
		embLen:      embeddingLength,
		trainingCtx: trainingCtx,
		class:       class,
		gpuBudget:   gpuBudget,
		hasGPU:      sf.SupportsGPUOffload,
		attn:        attn,
	}

	balanced := buildProfile("balanced", profileInput, 0, 0)
	maxCtx := buildProfile("max_context", profileInput, 1, 0)
	maxConc := buildProfile("max_concurrency", profileInput, 0, 1)

	profiles := []RuntimeRecommendation{balanced, maxCtx, maxConc}

	// -------------------------------------------------------------------------
	// Warnings.

	var warnings []string

	if !sf.SupportsGPUOffload {
		warnings = append(warnings, "No GPU offload support detected; inference will use CPU only")
	}

	if !fullGPUFit && sf.SupportsGPUOffload {
		warnings = append(warnings, fmt.Sprintf("Model weights (%.1f GiB) may not fully fit in GPU memory (%.1f GiB free); partial offload may be needed",
			float64(modelSize)/(1024*1024*1024), float64(sf.GPUFreeBytes)/(1024*1024*1024)))
	}

	if trainingCtx > 0 && balanced.ContextWindow < trainingCtx {
		warnings = append(warnings, fmt.Sprintf("Context window capped to %d (training context: %d); use max_context profile or YaRN for full range",
			balanced.ContextWindow, trainingCtx))
	}

	if attn.SlidingWindow > 0 {
		warnings = append(warnings, fmt.Sprintf("Model uses sliding window attention (window=%d); SWA layers use less KV cache than estimated",
			attn.SlidingWindow))
	}

	return Analysis{
		Model:       mf,
		System:      sf,
		Memory:      mem,
		Recommended: balanced,
		Profiles:    profiles,
		Warnings:    warnings,
	}, nil
}

// =============================================================================
// Profile building

type profileInput struct {
	modelSize   int64
	blockCount  int64
	headCountKV int64
	keyLength   int64
	valueLength int64
	embLen      int64
	trainingCtx int64
	class       string
	gpuBudget   int64
	hasGPU      bool
	attn        AttentionFacts
}

// buildProfile creates a RuntimeRecommendation for a given profile strategy.
//
// overrideSlots > 0 forces a specific slot count (used by max_context).
// overrideConcurrency > 0 signals max-concurrency mode which maximizes slots.
func buildProfile(name string, p profileInput, overrideSlots int64, overrideConcurrency int64) RuntimeRecommendation {
	rec := RuntimeRecommendation{
		Name: name,
	}

	// Determine flash attention.
	if p.hasGPU {
		rec.FlashAttention = "auto"
	} else {
		rec.FlashAttention = "disabled"
	}

	// Determine target slots.
	switch {
	case overrideSlots > 0:
		rec.NSeqMax = overrideSlots
	case overrideConcurrency > 0:
		rec.NSeqMax = 8
	case p.class == "embedding" || p.class == "rerank" || p.class == "vision":
		rec.NSeqMax = 1
	default:
		rec.NSeqMax = 1
	}

	// Determine context window.
	ctxCap := p.trainingCtx
	if ctxCap <= 0 {
		ctxCap = vram.ContextWindow8K
	}

	switch name {
	case "balanced":
		ctxCap = minInt64(ctxCap, vram.ContextWindow32K)
	case "max_context":
		// Use full training context.
	case "max_concurrency":
		ctxCap = minInt64(ctxCap, vram.ContextWindow8K)
	}

	// Select the largest context bucket that fits within the GPU budget.
	buckets := []int64{
		vram.ContextWindow4K, vram.ContextWindow8K, vram.ContextWindow16K,
		vram.ContextWindow32K, vram.ContextWindow64K, vram.ContextWindow128K, vram.ContextWindow256K,
	}

	rec.CacheTypeK = "f16"
	rec.CacheTypeV = "f16"
	bytesPerElem := vram.BytesPerElementF16

	computeBuf := vram.EstimateComputeBuffer(vram.Input{
		ModelSizeBytes:  p.modelSize,
		EmbeddingLength: p.embLen,
	})

	bestCtx := vram.ContextWindow4K
	for _, bucket := range buckets {
		if bucket > ctxCap {
			break
		}

		kvPerSlot := bucket * p.blockCount * p.headCountKV * (p.keyLength + p.valueLength) * bytesPerElem
		totalVRAM := p.modelSize + (rec.NSeqMax * kvPerSlot) + computeBuf

		if p.gpuBudget > 0 && totalVRAM <= p.gpuBudget {
			bestCtx = bucket
		} else if p.gpuBudget <= 0 {
			// No GPU budget info — just use the capped context.
			bestCtx = bucket
		}
	}

	// If f16 doesn't fit at all with the minimum bucket, try q8_0.
	minKVF16 := vram.ContextWindow4K * p.blockCount * p.headCountKV * (p.keyLength + p.valueLength) * vram.BytesPerElementF16
	minTotalF16 := p.modelSize + (rec.NSeqMax * minKVF16) + computeBuf

	if p.gpuBudget > 0 && minTotalF16 > p.gpuBudget {
		rec.CacheTypeK = "q8_0"
		rec.CacheTypeV = "q8_0"
		bytesPerElem = vram.BytesPerElementQ8_0

		bestCtx = vram.ContextWindow4K
		for _, bucket := range buckets {
			if bucket > ctxCap {
				break
			}

			kvPerSlot := bucket * p.blockCount * p.headCountKV * (p.keyLength + p.valueLength) * bytesPerElem
			totalVRAM := p.modelSize + (rec.NSeqMax * kvPerSlot) + computeBuf

			if totalVRAM <= p.gpuBudget {
				bestCtx = bucket
			}
		}
	}

	rec.ContextWindow = bestCtx

	// For max_concurrency, see how many slots we can actually fit.
	if overrideConcurrency > 0 && p.gpuBudget > 0 {
		kvPerSlot := bestCtx * p.blockCount * p.headCountKV * (p.keyLength + p.valueLength) * bytesPerElem
		if kvPerSlot > 0 {
			available := p.gpuBudget - p.modelSize - computeBuf
			if available > 0 {
				maxSlots := min(max(available/kvPerSlot, 1), 8)
				rec.NSeqMax = maxSlots
			} else {
				rec.NSeqMax = 1
			}
		}
	}

	// GPU layers: model.Config uses 0 = all on GPU, -1 = all on CPU.
	if p.hasGPU {
		rec.NGPULayers = 0
	} else {
		rec.NGPULayers = -1
	}

	// Estimate VRAM for the chosen configuration.
	kvPerSlot := bestCtx * p.blockCount * p.headCountKV * (p.keyLength + p.valueLength) * bytesPerElem
	rec.EstimatedVRAMBytes = p.modelSize + (rec.NSeqMax * kvPerSlot) + computeBuf
	rec.Fits = p.gpuBudget <= 0 || rec.EstimatedVRAMBytes <= p.gpuBudget

	// Build a human-readable reason.
	rec.Reason = buildReason(name, rec, p)

	return rec
}

func buildReason(name string, rec RuntimeRecommendation, p profileInput) string {
	var parts []string

	switch name {
	case "balanced":
		parts = append(parts, "Good default for chat and API serving")
	case "max_context":
		parts = append(parts, "Maximizes context window with single slot")
	case "max_concurrency":
		parts = append(parts, "Maximizes concurrent requests with smaller context")
	}

	if !rec.Fits {
		parts = append(parts, "WARNING: exceeds estimated GPU budget")
	}

	return strings.Join(parts, "; ")
}

// =============================================================================
// Helpers

func classifyModel(info ModelInfo, moe MoEInfo, arch string) string {
	if gguf.IsVisionEncoder(arch) || info.HasProjection {
		return "vision"
	}

	if info.IsEmbedModel {
		return "embedding"
	}

	if info.IsRerankModel {
		return "rerank"
	}

	if moe.IsMoE {
		return "moe"
	}

	return "dense"
}

// ropeFactsFromGGUF converts a gguf.RopeFacts into the models-side
// RopeFacts so the public API does not leak the gguf type.
func ropeFactsFromGGUF(r gguf.RopeFacts) RopeFacts {
	return RopeFacts{
		FreqBase:    r.FreqBase,
		FreqScale:   r.FreqScale,
		ScalingType: r.ScalingType,
		OriginalCtx: r.OriginalCtx,
		DimCount:    r.DimCount,
	}
}

// attentionFactsFromGGUF converts a gguf.AttentionFacts into the
// models-side AttentionFacts so the public API does not leak the
// gguf type.
func attentionFactsFromGGUF(a gguf.AttentionFacts) AttentionFacts {
	return AttentionFacts{
		SlidingWindow:       a.SlidingWindow,
		SlidingWindowLayers: a.SlidingWindowLayers,
		FullAttentionLayers: a.FullAttentionLayers,
		LogitSoftcapping:    a.LogitSoftcapping,
	}
}

func buildSystemFacts(devs devices.Devices) SystemFacts {
	sf := SystemFacts{
		SystemRAMBytes:     devs.SystemRAMBytes,
		SupportsGPUOffload: devs.SupportsGPUOffload,
	}

	// Find the primary GPU (largest free memory).
	for _, d := range devs.Devices {
		if !strings.HasPrefix(d.Type, "gpu_") {
			continue
		}

		if d.FreeBytes > sf.GPUFreeBytes {
			sf.GPUName = d.Name
			sf.GPUType = d.Type
			sf.GPUFreeBytes = d.FreeBytes
			sf.GPUTotalBytes = d.TotalBytes
		}
	}

	return sf
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}

	return b
}

package models

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

// ggufFileTypeNames maps the GGUF general.file_type integer to a
// human-readable quantization name. These values come from the
// llama.cpp LLAMA_FTYPE_* enum.
var ggufFileTypeNames = map[int64]string{
	0:  "F32",
	1:  "F16",
	2:  "Q4_0",
	3:  "Q4_1",
	7:  "Q8_0",
	8:  "Q8_1",
	10: "Q2_K",
	11: "Q3_K_S",
	12: "Q3_K_M",
	13: "Q3_K_L",
	14: "Q4_K_S",
	15: "Q4_K_M",
	16: "Q5_K_S",
	17: "Q5_K_M",
	18: "Q6_K",
	19: "IQ2_XXS",
	20: "IQ2_XS",
	21: "IQ3_XXS",
	22: "IQ1_S",
	23: "IQ4_NL",
	24: "IQ3_S",
	25: "IQ2_S",
	26: "IQ4_XS",
	27: "IQ1_M",
	28: "BF16",
	29: "Q4_0_4_4",
	30: "Q4_0_4_8",
	31: "Q4_0_8_8",
	32: "TQ1_0",
	33: "TQ2_0",
}

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
		ContextWindow: int(r.ContextWindow),
		NSeqMax:       int(r.NSeqMax),
		NBatch:        defNBatch,
		NUBatch:       defNUBatch,
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

	// model.Config: NGpuLayers nil = all on GPU, 0 = all on GPU, -1 = all on CPU.
	// Only set when we explicitly want CPU-only.
	if r.NGPULayers < 0 {
		n := int(r.NGPULayers)
		cfg.NGpuLayers = &n
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

	arch := detectArchitecture(md)
	if arch == "" {
		return Analysis{}, fmt.Errorf("model-analysis: unable to detect architecture")
	}

	// -------------------------------------------------------------------------
	// Parse model facts.

	blockCount, err := parseMetadataInt64WithFallback(md, arch+".block_count", ".block_count")
	if err != nil {
		return Analysis{}, fmt.Errorf("model-analysis: block_count: %w", err)
	}

	headCount, _ := parseMetadataInt64(md, arch+".attention.head_count")
	headCountKV, _ := parseMetadataInt64OrArrayAvg(md, arch+".attention.head_count_kv")
	keyLength, valueLength, _ := resolveKVLengths(md, arch)
	embeddingLength, _ := parseMetadataInt64WithFallback(md, arch+".embedding_length", ".embedding_length")
	feedForward, _ := parseMetadataInt64WithFallback(md, arch+".feed_forward_length", ".feed_forward_length")
	trainingCtx, _ := parseMetadataInt64WithFallback(md, arch+".context_length", ".context_length")
	vocabSize, _ := parseMetadataInt64WithFallback(md, arch+".vocab_size", "tokenizer.ggml.tokens")

	fileType, _ := parseMetadataInt64(md, "general.file_type")
	quantName := ggufFileTypeName(fileType)

	moeInfo := detectMoE(md)
	class := classifyModel(info, moeInfo, arch)

	rope := parseRopeFacts(md, arch)
	attn := parseAttentionFacts(md, arch, blockCount)

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

	kvBytesF16 := headCountKV * (keyLength + valueLength) * BytesPerElementF16
	kvBytesQ8 := headCountKV * (keyLength + valueLength) * BytesPerElementQ8_0

	// Use 85% of free GPU as the budget.
	gpuBudget := int64(float64(sf.GPUFreeBytes) * 0.85)

	modelSize := int64(info.Size)
	computeBuf := estimateComputeBuffer(VRAMInput{
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
		rec.NSeqMax = 2
	}

	// Determine context window.
	ctxCap := p.trainingCtx
	if ctxCap <= 0 {
		ctxCap = ContextWindow8K
	}

	switch name {
	case "balanced":
		ctxCap = minInt64(ctxCap, ContextWindow32K)
	case "max_context":
		// Use full training context.
	case "max_concurrency":
		ctxCap = minInt64(ctxCap, ContextWindow8K)
	}

	// Select the largest context bucket that fits within the GPU budget.
	buckets := []int64{
		ContextWindow4K, ContextWindow8K, ContextWindow16K,
		ContextWindow32K, ContextWindow64K, ContextWindow128K, ContextWindow256K,
	}

	rec.CacheTypeK = "f16"
	rec.CacheTypeV = "f16"
	bytesPerElem := BytesPerElementF16

	computeBuf := estimateComputeBuffer(VRAMInput{
		ModelSizeBytes:  p.modelSize,
		EmbeddingLength: p.embLen,
	})

	bestCtx := ContextWindow4K
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
	minKVF16 := ContextWindow4K * p.blockCount * p.headCountKV * (p.keyLength + p.valueLength) * BytesPerElementF16
	minTotalF16 := p.modelSize + (rec.NSeqMax * minKVF16) + computeBuf

	if p.gpuBudget > 0 && minTotalF16 > p.gpuBudget {
		rec.CacheTypeK = "q8_0"
		rec.CacheTypeV = "q8_0"
		bytesPerElem = BytesPerElementQ8_0

		bestCtx = ContextWindow4K
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
	if isVisionEncoder(arch) || info.HasProjection {
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

func parseRopeFacts(md map[string]string, arch string) RopeFacts {
	var r RopeFacts

	if v, err := parseMetadataFloat64(md, arch+".rope.freq_base"); err == nil {
		r.FreqBase = v
	}

	if v, err := parseMetadataFloat64(md, arch+".rope.freq_scale"); err == nil {
		r.FreqScale = v
	}

	if v, ok := md[arch+".rope.scaling.type"]; ok {
		r.ScalingType = v
	} else if v, ok := md["rope_scaling"]; ok {
		r.ScalingType = v
	}

	if v, err := parseMetadataInt64(md, arch+".rope.scaling.original_context_length"); err == nil {
		r.OriginalCtx = v
	}

	if v, err := parseMetadataInt64(md, arch+".rope.dimension_count"); err == nil {
		r.DimCount = v
	}

	return r
}

func parseAttentionFacts(md map[string]string, arch string, blockCount int64) AttentionFacts {
	var a AttentionFacts

	if v, err := parseMetadataInt64(md, arch+".attention.sliding_window"); err == nil {
		a.SlidingWindow = v
	}

	// Count SWA layers from the sliding_window_pattern array if present.
	if pattern, ok := md[arch+".attention.sliding_window_pattern"]; ok {
		swaCount := countSWALayers(pattern)
		a.SlidingWindowLayers = swaCount
		a.FullAttentionLayers = blockCount - swaCount
	} else if a.SlidingWindow > 0 {
		// If there's a sliding window but no pattern, assume all layers are SWA.
		a.SlidingWindowLayers = blockCount
		a.FullAttentionLayers = 0
	} else {
		a.FullAttentionLayers = blockCount
	}

	if v, err := parseMetadataFloat64(md, arch+".final_logit_softcapping"); err == nil {
		a.LogitSoftcapping = v
	}

	return a
}

// countSWALayers counts true values in a stringified bool array like
// "[true true true true true false true ...]".
func countSWALayers(pattern string) int64 {
	trimmed := strings.TrimSpace(pattern)
	trimmed = strings.Trim(trimmed, "[]")
	fields := strings.Fields(trimmed)

	var count int64
	for _, f := range fields {
		if f == "true" {
			count++
		}
	}

	return count
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

func ggufFileTypeName(ft int64) string {
	if name, ok := ggufFileTypeNames[ft]; ok {
		return name
	}

	if ft == 0 {
		return ""
	}

	return fmt.Sprintf("unknown(%d)", ft)
}

func parseMetadataFloat64(md map[string]string, key string) (float64, error) {
	val, ok := md[key]
	if !ok {
		return 0, fmt.Errorf("parse-metadata-float64: key %q not found", key)
	}

	return strconv.ParseFloat(val, 64)
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}

	return b
}

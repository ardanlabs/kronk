package models

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
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
	RecurrentLayers     int64   `json:"recurrent_layers,omitempty"`
	NextNPredictLayers  int64   `json:"nextn_predict_layers,omitempty"`
	SharedKVLayers      int64   `json:"shared_kv_layers,omitempty"`
	SWAPattern          []bool  `json:"-"`
	RecurrentPattern    []bool  `json:"-"`
	HeadCountKVByLayer  []int64 `json:"-"`
	FullHeadCountKV     int64   `json:"full_head_count_kv,omitempty"`
	SWAHeadCountKV      int64   `json:"swa_head_count_kv,omitempty"`
	FullKeyLength       int64   `json:"full_key_length,omitempty"`
	FullValueLength     int64   `json:"full_value_length,omitempty"`
	SWAKeyLength        int64   `json:"swa_key_length,omitempty"`
	SWAValueLength      int64   `json:"swa_value_length,omitempty"`
	RecurrentStateBytes int64   `json:"recurrent_state_bytes,omitempty"`
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

// AutoTuneBudgetPercent is the percentage of available memory AutoTune uses
// when selecting a runtime profile.
const AutoTuneBudgetPercent = 85

// AutoTuneBudget provides stable memory limits for an AutoTune analysis.
// Pool callers use the resource manager's empty-pool budgets so resident
// models do not change the recommendation for an incoming model.
type AutoTuneBudget struct {
	// Devices is the immutable hardware snapshot used for the analysis.
	Devices devices.Devices
	// GPUBytes is the effective empty-pool capacity for the configured GPU
	// placement. Multi-GPU split modes account for every participating device.
	GPUBytes int64
	// SystemRAMBytes is the empty-pool system RAM budget.
	SystemRAMBytes int64
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
	SplitMode          string `json:"split_mode"`
	NGPULayers         int64  `json:"ngpu_layers"`
	EstimatedVRAMBytes int64  `json:"estimated_vram_bytes"`
	Fits               bool   `json:"fits"`
	Reason             string `json:"reason,omitempty"`
}

// =============================================================================
// Public API

// Analyze produces a hardware-aware analysis with recommended runtime settings
// from already-gathered model facts and device information. It is the pure entry
// point (no disk or hardware I/O) shared by the catalog-based ModelAnalysis and
// path-based callers such as the SDK auto-tune flow in kronk.New.
func Analyze(info ModelInfo, devs devices.Devices) (Analysis, error) {
	return analyzeModelWithConfig(info, devs, ModelConfig{})
}

// ModelAnalysis reads a GGUF model file and produces an analysis with
// recommended runtime settings based on the model's architecture and
// the available system hardware.
func (m *Models) ModelAnalysis(modelID string) (Analysis, error) {
	return m.ModelAnalysisWithConfig(modelID, ModelConfig{})
}

// ModelAnalysisWithConfig reads a GGUF model file and produces an analysis
// while treating explicitly configured sizing values as fixed constraints.
func (m *Models) ModelAnalysisWithConfig(modelID string, constraints ModelConfig) (Analysis, error) {
	info, err := m.ModelInformation(modelID)
	if err != nil {
		return Analysis{}, fmt.Errorf("model-analysis: %w", err)
	}

	devs := devices.List()

	return analyzeModelWithConfig(info, devs, constraints)
}

// =============================================================================
// Core analysis (pure, testable)

// analyzeModel performs the analysis given parsed model info and hardware.
func analyzeModel(info ModelInfo, devs devices.Devices) (Analysis, error) {
	return analyzeModelWithConfig(info, devs, ModelConfig{})
}

// analyzeModelWithConfig performs the analysis while treating explicitly set
// AutoTune-owned values as fixed constraints.
func analyzeModelWithConfig(info ModelInfo, devs devices.Devices, cfg ModelConfig) (Analysis, error) {
	return analyzeModelWithConfigAndBudget(info, devs, cfg, nil)
}

func analyzeModelWithConfigAndBudget(info ModelInfo, devs devices.Devices, cfg ModelConfig, budget *AutoTuneBudget) (Analysis, error) {
	md := info.Metadata
	devs = analysisDevices(devs, cfg.Devices)

	profile := modelprofile.Resolve(md)
	arch := profile.Architecture
	if err := profile.ValidateForAnalysis(); err != nil {
		return Analysis{}, fmt.Errorf("model-analysis: %w", err)
	}

	// -------------------------------------------------------------------------
	// Parse model facts.

	dimensions := profile.Dimensions
	blockCount := dimensions.BlockCount
	headCount := dimensions.HeadCount
	headCountKV := dimensions.HeadCountKV
	keyLength := dimensions.KeyLength
	valueLength := dimensions.ValueLength
	embeddingLength := dimensions.EmbeddingLength
	feedForward := dimensions.FeedForwardLength
	trainingCtx := dimensions.ContextLength
	vocabSize := dimensions.VocabularySize
	fileType := profile.FileType
	quantName := gguf.FileTypeName(fileType)

	moeInfo := moeInfoFromGGUF(profile.MoE)
	class := classifyModel(info, moeInfo, profile)

	rope := ropeFactsFromGGUF(profile.Rope)
	attn := attentionFactsFromGGUF(profile.Attention)

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
	gpuBudget := int64(float64(sf.GPUFreeBytes) * AutoTuneBudgetPercent / 100)
	ramBudget := int64(float64(sf.SystemRAMBytes) * AutoTuneBudgetPercent / 100)
	if budget != nil {
		gpuBudget = budget.GPUBytes
		ramBudget = budget.SystemRAMBytes
		if devs.UnifiedMemory {
			gpuBudget = budget.SystemRAMBytes
		}
	}

	modelSize := int64(info.Size)
	computeBuf := vram.EstimateComputeBuffer(vram.Input{
		ModelSizeBytes:  modelSize,
		NUBatch:         effectivePrefillBatchSize(cfg.PtrPrefillBatchSize),
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
	cfg = normalizeAnalysisConfig(cfg, trainingCtx)

	profileInput := profileInput{
		modelSize:      modelSize,
		blockCount:     blockCount,
		headCountKV:    headCountKV,
		keyLength:      keyLength,
		valueLength:    valueLength,
		embLen:         embeddingLength,
		trainingCtx:    trainingCtx,
		class:          class,
		gpuBudget:      gpuBudget,
		ramBudget:      ramBudget,
		hasGPU:         sf.SupportsGPUOffload,
		unifiedMemory:  devs.UnifiedMemory,
		gpuCount:       devs.GPUCount,
		attn:           attn,
		contextWindow:  cfg.PtrContextWindow,
		nSeqMax:        cfg.PtrNSeqMax,
		cacheTypeK:     cfg.CacheTypeK,
		cacheTypeV:     cfg.CacheTypeV,
		flashAttention: cfg.FlashAttention,
		splitMode:      cfg.PtrSplitMode,
		nGpuLayers:     cfg.PtrNGpuLayers,
		nUBatch:        effectivePrefillBatchSize(cfg.PtrPrefillBatchSize),
		kvCacheOnCPU:   cfg.PtrOffloadKQV != nil && !*cfg.PtrOffloadKQV,
		swaFull:        cfg.PtrSWAFull == nil || *cfg.PtrSWAFull,
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

	return Analysis{
		Model:       mf,
		System:      sf,
		Memory:      mem,
		Recommended: balanced,
		Profiles:    profiles,
		Warnings:    warnings,
	}, nil
}

// normalizeAnalysisConfig resolves non-positive explicit sizing values using
// the same effective defaults as the runtime before memory is calculated.
func normalizeAnalysisConfig(cfg ModelConfig, trainingCtx int64) ModelConfig {
	if cfg.PtrContextWindow != nil && *cfg.PtrContextWindow <= 0 {
		contextWindow := int64(vram.ContextWindow8K)
		if trainingCtx > 0 {
			contextWindow = min(trainingCtx, contextWindow)
		}
		cfg.PtrContextWindow = new(int(contextWindow))
	}

	if cfg.PtrNSeqMax != nil && *cfg.PtrNSeqMax <= 0 {
		cfg.PtrNSeqMax = new(1)
	}

	return cfg
}

// =============================================================================
// Profile building

type profileInput struct {
	modelSize      int64
	blockCount     int64
	headCountKV    int64
	keyLength      int64
	valueLength    int64
	embLen         int64
	trainingCtx    int64
	class          string
	gpuBudget      int64
	ramBudget      int64
	hasGPU         bool
	unifiedMemory  bool
	gpuCount       int
	attn           AttentionFacts
	contextWindow  *int
	nSeqMax        *int
	cacheTypeK     model.GGMLType
	cacheTypeV     model.GGMLType
	flashAttention *model.FlashAttentionType
	splitMode      *model.SplitMode
	nGpuLayers     *int
	nUBatch        int64
	kvCacheOnCPU   bool
	swaFull        bool
}

type cacheRecommendation struct {
	typeK           string
	typeV           string
	ggmlTypeK       int32
	ggmlTypeV       int32
	bytesPerElement int64
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
	if p.flashAttention != nil {
		rec.FlashAttention = p.flashAttention.String()
	} else if p.hasGPU {
		rec.FlashAttention = "auto"
	} else {
		rec.FlashAttention = "disabled"
	}

	// Use the same layer-split default as the load path and llama.cpp. Explicit
	// user configuration still wins.
	if p.splitMode != nil {
		rec.SplitMode = p.splitMode.String()
	} else {
		rec.SplitMode = model.DefaultSplitMode(p.gpuCount).String()
	}

	// Determine target slots. An explicit value is a sizing constraint, not an
	// override to apply after the recommendation has been calculated.
	switch {
	case p.nSeqMax != nil:
		rec.NSeqMax = int64(*p.nSeqMax)
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
		ctxCap = minInt64(ctxCap, vram.ContextWindow128K)
	case "max_context":
		// Use full training context.
	case "max_concurrency":
		ctxCap = minInt64(ctxCap, vram.ContextWindow8K)
	}
	if p.contextWindow != nil {
		ctxCap = int64(*p.contextWindow)
	}

	// Select the largest context bucket that fits within the GPU budget. An
	// explicit context is the only candidate: cache precision may be reduced to
	// preserve it, but AutoTune must not silently reduce a requested context.
	// Prefer f16 at each context length, then try q8_0 before reducing the
	// context. This preserves the profile's context objective whenever KV
	// quantization is enough to make it fit.
	buckets := []int64{
		vram.ContextWindow4K, vram.ContextWindow8K, vram.ContextWindow16K,
		vram.ContextWindow32K, vram.ContextWindow64K, vram.ContextWindow128K, vram.ContextWindow256K,
	}
	if p.contextWindow != nil {
		buckets = []int64{int64(*p.contextWindow)}
	}

	cacheTypes := cacheRecommendations(p.cacheTypeK, p.cacheTypeV)
	fallback := cacheTypes[len(cacheTypes)-1]
	bestCtx := min(ctxCap, vram.ContextWindow4K)
	if p.contextWindow != nil {
		bestCtx = int64(*p.contextWindow)
	}
	selectedCache := fallback
	rec.CacheTypeK = fallback.typeK
	rec.CacheTypeV = fallback.typeV

	found := false
	for _, bucket := range slices.Backward(buckets) {
		if bucket > ctxCap {
			continue
		}

		for _, cacheType := range cacheTypes {
			result := calculateProfile(p, bucket, rec.NSeqMax, cacheType)
			if !profileFits(p, result) {
				continue
			}

			bestCtx = bucket
			selectedCache = cacheType
			rec.CacheTypeK = cacheType.typeK
			rec.CacheTypeV = cacheType.typeV
			found = true
			break
		}

		if found {
			break
		}
	}

	rec.ContextWindow = bestCtx

	// For max_concurrency, see how many slots we can actually fit unless the
	// user fixed NSeqMax. AutoTune never changes an explicit concurrency.
	if overrideConcurrency > 0 && p.nSeqMax == nil {
		rec.NSeqMax = 1
		for slots := int64(8); slots > 1; slots-- {
			if profileFits(p, calculateProfile(p, bestCtx, slots, selectedCache)) {
				rec.NSeqMax = slots
				break
			}
		}
	}

	// GPU layers: model.Config uses 0 = all on GPU, -1 = all on CPU.
	if p.nGpuLayers != nil {
		rec.NGPULayers = int64(*p.nGpuLayers)
	} else if p.hasGPU {
		rec.NGPULayers = 0
	} else {
		rec.NGPULayers = -1
	}

	// Estimate VRAM for the chosen configuration using the same hybrid-SWA,
	// KV-placement, and layer-placement calculator as admission planning.
	result := calculateProfile(p, bestCtx, rec.NSeqMax, selectedCache)
	rec.EstimatedVRAMBytes = result.TotalVRAM
	rec.Fits = profileFits(p, result)

	// Build a human-readable reason.
	rec.Reason = buildReason(name, rec, p)

	return rec
}

func profileFits(p profileInput, result vram.Result) bool {
	if p.unifiedMemory {
		return result.UnifiedFootprint() <= p.gpuBudget
	}

	usesGPU := p.hasGPU && (p.nGpuLayers == nil || *p.nGpuLayers != -1)
	if usesGPU && result.TotalVRAM > p.gpuBudget {
		return false
	}

	ram := result.TotalSystemRAMEst
	if !usesGPU {
		ram += result.TotalVRAM
	}
	return p.ramBudget <= 0 || ram <= p.ramBudget
}

func analysisDevices(devs devices.Devices, selected []string) devices.Devices {
	if len(selected) == 0 {
		return devs
	}

	wanted := make(map[string]struct{}, len(selected))
	for _, name := range selected {
		wanted[name] = struct{}{}
	}

	filtered := devices.Devices{
		SystemRAMBytes: devs.SystemRAMBytes,
		MaxDevices:     devs.MaxDevices,
		UnifiedMemory:  devs.UnifiedMemory,
	}
	for _, device := range devs.Devices {
		if _, ok := wanted[device.Name]; !ok {
			continue
		}
		filtered.Devices = append(filtered.Devices, device)
		if strings.HasPrefix(device.Type, "gpu_") {
			filtered.GPUCount++
			filtered.GPUTotalBytes += device.TotalBytes
			filtered.SupportsGPUOffload = true
		}
	}
	return filtered
}

func calculateProfile(p profileInput, contextWindow, slots int64, cache cacheRecommendation) vram.Result {
	headCountKV := p.attn.FullHeadCountKV
	if headCountKV <= 0 {
		headCountKV = p.headCountKV
	}
	keyLength := p.attn.FullKeyLength
	if keyLength <= 0 {
		keyLength = p.keyLength
	}
	valueLength := p.attn.FullValueLength
	if valueLength <= 0 {
		valueLength = p.valueLength
	}

	gpuLayers := int64(0)
	if p.nGpuLayers != nil {
		gpuLayers = int64(*p.nGpuLayers)
	} else if !p.hasGPU {
		gpuLayers = -1
	}

	return vram.Calculate(vram.Input{
		ModelSizeBytes:       p.modelSize,
		ContextWindow:        contextWindow,
		BlockCount:           p.blockCount,
		HeadCountKV:          headCountKV,
		KeyLength:            keyLength,
		ValueLength:          valueLength,
		SWAHeadCountKV:       p.attn.SWAHeadCountKV,
		SWAKeyLength:         p.attn.SWAKeyLength,
		SWAValueLength:       p.attn.SWAValueLength,
		HeadCountKVByLayer:   p.attn.HeadCountKVByLayer,
		BytesPerElement:      cache.bytesPerElement,
		TypeK:                cache.ggmlTypeK,
		TypeV:                cache.ggmlTypeV,
		Slots:                slots,
		SlidingWindow:        p.attn.SlidingWindow,
		SlidingWindowLayers:  p.attn.SlidingWindowLayers,
		SharedKVLayers:       p.attn.SharedKVLayers,
		SWAPattern:           p.attn.SWAPattern,
		RecurrentPattern:     p.attn.RecurrentPattern,
		NextNPredictLayers:   p.attn.NextNPredictLayers,
		RecurrentStateBytes:  p.attn.RecurrentStateBytes,
		RecurrentStateCopies: 1,
		NUBatch:              p.nUBatch,
		KVUnified:            false,
		EmbeddingLength:      p.embLen,
		GPULayers:            gpuLayers,
		KVCacheOnCPU:         p.kvCacheOnCPU,
		SWAFull:              p.swaFull,
		VTransposed:          profileVTransposed(p),
	})
}

func profileVTransposed(p profileInput) bool {
	if p.flashAttention != nil {
		return *p.flashAttention == model.FlashAttentionDisabled
	}
	return !p.hasGPU
}

func effectivePrefillBatchSize(prefillBatchSize *int) int64 {
	if prefillBatchSize != nil && *prefillBatchSize > 0 {
		return int64(*prefillBatchSize)
	}
	return int64(model.DefaultPrefillBatchSize)
}

// cacheRecommendations returns cache candidates in quality order while
// preserving either cache type explicitly selected by the user.
func cacheRecommendations(typeK, typeV model.GGMLType) []cacheRecommendation {
	build := func(defaultType model.GGMLType) cacheRecommendation {
		k := typeK
		if k == model.GGMLTypeAuto {
			k = defaultType
		}

		v := typeV
		if v == model.GGMLTypeAuto {
			v = defaultType
		}

		return cacheRecommendation{
			typeK:           k.String(),
			typeV:           v.String(),
			ggmlTypeK:       int32(k),
			ggmlTypeV:       int32(v),
			bytesPerElement: gguf.MaxBytesPerElement(int32(k), int32(v)),
		}
	}

	f16 := build(model.GGMLTypeF16)
	q8 := build(model.GGMLTypeQ8_0)
	if f16.typeK == q8.typeK && f16.typeV == q8.typeV {
		return []cacheRecommendation{f16}
	}

	return []cacheRecommendation{f16, q8}
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

func classifyModel(info ModelInfo, moe MoEInfo, profile modelprofile.Profile) string {
	if profile.Role == modelprofile.RoleVisionEncoder || info.HasProjection {
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
		RecurrentLayers:     a.RecurrentLayers,
		NextNPredictLayers:  a.NextNPredictLayers,
		SharedKVLayers:      a.SharedKVLayers,
		SWAPattern:          a.SWAPattern,
		RecurrentPattern:    a.RecurrentPattern,
		HeadCountKVByLayer:  a.HeadCountKV,
		FullHeadCountKV:     a.FullHeadCountKV,
		SWAHeadCountKV:      a.SWAHeadCountKV,
		FullKeyLength:       a.FullKeyLength,
		FullValueLength:     a.FullValueLength,
		SWAKeyLength:        a.SWAKeyLength,
		SWAValueLength:      a.SWAValueLength,
		RecurrentStateBytes: a.RecurrentStateBytes,
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

		if sf.GPUName == "" || d.FreeBytes > sf.GPUFreeBytes {
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

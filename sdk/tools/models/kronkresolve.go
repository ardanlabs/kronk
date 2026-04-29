package models

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Default batch sizes used when seeding from analysis.
const (
	defAnalysisNBatch  = 2048
	defAnalysisNUBatch = 512
)

// KronkResolvedConfig builds a model.Config for kronk.New() using the new
// resolution flow: analysis defaults (layer 1) overridden by user-supplied
// model_config.yaml entries (layer 3), then sampling defaults via
// SamplingConfig.WithDefaults(), grammar resolution, and on-disk file paths.
//
// The catalog YAML middle layer used by the legacy resolution path is not
// applied here.
func (m *Models) KronkResolvedConfig(modelID string, mc map[string]ModelConfig) (model.Config, error) {

	// Confirm the model is on disk before resolving anything else.
	fp, err := m.FullPath(modelID)
	if err != nil {
		return model.Config{}, fmt.Errorf("kronk-resolved-config: unable to get model[%s] path: %w", modelID, err)
	}

	// Layer 1: hardware-aware defaults derived from the GGUF file metadata.
	cfg := m.AnalysisDefaults(modelID)

	// Layer 3: user overrides from model_config.yaml.
	if override, ok := mc[modelID]; ok {
		MergeModelConfig(&cfg, override)
	}

	// Resolve grammar (.grm filename -> contents) before converting.
	if err := m.ResolveGrammar(&cfg.Sampling); err != nil {
		return model.Config{}, fmt.Errorf("kronk-resolved-config: %w", err)
	}

	// Convert to model.Config and attach on-disk paths.
	out := cfg.ToKronkConfig()
	out.ModelFiles = fp.ModelFiles
	out.ProjFile = fp.ProjFile

	// Resolve draft model file paths if configured.
	if cfg.DraftModel != nil && cfg.DraftModel.ModelID != "" {
		draftPath, err := m.FullPath(cfg.DraftModel.ModelID)
		if err != nil {
			return model.Config{}, fmt.Errorf("kronk-resolved-config: unable to get draft model[%s] path: %w", cfg.DraftModel.ModelID, err)
		}

		if out.DraftModel == nil {
			out.DraftModel = &model.DraftModelConfig{}
		}

		out.DraftModel.ModelFiles = draftPath.ModelFiles
	}

	return out, nil
}

// AnalysisDefaults runs ModelAnalysis on the specified model and converts
// the balanced recommendation into a ModelConfig. If the model is not
// downloaded or analysis fails, an empty ModelConfig is returned.
func (m *Models) AnalysisDefaults(modelID string) ModelConfig {
	analysis, err := m.ModelAnalysis(modelID)
	if err != nil {
		return ModelConfig{}
	}

	rec := analysis.Recommended

	var cfg ModelConfig

	cfg.PtrContextWindow = new(int(rec.ContextWindow))
	cfg.PtrNBatch = new(defAnalysisNBatch)
	cfg.PtrNUBatch = new(defAnalysisNUBatch)
	cfg.PtrNSeqMax = new(int(rec.NSeqMax))

	if k, err := model.ParseGGMLType(rec.CacheTypeK); err == nil {
		cfg.CacheTypeK = k
	}

	if v, err := model.ParseGGMLType(rec.CacheTypeV); err == nil {
		cfg.CacheTypeV = v
	}

	switch rec.FlashAttention {
	case "auto":
		cfg.FlashAttention = new(model.FlashAttentionAuto)
	case "disabled":
		cfg.FlashAttention = new(model.FlashAttentionDisabled)
	default:
		cfg.FlashAttention = new(model.FlashAttentionEnabled)
	}

	// model.Config: PtrNGpuLayers nil = all on GPU, 0 = all on GPU, -1 = all on CPU.
	// Only set when we explicitly want CPU-only.
	if rec.NGPULayers < 0 {
		n := int(rec.NGPULayers)
		cfg.PtrNGpuLayers = &n
	}

	return cfg
}

// MergeModelConfig overlays non-zero fields from src onto dst.
func MergeModelConfig(dst *ModelConfig, src ModelConfig) {
	if src.Template != "" {
		dst.Template = src.Template
	}
	if src.PtrContextWindow != nil {
		dst.PtrContextWindow = src.PtrContextWindow
	}
	if src.PtrNBatch != nil {
		dst.PtrNBatch = src.PtrNBatch
	}
	if src.PtrNUBatch != nil {
		dst.PtrNUBatch = src.PtrNUBatch
	}
	if src.PtrNThreads != nil {
		dst.PtrNThreads = src.PtrNThreads
	}
	if src.PtrNThreadsBatch != nil {
		dst.PtrNThreadsBatch = src.PtrNThreadsBatch
	}
	if src.CacheTypeK != 0 {
		dst.CacheTypeK = src.CacheTypeK
	}
	if src.CacheTypeV != 0 {
		dst.CacheTypeV = src.CacheTypeV
	}
	if src.FlashAttention != nil {
		dst.FlashAttention = src.FlashAttention
	}
	if src.PtrUseDirectIO != nil {
		dst.PtrUseDirectIO = src.PtrUseDirectIO
	}
	if src.PtrUseMMap != nil {
		dst.PtrUseMMap = src.PtrUseMMap
	}
	if src.NUMA != "" {
		dst.NUMA = src.NUMA
	}
	if src.PtrNSeqMax != nil {
		dst.PtrNSeqMax = src.PtrNSeqMax
	}
	if src.PtrOffloadKQV != nil {
		dst.PtrOffloadKQV = src.PtrOffloadKQV
	}
	if src.PtrOpOffload != nil {
		dst.PtrOpOffload = src.PtrOpOffload
	}
	if src.PtrOpOffloadMinBatch != nil {
		dst.PtrOpOffloadMinBatch = src.PtrOpOffloadMinBatch
	}
	if src.PtrNGpuLayers != nil {
		dst.PtrNGpuLayers = src.PtrNGpuLayers
	}
	if src.PtrSplitMode != nil {
		dst.PtrSplitMode = src.PtrSplitMode
	}
	if len(src.TensorSplit) > 0 {
		dst.TensorSplit = src.TensorSplit
	}
	if len(src.TensorBuftOverrides) > 0 {
		dst.TensorBuftOverrides = src.TensorBuftOverrides
	}
	if src.PtrMainGPU != nil {
		dst.PtrMainGPU = src.PtrMainGPU
	}
	if len(src.Devices) > 0 {
		dst.Devices = src.Devices
	}
	if src.MoE != nil {
		dst.MoE = src.MoE
	}
	if src.PtrSWAFull != nil {
		dst.PtrSWAFull = src.PtrSWAFull
	}
	if src.PtrIncrementalCache != nil {
		dst.PtrIncrementalCache = src.PtrIncrementalCache
	}
	if src.PtrCacheMinTokens != nil {
		dst.PtrCacheMinTokens = src.PtrCacheMinTokens
	}
	if src.PtrCacheSlotTimeout != nil {
		dst.PtrCacheSlotTimeout = src.PtrCacheSlotTimeout
	}
	if src.PtrInsecureLogging != nil {
		dst.PtrInsecureLogging = src.PtrInsecureLogging
	}
	if src.RopeScaling != 0 {
		dst.RopeScaling = src.RopeScaling
	}
	if src.PtrRopeFreqBase != nil {
		dst.PtrRopeFreqBase = src.PtrRopeFreqBase
	}
	if src.PtrRopeFreqScale != nil {
		dst.PtrRopeFreqScale = src.PtrRopeFreqScale
	}
	if src.PtrYarnExtFactor != nil {
		dst.PtrYarnExtFactor = src.PtrYarnExtFactor
	}
	if src.PtrYarnAttnFactor != nil {
		dst.PtrYarnAttnFactor = src.PtrYarnAttnFactor
	}
	if src.PtrYarnBetaFast != nil {
		dst.PtrYarnBetaFast = src.PtrYarnBetaFast
	}
	if src.PtrYarnBetaSlow != nil {
		dst.PtrYarnBetaSlow = src.PtrYarnBetaSlow
	}
	if src.PtrYarnOrigCtx != nil {
		dst.PtrYarnOrigCtx = src.PtrYarnOrigCtx
	}
	if src.DraftModel != nil {
		dst.DraftModel = src.DraftModel
	}

	// Merge sampling: src overrides non-zero fields in dst.
	dst.Sampling = mergeSampling(dst.Sampling, src.Sampling)
}

package models

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage"
	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/ram"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

// KronkResolvedConfig builds a model.Config for kronk.New() using the new
// resolution flow: analysis defaults (layer 1) overridden by user-supplied
// model_config.yaml entries (layer 3), then grammar resolution and on-disk
// file paths. Sampling defaults remain unset until the model is loaded so GGUF
// recommendations can take precedence over Kronk defaults.
//
// The catalog YAML middle layer used by the legacy resolution path is not
// applied here.
func (m *Models) KronkResolvedConfig(modelID string, mc map[string]ModelConfig, nativeSWAFull ...bool) (model.Config, error) {
	return m.kronkResolvedConfig(modelID, mc, nil, nativeSWAFull...)
}

// KronkResolvedConfigWithBudget builds a model.Config using a stable AutoTune
// budget. Pool callers use this so resident reservations do not affect sizing.
func (m *Models) KronkResolvedConfigWithBudget(modelID string, mc map[string]ModelConfig, budget AutoTuneBudget, nativeSWAFull ...bool) (model.Config, error) {
	return m.kronkResolvedConfig(modelID, mc, &budget, nativeSWAFull...)
}

func (m *Models) kronkResolvedConfig(modelID string, mc map[string]ModelConfig, budget *AutoTuneBudget, nativeSWAFull ...bool) (model.Config, error) {

	// Confirm the model is on disk before resolving anything else.
	fp, err := m.FullPath(modelID)
	if err != nil {
		return model.Config{}, fmt.Errorf("kronk-resolved-config: unable to get model[%s] path: %w", modelID, err)
	}

	// Layer 1: hardware-aware defaults derived from the GGUF file metadata.
	// Memory-affecting user settings participate in the analysis so the
	// recommendation is sized for the final overlaid configuration.
	override := mc[modelID]
	analysisOverride := override
	if analysisOverride.PtrSWAFull == nil && len(nativeSWAFull) > 0 {
		analysisOverride.PtrSWAFull = new(nativeSWAFull[0])
	}
	cfg := m.analysisDefaultsWithConfigAndBudget(modelID, analysisOverride, budget)
	sizing := cfg

	// Layer 3: user overrides from model_config.yaml.
	if _, ok := mc[modelID]; ok {
		MergeModelConfig(&cfg, override)
		restoreAutoTunedSizing(&cfg, sizing)
	}

	// Resolve grammar (.grm filename -> contents) before converting.
	if err := m.ResolveGrammar(&cfg.Sampling); err != nil {
		return model.Config{}, fmt.Errorf("kronk-resolved-config: %w", err)
	}

	// Convert to model.Config and attach on-disk paths.
	out := cfg.ToKronkConfig()
	out.AutoTune = true
	out.AutoTuned = autoTuneApplied(sizing)
	out.ModelFiles = fp.ModelFiles
	out.ProjFile = fp.ProjFile
	out.SessionStoreFactory, err = resolveSessionStoreFactory(cfg)
	if err != nil {
		return model.Config{}, fmt.Errorf("kronk-resolved-config: %w", err)
	}

	adapters, err := m.resolveAdapters(cfg.Adapters)
	if err != nil {
		return model.Config{}, fmt.Errorf("kronk-resolved-config: %w", err)
	}
	out.Adapters = adapters

	// fp.MTPFile is the on-disk path to the separate-file MTP assistant
	// drafter companion (e.g. Gemma4's "mtp-*.gguf"), discovered by the
	// download/catalog layer. The runtime config calls it MTPDrafterFile to
	// avoid confusion with MTP-capable MAIN models (whose own filenames
	// also start with "mtp-", e.g. mtp-Qwen3.6-...).
	out.MTPDrafterFile = fp.MTPFile

	// Resolve a relative jinja template path against the kronk base
	// directory so users can write portable values like
	// "jinja/Qwen3.5-0.8B-Q8_0.jinja" in model_config.yaml without
	// needing OS-specific home expansion.
	if out.JinjaFile != "" && !filepath.IsAbs(out.JinjaFile) {
		out.JinjaFile = filepath.Join(m.basePath, out.JinjaFile)
	}

	// Resolve draft model file paths if configured.
	if cfg.DraftModel != nil && cfg.DraftModel.ModelID != "" {
		draftPath, err := m.FullPath(cfg.DraftModel.ModelID)
		if err != nil {
			return model.Config{}, fmt.Errorf("kronk-resolved-config: unable to get draft model[%s] path: %w", cfg.DraftModel.ModelID, err)
		}

		if out.PtrDraftModel == nil {
			out.PtrDraftModel = &model.DraftModelConfig{}
		}

		out.PtrDraftModel.ModelFiles = draftPath.ModelFiles
	}

	return out, nil
}

func resolveSessionStoreFactory(cfg ModelConfig) (model.SessionStoreFactory, error) {
	switch cfg.SessionStoreKind {
	case kvstorage.Kind{}, kvstorage.RAM:
		return ram.NewFactory(), nil
	default:
		return nil, fmt.Errorf("session-store: unknown kind %q (valid: %q)", cfg.SessionStoreKind, kvstorage.RAM)
	}
}

func autoTuneApplied(cfg ModelConfig) bool {
	return cfg.PtrContextWindow != nil && cfg.PtrNSeqMax != nil
}

// restoreAutoTunedSizing ensures the resolved config uses the sizing values
// included in the AutoTune memory calculation.
func restoreAutoTunedSizing(cfg *ModelConfig, sizing ModelConfig) {
	if sizing.PtrContextWindow != nil {
		cfg.PtrContextWindow = sizing.PtrContextWindow
	}
	if sizing.PtrNSeqMax != nil {
		cfg.PtrNSeqMax = sizing.PtrNSeqMax
	}
	if sizing.CacheTypeK != model.GGMLTypeAuto {
		cfg.CacheTypeK = sizing.CacheTypeK
	}
	if sizing.CacheTypeV != model.GGMLTypeAuto {
		cfg.CacheTypeV = sizing.CacheTypeV
	}
}

// resolveAdapters converts user-facing adapter ids and paths into concrete
// runtime paths. Adapter ids are local identifiers, not model catalog ids.
func (m *Models) resolveAdapters(adapters []AdapterConfig) ([]model.AdapterConfig, error) {
	resolved := make([]model.AdapterConfig, 0, len(adapters))
	seen := make(map[string]struct{}, len(adapters))

	for i, adapter := range adapters {
		if (adapter.ID == "") == (adapter.Path == "") {
			return nil, fmt.Errorf("resolve-adapters: adapter[%d] must set exactly one of id or path", i)
		}

		var adapterPath string
		switch {
		case adapter.ID != "":
			if err := validateAdapterID(adapter.ID); err != nil {
				return nil, fmt.Errorf("resolve-adapters: adapter[%d] id %q: %w", i, adapter.ID, err)
			}
			adapterPath = filepath.Join(m.basePath, "lora", filepath.FromSlash(adapter.ID)+".gguf")

		default:
			if !filepath.IsAbs(adapter.Path) {
				return nil, fmt.Errorf("resolve-adapters: adapter[%d] path must be absolute: %q", i, adapter.Path)
			}
			adapterPath = adapter.Path
		}

		adapterPath = filepath.Clean(adapterPath)
		if !strings.EqualFold(filepath.Ext(adapterPath), ".gguf") {
			return nil, fmt.Errorf("resolve-adapters: adapter[%d] path must have a .gguf extension: %q", i, adapterPath)
		}
		if _, exists := seen[adapterPath]; exists {
			return nil, fmt.Errorf("resolve-adapters: duplicate adapter path: %q", adapterPath)
		}

		fi, err := os.Stat(adapterPath)
		if err != nil {
			return nil, fmt.Errorf("resolve-adapters: adapter[%d] path %q: %w", i, adapterPath, err)
		}
		if !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("resolve-adapters: adapter[%d] path is not a regular file: %q", i, adapterPath)
		}

		scale := float32(1)
		if adapter.PtrScale != nil {
			scale = *adapter.PtrScale
		}
		if scale < 0 || math.IsNaN(float64(scale)) || math.IsInf(float64(scale), 0) {
			return nil, fmt.Errorf("resolve-adapters: adapter[%d] scale must be finite and >= 0, got %g", i, scale)
		}

		seen[adapterPath] = struct{}{}
		resolved = append(resolved, model.AdapterConfig{Path: adapterPath, Scale: scale})
	}

	return resolved, nil
}

func validateAdapterID(id string) error {
	if id != strings.TrimSpace(id) || id == "" {
		return fmt.Errorf("must not be empty or contain surrounding whitespace")
	}
	if filepath.IsAbs(id) || strings.Contains(id, `\`) {
		return fmt.Errorf("must be a slash-separated relative id")
	}
	if strings.EqualFold(filepath.Ext(id), ".gguf") {
		return fmt.Errorf("must omit the .gguf extension")
	}

	for part := range strings.SplitSeq(id, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("contains an invalid path component")
		}
	}

	return nil
}

// AutoTune is the single source of analysis-derived defaults. It analyzes the
// given model facts against the available hardware and returns the recommended
// settings as a ModelConfig. Both the model pool (via AnalysisDefaults /
// KronkResolvedConfig) and the SDK auto-tune path (kronk.New with WithAutoTune)
// call it so the two never seed defaults differently. Callers overlay their own
// explicit settings on top of the returned config.
func AutoTune(info ModelInfo, devs devices.Devices) (ModelConfig, error) {
	return AutoTuneWithConfig(info, devs, ModelConfig{})
}

// AutoTuneWithConfig returns hardware-aware defaults while treating explicitly
// configured AutoTune-owned values as fixed constraints.
func AutoTuneWithConfig(info ModelInfo, devs devices.Devices, constraints ModelConfig) (ModelConfig, error) {
	return autoTuneWithConfigAndBudget(info, devs, constraints, nil)
}

func autoTuneWithConfigAndBudget(info ModelInfo, devs devices.Devices, constraints ModelConfig, budget *AutoTuneBudget) (ModelConfig, error) {
	analysis, err := analyzeModelWithConfigAndBudget(info, devs, constraints, budget)
	if err != nil {
		return ModelConfig{}, fmt.Errorf("auto-tune: %w", err)
	}

	rec := analysis.Recommended

	var cfg ModelConfig

	// NBatch and NUBatch are intentionally left unset here so the load-time
	// adjustConfig (the single source of batch sizing) derives them: NUBatch
	// defaults to 2048 and NBatch to NUBatch * NSeqMax.
	cfg.PtrContextWindow = new(int(rec.ContextWindow))
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

	// Preserve an explicit split mode. When unset, leave it unset so the load
	// path can resolve it from the devices selected by the final configuration,
	// rather than all devices visible during analysis.
	cfg.PtrSplitMode = constraints.PtrSplitMode

	// model.Config: PtrNGpuLayers nil = all on GPU, 0 = all on GPU, -1 = all on CPU.
	// Preserve an explicit constraint, including pointer-to-zero. Otherwise only
	// set the pointer when AutoTune explicitly recommends CPU-only execution.
	if constraints.PtrNGpuLayers != nil {
		cfg.PtrNGpuLayers = constraints.PtrNGpuLayers
	} else if rec.NGPULayers < 0 {
		n := int(rec.NGPULayers)
		cfg.PtrNGpuLayers = &n
	}

	return cfg, nil
}

// AnalysisDefaults runs the hardware analysis on a catalog model and returns
// the recommended settings as a ModelConfig. It is the catalog/modelID-based
// entry point onto the shared AutoTune logic. If the model is not downloaded or
// analysis fails, an empty ModelConfig is returned.
func (m *Models) AnalysisDefaults(modelID string) ModelConfig {
	return m.AnalysisDefaultsWithConfig(modelID, ModelConfig{})
}

// AnalysisDefaultsWithConfig returns analysis-derived defaults while treating
// explicitly configured context, concurrency, and cache types as constraints.
func (m *Models) AnalysisDefaultsWithConfig(modelID string, constraints ModelConfig) ModelConfig {
	return m.analysisDefaultsWithConfigAndBudget(modelID, constraints, nil)
}

func (m *Models) analysisDefaultsWithConfigAndBudget(modelID string, constraints ModelConfig, budget *AutoTuneBudget) ModelConfig {
	info, err := m.ModelInformation(modelID)
	if err != nil {
		return ModelConfig{}
	}

	cfg, err := autoTuneWithConfigAndBudget(info, devices.List(), constraints, budget)
	if err != nil {
		return ModelConfig{}
	}

	return cfg
}

// MergeModelConfig overlays non-zero fields from src onto dst.
func MergeModelConfig(dst *ModelConfig, src ModelConfig) {
	if len(src.Adapters) > 0 {
		dst.Adapters = src.Adapters
	}
	if src.PtrAdmissionTimeout != nil {
		dst.PtrAdmissionTimeout = src.PtrAdmissionTimeout
	}
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
	if src.PtrLoadMode != nil {
		dst.PtrLoadMode = src.PtrLoadMode
	}
	if src.NUMA != "" {
		dst.NUMA = src.NUMA
	}
	if src.PtrNSeqMax != nil {
		dst.PtrNSeqMax = src.PtrNSeqMax
	}
	if src.PtrIMCSessionCapacity != nil {
		dst.PtrIMCSessionCapacity = src.PtrIMCSessionCapacity
	}
	if src.PtrQueueDepth != nil {
		dst.PtrQueueDepth = src.PtrQueueDepth
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
	if src.SessionStoreKind != (kvstorage.Kind{}) {
		dst.SessionStoreKind = src.SessionStoreKind
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

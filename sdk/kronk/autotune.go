package kronk

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// autoTune seeds unset settings from a hardware-aware analysis of the model and
// returns the resulting Config. It uses the same shared models.AutoTune logic as
// the model pool so the SDK and pool seed defaults identically; the only
// SDK-specific part is the override mechanism: the analysis result is the base
// and the caller's functional options are re-applied on top. The analyzed
// context, concurrency, and cache types are then restored because they already
// include explicit sizing constraints and must match the configuration that was
// sized. On any failure the original cfg is returned unchanged so auto-tune
// never blocks a load.
func autoTune(ctx context.Context, cfg model.Config, opts []model.Option) model.Config {
	if len(cfg.ModelFiles) == 0 {
		logAutoTune(ctx, cfg.Log, "status", "skipped", "reason", "no model files configured")
		return cfg
	}

	info, err := models.ModelInfoFromPath("", cfg.ModelFiles, cfg.ProjFile, "")
	if err != nil {
		logAutoTune(ctx, cfg.Log, "status", "skipped", "error", fmt.Sprintf("model-info: %v", err))
		return cfg
	}

	constraints := models.ModelConfig{
		PtrContextWindow: cfg.PtrContextWindow,
		PtrNSeqMax:       cfg.PtrNSeqMax,
		CacheTypeK:       cfg.CacheTypeK,
		CacheTypeV:       cfg.CacheTypeV,
		FlashAttention:   cfg.PtrFlashAttention,
		PtrSplitMode:     cfg.PtrSplitMode,
		PtrNGpuLayers:    cfg.PtrNGpuLayers,
	}
	base, err := models.AutoTuneWithConfig(info, devices.List(), constraints)
	if err != nil {
		logAutoTune(ctx, cfg.Log, "status", "skipped", "error", err.Error())
		return cfg
	}

	// Analysis recommendation is the base; the user's non-AutoTune options
	// override it. Restore the analyzed fields after replay because WithConfig
	// and explicit auto values can otherwise erase the recommendation.
	recommended := base.ToKronkConfig()
	tuned := recommended
	for _, opt := range opts {
		opt(&tuned)
	}
	restoreAutoTunedFields(&tuned, recommended)
	tuned.AutoTune = true
	tuned.AutoTuned = true

	selected, constrained := autoTuneLogValues(tuned, constraints)
	logAutoTune(ctx, cfg.Log,
		"status", "applied",
		"context_window", tuned.ContextWindow(),
		"nseq_max", tuned.NSeqMax(),
		"cache_type_k", tuned.CacheTypeK,
		"cache_type_v", tuned.CacheTypeV,
		"flash_attention", tuned.FlashAttention(),
		"split_mode", splitModeName(tuned.PtrSplitMode),
		"ngpu_layers", nGpuLayersName(tuned.PtrNGpuLayers),
		"selected", selected,
		"constraints", constrained,
	)

	return tuned
}

// restoreAutoTunedFields ensures the runtime uses the exact values included in
// the AutoTune recommendation and its explicit constraints.
func restoreAutoTunedFields(tuned *model.Config, recommended model.Config) {
	tuned.PtrContextWindow = recommended.PtrContextWindow
	tuned.PtrNSeqMax = recommended.PtrNSeqMax
	tuned.CacheTypeK = recommended.CacheTypeK
	tuned.CacheTypeV = recommended.CacheTypeV
	tuned.PtrFlashAttention = recommended.PtrFlashAttention
	tuned.PtrSplitMode = recommended.PtrSplitMode
	tuned.PtrNGpuLayers = recommended.PtrNGpuLayers
}

func autoTuneLogValues(cfg model.Config, constraints models.ModelConfig) ([]string, []string) {
	selected := make([]string, 0, 7)
	constrained := make([]string, 0, 7)
	add := func(explicit bool, value string) {
		if explicit {
			constrained = append(constrained, value)
			return
		}
		selected = append(selected, value)
	}

	add(constraints.PtrContextWindow != nil, fmt.Sprintf("context_window=%d", cfg.ContextWindow()))
	add(constraints.PtrNSeqMax != nil, fmt.Sprintf("nseq_max=%d", cfg.NSeqMax()))
	add(constraints.CacheTypeK != model.GGMLTypeAuto, fmt.Sprintf("cache_type_k=%s", cfg.CacheTypeK))
	add(constraints.CacheTypeV != model.GGMLTypeAuto, fmt.Sprintf("cache_type_v=%s", cfg.CacheTypeV))
	add(constraints.FlashAttention != nil, fmt.Sprintf("flash_attention=%s", cfg.FlashAttention()))
	add(constraints.PtrSplitMode != nil, fmt.Sprintf("split_mode=%s", splitModeName(cfg.PtrSplitMode)))
	add(constraints.PtrNGpuLayers != nil, fmt.Sprintf("ngpu_layers=%s", nGpuLayersName(cfg.PtrNGpuLayers)))

	return selected, constrained
}

// splitModeName renders a split-mode pointer for logging; nil means the
// device-aware default is left to apply at load time.
func splitModeName(p *model.SplitMode) string {
	if p == nil {
		return "auto"
	}
	return p.String()
}

func nGpuLayersName(p *int) string {
	if p == nil {
		return "all"
	}
	return fmt.Sprintf("%d", *p)
}

// logAutoTune emits an auto-tune log line when a logger is configured.
func logAutoTune(ctx context.Context, l applog.Logger, args ...any) {
	if l == nil {
		return
	}
	l(ctx, "AUTO-TUNE", args...)
}

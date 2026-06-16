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
// and the caller's functional options are re-applied on top, so explicit options
// always win. The original opts are required (not just the built cfg) to
// preserve "explicitly set" vs "left at zero". On any failure the original cfg
// is returned unchanged so auto-tune never blocks a load.
func autoTune(ctx context.Context, cfg model.Config, opts []model.Option) model.Config {
	if len(cfg.ModelFiles) == 0 {
		logAutoTune(ctx, cfg.Log, "status", "skipped", "reason", "no model files configured")
		return cfg
	}

	info, err := models.ModelInfoFromPath("", cfg.ModelFiles, cfg.ProjFile)
	if err != nil {
		logAutoTune(ctx, cfg.Log, "status", "skipped", "error", fmt.Sprintf("model-info: %v", err))
		return cfg
	}

	base, err := models.AutoTune(info, devices.List())
	if err != nil {
		logAutoTune(ctx, cfg.Log, "status", "skipped", "error", err.Error())
		return cfg
	}

	// Analysis recommendation is the base; the user's explicit options override.
	tuned := base.ToKronkConfig()
	tuned.AutoTune = true
	for _, opt := range opts {
		opt(&tuned)
	}

	logAutoTune(ctx, cfg.Log,
		"status", "applied",
		"context_window", tuned.ContextWindow(),
		"nseq_max", tuned.NSeqMax(),
		"cache_type_k", tuned.CacheTypeK,
		"cache_type_v", tuned.CacheTypeV,
		"split_mode", splitModeName(tuned.PtrSplitMode),
	)

	return tuned
}

// splitModeName renders a split-mode pointer for logging; nil means the
// device-aware default is left to apply at load time.
func splitModeName(p *model.SplitMode) string {
	if p == nil {
		return "auto"
	}
	return p.String()
}

// logAutoTune emits an auto-tune log line when a logger is configured.
func logAutoTune(ctx context.Context, l applog.Logger, args ...any) {
	if l == nil {
		return
	}
	l(ctx, "AUTO-TUNE", args...)
}

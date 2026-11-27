package model

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

var (
	speculationSourceNone    = internalspec.SourceNone
	speculationSourceClassic = internalspec.SourceClassic
	speculationSourceMTP     = internalspec.SourceMTP
)

// resolveSpeculationPlan coordinates source selection. Algorithm adapters own
// capability discovery and config translation; internal packages own policy.
func resolveSpeculationPlan(ctx context.Context, log applog.Logger, cfg Config) (internalspec.Plan, error) {
	mode := cfg.SpeculationMode()
	classic := cfg.PtrDraftModel != nil && cfg.PtrDraftModel.IsSeparate()
	if mode == SpeculationDisabled || classic && mode == SpeculationAuto || mode == SpeculationClassic {
		return internalspec.Resolve(internalspec.Config{
			Mode:              mode,
			ClassicConfigured: classic,
			ClassicNDraft:     configuredClassicNDraft(cfg),
		})
	}

	if mode == SpeculationDSpark && (classic || cfg.DrafterFile == "") {
		return internalspec.Resolve(internalspec.Config{Mode: mode, ClassicConfigured: classic})
	}

	companion := modelprofile.MTPArchitectureNone
	if cfg.DrafterFile != "" && mode != SpeculationDSpark {
		companion = probeMTPCompanion(ctx, log, cfg.DrafterFile)
	}

	if (mode == SpeculationDSpark || mode == SpeculationAuto && companion.IsZero()) && cfg.DrafterFile != "" {
		plan, err := resolveDSparkSpeculationPlan(ctx, log, cfg)
		if err != nil || plan.Source == internalspec.SourceDSpark {
			return plan, err
		}
	}

	return resolveMTPSpeculationPlan(cfg, companion)
}

func configuredClassicNDraft(cfg Config) int {
	if cfg.PtrDraftModel != nil && cfg.PtrDraftModel.NDraft > 0 {
		return cfg.PtrDraftModel.NDraft
	}
	return defNDraft
}

package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
)

// SpeculationMode selects the speculative-decoding implementation for a model.
type SpeculationMode = internalspec.Mode

const (
	SpeculationAuto     = internalspec.ModeAuto
	SpeculationDisabled = internalspec.ModeDisabled
	SpeculationClassic  = internalspec.ModeClassic
	SpeculationMTP      = internalspec.ModeMTP

	speculationSourceNone         = internalspec.SourceNone
	speculationSourceClassic      = internalspec.SourceClassic
	speculationSourceMTPCompanion = internalspec.SourceMTPCompanion
	speculationSourceMTPEmbedded  = internalspec.SourceMTPEmbedded
)

type speculationPlan = internalspec.Plan

func resolveSpeculationPlan(ctx context.Context, log applog.Logger, cfg Config) (speculationPlan, error) {
	mode := cfg.SpeculationMode()
	classic := cfg.PtrDraftModel != nil && cfg.PtrDraftModel.IsSeparate()
	if mode == SpeculationDisabled || classic && mode == SpeculationAuto || mode == SpeculationClassic {
		return internalspec.Resolve(internalspec.Config{
			Mode:              mode,
			ClassicConfigured: classic,
			ClassicNDraft:     configuredClassicNDraft(cfg),
		})
	}

	embedded, err := modelFilesLoadMTP(cfg.ModelFiles)
	if err != nil {
		return speculationPlan{}, fmt.Errorf("detect embedded MTP: %w", err)
	}
	companion := cfg.MTPDrafterFile != "" && probeSharedKVCompanionMTP(ctx, log, cfg.MTPDrafterFile)

	return internalspec.Resolve(internalspec.Config{
		Mode:              mode,
		ClassicConfigured: classic,
		ClassicNDraft:     configuredClassicNDraft(cfg),
		MTPNDraft:         mtpNDraft(cfg),
		EmbeddedMTP:       embedded,
		CompanionMTP:      companion,
		MTPAvailable:      MTPAvailable(),
	})
}

// resolveEmbeddedMTPCompatibility verifies that the embedded MTP head consumes
// the target model's hidden-state width. Automatic speculation falls back to
// target-only generation when the widths differ; an explicitly required MTP
// implementation fails instead.
func resolveEmbeddedMTPCompatibility(plan speculationPlan, targetEmbeddingWidth, mtpOutputWidth int32) (speculationPlan, error) {
	if plan.Source != speculationSourceMTPEmbedded || targetEmbeddingWidth == mtpOutputWidth {
		return plan, nil
	}

	if plan.Mode == SpeculationMTP {
		return speculationPlan{}, fmt.Errorf("embedded MTP output width %d does not match target embedding width %d", mtpOutputWidth, targetEmbeddingWidth)
	}

	return speculationPlan{Mode: plan.Mode}, nil
}

func configuredClassicNDraft(cfg Config) int {
	if cfg.PtrDraftModel != nil && cfg.PtrDraftModel.NDraft > 0 {
		return cfg.PtrDraftModel.NDraft
	}
	return defNDraft
}

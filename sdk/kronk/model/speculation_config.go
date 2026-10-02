package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
)

// SpeculationMode selects the speculative-decoding implementation for a model.
type SpeculationMode = internalspec.Mode
type mtpArtifact = internalspec.MTPArtifact

const (
	SpeculationAuto     = internalspec.ModeAuto
	SpeculationDisabled = internalspec.ModeDisabled
	SpeculationClassic  = internalspec.ModeClassic
	SpeculationMTP      = internalspec.ModeMTP

	speculationSourceNone    = internalspec.SourceNone
	speculationSourceClassic = internalspec.SourceClassic
	speculationSourceMTP     = internalspec.SourceMTP

	mtpArchitectureQwen35OwnKV   = internalspec.MTPArchitectureQwen35OwnKV
	mtpArchitectureQwen4ExpOwnKV = internalspec.MTPArchitectureQwen4ExpOwnKV
	mtpArchitectureGemmaSharedKV = internalspec.MTPArchitectureGemmaSharedKV

	mtpArtifactEmbedded  = internalspec.MTPArtifactEmbedded
	mtpArtifactCompanion = internalspec.MTPArtifactCompanion
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

	embedded, err := modelFilesMTPArchitecture(cfg.ModelFiles)
	if err != nil {
		return speculationPlan{}, fmt.Errorf("detect embedded MTP: %w", err)
	}
	companion := modelprofile.MTPArchitectureNone
	if cfg.MTPDrafterFile != "" {
		companion = probeMTPCompanion(ctx, log, cfg.MTPDrafterFile)
	}
	architecture := companion
	if architecture == modelprofile.MTPArchitectureNone {
		architecture = embedded
	}

	return internalspec.Resolve(internalspec.Config{
		Mode:                     mode,
		ClassicConfigured:        classic,
		ClassicNDraft:            configuredClassicNDraft(cfg),
		MTPNDraft:                mtpNDraft(cfg),
		EmbeddedMTPArchitecture:  internalMTPArchitecture(embedded),
		CompanionMTPArchitecture: internalMTPArchitecture(companion),
		MTPAvailable:             yzmaspec.Available(),
		MTPArchitectureEnabled:   modelprofile.MTPEnabled(architecture),
	})
}

func internalMTPArchitecture(architecture modelprofile.MTPArchitecture) internalspec.MTPArchitecture {
	switch architecture {
	case modelprofile.MTPArchitectureQwen35OwnKV:
		return internalspec.MTPArchitectureQwen35OwnKV
	case modelprofile.MTPArchitectureQwen4ExpOwnKV:
		return internalspec.MTPArchitectureQwen4ExpOwnKV
	case modelprofile.MTPArchitectureGemmaSharedKV:
		return internalspec.MTPArchitectureGemmaSharedKV
	default:
		return internalspec.MTPArchitectureNone
	}
}

// resolveEmbeddedMTPCompatibility verifies the hidden-state width consumed by
// an embedded MTP head. Qwen35 requires the target embedding width; Qwen4Exp
// can expose an integral multiple containing several hidden-state planes.
// Automatic speculation falls back to target-only generation on mismatch; an
// explicitly required MTP implementation fails instead.
func resolveEmbeddedMTPCompatibility(plan speculationPlan, targetEmbeddingWidth, mtpOutputWidth int32) (speculationPlan, error) {
	if plan.Source != speculationSourceMTP || plan.MTPArtifact != mtpArtifactEmbedded {
		return plan, nil
	}
	compatible := targetEmbeddingWidth > 0 && targetEmbeddingWidth == mtpOutputWidth
	if plan.MTPArchitecture == mtpArchitectureQwen4ExpOwnKV {
		compatible = targetEmbeddingWidth > 0 && mtpOutputWidth >= targetEmbeddingWidth && mtpOutputWidth%targetEmbeddingWidth == 0
	}
	if compatible {
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

package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	dsparkengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/dspark"
)

// resolveDSparkSpeculationPlan adapts model configuration and artifacts to the
// DSpark policy. Incompatible automatic selections return an empty plan so the
// shared selector can continue to another implementation.
func resolveDSparkSpeculationPlan(ctx context.Context, log applog.Logger, cfg Config) (internalspec.Plan, error) {
	if err := verifyConfiguredArtifact(ctx, cfg, log, cfg.DrafterFile); err != nil {
		return internalspec.Plan{}, fmt.Errorf("drafter artifact %q: %w", cfg.DrafterFile, err)
	}

	nDraft, err := dsparkProposalLimit(cfg)
	if err != nil {
		if cfg.SpeculationMode() == SpeculationDSpark {
			return internalspec.Plan{}, fmt.Errorf("DSpark companion %q: %w", cfg.DrafterFile, err)
		}
		if log != nil {
			log(ctx, "speculation", "source", internalspec.SourceDSpark, "file", cfg.DrafterFile,
				"reason", "ignoring incompatible DSpark companion", "ERROR", err)
		}
		return internalspec.Plan{}, nil
	}

	// Keep the selection visible without activating an unfinished runtime.
	plan, err := internalspec.Resolve(internalspec.Config{
		Mode: cfg.SpeculationMode(), DSparkConfigured: true, DSparkNDraft: nDraft,
	})
	if log != nil && err == nil {
		log(ctx, "speculation", "source", plan.Source, "active", false,
			"reason", "DSpark execution is not yet available in this Kronk release")
	}
	return plan, err
}

// dsparkProposalLimit reads model-owned artifacts; the internal DSpark package
// validates their metadata and computes the trained proposal bound.
func dsparkProposalLimit(cfg Config) (int, error) {
	if len(cfg.ModelFiles) == 0 {
		return 0, fmt.Errorf("no target model files provided")
	}
	var metadata [2]map[string]string
	for i, file := range []string{cfg.ModelFiles[0], cfg.DrafterFile} {
		data, err := gguf.ReadHeaderBytes(file)
		if err != nil {
			return 0, fmt.Errorf("read %q: %w", file, err)
		}
		metadata[i], err = gguf.ParseMetadata(data)
		if err != nil {
			return 0, fmt.Errorf("parse %q: %w", file, err)
		}
	}
	requested := 0
	if cfg.PtrDraftModel != nil && !cfg.PtrDraftModel.IsSeparate() {
		requested = cfg.PtrDraftModel.NDraft
	}
	return dsparkengine.ProposalLimit(metadata[0], metadata[1], requested)
}

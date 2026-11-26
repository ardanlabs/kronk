package model

import (
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
)

// CompanionMemory estimates independently shaped companion allocations in
// addition to target-context accounting. Currently this covers DSpark's own
// context and feature storage; target-shaped MTP contexts are accounted through
// SpeculativeContextCount. Disabled or unselected companions contribute nothing.
func CompanionMemory(cfg Config, target vram.Config) (vram.Result, error) {
	if cfg.SpeculationMode() != SpeculationAuto || cfg.DrafterFile == "" ||
		cfg.PtrDraftModel != nil && cfg.PtrDraftModel.IsSeparate() {
		return vram.Result{}, nil
	}
	if _, err := dsparkProposalLimit(cfg); err != nil {
		return vram.Result{}, nil
	}
	target.ComputeContexts = 1
	target.RecurrentStateCopies = 1
	target.EmbeddedMTPStateCopies = 1
	target.MoECacheSize = 0
	target.MTPContext = false
	return vram.FromFiles([]string{cfg.DrafterFile}, target)
}

// RecurrentStateCopies returns one plus the configured recurrent rollback depth.
// Snapshot architectures allocate that many state planes; replay architectures
// retain one state plus that many pending input columns. The same bound sizes
// speculative output rows independently of the recurrent allocation strategy.
// embeddedMTP selects the auto-detected MTP path; false selects an explicit
// separate drafter or companion MTP file.
func RecurrentStateCopies(cfg Config, embeddedMTP bool) int64 {
	mode := cfg.SpeculationMode()
	if mode == SpeculationDisabled {
		return 1
	}

	if embeddedMTP {
		if mode != SpeculationClassic && yzmaspec.Available() && mtpFilesEnabled(cfg.ModelFiles) {
			return int64(1 + mtpNDraft(cfg))
		}
		return 1
	}

	if mode != SpeculationMTP && cfg.PtrDraftModel != nil && cfg.PtrDraftModel.IsSeparate() {
		nDraft := cfg.PtrDraftModel.NDraft
		if nDraft <= 0 {
			nDraft = defNDraft
		}
		return int64(1 + nDraft)
	}
	if mode != SpeculationClassic && cfg.DrafterFile != "" && yzmaspec.Available() && mtpFilesEnabled([]string{cfg.DrafterFile}) {
		return int64(1 + mtpNDraft(cfg))
	}

	return 1
}

// SpeculativeContextCount returns the native contexts known from configuration
// before the target GGUF metadata is inspected. A companion MTP model creates
// a second target-shaped context. A separate classic drafter is estimated from
// its own GGUF, while embedded MTP is detected from the target's NextN metadata.
func SpeculativeContextCount(cfg Config) int64 {
	if cfg.SpeculationMode() == SpeculationDisabled || cfg.SpeculationMode() == SpeculationClassic || cfg.SpeculationMode() == SpeculationDSpark {
		return 1
	}
	if cfg.DrafterFile != "" && mtpFilesEnabled([]string{cfg.DrafterFile}) {
		return 2
	}
	return 1
}

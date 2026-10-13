// Package speculation defines the model-internal speculative-decoding contract.
package speculation

import (
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Active reports whether the selected implementation can run.
func (p Plan) Active() bool {
	return p.Source != SourceNone && p.Available
}

// MTP reports whether the plan selects an MTP implementation.
func (p Plan) MTP() bool {
	return p.Source == SourceMTP
}

// RowsPerSequence returns the worst-case target rows contributed per sequence.
func (p Plan) RowsPerSequence() int {
	if !p.Active() {
		return 1
	}
	return 1 + p.NDraft
}

// Resolve selects one concrete implementation from config and capabilities.
func Resolve(cfg Config) (Plan, error) {
	switch cfg.Mode {
	case ModeDisabled:
		return Plan{Mode: cfg.Mode}, nil

	case ModeClassic:
		if !cfg.ClassicConfigured {
			return Plan{}, fmt.Errorf("speculation mode %q requires a separate draft model", cfg.Mode)
		}
		return Plan{Mode: cfg.Mode, Source: SourceClassic, NDraft: cfg.ClassicNDraft, Available: true}, nil

	case ModeMTP:
		if cfg.ClassicConfigured {
			return Plan{}, fmt.Errorf("speculation mode %q cannot use a separate draft model", cfg.Mode)
		}

	case ModeAuto:
		if cfg.ClassicConfigured {
			return Plan{Mode: cfg.Mode, Source: SourceClassic, NDraft: cfg.ClassicNDraft, Available: true}, nil
		}

	default:
		return Plan{}, fmt.Errorf("unknown speculation mode %q", cfg.Mode)
	}

	plan := Plan{
		Mode:      cfg.Mode,
		NDraft:    cfg.MTPNDraft,
		Available: cfg.MTPAvailable,
	}
	switch {
	case cfg.CompanionMTPArchitecture != MTPArchitectureNone:
		plan.Source = SourceMTP
		plan.MTPArchitecture = cfg.CompanionMTPArchitecture
		plan.MTPArtifact = MTPArtifactCompanion
	case cfg.EmbeddedMTPArchitecture != MTPArchitectureNone:
		plan.Source = SourceMTP
		plan.MTPArchitecture = cfg.EmbeddedMTPArchitecture
		plan.MTPArtifact = MTPArtifactEmbedded
	case cfg.Mode == ModeMTP:
		return Plan{}, fmt.Errorf("speculation mode %q requested but the model has no companion or embedded MTP implementation", cfg.Mode)
	}
	if plan.Source == SourceMTP && !cfg.MTPArchitectureEnabled {
		if cfg.Mode == ModeMTP {
			return Plan{}, fmt.Errorf("speculation mode %q requested but MTP architecture %s is unsupported by this Kronk release", cfg.Mode, plan.MTPArchitecture)
		}
		return Plan{Mode: cfg.Mode}, nil
	}
	plan.LoadMTP = plan.MTPArtifact == MTPArtifactEmbedded && plan.Available
	if cfg.Mode == ModeMTP && !plan.Available {
		return Plan{}, fmt.Errorf("speculation mode %q requested but MTP is unavailable in the loaded llama library", cfg.Mode)
	}

	return plan, nil
}

// TargetRange identifies rows contributed by one slot to a target decode.
type TargetRange struct {
	Start   int32
	Count   int32
	BasePos llama.Pos
}

// Generation contains candidates proposed for one target verification round.
type Generation struct {
	Candidates []llama.Token
	Mode       string
}

// Host is the narrow batch-engine surface available to implementations.
type Host interface {
	SlotCount() int
	SlotActive(slot int) bool
	ProcessOrdinary(slot int, buf []byte)
}

// Controller owns the speculative lifecycle around each target decode.
type Controller interface {
	Mode() Mode
	Enabled() bool
	BeginBatch()
	Prepare()
	PlanGeneration(slot int) (Generation, error)
	CommitGeneration(slot int, candidates []llama.Token, targetRange TargetRange) error
	TargetRowsStaged(slot int, targetRange TargetRange)
	AfterTargetDecode(buf []byte)
}

// Package speculation defines the model-internal speculative-decoding contract.
package speculation

import (
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Mode selects the speculative-decoding implementation for a model.
type Mode string

const (
	// ModeAuto selects a classic drafter when explicitly configured, otherwise
	// a companion or embedded MTP implementation when available.
	ModeAuto Mode = "auto"

	// ModeDisabled runs target-only generation.
	ModeDisabled Mode = "disabled"

	// ModeClassic requires an explicitly configured separate draft model.
	ModeClassic Mode = "classic"

	// ModeMTP requires a companion or embedded MTP implementation.
	ModeMTP Mode = "mtp"
)

// Source identifies the broad speculation engine selected for a model.
type Source uint8

const (
	// SourceNone selects target-only generation.
	SourceNone Source = iota

	// SourceClassic selects a separate-GGUF drafter.
	SourceClassic

	// SourceMTP selects an architecture-specific MTP backend.
	SourceMTP
)

// MTPArchitecture identifies the model-family runtime contract. It is kept
// separate from MTPArtifact because embedded and companion Qwen35 heads use
// the same own-KV synchronization and rollback behavior.
type MTPArchitecture uint8

const (
	MTPArchitectureNone MTPArchitecture = iota
	MTPArchitectureQwen35OwnKV
	MTPArchitectureGemmaSharedKV
)

// MTPArtifact identifies where an MTP head's weights are stored.
type MTPArtifact uint8

const (
	MTPArtifactNone MTPArtifact = iota
	MTPArtifactEmbedded
	MTPArtifactCompanion
)

// Config contains the capabilities needed to resolve one speculation plan.
type Config struct {
	Mode              Mode
	ClassicConfigured bool
	ClassicNDraft     int
	MTPNDraft         int
	EmbeddedMTP       bool
	CompanionMTP      bool
	OwnKVCompanionMTP bool
	MTPAvailable      bool
}

// Plan is the immutable decision shared by model loading, context sizing, and
// runtime controller construction.
type Plan struct {
	Mode            Mode
	Source          Source
	MTPArchitecture MTPArchitecture
	MTPArtifact     MTPArtifact
	NDraft          int
	LoadMTP         bool
	Available       bool
}

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
	case cfg.CompanionMTP:
		plan.Source = SourceMTP
		plan.MTPArchitecture = MTPArchitectureGemmaSharedKV
		plan.MTPArtifact = MTPArtifactCompanion
	case cfg.OwnKVCompanionMTP:
		plan.Source = SourceMTP
		plan.MTPArchitecture = MTPArchitectureQwen35OwnKV
		plan.MTPArtifact = MTPArtifactCompanion
	case cfg.EmbeddedMTP:
		plan.Source = SourceMTP
		plan.MTPArchitecture = MTPArchitectureQwen35OwnKV
		plan.MTPArtifact = MTPArtifactEmbedded
		plan.LoadMTP = true
	case cfg.Mode == ModeMTP:
		return Plan{}, fmt.Errorf("speculation mode %q requested but the model has no companion or embedded MTP implementation", cfg.Mode)
	}
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

package classic

import (
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const probeInterval = 32

// GenerationInput contains policy state and the low-level draft operation.
type GenerationInput struct {
	State    *SlotState
	MaxDraft int
	Generate func(count int) (GenerationResult, error)
}

// GenerationResult contains one classic speculative proposal.
type GenerationResult struct {
	Candidates    []llama.Token
	Distributions [][]Candidate
}

// Generate applies adaptive draft sizing and invokes the draft backend.
func Generate(input GenerationInput) (GenerationResult, error) {
	if input.State == nil || input.MaxDraft <= 0 {
		return GenerationResult{}, nil
	}
	count := input.State.draftCount(input.MaxDraft)
	if count == 0 {
		input.State.DraftDistributions = nil
		return GenerationResult{}, nil
	}
	if input.Generate == nil {
		return GenerationResult{}, fmt.Errorf("generate callback is nil")
	}
	result, err := input.Generate(count)
	if err != nil {
		return GenerationResult{}, err
	}
	input.State.DraftDistributions = result.Distributions
	return result, nil
}

func (s *SlotState) draftCount(maxDraft int) int {
	switch {
	case s.AcceptanceEMA < 0.30:
		s.ProbeTick++
		if s.ProbeTick >= probeInterval {
			s.ProbeTick = 0
			return min(1, maxDraft)
		}
		return 0
	case s.AcceptanceEMA < 0.50:
		s.ProbeTick = 0
		return min(1, maxDraft)
	case s.AcceptanceEMA < 0.70:
		return min(2, maxDraft)
	case s.AcceptanceEMA < 0.85:
		return min(3, maxDraft)
	default:
		return maxDraft
	}
}

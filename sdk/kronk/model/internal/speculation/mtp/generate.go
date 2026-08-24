package mtp

import (
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// DraftInput contains the MTP-owned inputs for one autoregressive draft round.
// DecodeStep is deliberately limited to one llama decode operation; the token
// loop, hidden-state progression, and fixed-position policy live here.
type DraftInput struct {
	State           *SlotState
	Policy          *Policy
	Token           llama.Token
	Position        llama.Pos
	Hidden          []float32
	Count           int
	ConfiguredCount int
	FixedPosition   bool
	Candidates      []llama.Token
	HiddenScratch   []float32
	IsEOG           func(llama.Token) bool
	DecodeStep      func(token llama.Token, position llama.Pos, hidden []float32) (llama.Token, []float32, bool, error)
}

// DraftResult contains the candidates and resulting MTP position/state.
type DraftResult struct {
	Candidates []llama.Token
	Position   llama.Pos
	Hidden     []float32
}

// Generate runs one MTP autoregressive candidate-generation round.
func Generate(input DraftInput) (DraftResult, error) {
	hidden := input.HiddenScratch
	if cap(hidden) < len(input.Hidden) {
		hidden = make([]float32, len(input.Hidden))
	} else {
		hidden = hidden[:len(input.Hidden)]
	}
	copy(hidden, input.Hidden)

	result := DraftResult{
		Candidates: input.Candidates[:0],
		Position:   input.Position,
		Hidden:     hidden,
	}
	count := input.Count
	if input.State != nil && input.Policy != nil {
		count = input.Policy.draftCountAt(input.State, count, input.ConfiguredCount, time.Now())
	}
	if count <= 0 || len(input.Hidden) == 0 {
		return result, nil
	}

	token := input.Token
	for range count {
		nextToken, nextHidden, decoded, err := input.DecodeStep(token, result.Position, result.Hidden)
		if err != nil {
			return DraftResult{}, err
		}
		if !decoded {
			break
		}
		if !input.FixedPosition {
			result.Position++
		}

		result.Candidates = append(result.Candidates, nextToken)
		if len(nextHidden) == 0 {
			break
		}

		if cap(result.Hidden) < len(nextHidden) {
			result.Hidden = make([]float32, len(nextHidden))
		} else {
			result.Hidden = result.Hidden[:len(nextHidden)]
		}
		copy(result.Hidden, nextHidden)
		if input.IsEOG(nextToken) {
			break
		}
		token = nextToken
	}

	return result, nil
}

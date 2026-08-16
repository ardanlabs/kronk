package mtp

import "github.com/hybridgroup/yzma/pkg/llama"

// DraftInput contains the MTP-owned inputs for one autoregressive draft round.
// DecodeStep is deliberately limited to one llama decode operation; the token
// loop, hidden-state progression, and fixed-position policy live here.
type DraftInput struct {
	Token         llama.Token
	Position      llama.Pos
	Hidden        []float32
	Count         int
	FixedPosition bool
	Candidates    []llama.Token
	HiddenScratch []float32
	IsEOG         func(llama.Token) bool
	DecodeStep    func(token llama.Token, position llama.Pos, hidden []float32) (llama.Token, []float32, bool, error)
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
	if input.Count <= 0 || len(input.Hidden) == 0 {
		return result, nil
	}

	token := input.Token
	for range input.Count {
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

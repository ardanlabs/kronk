package classic

import "github.com/hybridgroup/yzma/pkg/llama"

// Candidate contains one token probability from a sparse draft distribution.
type Candidate struct {
	Token       llama.Token
	Probability float32
}

// Target contains the target model's decision for one verification row.
type Target struct {
	Token           llama.Token
	Probabilities   []float32
	SamplerAccepted bool
}

// VerifyInput contains one proposal and callbacks into target/output plumbing.
type VerifyInput struct {
	State         *SlotState
	Candidates    []llama.Token
	Distributions [][]Candidate
	Greedy        bool
	Target        func(index int) Target
	Accept        func(index int, token llama.Token, samplerAccepted bool) bool
	Random        func() float64
	Random32      func() float32
}

// VerifyResult describes the accepted prefix and target bonus token.
type VerifyResult struct {
	Accepted        int
	Drafted         int
	Bonus           llama.Token
	SamplerAccepted bool
	Complete        bool
}

// Verify applies classic greedy or probabilistic speculative verification.
func Verify(input VerifyInput) VerifyResult {
	result := VerifyResult{Drafted: len(input.Candidates)}
	if input.State == nil || input.Target == nil || input.Accept == nil {
		return result
	}
	defer func() {
		if result.Drafted > 0 {
			rate := float64(result.Accepted) / float64(result.Drafted)
			input.State.AcceptanceEMA = 0.9*input.State.AcceptanceEMA + 0.1*rate
		}
	}()

	useSparse := !input.Greedy && input.Distributions != nil
	for i, candidate := range input.Candidates {
		target := input.Target(i)
		if input.Greedy {
			if candidate != target.Token {
				result.Bonus = target.Token
				result.SamplerAccepted = target.SamplerAccepted
				result.Complete = true
				return result
			}
		} else {
			if !useSparse || i >= len(input.Distributions) || len(input.Distributions[i]) == 0 || len(target.Probabilities) == 0 {
				result.Bonus = chooseTarget(input, target)
				result.SamplerAccepted = target.SamplerAccepted
				result.Complete = true
				return result
			}
			qDraft := lookupProbability(input.Distributions[i], candidate)
			if qDraft <= 0 {
				useSparse = false
				result.Bonus = chooseTarget(input, target)
				result.SamplerAccepted = target.SamplerAccepted
				result.Complete = true
				return result
			}
			pTarget := target.Probabilities[candidate]
			ratio := float64(pTarget) / float64(qDraft)
			if ratio < 1 && random(input) >= ratio {
				result.Bonus = sampleAdjusted(input, target.Probabilities, input.Distributions[i])
				result.Complete = true
				return result
			}
		}

		result.Accepted++
		if !input.Accept(i, candidate, target.SamplerAccepted) {
			return result
		}
	}

	target := input.Target(len(input.Candidates))
	result.Bonus = chooseTarget(input, target)
	result.SamplerAccepted = target.SamplerAccepted
	result.Complete = true
	return result
}

// GreedyToken returns the token with the highest logit.
func GreedyToken(logits []float32) llama.Token {
	if len(logits) == 0 {
		return 0
	}
	maxIndex := 0
	maxValue := logits[0]
	for i := 1; i < len(logits); i++ {
		if logits[i] > maxValue {
			maxValue = logits[i]
			maxIndex = i
		}
	}
	return llama.Token(maxIndex)
}

func chooseTarget(input VerifyInput, target Target) llama.Token {
	if input.Greedy || len(target.Probabilities) == 0 {
		return target.Token
	}
	return sampleProbabilities(input, target.Probabilities)
}

func random(input VerifyInput) float64 {
	if input.Random == nil {
		return 0
	}
	return input.Random()
}

func lookupProbability(entries []Candidate, token llama.Token) float32 {
	for _, entry := range entries {
		if entry.Token == token {
			return entry.Probability
		}
	}
	return 0
}

func sampleProbabilities(input VerifyInput, probabilities []float32) llama.Token {
	var r float32
	if input.Random32 != nil {
		r = input.Random32()
	} else {
		r = float32(random(input))
	}
	var cumulative float32
	last := 0
	for i, probability := range probabilities {
		if probability > 0 {
			last = i
		}
		cumulative += probability
		if r < cumulative {
			return llama.Token(i)
		}
	}
	return llama.Token(last)
}

func sampleAdjusted(input VerifyInput, target []float32, draft []Candidate) llama.Token {
	scratch := input.State.AdjustedScratch[:0]
	var adjustedSum float64
	var draftTargetSum float64
	for _, candidate := range draft {
		pTarget := float64(target[candidate.Token])
		draftTargetSum += pTarget
		difference := pTarget - float64(candidate.Probability)
		if difference > 0 {
			scratch = append(scratch, Candidate{Token: candidate.Token, Probability: float32(difference)})
			adjustedSum += difference
		}
	}
	input.State.AdjustedScratch = scratch

	residualMass := max(0, 1-draftTargetSum)
	totalMass := adjustedSum + residualMass
	if totalMass <= 0 {
		return sampleProbabilities(input, target)
	}
	r := random(input) * totalMass
	if r < adjustedSum && len(scratch) > 0 {
		var cumulative float64
		for _, candidate := range scratch {
			cumulative += float64(candidate.Probability)
			if r < cumulative {
				return candidate.Token
			}
		}
		return scratch[len(scratch)-1].Token
	}

	r -= adjustedSum
	var cumulative float64
	for i, probability := range target {
		if probability == 0 || lookupProbability(draft, llama.Token(i)) > 0 {
			continue
		}
		cumulative += float64(probability)
		if r < cumulative {
			return llama.Token(i)
		}
	}
	return sampleProbabilities(input, target)
}

// FinalizePlan contains the accepted and drafted counts needed for rollback.
type FinalizePlan struct {
	Accepted int
	Drafted  int
}

// DraftKeepPosition returns the classic draft KV end after verification.
func DraftKeepPosition(start, end llama.Pos, accepted int) llama.Pos {
	return min(start+llama.Pos(accepted+1), end)
}

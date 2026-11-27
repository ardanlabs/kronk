package speculation

import "github.com/hybridgroup/yzma/pkg/llama"

// DistributionTarget contains the target model's decision for one verification row.
type DistributionTarget struct {
	Token           llama.Token
	Probabilities   []float32
	SamplerAccepted bool
}

// DistributionInput contains a proposal and callbacks into target/output plumbing.
// Probabilities are normalized, vocabulary-indexed target laws. Each sparse draft
// row is normalized and contains unique, valid vocabulary tokens with positive
// proposal probabilities; omitted tokens have zero proposal probability.
// Candidates must be sampled from these same draft laws. Random and Random32
// return independent draws in [0, 1);
// nil random callbacks use zero, with Random32 falling back to Random.
// Target is called in order through the first rejection, or through the bonus row
// at len(Candidates). Accept handles accepted candidates only; returning false
// stops verification without requesting another row or producing a bonus token.
type DistributionInput struct {
	Candidates    []llama.Token
	Distributions [][]llama.DraftCandidate
	Greedy        bool
	Target        func(index int) DistributionTarget
	Accept        func(index int, token llama.Token, samplerAccepted bool) bool
	Random        func() float64
	Random32      func() float32
}

// DistributionResult describes the accepted prefix and target replacement/bonus.
// Complete means Bonus is ready. SamplerAccepted accompanies target-selected
// tokens; it is false for a replacement drawn from the adjusted distribution.
type DistributionResult struct {
	Accepted        int
	Drafted         int
	Bonus           llama.Token
	SamplerAccepted bool
	Complete        bool
}

// VerifyDistribution applies greedy matching or speculative rejection sampling.
// A candidate is accepted with probability min(1, p(candidate)/q(candidate));
// rejection draws from normalized max(0, p-q), including outside sparse q support.
// Missing draft/target probabilities or zero candidate proposal mass fall back
// to a target draw rather than accepting an unverifiable candidate. Nil Target
// or Accept callbacks leave the result incomplete without invoking callbacks.
// Callers consume all slots' verification outputs before mutating native state.
func VerifyDistribution(input DistributionInput) DistributionResult {
	result := DistributionResult{Drafted: len(input.Candidates)}
	if input.Target == nil || input.Accept == nil {
		return result
	}

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

func chooseTarget(input DistributionInput, target DistributionTarget) llama.Token {
	if input.Greedy || len(target.Probabilities) == 0 {
		return target.Token
	}
	return sampleProbabilities(input, target.Probabilities)
}

func random(input DistributionInput) float64 {
	if input.Random == nil {
		return 0
	}
	return input.Random()
}

func lookupProbability(entries []llama.DraftCandidate, token llama.Token) float32 {
	for _, entry := range entries {
		if entry.Tok == token {
			return entry.Prob
		}
	}
	return 0
}

func sampleProbabilities(input DistributionInput, probabilities []float32) llama.Token {
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

func sampleAdjusted(input DistributionInput, target []float32, draft []llama.DraftCandidate) llama.Token {
	scratch := make([]llama.DraftCandidate, 0, len(draft))
	var adjustedSum float64
	var draftTargetSum float64
	for _, candidate := range draft {
		pTarget := float64(target[candidate.Tok])
		draftTargetSum += pTarget
		difference := pTarget - float64(candidate.Prob)
		if difference > 0 {
			scratch = append(scratch, llama.DraftCandidate{Tok: candidate.Tok, Prob: float32(difference)})
			adjustedSum += difference
		}
	}

	residualMass := max(0, 1-draftTargetSum)
	totalMass := adjustedSum + residualMass
	if totalMass <= 0 {
		return sampleProbabilities(input, target)
	}
	r := random(input) * totalMass
	if r < adjustedSum && len(scratch) > 0 {
		var cumulative float64
		for _, candidate := range scratch {
			cumulative += float64(candidate.Prob)
			if r < cumulative {
				return candidate.Tok
			}
		}
		return scratch[len(scratch)-1].Tok
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

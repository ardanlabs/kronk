package model

import (
	"math/rand"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// candidateEntry holds a token and its probability from the sampler's
// candidate list. Used for sparse speculative decoding verification
// instead of full-vocab probability distributions.
type candidateEntry struct {
	tok  llama.Token
	prob float32
}

// lookupProb finds the probability for a given token in a sparse candidate
// list. Returns 0 if the token is not present.
func lookupProb(entries []candidateEntry, tok llama.Token) float32 {
	for _, e := range entries {
		if e.tok == tok {
			return e.prob
		}
	}
	return 0
}

// sampleAdjustedSparseFromFull samples from the adjusted distribution
// max(0, p_target - q_draft) when the target distribution is full-vocab
// and the draft distribution is sparse. This is used when the target context
// doesn't have backend samplers (no sparse target candidates available).
//
// The algorithm:
//  1. For each draft candidate, compute max(0, p_target[tok] - q_draft)
//  2. Compute residual mass: probability mass in targetProbs for tokens NOT
//     in the draft set (these have q=0, so adjusted = p_target directly)
//  3. Sample: with probability proportional to the adjusted draft entries,
//     pick from those. Otherwise, sample from the full target distribution
//     (which correctly favors tokens the draft model missed).
func sampleAdjustedSparseFromFull(targetProbs []float32, draftEntries, scratch []candidateEntry) llama.Token {
	scratch = scratch[:0]
	var adjustedSum float64
	var draftTargetSum float64

	for _, de := range draftEntries {
		pTarget := float64(targetProbs[de.tok])
		draftTargetSum += pTarget
		diff := pTarget - float64(de.prob)
		if diff > 0 {
			scratch = append(scratch, candidateEntry{tok: de.tok, prob: float32(diff)})
			adjustedSum += diff
		}
	}

	// Residual mass: target probability mass for tokens not in draft set.
	residualMass := 1.0 - draftTargetSum
	if residualMass < 0 {
		residualMass = 0
	}

	totalMass := adjustedSum + residualMass

	if !(totalMass > 0) {
		return sampleFromProbs(targetProbs)
	}

	// Sample: pick adjusted draft candidate or fall back to full target.
	r := rand.Float64() * totalMass

	if r < adjustedSum && len(scratch) > 0 {
		// Sample from adjusted draft entries.
		var cumulative float64
		for _, e := range scratch {
			cumulative += float64(e.prob)
			if r < cumulative {
				return e.tok
			}
		}
		return scratch[len(scratch)-1].tok
	}

	// Sample from target distribution restricted to tokens NOT in draft set.
	// These tokens have q_draft=0, so their adjusted probability equals p_target.
	r -= adjustedSum // shift r into [0, residualMass) range
	var cumulative float64
	for i, p := range targetProbs {
		if p == 0 {
			continue
		}
		// Skip tokens in the draft set.
		isDraft := false
		for _, de := range draftEntries {
			if de.tok == llama.Token(i) {
				isDraft = true
				break
			}
		}
		if isDraft {
			continue
		}
		cumulative += float64(p)
		if r < cumulative {
			return llama.Token(i)
		}
	}

	// Fallback: degenerate case, sample from full target distribution.
	return sampleFromProbs(targetProbs)
}

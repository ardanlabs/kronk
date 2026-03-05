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

// sampleAdjustedSparse samples from the adjusted distribution max(0, p - q)
// over sparse candidate lists. This is the rejection branch of speculative
// sampling using sparse (top-K) distributions instead of full-vocab arrays.
//
// pEntries are the target model's candidates, qEntries are the draft model's.
// scratch is a reusable buffer to avoid allocations.
func sampleAdjustedSparse(pEntries, qEntries, scratch []candidateEntry) llama.Token {
	scratch = scratch[:0]
	var sum float64

	for _, pe := range pEntries {
		q := lookupProb(qEntries, pe.tok)
		diff := float64(pe.prob) - float64(q)
		if diff > 0 {
			scratch = append(scratch, candidateEntry{tok: pe.tok, prob: float32(diff)})
			sum += diff
		}
	}

	// If the adjusted distribution is empty, fall back to sampling
	// from the target distribution directly.
	if sum <= 0 || len(scratch) == 0 {
		return sampleFromCandidates(pEntries)
	}

	// Normalize and sample.
	invSum := float32(1.0 / sum)
	for i := range scratch {
		scratch[i].prob *= invSum
	}

	return sampleFromCandidates(scratch)
}

// sampleFromCandidates samples a token from a sparse candidate distribution
// using inverse transform sampling.
func sampleFromCandidates(entries []candidateEntry) llama.Token {
	r := rand.Float32()
	var cumulative float32

	for _, e := range entries {
		cumulative += e.prob
		if r < cumulative {
			return e.tok
		}
	}

	// Fallback: return last candidate.
	if len(entries) > 0 {
		return entries[len(entries)-1].tok
	}
	return 0
}

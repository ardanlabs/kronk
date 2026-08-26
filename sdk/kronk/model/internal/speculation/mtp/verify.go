package mtp

import "github.com/hybridgroup/yzma/pkg/llama"

// VerifyInput contains callbacks into target sampling and output processing.
type VerifyInput struct {
	Candidates []llama.Token
	Sample     func(index int) llama.Token
	Accept     func(index int, token llama.Token) bool
}

// VerifyResult contains the accepted prefix and target bonus token.
type VerifyResult struct {
	Accepted int
	Drafted  int
	Bonus    llama.Token
	Complete bool
}

// Verify performs MTP's greedy target verification. MTP has no draft
// distribution, so each proposal is accepted only when it matches the target
// request sampler at the corresponding output row.
func Verify(input VerifyInput) VerifyResult {
	result := VerifyResult{Drafted: len(input.Candidates), Complete: true}
	for i, candidate := range input.Candidates {
		target := input.Sample(i)
		if candidate != target {
			result.Bonus = target
			return result
		}
		result.Accepted++
		if !input.Accept(i, candidate) {
			result.Complete = false
			return result
		}
	}

	result.Bonus = input.Sample(len(input.Candidates))
	return result
}

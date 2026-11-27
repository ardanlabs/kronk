package speculation

import "github.com/hybridgroup/yzma/pkg/llama"

// MatchInput supplies authoritative target samples and accepted-token handling.
// Matching proposals is valid for greedy DSpark and MTP's existing sample-and-
// match policy, not probabilistic rejection sampling against a draft law.
type MatchInput struct {
	Candidates []llama.Token
	Sample     func(index int) llama.Token
	Accept     func(index int, token llama.Token) bool
}

// MatchResult identifies the accepted prefix and target replacement or bonus.
type MatchResult struct {
	Accepted int
	Drafted  int
	Bonus    llama.Token
	Complete bool
}

// VerifyMatches accepts matching proposals until rejection or output completion.
// A completed prefix always gets its next token from the target, including after
// full acceptance. Callers consume every slot's outputs before native rollback.
func VerifyMatches(input MatchInput) MatchResult {
	result := MatchResult{Drafted: len(input.Candidates), Complete: true}
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

// FinalizePlan describes a verified prefix independently of proposal policy.
type FinalizePlan struct {
	Accepted int
	Drafted  int
}

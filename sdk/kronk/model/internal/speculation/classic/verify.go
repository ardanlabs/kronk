package classic

import (
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Target contains the target model's decision for one verification row.
type Target = speculation.DistributionTarget

// VerifyInput contains one proposal and callbacks into target/output plumbing.
type VerifyInput struct {
	State         *SlotState
	Candidates    []llama.Token
	Distributions [][]llama.DraftCandidate
	Greedy        bool
	Target        func(index int) Target
	Accept        func(index int, token llama.Token, samplerAccepted bool) bool
	Random        func() float64
	Random32      func() float32
}

// VerifyResult describes the accepted prefix and target bonus token.
type VerifyResult = speculation.DistributionResult

// Verify applies classic greedy or probabilistic speculative verification.
func Verify(input VerifyInput) VerifyResult {
	result := VerifyResult{Drafted: len(input.Candidates)}
	if input.State == nil || input.Target == nil || input.Accept == nil {
		return result
	}
	result = speculation.VerifyDistribution(speculation.DistributionInput{
		Candidates:    input.Candidates,
		Distributions: input.Distributions,
		Greedy:        input.Greedy,
		Target:        input.Target,
		Accept:        input.Accept,
		Random:        input.Random,
		Random32:      input.Random32,
	})
	if result.Drafted > 0 {
		rate := float64(result.Accepted) / float64(result.Drafted)
		input.State.AcceptanceEMA = 0.9*input.State.AcceptanceEMA + 0.1*rate
	}
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

// FinalizePlan contains the accepted and drafted counts needed for rollback.
type FinalizePlan = speculation.FinalizePlan

// DraftKeepPosition returns the classic draft KV end after verification.
func DraftKeepPosition(start, end llama.Pos, accepted int) llama.Pos {
	return min(start+llama.Pos(accepted+1), end)
}

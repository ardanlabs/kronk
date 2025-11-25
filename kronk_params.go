package kronk

import "github.com/hybridgroup/yzma/pkg/llama"

const (
	defTopK      = 40
	defTopP      = 0.9
	defTemp      = 0.7
	defMaxTokens = 512
)

// Params represents the different options when using a model. The defaults are
// used when these values are set to 0.
//
// Temperature controls the randomness of the output. It rescales the probability
// distribution of possible next tokens.
// When set to 0, the default value is 0.7.
//
// TopK limits the pool of possible next tokens to the K number of most probable
// tokens. If a model predicts 10,000 possible next tokens, setting Top-K to 50
// means only the 50 tokens with the highest probabilities are considered for
// selection (after temperature scaling). The rest are ignored.
// When set to 0, the default value is 40.
//
// TopP, also known as nucleus sampling, works differently than Top-K by
// selecting a dynamic pool of tokens whose cumulative probability exceeds a
// threshold P. Instead of a fixed number of tokens (K), it selects the minimum
// number of most probable tokens required to reach the cumulative probability P.
// When set to 0, the default value is 0.9.
//
// These parameters (TopK, TopP, Temperature) are typically used together. The
// sampling process usually applies temperature first, then filters the token
// list using Top-K, and finally filters it again using Top-P before selecting
// the next token randomly from the remaining pool based on their (now adjusted)
// probabilities.
//
// MaxTokens defines the maximum number of output tokens to generate for a
// single response.
// When set to 0, the default value is 512.
type Params struct {
	Temperature float32
	TopK        int32
	TopP        float32
	MaxTokens   int
}

func adjustParams(p Params) Params {
	if p.Temperature <= 0 {
		p.Temperature = defTemp
	}

	if p.TopK <= 0 {
		p.TopK = defTopK
	}

	if p.TopP <= 0 {
		p.TopP = defTopP
	}

	if p.MaxTokens <= 0 {
		p.MaxTokens = defMaxTokens
	}

	return p
}

func toSampler(p Params) llama.Sampler {
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())

	llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(p.Temperature, 0, 1.0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(p.TopK))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(p.TopP, 0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(llama.DefaultSeed))

	return sampler
}

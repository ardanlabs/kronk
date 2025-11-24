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
// TopK      default: 40
// TopP      default: 0.9
// Temp      default: 0.7
// MaxTokens default: 512
type Params struct {
	TopK      int32
	TopP      float32
	Temp      float32
	MaxTokens int
}

func adjustParams(p Params) Params {
	if p.TopK <= 0 {
		p.TopK = defTopK
	}

	if p.TopP <= 0 {
		p.TopP = defTopP
	}

	if p.Temp <= 0 {
		p.Temp = defTemp
	}

	if p.MaxTokens <= 0 {
		p.MaxTokens = defMaxTokens
	}

	return p
}

func toSampler(p Params) llama.Sampler {
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(p.TopK))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(p.TopP, 0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(p.Temp, 0, 1.0))
	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(llama.DefaultSeed))

	return sampler
}

package kronk

import "github.com/hybridgroup/yzma/pkg/llama"

// Params represents the different sample options when using a model.
type Params struct {
	TopK int32
	TopP float32
	Temp float32
}

func toSampler(p Params) llama.Sampler {
	sampler := llama.SamplerChainInit(llama.SamplerChainDefaultParams())

	if p.TopK > 0 {
		llama.SamplerChainAdd(sampler, llama.SamplerInitTopK(p.TopK))
	}
	if p.TopP > 0 {
		llama.SamplerChainAdd(sampler, llama.SamplerInitTopP(p.TopP, 0))
	}
	if p.Temp > 0 {
		llama.SamplerChainAdd(sampler, llama.SamplerInitTempExt(p.Temp, 0, 1.0))
	}

	llama.SamplerChainAdd(sampler, llama.SamplerInitDist(llama.DefaultSeed))

	return sampler
}

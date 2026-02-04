package model

import (
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/jupiterrider/ffi"
)

// GrammarSampler holds a separate grammar sampler that is NOT part of the
// main sampler chain. This matches llama.cpp's approach where grammar is
// managed separately and applied during sampling but with special handling
// for accept.
type GrammarSampler struct {
	sampler llama.Sampler
	nVocab  int

	// Pre-allocated buffer to avoid allocations during sampling.
	tokenData []tokenData
}

// tokenData mirrors llama_token_data from llama.cpp.
// Layout: int32_t id (4) + float logit (4) + float p (4) = 12 bytes
type tokenData struct {
	ID    int32   // llama_token is int32_t
	Logit float32 // log-odds of the token
	P     float32 // probability of the token
}

// tokenDataArray mirrors llama_token_data_array from llama.cpp.
// Layout: pointer (8) + size_t (8) + int64_t (8) + bool (1) + padding (7) = 32 bytes
type tokenDataArray struct {
	Data     unsafe.Pointer // *tokenData
	Size     uint64         // size_t
	Selected int64          // -1 if no token has been selected
	Sorted   uint8          // bool in C, use uint8 to match
	_        [7]byte        // padding to match C struct alignment
}

// SamplerApplyFunc holds the FFI function for llama_sampler_apply.
// This is set by the kronk package during initialization.
var SamplerApplyFunc ffi.Fun

// NewGrammarSampler creates a grammar sampler that will be managed separately
// from the main sampler chain.
func NewGrammarSampler(vocab llama.Vocab, grammar string) *GrammarSampler {
	if grammar == "" {
		return nil
	}

	sampler := llama.SamplerInitGrammar(vocab, grammar, "root")
	if sampler == 0 {
		return nil
	}

	nVocab := int(llama.VocabNTokens(vocab))

	// Pre-allocate token data buffer.
	tokenData := make([]tokenData, nVocab)

	return &GrammarSampler{
		sampler:   sampler,
		nVocab:    nVocab,
		tokenData: tokenData,
	}
}

// SampleWithGrammar samples a token using the main sampler chain with grammar
// constraints applied first. This is the key integration point that:
// 1. Gets logits from the context
// 2. Builds a token_data_array
// 3. Applies grammar constraints (sets invalid tokens to -inf logits)
// 4. Copies modified logits back to context
// 5. Uses normal SamplerSample which reads from context
//
// The caller must still call Accept() on both the grammar sampler and the
// main sampler after selecting the token.
func (gs *GrammarSampler) SampleWithGrammar(ctx llama.Context, chainSampler llama.Sampler, idx int32) llama.Token {
	if gs == nil || gs.sampler == 0 {
		return llama.SamplerSample(chainSampler, ctx, idx)
	}

	// Get logits from context.
	logits, err := llama.GetLogitsIth(ctx, idx, gs.nVocab)
	if err != nil || logits == nil {
		return llama.SamplerSample(chainSampler, ctx, idx)
	}

	// Build token_data_array from logits.
	for i := range gs.nVocab {
		gs.tokenData[i] = tokenData{
			ID:    int32(i),
			Logit: logits[i],
			P:     0, // Will be computed by samplers
		}
	}

	curP := tokenDataArray{
		Data:     unsafe.Pointer(&gs.tokenData[0]),
		Size:     uint64(gs.nVocab),
		Selected: -1,
		Sorted:   0, // false
	}

	// Apply grammar constraints - this sets invalid tokens to -inf logits.
	// For C functions taking pointers, we need to pass the address of the pointer value.
	smpl := gs.sampler
	curPPtr := unsafe.Pointer(&curP)
	SamplerApplyFunc.Call(nil, unsafe.Pointer(&smpl), unsafe.Pointer(&curPPtr))

	// Copy modified logits back to context's logits array.
	// This allows the normal SamplerSample to work with grammar-constrained logits.
	for i := range gs.nVocab {
		logits[i] = gs.tokenData[i].Logit
	}

	// Now use normal sampling - it will read the modified logits from context.
	return llama.SamplerSample(chainSampler, ctx, idx)
}

// Accept advances the grammar state machine after a token is selected.
func (gs *GrammarSampler) Accept(token llama.Token) {
	if gs == nil || gs.sampler == 0 {
		return
	}

	llama.SamplerAccept(gs.sampler, token)
}

// Free releases the grammar sampler resources.
func (gs *GrammarSampler) Free() {
	if gs == nil || gs.sampler == 0 {
		return
	}

	llama.SamplerFree(gs.sampler)
	gs.sampler = 0
}

// Reset resets the grammar sampler state.
func (gs *GrammarSampler) Reset() {
	if gs == nil || gs.sampler == 0 {
		return
	}

	llama.SamplerReset(gs.sampler)
}

package llama

import (
	"math"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

type SamplerType int32

const (
	SamplerTypeNone        SamplerType = iota
	SamplerTypeDry                     = 1
	SamplerTypeTopK                    = 2
	SamplerTypeTopP                    = 3
	SamplerTypeMinP                    = 4
	SamplerTypeTypicalP                = 6
	SamplerTypeTemperature             = 7
	SamplerTypeXTC                     = 8
	SamplerTypeInfill                  = 9
	SamplerTypePenalties               = 10
	SamplerTypeTopNSigma               = 11
	SamplerTypeAdaptiveP               = 12
	SamplerTypeLogitBias               = 13
)

var (
	// ffiSamplerChainParams represents the C struct llama_sampler_chain_params
	ffiSamplerChainParams = ffi.NewType(&ffi.TypePointer)
)

var (
	// LLAMA_API struct llama_sampler_chain_params  llama_sampler_chain_default_params(void);
	samplerChainDefaultParamsFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_chain_init(struct llama_sampler_chain_params params);
	samplerChainInitFunc ffi.Fun

	// LLAMA_API const char * llama_sampler_name(const struct llama_sampler * smpl);
	samplerNameFunc ffi.Fun

	// LLAMA_API void llama_sampler_chain_add(struct llama_sampler * chain, struct llama_sampler * smpl);
	samplerChainAddFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_chain_get(struct llama_sampler * chain, int32_t i);
	samplerChainGetFunc ffi.Fun

	// LLAMA_API int llama_sampler_chain_n(const struct llama_sampler * chain);
	samplerChainNFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_chain_remove(struct llama_sampler * chain, int32_t i);
	samplerChainRemoveFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_greedy(void);
	samplerInitGreedyFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_dist  (uint32_t seed);
	samplerInitDistFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_logit_bias(
	//                  int32_t   n_vocab,
	//                  int32_t   n_logit_bias,
	//   				const llama_logit_bias * logit_bias);
	samplerInitLogitBiasFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_penalties(
	// 						int32_t   penalty_last_n,   // last n tokens to penalize (0 = disable penalty, -1 = context size)
	// 						float   penalty_repeat,   // 1.0 = disabled
	// 						float   penalty_freq,     // 0.0 = disabled
	// 						float   penalty_present); // 0.0 = disabled
	samplerInitPenaltiesFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_dry(
	// 	const struct llama_vocab *  vocab,
	// 						int32_t    n_ctx_train,
	// 						float    dry_multiplier,
	// 						float    dry_base,
	// 						int32_t    dry_allowed_length,
	// 						int32_t    dry_penalty_last_n,
	// 					const char ** seq_breakers,
	// 						size_t    num_breakers);
	samplerInitDryFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_top_n_sigma(float   n);
	samplerInitTopNSigmaFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_top_k      (int32_t k);
	samplerInitTopKFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_typical    (float   p, size_t min_keep);
	samplerInitTypicalFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_top_p      (float   p, size_t min_keep);
	samplerInitTopPFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_min_p      (float   p, size_t min_keep);
	samplerInitMinPFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_xtc        (float   p, float   t,     size_t min_keep, uint32_t seed);
	samplerInitXTCFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_temp_ext   (float   t, float   delta, float exponent);
	samplerInitTempExtFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_grammar(
	// 					const struct llama_vocab * vocab,
	//               	const char * grammar_str,
	//               	const char * grammar_root);
	samplerInitGrammarFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_grammar_lazy_patterns(
	//   const struct llama_vocab * vocab,
	//   const char * grammar_str,
	//   const char * grammar_root,
	//   const char ** trigger_patterns,
	//   size_t num_trigger_patterns,
	//   const llama_token * trigger_tokens,
	//   size_t num_trigger_tokens);
	samplerInitGrammarLazyPatternsFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_adaptive_p(
	//                            float   target,
	//                            float   decay,
	//                         uint32_t   seed);
	samplerInitAdaptivePFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_infill(const struct llama_vocab * vocab);
	samplerInitInfillFunc ffi.Fun

	// LLAMA_API llama_token llama_sampler_sample(struct llama_sampler * smpl, struct llama_context * ctx, int32_t idx);
	samplerSampleFunc ffi.Fun

	// LLAMA_API void  llama_sampler_accept(struct llama_sampler * smpl, llama_token token);
	samplerAcceptFunc ffi.Fun

	// LLAMA_API void  llama_sampler_apply (struct llama_sampler * smpl, llama_token_data_array * cur_p);
	samplerApplyFunc ffi.Fun

	// LLAMA_API void llama_sampler_free  (struct llama_sampler * smpl);
	samplerFreeFunc ffi.Fun

	// LLAMA_API void llama_sampler_reset (struct llama_sampler * smpl);
	samplerResetFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_clone(const struct llama_sampler * smpl);
	samplerCloneFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_mirostat(
	//                          int32_t   n_vocab,
	//                         uint32_t   seed,
	//                            float   tau,
	//                            float   eta,
	//                          int32_t   m);
	samplerInitMirostatFunc ffi.Fun

	// LLAMA_API struct llama_sampler * llama_sampler_init_mirostat_v2(
	//                         uint32_t   seed,
	//                            float   tau,
	//                            float   eta);
	samplerInitMirostatV2Func ffi.Fun

	// LLAMA_API uint32_t llama_sampler_get_seed(const struct llama_sampler * smpl);
	samplerGetSeedFunc ffi.Fun
)

func loadSamplingFuncs(lib ffi.Lib) error {
	var err error

	if samplerChainDefaultParamsFunc, err = lib.Prep("llama_sampler_chain_default_params", &ffiSamplerChainParams); err != nil {
		return loadError("llama_sampler_chain_default_params", err)
	}

	if samplerChainInitFunc, err = lib.Prep("llama_sampler_chain_init", &ffi.TypePointer, &ffiSamplerChainParams); err != nil {
		return loadError("llama_sampler_chain_init", err)
	}

	if samplerNameFunc, err = lib.Prep("llama_sampler_name", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_name", err)
	}

	if samplerChainAddFunc, err = lib.Prep("llama_sampler_chain_add", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_chain_add", err)
	}

	if samplerChainGetFunc, err = lib.Prep("llama_sampler_chain_get", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_sampler_chain_get", err)
	}

	if samplerChainNFunc, err = lib.Prep("llama_sampler_chain_n", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_chain_n", err)
	}

	if samplerChainRemoveFunc, err = lib.Prep("llama_sampler_chain_remove", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_sampler_chain_remove", err)
	}

	if samplerInitGreedyFunc, err = lib.Prep("llama_sampler_init_greedy", &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_init_greedy", err)
	}

	if samplerInitDistFunc, err = lib.Prep("llama_sampler_init_dist", &ffi.TypePointer, &ffi.TypeUint32); err != nil {
		return loadError("llama_sampler_init_dist", err)
	}

	if samplerInitLogitBiasFunc, err = lib.Prep("llama_sampler_init_logit_bias", &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_init_logit_bias", err)
	}

	if samplerInitPenaltiesFunc, err = lib.Prep("llama_sampler_init_penalties", &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeFloat, &ffi.TypeFloat, &ffi.TypeFloat); err != nil {
		return loadError("llama_sampler_init_penalties", err)
	}

	if samplerInitDryFunc, err = lib.Prep("llama_sampler_init_dry", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeFloat, &ffi.TypeFloat,
		&ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypePointer, &ffiTypeSize); err != nil {

		return loadError("llama_sampler_init_dry", err)
	}

	if samplerInitTopNSigmaFunc, err = lib.Prep("llama_sampler_init_top_n_sigma", &ffi.TypePointer, &ffi.TypeFloat); err != nil {
		return loadError("llama_sampler_init_top_n_sigma", err)
	}

	if samplerInitTopKFunc, err = lib.Prep("llama_sampler_init_top_k", &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_sampler_init_top_k", err)
	}

	if samplerInitTypicalFunc, err = lib.Prep("llama_sampler_init_typical", &ffi.TypePointer, &ffi.TypeFloat, &ffiTypeSize); err != nil {
		return loadError("llama_sampler_init_typical", err)
	}

	if samplerInitTopPFunc, err = lib.Prep("llama_sampler_init_top_p", &ffi.TypePointer, &ffi.TypeFloat, &ffiTypeSize); err != nil {
		return loadError("llama_sampler_init_top_p", err)
	}

	if samplerInitMinPFunc, err = lib.Prep("llama_sampler_init_min_p", &ffi.TypePointer, &ffi.TypeFloat, &ffiTypeSize); err != nil {
		return loadError("llama_sampler_init_min_p", err)
	}

	if samplerInitXTCFunc, err = lib.Prep("llama_sampler_init_xtc", &ffi.TypePointer, &ffi.TypeFloat, &ffi.TypeFloat, &ffiTypeSize, &ffi.TypeUint32); err != nil {
		return loadError("llama_sampler_init_xtc", err)
	}

	if samplerInitTempExtFunc, err = lib.Prep("llama_sampler_init_temp_ext", &ffi.TypePointer, &ffi.TypeFloat, &ffi.TypeFloat, &ffi.TypeFloat); err != nil {
		return loadError("llama_sampler_init_temp_ext", err)
	}

	if samplerInitGrammarFunc, err = lib.Prep("llama_sampler_init_grammar", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_init_grammar", err)
	}

	if samplerInitGrammarLazyPatternsFunc, err = lib.Prep("llama_sampler_init_grammar_lazy_patterns",
		&ffi.TypePointer, // return: struct llama_sampler *
		&ffi.TypePointer, // vocab
		&ffi.TypePointer, // grammar_str
		&ffi.TypePointer, // grammar_root
		&ffi.TypePointer, // trigger_patterns
		&ffiTypeSize,     // num_trigger_patterns
		&ffi.TypePointer, // trigger_tokens
		&ffiTypeSize,     // num_trigger_tokens
	); err != nil {
		return loadError("llama_sampler_init_grammar_lazy_patterns", err)
	}

	if samplerInitAdaptivePFunc, err = lib.Prep("llama_sampler_init_adaptive_p", &ffi.TypePointer, &ffi.TypeFloat, &ffi.TypeFloat, &ffi.TypeUint32); err != nil {
		return loadError("llama_sampler_init_adaptive_p", err)
	}

	if samplerInitInfillFunc, err = lib.Prep("llama_sampler_init_infill", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_init_infill", err)
	}

	if samplerSampleFunc, err = lib.Prep("llama_sampler_sample", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_sampler_sample", err)
	}

	if samplerAcceptFunc, err = lib.Prep("llama_sampler_accept", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_sampler_accept", err)
	}

	if samplerApplyFunc, err = lib.Prep("llama_sampler_apply", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_apply", err)
	}

	if samplerFreeFunc, err = lib.Prep("llama_sampler_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_free", err)
	}

	if samplerResetFunc, err = lib.Prep("llama_sampler_reset", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_reset", err)
	}

	if samplerCloneFunc, err = lib.Prep("llama_sampler_clone", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_clone", err)
	}

	if samplerInitMirostatFunc, err = lib.Prep("llama_sampler_init_mirostat", &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeUint32, &ffi.TypeFloat, &ffi.TypeFloat, &ffi.TypeSint32); err != nil {
		return loadError("llama_sampler_init_mirostat", err)
	}

	if samplerInitMirostatV2Func, err = lib.Prep("llama_sampler_init_mirostat_v2", &ffi.TypePointer, &ffi.TypeUint32, &ffi.TypeFloat, &ffi.TypeFloat); err != nil {
		return loadError("llama_sampler_init_mirostat_v2", err)
	}

	if samplerGetSeedFunc, err = lib.Prep("llama_sampler_get_seed", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("llama_sampler_get_seed", err)
	}

	return nil
}

// SamplerChainDefaultParams returns the default parameters to create a new sampling chain.
func SamplerChainDefaultParams() SamplerChainParams {
	var p SamplerChainParams
	samplerChainDefaultParamsFunc.Call(unsafe.Pointer(&p))

	return p
}

// SamplerChainInit initializes a new sampling chain.
func SamplerChainInit(params SamplerChainParams) Sampler {
	var p Sampler
	samplerChainInitFunc.Call(unsafe.Pointer(&p), unsafe.Pointer(&params))

	return p
}

// SamplerName returns the name of the sampler as a string.
func SamplerName(smpl Sampler) string {
	if smpl == 0 {
		return ""
	}
	var ptr *byte
	samplerNameFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&smpl))
	if ptr == nil {
		return ""
	}

	return utils.BytePtrToString(ptr)
}

// SamplerChainAdd adds a sampler to a sampling chain.
func SamplerChainAdd(chain Sampler, smpl Sampler) {
	if chain == 0 || smpl == 0 {
		return
	}
	samplerChainAddFunc.Call(nil, unsafe.Pointer(&chain), unsafe.Pointer(&smpl))
}

// SamplerChainGet returns the i-th sampler from a sampler chain, or the chain itself if i == -1.
// Returns 0 if the chain is not valid or index is out of bounds.
func SamplerChainGet(chain Sampler, i int32) Sampler {
	if chain == 0 {
		return 0
	}
	var s Sampler
	samplerChainGetFunc.Call(unsafe.Pointer(&s), unsafe.Pointer(&chain), &i)
	return s
}

// SamplerChainN returns the total number of samplers in the chain.
func SamplerChainN(chain Sampler) int {
	if chain == 0 {
		return 0
	}
	var n ffi.Arg
	samplerChainNFunc.Call(unsafe.Pointer(&n), unsafe.Pointer(&chain))
	return int(n)
}

// SamplerChainRemove removes the i-th sampler from the chain and returns it.
// After removal, the chain will no longer own the sampler, and it will not be freed when the chain is freed.
func SamplerChainRemove(chain Sampler, i int32) Sampler {
	if chain == 0 {
		return 0
	}
	var removed Sampler
	samplerChainRemoveFunc.Call(unsafe.Pointer(&removed), unsafe.Pointer(&chain), &i)
	return removed
}

// SamplerInitGreedy initializes a new greedy sampler.
func SamplerInitGreedy() Sampler {
	var p Sampler
	samplerInitGreedyFunc.Call(unsafe.Pointer(&p))

	return p
}

// SamplerInitDist initializes a new distribution sampler with the specified seed.
func SamplerInitDist(seed uint32) Sampler {
	var p Sampler
	samplerInitDistFunc.Call(unsafe.Pointer(&p), &seed)

	return p
}

// SamplerInitLogitBias initializes a new logit bias sampler.
func SamplerInitLogitBias(nVocab int32, nLogitBias int32, logitBias *LogitBias) Sampler {
	var p Sampler
	samplerInitLogitBiasFunc.Call(unsafe.Pointer(&p), &nVocab, &nLogitBias, unsafe.Pointer(&logitBias))

	return p
}

// SamplerInitPenalties initializes a new penalties sampler.
func SamplerInitPenalties(lastN int32, repeat float32, freq float32, present float32) Sampler {
	var p Sampler
	samplerInitPenaltiesFunc.Call(unsafe.Pointer(&p), &lastN, &repeat, &freq, &present)

	return p
}

// SamplerInitDry initializes a new DRY sampler.
func SamplerInitDry(vocab Vocab, nCtxTrain int32, multiplier float32, base float32, allowedLength int32, penaltyLast int32,
	seqBreakers []string) Sampler {
	var sp unsafe.Pointer
	numBreakers := uint64(len(seqBreakers))
	if numBreakers > 0 {
		seq := make([]*byte, 0)
		for _, s := range seqBreakers {
			ptr, err := utils.BytePtrFromString(s)
			if err != nil {
				return Sampler(0)
			}
			seq = append(seq, ptr)
		}
		sp = unsafe.Pointer(&seq[0])
	}

	var p Sampler
	samplerInitDryFunc.Call(unsafe.Pointer(&p), unsafe.Pointer(&vocab), &nCtxTrain, &multiplier, &base, &allowedLength, &penaltyLast,
		&sp, &numBreakers)
	return p
}

// SamplerInitTopNSigma initializes a new Top-N Sigma sampler.
func SamplerInitTopNSigma(n float32) Sampler {
	var p Sampler
	samplerInitTopNSigmaFunc.Call(unsafe.Pointer(&p), &n)

	return p
}

// SamplerInitTopK initializes a new Top-K sampler.
func SamplerInitTopK(k int32) Sampler {
	var p Sampler
	samplerInitTopKFunc.Call(unsafe.Pointer(&p), &k)

	return p
}

// SamplerInitTypical initializes a new Typical-P sampler.
func SamplerInitTypical(p float32, keep uint32) Sampler {
	var s Sampler
	samplerInitTypicalFunc.Call(unsafe.Pointer(&s), &p, &keep)

	return s
}

// SamplerInitTopP initializes a new Top-P sampler.
func SamplerInitTopP(p float32, keep uint32) Sampler {
	var s Sampler
	samplerInitTopPFunc.Call(unsafe.Pointer(&s), &p, &keep)

	return s
}

// SamplerInitMinP initializes a new Min-P sampler.
func SamplerInitMinP(p float32, keep uint32) Sampler {
	var s Sampler
	samplerInitMinPFunc.Call(unsafe.Pointer(&s), &p, &keep)

	return s
}

// SamplerInitXTC initializes a new XTC sampler.
func SamplerInitXTC(p float32, t float32, minKeep uint32, seed uint32) Sampler {
	var s Sampler
	samplerInitXTCFunc.Call(unsafe.Pointer(&s), &p, &t, &minKeep, &seed)

	return s
}

// SamplerInitTempExt initializes a new Temperature Extended sampler.
func SamplerInitTempExt(t float32, delta float32, exponent float32) Sampler {
	var s Sampler
	samplerInitTempExtFunc.Call(unsafe.Pointer(&s), &t, &delta, &exponent)

	return s
}

// SamplerInitGrammar initializes a new Grammar sampler.
func SamplerInitGrammar(vocab Vocab, grammar, root string) Sampler {
	var s Sampler
	if vocab == 0 {
		return s
	}
	grmr, _ := utils.BytePtrFromString(grammar)
	r, _ := utils.BytePtrFromString(root)

	samplerInitGrammarFunc.Call(unsafe.Pointer(&s), unsafe.Pointer(&vocab), unsafe.Pointer(&grmr), unsafe.Pointer(&r))

	return s
}

// SamplerInitGrammarLazyPatterns initializes a lazy grammar sampler with trigger patterns and tokens.
func SamplerInitGrammarLazyPatterns(
	vocab Vocab,
	grammar, root string,
	triggerPatterns []string,
	triggerTokens []Token,
) Sampler {
	var s Sampler
	if vocab == 0 {
		return s
	}
	grmr, _ := utils.BytePtrFromString(grammar)
	r, _ := utils.BytePtrFromString(root)

	var tp unsafe.Pointer
	numPatterns := uint64(len(triggerPatterns))
	if numPatterns > 0 {
		ptrs := make([]*byte, 0, numPatterns)
		for _, pat := range triggerPatterns {
			ptr, err := utils.BytePtrFromString(pat)
			if err != nil {
				return s
			}
			ptrs = append(ptrs, ptr)
		}
		tp = unsafe.Pointer(&ptrs[0])
	}

	var tt unsafe.Pointer
	numTokens := uint64(len(triggerTokens))
	if numTokens > 0 {
		tt = unsafe.Pointer(&triggerTokens[0])
	}

	samplerInitGrammarLazyPatternsFunc.Call(
		unsafe.Pointer(&s),
		unsafe.Pointer(&vocab),
		unsafe.Pointer(&grmr),
		unsafe.Pointer(&r),
		&tp,
		&numPatterns,
		&tt,
		&numTokens,
	)
	return s
}

// SamplerInitAdaptiveP initializes a new Adaptive-P sampler.
func SamplerInitAdaptiveP(target float32, decay float32, seed uint32) Sampler {
	var s Sampler
	samplerInitAdaptivePFunc.Call(unsafe.Pointer(&s), &target, &decay, &seed)

	return s
}

// SamplerInitInfill initializes a new infill sampler for fill-in-the-middle infilling.
// Supposed to be used after top_k + top_p sampling.
func SamplerInitInfill(vocab Vocab) Sampler {
	var s Sampler
	samplerInitInfillFunc.Call(unsafe.Pointer(&s), unsafe.Pointer(&vocab))
	return s
}

// SamplerSample samples a token from the sampler given the context and index.
func SamplerSample(smpl Sampler, ctx Context, idx int32) Token {
	if smpl == 0 || ctx == 0 {
		return TokenNull
	}

	var result ffi.Arg
	samplerSampleFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&smpl), unsafe.Pointer(&ctx), &idx)

	return Token(result)
}

// SamplerAccept informs the sampler of the accepted token.
func SamplerAccept(smpl Sampler, token Token) {
	if smpl == 0 {
		return
	}
	samplerAcceptFunc.Call(nil, unsafe.Pointer(&smpl), unsafe.Pointer(&token))
}

// SamplerApply applies the sampler to the current token data array.
func SamplerApply(smpl Sampler, curP *TokenDataArray) {
	if smpl == 0 || curP == nil {
		return
	}
	samplerApplyFunc.Call(nil, unsafe.Pointer(&smpl), unsafe.Pointer(&curP))
}

// SamplerFree frees the sampler.
func SamplerFree(smpl Sampler) {
	if smpl == 0 {
		return
	}
	samplerFreeFunc.Call(nil, unsafe.Pointer(&smpl))
}

// SamplerReset resets the sampler state.
func SamplerReset(smpl Sampler) {
	if smpl == 0 {
		return
	}
	samplerResetFunc.Call(nil, unsafe.Pointer(&smpl))
}

// SamplerClone creates a clone of the given sampler.
func SamplerClone(smpl Sampler) Sampler {
	if smpl == 0 {
		return 0
	}
	var clone Sampler
	samplerCloneFunc.Call(unsafe.Pointer(&clone), unsafe.Pointer(&smpl))
	return clone
}

// SamplerInitMirostat initializes a Mirostat sampler.
// nVocab is the vocabulary size, seed is the random seed, tau is the target entropy,
// eta is the learning rate, and m is the number of tokens considered in the estimation.
func SamplerInitMirostat(nVocab int32, seed uint32, tau, eta float32, m int32) Sampler {
	var s Sampler
	samplerInitMirostatFunc.Call(unsafe.Pointer(&s), &nVocab, &seed, &tau, &eta, &m)
	return s
}

// SamplerInitMirostatV2 initializes a Mirostat v2 sampler.
// seed is the random seed, tau is the target entropy, and eta is the learning rate.
func SamplerInitMirostatV2(seed uint32, tau, eta float32) Sampler {
	var s Sampler
	samplerInitMirostatV2Func.Call(unsafe.Pointer(&s), &seed, &tau, &eta)
	return s
}

// SamplerGetSeed returns the seed used by the sampler if applicable, or DefaultSeed otherwise.
func SamplerGetSeed(smpl Sampler) uint32 {
	if smpl == 0 {
		return DefaultSeed
	}
	var result ffi.Arg
	samplerGetSeedFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&smpl))
	return uint32(result)
}

var (
	// DefaultSamplers is the list of default samplers to use in a sampling chain.
	DefaultSamplers = []SamplerType{
		SamplerTypeLogitBias,
		SamplerTypePenalties,
		SamplerTypeDry,
		SamplerTypeTopNSigma,
		SamplerTypeTopK,
		SamplerTypeTypicalP,
		SamplerTypeTopP,
		SamplerTypeMinP,
		SamplerTypeXTC,
		SamplerTypeTemperature,
	}
)

// NewSampler creates a new sampling chain.
// The samplers parameter is a list of SamplerType values to include in the chain.
// The samplers are added in the order they appear in the list.
// The distribution sampler is always added last.
// If the model is nil or the samplers list is empty, a zero Sampler is returned.
func NewSampler(model Model, samplers []SamplerType, params *SamplerParams) Sampler {
	var sampler Sampler
	if model == 0 || len(samplers) == 0 {
		return sampler
	}
	vocab := ModelGetVocab(model)
	nTokens := VocabNTokens(vocab)

	sampler = SamplerChainInit(SamplerChainDefaultParams())

	// add EOG logit bias to prevent generating EOG tokens
	logitBiasEOG := make([]LogitBias, 0)
	for i := range nTokens {
		token := Token(i)
		if VocabIsEOG(vocab, token) {
			logitBiasEOG = append(logitBiasEOG, LogitBias{Token: token, Bias: math.SmallestNonzeroFloat32})
		}
	}

	// add other samplers
	for _, samplerType := range samplers {
		switch samplerType {
		case SamplerTypeLogitBias:
			bias := SamplerInitLogitBias(nTokens, int32(len(logitBiasEOG)), unsafe.SliceData(logitBiasEOG))
			SamplerChainAdd(sampler, bias)

		case SamplerTypeDry:
			dry := SamplerInitDry(vocab, ModelNCtxTrain(model), params.DryMultiplier, params.DryBase, params.DryAllowedLength, params.DryPenaltyLastN, params.DrySequenceBreakers)
			SamplerChainAdd(sampler, dry)

		case SamplerTypeTopK:
			topK := SamplerInitTopK(params.TopK)
			SamplerChainAdd(sampler, topK)

		case SamplerTypeTopP:
			topP := SamplerInitTopP(params.TopP, 0)
			SamplerChainAdd(sampler, topP)

		case SamplerTypeMinP:
			minP := SamplerInitMinP(params.MinP, 0)
			SamplerChainAdd(sampler, minP)

		case SamplerTypeTypicalP:
			typical := SamplerInitTypical(params.TypP, 0)
			SamplerChainAdd(sampler, typical)

		case SamplerTypeTemperature:
			temp := SamplerInitTempExt(params.Temp, 0, 1.0)
			SamplerChainAdd(sampler, temp)

		case SamplerTypeXTC:
			xtc := SamplerInitXTC(params.XTCProbability, params.XTCThreshold, 0, params.Seed)
			SamplerChainAdd(sampler, xtc)

		case SamplerTypeInfill:
			// TODO: add implementation

		case SamplerTypePenalties:
			penalties := SamplerInitPenalties(params.PenaltyLastN, params.PenaltyRepeat, params.PenaltyFreq, params.PenaltyPresent)
			SamplerChainAdd(sampler, penalties)

		case SamplerTypeTopNSigma:
			topNSigma := SamplerInitTopNSigma(params.TopNSigma)
			SamplerChainAdd(sampler, topNSigma)
		}
	}

	// always add dist sampler last
	dist := SamplerInitDist(params.Seed)
	SamplerChainAdd(sampler, dist)

	return sampler
}

// SamplerParams holds the parameters for creating samplers.
type SamplerParams struct {
	Seed                uint32
	NPrev               int32
	NProbs              int32
	MinKeep             int32
	TopK                int32
	TopP                float32
	MinP                float32
	XTCProbability      float32
	XTCThreshold        float32
	TypP                float32
	Temp                float32
	DynatempRange       float32
	DynatempExponent    float32
	PenaltyLastN        int32
	PenaltyRepeat       float32
	PenaltyFreq         float32
	PenaltyPresent      float32
	DryMultiplier       float32
	DryBase             float32
	DryAllowedLength    int32
	DryPenaltyLastN     int32
	Mirostat            int32
	TopNSigma           float32
	MirostatTau         float32
	MirostatEta         float32
	IgnoreEos           bool
	NoPerf              bool
	TimingPerToken      bool
	DrySequenceBreakers []string
}

// DefaultSamplerParams returns the default sampler parameters.
func DefaultSamplerParams() *SamplerParams {
	return &SamplerParams{
		Seed: DefaultSeed,
		// number of previous tokens to remember
		NPrev: 64,
		// if greater than 0, output the probabilities of top n_probs tokens.
		NProbs: 0,
		// 0 = disabled, otherwise samplers should return at least min_keep tokens
		MinKeep: 0,
		// <= 0 to use vocab size
		TopK: 40,
		// 1.0 = disabled
		TopP: 0.95,
		// 0.0 = disabled
		MinP: 0.05,
		// 0.0 = disabled
		XTCProbability: 0.0,
		// > 0.5 disables XTC
		XTCThreshold: 0.1,
		// typical_p, 1.0 = disabled
		TypP: 1.0,
		// <= 0.0 to sample greedily, 0.0 to not output probabilities
		Temp: 0.8,
		// 0.0 = disabled
		DynatempRange: 0.0,
		// controls how entropy maps to temperature in dynamic temperature sampler
		DynatempExponent: 1.0,
		// last n tokens to penalize (0 = disable penalty, -1 = context size)
		PenaltyLastN: 64,
		// 1.0 = disabled
		PenaltyRepeat: 1.0,
		// 0.0 = disabled
		PenaltyFreq: 0.0,
		// 0.0 = disabled
		PenaltyPresent: 0.0,
		// 0.0 = disabled;      DRY repetition penalty for tokens extending repetition:
		DryMultiplier: 0.0,
		// 0.0 = disabled;      multiplier * base ^ (length of sequence before token - allowed length)
		DryBase: 1.75,
		// tokens extending repetitions beyond this receive penalty
		DryAllowedLength: 2,
		// how many tokens to scan for repetitions (0 = disable penalty, -1 = context size)
		DryPenaltyLastN: -1,
		// 0 = disabled, 1 = mirostat, 2 = mirostat 2.0
		Mirostat: 0,
		// -1.0 = disabled
		TopNSigma: -1.0,
		// target entropy
		MirostatTau: 5.0,
		// learning rate
		MirostatEta: 0.1,
		// if true, ignore end-of-sequence token
		IgnoreEos: false,
		// disable performance metrics
		NoPerf: false,
		// if true, enable timing per token
		TimingPerToken: false,
		// default sequence breakers for DRY
		DrySequenceBreakers: []string{"\n", ":", "\"", "*"},
	}
}

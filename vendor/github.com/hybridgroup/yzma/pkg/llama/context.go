package llama

import (
	"errors"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/jupiterrider/ffi"
)

// ffiTypeContextParams represents the C struct llama_context_params
var ffiTypeContextParams = ffi.NewType(
	&ffi.TypeUint32, &ffi.TypeUint32,
	&ffi.TypeUint32, &ffi.TypeUint32,
	&ffi.TypeSint32, &ffi.TypeSint32,
	&ffi.TypeSint32, &ffi.TypeSint32,
	&ffi.TypeSint32, &ffi.TypeSint32,
	&ffi.TypeFloat, &ffi.TypeFloat,
	&ffi.TypeFloat, &ffi.TypeFloat,
	&ffi.TypeFloat, &ffi.TypeFloat,
	&ffi.TypeUint32, &ffi.TypeFloat,
	&ffi.TypePointer, &ffi.TypePointer,
	&ffi.TypeSint32, &ffi.TypeSint32,
	&ffi.TypePointer, &ffi.TypePointer,
	&ffi.TypeUint8, &ffi.TypeUint8,
	&ffi.TypeUint8, &ffi.TypeUint8,
	&ffi.TypeUint8, &ffi.TypeUint8,
	&ffi.TypePointer, &ffi.TypeUint64)

var (
	// LLAMA_API struct llama_context_params        llama_context_default_params(void);
	contextDefaultParamsFunc ffi.Fun

	// LLAMA_API void llama_free(struct llama_context * ctx);
	freeFunc ffi.Fun

	// LLAMA_API void llama_set_warmup(struct llama_context * ctx, bool warmup);
	setWarmupFunc ffi.Fun

	// LLAMA_API int32_t llama_encode(
	//         		struct llama_context * ctx,
	//          	struct llama_batch   batch);
	encodeFunc ffi.Fun

	// LLAMA_API int32_t llama_decode(
	// 				struct llama_context * ctx,
	// 				struct llama_batch   batch);
	decodeFunc ffi.Fun

	// LLAMA_API void                           llama_perf_context_reset(      struct llama_context * ctx);
	perfContextResetFunc ffi.Fun

	// LLAMA_API llama_memory_t llama_get_memory (const struct llama_context * ctx);
	getMemoryFunc ffi.Fun

	// LLAMA_API void llama_synchronize(struct llama_context * ctx);
	synchronizeFunc ffi.Fun

	// LLAMA_API  enum llama_pooling_type   llama_pooling_type(const struct llama_context * ctx); // TODO: rename to llama_get_pooling_type
	poolingTypeFunc ffi.Fun

	// Get the embeddings for the ith token. For positive indices, Equivalent to:
	// llama_get_embeddings(ctx) + ctx->output_ids[i]*n_embd
	// Negative indicies can be used to access embeddings in reverse order, -1 is the last embedding.
	// shape: [n_embd] (1-dimensional)
	// returns NULL for invalid ids.
	// LLAMA_API float * llama_get_embeddings_ith(struct llama_context * ctx, int32_t i);
	getEmbeddingsIthFunc ffi.Fun

	// Get the embeddings for a sequence id
	// Returns NULL if pooling_type is LLAMA_POOLING_TYPE_NONE
	// when pooling_type == LLAMA_POOLING_TYPE_RANK, returns float[n_cls_out] with the rank(s) of the sequence
	// otherwise: float[n_embd] (1-dimensional)
	// LLAMA_API float * llama_get_embeddings_seq(struct llama_context * ctx, llama_seq_id seq_id);
	getEmbeddingsSeqFunc ffi.Fun

	// Get all output token embeddings.
	// when pooling_type == LLAMA_POOLING_TYPE_NONE or when using a generative model,
	// the embeddings for which llama_batch.logits[i] != 0 are stored contiguously
	// in the order they have appeared in the batch.
	// shape: [n_outputs*n_embd]
	// Otherwise, returns NULL.
	// TODO: deprecate in favor of llama_get_embeddings_ith() (ref: https://github.com/ggml-org/llama.cpp/pull/14853#issuecomment-3113143522)
	// LLAMA_API float * llama_get_embeddings(struct llama_context * ctx);
	getEmbeddingsFunc ffi.Fun

	// LLAMA_API float * llama_get_logits_ith(struct llama_context * ctx, int32_t i);
	getLogitsIthFunc ffi.Fun

	// Token logits obtained from the last call to llama_decode()
	// The logits for which llama_batch.logits[i] != 0 are stored contiguously
	// in the order they have appeared in the batch.
	// Rows: number of tokens for which llama_batch.logits[i] != 0
	// Cols: n_vocab
	// TODO: deprecate in favor of llama_get_logits_ith() (ref: https://github.com/ggml-org/llama.cpp/pull/14853#issuecomment-3113143522)
	// LLAMA_API float * llama_get_logits(struct llama_context * ctx);
	getLogitsFunc ffi.Fun

	// LLAMA_API uint32_t llama_n_ctx(const struct llama_context * ctx);
	nCtxFunc ffi.Fun

	// LLAMA_API uint32_t llama_n_batch(const struct llama_context * ctx);
	nBatchFunc ffi.Fun

	// LLAMA_API uint32_t llama_n_ubatch(const struct llama_context * ctx);
	nUBatchFunc ffi.Fun

	// LLAMA_API uint32_t llama_n_seq_max(const struct llama_context * ctx);
	nSeqMaxFunc ffi.Fun

	// LLAMA_API const struct llama_model * llama_get_model(const struct llama_context * ctx);
	getModelFunc ffi.Fun

	// LLAMA_API void llama_set_embeddings(struct llama_context * ctx, bool embeddings);
	setEmbeddingsFunc ffi.Fun

	// LLAMA_API void llama_set_causal_attn(struct llama_context * ctx, bool causal_attn);
	setCausalAttnFunc ffi.Fun

	// LLAMA_API int32_t llama_set_adapter_cvec(
	//         struct llama_context * ctx,
	//                  const float * data,
	//                       size_t   len,
	//                      int32_t   n_embd,
	//                      int32_t   il_start,
	//                      int32_t   il_end);
	setAdapterCvecFunc ffi.Fun

	// LLAMA_API llama_token llama_get_sampled_token_ith(struct llama_context * ctx, int32_t i);
	getSampledTokenIthFunc ffi.Fun

	// LLAMA_API float * llama_get_sampled_probs_ith(struct llama_context * ctx, int32_t i);
	getSampledProbsIthFunc ffi.Fun

	// LLAMA_API uint32_t llama_get_sampled_probs_count_ith(struct llama_context * ctx, int32_t i);
	getSampledProbsCountIthFunc ffi.Fun

	// LLAMA_API float * llama_get_sampled_logits_ith(struct llama_context * ctx, int32_t i);
	getSampledLogitsIthFunc ffi.Fun

	// LLAMA_API uint32_t llama_get_sampled_logits_count_ith(struct llama_context * ctx, int32_t i);
	getSampledLogitsCountIthFunc ffi.Fun

	// LLAMA_API llama_token * llama_get_sampled_candidates_ith(struct llama_context * ctx, int32_t i);
	getSampledCandidatesIthFunc ffi.Fun

	// LLAMA_API uint32_t llama_get_sampled_candidates_count_ith(struct llama_context * ctx, int32_t i);
	getSampledCandidatesCountIthFunc ffi.Fun

	// LLAMA_API void llama_set_abort_callback(struct llama_context * ctx, ggml_abort_callback abort_callback, void * abort_callback_data);
	setAbortCallbackFunc ffi.Fun

	// LLAMA_API bool llama_set_sampler(struct llama_context * ctx, llama_seq_id seq_id, struct llama_sampler * smpl);
	setSamplerFunc ffi.Fun

	// LLAMA_API void llama_attach_threadpool(
	//         struct llama_context * ctx,
	//            ggml_threadpool_t   threadpool,
	//            ggml_threadpool_t   threadpool_batch);
	attachThreadpoolFunc ffi.Fun

	// LLAMA_API void llama_detach_threadpool(struct llama_context * ctx);
	detachThreadpoolFunc ffi.Fun

	// LLAMA_API void llama_set_n_threads(struct llama_context * ctx, int32_t n_threads, int32_t n_threads_batch);
	setNThreadsFunc ffi.Fun

	// LLAMA_API int32_t llama_n_threads(struct llama_context * ctx);
	nThreadsFunc ffi.Fun

	// LLAMA_API int32_t llama_n_threads_batch(struct llama_context * ctx);
	nThreadsBatchFunc ffi.Fun

	// LLAMA_API uint32_t llama_n_ctx_seq(const struct llama_context * ctx);
	nCtxSeqFunc ffi.Fun
)

func loadContextFuncs(lib ffi.Lib) error {
	var err error

	if contextDefaultParamsFunc, err = lib.Prep("llama_context_default_params", &ffiTypeContextParams); err != nil {
		return loadError("llama_context_default_params", err)
	}

	if freeFunc, err = lib.Prep("llama_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_free", err)
	}

	if setWarmupFunc, err = lib.Prep("llama_set_warmup", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeUint8); err != nil {
		return loadError("llama_set_warmup", err)
	}

	if encodeFunc, err = lib.Prep("llama_encode", &ffi.TypeSint32, &ffi.TypePointer, &ffiTypeBatch); err != nil {
		return loadError("llama_encode", err)
	}

	if decodeFunc, err = lib.Prep("llama_decode", &ffi.TypeSint32, &ffi.TypePointer, &ffiTypeBatch); err != nil {
		return loadError("llama_decode", err)
	}

	if perfContextResetFunc, err = lib.Prep("llama_perf_context_reset", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_perf_context_reset", err)
	}

	if getMemoryFunc, err = lib.Prep("llama_get_memory", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_get_memory", err)
	}

	if synchronizeFunc, err = lib.Prep("llama_synchronize", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_synchronize", err)
	}

	if poolingTypeFunc, err = lib.Prep("llama_pooling_type", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_pooling_type", err)
	}

	if getEmbeddingsIthFunc, err = lib.Prep("llama_get_embeddings_ith", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_embeddings_ith", err)
	}

	if getEmbeddingsSeqFunc, err = lib.Prep("llama_get_embeddings_seq", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_embeddings_seq", err)
	}

	if getEmbeddingsFunc, err = lib.Prep("llama_get_embeddings", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_get_embeddings", err)
	}

	if getLogitsIthFunc, err = lib.Prep("llama_get_logits_ith", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_logits_ith", err)
	}

	if getLogitsFunc, err = lib.Prep("llama_get_logits", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_get_logits", err)
	}

	if nCtxFunc, err = lib.Prep("llama_n_ctx", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("llama_n_ctx", err)
	}

	if nBatchFunc, err = lib.Prep("llama_n_batch", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("llama_n_batch", err)
	}

	if nUBatchFunc, err = lib.Prep("llama_n_ubatch", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("llama_n_ubatch", err)
	}

	if nSeqMaxFunc, err = lib.Prep("llama_n_seq_max", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("llama_n_seq_max", err)
	}

	if getModelFunc, err = lib.Prep("llama_get_model", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_get_model", err)
	}

	if setEmbeddingsFunc, err = lib.Prep("llama_set_embeddings", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeUint8); err != nil {
		return loadError("llama_set_embeddings", err)
	}

	if setCausalAttnFunc, err = lib.Prep("llama_set_causal_attn", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeUint8); err != nil {
		return loadError("llama_set_causal_attn", err)
	}

	if setAdapterCvecFunc, err = lib.Prep("llama_set_adapter_cvec", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("llama_set_adapter_cvec", err)
	}

	if getSampledTokenIthFunc, err = lib.Prep("llama_get_sampled_token_ith", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_sampled_token_ith", err)
	}

	if getSampledProbsIthFunc, err = lib.Prep("llama_get_sampled_probs_ith", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_sampled_probs_ith", err)
	}

	if getSampledProbsCountIthFunc, err = lib.Prep("llama_get_sampled_probs_count_ith", &ffi.TypeUint32, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_sampled_probs_count_ith", err)
	}

	if getSampledLogitsIthFunc, err = lib.Prep("llama_get_sampled_logits_ith", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_sampled_logits_ith", err)
	}

	if getSampledLogitsCountIthFunc, err = lib.Prep("llama_get_sampled_logits_count_ith", &ffi.TypeUint32, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_sampled_logits_count_ith", err)
	}

	if getSampledCandidatesIthFunc, err = lib.Prep("llama_get_sampled_candidates_ith", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_sampled_candidates_ith", err)
	}

	if getSampledCandidatesCountIthFunc, err = lib.Prep("llama_get_sampled_candidates_count_ith", &ffi.TypeUint32, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_get_sampled_candidates_count_ith", err)
	}

	if setAbortCallbackFunc, err = lib.Prep("llama_set_abort_callback", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_set_abort_callback", err)
	}

	if setSamplerFunc, err = lib.Prep("llama_set_sampler", &ffi.TypeUint8, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_set_sampler", err)
	}

	if attachThreadpoolFunc, err = lib.Prep("llama_attach_threadpool", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_attach_threadpool", err)
	}

	if detachThreadpoolFunc, err = lib.Prep("llama_detach_threadpool", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_detach_threadpool", err)
	}

	if setNThreadsFunc, err = lib.Prep("llama_set_n_threads", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("llama_set_n_threads", err)
	}

	if nThreadsFunc, err = lib.Prep("llama_n_threads", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_n_threads", err)
	}

	if nThreadsBatchFunc, err = lib.Prep("llama_n_threads_batch", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("llama_n_threads_batch", err)
	}

	if nCtxSeqFunc, err = lib.Prep("llama_n_ctx_seq", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("llama_n_ctx_seq", err)
	}

	return nil
}

var (
	errInvalidContext = errors.New("invalid context")
)

// ContextDefaultParams returns the default params to initialize a model context.
func ContextDefaultParams() ContextParams {
	var p ContextParams
	contextDefaultParamsFunc.Call(unsafe.Pointer(&p))
	return p
}

// Free frees the resources for a model context.
func Free(ctx Context) error {
	if ctx == 0 {
		return errInvalidContext
	}
	freeFunc.Call(nil, unsafe.Pointer(&ctx))
	return nil
}

// SetWarmup sets the model context warmup mode on or off.
func SetWarmup(ctx Context, warmup bool) error {
	if ctx == 0 {
		return errInvalidContext
	}
	setWarmupFunc.Call(nil, unsafe.Pointer(&ctx), &warmup)
	return nil
}

// Encode encodes a batch of Token.
func Encode(ctx Context, batch Batch) (int32, error) {
	if ctx == 0 {
		return 0, errInvalidContext
	}
	var result ffi.Arg
	encodeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&batch))

	return int32(result), nil
}

// Decode decodes a batch of Token.
func Decode(ctx Context, batch Batch) (int32, error) {
	if ctx == 0 {
		return 0, errInvalidContext
	}
	var result ffi.Arg
	decodeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&batch))

	return int32(result), nil
}

// PerfContextReset resets the performance metrics for the model context.
func PerfContextReset(ctx Context) error {
	if ctx == 0 {
		return errInvalidContext
	}
	perfContextResetFunc.Call(nil, unsafe.Pointer(&ctx))
	return nil
}

// GetMemory returns the current Memory for the Context.
func GetMemory(ctx Context) (Memory, error) {
	if ctx == 0 {
		return 0, errInvalidContext
	}
	var mem Memory
	getMemoryFunc.Call(unsafe.Pointer(&mem), unsafe.Pointer(&ctx))

	return mem, nil
}

// Synchronize waits until all computations are finished.
// This is automatically done when using one of the functions that obtains computation results
// and is not necessary to call it explicitly in most cases.
func Synchronize(ctx Context) error {
	if ctx == 0 {
		return errInvalidContext
	}
	synchronizeFunc.Call(nil, unsafe.Pointer(&ctx))
	return nil
}

// GetPoolingType returns the PoolingType for this context.
func GetPoolingType(ctx Context) PoolingType {
	if ctx == 0 {
		return PoolingTypeNone
	}
	var result ffi.Arg
	poolingTypeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))

	return PoolingType(result)
}

// GetEmbeddingsIth gets the embeddings for the ith token.
func GetEmbeddingsIth(ctx Context, i int32, nVocab int32) ([]float32, error) {
	if ctx == 0 {
		return nil, errInvalidContext
	}
	var result *float32
	getEmbeddingsIthFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &i)

	if result == nil {
		return nil, nil
	}

	return unsafe.Slice(result, nVocab), nil
}

// GetEmbeddingsSeq gets the embeddings for this sequence ID.
func GetEmbeddingsSeq(ctx Context, seqID SeqId, nVocab int32) ([]float32, error) {
	if ctx == 0 {
		return nil, errInvalidContext
	}
	var result *float32
	getEmbeddingsSeqFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &seqID)

	if result == nil {
		return nil, nil
	}

	return unsafe.Slice(result, nVocab), nil
}

// GetEmbeddings retrieves all output token embeddings.
// Returns a slice of float32 of length nOutputs * nEmbeddings, or nil if not available.
func GetEmbeddings(ctx Context, nOutputs, nEmbeddings int) ([]float32, error) {
	if ctx == 0 {
		return nil, errInvalidContext
	}
	var result *float32
	getEmbeddingsFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	if result == nil || nOutputs <= 0 || nEmbeddings <= 0 {
		return nil, nil
	}
	return unsafe.Slice(result, nOutputs*nEmbeddings), nil
}

// GetLogitsIth retrieves the logits for the ith token.
func GetLogitsIth(ctx Context, i int32, nVocab int) ([]float32, error) {
	if ctx == 0 {
		return nil, errInvalidContext
	}
	var logitsPtr *float32
	getLogitsIthFunc.Call(unsafe.Pointer(&logitsPtr), unsafe.Pointer(&ctx), &i)

	if logitsPtr == nil {
		return nil, nil
	}

	return unsafe.Slice(logitsPtr, nVocab), nil
}

// GetLogits retrieves all token logits from the last call to llama_decode.
// Returns a slice of float32 of length nTokens * nVocab, or nil if not available.
func GetLogits(ctx Context, nTokens, nVocab int) ([]float32, error) {
	if ctx == 0 {
		return nil, errInvalidContext
	}
	var result *float32
	getLogitsFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	if result == nil || nTokens <= 0 || nVocab <= 0 {
		return nil, nil
	}
	return unsafe.Slice(result, nTokens*nVocab), nil
}

// NCtx returns the number of context tokens.
func NCtx(ctx Context) uint32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nCtxFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return uint32(result)
}

// NBatch returns the number of batch tokens.
func NBatch(ctx Context) uint32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nBatchFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return uint32(result)
}

// NUBatch returns the number of micro-batch tokens.
func NUBatch(ctx Context) uint32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nUBatchFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return uint32(result)
}

// NSeqMax returns the maximum number of sequences.
func NSeqMax(ctx Context) uint32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nSeqMaxFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return uint32(result)
}

// GetModel retrieves the model associated with the given context.
func GetModel(ctx Context) Model {
	var model Model
	if ctx == 0 {
		return model
	}
	getModelFunc.Call(unsafe.Pointer(&model), unsafe.Pointer(&ctx))

	return model
}

// SetEmbeddings sets whether the context outputs embeddings or not.
func SetEmbeddings(ctx Context, embeddings bool) {
	if ctx == 0 {
		return
	}
	setEmbeddingsFunc.Call(nil, unsafe.Pointer(&ctx), &embeddings)
}

// SetCausalAttn sets whether to use causal attention or not.
func SetCausalAttn(ctx Context, causalAttn bool) {
	if ctx == 0 {
		return
	}
	setCausalAttnFunc.Call(nil, unsafe.Pointer(&ctx), &causalAttn)
}

// SetAdapterCvec sets a loaded control vector to a llama_context, or if data is nil, clears
// the currently loaded vector.
// nEmbd should be the size of a single layer's control, and data should point
// to an nEmbd x nLayers buffer starting from layer 1.
// ilStart and ilEnd are the layer range the vector should apply to (both inclusive)
// Returns 0 on success, or a negative value on failure.
func SetAdapterCvec(ctx Context, data []float32, nEmbd, ilStart, ilEnd int32) int32 {
	if ctx == 0 {
		return -1
	}

	var (
		result  ffi.Arg
		dataPtr *float32
		length  uint64
	)

	// If data is nil, we're clearing the vector
	if data != nil {
		dataPtr = unsafe.SliceData(data)
		length = uint64(len(data))
	}

	setAdapterCvecFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&dataPtr),
		&length,
		&nEmbd,
		&ilStart,
		&ilEnd,
	)

	return int32(result)
}

// GetSampledTokenIth retrieves the sampled token for the ith output.
func GetSampledTokenIth(ctx Context, i int32) (Token, error) {
	if ctx == 0 {
		return TokenNull, errInvalidContext
	}
	var result ffi.Arg
	getSampledTokenIthFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &i)
	return Token(result), nil
}

// GetSampledProbsIth retrieves the sampled probabilities for the ith output.
func GetSampledProbsIth(ctx Context, i int32, nVocab int) ([]float32, error) {
	if ctx == 0 {
		return nil, errInvalidContext
	}
	var result *float32
	getSampledProbsIthFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &i)
	if result == nil {
		return nil, nil
	}
	return unsafe.Slice(result, nVocab), nil
}

// GetSampledProbsCountIth retrieves the count of sampled probabilities for the ith output.
func GetSampledProbsCountIth(ctx Context, i int32) (uint32, error) {
	if ctx == 0 {
		return 0, errInvalidContext
	}
	var result ffi.Arg
	getSampledProbsCountIthFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &i)
	return uint32(result), nil
}

// GetSampledLogitsIth retrieves the sampled logits for the ith output.
func GetSampledLogitsIth(ctx Context, i int32, nVocab int) ([]float32, error) {
	if ctx == 0 {
		return nil, errInvalidContext
	}
	var result *float32
	getSampledLogitsIthFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &i)
	if result == nil {
		return nil, nil
	}
	return unsafe.Slice(result, nVocab), nil
}

// GetSampledLogitsCountIth retrieves the count of sampled logits for the ith output.
func GetSampledLogitsCountIth(ctx Context, i int32) (uint32, error) {
	if ctx == 0 {
		return 0, errInvalidContext
	}
	var result ffi.Arg
	getSampledLogitsCountIthFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &i)
	return uint32(result), nil
}

// GetSampledCandidatesIth retrieves the sampled candidates for the ith output.
func GetSampledCandidatesIth(ctx Context, i int32, nVocab int) ([]Token, error) {
	if ctx == 0 {
		return nil, errInvalidContext
	}
	var result *Token
	getSampledCandidatesIthFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &i)
	if result == nil {
		return nil, nil
	}
	return unsafe.Slice(result, nVocab), nil
}

// GetSampledCandidatesCountIth retrieves the count of sampled candidates for the ith output.
func GetSampledCandidatesCountIth(ctx Context, i int32) (uint32, error) {
	if ctx == 0 {
		return 0, errInvalidContext
	}
	var result ffi.Arg
	getSampledCandidatesCountIthFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &i)
	return uint32(result), nil
}

// AbortFunc is a callback function that can be used to abort computation.
type AbortFunc func() bool

// SetAbortCallback sets a callback function that can be used to abort computation.
// The callback is called before ggml computation. If it returns true, the computation is aborted.
// The data parameter is passed to the callback function on each invocation.
// Pass nil for fn to clear the abort callback.
func SetAbortCallback(ctx Context, fn AbortFunc) {
	callback := newAbortCallback(fn)

	var nilPtr uintptr
	setAbortCallbackFunc.Call(nil, unsafe.Pointer(&ctx), unsafe.Pointer(&callback), unsafe.Pointer(&nilPtr))
}

// SetSampler attaches a sampler to the context for the given sequence ID,
// enabling backend (GPU-side) sampling during decode. When a sampler is
// registered, llama_decode produces sampled tokens, probabilities, and
// candidate lists as part of the compute graph, making them available via
// GetSampledCandidatesIth / GetSampledProbsIth.
//
// Pass a zero Sampler to remove the sampler for the given sequence.
// Returns true if the sampler was successfully attached (or removed).
func SetSampler(ctx Context, seqID SeqId, smpl Sampler) bool {
	if ctx == 0 {
		return false
	}

	var result ffi.Arg
	id := int32(seqID)

	if smpl == 0 {
		var nilPtr uintptr
		setSamplerFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &id, unsafe.Pointer(&nilPtr))
	} else {
		setSamplerFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &id, unsafe.Pointer(&smpl))
	}

	return result.Bool()
}

// AttachThreadpool attaches a ggml threadpool to the context.
// The threadpool and threadpoolBatch are opaque pointers (uintptr) to ggml_threadpool_t.
func AttachThreadpool(ctx Context, threadpool, threadpoolBatch uintptr) {
	if ctx == 0 {
		return
	}
	attachThreadpoolFunc.Call(nil, unsafe.Pointer(&ctx), unsafe.Pointer(&threadpool), unsafe.Pointer(&threadpoolBatch))
}

// DetachThreadpool detaches the ggml threadpool from the context.
func DetachThreadpool(ctx Context) {
	if ctx == 0 {
		return
	}
	detachThreadpoolFunc.Call(nil, unsafe.Pointer(&ctx))
}

// SetNThreads sets the number of threads used for decoding.
// nThreads is the number of threads used for generation (single token).
// nThreadsBatch is the number of threads used for prompt and batch processing (multiple tokens).
func SetNThreads(ctx Context, nThreads, nThreadsBatch int32) {
	if ctx == 0 {
		return
	}
	setNThreadsFunc.Call(nil, unsafe.Pointer(&ctx), &nThreads, &nThreadsBatch)
}

// NThreads returns the number of threads used for generation of a single token.
func NThreads(ctx Context) int32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nThreadsFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return int32(result)
}

// NThreadsBatch returns the number of threads used for prompt and batch processing.
func NThreadsBatch(ctx Context) int32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nThreadsBatchFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return int32(result)
}

// NCtxSeq returns the context size per sequence.
func NCtxSeq(ctx Context) uint32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nCtxSeqFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return uint32(result)
}

// newAbortCallback creates a C-compatible callback from a Go AbortFunc.
func newAbortCallback(fn AbortFunc) uintptr {
	return purego.NewCallback(func(data uintptr) uintptr {
		if fn() {
			return 1
		}
		return 0
	})
}

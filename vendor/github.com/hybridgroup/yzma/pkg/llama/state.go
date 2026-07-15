package llama

import (
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

var (
	// LLAMA_API bool llama_state_save_file(
	//     struct llama_context * ctx,
	//     const char * path_session,
	//     const llama_token * tokens,
	//     size_t   n_token_count);
	stateSaveFileFunc ffi.Fun

	// LLAMA_API bool llama_state_load_file(
	//     struct llama_context * ctx,
	//               const char * path_session,
	//              llama_token * tokens_out,
	//                   size_t   n_token_capacity,
	//                   size_t * n_token_count_out);
	stateLoadFileFunc ffi.Fun

	// Returns the *actual* size in bytes of the state
	// (logits, embedding and memory)
	// Only use when saving the state, not when restoring it, otherwise the size may be too small.
	// LLAMA_API size_t llama_state_get_size(struct llama_context * ctx);
	stateGetSizeFunc ffi.Fun

	// Copies the state to the specified destination address.
	// Destination needs to have allocated enough memory.
	// Returns the number of bytes copied
	// LLAMA_API size_t llama_state_get_data(
	//         struct llama_context * ctx,
	//                      uint8_t * dst,
	//                       size_t   size);
	stateGetDataFunc ffi.Fun

	// Set the state reading from the specified address
	// Returns the number of bytes read
	// LLAMA_API size_t llama_state_set_data(
	//         struct llama_context * ctx,
	//                const uint8_t * src,
	//                       size_t   size);
	stateSetDataFunc ffi.Fun

	// Get the exact size needed to copy the state of a single sequence
	// LLAMA_API size_t llama_state_seq_get_size(
	//         struct llama_context * ctx,
	//                 llama_seq_id   seq_id);
	stateSeqGetSizeFunc ffi.Fun

	// Copy the state of a single sequence into the specified buffer
	// LLAMA_API size_t llama_state_seq_get_data(
	//         struct llama_context * ctx,
	//                      uint8_t * dst,
	//                       size_t   size,
	//                 llama_seq_id   seq_id);
	stateSeqGetDataFunc ffi.Fun

	// Copy the sequence data (originally copied with `llama_state_seq_get_data`) into the specified sequence
	// Returns:
	//  - Positive: Ok
	//  - Zero: Failed to load
	// LLAMA_API size_t llama_state_seq_set_data(
	//         struct llama_context * ctx,
	//                const uint8_t * src,
	//                       size_t   size,
	//                 llama_seq_id   dest_seq_id);
	stateSeqSetDataFunc ffi.Fun

	// LLAMA_API size_t llama_state_seq_save_file(
	//         struct llama_context * ctx,
	//                   const char * filepath,
	//                 llama_seq_id   seq_id,
	//            const llama_token * tokens,
	//                       size_t   n_token_count);
	stateSeqSaveFileFunc ffi.Fun

	// LLAMA_API size_t llama_state_seq_load_file(
	//         struct llama_context * ctx,
	//                   const char * filepath,
	//                 llama_seq_id   dest_seq_id,
	//                  llama_token * tokens_out,
	//                       size_t   n_token_capacity,
	//                       size_t * n_token_count_out);
	stateSeqLoadFileFunc ffi.Fun

	// for backwards-compat
	//#define LLAMA_STATE_SEQ_FLAGS_SWA_ONLY 1

	// work only with partial states, such as SWA KV cache or recurrent cache (e.g. Mamba)
	// #define LLAMA_STATE_SEQ_FLAGS_PARTIAL_ONLY 1

	// typedef uint32_t llama_state_seq_flags;

	// LLAMA_API size_t llama_state_seq_get_size_ext(
	//         struct llama_context * ctx,
	//                 llama_seq_id   seq_id,
	//        llama_state_seq_flags   flags);
	stateSeqGetSizeExtFunc ffi.Fun

	// LLAMA_API size_t llama_state_seq_get_data_ext(
	//         struct llama_context * ctx,
	//                      uint8_t * dst,
	//                       size_t   size,
	//                 llama_seq_id   seq_id,
	//        llama_state_seq_flags   flags);
	stateSeqGetDataExtFunc ffi.Fun

	// LLAMA_API size_t llama_state_seq_set_data_ext(
	//         struct llama_context * ctx,
	//                const uint8_t * src,
	//                       size_t   size,
	//                 llama_seq_id   dest_seq_id,
	//        llama_state_seq_flags   flags);
	stateSeqSetDataExtFunc ffi.Fun
)

func loadStateFuncs(lib ffi.Lib) error {
	var err error

	if stateSaveFileFunc, err = lib.Prep("llama_state_save_file", &ffi.TypeUint8, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64); err != nil {
		return loadError("llama_state_save_file", err)
	}

	if stateLoadFileFunc, err = lib.Prep("llama_state_load_file", &ffi.TypeUint8, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64, &ffi.TypePointer); err != nil {
		return loadError("llama_state_load_file", err)
	}

	if stateGetSizeFunc, err = lib.Prep("llama_state_get_size", &ffi.TypeUint64, &ffi.TypePointer); err != nil {
		return loadError("llama_state_get_size", err)
	}

	if stateGetDataFunc, err = lib.Prep("llama_state_get_data", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64); err != nil {
		return loadError("llama_state_get_data", err)
	}

	if stateSetDataFunc, err = lib.Prep("llama_state_set_data", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64); err != nil {
		return loadError("llama_state_set_data", err)
	}

	if stateSeqGetSizeFunc, err = lib.Prep("llama_state_seq_get_size", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_state_seq_get_size", err)
	}

	if stateSeqGetDataFunc, err = lib.Prep("llama_state_seq_get_data", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64, &ffi.TypeSint32); err != nil {
		return loadError("llama_state_seq_get_data", err)
	}

	if stateSeqSetDataFunc, err = lib.Prep("llama_state_seq_set_data", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64, &ffi.TypeSint32); err != nil {
		return loadError("llama_state_seq_set_data", err)
	}

	if stateSeqSaveFileFunc, err = lib.Prep("llama_state_seq_save_file", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeUint64); err != nil {
		return loadError("llama_state_seq_save_file", err)
	}

	if stateSeqLoadFileFunc, err = lib.Prep("llama_state_seq_load_file", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeUint64, &ffi.TypePointer); err != nil {
		return loadError("llama_state_seq_load_file", err)
	}

	if stateSeqGetSizeExtFunc, err = lib.Prep("llama_state_seq_get_size_ext", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeUint32); err != nil {
		return loadError("llama_state_seq_get_size_ext", err)
	}

	if stateSeqGetDataExtFunc, err = lib.Prep("llama_state_seq_get_data_ext", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64, &ffi.TypeSint32, &ffi.TypeUint32); err != nil {
		return loadError("llama_state_seq_get_data_ext", err)
	}

	if stateSeqSetDataExtFunc, err = lib.Prep("llama_state_seq_set_data_ext", &ffi.TypeUint64, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64, &ffi.TypeSint32, &ffi.TypeUint32); err != nil {
		return loadError("llama_state_seq_set_data_ext", err)
	}

	return err
}

// StateSaveFile saves the state to a file and returns true on success.
func StateSaveFile(ctx Context, path string, tokens []Token) bool {
	if ctx == 0 {
		return false
	}
	pathPtr, _ := utils.BytePtrFromString(path)
	var toks *Token
	if len(tokens) > 0 {
		toks = unsafe.SliceData(tokens)
	}
	tlen := int64(len(tokens))

	var result ffi.Arg
	stateSaveFileFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&pathPtr), unsafe.Pointer(&toks), &tlen)
	return result.Bool()
}

// StateLoadFile loads the state from a file and returns true on success.
// tokensOut should be a slice with capacity nTokenCapacity. nTokenCountOut will be set to the number of tokens loaded.
func StateLoadFile(ctx Context, path string, tokensOut []Token, nTokenCapacity uint64, nTokenCountOut *uint64) bool {
	if ctx == 0 {
		return false
	}
	pathPtr, _ := utils.BytePtrFromString(path)
	var toks *Token
	if len(tokensOut) > 0 {
		toks = unsafe.SliceData(tokensOut)
	}
	var result ffi.Arg
	stateLoadFileFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&pathPtr),
		unsafe.Pointer(&toks),
		&nTokenCapacity,
		unsafe.Pointer(&nTokenCountOut),
	)
	return result.Bool()
}

// StateGetSize returns the actual size in bytes of the state (logits, embedding and memory).
// Only use when saving the state, not when restoring it.
func StateGetSize(ctx Context) uint64 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	stateGetSizeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return uint64(result)
}

// StateGetData copies the state to the specified destination address.
// Returns the number of bytes copied.
func StateGetData(ctx Context, dst []byte) uint64 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	var size int64 = int64(len(dst))
	var dstPtr *byte
	if len(dst) > 0 {
		dstPtr = &dst[0]
	}
	stateGetDataFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&dstPtr), &size)
	return uint64(result)
}

// StateSetData sets the state by reading from the specified address.
// Returns the number of bytes read.
func StateSetData(ctx Context, src []byte) uint64 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	var size int64 = int64(len(src))
	var srcPtr *byte
	if len(src) > 0 {
		srcPtr = &src[0]
	}
	stateSetDataFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&srcPtr), &size)
	return uint64(result)
}

// StateSeqGetSize returns the exact size needed to copy the state of a single sequence.
func StateSeqGetSize(ctx Context, seqId SeqId) uint64 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	stateSeqGetSizeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &seqId)
	return uint64(result)
}

// StateSeqGetData copies the state of a single sequence into the specified buffer.
func StateSeqGetData(ctx Context, dst []byte, seqId SeqId) uint64 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	size := uint64(len(dst))
	var dstPtr *byte
	if len(dst) > 0 {
		dstPtr = &dst[0]
	}
	stateSeqGetDataFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&dstPtr), unsafe.Pointer(&size), &seqId)
	return uint64(result)
}

// StateSeqSetData copies the sequence data into the specified sequence.
func StateSeqSetData(ctx Context, src []byte, destSeqId SeqId) uint64 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	size := uint64(len(src))
	var srcPtr *byte
	if len(src) > 0 {
		srcPtr = &src[0]
	}
	stateSeqSetDataFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&srcPtr), &size, &destSeqId)
	return uint64(result)
}

// StateSeqSaveFile saves the state of a single sequence to a file.
func StateSeqSaveFile(ctx Context, filepath string, seqId SeqId, tokens []Token) uint64 {
	if ctx == 0 {
		return 0
	}
	pathPtr, _ := utils.BytePtrFromString(filepath)
	var toks *Token
	if len(tokens) > 0 {
		toks = unsafe.SliceData(tokens)
	}
	tlen := uint64(len(tokens))
	var result ffi.Arg
	stateSeqSaveFileFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&pathPtr), &seqId, unsafe.Pointer(&toks), &tlen)
	return uint64(result)
}

// StateSeqLoadFile loads the state of a single sequence from a file.
func StateSeqLoadFile(ctx Context, filepath string, destSeqId SeqId, tokensOut []Token, nTokenCapacity uint64, nTokenCountOut *uint64) uint64 {
	if ctx == 0 {
		return 0
	}
	pathPtr, _ := utils.BytePtrFromString(filepath)
	var toks *Token
	if len(tokensOut) > 0 {
		toks = unsafe.SliceData(tokensOut)
	}
	var result ffi.Arg
	stateSeqLoadFileFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&pathPtr),
		&destSeqId,
		unsafe.Pointer(&toks),
		&nTokenCapacity,
		unsafe.Pointer(&nTokenCountOut),
	)
	return uint64(result)
}

// StateSeqGetSizeExt returns the size needed for a sequence with flags.
func StateSeqGetSizeExt(ctx Context, seqId SeqId, flags uint32) uint64 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	stateSeqGetSizeExtFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &seqId, &flags)
	return uint64(result)
}

// StateSeqGetDataExt copies the state of a sequence with flags into the buffer.
func StateSeqGetDataExt(ctx Context, dst []byte, seqId SeqId, flags uint32) uint64 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	size := uint64(len(dst))
	var dstPtr *byte
	if len(dst) > 0 {
		dstPtr = &dst[0]
	}
	stateSeqGetDataExtFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&dstPtr), unsafe.Pointer(&size), &seqId, &flags)
	return uint64(result)
}

// StateSeqSetDataExt sets the state of a sequence with flags from the buffer.
func StateSeqSetDataExt(ctx Context, src []byte, destSeqId SeqId, flags uint32) uint64 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	size := uint64(len(src))
	var srcPtr *byte
	if len(src) > 0 {
		srcPtr = &src[0]
	}
	stateSeqSetDataExtFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&srcPtr), &size, &destSeqId, &flags)
	return uint64(result)
}

package mtmd

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/jupiterrider/ffi"
)

// Batch is an opaque handle to an mtmd_batch object.
// It is valid only for the context that created it and cannot be shared across contexts.
// Chunks added to the batch are not owned by it; they will not be freed when the batch is freed.
type Batch uintptr

// BatchAddResult is the return code from BatchAddChunk.
type BatchAddResult int32

const (
	// BatchAddSuccess indicates the chunk was added to the batch successfully.
	BatchAddSuccess BatchAddResult = 0
	// BatchAddError indicates a generic error while adding the chunk.
	BatchAddError BatchAddResult = 1
	// BatchAddTooLarge indicates the batch is already full; the chunk was not added.
	BatchAddTooLarge BatchAddResult = 2
	// BatchAddIncompatible indicates the chunk cannot be batched with the existing chunks.
	BatchAddIncompatible BatchAddResult = 3
)

var (
	// MTMD_API mtmd_batch * mtmd_batch_init(mtmd_context * ctx);
	batchInitFunc ffi.Fun

	// MTMD_API void mtmd_batch_free(mtmd_batch * batch);
	batchFreeFunc ffi.Fun

	// MTMD_API int32_t mtmd_batch_add_chunk(mtmd_batch * batch, const mtmd_input_chunk * chunk);
	// returns 0 on success
	// returns 1 on generic error
	// returns 2 if the batch is too large (chunk won't be added)
	// returns 3 if it cannot be batched with the existing chunks in the batch
	batchAddChunkFunc ffi.Fun

	// MTMD_API int32_t mtmd_batch_encode(mtmd_batch * batch);
	// returns 0 on success
	// returns 1 on generic error
	batchEncodeFunc ffi.Fun

	// MTMD_API float * mtmd_batch_get_output_embd(mtmd_batch * batch, const mtmd_input_chunk * chunk);
	batchGetOutputEmbdFunc ffi.Fun
)

func loadBatchFuncs(lib ffi.Lib) error {
	var err error

	if batchInitFunc, err = lib.Prep("mtmd_batch_init", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_batch_init", err)
	}

	if batchFreeFunc, err = lib.Prep("mtmd_batch_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("mtmd_batch_free", err)
	}

	if batchAddChunkFunc, err = lib.Prep("mtmd_batch_add_chunk", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_batch_add_chunk", err)
	}

	if batchEncodeFunc, err = lib.Prep("mtmd_batch_encode", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_batch_encode", err)
	}

	if batchGetOutputEmbdFunc, err = lib.Prep("mtmd_batch_get_output_embd", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_batch_get_output_embd", err)
	}

	return nil
}

// BatchInit creates a new Batch object for the given context.
// The batch is valid only for the given context.
// Call BatchFree when done.
func BatchInit(ctx Context) (Batch, error) {
	if ctx == 0 {
		return 0, errors.New("invalid mtmd context handle")
	}
	var batch Batch
	batchInitFunc.Call(unsafe.Pointer(&batch), unsafe.Pointer(&ctx))
	if batch == 0 {
		return 0, errors.New("mtmd_batch_init returned null")
	}
	return batch, nil
}

// BatchFree frees a Batch object previously created by BatchInit.
// Chunks added to the batch are not freed; only the batch container itself is freed.
func BatchFree(batch Batch) {
	if batch == 0 {
		return
	}
	batchFreeFunc.Call(nil, unsafe.Pointer(&batch))
}

// BatchAddChunk attempts to add a media chunk to the batch.
// Only media chunks (image/audio) are accepted; text chunks will be rejected with BatchAddError.
// The chunk is not owned by the batch and must outlive the batch encode call.
//
// Return values:
//
//	BatchAddSuccess      – chunk was added
//	BatchAddError        – generic error (e.g. text chunk passed)
//	BatchAddTooLarge     – batch is full; chunk was not added
//	BatchAddIncompatible – chunk cannot be batched with existing chunks (e.g. different size)
func BatchAddChunk(batch Batch, chunk InputChunk) BatchAddResult {
	if batch == 0 || chunk == 0 {
		return BatchAddError
	}
	var result ffi.Arg
	batchAddChunkFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&batch), unsafe.Pointer(&chunk))
	return BatchAddResult(result)
}

// BatchEncode runs the encoder on all chunks currently in the batch.
// Returns nil on success, or an error if encoding fails.
func BatchEncode(batch Batch) error {
	if batch == 0 {
		return errors.New("invalid batch handle")
	}
	var result ffi.Arg
	batchEncodeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&batch))
	if int32(result) != 0 {
		return fmt.Errorf("mtmd_batch_encode failed: %d", result)
	}
	return nil
}

// BatchGetOutputEmbd returns the output embeddings for a specific chunk after a BatchEncode call.
// embedSize must be:
//
//	llama.ModelNEmbdInp(model) * int32(InputChunkGetNTokens(chunk))
//
// The returned slice is valid until the next encode call or BatchFree.
func BatchGetOutputEmbd(batch Batch, chunk InputChunk, embedSize int32) ([]float32, error) {
	if batch == 0 {
		return nil, errors.New("invalid batch handle")
	}
	if chunk == 0 {
		return nil, errors.New("invalid chunk handle")
	}
	var embdPtr unsafe.Pointer
	batchGetOutputEmbdFunc.Call(unsafe.Pointer(&embdPtr), unsafe.Pointer(&batch), unsafe.Pointer(&chunk))
	if embdPtr == nil {
		return nil, errors.New("mtmd_batch_get_output_embd returned null pointer")
	}
	return unsafe.Slice((*float32)(embdPtr), embedSize), nil
}

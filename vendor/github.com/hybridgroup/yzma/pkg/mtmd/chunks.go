package mtmd

import (
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

// InputChunks represents the type of mtmd_input_chunks
type InputChunkType int32

const (
	InputChunkTypeText InputChunkType = iota
	InputChunkTypeImage
	InputChunkTypeAudio
)

var (
	// MTMD_API mtmd_input_chunks *      mtmd_input_chunks_init(void);
	inputChunksInitFunc ffi.Fun

	// MTMD_API void mtmd_input_chunks_free(mtmd_input_chunks * chunks);
	inputChunksFreeFunc ffi.Fun

	// MTMD_API size_t mtmd_input_chunks_size(const mtmd_input_chunks * chunks);
	inputChunksSizeFunc ffi.Fun

	// MTMD_API const mtmd_input_chunk * mtmd_input_chunks_get(const mtmd_input_chunks * chunks, size_t idx);
	inputChunksGetFunc ffi.Fun

	// MTMD_API enum mtmd_input_chunk_type mtmd_input_chunk_get_type(const mtmd_input_chunk * chunk);
	inputChunkGetTypeFunc ffi.Fun

	// MTMD_API const llama_token * mtmd_input_chunk_get_tokens_text(const mtmd_input_chunk * chunk, size_t * n_tokens_output);
	inputChunkGetTokensTextFunc ffi.Fun

	// MTMD_API const mtmd_image_tokens * mtmd_input_chunk_get_tokens_image(const mtmd_input_chunk * chunk);
	inputChunkGetTokensImageFunc ffi.Fun

	// MTMD_API size_t mtmd_input_chunk_get_n_tokens(const mtmd_input_chunk * chunk);
	inputChunkGetNTokensFunc ffi.Fun

	// MTMD_API const char * mtmd_input_chunk_get_id(const mtmd_input_chunk * chunk);
	inputChunkGetIdFunc ffi.Fun

	// MTMD_API llama_pos mtmd_input_chunk_get_n_pos(const mtmd_input_chunk * chunk);
	inputChunkGetNPosFunc ffi.Fun

	// MTMD_API mtmd_input_chunk * mtmd_input_chunk_copy(const mtmd_input_chunk * chunk);
	inputChunkCopyFunc ffi.Fun

	// MTMD_API void mtmd_input_chunk_free(mtmd_input_chunk * chunk);
	inputChunkFreeFunc ffi.Fun

	// MTMD_API size_t       mtmd_image_tokens_get_n_tokens(const mtmd_image_tokens * image_tokens); // TODO: deprecate
	inputImageTokensGetNTokensFunc ffi.Fun

	// MTMD_API size_t       mtmd_image_tokens_get_nx      (const mtmd_image_tokens * image_tokens);
	inputImageTokensGetNXFunc ffi.Fun

	// MTMD_API size_t       mtmd_image_tokens_get_ny      (const mtmd_image_tokens * image_tokens);
	inputImageTokensGetNYFunc ffi.Fun

	// MTMD_API const char * mtmd_image_tokens_get_id      (const mtmd_image_tokens * image_tokens); // TODO: deprecate
	inputImageTokensGetIdFunc ffi.Fun

	// number of temporal positions (always 1 for M-RoPE, n_tokens otherwise)
	// MTMD_API llama_pos    mtmd_image_tokens_get_n_pos   (const mtmd_image_tokens * image_tokens); // TODO: deprecate
	inputImageTokensGetNPosFunc ffi.Fun
)

func loadChunkFuncs(lib ffi.Lib) error {
	var err error

	if inputChunksInitFunc, err = lib.Prep("mtmd_input_chunks_init", &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunks_init", err)
	}

	if inputChunksFreeFunc, err = lib.Prep("mtmd_input_chunks_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunks_free", err)
	}

	if inputChunksSizeFunc, err = lib.Prep("mtmd_input_chunks_size", &ffiTypeSize, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunks_size", err)
	}

	if inputChunksGetFunc, err = lib.Prep("mtmd_input_chunks_get", &ffi.TypePointer, &ffi.TypePointer, &ffiTypeSize); err != nil {
		return loadError("mtmd_input_chunks_get", err)
	}

	if inputChunkGetTypeFunc, err = lib.Prep("mtmd_input_chunk_get_type", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_get_type", err)
	}

	if inputChunkGetTokensTextFunc, err = lib.Prep("mtmd_input_chunk_get_tokens_text", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_get_tokens_text", err)
	}

	if inputChunkGetNTokensFunc, err = lib.Prep("mtmd_input_chunk_get_n_tokens", &ffiTypeSize, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_get_n_tokens", err)
	}

	if inputChunkGetIdFunc, err = lib.Prep("mtmd_input_chunk_get_id", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_get_id", err)
	}

	if inputChunkGetNPosFunc, err = lib.Prep("mtmd_input_chunk_get_n_pos", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_get_n_pos", err)
	}

	if inputChunkCopyFunc, err = lib.Prep("mtmd_input_chunk_copy", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_copy", err)
	}

	if inputChunkFreeFunc, err = lib.Prep("mtmd_input_chunk_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_free", err)
	}

	if inputChunkGetTokensImageFunc, err = lib.Prep("mtmd_input_chunk_get_tokens_image", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_get_tokens_image", err)
	}

	if inputImageTokensGetNTokensFunc, err = lib.Prep("mtmd_image_tokens_get_n_tokens", &ffiTypeSize, &ffi.TypePointer); err != nil {
		return loadError("mtmd_image_tokens_get_n_tokens", err)
	}

	if inputImageTokensGetNXFunc, err = lib.Prep("mtmd_image_tokens_get_nx", &ffiTypeSize, &ffi.TypePointer); err != nil {
		return loadError("mtmd_image_tokens_get_nx", err)
	}

	if inputImageTokensGetNYFunc, err = lib.Prep("mtmd_image_tokens_get_ny", &ffiTypeSize, &ffi.TypePointer); err != nil {
		return loadError("mtmd_image_tokens_get_ny", err)
	}

	if inputImageTokensGetIdFunc, err = lib.Prep("mtmd_image_tokens_get_id", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_image_tokens_get_id", err)
	}

	if inputImageTokensGetNPosFunc, err = lib.Prep("mtmd_image_tokens_get_n_pos", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_image_tokens_get_n_pos", err)
	}

	return nil
}

// InputChunksInit initializes a list of InputChunk.
// It can only be populated via Tokenize().
func InputChunksInit() InputChunks {
	var chunks InputChunks
	inputChunksInitFunc.Call(unsafe.Pointer(&chunks))

	return chunks
}

// InputChunksFree frees the InputChunks.
func InputChunksFree(chunks InputChunks) {
	if chunks == 0 {
		return
	}
	inputChunksFreeFunc.Call(nil, unsafe.Pointer(&chunks))
}

// InputChunksSize returns the number of InputChunk in the list.
func InputChunksSize(chunks InputChunks) uint64 {
	if chunks == 0 {
		return 0
	}
	var result ffi.Arg
	inputChunksSizeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&chunks))

	return uint64(result)
}

// InputChunksGet retrieves the input chunk at the specified index.
func InputChunksGet(chunks InputChunks, idx uint64) InputChunk {
	var chunk InputChunk
	if chunks == 0 {
		return chunk
	}
	inputChunksGetFunc.Call(unsafe.Pointer(&chunk), unsafe.Pointer(&chunks), &idx)
	return chunk
}

// InputChunkGetType retrieves the type of the input chunk.
func InputChunkGetType(chunk InputChunk) InputChunkType {
	if chunk == 0 {
		return InputChunkType(0)
	}
	var result ffi.Arg
	inputChunkGetTypeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&chunk))
	return InputChunkType(result)
}

// InputChunkGetTokensText retrieves the text tokens of the input chunk.
func InputChunkGetTokensText(chunk InputChunk) []llama.Token {
	if chunk == 0 {
		return nil
	}
	var tokensPtr *llama.Token
	var nTokens uint64
	nt := &nTokens
	inputChunkGetTokensTextFunc.Call(unsafe.Pointer(&tokensPtr), unsafe.Pointer(&chunk), unsafe.Pointer(&nt))

	if tokensPtr == nil || nTokens == 0 {
		return nil
	}

	return unsafe.Slice(tokensPtr, int(nTokens))
}

// InputChunkGetNTokens retrieves the number of tokens in the input chunk.
func InputChunkGetNTokens(chunk InputChunk) uint64 {
	if chunk == 0 {
		return 0
	}
	var result ffi.Arg
	inputChunkGetNTokensFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&chunk))
	return uint64(result)
}

// InputChunkGetId retrieves the ID of the input chunk.
func InputChunkGetId(chunk InputChunk) string {
	if chunk == 0 {
		return ""
	}
	var idPtr *byte
	inputChunkGetIdFunc.Call(unsafe.Pointer(&idPtr), unsafe.Pointer(&chunk))

	if idPtr == nil {
		return ""
	}

	return utils.BytePtrToString(idPtr)
}

// InputChunkGetNPos retrieves the number of temporal positions in the input chunk.
func InputChunkGetNPos(chunk InputChunk) llama.Pos {
	if chunk == 0 {
		return 0
	}
	var result ffi.Arg
	inputChunkGetNPosFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&chunk))
	return llama.Pos(result)
}

// InputChunkCopy creates a copy of the input chunk.
func InputChunkCopy(chunk InputChunk) InputChunk {
	if chunk == 0 {
		return 0
	}
	var copy InputChunk
	inputChunkCopyFunc.Call(unsafe.Pointer(&copy), unsafe.Pointer(&chunk))
	return copy
}

// InputChunkFree frees the input chunk.
func InputChunkFree(chunk InputChunk) {
	if chunk == 0 {
		return
	}
	inputChunkFreeFunc.Call(nil, unsafe.Pointer(&chunk))
}

// InputChunkGetTokensImage retrieves the image tokens in the input chunk.
func InputChunkGetTokensImage(chunk InputChunk) ImageTokens {
	if chunk == 0 {
		return 0
	}
	var result ffi.Arg
	inputChunkGetTokensImageFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&chunk))
	return ImageTokens(result)
}

// ImageTokensGetNTokens returns the number of tokens in the image.
func ImageTokensGetNTokens(imageTokens ImageTokens) uint64 {
	if imageTokens == 0 {
		return 0
	}
	var result ffi.Arg
	inputImageTokensGetNTokensFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&imageTokens))
	return uint64(result)
}

// ImageTokensGetX returns the x size of the image tokens.
func ImageTokensGetNX(imageTokens ImageTokens) uint64 {
	if imageTokens == 0 {
		return 0
	}
	var result ffi.Arg
	inputImageTokensGetNXFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&imageTokens))
	return uint64(result)
}

// ImageTokensGetY returns the y size of the image tokens.
func ImageTokensGetNY(imageTokens ImageTokens) uint64 {
	if imageTokens == 0 {
		return 0
	}
	var result ffi.Arg
	inputImageTokensGetNYFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&imageTokens))
	return uint64(result)
}

// ImageTokensGetId returns the id of the image tokens.
func ImageTokensGetId(imageTokens ImageTokens) string {
	if imageTokens == 0 {
		return ""
	}
	var idPtr *byte
	inputImageTokensGetIdFunc.Call(unsafe.Pointer(&idPtr), unsafe.Pointer(&imageTokens))

	if idPtr == nil {
		return ""
	}

	return utils.BytePtrToString(idPtr)
}

// ImageTokensGetNPos returns the npos of the image tokens.
func ImageTokensGetNPos(imageTokens ImageTokens) llama.Pos {
	if imageTokens == 0 {
		return 0
	}
	var result ffi.Arg
	inputImageTokensGetNPosFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&imageTokens))
	return llama.Pos(result)
}

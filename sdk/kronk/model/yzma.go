package model

import (
	"sync"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/jupiterrider/ffi"
)

// This file contains workarounds for yzma FFI issues that haven't been
// fixed upstream. These functions wrap or replace yzma functions with
// correct FFI calling conventions.

var (
	yzmaOnce                    sync.Once
	yzmaInputChunkGetTokensText ffi.Fun
)

// InitYzmaWorkarounds loads the mtmd library and preps our fixed FFI functions.
// This is safe to call multiple times; it only initializes once.
func InitYzmaWorkarounds(libPath string) error {
	var initErr error
	yzmaOnce.Do(func() {
		lib, err := loader.LoadLibrary(libPath, "mtmd")
		if err != nil {
			initErr = err
			return
		}

		// Prep the function with correct types:
		// const llama_token * mtmd_input_chunk_get_tokens_text(const mtmd_input_chunk * chunk, size_t * n_tokens_output)
		// Returns pointer, takes pointer, takes pointer (for size_t* output)
		yzmaInputChunkGetTokensText, err = lib.Prep("mtmd_input_chunk_get_tokens_text", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer)
		if err != nil {
			initErr = err
			return
		}
	})
	return initErr
}

// InputChunkGetTokensText retrieves the text tokens from an input chunk.
// This is a fixed version of mtmd.InputChunkGetTokensText that correctly
// handles the size_t* output parameter by passing the address of a pointer.
func InputChunkGetTokensText(chunk mtmd.InputChunk) []llama.Token {
	if chunk == 0 {
		return nil
	}

	var tokensPtr *llama.Token
	var nTokens uint64
	nTokensPtr := &nTokens // C expects size_t*, FFI TypePointer needs address of pointer
	yzmaInputChunkGetTokensText.Call(unsafe.Pointer(&tokensPtr), unsafe.Pointer(&chunk), unsafe.Pointer(&nTokensPtr))

	if tokensPtr == nil || nTokens == 0 {
		return nil
	}

	return unsafe.Slice(tokensPtr, int(nTokens))
}

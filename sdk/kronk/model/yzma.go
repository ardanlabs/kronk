package model

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/jupiterrider/ffi"
)

// This file contains workarounds for yzma FFI bindings that aren't
// exposed upstream. Each section wraps a single C symbol and exposes a
// Go function that mirrors the rest of yzma's calling conventions
// (Context as uintptr, bool as Go bool passed through to a C uint8,
// returned float* as unsafe.Slice).
//
// Once a binding here has been validated end-to-end we should push it
// upstream to github.com/hybridgroup/yzma and remove the local copy.

// Pre-norm hidden-state APIs required for MTP (Multi-Token Prediction)
// speculative decoding on Qwen3.5 / Qwen3.6 (architecture qwen35).
//
// The MTP head is NOT a standalone LM — it takes two inputs per token:
//
//	(1) the token id, and
//	(2) the pre-norm hidden state from the target model at that position.
//
// llama.cpp exposes the necessary plumbing as three LLAMA_API symbols
// declared in src/llama-ext.h:
//
//	void  llama_set_embeddings_pre_norm   (llama_context*, bool value, bool masked);
//	float*llama_get_embeddings_pre_norm   (llama_context*);
//	float*llama_get_embeddings_pre_norm_ith(llama_context*, int32_t i);
//
// Reference implementation: common/speculative.cpp
// common_speculative_impl_draft_mtp constructor sets:
//
//	llama_set_embeddings_pre_norm(ctx_tgt, true, false)  // dense, all rows
//	llama_set_embeddings_pre_norm(ctx_dft, true, true)   // sparse, logit-flagged rows only
//
// and then per-iteration mirrors the target batch into ctx_dft with
// batch.embd populated from get_embeddings_pre_norm (shifted by 1 with
// pending_h slotted in at position 0).
var (
	setEmbeddingsPreNormFunc    ffi.Fun
	getEmbeddingsPreNormFunc    ffi.Fun
	getEmbeddingsPreNormIthFunc ffi.Fun
)

var (
	yzmaOnce    sync.Once
	yzmaInitErr error
)

// MTPAvailable reports whether the loaded llama library exports the
// three pre-norm hidden-state symbols required for MTP speculative
// decoding. Older llama.cpp builds (pre src/llama-ext.h pre-norm API)
// won't have them; the MTP auto-detect path checks this and skips
// silently when false, so kronk still starts up — it just runs without
// MTP speculation.
func MTPAvailable() bool {
	return setEmbeddingsPreNormFunc.Cif != nil &&
		getEmbeddingsPreNormFunc.Cif != nil &&
		getEmbeddingsPreNormIthFunc.Cif != nil
}

// InitYzmaWorkarounds loads the llama library and preps our extra FFI
// functions that yzma upstream doesn't bind yet. Safe to call multiple
// times; only the first call does any work.
//
// Pre-norm bindings are BEST-EFFORT: if the loaded llama library
// doesn't export them (older build, e.g. b9222), the corresponding
// ffi.Fun stays zero-valued and MTPAvailable() returns false. Init
// never fails on a missing pre-norm symbol so kronk still boots and
// can serve non-MTP models.
func InitYzmaWorkarounds(libPath string) error {
	yzmaOnce.Do(func() {
		lib, err := loader.LoadLibrary(libPath, "llama")
		if err != nil {
			yzmaInitErr = fmt.Errorf("load llama library: %w", err)
			return
		}

		// Try the three pre-norm hidden-state symbols. llama.cpp has
		// renamed these at least once (pre_norm → nextn in b9496+), so
		// each symbol is attempted under every name we know about,
		// first under its C linkage name (LLAMA_API / extern "C") and,
		// on miss, under the Itanium C++ ABI mangled name observed in
		// builds that compiled the declarations without LLAMA_API. The
		// mangled forms come from `nm libllama.dylib`:
		//
		//   b9222 (old, "pre_norm"):
		//     __Z29llama_set_embeddings_pre_normP13llama_contextbb
		//     __Z29llama_get_embeddings_pre_normP13llama_context
		//     __Z33llama_get_embeddings_pre_norm_ithP13llama_contexti
		//
		//   b9496 (current, "nextn"):
		//     __Z26llama_set_embeddings_nextnP13llama_contextbb
		//     __Z26llama_get_embeddings_nextnP13llama_context
		//     __Z30llama_get_embeddings_nextn_ithP13llama_contexti
		//
		// (The leading "_" is Mach-O's own underscore which dlsym
		// strips, so we pass "_Z..." to Prep.) The mangling is
		// deterministic for these signatures so this works across
		// clang/gcc on Linux + macOS arm64/x86_64. Log-and-continue
		// on miss; MTPAvailable() reflects what bound.
		prepAny := func(names []string, ret *ffi.Type, args ...*ffi.Type) (ffi.Fun, bool) {
			for _, n := range names {
				if fn, err := lib.Prep(n, ret, args...); err == nil {
					return fn, true
				}
			}
			return ffi.Fun{}, false
		}

		if fn, ok := prepAny(
			[]string{
				"llama_set_embeddings_nextn",
				"_Z26llama_set_embeddings_nextnP13llama_contextbb",
				"llama_set_embeddings_pre_norm",
				"_Z29llama_set_embeddings_pre_normP13llama_contextbb",
			},
			&ffi.TypeVoid,
			&ffi.TypePointer, // llama_context *
			&ffi.TypeUint8,   // bool value
			&ffi.TypeUint8,   // bool masked
		); ok {
			setEmbeddingsPreNormFunc = fn
		}

		if fn, ok := prepAny(
			[]string{
				"llama_get_embeddings_nextn",
				"_Z26llama_get_embeddings_nextnP13llama_context",
				"llama_get_embeddings_pre_norm",
				"_Z29llama_get_embeddings_pre_normP13llama_context",
			},
			&ffi.TypePointer, // float * return
			&ffi.TypePointer, // llama_context *
		); ok {
			getEmbeddingsPreNormFunc = fn
		}

		if fn, ok := prepAny(
			[]string{
				"llama_get_embeddings_nextn_ith",
				"_Z30llama_get_embeddings_nextn_ithP13llama_contexti",
				"llama_get_embeddings_pre_norm_ith",
				"_Z33llama_get_embeddings_pre_norm_ithP13llama_contexti",
			},
			&ffi.TypePointer, // float * return
			&ffi.TypePointer, // llama_context *
			&ffi.TypeSint32,  // int32_t i
		); ok {
			getEmbeddingsPreNormIthFunc = fn
		}
	})

	return yzmaInitErr
}

// SetEmbeddingsPreNorm enables (or disables) pre-norm hidden-state
// extraction on the given context.
//
//   - value  == true: the next llama_decode will produce a pre-norm
//     embedding buffer accessible via GetEmbeddingsPreNorm /
//     GetEmbeddingsPreNormIth.
//   - masked == false: rows are stored densely, indexed by raw batch
//     position. Used on the target context (caller wants every row).
//   - masked == true:  rows are stored only for batch positions whose
//     logits flag is non-zero, indexed via the output_ids table. Used
//     on the MTP draft context (caller only needs the output rows).
//
// Mirrors llama_set_embeddings_pre_norm in src/llama-ext.h.
func SetEmbeddingsPreNorm(ctx llama.Context, value, masked bool) {
	if ctx == 0 || setEmbeddingsPreNormFunc.Cif == nil {
		return
	}
	setEmbeddingsPreNormFunc.Call(nil, unsafe.Pointer(&ctx), &value, &masked)
}

// GetEmbeddingsPreNorm returns the dense pre-norm hidden-state buffer
// produced by the most recent llama_decode on ctx. nRows is the number
// of rows the caller expects (typically batch.NTokens for an unmasked
// context); nEmbd is the model's embedding width (llama.ModelNEmbd).
//
// Returns nil when the binding isn't loaded, the context is zero, or
// the underlying C call returned NULL (no pre-norm buffer available —
// usually means SetEmbeddingsPreNorm wasn't enabled before the decode).
//
// The returned slice aliases C-owned memory; the caller MUST NOT retain
// it past the next decode/synchronize call. Copy out rows that need to
// survive.
func GetEmbeddingsPreNorm(ctx llama.Context, nRows, nEmbd int) []float32 {
	if ctx == 0 || getEmbeddingsPreNormFunc.Cif == nil {
		return nil
	}
	var result *float32
	getEmbeddingsPreNormFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	if result == nil {
		return nil
	}
	return unsafe.Slice(result, nRows*nEmbd)
}

// GetEmbeddingsPreNormIth returns the pre-norm hidden-state row for the
// ith output of the most recent llama_decode on ctx. nEmbd is the
// model's embedding width.
//
// On a masked context (ctx_dft for MTP) i indexes through the
// output_ids table, so it must correspond to a batch position whose
// logits flag was set. On an unmasked context (ctx_tgt) i is the raw
// batch position.
//
// Returns nil when the binding isn't loaded, the context is zero, or
// the row isn't available. The returned slice aliases C-owned memory;
// don't retain past the next decode/synchronize.
func GetEmbeddingsPreNormIth(ctx llama.Context, i int32, nEmbd int) []float32 {
	if ctx == 0 || getEmbeddingsPreNormIthFunc.Cif == nil {
		return nil
	}
	var result *float32
	getEmbeddingsPreNormIthFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), &i)
	if result == nil {
		return nil
	}
	return unsafe.Slice(result, nEmbd)
}

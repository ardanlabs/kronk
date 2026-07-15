package whisper

import (
	"unsafe"

	"github.com/jupiterrider/ffi"
)

// ContextParams mirrors struct whisper_context_params from whisper.h.
//
// Layout (darwin/arm64, total 48 bytes):
//
//	bool   use_gpu;              // 0
//	bool   flash_attn;           // 1
//	[2]byte _pad                 // 2..3
//	int    gpu_device;           // 4
//	bool   dtw_token_timestamps; // 8
//	[3]byte _pad                 // 9..11
//	int    dtw_aheads_preset;    // 12
//	int    dtw_n_top;            // 16
//	[4]byte _pad                 // 20..23 (size_t aligns to 8)
//	struct whisper_aheads dtw_aheads { size_t n_heads; const whisper_ahead *heads; } // 24..40
//	size_t dtw_mem_size;         // 40..48
type ContextParams struct {
	UseGPU             uint8
	FlashAttn          uint8
	_                  [2]byte
	GPUDevice          int32
	DtwTokenTimestamps uint8
	_                  [3]byte
	DtwAheadsPreset    AlignmentHeadsPreset
	DtwNTop            int32
	_                  [4]byte
	DtwAheads          Aheads // size_t + ptr (16 bytes)
	DtwMemSize         uint64 // size_t
}

// ffiTypeContextParams describes whisper_context_params to libffi.
var ffiTypeContextParams = ffi.NewType(
	&ffi.TypeUint8,   // use_gpu
	&ffi.TypeUint8,   // flash_attn
	&ffi.TypeSint32,  // gpu_device (libffi pads to 4 automatically)
	&ffi.TypeUint8,   // dtw_token_timestamps
	&ffi.TypeSint32,  // dtw_aheads_preset (enum / int)
	&ffi.TypeSint32,  // dtw_n_top
	&ffi.TypeUint64,  // dtw_aheads.n_heads (size_t)
	&ffi.TypePointer, // dtw_aheads.heads
	&ffi.TypeUint64,  // dtw_mem_size (size_t)
)

var (
	// WHISPER_API struct whisper_context_params whisper_context_default_params(void);
	contextDefaultParamsFunc ffi.Fun

	// WHISPER_API void whisper_free(struct whisper_context * ctx);
	freeFunc ffi.Fun

	// WHISPER_API int whisper_n_len     (struct whisper_context * ctx); // mel length
	nLenFunc ffi.Fun

	// WHISPER_API int whisper_n_vocab   (struct whisper_context * ctx);
	nVocabFunc ffi.Fun

	// WHISPER_API int whisper_n_text_ctx(struct whisper_context * ctx);
	nTextCtxFunc ffi.Fun

	// WHISPER_API int whisper_n_audio_ctx(struct whisper_context * ctx);
	nAudioCtxFunc ffi.Fun

	// WHISPER_API int whisper_is_multilingual(struct whisper_context * ctx);
	isMultilingualFunc ffi.Fun
)

func loadContextFuncs(lib ffi.Lib) error {
	var err error

	if contextDefaultParamsFunc, err = lib.Prep("whisper_context_default_params", &ffiTypeContextParams); err != nil {
		return loadError("whisper_context_default_params", err)
	}

	if freeFunc, err = lib.Prep("whisper_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("whisper_free", err)
	}

	if nLenFunc, err = lib.Prep("whisper_n_len", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("whisper_n_len", err)
	}

	if nVocabFunc, err = lib.Prep("whisper_n_vocab", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("whisper_n_vocab", err)
	}

	if nTextCtxFunc, err = lib.Prep("whisper_n_text_ctx", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("whisper_n_text_ctx", err)
	}

	if nAudioCtxFunc, err = lib.Prep("whisper_n_audio_ctx", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("whisper_n_audio_ctx", err)
	}

	if isMultilingualFunc, err = lib.Prep("whisper_is_multilingual", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("whisper_is_multilingual", err)
	}

	return nil
}

// ContextDefaultParams returns the default whisper_context_params populated by
// the C library. Callers may modify fields before passing to ModelInitFromFile.
func ContextDefaultParams() ContextParams {
	var p ContextParams
	contextDefaultParamsFunc.Call(unsafe.Pointer(&p))
	return p
}

// Free releases a Context previously returned by ModelInitFromFile.
func Free(ctx Context) {
	if ctx == 0 {
		return
	}
	freeFunc.Call(nil, unsafe.Pointer(&ctx))
}

// NLen returns the mel length for the current spectrogram.
func NLen(ctx Context) int32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nLenFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return int32(result)
}

// NVocab returns the vocabulary size of the loaded model.
func NVocab(ctx Context) int32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nVocabFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return int32(result)
}

// NTextCtx returns the text context length of the loaded model.
func NTextCtx(ctx Context) int32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nTextCtxFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return int32(result)
}

// NAudioCtx returns the audio context length of the loaded model.
func NAudioCtx(ctx Context) int32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	nAudioCtxFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return int32(result)
}

// IsMultilingual reports whether the loaded model supports multiple languages.
func IsMultilingual(ctx Context) bool {
	if ctx == 0 {
		return false
	}
	var result ffi.Arg
	isMultilingualFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return int32(result) != 0
}

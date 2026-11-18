// DO NOT DELETE THIS FILE when InitYzmaWorkarounds is empty.

package model

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

// Temporary TTS dictionary and pronunciation bindings until yzma exposes these APIs:
// https://github.com/hybridgroup/yzma/issues/506
var ttsDictionaryBindings struct {
	count ffi.Fun
	value ffi.Fun
	g2p   ffi.Fun
}

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
// Call it during native initialization, before using these bindings.
func InitYzmaWorkarounds(path string) error {
	ttsDictionaryBindings.count = ffi.Fun{}
	ttsDictionaryBindings.value = ffi.Fun{}
	ttsDictionaryBindings.g2p = ffi.Fun{}
	lib, err := loader.LoadLibrary(path, "llama")
	if err != nil {
		return fmt.Errorf("loading TTS dictionary compatibility library: %w", err)
	}

	// Older bundles may omit these symbols. Only dictionary-based speech
	// protocols need them, so absence must not fail global initialization.
	if fn, err := lib.Prep("llama_model_meta_arr_n", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer); err == nil {
		ttsDictionaryBindings.count = fn
	}
	if fn, err := lib.Prep("llama_model_meta_arr_str", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err == nil {
		ttsDictionaryBindings.value = fn
	}
	mtmdLib, err := loader.LoadLibrary(path, "mtmd")
	if err != nil {
		return fmt.Errorf("loading TTS pronunciation compatibility library: %w", err)
	}
	if fn, err := mtmdLib.Prep("mtmd_gen_audio_g2p", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint64); err == nil {
		ttsDictionaryBindings.g2p = fn
	}
	return nil
}

// modelMetaArrayCount returns -1 for a missing array, matching llama.cpp. Only
// tokenizer.tts_dicts and tokenizer.tts_dict.* string arrays are exposed upstream.
func modelMetaArrayCount(mdl llama.Model, key string) (int, error) {
	if ttsDictionaryBindings.count.Cif == nil || ttsDictionaryBindings.value.Cif == nil {
		return -1, fmt.Errorf("%w: native TTS dictionary APIs unavailable", ErrSpeechUnsupported)
	}
	if mdl == 0 {
		return -1, fmt.Errorf("TTS dictionary count: nil model")
	}
	p, err := utils.BytePtrFromString(key)
	if err != nil {
		return -1, fmt.Errorf("TTS dictionary key: %w", err)
	}
	var result ffi.Arg
	ttsDictionaryBindings.count.Call(&result, &mdl, &p)
	runtime.KeepAlive(p)
	return int(int32(result)), nil
}

// modelMetaArrayString copies model-owned storage before returning. A valid
// empty element is distinct from an absent array or an out-of-range index.
func modelMetaArrayString(mdl llama.Model, key string, index int32) (string, bool) {
	if ttsDictionaryBindings.value.Cif == nil || mdl == 0 || index < 0 {
		return "", false
	}
	p, err := utils.BytePtrFromString(key)
	if err != nil {
		return "", false
	}
	var result *byte
	ttsDictionaryBindings.value.Call(unsafe.Pointer(&result), &mdl, &p, &index)
	runtime.KeepAlive(p)
	if result == nil {
		return "", false
	}
	return utils.BytePtrToString(result), true
}

// genAudioG2P copies the decoder's unknown-word pronunciation. The native API
// returns -1 for an undersized buffer and zero for an unavailable pronunciation.
func genAudioG2P(decoder mtmd.Context, word string) (string, error) {
	if ttsDictionaryBindings.g2p.Cif == nil {
		return "", fmt.Errorf("%w: native TTS pronunciation API unavailable", ErrSpeechUnsupported)
	}
	p, err := utils.BytePtrFromString(word)
	if err != nil {
		return "", fmt.Errorf("TTS pronunciation word: %w", err)
	}
	for size := uint64(256); size <= 1024*1024; size *= 2 {
		buf := make([]byte, size)
		out := unsafe.SliceData(buf)
		var result ffi.Arg
		ttsDictionaryBindings.g2p.Call(&result, &decoder, &p, &out, &size)
		runtime.KeepAlive(p)
		runtime.KeepAlive(buf)
		n := int32(result)
		if n == -1 {
			continue
		}
		if n < 0 || uint64(n) >= size {
			return "", fmt.Errorf("invalid TTS pronunciation length %d", n)
		}
		return string(buf[:n]), nil
	}
	return "", fmt.Errorf("TTS pronunciation exceeds 1 MiB")
}

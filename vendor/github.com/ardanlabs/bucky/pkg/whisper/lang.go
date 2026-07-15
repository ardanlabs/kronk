package whisper

import (
	"unsafe"

	"github.com/ardanlabs/bucky/pkg/utils"
	"github.com/jupiterrider/ffi"
)

var (
	// WHISPER_API int whisper_lang_max_id(void);
	langMaxIDFunc ffi.Fun

	// WHISPER_API int whisper_lang_id(const char * lang);
	langIDFunc ffi.Fun

	// WHISPER_API const char * whisper_lang_str(int id);
	langStrFunc ffi.Fun

	// WHISPER_API int whisper_lang_auto_detect(
	//             struct whisper_context * ctx,
	//                                int   offset_ms,
	//                                int   n_threads,
	//                              float * lang_probs);
	langAutoDetectFunc ffi.Fun

	// WHISPER_API int whisper_lang_auto_detect_with_state(
	//             struct whisper_context * ctx,
	//               struct whisper_state * state,
	//                                int   offset_ms,
	//                                int   n_threads,
	//                              float * lang_probs);
	langAutoDetectWithStateFunc ffi.Fun
)

func loadLangFuncs(lib ffi.Lib) error {
	var err error

	if langMaxIDFunc, err = lib.Prep("whisper_lang_max_id", &ffi.TypeSint32); err != nil {
		return loadError("whisper_lang_max_id", err)
	}

	if langIDFunc, err = lib.Prep("whisper_lang_id", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("whisper_lang_id", err)
	}

	if langStrFunc, err = lib.Prep("whisper_lang_str", &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_lang_str", err)
	}

	if langAutoDetectFunc, err = lib.Prep("whisper_lang_auto_detect",
		&ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypePointer,
	); err != nil {
		return loadError("whisper_lang_auto_detect", err)
	}

	if langAutoDetectWithStateFunc, err = lib.Prep("whisper_lang_auto_detect_with_state",
		&ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypePointer,
	); err != nil {
		return loadError("whisper_lang_auto_detect_with_state", err)
	}

	return nil
}

// LangMaxID returns the largest language id (i.e. number of languages - 1).
func LangMaxID() int32 {
	var result ffi.Arg
	langMaxIDFunc.Call(unsafe.Pointer(&result))
	return int32(result)
}

// LangID returns the id of the specified language code (e.g. "de" -> 2),
// or -1 if the language is unknown.
func LangID(lang string) int32 {
	cstr, err := utils.BytePtrFromString(lang)
	if err != nil {
		return -1
	}
	var result ffi.Arg
	langIDFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&cstr))
	return int32(result)
}

// LangStr returns the short language code for the given id (e.g. 2 -> "de"),
// or an empty string if the id is invalid.
func LangStr(id int32) string {
	var ptr *byte
	langStrFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&id))
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}

// LangAutoDetect attempts to identify the language of the audio at offsetMs.
// PcmToMel must have been called first (Full does this implicitly).
//
// Returns the top language id and, if probs is non-nil and pre-sized to at
// least LangMaxID()+1 entries, fills it with the probability of every
// language. Returns a negative value on failure.
func LangAutoDetect(ctx Context, offsetMs, nThreads int32, probs []float32) int32 {
	if ctx == 0 {
		return -1
	}
	var probsPtr unsafe.Pointer
	if len(probs) > 0 {
		probsPtr = unsafe.Pointer(unsafe.SliceData(probs))
	}
	var result ffi.Arg
	langAutoDetectFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&offsetMs),
		unsafe.Pointer(&nThreads),
		unsafe.Pointer(&probsPtr),
	)
	return int32(result)
}

// LangAutoDetectWithState is the explicit-state variant of LangAutoDetect.
// It reads the mel spectrogram from the supplied state rather than the
// context's default state, so it can run concurrently with other
// language-detect / transcribe calls that own their own State.
//
// FullWithState must have populated state's mel before this call.
func LangAutoDetectWithState(ctx Context, state State, offsetMs, nThreads int32, probs []float32) int32 {
	if ctx == 0 || state == 0 {
		return -1
	}
	var probsPtr unsafe.Pointer
	if len(probs) > 0 {
		probsPtr = unsafe.Pointer(unsafe.SliceData(probs))
	}
	var result ffi.Arg
	langAutoDetectWithStateFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&state),
		unsafe.Pointer(&offsetMs),
		unsafe.Pointer(&nThreads),
		unsafe.Pointer(&probsPtr),
	)
	return int32(result)
}

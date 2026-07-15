package whisper

import (
	"unsafe"

	"github.com/ardanlabs/bucky/pkg/utils"
	"github.com/jupiterrider/ffi"
)

var (
	// WHISPER_API int whisper_full_n_segments(struct whisper_context * ctx);
	fullNSegmentsFunc ffi.Fun

	// WHISPER_API const char * whisper_full_get_segment_text(struct whisper_context * ctx, int i_segment);
	fullGetSegmentTextFunc ffi.Fun

	// WHISPER_API int64_t whisper_full_get_segment_t0(struct whisper_context * ctx, int i_segment);
	fullGetSegmentT0Func ffi.Fun

	// WHISPER_API int64_t whisper_full_get_segment_t1(struct whisper_context * ctx, int i_segment);
	fullGetSegmentT1Func ffi.Fun

	// WHISPER_API bool whisper_full_get_segment_speaker_turn_next(struct whisper_context * ctx, int i_segment);
	fullGetSegmentSpeakerTurnNextFunc ffi.Fun

	// WHISPER_API float whisper_full_get_segment_no_speech_prob(struct whisper_context * ctx, int i_segment);
	fullGetSegmentNoSpeechProbFunc ffi.Fun

	// WHISPER_API int whisper_full_lang_id(struct whisper_context * ctx);
	fullLangIDFunc ffi.Fun
)

func loadSegmentsFuncs(lib ffi.Lib) error {
	var err error

	if fullNSegmentsFunc, err = lib.Prep("whisper_full_n_segments", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("whisper_full_n_segments", err)
	}

	if fullGetSegmentTextFunc, err = lib.Prep("whisper_full_get_segment_text", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_full_get_segment_text", err)
	}

	if fullGetSegmentT0Func, err = lib.Prep("whisper_full_get_segment_t0", &ffi.TypeSint64, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_full_get_segment_t0", err)
	}

	if fullGetSegmentT1Func, err = lib.Prep("whisper_full_get_segment_t1", &ffi.TypeSint64, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_full_get_segment_t1", err)
	}

	if fullGetSegmentSpeakerTurnNextFunc, err = lib.Prep("whisper_full_get_segment_speaker_turn_next", &ffi.TypeUint8, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_full_get_segment_speaker_turn_next", err)
	}

	if fullGetSegmentNoSpeechProbFunc, err = lib.Prep("whisper_full_get_segment_no_speech_prob", &ffi.TypeFloat, &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_full_get_segment_no_speech_prob", err)
	}

	if fullLangIDFunc, err = lib.Prep("whisper_full_lang_id", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("whisper_full_lang_id", err)
	}

	return nil
}

// FullNSegments returns the number of generated text segments.
func FullNSegments(ctx Context) int32 {
	if ctx == 0 {
		return 0
	}
	var result ffi.Arg
	fullNSegmentsFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return int32(result)
}

// FullGetSegmentText returns the transcribed text for the given segment index.
func FullGetSegmentText(ctx Context, iSegment int32) string {
	if ctx == 0 {
		return ""
	}
	var ptr *byte
	fullGetSegmentTextFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment))
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}

// FullGetSegmentT0 returns the start time of the segment in 10 ms units.
// Multiply by 10 to get milliseconds.
func FullGetSegmentT0(ctx Context, iSegment int32) int64 {
	if ctx == 0 {
		return 0
	}
	var result int64
	fullGetSegmentT0Func.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment))
	return result
}

// FullGetSegmentT1 returns the end time of the segment in 10 ms units.
// Multiply by 10 to get milliseconds.
func FullGetSegmentT1(ctx Context, iSegment int32) int64 {
	if ctx == 0 {
		return 0
	}
	var result int64
	fullGetSegmentT1Func.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment))
	return result
}

// FullGetSegmentSpeakerTurnNext reports whether the next segment is predicted
// to be a speaker turn (requires tdrz-enabled tinydiarize models).
func FullGetSegmentSpeakerTurnNext(ctx Context, iSegment int32) bool {
	if ctx == 0 {
		return false
	}
	var result ffi.Arg
	fullGetSegmentSpeakerTurnNextFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment))
	return result.Bool()
}

// FullGetSegmentNoSpeechProb returns the probability that the segment
// contains no speech.
func FullGetSegmentNoSpeechProb(ctx Context, iSegment int32) float32 {
	if ctx == 0 {
		return 0
	}
	var result float32
	fullGetSegmentNoSpeechProbFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&iSegment))
	return result
}

// FullLangID returns the language id detected/selected for the context's
// default state during the most recent Full call.
func FullLangID(ctx Context) int32 {
	if ctx == 0 {
		return -1
	}
	var result ffi.Arg
	fullLangIDFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return int32(result)
}

package whisper

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/ardanlabs/bucky/pkg/utils"
	"github.com/jupiterrider/ffi"
)

var (
	// WHISPER_API struct whisper_state * whisper_init_state(struct whisper_context * ctx);
	initStateFunc ffi.Fun

	// WHISPER_API void whisper_free_state(struct whisper_state * state);
	freeStateFunc ffi.Fun

	// WHISPER_API int whisper_full_with_state(
	//             struct whisper_context * ctx,
	//               struct whisper_state * state,
	//         struct whisper_full_params   params,
	//                        const float * samples,
	//                                int   n_samples);
	fullWithStateFunc ffi.Fun

	// WHISPER_API int whisper_full_n_segments_from_state(struct whisper_state * state);
	fullNSegmentsFromStateFunc ffi.Fun

	// WHISPER_API int whisper_full_lang_id_from_state(struct whisper_state * state);
	fullLangIDFromStateFunc ffi.Fun

	// WHISPER_API int64_t whisper_full_get_segment_t0_from_state(struct whisper_state * state, int i_segment);
	fullGetSegmentT0FromStateFunc ffi.Fun

	// WHISPER_API int64_t whisper_full_get_segment_t1_from_state(struct whisper_state * state, int i_segment);
	fullGetSegmentT1FromStateFunc ffi.Fun

	// WHISPER_API const char * whisper_full_get_segment_text_from_state(struct whisper_state * state, int i_segment);
	fullGetSegmentTextFromStateFunc ffi.Fun

	// WHISPER_API bool whisper_full_get_segment_speaker_turn_next_from_state(struct whisper_state * state, int i_segment);
	fullGetSegmentSpeakerTurnNextFromStateFunc ffi.Fun

	// WHISPER_API float whisper_full_get_segment_no_speech_prob_from_state(struct whisper_state * state, int i_segment);
	fullGetSegmentNoSpeechProbFromStateFunc ffi.Fun

	// WHISPER_API int whisper_full_n_tokens_from_state(struct whisper_state * state, int i_segment);
	fullNTokensFromStateFunc ffi.Fun

	// WHISPER_API const char * whisper_full_get_token_text_from_state(struct whisper_context * ctx, struct whisper_state * state, int i_segment, int i_token);
	fullGetTokenTextFromStateFunc ffi.Fun

	// WHISPER_API whisper_token whisper_full_get_token_id_from_state(struct whisper_state * state, int i_segment, int i_token);
	fullGetTokenIDFromStateFunc ffi.Fun

	// WHISPER_API whisper_token_data whisper_full_get_token_data_from_state(struct whisper_state * state, int i_segment, int i_token);
	fullGetTokenDataFromStateFunc ffi.Fun

	// WHISPER_API float whisper_full_get_token_p_from_state(struct whisper_state * state, int i_segment, int i_token);
	fullGetTokenPFromStateFunc ffi.Fun
)

func loadStateFuncs(lib ffi.Lib) error {
	// Note: whisper_full_get_token_text_from_state takes BOTH ctx and state.
	specs := []struct {
		name string
		out  *ffi.Fun
		ret  *ffi.Type
		args []*ffi.Type
	}{
		{"whisper_init_state", &initStateFunc, &ffi.TypePointer, []*ffi.Type{&ffi.TypePointer}},
		{"whisper_free_state", &freeStateFunc, &ffi.TypeVoid, []*ffi.Type{&ffi.TypePointer}},
		{"whisper_full_with_state", &fullWithStateFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer, &ffi.TypePointer, &ffiTypeFullParams, &ffi.TypePointer, &ffi.TypeSint32}},
		{"whisper_full_n_segments_from_state", &fullNSegmentsFromStateFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer}},
		{"whisper_full_lang_id_from_state", &fullLangIDFromStateFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer}},
		{"whisper_full_get_segment_t0_from_state", &fullGetSegmentT0FromStateFunc, &ffi.TypeSint64, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32}},
		{"whisper_full_get_segment_t1_from_state", &fullGetSegmentT1FromStateFunc, &ffi.TypeSint64, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32}},
		{"whisper_full_get_segment_text_from_state", &fullGetSegmentTextFromStateFunc, &ffi.TypePointer, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32}},
		{"whisper_full_get_segment_speaker_turn_next_from_state", &fullGetSegmentSpeakerTurnNextFromStateFunc, &ffi.TypeUint8, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32}},
		{"whisper_full_get_segment_no_speech_prob_from_state", &fullGetSegmentNoSpeechProbFromStateFunc, &ffi.TypeFloat, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32}},
		{"whisper_full_n_tokens_from_state", &fullNTokensFromStateFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32}},
		{"whisper_full_get_token_text_from_state", &fullGetTokenTextFromStateFunc, &ffi.TypePointer, []*ffi.Type{&ffi.TypePointer, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32}},
		{"whisper_full_get_token_id_from_state", &fullGetTokenIDFromStateFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32}},
		{"whisper_full_get_token_data_from_state", &fullGetTokenDataFromStateFunc, &ffiTypeTokenData, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32}},
		{"whisper_full_get_token_p_from_state", &fullGetTokenPFromStateFunc, &ffi.TypeFloat, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32}},
	}

	for _, s := range specs {
		fn, err := lib.Prep(s.name, s.ret, s.args...)
		if err != nil {
			return loadError(s.name, err)
		}
		*s.out = fn
	}

	return nil
}

// InitState allocates a new whisper_state for the given context. The returned
// State must be released with FreeState. Use this to run multiple parallel
// transcriptions on the same model context (each transcription needs its own
// state).
func InitState(ctx Context) (State, error) {
	if ctx == 0 {
		return 0, errors.New("whisper.InitState: nil context")
	}
	var state State
	initStateFunc.Call(unsafe.Pointer(&state), unsafe.Pointer(&ctx))
	if state == 0 {
		return 0, errors.New("whisper_init_state returned NULL")
	}
	return state, nil
}

// FreeState releases a State previously returned by InitState.
func FreeState(state State) {
	if state == 0 {
		return
	}
	freeStateFunc.Call(nil, unsafe.Pointer(&state))
}

// FullWithState runs the full whisper pipeline using the supplied state
// instead of the context's default state. Use to drive multiple concurrent
// transcriptions on the same model.
func FullWithState(ctx Context, state State, params WhisperFullParams, samples []float32) error {
	if ctx == 0 {
		return errors.New("whisper.FullWithState: nil context")
	}
	if state == 0 {
		return errors.New("whisper.FullWithState: nil state")
	}
	if len(samples) == 0 {
		return errors.New("whisper.FullWithState: empty samples")
	}

	samplesPtr := unsafe.Pointer(unsafe.SliceData(samples))
	nSamples := int32(len(samples))

	var result ffi.Arg
	fullWithStateFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&state),
		unsafe.Pointer(&params),
		unsafe.Pointer(&samplesPtr),
		unsafe.Pointer(&nSamples),
	)

	if rc := int32(result); rc != 0 {
		return fmt.Errorf("whisper_full_with_state returned %d", rc)
	}
	return nil
}

// FullNSegmentsFromState returns the number of generated text segments for
// the given state.
func FullNSegmentsFromState(state State) int32 {
	if state == 0 {
		return 0
	}
	var result ffi.Arg
	fullNSegmentsFromStateFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&state))
	return int32(result)
}

// FullLangIDFromState returns the language id detected/selected for the
// given state during the most recent FullWithState call.
func FullLangIDFromState(state State) int32 {
	if state == 0 {
		return -1
	}
	var result ffi.Arg
	fullLangIDFromStateFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&state))
	return int32(result)
}

// FullGetSegmentT0FromState returns the start time of the segment in 10 ms
// units. Multiply by 10 to get milliseconds.
func FullGetSegmentT0FromState(state State, iSegment int32) int64 {
	if state == 0 {
		return 0
	}
	var result int64
	fullGetSegmentT0FromStateFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&state), unsafe.Pointer(&iSegment))
	return result
}

// FullGetSegmentT1FromState returns the end time of the segment in 10 ms
// units. Multiply by 10 to get milliseconds.
func FullGetSegmentT1FromState(state State, iSegment int32) int64 {
	if state == 0 {
		return 0
	}
	var result int64
	fullGetSegmentT1FromStateFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&state), unsafe.Pointer(&iSegment))
	return result
}

// FullGetSegmentTextFromState returns the transcribed text for the given
// segment index in the given state.
func FullGetSegmentTextFromState(state State, iSegment int32) string {
	if state == 0 {
		return ""
	}
	var ptr *byte
	fullGetSegmentTextFromStateFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&state), unsafe.Pointer(&iSegment))
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}

// FullGetSegmentSpeakerTurnNextFromState reports whether the next segment is
// predicted to be a speaker turn (requires tdrz-enabled tinydiarize models).
func FullGetSegmentSpeakerTurnNextFromState(state State, iSegment int32) bool {
	if state == 0 {
		return false
	}
	var result ffi.Arg
	fullGetSegmentSpeakerTurnNextFromStateFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&state), unsafe.Pointer(&iSegment))
	return result.Bool()
}

// FullGetSegmentNoSpeechProbFromState returns the probability that the
// segment contains no speech.
func FullGetSegmentNoSpeechProbFromState(state State, iSegment int32) float32 {
	if state == 0 {
		return 0
	}
	var result float32
	fullGetSegmentNoSpeechProbFromStateFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&state), unsafe.Pointer(&iSegment))
	return result
}

// FullNTokensFromState returns the number of tokens generated for the
// segment in the given state.
func FullNTokensFromState(state State, iSegment int32) int32 {
	if state == 0 {
		return 0
	}
	var result ffi.Arg
	fullNTokensFromStateFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&state), unsafe.Pointer(&iSegment))
	return int32(result)
}

// FullGetTokenTextFromState returns the textual form of the iToken'th token
// in the iSegment'th segment of the given state. The C signature requires
// both ctx and state since token text comes from the model's vocabulary.
func FullGetTokenTextFromState(ctx Context, state State, iSegment, iToken int32) string {
	if ctx == 0 || state == 0 {
		return ""
	}
	var ptr *byte
	fullGetTokenTextFromStateFunc.Call(
		unsafe.Pointer(&ptr),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&state),
		unsafe.Pointer(&iSegment),
		unsafe.Pointer(&iToken),
	)
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}

// FullGetTokenIDFromState returns the vocabulary id of the iToken'th token
// in the iSegment'th segment of the given state.
func FullGetTokenIDFromState(state State, iSegment, iToken int32) Token {
	if state == 0 {
		return TokenNull
	}
	var result ffi.Arg
	fullGetTokenIDFromStateFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&state), unsafe.Pointer(&iSegment), unsafe.Pointer(&iToken))
	return Token(int32(result))
}

// FullGetTokenDataFromState returns the full TokenData for the iToken'th
// token in the iSegment'th segment of the given state.
func FullGetTokenDataFromState(state State, iSegment, iToken int32) TokenData {
	var td TokenData
	if state == 0 {
		return td
	}
	fullGetTokenDataFromStateFunc.Call(unsafe.Pointer(&td), unsafe.Pointer(&state), unsafe.Pointer(&iSegment), unsafe.Pointer(&iToken))
	return td
}

// FullGetTokenPFromState returns the probability of the iToken'th token in
// the iSegment'th segment of the given state.
func FullGetTokenPFromState(state State, iSegment, iToken int32) float32 {
	if state == 0 {
		return 0
	}
	var result float32
	fullGetTokenPFromStateFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&state), unsafe.Pointer(&iSegment), unsafe.Pointer(&iToken))
	return result
}

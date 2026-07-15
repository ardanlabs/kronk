package whisper

import (
	"unsafe"

	"github.com/jupiterrider/ffi"
)

// WhisperFullParams mirrors struct whisper_full_params from whisper.h.
//
// The C struct has many bool fields interleaved with ints/floats/pointers.
// On the LP64 / LLP64 platforms whisper.cpp targets, the C compiler applies
// natural alignment per type. Go's struct layout follows the same rules, so
// matching the field order produces a binary-compatible struct.
//
// Where C inserts implicit padding, this struct uses explicit `_padN`
// blank fields so that the Go layout exactly matches the C ABI on
// darwin/arm64, darwin/amd64, linux/amd64 and windows/amd64. A unit test
// verifies the size against what whisper_full_default_params() reports.
//
// Total size: 304 bytes on darwin/arm64.
type WhisperFullParams struct {
	Strategy SamplingStrategy // 0..4

	NThreads    int32 // 4..8
	NMaxTextCtx int32 // 8..12
	OffsetMs    int32 // 12..16
	DurationMs  int32 // 16..20

	Translate       uint8 // 20
	NoContext       uint8 // 21
	NoTimestamps    uint8 // 22
	SingleSegment   uint8 // 23
	PrintSpecial    uint8 // 24
	PrintProgress   uint8 // 25
	PrintRealtime   uint8 // 26
	PrintTimestamps uint8 // 27

	TokenTimestamps uint8 // 28
	_pad0           [3]byte
	TholdPt         float32 // 32..36
	TholdPtsum      float32 // 36..40
	MaxLen          int32   // 40..44
	SplitOnWord     uint8   // 44
	_pad1           [3]byte
	MaxTokens       int32 // 48..52

	DebugMode uint8 // 52
	_pad2     [3]byte
	AudioCtx  int32 // 56..60

	TdrzEnable    uint8 // 60
	_pad3         [3]byte
	SuppressRegex uintptr // 64..72 (const char *)

	InitialPrompt      uintptr // 72..80 (const char *)
	CarryInitialPrompt uint8   // 80
	_pad4              [7]byte
	PromptTokens       uintptr // 88..96 (const whisper_token *)
	PromptNTokens      int32   // 96..100
	_pad5              [4]byte

	Language       uintptr // 104..112 (const char *)
	DetectLanguage uint8   // 112
	SuppressBlank  uint8   // 113
	SuppressNST    uint8   // 114
	_pad6          [1]byte

	Temperature    float32 // 116..120
	MaxInitialTS   float32 // 120..124
	LengthPenalty  float32 // 124..128
	TemperatureInc float32 // 128..132
	EntropyThold   float32 // 132..136
	LogprobThold   float32 // 136..140
	NoSpeechThold  float32 // 140..144

	// nested struct { int best_of; } greedy;
	GreedyBestOf int32 // 144..148

	// nested struct { int beam_size; float patience; } beam_search;
	BeamSearchBeamSize int32   // 148..152
	BeamSearchPatience float32 // 152..156
	_pad7              [4]byte // align to ptr (8) for callback

	NewSegmentCallback           uintptr // 160..168
	NewSegmentCallbackUserData   uintptr // 168..176
	ProgressCallback             uintptr // 176..184
	ProgressCallbackUserData     uintptr // 184..192
	EncoderBeginCallback         uintptr // 192..200
	EncoderBeginCallbackUserData uintptr // 200..208
	AbortCallback                uintptr // 208..216
	AbortCallbackUserData        uintptr // 216..224
	LogitsFilterCallback         uintptr // 224..232
	LogitsFilterCallbackUserData uintptr // 232..240

	GrammarRules   uintptr // 240..248 (const whisper_grammar_element **)
	NGrammarRules  uint64  // 248..256 (size_t)
	IStartRule     uint64  // 256..264 (size_t)
	GrammarPenalty float32 // 264..268
	Vad            uint8   // 268
	_pad8          [3]byte
	VadModelPath   uintptr // 272..280 (const char *)

	// embedded whisper_vad_params
	VadThreshold            float32 // 280..284
	VadMinSpeechDurationMs  int32   // 284..288
	VadMinSilenceDurationMs int32   // 288..292
	VadMaxSpeechDurationS   float32 // 292..296
	VadSpeechPadMs          int32   // 296..300
	VadSamplesOverlap       float32 // 300..304
}

// ffiTypeFullParams describes whisper_full_params to libffi. Padding bytes
// are NOT listed; libffi computes alignment from the field types just as the
// C compiler does.
var ffiTypeFullParams = ffi.NewType(
	&ffi.TypeSint32, // strategy
	&ffi.TypeSint32, // n_threads
	&ffi.TypeSint32, // n_max_text_ctx
	&ffi.TypeSint32, // offset_ms
	&ffi.TypeSint32, // duration_ms

	&ffi.TypeUint8, // translate
	&ffi.TypeUint8, // no_context
	&ffi.TypeUint8, // no_timestamps
	&ffi.TypeUint8, // single_segment
	&ffi.TypeUint8, // print_special
	&ffi.TypeUint8, // print_progress
	&ffi.TypeUint8, // print_realtime
	&ffi.TypeUint8, // print_timestamps

	&ffi.TypeUint8,  // token_timestamps
	&ffi.TypeFloat,  // thold_pt
	&ffi.TypeFloat,  // thold_ptsum
	&ffi.TypeSint32, // max_len
	&ffi.TypeUint8,  // split_on_word
	&ffi.TypeSint32, // max_tokens

	&ffi.TypeUint8,  // debug_mode
	&ffi.TypeSint32, // audio_ctx

	&ffi.TypeUint8,   // tdrz_enable
	&ffi.TypePointer, // suppress_regex

	&ffi.TypePointer, // initial_prompt
	&ffi.TypeUint8,   // carry_initial_prompt
	&ffi.TypePointer, // prompt_tokens
	&ffi.TypeSint32,  // prompt_n_tokens

	&ffi.TypePointer, // language
	&ffi.TypeUint8,   // detect_language
	&ffi.TypeUint8,   // suppress_blank
	&ffi.TypeUint8,   // suppress_nst

	&ffi.TypeFloat, // temperature
	&ffi.TypeFloat, // max_initial_ts
	&ffi.TypeFloat, // length_penalty
	&ffi.TypeFloat, // temperature_inc
	&ffi.TypeFloat, // entropy_thold
	&ffi.TypeFloat, // logprob_thold
	&ffi.TypeFloat, // no_speech_thold

	&ffi.TypeSint32, // greedy.best_of

	&ffi.TypeSint32, // beam_search.beam_size
	&ffi.TypeFloat,  // beam_search.patience

	&ffi.TypePointer, // new_segment_callback
	&ffi.TypePointer, // new_segment_callback_user_data
	&ffi.TypePointer, // progress_callback
	&ffi.TypePointer, // progress_callback_user_data
	&ffi.TypePointer, // encoder_begin_callback
	&ffi.TypePointer, // encoder_begin_callback_user_data
	&ffi.TypePointer, // abort_callback
	&ffi.TypePointer, // abort_callback_user_data
	&ffi.TypePointer, // logits_filter_callback
	&ffi.TypePointer, // logits_filter_callback_user_data

	&ffi.TypePointer, // grammar_rules
	&ffi.TypeUint64,  // n_grammar_rules
	&ffi.TypeUint64,  // i_start_rule
	&ffi.TypeFloat,   // grammar_penalty

	&ffi.TypeUint8,   // vad
	&ffi.TypePointer, // vad_model_path

	// vad_params
	&ffi.TypeFloat,  // threshold
	&ffi.TypeSint32, // min_speech_duration_ms
	&ffi.TypeSint32, // min_silence_duration_ms
	&ffi.TypeFloat,  // max_speech_duration_s
	&ffi.TypeSint32, // speech_pad_ms
	&ffi.TypeFloat,  // samples_overlap
)

var (
	// WHISPER_API struct whisper_full_params whisper_full_default_params(enum whisper_sampling_strategy strategy);
	fullDefaultParamsFunc ffi.Fun

	// WHISPER_API struct whisper_full_params * whisper_full_default_params_by_ref(enum whisper_sampling_strategy strategy);
	fullDefaultParamsByRefFunc ffi.Fun

	// WHISPER_API void whisper_free_params(struct whisper_full_params * params);
	freeParamsFunc ffi.Fun
)

func loadParamsFuncs(lib ffi.Lib) error {
	var err error

	if fullDefaultParamsFunc, err = lib.Prep("whisper_full_default_params", &ffiTypeFullParams, &ffi.TypeSint32); err != nil {
		return loadError("whisper_full_default_params", err)
	}

	if fullDefaultParamsByRefFunc, err = lib.Prep("whisper_full_default_params_by_ref", &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_full_default_params_by_ref", err)
	}

	if freeParamsFunc, err = lib.Prep("whisper_free_params", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("whisper_free_params", err)
	}

	return nil
}

// FullDefaultParams returns the default WhisperFullParams populated by the C
// library for the given sampling strategy. Callers may modify fields before
// passing to Full.
func FullDefaultParams(strategy SamplingStrategy) WhisperFullParams {
	var p WhisperFullParams
	s := int32(strategy)
	fullDefaultParamsFunc.Call(unsafe.Pointer(&p), unsafe.Pointer(&s))
	return p
}

// FullDefaultParamsByRef returns the C-allocated default whisper_full_params
// pointer via the upstream by-ref entry point. The returned pointer is owned
// by whisper.cpp and must be released with FreeParams. Useful for verifying
// the struct layout.
func FullDefaultParamsByRef(strategy SamplingStrategy) *WhisperFullParams {
	var ptr *WhisperFullParams
	s := int32(strategy)
	fullDefaultParamsByRefFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&s))
	return ptr
}

// FreeParams releases a WhisperFullParams pointer previously returned by
// FullDefaultParamsByRef.
func FreeParams(p *WhisperFullParams) {
	if p == nil {
		return
	}
	freeParamsFunc.Call(nil, unsafe.Pointer(&p))
}

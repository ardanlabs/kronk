// Package whisper provides Go FFI bindings to whisper.cpp using purego and
// jupiterrider/ffi. It mirrors the public C API exposed by whisper.h as a
// thin layer; higher-level ergonomics live in cmd/ or downstream consumers.
package whisper

// Common types matching whisper.cpp.
type (
	Pos    int32
	Token  int32
	SeqID  int32
	Memory uintptr
)

// Audio constants from whisper.h.
const (
	// SampleRate is the sample rate expected by whisper models (16 kHz).
	SampleRate = 16000

	// NFFT is the FFT window size used by the mel spectrogram.
	NFFT = 400

	// HopLength is the hop length used by the mel spectrogram.
	HopLength = 160

	// ChunkSize is the audio chunk size in seconds (30s).
	ChunkSize = 30

	// TokenNull marks an invalid token.
	TokenNull = -1
)

// SamplingStrategy mirrors enum whisper_sampling_strategy.
type SamplingStrategy int32

const (
	SamplingGreedy     SamplingStrategy = 0
	SamplingBeamSearch SamplingStrategy = 1
)

// AlignmentHeadsPreset mirrors enum whisper_alignment_heads_preset.
type AlignmentHeadsPreset int32

const (
	AHeadsNone         AlignmentHeadsPreset = 0
	AHeadsNTopMost     AlignmentHeadsPreset = 1
	AHeadsCustom       AlignmentHeadsPreset = 2
	AHeadsTinyEN       AlignmentHeadsPreset = 3
	AHeadsTiny         AlignmentHeadsPreset = 4
	AHeadsBaseEN       AlignmentHeadsPreset = 5
	AHeadsBase         AlignmentHeadsPreset = 6
	AHeadsSmallEN      AlignmentHeadsPreset = 7
	AHeadsSmall        AlignmentHeadsPreset = 8
	AHeadsMediumEN     AlignmentHeadsPreset = 9
	AHeadsMedium       AlignmentHeadsPreset = 10
	AHeadsLargeV1      AlignmentHeadsPreset = 11
	AHeadsLargeV2      AlignmentHeadsPreset = 12
	AHeadsLargeV3      AlignmentHeadsPreset = 13
	AHeadsLargeV3Turbo AlignmentHeadsPreset = 14
)

// GretType mirrors enum whisper_gretype (grammar element type).
type GretType int32

const (
	GretypeEnd          GretType = 0
	GretypeAlt          GretType = 1
	GretypeRuleRef      GretType = 2
	GretypeChar         GretType = 3
	GretypeCharNot      GretType = 4
	GretypeCharRngUpper GretType = 5
	GretypeCharAlt      GretType = 6
)

// Opaque handles. These are pointers in C; in Go we carry them as uintptr
// so they round-trip through the FFI boundary without retainability issues.
//
// # Concurrency
//
// A Context is NOT safe for concurrent inference calls. Upstream
// whisper.h says verbatim:
//
//	The following interface is thread-safe as long as the same
//	whisper_context is not used by multiple threads concurrently.
//
// Specifically, Full delegates directly to whisper_full(ctx, ...), which
// the upstream header marks "Not thread safe for same context". The
// reason is that whisper_context embeds a single default whisper_state
// pointer, and every call writes through it: mel spectrogram buffer,
// self / cross / pad KV caches, decoder hypotheses, logit buffer,
// accumulated result segments, rolling prompt history, and timing
// counters. There is no internal lock.
//
// To run transcriptions in parallel, allocate one Context per goroutine
// via InitFromFileWithParams. The model weights are reloaded per Context,
// so the memory cost scales linearly — plan for it.
//
// (The model weights and vocabulary themselves are read-only after init
// and could in principle be shared across goroutines via a per-goroutine
// State allocated with InitState plus FullWithState; that's the pattern
// whisper_full_parallel uses internally. The Go bindings expose those
// primitives, but the supported concurrency story for this package is
// "one Context per goroutine".)
//
// # Read-only accessors
//
// Metadata accessors (NVocab, NLen, NAudioCtx, NTextCtx, IsMultilingual,
// the token-to-string helpers, and the Model* dimension queries) read
// only from the immutable-after-init model and vocab. They are safe to
// call from any goroutine concurrently with each other, but must NOT
// race with Free.
type (
	Context    uintptr
	State      uintptr
	VadContext uintptr
)

// Ahead mirrors struct whisper_ahead.
type Ahead struct {
	NTextLayer int32
	NHead      int32
}

// Aheads mirrors struct whisper_aheads.
type Aheads struct {
	NHeads uint64 // size_t
	Heads  *Ahead
}

// TokenData mirrors struct whisper_token_data.
type TokenData struct {
	ID    Token
	Tid   Token
	P     float32
	Plog  float32
	Pt    float32
	Ptsum float32
	T0    int64
	T1    int64
	TDtw  int64
	Vlen  float32
	_     [4]byte // C trailing padding to 8-byte alignment
}

// VadParams mirrors struct whisper_vad_params (also embedded inside
// WhisperFullParams).
type VadParams struct {
	Threshold            float32
	MinSpeechDurationMs  int32
	MinSilenceDurationMs int32
	MaxSpeechDurationS   float32
	SpeechPadMs          int32
	SamplesOverlap       float32
}

// VadContextParams mirrors struct whisper_vad_context_params.
type VadContextParams struct {
	NThreads  int32
	UseGPU    uint8
	_         [3]byte // pad before GPUDevice (4-byte align)
	GPUDevice int32
}

// GrammarElement mirrors struct whisper_grammar_element.
type GrammarElement struct {
	Type  GretType
	Value uint32
}

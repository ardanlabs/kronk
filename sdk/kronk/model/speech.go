package model

import "errors"

// ErrSpeechUnsupported indicates that a detected speech model cannot be served
// by the current SDK/native runtime or is missing its required decoder.
var ErrSpeechUnsupported = errors.New("speech generation is not supported")

// SpeechConfig configures speech generation limits. It does not select the
// model's runtime; speech models are identified from their model bundle.
// The initial runtime uses 2,048 context tokens per sequence, 512-token logical
// and physical batches, and partitioned F16 KV cache. Incompatible overrides,
// IMC, speculative decoding, and adapters are rejected during model loading.
type SpeechConfig struct {
	// MaxFrames limits generated speech frames. Zero uses 512. The initial
	// supported range is 1 through 512. Frames are model-specific units, not
	// samples or seconds. Exhausting the limit fails the request rather than
	// returning successful truncated audio.
	MaxFrames int
}

// SpeechRequest supplies text and an optional preset voice for speech generation.
type SpeechRequest struct {
	Input string // Text to speak.
	Voice string // Preset voice name; empty selects the model's default voice.
}

// SpeechResponse contains a complete PCM16 little-endian mono WAV. Audio is
// caller-owned and remains valid after subsequent requests or model unload.
type SpeechResponse struct {
	Audio      []byte // Complete WAV file, including its header.
	SampleRate int32  // Samples per second.
	Samples    int64  // Audio sample count, excluding the WAV header.
}

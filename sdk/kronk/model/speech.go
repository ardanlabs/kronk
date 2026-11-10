package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
)

// ErrSpeechUnsupported indicates that a detected speech model cannot be served
// by the current SDK/native runtime or is missing its required decoder.
var ErrSpeechUnsupported = errors.New("speech generation is not supported")

// ErrSpeechLimit indicates that speech did not reach EOS within MaxFrames.
// No partial audio is returned.
var ErrSpeechLimit = errors.New("speech frame limit reached")

// SpeechConfig configures speech generation limits. It does not select the
// model's runtime; speech models are identified from their model bundle.
// The initial runtime uses 2,048 context tokens per sequence, 512-token logical
// and physical batches, and partitioned F16 KV cache. Incompatible overrides,
// IMC, speculative decoding, and adapters are rejected during model loading.
// NSeqMax controls overlapping requests (default one); native execution remains
// serialized. Chat sampling settings do not configure speech generation.
type SpeechConfig struct {
	// MaxFrames limits generated speech frames. Zero uses 512. The initial
	// supported range is 1 through 512. Frames are model-specific units, not
	// samples or seconds. Exhausting the limit fails the request rather than
	// returning successful truncated audio.
	MaxFrames int
}

// SpeechRequest supplies text and an optional preset voice for speech generation.
// Input must be nonblank UTF-8 without NUL bytes, at most 64 KiB, and fit the
// context window together with voice conditioning and the full frame budget.
// Empty Voice or "default" uses default model conditioning; other names must
// be presets in the loaded decoder. A bundle need not contain a named preset.
// Language selection, reference audio, and voice cloning are not exposed.
type SpeechRequest struct {
	Input string // Text to speak.
	Voice string // Preset voice name; empty or "default" uses default conditioning.
}

// SpeechResponse contains a complete PCM16 little-endian mono WAV. Audio is
// caller-owned and remains valid after subsequent requests or model unload.
type SpeechResponse struct {
	Audio      []byte // Complete WAV file, including its header.
	SampleRate int32  // Samples per second.
	Samples    int64  // Audio sample count, excluding the WAV header.
}

// Speech generates one complete utterance. Calls may overlap across NSeqMax
// slots; native inference is serialized. Cancellation is checked between native
// steps and after vocoding, and returns only after sequence cleanup completes.
func (m *Model) Speech(ctx context.Context, req SpeechRequest) (SpeechResponse, error) {
	if !m.modelInfo.IsSpeechModel {
		return SpeechResponse{}, fmt.Errorf("speech: model %q: %w", m.modelInfo.ID, ErrSpeechUnsupported)
	}

	if strings.TrimSpace(req.Input) == "" || len(req.Input) > 64*1024 || !utf8.ValidString(req.Input) || strings.ContainsRune(req.Input, 0) {
		return SpeechResponse{}, fmt.Errorf("%w: speech input must be nonblank UTF-8 without NUL bytes, at most 64 KiB", ErrInvalidRequest)
	}

	if !utf8.ValidString(req.Voice) || strings.ContainsRune(req.Voice, 0) {
		return SpeechResponse{}, fmt.Errorf("%w: invalid speech voice name", ErrInvalidRequest)
	}

	if err := m.beginActiveOperation(); err != nil {
		return SpeechResponse{}, fmt.Errorf("speech: %w", err)
	}

	defer m.endActiveOperation()

	ctx, span := otel.AddSpan(ctx, "speech")
	defer span.End()

	started := time.Now()

	metrics.AddInferenceActiveRequests(m.modelInfo.ID, "speech", "speech", 1)

	result := m.speech.run(ctx, req)
	metrics.AddInferenceActiveRequests(m.modelInfo.ID, "speech", "speech", -1)
	status := "ok"

	if result.err != nil {
		status = lifecycleStatus(result.err)
	}

	metrics.ObserveInferenceRequest(m.modelInfo.ID, "speech", "speech", status, time.Since(started), result.promptTokens)

	return result.response, result.err
}

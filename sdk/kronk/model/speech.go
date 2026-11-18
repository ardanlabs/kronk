package model

import (
	"context"
	"encoding/binary"
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

// SpeechReference supplies an optional speaker recording. Its zero value means
// no reference. Audio must be a PCM16 little-endian mono WAV at 8–48 kHz,
// no longer than 30 seconds and at most 8 MiB including container metadata.
// Text is an optional nonblank UTF-8 transcript of Audio, at most 64 KiB;
// it is supported only by Qwen3-TTS. A decoder with a reference encoder uses
// the transcript for in-context conditioning; older decoders fall back to
// speaker-embedding conditioning. The caller must not modify Audio until
// Speech returns. Use recordings you own or have permission to use.
type SpeechReference struct {
	Audio []byte // Encoded speaker recording, not a filename or base64.
	Text  string // Optional transcript of Audio, not the new text to generate.
}

// SpeechRequest supplies text and optional conditioning for speech generation.
// Input must be nonblank UTF-8 without NUL bytes, at most 64 KiB, and fit the
// context window together with voice conditioning and the full frame budget.
// Empty Voice or "default" uses default model conditioning; other names must
// be presets in the loaded decoder. A bundle need not contain a named preset.
// Reference audio replaces default conditioning and cannot be combined with a
// named preset. Language is supported only by Qwen3-TTS; empty uses English.
type SpeechRequest struct {
	Input     string          // Text to speak.
	Voice     string          // Preset voice name; empty or "default" uses default conditioning.
	Language  string          // Qwen language name or two-letter code; empty uses English.
	Reference SpeechReference // Optional; the zero value preserves default/preset conditioning.
}

// SpeechResponse contains a complete PCM16 little-endian WAV. Audio is
// caller-owned and remains valid after subsequent requests or model unload.
// Currently supported speech models return mono audio (Stereo is false).
type SpeechResponse struct {
	Audio      []byte // Complete WAV file, including its header.
	SampleRate int    // Samples per second per channel.
	Samples    int    // Sample count per channel, excluding the WAV header.
	Stereo     bool   // False is mono; true is stereo.
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

	if err := validateSpeechReference(req.Reference); err != nil {
		return SpeechResponse{}, err
	}
	if len(req.Reference.Audio) > 0 && req.Voice != "" && req.Voice != "default" {
		return SpeechResponse{}, fmt.Errorf("%w: speech reference and named voice are mutually exclusive", ErrInvalidRequest)
	}

	if err := m.beginActiveOperation(); err != nil {
		return SpeechResponse{}, fmt.Errorf("speech: %w", err)
	}

	defer m.endActiveOperation()

	language, err := m.speech.adapter.Validate(req.Language, req.Reference.Text)
	if err != nil {
		return SpeechResponse{}, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	req.Language = language

	ctx, span := otel.AddSpan(ctx, "speech")
	defer span.End()

	started := time.Now()

	metrics.AddInferenceActiveRequests(m.modelInfo.ID, "speech", "speech", 1)

	response, promptTokens, err := m.speech.run(ctx, req)
	metrics.AddInferenceActiveRequests(m.modelInfo.ID, "speech", "speech", -1)
	status := "ok"

	if err != nil {
		status = lifecycleStatus(err)
	}

	metrics.ObserveInferenceRequest(m.modelInfo.ID, "speech", "speech", status, time.Since(started), promptTokens)

	return response, err
}

// validateSpeechReference bounds decoded PCM before native media decoding. Walk
// RIFF chunks instead of assuming the canonical 44-byte header: recordings may
// contain additional metadata and padded chunks.
func validateSpeechReference(ref SpeechReference) error {
	if ref.Text != "" && (len(ref.Audio) == 0 || strings.TrimSpace(ref.Text) == "" || len(ref.Text) > 64*1024 ||
		!utf8.ValidString(ref.Text) || strings.ContainsRune(ref.Text, 0)) {
		return fmt.Errorf("%w: speech reference transcript requires audio and nonblank UTF-8 without NUL bytes, at most 64 KiB", ErrInvalidRequest)
	}
	if len(ref.Audio) == 0 {
		return nil
	}
	b := ref.Audio
	if len(b) < 12 || len(b) > 8*1024*1024 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" ||
		uint64(binary.LittleEndian.Uint32(b[4:8]))+8 != uint64(len(b)) {
		return fmt.Errorf("%w: speech reference must be a complete WAV file at most 8 MiB", ErrInvalidRequest)
	}
	var rate, samples uint32
	var haveFormat, haveData bool
	for offset := 12; offset < len(b); {
		if len(b)-offset < 8 {
			return fmt.Errorf("%w: truncated speech reference chunk", ErrInvalidRequest)
		}
		size := int(binary.LittleEndian.Uint32(b[offset+4 : offset+8]))
		start := offset + 8
		if size > len(b)-start || size+size%2 > len(b)-start {
			return fmt.Errorf("%w: truncated speech reference chunk", ErrInvalidRequest)
		}
		chunk := b[start : start+size]
		switch string(b[offset : offset+4]) {
		case "fmt ":
			if haveFormat || size < 16 || binary.LittleEndian.Uint16(chunk[:2]) != 1 ||
				binary.LittleEndian.Uint16(chunk[2:4]) != 1 || binary.LittleEndian.Uint16(chunk[14:16]) != 16 {
				return fmt.Errorf("%w: speech reference requires PCM16 mono WAV", ErrInvalidRequest)
			}
			rate = binary.LittleEndian.Uint32(chunk[4:8])
			if rate < 8000 || rate > 48000 || binary.LittleEndian.Uint32(chunk[8:12]) != rate*2 || binary.LittleEndian.Uint16(chunk[12:14]) != 2 {
				return fmt.Errorf("%w: speech reference requires a valid 8–48 kHz sample rate and PCM alignment", ErrInvalidRequest)
			}
			haveFormat = true
		case "data":
			if haveData || size == 0 || size%2 != 0 {
				return fmt.Errorf("%w: invalid speech reference PCM data", ErrInvalidRequest)
			}
			samples, haveData = uint32(size/2), true
		}
		offset = start + size + size%2
	}
	if !haveFormat || !haveData || samples > 30*rate {
		return fmt.Errorf("%w: speech reference must contain between one sample and 30 seconds of PCM audio", ErrInvalidRequest)
	}
	return nil
}

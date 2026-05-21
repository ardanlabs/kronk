package bucky

import (
	"context"
	"fmt"
	"strings"

	"github.com/ardanlabs/bucky/pkg/whisper"
)

// Segment is one decoded segment from a Transcribe call.
type Segment struct {
	Index        int32
	StartMs      int64
	EndMs        int64
	Text         string
	NoSpeechProb float32
}

// Transcription is the full result of a Transcribe call. Text is the
// concatenation of Segment.Text trimmed of leading and trailing
// whitespace. Language is the language code whisper.cpp detected (or
// the hint that was passed in).
type Transcription struct {
	Text     string
	Language string
	Segments []Segment
}

// =============================================================================

// TranscribeConfig captures the per-call settings Transcribe consults.
// Defaults match the whisper.cpp greedy-sampling profile with
// progress / realtime printing disabled.
type TranscribeConfig struct {
	// Language is the BCP-47 / ISO 639-1 language hint (e.g. "en",
	// "de"). When empty whisper.cpp auto-detects.
	Language string

	// InitialPrompt biases the decoder with prior context.
	InitialPrompt string

	// Translate, when true, translates the source audio to English.
	Translate bool

	// NThreads overrides Config.NThreads for this call when > 0.
	NThreads int32

	// BeamSize, when > 0, switches the sampler to beam search with
	// the specified beam size. Defaults to greedy.
	BeamSize int32

	// NoTimestamps suppresses per-segment t0/t1 emission in the
	// returned text. Segment-level timestamps remain available on
	// each Segment.
	NoTimestamps bool

	// OnSegment, when non-nil, is invoked once per decoded segment
	// after Full returns. The callback is synchronous and runs on the
	// caller's goroutine.
	OnSegment func(Segment)
}

// TranscribeOption is a functional option for TranscribeConfig.
type TranscribeOption func(*TranscribeConfig)

// WithLanguage sets the language hint.
func WithLanguage(v string) TranscribeOption {
	return func(c *TranscribeConfig) { c.Language = v }
}

// WithInitialPrompt sets the decoder bias prompt.
func WithInitialPrompt(v string) TranscribeOption {
	return func(c *TranscribeConfig) { c.InitialPrompt = v }
}

// WithTranslate enables source-to-English translation.
func WithTranslate(v bool) TranscribeOption {
	return func(c *TranscribeConfig) { c.Translate = v }
}

// WithTranscribeNThreads overrides Config.NThreads for this call.
func WithTranscribeNThreads(v int32) TranscribeOption {
	return func(c *TranscribeConfig) { c.NThreads = v }
}

// WithBeamSize switches the sampler to beam search of the specified
// size.
func WithBeamSize(v int32) TranscribeOption {
	return func(c *TranscribeConfig) { c.BeamSize = v }
}

// WithNoTimestamps disables timestamp emission in the rendered text
// output.
func WithNoTimestamps(v bool) TranscribeOption {
	return func(c *TranscribeConfig) { c.NoTimestamps = v }
}

// WithOnSegment registers a callback invoked once per decoded
// segment after Full returns.
func WithOnSegment(fn func(Segment)) TranscribeOption {
	return func(c *TranscribeConfig) { c.OnSegment = fn }
}

// =============================================================================

// Transcribe runs the whisper.cpp pipeline on the provided 16 kHz
// mono float32 PCM samples and returns the decoded text along with
// per-segment metadata. The call blocks for the duration of the
// underlying whisper_full invocation.
func (w *Whisper) Transcribe(ctx context.Context, samples []float32, opts ...TranscribeOption) (Transcription, error) {
	if len(samples) == 0 {
		return Transcription{}, fmt.Errorf("transcribe: empty samples")
	}

	handle, err := w.acquire(ctx)
	if err != nil {
		return Transcription{}, fmt.Errorf("transcribe: %w", err)
	}
	defer w.release()

	var tcfg TranscribeConfig
	for _, opt := range opts {
		opt(&tcfg)
	}

	params, refs, err := w.buildFullParams(tcfg)
	if err != nil {
		return Transcription{}, fmt.Errorf("transcribe: %w", err)
	}
	defer refs.KeepAlive()

	if err := whisper.Full(handle, params, samples); err != nil {
		return Transcription{}, fmt.Errorf("transcribe: %w", err)
	}

	return collectTranscription(handle, tcfg.OnSegment), nil
}

// =============================================================================

func (w *Whisper) buildFullParams(tcfg TranscribeConfig) (whisper.WhisperFullParams, whisper.StringRefs, error) {
	strategy := whisper.SamplingGreedy
	if tcfg.BeamSize > 0 {
		strategy = whisper.SamplingBeamSearch
	}

	params := whisper.FullDefaultParams(strategy)

	switch {
	case tcfg.NThreads > 0:
		params.NThreads = tcfg.NThreads
	case w.cfg.NThreads > 0:
		params.NThreads = w.cfg.NThreads
	}

	if tcfg.BeamSize > 0 {
		params.BeamSearchBeamSize = tcfg.BeamSize
	}
	if tcfg.Translate {
		params.Translate = 1
	}
	if tcfg.NoTimestamps {
		params.NoTimestamps = 1
	}

	params.PrintProgress = 0
	params.PrintRealtime = 0
	params.PrintTimestamps = 0

	var refs whisper.StringRefs
	if err := refs.SetLanguage(&params, tcfg.Language); err != nil {
		return whisper.WhisperFullParams{}, whisper.StringRefs{}, fmt.Errorf("build-params: language: %w", err)
	}
	if err := refs.SetInitialPrompt(&params, tcfg.InitialPrompt); err != nil {
		return whisper.WhisperFullParams{}, whisper.StringRefs{}, fmt.Errorf("build-params: initial-prompt: %w", err)
	}

	return params, refs, nil
}

func collectTranscription(handle whisper.Context, onSegment func(Segment)) Transcription {
	n := whisper.FullNSegments(handle)

	out := Transcription{
		Segments: make([]Segment, 0, n),
		Language: whisper.LangStr(whisper.FullLangID(handle)),
	}

	var sb strings.Builder
	for i := range n {
		seg := Segment{
			Index:        i,
			StartMs:      whisper.FullGetSegmentT0(handle, i) * 10,
			EndMs:        whisper.FullGetSegmentT1(handle, i) * 10,
			Text:         whisper.FullGetSegmentText(handle, i),
			NoSpeechProb: whisper.FullGetSegmentNoSpeechProb(handle, i),
		}
		out.Segments = append(out.Segments, seg)
		sb.WriteString(seg.Text)

		if onSegment != nil {
			onSegment(seg)
		}
	}

	out.Text = strings.TrimSpace(sb.String())
	return out
}

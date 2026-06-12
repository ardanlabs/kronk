package model

import (
	"context"
	"fmt"
	"io"
	"sort"
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
// the hint that was passed in). Duration is the length of the
// transcribed audio in seconds.
type Transcription struct {
	Text     string
	Language string
	Duration float64
	Segments []Segment
}

// =============================================================================

// ChannelTranscription is the transcription of a single source channel
// in a channel-separated (diarized) result. Channel is the zero-based
// source channel index (0 = left, 1 = right for stereo).
type ChannelTranscription struct {
	Channel int
	Transcription
}

// DiarizedSegment is one decoded segment tagged with the source channel
// it came from, so a merged multi-channel transcript can attribute each
// segment to a speaker.
type DiarizedSegment struct {
	Channel int
	Segment
}

// Diarization is the result of a channel-separated transcribe. Channels
// holds one transcription per non-empty source channel; Segments merges
// every channel's segments into a single list sorted by start time, each
// tagged with the channel (speaker) it came from.
type Diarization struct {
	Channels []ChannelTranscription
	Segments []DiarizedSegment
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

	// PromptTokens seeds the decoder with token ids harvested from a
	// prior decode, carrying linguistic context across windows. Used by
	// the streaming worker for cross-window continuity; empty for batch
	// transcription.
	PromptTokens []whisper.Token

	// Translate, when true, translates the source audio to English.
	Translate bool

	// NThreads overrides Config.NThreads for this call when > 0.
	NThreads int32

	// BeamSize, when > 0, switches the sampler to beam search with
	// the specified beam size. Defaults to greedy.
	BeamSize int32

	// NoTimestamps suppresses per-segment t0/t1 emission in the
	// rendered text output. Segment-level timestamps remain available
	// on each Segment value.
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
// per-segment metadata.
//
// Transcribe acquires a whisper.State from the model's internal
// pool, so up to Config.NSeqMax goroutines may run Transcribe in
// parallel against the same Model. The acquired state is released
// back to the pool when Transcribe returns.
func (m *Model) Transcribe(ctx context.Context, samples []float32, opts ...TranscribeOption) (Transcription, error) {
	if m.handle == 0 {
		return Transcription{}, fmt.Errorf("transcribe: model has been unloaded")
	}
	if len(samples) == 0 {
		return Transcription{}, fmt.Errorf("transcribe: empty samples")
	}

	var tcfg TranscribeConfig
	for _, opt := range opts {
		opt(&tcfg)
	}

	params, refs, err := m.buildFullParams(tcfg)
	if err != nil {
		return Transcription{}, fmt.Errorf("transcribe: %w", err)
	}
	defer refs.KeepAlive()

	ps, err := m.pool.acquire(ctx)
	if err != nil {
		return Transcription{}, fmt.Errorf("transcribe: %w", err)
	}
	defer m.pool.release(ps)

	if err := whisper.FullWithState(m.handle, ps.state, params, samples); err != nil {
		return Transcription{}, fmt.Errorf("transcribe: %w", err)
	}

	tr := collectTranscription(ps.state, tcfg.Language, tcfg.OnSegment)
	tr.Duration = float64(len(samples)) / float64(whisper.SampleRate)
	return tr, nil
}

// TranscribeFile is a convenience wrapper that decodes audio from r
// via Decode and then runs Transcribe on the resulting samples. It is
// intended for HTTP handlers and CLI callers that have an io.Reader
// (form upload, file on disk) rather than pre-decoded PCM.
func (m *Model) TranscribeFile(ctx context.Context, r io.Reader, opts ...TranscribeOption) (Transcription, error) {
	samples, err := Decode(ctx, r)
	if err != nil {
		return Transcription{}, fmt.Errorf("transcribe-file: %w", err)
	}
	return m.Transcribe(ctx, samples, opts...)
}

// TranscribeChannels transcribes each supplied channel of 16 kHz mono
// float32 PCM separately and merges the results into a diarized
// transcript. Each channel is treated as one speaker, the common layout
// for call-center and meeting recordings where every participant has a
// dedicated channel. Pass the slices returned by DecodeChannels.
//
// Channels are transcribed sequentially. Each channel acquires and
// releases a whisper.State from the model's internal pool, so other
// goroutines may still run Transcribe against the same Model between
// channels. Empty channels are skipped.
func (m *Model) TranscribeChannels(ctx context.Context, channels [][]float32, opts ...TranscribeOption) (Diarization, error) {
	if m.handle == 0 {
		return Diarization{}, fmt.Errorf("transcribe-channels: model has been unloaded")
	}
	if len(channels) == 0 {
		return Diarization{}, fmt.Errorf("transcribe-channels: no channels")
	}

	var d Diarization
	for ch, samples := range channels {
		if len(samples) == 0 {
			continue
		}

		tr, err := m.Transcribe(ctx, samples, opts...)
		if err != nil {
			return Diarization{}, fmt.Errorf("transcribe-channels: channel[%d]: %w", ch, err)
		}

		d.Channels = append(d.Channels, ChannelTranscription{Channel: ch, Transcription: tr})
		for _, seg := range tr.Segments {
			d.Segments = append(d.Segments, DiarizedSegment{Channel: ch, Segment: seg})
		}
	}

	if len(d.Channels) == 0 {
		return Diarization{}, fmt.Errorf("transcribe-channels: all channels empty")
	}

	sort.SliceStable(d.Segments, func(i, j int) bool {
		return d.Segments[i].StartMs < d.Segments[j].StartMs
	})

	return d, nil
}

// TranscribeChannelsFile decodes audio from r into one 16 kHz mono
// float32 channel per source channel (via DecodeChannels) and then runs
// TranscribeChannels. Native multi-channel formats (WAV, FLAC) yield
// true per-channel diarization; formats that require ffmpeg are
// downmixed to a single channel before transcription.
func (m *Model) TranscribeChannelsFile(ctx context.Context, r io.Reader, opts ...TranscribeOption) (Diarization, error) {
	channels, err := DecodeChannels(ctx, r)
	if err != nil {
		return Diarization{}, fmt.Errorf("transcribe-channels-file: %w", err)
	}
	return m.TranscribeChannels(ctx, channels, opts...)
}

// =============================================================================

func (m *Model) buildFullParams(tcfg TranscribeConfig) (whisper.WhisperFullParams, whisper.StringRefs, error) {
	strategy := whisper.SamplingGreedy
	if tcfg.BeamSize > 0 {
		strategy = whisper.SamplingBeamSearch
	}

	params := whisper.FullDefaultParams(strategy)

	switch {
	case tcfg.NThreads > 0:
		params.NThreads = tcfg.NThreads
	case m.cfg.NThreads > 0:
		params.NThreads = m.cfg.NThreads
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
	refs.SetPromptTokens(&params, tcfg.PromptTokens)

	return params, refs, nil
}

func collectTranscription(state whisper.State, requestedLang string, onSegment func(Segment)) Transcription {
	n := whisper.FullNSegmentsFromState(state)

	out := Transcription{
		Segments: make([]Segment, 0, n),
		Language: resolveLanguage(state, requestedLang),
	}

	var sb strings.Builder
	for i := range n {
		seg := Segment{
			Index:        i,
			StartMs:      whisper.FullGetSegmentT0FromState(state, i) * 10,
			EndMs:        whisper.FullGetSegmentT1FromState(state, i) * 10,
			Text:         whisper.FullGetSegmentTextFromState(state, i),
			NoSpeechProb: whisper.FullGetSegmentNoSpeechProbFromState(state, i),
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

// resolveLanguage reports the language for a transcription. When the
// caller pinned an explicit language, that is exactly what whisper.cpp
// used, so it is reported verbatim. This also avoids a whisper.cpp
// quirk: when a fixed language is supplied to a non-multilingual model,
// whisper_full_with_state never writes the per-state lang_id, so a
// pooled whisper.State reused after a DetectLanguage pass (which sets
// detect_language=1 and writes the detected id) would otherwise report
// that prior pass's stale language. Only when auto-detecting (empty or
// "auto") does whisper populate the state, so the detected id is read
// back in that case.
func resolveLanguage(state whisper.State, requestedLang string) string {
	if r := strings.ToLower(strings.TrimSpace(requestedLang)); r != "" && r != "auto" {
		return r
	}
	return whisper.LangStr(whisper.FullLangIDFromState(state))
}

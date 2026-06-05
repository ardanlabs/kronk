package model

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by the streaming API stubs until the
// worker, ring buffer, and VAD paths are implemented. It exists so
// downstream callers can compile and wire against the surface before
// the implementation lands.
var ErrNotImplemented = errors.New("not implemented")

// =============================================================================
// Events

// EventKind classifies a transcript Event delivered on a Stream's channel.
type EventKind int

const (
	// EventPartial is tentative text for the current utterance. A later
	// Partial or Final may revise it. Partials may be dropped under load.
	EventPartial EventKind = iota

	// EventFinal is utterance-aligned, authoritative text. Never revised,
	// never dropped.
	EventFinal

	// EventReset is emitted after Reset completes, only when the stream
	// was opened WithEmitResetEvent. Text and Segments are empty.
	EventReset

	// EventError is terminal. Err is set; the Events channel closes next.
	EventError
)

// Event is one item delivered on a Stream's Events channel. Text holds
// the text for THIS event only (a delta), not the running transcript.
type Event struct {
	Kind     EventKind
	Text     string
	StartMs  int64 // session-local; rebases to 0 after Reset by default
	EndMs    int64
	Segments []Segment
	Err      error // non-nil only when Kind == EventError
}

// =============================================================================
// Stream configuration

// StreamConfig captures the per-stream settings NewStream consults.
// Defaults are applied for any zero-value field (see the per-field
// comments).
type StreamConfig struct {
	// Language is the BCP-47 / ISO 639-1 language hint. When empty
	// whisper.cpp auto-detects once at the start of the session.
	Language string

	// InitialPrompt biases the decoder on the first window only.
	InitialPrompt string

	// Translate, when true, translates the source audio to English.
	Translate bool

	// NThreads overrides Config.NThreads for this session when > 0.
	NThreads int32

	// PartialEveryMs is the partial-emit cadence. 0 = 1000;
	// <0 disables partials (final-only mode).
	PartialEveryMs int

	// WindowMs is the analysis window length. 0 = 6000.
	WindowMs int

	// KeepMs is the trailing audio kept across windows. 0 = 300.
	KeepMs int

	// MaxUtteranceMs force-flushes a Final if no VAD silence is seen.
	// 0 = 25000.
	MaxUtteranceMs int

	// BufferCapMs is the internal buffer capacity. 0 = 60000.
	BufferCapMs int

	// UseVAD gates Final emission on detected silence. Default true;
	// requires a VAD model loaded on the Model.
	UseVAD bool

	// VADModelPath overrides Config.VADModelPath for this session.
	VADModelPath string

	// EmitResetEvent, when true, emits an EventReset after Reset.
	// Default false. Useful when a different goroutine consumes Events
	// and keeps partial-text accumulators it must clear at a boundary.
	EmitResetEvent bool
}

// StreamOption is a functional option for StreamConfig.
type StreamOption func(*StreamConfig)

// WithStreamLanguage sets the language hint for the session.
func WithStreamLanguage(v string) StreamOption {
	return func(c *StreamConfig) { c.Language = v }
}

// WithStreamInitialPrompt biases the first window's decode.
func WithStreamInitialPrompt(v string) StreamOption {
	return func(c *StreamConfig) { c.InitialPrompt = v }
}

// WithStreamTranslate enables source-to-English translation.
func WithStreamTranslate(v bool) StreamOption {
	return func(c *StreamConfig) { c.Translate = v }
}

// WithStreamNThreads overrides Config.NThreads for this session.
func WithStreamNThreads(v int32) StreamOption {
	return func(c *StreamConfig) { c.NThreads = v }
}

// WithPartialEveryMs sets the partial-emit cadence. <0 disables partials.
func WithPartialEveryMs(v int) StreamOption {
	return func(c *StreamConfig) { c.PartialEveryMs = v }
}

// WithWindowMs sets the analysis window length.
func WithWindowMs(v int) StreamOption {
	return func(c *StreamConfig) { c.WindowMs = v }
}

// WithKeepMs sets the trailing audio kept across windows.
func WithKeepMs(v int) StreamOption {
	return func(c *StreamConfig) { c.KeepMs = v }
}

// WithMaxUtteranceMs sets the forced-cut ceiling for a single utterance.
func WithMaxUtteranceMs(v int) StreamOption {
	return func(c *StreamConfig) { c.MaxUtteranceMs = v }
}

// WithBufferCapMs sets the internal buffer capacity.
func WithBufferCapMs(v int) StreamOption {
	return func(c *StreamConfig) { c.BufferCapMs = v }
}

// WithVAD toggles VAD-gated Final emission.
func WithVAD(v bool) StreamOption {
	return func(c *StreamConfig) { c.UseVAD = v }
}

// WithVADModelPath overrides Config.VADModelPath for this session.
func WithVADModelPath(v string) StreamOption {
	return func(c *StreamConfig) { c.VADModelPath = v }
}

// WithEmitResetEvent makes Reset emit an EventReset on the channel.
func WithEmitResetEvent(v bool) StreamOption {
	return func(c *StreamConfig) { c.EmitResetEvent = v }
}

// =============================================================================
// Reset configuration

// ResetConfig tunes Reset behavior. All fields are optional and have
// the defaults documented per field.
type ResetConfig struct {
	// FlushPending, when true (default), runs one final transcribe over
	// any audio still in the buffer and emits the resulting Final
	// event(s) before clearing.
	FlushPending bool

	// KeepPromptTokens, when true, preserves the rolling prompt-token
	// history across the reset. Default false (hard session boundary).
	KeepPromptTokens bool

	// RebaseTimestamps, when true (default), restarts StartMs/EndMs from
	// zero on subsequent events.
	RebaseTimestamps bool
}

// ResetOption is a functional option for ResetConfig.
type ResetOption func(*ResetConfig)

// WithFlushPending controls whether Reset runs a final pass before clearing.
func WithFlushPending(v bool) ResetOption {
	return func(c *ResetConfig) { c.FlushPending = v }
}

// WithKeepPromptTokens preserves rolling prompt tokens across the reset.
func WithKeepPromptTokens(v bool) ResetOption {
	return func(c *ResetConfig) { c.KeepPromptTokens = v }
}

// WithRebaseTimestamps controls whether event timestamps restart at 0.
func WithRebaseTimestamps(v bool) ResetOption {
	return func(c *ResetConfig) { c.RebaseTimestamps = v }
}

// =============================================================================
// Stream

// Stream is a long-lived transcription session. It borrows one
// whisper.State from the model's pool for its entire lifetime and emits
// transcript Events incrementally as audio is fed in. A Stream is
// reusable indefinitely via Reset. Close must be called exactly once
// when done.
//
// Feed is producer-side (single goroutine). Events is consumer-side.
// Reset and Close are safe to call from any goroutine; both serialize
// internally with the worker.
type Stream struct {
	cfg StreamConfig

	// TODO(impl): borrowed whisper.State, bounded input channel, worker
	// goroutine, events channel, prompt-token slice, VAD state machine.
}

// NewStream opens a streaming transcription session against the loaded
// model. It reserves one whisper.State from the pool for the lifetime
// of the stream. Caller must call Close exactly once.
func (m *Model) NewStream(ctx context.Context, opts ...StreamOption) (*Stream, error) {
	var cfg StreamConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	s := Stream{
		cfg: cfg,
	}

	// TODO(impl): acquire whisper.State from the pool, allocate the
	// bounded input channel, start the worker goroutine.
	_ = ctx

	return &s, ErrNotImplemented
}

// Feed pushes 16 kHz mono float32 PCM into the stream. It blocks,
// respecting ctx, when the internal buffer is full, back-pressuring a
// fast producer. Returns ctx.Err() on cancellation.
func (s *Stream) Feed(ctx context.Context, samples []float32) error {
	_, _ = ctx, samples

	// TODO(impl): ctx-aware send into the bounded input channel.
	return ErrNotImplemented
}

// Events returns the channel of transcript events. It is closed after
// Close finishes its final flush, or after an EventError. Range over it.
func (s *Stream) Events() <-chan Event {
	// TODO(impl): return the worker's events channel.
	return nil
}

// Reset clears the audio buffer and rolling linguistic context so the
// same Stream can begin a fresh logical session WITHOUT releasing its
// pool slot or worker. The whisper.State, pool slot, VAD context,
// Events channel, and ActiveStreams count all survive. Reset is O(1)
// and allocation-free.
//
// Behavior is tunable via ResetOption.
func (s *Stream) Reset(ctx context.Context, opts ...ResetOption) error {
	cfg := ResetConfig{
		FlushPending:     true,
		RebaseTimestamps: true,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	_ = ctx

	// TODO(impl): quiesce worker, optional flush, clear buffer +
	// prompts, rebase timestamps, optionally emit EventReset, resume.
	return ErrNotImplemented
}

// Close performs one final flush over remaining audio, emits the
// resulting Final event(s), closes Events, and returns the
// whisper.State to the pool. Idempotent.
func (s *Stream) Close() error {
	// TODO(impl): drain, final transcribe pass, close events, release
	// the pool slot.
	return ErrNotImplemented
}

package model

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ardanlabs/bucky/pkg/audio"
	"github.com/ardanlabs/bucky/pkg/whisper"
)

// Stream defaults, applied for any zero-value StreamConfig field.
const (
	defaultPartialEveryMs = 1000
	defaultKeepMs         = 300
	defaultCommitEveryMs  = 6000
	defaultMaxUtteranceMs = 25000
	defaultVADThreshold   = 0.6
)

// Worker tuning constants.
const (
	heartbeatInterval = 100 * time.Millisecond // worker poll cadence
	vadTailMs         = 700                    // trailing window measured for silence
	minNewAudioMs     = 200                    // skip a partial re-decode below this much new audio
	speechFloor       = 1e-4                   // mean energy below this is treated as no speech
	inChanBatches     = 64                     // Feed backpressure: queued batches before Feed blocks
	eventsBuffer      = 16                     // Events channel capacity
	maxPromptTokens   = 64                     // tail tokens carried as cross-window prompt context
)

// =============================================================================
// Events

// EventKind classifies a transcript Event delivered on a Stream's channel.
type EventKind int

const (
	// EventPartial is a tentative, revisable re-decode of the current
	// un-committed audio window. Its Text is the COMPLETE hypothesis for
	// that window (not a delta): each Partial SUPERSEDES the previous one,
	// so a consumer should REPLACE its pending-text buffer with Text. This
	// is what produces the "words change as you keep talking" rewrite
	// effect seen in whisper.cpp's stream example. Partials may be dropped
	// under load.
	EventPartial EventKind = iota

	// EventFinal commits the current un-committed region: it will not be
	// revised. Its Text is the authoritative text for that region. A
	// consumer should APPEND Text to its permanent transcript and clear
	// its pending-partial buffer. Finals are never dropped.
	EventFinal

	// EventReset is emitted after Reset completes, only when the stream
	// was opened WithEmitResetEvent. Text and Segments are empty.
	EventReset

	// EventError is terminal. Err is set; the Events channel closes next.
	EventError
)

// Event is one item delivered on a Stream's Events channel.
//
// Text semantics depend on Kind (see EventPartial / EventFinal): a Partial
// carries the full replaceable hypothesis for the un-committed window, a
// Final carries the committed text to append. Consumers render live by
// replacing on Partial and committing on Final.
type Event struct {
	Kind     EventKind
	Text     string
	StartMs  int64 // session-local; rebases to 0 after Reset by default
	EndMs    int64
	Segments []Segment
	Err      error // non-nil only when Kind == EventError
}

// =============================================================================
// Raw PCM input

// SampleEncoding identifies how raw PCM samples are laid out in bytes.
// Only little-endian encodings are listed because every mainstream
// capture stack (CoreAudio, WASAPI, ALSA/PulseAudio/PipeWire, Web
// Audio) emits little-endian on x86 and ARM.
type SampleEncoding int

const (
	// Int16LE is signed 16-bit little-endian PCM (2 bytes/sample). The
	// most universal microphone format. Normalized as v / 32768.
	Int16LE SampleEncoding = iota

	// Float32LE is 32-bit IEEE-754 little-endian PCM (4 bytes/sample),
	// already in the [-1, 1] range. The native CoreAudio / Web Audio
	// format.
	Float32LE
)

// AudioFormat describes the raw PCM a producer delivers to FeedPCM. The
// engine always converts to its fixed internal format: 16 kHz, mono,
// float32 in [-1, 1].
type AudioFormat struct {
	// SampleRate is the producer's sample rate in Hz, e.g. 48000. When
	// it differs from 16000 the input is resampled.
	SampleRate int

	// Channels is the number of interleaved channels, e.g. 2 for
	// stereo (L,R,L,R,...). More than one channel is downmixed to mono.
	Channels int

	// Sample is the byte layout of each sample.
	Sample SampleEncoding
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

	// KeepMs is the trailing audio kept across commits so a word on the
	// boundary is not clipped. 0 = 300.
	KeepMs int

	// CommitEveryMs force-commits a Final on a fixed cadence even when no
	// silence is detected (e.g. continuous speech, or VAD disabled), so
	// Finals roll out steadily instead of waiting for MaxUtteranceMs.
	// Mirrors whisper.cpp stream's n_new_line cadence. 0 = 6000.
	CommitEveryMs int

	// MaxUtteranceMs force-flushes a Final if no silence is detected.
	// 0 = 25000.
	MaxUtteranceMs int

	// DisableVAD turns OFF the silence-gated commit. By default (zero
	// value) the stream uses a pure-Go energy-ratio voice-activity
	// detector (the same approach as whisper.cpp stream's vad_simple — no
	// model file, no extra inference) to commit a Final when the speaker
	// pauses, so cuts land in natural gaps instead of mid-word. Set it
	// true (via WithVAD(false)) to fall back to fixed-cadence commits on
	// CommitEveryMs / MaxUtteranceMs / Close only. The field is named for
	// the non-default state, matching the http.Transport.DisableKeepAlives
	// idiom, so the default-on behavior needs no sentinel.
	DisableVAD bool

	// VADThreshold is the energy-ratio threshold for the silence detector:
	// the trailing window is treated as silence when its mean energy falls
	// below VADThreshold * the whole window's mean energy. Lower = less
	// eager to cut. 0 = 0.6.
	VADThreshold float32

	// EmitResetEvent, when true, emits an EventReset after Reset.
	// Default false. Useful when a different goroutine consumes Events
	// and keeps partial-text accumulators it must clear at a boundary.
	EmitResetEvent bool
}

// withDefaults returns a copy of c with zero-value fields replaced by
// their defaults. VAD needs no entry here: its zero value (DisableVAD ==
// false) already means "VAD on", which is the default.
func (c StreamConfig) withDefaults() StreamConfig {
	if c.PartialEveryMs == 0 {
		c.PartialEveryMs = defaultPartialEveryMs
	}
	if c.KeepMs <= 0 {
		c.KeepMs = defaultKeepMs
	}
	if c.CommitEveryMs <= 0 {
		c.CommitEveryMs = defaultCommitEveryMs
	}
	if c.MaxUtteranceMs <= 0 {
		c.MaxUtteranceMs = defaultMaxUtteranceMs
	}
	if c.VADThreshold <= 0 {
		c.VADThreshold = defaultVADThreshold
	}
	return c
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

// WithKeepMs sets the trailing audio kept across commits.
func WithKeepMs(v int) StreamOption {
	return func(c *StreamConfig) { c.KeepMs = v }
}

// WithCommitEveryMs sets the fixed cadence at which a Final is committed
// when no silence is detected.
func WithCommitEveryMs(v int) StreamOption {
	return func(c *StreamConfig) { c.CommitEveryMs = v }
}

// WithMaxUtteranceMs sets the forced-cut ceiling for a single utterance.
func WithMaxUtteranceMs(v int) StreamOption {
	return func(c *StreamConfig) { c.MaxUtteranceMs = v }
}

// WithVAD toggles the energy-ratio silence detector that gates Finals.
// VAD is ON by default; pass false to fall back to fixed-cadence commits.
func WithVAD(v bool) StreamOption {
	return func(c *StreamConfig) { c.DisableVAD = !v }
}

// WithVADThreshold sets the energy-ratio threshold for the silence
// detector.
func WithVADThreshold(v float32) StreamOption {
	return func(c *StreamConfig) { c.VADThreshold = v }
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

	// RebaseTimestamps, when true (default), restarts StartMs/EndMs from
	// zero on subsequent events.
	RebaseTimestamps bool

	// KeepPromptTokens, when true, preserves the rolling prompt-token
	// history across the reset so the next window keeps linguistic
	// continuity ("rewind the audio buffer but keep context"). Default
	// false, which treats Reset as a hard session boundary (e.g. a topic
	// change) and clears the context.
	KeepPromptTokens bool
}

// ResetOption is a functional option for ResetConfig.
type ResetOption func(*ResetConfig)

// WithFlushPending controls whether Reset runs a final pass before clearing.
func WithFlushPending(v bool) ResetOption {
	return func(c *ResetConfig) { c.FlushPending = v }
}

// WithRebaseTimestamps controls whether event timestamps restart at 0.
func WithRebaseTimestamps(v bool) ResetOption {
	return func(c *ResetConfig) { c.RebaseTimestamps = v }
}

// WithKeepPromptTokens controls whether Reset preserves the rolling
// prompt-token context (true) or clears it as a hard boundary (false,
// the default).
func WithKeepPromptTokens(v bool) ResetOption {
	return func(c *ResetConfig) { c.KeepPromptTokens = v }
}

// =============================================================================
// Stream

// transcribeFunc decodes one window of 16 kHz mono float32 audio. It is
// the seam between the streaming worker and whisper.cpp: NewStream wires
// a closure that calls FullWithState, while tests inject a fake so the
// worker can be exercised without a loaded model. firstWindow reports
// whether InitialPrompt should still apply; prompt carries the tail
// tokens harvested from the previous committed window for cross-window
// continuity. It returns the transcript plus the tail tokens for the
// NEXT window's prompt (the worker only retains these after a commit).
type transcribeFunc func(samples []float32, firstWindow bool, prompt []whisper.Token) (Transcription, []whisper.Token, error)

type resetReq struct {
	cfg  ResetConfig
	done chan struct{}
}

// Stream is a long-lived transcription session. It borrows one
// whisper.State from the model's pool for its entire lifetime and emits
// transcript Events incrementally as audio is fed in. A Stream is
// reusable indefinitely via Reset. Close must be called exactly once
// when done.
//
// Feed/FeedPCM are producer-side (single goroutine). Consumers must
// range over Events() until it is closed. Reset and Close are safe to
// call from any goroutine; both serialize with the worker.
type Stream struct {
	cfg     StreamConfig
	decode  transcribeFunc
	release func()

	inC    chan []float32
	events chan Event
	resetC chan resetReq
	closeC chan struct{}
	doneC  chan struct{}

	closeOnce sync.Once

	// Worker-owned; only touched on the run goroutine.
	buf          []float32
	lastDecode   int
	baseMs       int64
	firstWindow  bool
	lastPartial  time.Time
	promptTokens []whisper.Token

	// Producer-owned; only touched by Feed/FeedPCM.
	resampler *audio.Resampler
	inRate    int
	pcmRem    []byte
}

// NewStream opens a streaming transcription session against the loaded
// model. It reserves one whisper.State from the pool for the lifetime
// of the stream; onClose, when non-nil, runs when the stream's worker
// exits (used by the bucky wrapper to release its backpressure slot).
// Caller must call Close exactly once.
func (m *Model) NewStream(ctx context.Context, onClose func(), opts ...StreamOption) (*Stream, error) {
	if m.handle == 0 {
		return nil, fmt.Errorf("new-stream: model has been unloaded")
	}

	var cfg StreamConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	cfg = cfg.withDefaults()

	ps, err := m.pool.acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("new-stream: %w", err)
	}

	decode := func(samples []float32, firstWindow bool, prompt []whisper.Token) (Transcription, []whisper.Token, error) {
		tcfg := TranscribeConfig{
			Language:     cfg.Language,
			Translate:    cfg.Translate,
			NThreads:     cfg.NThreads,
			PromptTokens: prompt,
		}
		if firstWindow {
			tcfg.InitialPrompt = cfg.InitialPrompt
		}

		params, refs, err := m.buildFullParams(tcfg)
		if err != nil {
			return Transcription{}, nil, err
		}
		defer refs.KeepAlive()

		if err := whisper.FullWithState(m.handle, ps.state, params, samples); err != nil {
			return Transcription{}, nil, err
		}

		return collectTranscription(ps.state, nil), harvestPromptTokens(ps.state, maxPromptTokens), nil
	}

	release := func() {
		m.pool.release(ps)
		if onClose != nil {
			onClose()
		}
	}

	return newStream(cfg, decode, release), nil
}

// newStream builds a Stream around a decode seam and starts its worker.
// It never fails, so it is the unit-test entry point: tests pass a fake
// decode and a no-op release.
func newStream(cfg StreamConfig, decode transcribeFunc, release func()) *Stream {
	s := Stream{
		cfg:         cfg,
		decode:      decode,
		release:     release,
		inC:         make(chan []float32, inChanBatches),
		events:      make(chan Event, eventsBuffer),
		resetC:      make(chan resetReq),
		closeC:      make(chan struct{}),
		doneC:       make(chan struct{}),
		firstWindow: true,
	}

	go s.run()

	return &s
}

// Feed pushes 16 kHz mono float32 PCM into the stream. It blocks,
// respecting ctx, when the internal buffer is full, back-pressuring a
// fast producer. Returns ctx.Err() on cancellation. The samples slice
// may be reused by the caller after Feed returns.
//
// This is the zero-conversion fast path: callers that already hold
// normalized 16 kHz mono float32 (browser Web Audio, CoreAudio) feed
// it directly. Callers with raw microphone PCM should use FeedPCM.
func (s *Stream) Feed(ctx context.Context, samples []float32) error {
	if len(samples) == 0 {
		return nil
	}

	cp := make([]float32, len(samples))
	copy(cp, samples)

	select {
	case s.inC <- cp:
		return nil
	case <-s.closeC:
		return fmt.Errorf("feed: stream closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FeedPCM is the raw-microphone adapter for Feed. A capture device never
// hands you 16 kHz mono float32; it hands you raw interleaved PCM at the
// hardware rate (commonly 48 kHz) as int16 or float32, in arbitrary-size
// blocks. FeedPCM interprets, downmixes, resamples, and normalizes raw
// into the 16 kHz mono float32 the engine consumes, then funnels it into
// the same buffer Feed writes to.
//
// All conversion is pure Go with NO ffmpeg or other subprocess, reusing
// the helpers already in github.com/ardanlabs/bucky/pkg/audio. The
// pipeline is:
//
//  1. interpret  raw bytes -> []float32 per f.Sample (int16 -> v/32768,
//     or reinterpret little-endian float32);
//  2. downmix    f.Channels interleaved -> mono via audio.DownmixToMono;
//  3. resample   f.SampleRate -> 16000 via a per-stream audio.Resampler,
//     the stateful streaming resampler that carries fractional phase
//     across calls so block seams introduce no discontinuity or drift
//     (linear interpolation, no anti-alias filter — adequate for
//     whisper, which is trained on 16 kHz);
//  4. normalize  clamp to [-1, 1].
//
// raw need not contain a whole number of frames; a partial trailing
// frame is buffered until the next call so block boundaries that fall
// mid-frame do not corrupt the stream.
func (s *Stream) FeedPCM(ctx context.Context, raw []byte, f AudioFormat) error {
	return s.Feed(ctx, s.convert(raw, f))
}

// convert performs the pure-Go FeedPCM pipeline. It is producer-owned
// state (resampler, partial-frame remainder) and must only be called
// from the single producer goroutine.
func (s *Stream) convert(raw []byte, f AudioFormat) []float32 {
	if f.Channels < 1 {
		f.Channels = 1
	}

	if len(s.pcmRem) > 0 {
		combined := make([]byte, 0, len(s.pcmRem)+len(raw))
		combined = append(combined, s.pcmRem...)
		combined = append(combined, raw...)
		raw = combined
		s.pcmRem = s.pcmRem[:0]
	}

	bytesPerSample := 2
	if f.Sample == Float32LE {
		bytesPerSample = 4
	}

	frameBytes := bytesPerSample * f.Channels
	full := (len(raw) / frameBytes) * frameBytes
	if rem := len(raw) - full; rem > 0 {
		s.pcmRem = append(s.pcmRem[:0], raw[full:]...)
		raw = raw[:full]
	}
	if len(raw) == 0 {
		return nil
	}

	nSamp := len(raw) / bytesPerSample
	inter := make([]float32, nSamp)
	switch f.Sample {
	case Float32LE:
		for i := range nSamp {
			inter[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
		}
	default:
		for i := range nSamp {
			inter[i] = float32(int16(binary.LittleEndian.Uint16(raw[i*2:]))) / 32768
		}
	}

	mono := audio.DownmixToMono(inter, f.Channels)

	if f.SampleRate > 0 && f.SampleRate != whisper.SampleRate {
		if s.resampler == nil || s.inRate != f.SampleRate {
			s.resampler = audio.NewResampler(f.SampleRate, whisper.SampleRate)
			s.inRate = f.SampleRate
		}
		mono = s.resampler.Process(mono)
	}

	for i, v := range mono {
		switch {
		case v > 1:
			mono[i] = 1
		case v < -1:
			mono[i] = -1
		}
	}

	return mono
}

// Events returns the channel of transcript events. It is closed after
// Close finishes its final flush, or after an EventError. Range over it.
func (s *Stream) Events() <-chan Event {
	return s.events
}

// Reset clears the audio buffer and rolling linguistic context so the
// same Stream can begin a fresh logical session WITHOUT releasing its
// pool slot or worker. The whisper.State, pool slot, Events channel, and
// ActiveStreams count all survive; the energy-VAD state is cleared.
//
// Behavior is tunable via ResetOption. Reset blocks until any in-flight
// decode finishes and the worker has applied the reset.
func (s *Stream) Reset(ctx context.Context, opts ...ResetOption) error {
	rc := ResetConfig{
		FlushPending:     true,
		RebaseTimestamps: true,
	}
	for _, opt := range opts {
		opt(&rc)
	}

	done := make(chan struct{})

	select {
	case s.resetC <- resetReq{cfg: rc, done: done}:
	case <-s.closeC:
		return fmt.Errorf("reset: stream closed")
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close performs one final flush over remaining audio, emits the
// resulting Final event, closes Events, and returns the whisper.State to
// the pool. It is idempotent and blocks until the worker has exited.
func (s *Stream) Close() error {
	s.closeOnce.Do(func() {
		close(s.closeC)
		<-s.doneC
	})

	return nil
}

// =============================================================================
// Worker

func (s *Stream) run() {
	defer close(s.doneC)
	defer s.release()
	defer close(s.events)

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case samples := <-s.inC:
			s.buf = append(s.buf, samples...)

		case rr := <-s.resetC:
			if err := s.doReset(rr.cfg); err != nil {
				close(rr.done)
				s.events <- Event{Kind: EventError, Err: err}
				return
			}
			close(rr.done)

		case <-heartbeat.C:
			if err := s.process(); err != nil {
				s.events <- Event{Kind: EventError, Err: err}
				return
			}

		case <-s.closeC:
			s.drainInput()
			if err := s.finalFlush(); err != nil {
				s.events <- Event{Kind: EventError, Err: err}
			}
			return
		}
	}
}

// drainInput moves any queued Feed batches into the buffer without
// blocking, so the final flush sees all fed audio.
func (s *Stream) drainInput() {
	for {
		select {
		case samples := <-s.inC:
			s.buf = append(s.buf, samples...)
		default:
			return
		}
	}
}

// process runs once per heartbeat: it commits a Final when a boundary is
// reached (silence / cadence / hard cap) or otherwise emits a Partial on
// the configured cadence.
func (s *Stream) process() error {
	if len(s.buf) == 0 {
		return nil
	}

	switch {
	case len(s.buf) >= samplesForMs(s.cfg.MaxUtteranceMs):
		return s.commit()
	case !s.cfg.DisableVAD && detectSilenceEnergy(s.buf, vadTailMs, s.cfg.VADThreshold):
		return s.commit()
	case len(s.buf) >= samplesForMs(s.cfg.CommitEveryMs):
		return s.commit()
	}

	if s.cfg.PartialEveryMs <= 0 {
		return nil
	}
	if !s.lastPartial.IsZero() && time.Since(s.lastPartial) < time.Duration(s.cfg.PartialEveryMs)*time.Millisecond {
		return nil
	}
	if s.lastDecode > 0 && len(s.buf)-s.lastDecode < samplesForMs(minNewAudioMs) {
		return nil
	}

	// Partials are tentative re-decodes; they consume the committed
	// prompt for continuity but never advance it.
	tr, _, err := s.decode(s.decodeWindow(), s.firstWindow, s.promptTokens)
	if err != nil {
		return err
	}

	s.lastDecode = len(s.buf)
	s.lastPartial = time.Now()
	s.emitPartial(tr)

	return nil
}

// commit decodes the un-committed window, emits a Final, advances the
// session clock, retains only KeepMs of trailing audio, and rolls the
// harvested tail tokens forward as the next window's prompt.
func (s *Stream) commit() error {
	tr, toks, err := s.decode(s.decodeWindow(), s.firstWindow, s.promptTokens)
	if err != nil {
		return err
	}

	s.emitFinal(tr)
	s.firstWindow = false
	s.promptTokens = toks

	keep := min(samplesForMs(s.cfg.KeepMs), len(s.buf))
	committed := len(s.buf) - keep
	s.baseMs += msForSamples(committed)

	tail := make([]float32, keep)
	copy(tail, s.buf[len(s.buf)-keep:])
	s.buf = tail
	s.lastDecode = 0
	s.lastPartial = time.Time{}

	return nil
}

// finalFlush decodes whatever audio remains and emits a closing Final.
func (s *Stream) finalFlush() error {
	if len(s.buf) == 0 {
		return nil
	}

	tr, _, err := s.decode(s.decodeWindow(), s.firstWindow, s.promptTokens)
	if err != nil {
		return err
	}

	s.emitFinal(tr)
	return nil
}

// doReset applies a reset on the worker goroutine.
func (s *Stream) doReset(rc ResetConfig) error {
	if rc.FlushPending && len(s.buf) > 0 {
		tr, toks, err := s.decode(s.decodeWindow(), s.firstWindow, s.promptTokens)
		if err != nil {
			return err
		}
		s.emitFinal(tr)

		// When continuity is requested, the flush is the freshest
		// context, so carry its tail tokens forward.
		if rc.KeepPromptTokens {
			s.promptTokens = toks
		}
	}

	s.buf = s.buf[:0]
	s.lastDecode = 0
	s.firstWindow = true
	s.lastPartial = time.Time{}
	if !rc.KeepPromptTokens {
		s.promptTokens = s.promptTokens[:0]
	}
	if rc.RebaseTimestamps {
		s.baseMs = 0
	}

	if s.cfg.EmitResetEvent {
		s.events <- Event{Kind: EventReset, StartMs: s.baseMs, EndMs: s.baseMs}
	}

	return nil
}

// decodeWindow returns the trailing audio to decode, capped at
// MaxUtteranceMs so a single pass never exceeds whisper's mel ceiling.
func (s *Stream) decodeWindow() []float32 {
	maxSamp := samplesForMs(s.cfg.MaxUtteranceMs)
	if len(s.buf) > maxSamp {
		return s.buf[len(s.buf)-maxSamp:]
	}
	return s.buf
}

// emitPartial sends a tentative event, dropping it if the consumer is
// behind (partials are revisable, so a drop is harmless).
func (s *Stream) emitPartial(tr Transcription) {
	ev := Event{
		Kind:     EventPartial,
		Text:     tr.Text,
		Segments: tr.Segments,
		StartMs:  s.baseMs,
		EndMs:    s.baseMs + msForSamples(len(s.buf)),
	}

	select {
	case s.events <- ev:
	default:
	}
}

// emitFinal sends a committed event. Finals are never dropped; the send
// blocks until the consumer reads it.
func (s *Stream) emitFinal(tr Transcription) {
	s.events <- Event{
		Kind:     EventFinal,
		Text:     tr.Text,
		Segments: tr.Segments,
		StartMs:  s.baseMs,
		EndMs:    s.baseMs + msForSamples(len(s.buf)),
	}
}

// =============================================================================
// Helpers

// detectSilenceEnergy reports whether the trailing tailMs of samples is
// quiet relative to the whole window, indicating an end of speech. It is
// the pure-Go energy-ratio detector modeled on whisper.cpp's vad_simple.
func detectSilenceEnergy(samples []float32, tailMs int, threshold float32) bool {
	n := len(samples)
	tail := samplesForMs(tailMs)
	if tail == 0 || n < tail*2 {
		return false
	}

	var allSum, lastSum float64
	for i, v := range samples {
		a := math.Abs(float64(v))
		allSum += a
		if i >= n-tail {
			lastSum += a
		}
	}

	energyAll := allSum / float64(n)
	if energyAll < speechFloor {
		return false
	}

	energyLast := lastSum / float64(tail)
	if threshold <= 0 {
		threshold = defaultVADThreshold
	}

	return energyLast < float64(threshold)*energyAll
}

// harvestPromptTokens reads the token ids decoded into state and returns
// the last maxTokens of them, to seed the next window's prompt for
// cross-window linguistic continuity. This mirrors the prompt-token
// rollover in whisper.cpp's stream example, capped so an indefinite
// session never accumulates unbounded context.
func harvestPromptTokens(state whisper.State, maxTokens int) []whisper.Token {
	nSeg := whisper.FullNSegmentsFromState(state)

	var toks []whisper.Token
	for seg := range nSeg {
		n := whisper.FullNTokensFromState(state, seg)
		for t := range n {
			toks = append(toks, whisper.FullGetTokenIDFromState(state, seg, t))
		}
	}

	if maxTokens > 0 && len(toks) > maxTokens {
		toks = toks[len(toks)-maxTokens:]
	}

	return toks
}

func samplesForMs(ms int) int {
	return ms * whisper.SampleRate / 1000
}

func msForSamples(n int) int64 {
	return int64(n) * 1000 / whisper.SampleRate
}

package model

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/bucky/pkg/whisper"
)

// =============================================================================
// Test helpers

// fakeDecoder returns a caller-supplied transcript (or error) for every
// window. It is the seam that lets the streaming worker run with no
// whisper model loaded. It records the prompt tokens it was called with
// and returns a fixed set of harvested tokens so prompt-token rollover
// can be exercised.
type fakeDecoder struct {
	mu      sync.Mutex
	text    string
	err     error
	harvest []whisper.Token   // tokens "decoded" this window
	prompts [][]whisper.Token // prompt seen on each call, in order
}

func (d *fakeDecoder) decode(samples []float32, firstWindow bool, prompt []whisper.Token) (Transcription, []whisper.Token, error) {
	d.mu.Lock()
	cp := append([]whisper.Token(nil), prompt...)
	d.prompts = append(d.prompts, cp)
	d.mu.Unlock()

	if d.err != nil {
		return Transcription{}, nil, d.err
	}

	return Transcription{Text: d.text}, d.harvest, nil
}

func (d *fakeDecoder) promptCalls() [][]whisper.Token {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([][]whisper.Token, len(d.prompts))
	copy(out, d.prompts)
	return out
}

// collector drains a Stream's Events channel on its own goroutine so
// blocking Finals are always read, mirroring a real consumer.
type collector struct {
	mu     sync.Mutex
	events []Event
	doneC  chan struct{}
}

func newCollector(s *Stream) *collector {
	c := collector{doneC: make(chan struct{})}
	go func() {
		defer close(c.doneC)
		for ev := range s.Events() {
			c.mu.Lock()
			c.events = append(c.events, ev)
			c.mu.Unlock()
		}
	}()
	return &c
}

func (c *collector) snapshot() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Event, len(c.events))
	copy(out, c.events)
	return out
}

func (c *collector) count(kind EventKind) int {
	n := 0
	for _, ev := range c.snapshot() {
		if ev.Kind == kind {
			n++
		}
	}
	return n
}

// waitFor polls until pred is satisfied or the deadline elapses.
func (c *collector) waitFor(t *testing.T, pred func([]Event) bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pred(c.snapshot()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within deadline; events=%v", c.snapshot())
}

// tone returns ms milliseconds of constant-amplitude 16 kHz audio.
func tone(ms int, amp float32) []float32 {
	out := make([]float32, samplesForMs(ms))
	for i := range out {
		out[i] = amp
	}
	return out
}

// =============================================================================
// Lifecycle

func TestStream_CloseClosesEvents(t *testing.T) {
	fd := &fakeDecoder{text: "x"}
	s := newStream(StreamConfig{}.withDefaults(), fd.decode, func() {})

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case _, ok := <-s.Events():
		if ok {
			// drain remaining buffered events, then verify closed.
			for range s.Events() {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("Events channel not closed after Close")
	}
}

func TestStream_CloseIdempotent(t *testing.T) {
	fd := &fakeDecoder{text: "x"}
	s := newStream(StreamConfig{}.withDefaults(), fd.decode, func() {})

	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestStream_CloseReleasesSlot(t *testing.T) {
	fd := &fakeDecoder{text: "x"}
	var released int
	var mu sync.Mutex
	s := newStream(StreamConfig{}.withDefaults(), fd.decode, func() {
		mu.Lock()
		released++
		mu.Unlock()
	})

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	got := released
	mu.Unlock()
	if got != 1 {
		t.Fatalf("release calls: got %d, want 1", got)
	}
}

func TestStream_FinalFlushOnClose(t *testing.T) {
	fd := &fakeDecoder{text: "flush"}
	cfg := StreamConfig{
		PartialEveryMs: -1, // final-only: no partials before close
		CommitEveryMs:  100000,
		MaxUtteranceMs: 100000,
		DisableVAD:     true, // commit only on Close
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	c := newCollector(s)

	if err := s.Feed(context.Background(), tone(500, 0.5)); err != nil {
		t.Fatalf("Feed: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	<-c.doneC

	if got := c.count(EventFinal); got != 1 {
		t.Fatalf("Final events: got %d, want 1; events=%v", got, c.snapshot())
	}
	if c.snapshot()[0].Text != "flush" {
		t.Fatalf("Final text: got %q, want %q", c.snapshot()[0].Text, "flush")
	}
}

// =============================================================================
// Emission paths

func TestStream_PartialEmission(t *testing.T) {
	fd := &fakeDecoder{text: "partial"}
	cfg := StreamConfig{
		PartialEveryMs: 10,
		CommitEveryMs:  100000,
		MaxUtteranceMs: 100000,
		DisableVAD:     true,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	defer s.Close()
	c := newCollector(s)

	if err := s.Feed(context.Background(), tone(500, 0.5)); err != nil {
		t.Fatalf("Feed: %v", err)
	}

	c.waitFor(t, func(evs []Event) bool {
		for _, ev := range evs {
			if ev.Kind == EventPartial && ev.Text == "partial" {
				return true
			}
		}
		return false
	})
}

func TestStream_PartialsDisabled(t *testing.T) {
	fd := &fakeDecoder{text: "x"}
	cfg := StreamConfig{
		PartialEveryMs: -1,
		CommitEveryMs:  100000,
		MaxUtteranceMs: 100000,
		DisableVAD:     true,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	c := newCollector(s)

	if err := s.Feed(context.Background(), tone(500, 0.5)); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	if got := c.count(EventPartial); got != 0 {
		t.Fatalf("Partial events with partials disabled: got %d, want 0", got)
	}

	s.Close()
	<-c.doneC
}

func TestStream_CommitOnCadence(t *testing.T) {
	fd := &fakeDecoder{text: "committed"}
	cfg := StreamConfig{
		PartialEveryMs: -1,
		CommitEveryMs:  200,
		MaxUtteranceMs: 100000,
		DisableVAD:     true,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	defer s.Close()
	c := newCollector(s)

	if err := s.Feed(context.Background(), tone(400, 0.5)); err != nil {
		t.Fatalf("Feed: %v", err)
	}

	c.waitFor(t, func(evs []Event) bool {
		for _, ev := range evs {
			if ev.Kind == EventFinal {
				return true
			}
		}
		return false
	})
}

func TestStream_CommitOnVADSilence(t *testing.T) {
	fd := &fakeDecoder{text: "vad"}
	cfg := StreamConfig{
		PartialEveryMs: -1,
		CommitEveryMs:  100000,
		MaxUtteranceMs: 100000,
		DisableVAD:     false,
		VADThreshold:   0.6,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	defer s.Close()
	c := newCollector(s)

	// 1s loud speech then 1s of silence triggers an end-of-speech cut.
	if err := s.Feed(context.Background(), tone(1000, 0.5)); err != nil {
		t.Fatalf("Feed loud: %v", err)
	}
	if err := s.Feed(context.Background(), tone(1000, 0)); err != nil {
		t.Fatalf("Feed silence: %v", err)
	}

	c.waitFor(t, func(evs []Event) bool {
		for _, ev := range evs {
			if ev.Kind == EventFinal {
				return true
			}
		}
		return false
	})
}

// =============================================================================
// Reset

func TestStream_ResetClearsBufferAndEmitsEvent(t *testing.T) {
	fd := &fakeDecoder{text: "before"}
	cfg := StreamConfig{
		PartialEveryMs: -1,
		CommitEveryMs:  100000,
		MaxUtteranceMs: 100000,
		DisableVAD:     true,
		EmitResetEvent: true,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	defer s.Close()
	c := newCollector(s)

	if err := s.Feed(context.Background(), tone(500, 0.5)); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	// Let the worker pull the fed audio into its buffer.
	time.Sleep(200 * time.Millisecond)

	// Default reset flushes pending audio (one Final) then EventReset.
	if err := s.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	c.waitFor(t, func(evs []Event) bool {
		return countKind(evs, EventReset) == 1
	})

	evs := c.snapshot()
	if countKind(evs, EventFinal) != 1 {
		t.Fatalf("Final before reset: got %d, want 1; events=%v", countKind(evs, EventFinal), evs)
	}
	// Reset event must come after the flush Final.
	lastFinal, resetIdx := -1, -1
	for i, ev := range evs {
		switch ev.Kind {
		case EventFinal:
			lastFinal = i
		case EventReset:
			resetIdx = i
		}
	}
	if resetIdx < lastFinal {
		t.Fatalf("EventReset (%d) emitted before flush Final (%d)", resetIdx, lastFinal)
	}
}

func TestStream_ResetNoFlush(t *testing.T) {
	fd := &fakeDecoder{text: "x"}
	cfg := StreamConfig{
		PartialEveryMs: -1,
		CommitEveryMs:  100000,
		MaxUtteranceMs: 100000,
		DisableVAD:     true,
		EmitResetEvent: true,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	defer s.Close()
	c := newCollector(s)

	if err := s.Feed(context.Background(), tone(500, 0.5)); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := s.Reset(context.Background(), WithFlushPending(false)); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	c.waitFor(t, func(evs []Event) bool {
		return countKind(evs, EventReset) == 1
	})

	if got := c.count(EventFinal); got != 0 {
		t.Fatalf("Final with FlushPending(false): got %d, want 0", got)
	}
}

// =============================================================================
// Prompt-token rollover

// tokensEqual reports whether a == b element-for-element.
func tokensEqual(a, b []whisper.Token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStream_PromptTokenRollover(t *testing.T) {
	fd := &fakeDecoder{text: "x", harvest: []whisper.Token{7, 8, 9}}
	cfg := StreamConfig{
		PartialEveryMs: -1,
		CommitEveryMs:  200,
		MaxUtteranceMs: 100000,
		DisableVAD:     true,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	c := newCollector(s)

	// Two windows worth of audio so at least two commits occur.
	if err := s.Feed(context.Background(), tone(400, 0.5)); err != nil {
		t.Fatalf("Feed 1: %v", err)
	}
	c.waitFor(t, func(evs []Event) bool { return countKind(evs, EventFinal) >= 1 })

	if err := s.Feed(context.Background(), tone(400, 0.5)); err != nil {
		t.Fatalf("Feed 2: %v", err)
	}
	c.waitFor(t, func(evs []Event) bool { return countKind(evs, EventFinal) >= 2 })

	s.Close()
	<-c.doneC

	// The first decode runs with no prompt; some decode after the first
	// commit must carry the harvested {7,8,9}.
	calls := fd.promptCalls()
	if len(calls) == 0 {
		t.Fatal("decoder never called")
	}
	if len(calls[0]) != 0 {
		t.Errorf("first window prompt: got %v, want empty", calls[0])
	}

	sawRolled := false
	for _, p := range calls {
		if tokensEqual(p, []whisper.Token{7, 8, 9}) {
			sawRolled = true
			break
		}
	}
	if !sawRolled {
		t.Fatalf("harvested prompt never rolled into a later decode; calls=%v", calls)
	}
}

func TestStream_ResetClearsPromptTokens(t *testing.T) {
	fd := &fakeDecoder{text: "x", harvest: []whisper.Token{1, 2, 3}}
	cfg := StreamConfig{
		PartialEveryMs: -1,
		CommitEveryMs:  200,
		MaxUtteranceMs: 100000,
		DisableVAD:     true,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	c := newCollector(s)

	// Commit once so promptTokens becomes {1,2,3}.
	if err := s.Feed(context.Background(), tone(400, 0.5)); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	c.waitFor(t, func(evs []Event) bool { return countKind(evs, EventFinal) >= 1 })

	// Hard-boundary reset (default) without a flush clears the context.
	if err := s.Reset(context.Background(), WithFlushPending(false)); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	before := len(fd.promptCalls())

	// Drive a fresh commit after the reset.
	if err := s.Feed(context.Background(), tone(400, 0.5)); err != nil {
		t.Fatalf("Feed after reset: %v", err)
	}
	c.waitFor(t, func(evs []Event) bool { return countKind(evs, EventFinal) >= 2 })

	s.Close()
	<-c.doneC

	// The first decode after the reset must see an empty prompt.
	calls := fd.promptCalls()
	if len(calls) <= before {
		t.Fatalf("no decode observed after reset; before=%d total=%d", before, len(calls))
	}
	if got := calls[before]; len(got) != 0 {
		t.Errorf("prompt on first decode after reset: got %v, want empty", got)
	}
}

func TestStream_ResetKeepsPromptTokens(t *testing.T) {
	fd := &fakeDecoder{text: "x", harvest: []whisper.Token{4, 5, 6}}
	cfg := StreamConfig{
		PartialEveryMs: -1,
		CommitEveryMs:  200,
		MaxUtteranceMs: 100000,
		DisableVAD:     true,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	c := newCollector(s)

	if err := s.Feed(context.Background(), tone(400, 0.5)); err != nil {
		t.Fatalf("Feed: %v", err)
	}
	c.waitFor(t, func(evs []Event) bool { return countKind(evs, EventFinal) >= 1 })

	// Keep context across the reset (no flush so the prior commit's
	// tokens remain the going-forward prompt).
	if err := s.Reset(context.Background(), WithFlushPending(false), WithKeepPromptTokens(true)); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	before := len(fd.promptCalls())

	if err := s.Feed(context.Background(), tone(400, 0.5)); err != nil {
		t.Fatalf("Feed after reset: %v", err)
	}
	c.waitFor(t, func(evs []Event) bool { return countKind(evs, EventFinal) >= 2 })

	s.Close()
	<-c.doneC

	calls := fd.promptCalls()
	if len(calls) <= before {
		t.Fatalf("no decode observed after reset; before=%d total=%d", before, len(calls))
	}
	if got := calls[before]; !tokensEqual(got, []whisper.Token{4, 5, 6}) {
		t.Errorf("prompt on first decode after keep-reset: got %v, want [4 5 6]", got)
	}
}

// =============================================================================
// Error path

func TestStream_DecodeErrorIsTerminal(t *testing.T) {
	fd := &fakeDecoder{text: "x", err: fmt.Errorf("boom")}
	cfg := StreamConfig{
		PartialEveryMs: -1,
		CommitEveryMs:  200,
		MaxUtteranceMs: 100000,
		DisableVAD:     true,
	}.withDefaults()

	s := newStream(cfg, fd.decode, func() {})
	defer s.Close()
	c := newCollector(s)

	if err := s.Feed(context.Background(), tone(400, 0.5)); err != nil {
		t.Fatalf("Feed: %v", err)
	}

	c.waitFor(t, func(evs []Event) bool {
		return countKind(evs, EventError) == 1
	})

	// Channel must close after a terminal error.
	select {
	case <-c.doneC:
	case <-time.After(2 * time.Second):
		t.Fatal("Events not closed after EventError")
	}
}

// =============================================================================
// Producer-side conversion

func TestConvert_Int16Mono(t *testing.T) {
	s := &Stream{}

	// Two samples: full positive and full negative scale.
	raw := make([]byte, 4)
	binary.LittleEndian.PutUint16(raw[0:], putI16(16384))  // +0.5
	binary.LittleEndian.PutUint16(raw[2:], putI16(-16384)) // -0.5

	got := s.convert(raw, AudioFormat{SampleRate: 16000, Channels: 1, Sample: Int16LE})
	if len(got) != 2 {
		t.Fatalf("samples: got %d, want 2", len(got))
	}
	if math.Abs(float64(got[0])-0.5) > 0.01 {
		t.Errorf("sample 0: got %f, want ~0.5", got[0])
	}
	if math.Abs(float64(got[1])+0.5) > 0.01 {
		t.Errorf("sample 1: got %f, want ~-0.5", got[1])
	}
}

func TestConvert_PartialFrameBuffered(t *testing.T) {
	s := &Stream{}
	f := AudioFormat{SampleRate: 16000, Channels: 1, Sample: Int16LE}

	// Feed 3 bytes: one whole int16 sample plus a dangling byte.
	raw := []byte{0x00, 0x40, 0x11}
	got := s.convert(raw, f)
	if len(got) != 1 {
		t.Fatalf("first convert samples: got %d, want 1", len(got))
	}
	if len(s.pcmRem) != 1 {
		t.Fatalf("remainder: got %d bytes, want 1", len(s.pcmRem))
	}

	// Supply the missing byte; the buffered byte completes a sample.
	got = s.convert([]byte{0x40}, f)
	if len(got) != 1 {
		t.Fatalf("second convert samples: got %d, want 1", len(got))
	}
	if len(s.pcmRem) != 0 {
		t.Fatalf("remainder after completion: got %d bytes, want 0", len(s.pcmRem))
	}
}

func TestConvert_StereoDownmix(t *testing.T) {
	s := &Stream{}
	f := AudioFormat{SampleRate: 16000, Channels: 2, Sample: Int16LE}

	// One stereo frame: L=+0.5, R=-0.5 -> mono ~0.
	raw := make([]byte, 4)
	binary.LittleEndian.PutUint16(raw[0:], putI16(16384))
	binary.LittleEndian.PutUint16(raw[2:], putI16(-16384))

	got := s.convert(raw, f)
	if len(got) != 1 {
		t.Fatalf("downmixed samples: got %d, want 1", len(got))
	}
	if math.Abs(float64(got[0])) > 0.01 {
		t.Errorf("downmixed sample: got %f, want ~0", got[0])
	}
}

// =============================================================================
// Pure helpers

func TestDetectSilenceEnergy(t *testing.T) {
	// Loud throughout: no end-of-speech.
	if detectSilenceEnergy(tone(2000, 0.5), 700, 0.6) {
		t.Error("constant loud audio reported as silence")
	}

	// Loud then silent tail: end-of-speech.
	loudThenQuiet := append(tone(1500, 0.5), tone(700, 0)...)
	if !detectSilenceEnergy(loudThenQuiet, 700, 0.6) {
		t.Error("loud-then-silent audio not reported as silence")
	}

	// Entirely silent: below the speech floor, never a cut.
	if detectSilenceEnergy(tone(2000, 0), 700, 0.6) {
		t.Error("pure silence reported as end-of-speech")
	}

	// Too short to measure.
	if detectSilenceEnergy(tone(100, 0.5), 700, 0.6) {
		t.Error("too-short buffer reported as silence")
	}
}

func TestSamplesMsRoundTrip(t *testing.T) {
	if got := samplesForMs(1000); got != 16000 {
		t.Fatalf("samplesForMs(1000): got %d, want 16000", got)
	}
	if got := msForSamples(16000); got != 1000 {
		t.Fatalf("msForSamples(16000): got %d, want 1000", got)
	}
}

// putI16 returns the little-endian uint16 bit pattern for a signed
// 16-bit value, avoiding constant-overflow at conversion sites.
func putI16(v int) uint16 {
	return uint16(int16(v))
}

func countKind(evs []Event, kind EventKind) int {
	n := 0
	for _, ev := range evs {
		if ev.Kind == kind {
			n++
		}
	}
	return n
}

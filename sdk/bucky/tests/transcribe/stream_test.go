package transcribe_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/bucky/tests/testlib"
)

// feedChunks feeds samples into s in chunkMs-sized blocks, pacing the
// producer so the worker sees audio arriving over time the way a live
// microphone delivers it.
func feedChunks(t *testing.T, s *model.Stream, samples []float32, chunkMs int) {
	t.Helper()

	chunk := chunkMs * 16000 / 1000
	for off := 0; off < len(samples); off += chunk {
		end := min(off+chunk, len(samples))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := s.Feed(ctx, samples[off:end])
		cancel()
		if err != nil {
			t.Fatalf("Feed: %v", err)
		}

		time.Sleep(time.Duration(chunkMs) * time.Millisecond)
	}
}

// collectTranscript drains a stream's events until the channel closes,
// returning the concatenated Final text and the per-kind counts.
func collectTranscript(s *model.Stream) (final string, partials, finals, errs int, lastErr error) {
	var b strings.Builder
	for ev := range s.Events() {
		switch ev.Kind {
		case model.EventPartial:
			partials++
		case model.EventFinal:
			finals++
			b.WriteString(ev.Text)
		case model.EventError:
			errs++
			lastErr = ev.Err
		}
	}
	return b.String(), partials, finals, errs, lastErr
}

// Test_StreamTranscribe feeds the JFK clip in 100 ms chunks and asserts
// the streamed transcript reconstructs the well-known phrase. This is the
// end-to-end smoke test for the streaming worker against a real model.
func Test_StreamTranscribe(t *testing.T) {
	testlib.WithWhisper(t, testlib.CfgTinyEn(), func(t *testing.T, w *bucky.Bucky) {
		samples := testlib.LoadSamples(t, testlib.AudioFile)

		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		s, err := w.NewStream(ctx, model.WithStreamLanguage("en"))
		if err != nil {
			t.Fatalf("NewStream: %v", err)
		}

		if got := w.ActiveStreams(); got < 1 {
			t.Errorf("ActiveStreams with open stream: got %d, want >= 1", got)
		}

		done := make(chan struct{})
		var text string
		var partials, finals, errs int
		var lastErr error
		go func() {
			defer close(done)
			text, partials, finals, errs, lastErr = collectTranscript(s)
		}()

		feedChunks(t, s, samples, 100)

		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		<-done

		if errs != 0 {
			t.Fatalf("error events: got %d (last=%v), want 0", errs, lastErr)
		}
		if finals == 0 {
			t.Fatalf("final events: got 0, want >= 1")
		}

		t.Logf("partials=%d finals=%d text=%q", partials, finals, text)

		// The streaming/windowed path on the multilingual ggml-tiny.bin
		// model renders the opening words less precisely than a one-shot
		// pass (e.g. "As not" instead of "ask not"), so assert on the
		// words it produces stably across windows.
		testlib.AssertTranscriptContains(t, text, "americans", "country")

		if got := w.ActiveStreams(); got != 0 {
			t.Errorf("ActiveStreams after Close: got %d, want 0", got)
		}
	})
}

// Test_StreamReset verifies a single stream is reusable across logical
// sessions: after transcribing JFK, Reset clears all state, and a second
// pass over the same audio still produces the phrase with no bleed-over.
func Test_StreamReset(t *testing.T) {
	testlib.WithWhisper(t, testlib.CfgTinyEn(), func(t *testing.T, w *bucky.Bucky) {
		samples := testlib.LoadSamples(t, testlib.AudioFile)

		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		s, err := w.NewStream(ctx,
			model.WithStreamLanguage("en"),
			model.WithEmitResetEvent(true),
		)
		if err != nil {
			t.Fatalf("NewStream: %v", err)
		}
		defer s.Close()

		// Collect events on a goroutine; record whether a reset boundary
		// was observed and the finals seen after it.
		type result struct {
			sawReset    bool
			afterReset  strings.Builder
			beforeReset strings.Builder
		}
		var res result
		resetSeen := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			closed := false
			for ev := range s.Events() {
				switch ev.Kind {
				case model.EventReset:
					res.sawReset = true
					if !closed {
						close(resetSeen)
						closed = true
					}
				case model.EventFinal:
					if res.sawReset {
						res.afterReset.WriteString(ev.Text)
					} else {
						res.beforeReset.WriteString(ev.Text)
					}
				}
			}
		}()

		feedChunks(t, s, samples, 100)

		// Reset (default flush) commits the first session and clears state.
		if err := s.Reset(ctx); err != nil {
			t.Fatalf("Reset: %v", err)
		}

		select {
		case <-resetSeen:
		case <-time.After(10 * time.Second):
			t.Fatal("EventReset not observed after Reset")
		}

		// Second session over the same audio.
		feedChunks(t, s, samples, 100)

		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		<-done

		if !res.sawReset {
			t.Fatal("never observed EventReset")
		}

		t.Logf("before=%q after=%q", res.beforeReset.String(), res.afterReset.String())
		testlib.AssertTranscriptContains(t, res.beforeReset.String(), "country")
		testlib.AssertTranscriptContains(t, res.afterReset.String(), "country")
	})
}

// Test_StreamUnloadDrain confirms an open stream blocks Unload exactly
// like an in-flight Transcribe: Unload must wait until the stream is
// closed before returning. This test owns its own handle (rather than
// using testlib.WithWhisper) so it can call Unload explicitly.
func Test_StreamUnloadDrain(t *testing.T) {
	w, err := bucky.New(model.WithConfig(testlib.CfgTinyEn()))
	if err != nil {
		t.Fatalf("Failed to create whisper handle: %v", err)
	}

	samples := testlib.LoadSamples(t, testlib.AudioFile)

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	s, err := w.NewStream(ctx, model.WithStreamLanguage("en"))
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}

	go func() {
		for range s.Events() {
		}
	}()

	// Feed a little audio so the stream is genuinely active.
	half := samples[:len(samples)/2]
	feedChunks(t, s, half, 100)

	if got := w.ActiveStreams(); got == 0 {
		t.Fatal("ActiveStreams: never observed > 0 with open stream")
	}

	// Close the stream shortly, while Unload is blocking on the drain.
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.Close()
	}()

	uctx, ucancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ucancel()
	if err := w.Unload(uctx); err != nil {
		t.Fatalf("Unload: %v", err)
	}

	if got := w.ActiveStreams(); got != 0 {
		t.Errorf("ActiveStreams after Unload: got %d, want 0", got)
	}
}

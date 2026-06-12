package transcribe_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/bucky/tests/testlib"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func TestSuite(t *testing.T) {
	testlib.WithWhisper(t, testlib.CfgTinyEn(), func(t *testing.T, w *bucky.Bucky) {
		t.Run("Transcribe", func(t *testing.T) { testTranscribe(t, w) })
		t.Run("TranscribeOnSegment", func(t *testing.T) { testTranscribeOnSegment(t, w) })
		t.Run("TranscribeChannels", func(t *testing.T) { testTranscribeChannels(t, w) })
		t.Run("DecodeChannels", func(t *testing.T) { testDecodeChannels(t, w) })
		t.Run("DetectLanguage", func(t *testing.T) { testDetectLanguage(t, w) })
	})
}

func testTranscribe(t *testing.T, w *bucky.Bucky) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	samples := testlib.LoadSamples(t, testlib.AudioFile)

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		tr, err := w.Transcribe(ctx, samples, model.WithLanguage("en"))
		done := time.Now()

		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, w.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))

		if err != nil {
			return fmt.Errorf("transcribe: %w", err)
		}

		if tr.Text == "" {
			return fmt.Errorf("empty transcript")
		}
		if len(tr.Segments) == 0 {
			return fmt.Errorf("no segments")
		}
		if tr.Language != "en" {
			return fmt.Errorf("language: got %q, want %q", tr.Language, "en")
		}

		// The JFK "ask not what your country" clip is famous and
		// stable; whisper output varies slightly so use substring
		// matches rather than equality.
		testlib.AssertTranscriptContains(t, tr.Text, "ask not", "country")
		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(testlib.WithRetry(t, f))
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func testTranscribeOnSegment(t *testing.T, w *bucky.Bucky) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	samples := testlib.LoadSamples(t, testlib.AudioFile)

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	var got []model.Segment
	tr, err := w.Transcribe(ctx, samples,
		model.WithLanguage("en"),
		model.WithOnSegment(func(s model.Segment) { got = append(got, s) }),
	)
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}

	if len(got) != len(tr.Segments) {
		t.Fatalf("OnSegment callback count: got %d, want %d", len(got), len(tr.Segments))
	}
	for i, s := range got {
		if s.Text != tr.Segments[i].Text {
			t.Errorf("OnSegment[%d].Text: got %q, want %q", i, s.Text, tr.Segments[i].Text)
		}
	}
}

func testTranscribeChannels(t *testing.T, w *bucky.Bucky) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	// Decode the real two-channel sample (English speaker on channel 0,
	// Spanish speaker on channel 1). This exercises the multi-channel
	// decode path: DecodeRaw -> SplitChannels -> ResampleLinear.
	f, err := os.Open(testlib.StereoAudioFile)
	if err != nil {
		t.Fatalf("open %q: %v", testlib.StereoAudioFile, err)
	}
	defer f.Close()

	channels, err := model.DecodeChannels(context.Background(), f)
	if err != nil {
		t.Fatalf("decode-channels: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("decoded channels: got %d, want 2", len(channels))
	}
	for i, ch := range channels {
		if len(ch) == 0 {
			t.Fatalf("decoded channel[%d]: got 0 samples", i)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	// Auto-detect (no language hint) so each channel's language is
	// identified independently, proving true per-speaker separation.
	d, err := w.TranscribeChannels(ctx, channels)
	if err != nil {
		t.Fatalf("transcribe-channels: %v", err)
	}

	if len(d.Channels) != 2 {
		t.Fatalf("result channels: got %d, want 2", len(d.Channels))
	}

	// Channel 0 is the English speaker; channel 1 is the Spanish speaker.
	if d.Channels[0].Channel != 0 || d.Channels[1].Channel != 1 {
		t.Errorf("channel indices: got %d,%d want 0,1", d.Channels[0].Channel, d.Channels[1].Channel)
	}
	testlib.AssertTranscriptContains(t, d.Channels[0].Text, "ask not", "country")
	if d.Channels[0].Language != "en" {
		t.Errorf("channel 0 language: got %q, want %q", d.Channels[0].Language, "en")
	}
	testlib.AssertTranscriptContains(t, d.Channels[1].Text, "hola")
	if d.Channels[1].Language != "es" {
		t.Errorf("channel 1 language: got %q, want %q", d.Channels[1].Language, "es")
	}

	// Merged segments must be sorted by start time, carry a valid channel
	// tag, and include segments from both channels.
	if len(d.Segments) == 0 {
		t.Fatal("merged segments: got 0, want > 0")
	}
	seen := map[int]bool{}
	for i, s := range d.Segments {
		if s.Channel < 0 || s.Channel >= len(d.Channels) {
			t.Errorf("segment[%d].Channel out of range: %d", i, s.Channel)
		}
		seen[s.Channel] = true
		if i > 0 && s.StartMs < d.Segments[i-1].StartMs {
			t.Errorf("segments not sorted by start: [%d]=%d < [%d]=%d", i, s.StartMs, i-1, d.Segments[i-1].StartMs)
		}
	}
	if !seen[0] || !seen[1] {
		t.Errorf("merged segments missing a channel: seen=%v", seen)
	}
}

func testDecodeChannels(t *testing.T, w *bucky.Bucky) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	// End-to-end: decode the two-channel sample straight from an
	// io.Reader and diarize it in one call. Channel 0 is the English
	// speaker, channel 1 the Spanish speaker.
	f, err := os.Open(testlib.StereoAudioFile)
	if err != nil {
		t.Fatalf("open %q: %v", testlib.StereoAudioFile, err)
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	d, err := w.TranscribeChannelsFile(ctx, f)
	if err != nil {
		t.Fatalf("transcribe-channels-file: %v", err)
	}

	if len(d.Channels) != 2 {
		t.Fatalf("channels: got %d, want 2", len(d.Channels))
	}
	testlib.AssertTranscriptContains(t, d.Channels[0].Text, "ask not", "country")
	if d.Channels[0].Language != "en" {
		t.Errorf("channel 0 language: got %q, want %q", d.Channels[0].Language, "en")
	}
	testlib.AssertTranscriptContains(t, d.Channels[1].Text, "hola")
	if d.Channels[1].Language != "es" {
		t.Errorf("channel 1 language: got %q, want %q", d.Channels[1].Language, "es")
	}
}

func testDetectLanguage(t *testing.T, w *bucky.Bucky) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	samples := testlib.LoadSamples(t, testlib.AudioFile)

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	// The multilingual ggml-tiny.bin model auto-detects the language
	// from the audio. The assertion is that the call succeeds and
	// returns a non-empty code (the JFK clip detects as "en").
	lang, probs, err := w.DetectLanguage(ctx, samples, false)
	if err != nil {
		t.Fatalf("DetectLanguage: %v", err)
	}
	if lang == "" {
		t.Fatalf("DetectLanguage code: got empty, want non-empty")
	}
	if probs != nil {
		t.Errorf("DetectLanguage probs: got %d entries, want nil (withProbs=false)", len(probs))
	}
}

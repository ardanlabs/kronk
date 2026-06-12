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

	// Build a synthetic two-channel input from the mono JFK clip so the
	// SplitChannels path is exercised: channel 0 is the clip, channel 1
	// is a half-amplitude copy. Both must still transcribe to the same
	// stable phrase.
	mono := testlib.LoadSamples(t, testlib.AudioFile)
	quiet := make([]float32, len(mono))
	for i, v := range mono {
		quiet[i] = v * 0.5
	}
	channels := [][]float32{mono, quiet}

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	d, err := w.TranscribeChannels(ctx, channels, model.WithLanguage("en"))
	if err != nil {
		t.Fatalf("transcribe-channels: %v", err)
	}

	if len(d.Channels) != 2 {
		t.Fatalf("channels: got %d, want 2", len(d.Channels))
	}
	for i, ct := range d.Channels {
		if ct.Channel != i {
			t.Errorf("channel[%d].Channel: got %d, want %d", i, ct.Channel, i)
		}
		testlib.AssertTranscriptContains(t, ct.Text, "ask not", "country")
	}

	if len(d.Segments) == 0 {
		t.Fatal("merged segments: got 0, want > 0")
	}

	// Merged segments must be sorted by start time and carry a valid
	// channel tag.
	for i := 1; i < len(d.Segments); i++ {
		if d.Segments[i].StartMs < d.Segments[i-1].StartMs {
			t.Errorf("segments not sorted by start: [%d]=%d < [%d]=%d", i, d.Segments[i].StartMs, i-1, d.Segments[i-1].StartMs)
		}
	}
	for i, s := range d.Segments {
		if s.Channel < 0 || s.Channel >= len(d.Channels) {
			t.Errorf("segment[%d].Channel out of range: %d", i, s.Channel)
		}
	}
}

func testDecodeChannels(t *testing.T, w *bucky.Bucky) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	// The bundled JFK clip is mono, so DecodeChannels must yield exactly
	// one channel and TranscribeChannelsFile must produce one speaker.
	f, err := os.Open(testlib.AudioFile)
	if err != nil {
		t.Fatalf("open %q: %v", testlib.AudioFile, err)
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	d, err := w.TranscribeChannelsFile(ctx, f, model.WithLanguage("en"))
	if err != nil {
		t.Fatalf("transcribe-channels-file: %v", err)
	}

	if len(d.Channels) != 1 {
		t.Fatalf("channels: got %d, want 1 (mono input)", len(d.Channels))
	}
	testlib.AssertTranscriptContains(t, d.Channels[0].Text, "ask not", "country")
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

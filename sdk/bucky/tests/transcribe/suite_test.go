package transcribe_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/bucky/tests/testlib"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func TestSuite(t *testing.T) {
	testlib.WithWhisper(t, testlib.CfgTinyEn(), func(t *testing.T, w *bucky.Whisper) {
		t.Run("Transcribe", func(t *testing.T) { testTranscribe(t, w) })
		t.Run("TranscribeOnSegment", func(t *testing.T) { testTranscribeOnSegment(t, w) })
		t.Run("DetectLanguage", func(t *testing.T) { testDetectLanguage(t, w) })
	})
}

func testTranscribe(t *testing.T, w *bucky.Whisper) {
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

func testTranscribeOnSegment(t *testing.T, w *bucky.Whisper) {
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

func testDetectLanguage(t *testing.T, w *bucky.Whisper) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	samples := testlib.LoadSamples(t, testlib.AudioFile)

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	// tiny.en is English-only — the language id from a Full pass is
	// always "en" regardless of audio content. The assertion is
	// that the call succeeds and returns a non-empty code.
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

package testlib

import (
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/ardanlabs/bucky/pkg/audio"
)

// WithRetry wraps a test function with retry logic to handle
// non-determinism under concurrent load. On failure it logs the
// attempt and retries up to MaxRetries times. Mirrors the
// sdk/kronk/tests/testlib.WithRetry helper.
func WithRetry(t testing.TB, f func() error) func() error {
	return func() error {
		var err error
		for attempt := 1; attempt <= MaxRetries; attempt++ {
			err = f()
			if err == nil {
				return nil
			}
			if attempt < MaxRetries {
				t.Logf("RETRY: attempt %d/%d failed: %v", attempt, MaxRetries, err)
			}
		}
		return err
	}
}

// LoadSamples decodes the supplied wav / mp3 / flac file into 16 kHz
// mono float32 PCM samples. Tests that need the bundled JFK clip
// pass testlib.AudioFile.
func LoadSamples(t *testing.T, path string) []float32 {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %q: %v", path, err)
	}
	defer f.Close()

	samples, err := audio.Decode(f)
	if err != nil {
		t.Fatalf("decode %q: %v", path, err)
	}
	if len(samples) == 0 {
		t.Fatalf("decode %q: no samples", path)
	}
	return samples
}

// AssertTranscriptContains reports an error when text does not
// contain every supplied substring (case-insensitive). Whisper
// output varies slightly across runs so transcribe assertions are
// substring-based, not equality-based. Punctuation and runs of
// whitespace are normalized away before matching, so "Ask, not."
// still satisfies a "ask not" expectation.
func AssertTranscriptContains(t *testing.T, text string, subs ...string) {
	t.Helper()

	norm := normalizeTranscript(text)
	for _, sub := range subs {
		if !strings.Contains(norm, normalizeTranscript(sub)) {
			t.Errorf("transcript: got %q, want substring %q", text, sub)
		}
	}
}

// normalizeTranscript lower-cases s, replaces every non-alphanumeric
// rune with a space, and collapses runs of whitespace to a single
// space. This makes substring matching tolerant of the punctuation
// jitter whisper produces between runs.
func normalizeTranscript(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte(' ')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

package ffmpeg_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky/ffmpeg"
)

func TestMain(m *testing.M) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Println("skipping ffmpeg tests in GitHub Actions")
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestNew_NotInstalled(t *testing.T) {
	t.Setenv("PATH", "")

	_, err := ffmpeg.New("")
	if !errors.Is(err, ffmpeg.ErrNotInstalled) {
		t.Fatalf("New(\"\"): got %v, want %v", err, ffmpeg.ErrNotInstalled)
	}
}

func TestNew_FoundOnPATH(t *testing.T) {
	requireFFmpeg(t)

	f, err := ffmpeg.New("")
	if err != nil {
		t.Fatalf("New(\"\"): got %v, want nil", err)
	}
	if f.Binary() == "" {
		t.Fatalf("Binary(): got empty, want resolved path")
	}
}

func TestNew_ExplicitPath(t *testing.T) {
	requireFFmpeg(t)

	resolved, err := exec.LookPath(ffmpeg.DefaultBinary)
	if err != nil {
		t.Fatalf("LookPath: got %v, want nil", err)
	}

	f, err := ffmpeg.New(resolved)
	if err != nil {
		t.Fatalf("New(%q): got %v, want nil", resolved, err)
	}
	if got := f.Binary(); got != resolved {
		t.Fatalf("Binary(): got %q, want %q", got, resolved)
	}
}

func TestToPCM16Bytes_FromWebMOpus(t *testing.T) {
	f := mustFFmpeg(t)

	in := generateAudio(t, "webm", "libopus")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	out, err := f.ToPCM16Bytes(ctx, bytes.NewReader(in))
	if err != nil {
		t.Fatalf("ToPCM16Bytes: got %v, want nil", err)
	}

	assertPCM16k(t, out)
}

func TestToPCM16Bytes_FromMP3(t *testing.T) {
	f := mustFFmpeg(t)

	in := generateAudio(t, "mp3", "libmp3lame")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	out, err := f.ToPCM16Bytes(ctx, bytes.NewReader(in))
	if err != nil {
		t.Fatalf("ToPCM16Bytes: got %v, want nil", err)
	}

	assertPCM16k(t, out)
}

func TestToPCM16_StreamingWriter(t *testing.T) {
	f := mustFFmpeg(t)

	in := generateAudio(t, "webm", "libopus")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	var out bytes.Buffer
	if err := f.ToPCM16(ctx, &out, bytes.NewReader(in)); err != nil {
		t.Fatalf("ToPCM16: got %v, want nil", err)
	}

	assertPCM16k(t, out.Bytes())
}

func TestToPCM16_BadInput(t *testing.T) {
	f := mustFFmpeg(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_, err := f.ToPCM16Bytes(ctx, strings.NewReader("this is not audio"))
	if err == nil {
		t.Fatalf("ToPCM16Bytes: got nil, want error")
	}
	if !strings.Contains(err.Error(), "ffmpeg") {
		t.Fatalf("ToPCM16Bytes: got %v, want error containing %q", err, "ffmpeg")
	}
}

func TestToPCM16_ContextCancel(t *testing.T) {
	f := mustFFmpeg(t)

	// ctxReader blocks on Read until ctx is done, then returns
	// ctx.Err(). This keeps ffmpeg alive on stdin so we can prove
	// that cancelling ctx tears the whole pipeline down.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := f.ToPCM16Bytes(ctx, ctxReader{ctx: ctx})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("ToPCM16Bytes: got nil, want error from cancelled context")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("ToPCM16Bytes: did not return after ctx cancel")
	}
}

// ctxReader blocks until ctx is done then returns ctx.Err().
type ctxReader struct{ ctx context.Context }

func (r ctxReader) Read(p []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

// =============================================================================

// requireFFmpeg skips the test when no ffmpeg binary is on PATH.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(ffmpeg.DefaultBinary); err != nil {
		t.Skip("ffmpeg not installed; skipping")
	}
}

// mustFFmpeg returns a working *FFmpeg or skips when not installed.
func mustFFmpeg(t *testing.T) *ffmpeg.FFmpeg {
	t.Helper()
	requireFFmpeg(t)

	f, err := ffmpeg.New("")
	if err != nil {
		t.Fatalf("New: got %v, want nil", err)
	}
	return f
}

// generateAudio uses the system ffmpeg directly (not the package under
// test) to synthesize a tiny 1s sine-tone stream in the requested
// container/codec. This avoids checking in binary fixtures.
func generateAudio(t *testing.T, format, codec string) []byte {
	t.Helper()

	args := []string{
		"-hide_banner",
		"-nostats",
		"-loglevel", "error",
		"-f", "lavfi",
		"-i", "sine=frequency=440:duration=1:sample_rate=48000",
		"-c:a", codec,
		"-f", format,
		"pipe:1",
	}

	cmd := exec.Command(ffmpeg.DefaultBinary, args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot synthesize %s/%s fixture (codec not built into ffmpeg?): %v: %s", format, codec, err, stderr.String())
	}

	if out.Len() == 0 {
		t.Fatalf("generated %s/%s fixture is empty", format, codec)
	}
	return out.Bytes()
}

// assertPCM16k checks that b is a non-trivial blob of signed 16-bit
// little-endian PCM at 16 kHz mono. A 1-second fixture should yield
// ~32 KB (16 000 samples × 2 bytes).
func assertPCM16k(t *testing.T, b []byte) {
	t.Helper()

	if len(b)%2 != 0 {
		t.Fatalf("len: got %d bytes (odd), want even (s16le)", len(b))
	}
	const wantMin = 16000 // 0.5s worth of samples, conservative floor
	if len(b)/2 < wantMin {
		t.Fatalf("samples: got %d, want >= %d", len(b)/2, wantMin)
	}
}

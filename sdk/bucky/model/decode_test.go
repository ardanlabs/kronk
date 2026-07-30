package model_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/bucky/pkg/audio"
	"github.com/ardanlabs/kronk/sdk/bucky/ffmpeg"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

func TestMain(m *testing.M) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Println("skipping bucky/model decode tests in GitHub Actions")
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestDecode_NativeWAV(t *testing.T) {
	// Native WAV path must work even without ffmpeg on PATH.
	t.Setenv("PATH", "")

	in := synthesizeWAV(t, 1.0)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	samples, err := model.Decode(ctx, bytes.NewReader(in))
	if err != nil {
		t.Fatalf("Decode: got %v, want nil", err)
	}

	wantMin := 15000
	if len(samples) < wantMin {
		t.Fatalf("samples: got %d, want >= %d", len(samples), wantMin)
	}
}

func TestDecode_WebMOpus_ViaFFmpeg(t *testing.T) {
	requireFFmpeg(t)

	in := synthesizeContainer(t, "webm", "libopus")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	samples, err := model.Decode(ctx, bytes.NewReader(in))
	if err != nil {
		t.Fatalf("Decode: got %v, want nil", err)
	}

	wantMin := 15000
	if len(samples) < wantMin {
		t.Fatalf("samples: got %d, want >= %d", len(samples), wantMin)
	}
}

func TestDecode_NonNative_FFmpegMissing(t *testing.T) {
	const helperEnv = "KRONK_TEST_FFMPEG_MISSING"
	if os.Getenv(helperEnv) == "1" {
		_, err := model.Decode(t.Context(), strings.NewReader("non-native audio"))
		if !errors.Is(err, audio.ErrUnsupportedFormat) {
			t.Fatalf("Decode: got %v, want wraps audio.ErrUnsupportedFormat", err)
		}
		if !errors.Is(err, ffmpeg.ErrNotInstalled) {
			t.Fatalf("Decode: got %v, want wraps ffmpeg.ErrNotInstalled", err)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestDecode_NonNative_FFmpegMissing$")
	cmd.Env = append(os.Environ(), helperEnv+"=1", "PATH=")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("missing-ffmpeg subprocess: %v\n%s", err, output)
	}
}

func TestDecode_Garbage(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	_, err := model.Decode(ctx, strings.NewReader("this is definitely not audio at all"))
	if err == nil {
		t.Fatalf("Decode: got nil, want error")
	}
	if !errors.Is(err, audio.ErrUnsupportedFormat) {
		t.Fatalf("Decode: got %v, want wraps audio.ErrUnsupportedFormat", err)
	}
}

// =============================================================================

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed; skipping")
	}
}

// synthesizeWAV generates a tiny 16 kHz mono PCM16 sine WAV directly in
// Go so the native-path test does not depend on ffmpeg being installed.
func synthesizeWAV(t *testing.T, seconds float64) []byte {
	t.Helper()

	const sampleRate = 16000
	const freq = 440.0

	n := int(seconds * sampleRate)
	pcm := make([]byte, n*2)
	for i := range n {
		v := math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
		s := int16(v * 32760)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(s))
	}

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))
	binary.Write(&buf, binary.LittleEndian, uint16(1))            // PCM
	binary.Write(&buf, binary.LittleEndian, uint16(1))            // channels
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))   // sample rate
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2)) // byte rate
	binary.Write(&buf, binary.LittleEndian, uint16(2))            // block align
	binary.Write(&buf, binary.LittleEndian, uint16(16))           // bits per sample
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)

	return buf.Bytes()
}

// synthesizeContainer shells out to the system ffmpeg to synthesize a
// 1s sine tone in the requested container/codec.
func synthesizeContainer(t *testing.T, format, codec string) []byte {
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

	cmd := exec.Command("ffmpeg", args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot synthesize %s/%s fixture: %v: %s", format, codec, err, stderr.String())
	}
	if out.Len() == 0 {
		t.Fatalf("generated %s/%s fixture is empty", format, codec)
	}
	return out.Bytes()
}

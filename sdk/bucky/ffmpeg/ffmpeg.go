// Package ffmpeg provides a thin wrapper around the ffmpeg command-line
// tool. It exists so the bucky SDK can transcode browser-recorded audio
// (WebM/Opus, MP4/AAC, OGG, ...) into the 16 kHz mono signed 16-bit
// little-endian PCM that whisper.cpp consumes.
//
// ffmpeg is invoked as a subprocess with input on stdin and output on
// stdout. Raw PCM is emitted rather than RIFF/WAVE because ffmpeg
// cannot patch the WAV header sizes when writing to a non-seekable
// pipe (it leaves them as 0xFFFFFFFF), which trips strict WAV decoders.
// No temporary files are written.
//
// If the binary is not present on PATH, New returns ErrNotInstalled so
// callers can fail gracefully and ask the user to install ffmpeg.
package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// DefaultBinary is the binary name New looks for on PATH when no
// explicit path is given.
const DefaultBinary = "ffmpeg"

// ErrNotInstalled is returned by New when no ffmpeg binary can be
// located on PATH.
var ErrNotInstalled = errors.New("ffmpeg: binary not found on PATH")

// stderrTailBytes caps the amount of ffmpeg stderr included in error
// messages so a chatty failure cannot blow up logs.
const stderrTailBytes = 4096

// waitDelay caps how long Cmd.Wait will keep waiting for the I/O
// goroutines (stdin copy in particular) after the process has been
// killed by ctx cancellation. Without it, a client that hangs mid-
// upload would pin a goroutine forever.
const waitDelay = 2 * time.Second

// =============================================================================

// FFmpeg wraps a resolved ffmpeg binary path and provides transcoding
// helpers used by the bucky SDK.
type FFmpeg struct {
	binary string
}

// New constructs an FFmpeg using the binary at the given path, or the
// first match for DefaultBinary on PATH when path is empty. Returns
// ErrNotInstalled when no binary can be located.
func New(path string) (*FFmpeg, error) {
	if path == "" {
		resolved, err := exec.LookPath(DefaultBinary)
		if err != nil {
			return nil, ErrNotInstalled
		}
		path = resolved
	}

	f := FFmpeg{
		binary: path,
	}

	return &f, nil
}

// Binary returns the resolved binary path being used.
func (f *FFmpeg) Binary() string {
	return f.binary
}

// ToPCM16 reads audio in any ffmpeg-supported container/codec from src
// and writes 16 kHz mono signed 16-bit little-endian PCM to dst. The
// ctx controls cancellation and timeout; cancelling ctx kills the
// ffmpeg process. On non-zero exit the returned error includes the
// tail of ffmpeg's stderr.
func (f *FFmpeg) ToPCM16(ctx context.Context, dst io.Writer, src io.Reader) error {
	args := []string{
		"-hide_banner",
		"-nostats",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-vn",
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ac", "1",
		"-ar", "16000",
		"pipe:1",
	}

	cmd := exec.CommandContext(ctx, f.binary, args...)
	cmd.Stdin = src
	cmd.Stdout = dst
	cmd.WaitDelay = waitDelay

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		tail := tailBytes(stderr.Bytes(), stderrTailBytes)
		if len(tail) == 0 {
			return fmt.Errorf("ffmpeg: %w", err)
		}
		return fmt.Errorf("ffmpeg: %w: %s", err, tail)
	}

	return nil
}

// ToPCM16Bytes is a convenience that reads all of src through ffmpeg
// and returns the resulting raw PCM as a []byte.
func (f *FFmpeg) ToPCM16Bytes(ctx context.Context, src io.Reader) ([]byte, error) {
	var dst bytes.Buffer
	if err := f.ToPCM16(ctx, &dst, src); err != nil {
		return nil, err
	}
	return dst.Bytes(), nil
}

// =============================================================================

// tailBytes returns the last n bytes of b, or b itself when shorter.
func tailBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[len(b)-n:]
}

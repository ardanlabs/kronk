package model

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/ardanlabs/bucky/pkg/audio"
	"github.com/ardanlabs/kronk/sdk/bucky/ffmpeg"
)

// sniffBytes is the size of the prefix read from the input stream to
// identify the container format. Twelve bytes is enough for every
// magic the upstream sniffer checks (RIFF/WAVE, fLaC, ID3, MPEG sync).
const sniffBytes = 12

// =============================================================================

// ffmpeg is resolved once on first use and reused for the lifetime of
// the process. The cost of exec.LookPath is small but pointless to
// repeat on every Decode call.
var (
	ffmpegOnce sync.Once
	ffmpegBin  *ffmpeg.FFmpeg
	ffmpegErr  error
)

func loadFFmpeg() (*ffmpeg.FFmpeg, error) {
	ffmpegOnce.Do(func() {
		ffmpegBin, ffmpegErr = ffmpeg.New("")
	})
	return ffmpegBin, ffmpegErr
}

// =============================================================================

// Decode reads audio in any format the bucky SDK supports and returns
// the 16 kHz mono float32 PCM that Transcribe expects.
//
// WAV, MP3, and FLAC are decoded in-process by the upstream
// github.com/ardanlabs/bucky/pkg/audio package. Anything else (WebM /
// Opus, MP4 / AAC, OGG, M4A, ...) is transcoded to WAV by shelling
// out to ffmpeg via sdk/bucky/ffmpeg. ffmpeg is located once on first
// use and reused for the lifetime of the process.
//
// When ffmpeg is not installed or the transcode fails, Decode returns
// an error that wraps audio.ErrUnsupportedFormat so callers that
// already match the upstream sentinel keep working and the
// user-visible error category remains "unsupported format".
func Decode(ctx context.Context, r io.Reader) ([]float32, error) {
	head := make([]byte, sniffBytes)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("sniff: %w", err)
	}
	head = head[:n]

	combined := io.MultiReader(bytes.NewReader(head), r)

	if isNative(head) {
		return audio.Decode(combined)
	}

	bin, err := loadFFmpeg()
	if err != nil {
		return nil, fmt.Errorf("%w: unknown magic %x: ffmpeg not installed", audio.ErrUnsupportedFormat, head)
	}

	raw, err := bin.ToPCM16Bytes(ctx, combined)
	if err != nil {
		return nil, fmt.Errorf("%w: ffmpeg transcode failed: %v", audio.ErrUnsupportedFormat, err)
	}

	return pcm16ToFloat32(raw), nil
}

// =============================================================================

// isNative reports whether head matches a format the upstream
// bucky/pkg/audio decoder handles natively (WAV, FLAC, MP3 with or
// without an ID3v2 tag).
func isNative(head []byte) bool {
	switch {
	case len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WAVE":
		return true
	case len(head) >= 4 && string(head[0:4]) == "fLaC":
		return true
	case len(head) >= 3 && string(head[0:3]) == "ID3":
		return true
	case len(head) >= 2 && head[0] == 0xFF && (head[1]&0xE0) == 0xE0:
		return true
	}
	return false
}

// pcm16ToFloat32 converts a buffer of signed 16-bit little-endian PCM
// samples to the float32 in [-1, 1] form that whisper.cpp expects. The
// ffmpeg subprocess is invoked with -ar 16000 -ac 1 so the result is
// already mono at the target sample rate.
func pcm16ToFloat32(raw []byte) []float32 {
	n := len(raw) / 2
	out := make([]float32, n)
	for i := range n {
		s := int16(binary.LittleEndian.Uint16(raw[i*2:]))
		out[i] = float32(s) / 32768.0
	}
	return out
}

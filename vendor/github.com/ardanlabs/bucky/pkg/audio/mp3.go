package audio

import (
	"encoding/binary"
	"io"

	gomp3 "github.com/hajimehoshi/go-mp3"
)

// DecodeMP3 decodes an MP3 stream into interleaved float32 samples in
// [-1, 1]. go-mp3 always emits 16-bit little-endian stereo PCM at the
// source's native sample rate, so channels is always 2.
func DecodeMP3(r io.Reader) ([]float32, int, int, error) {
	dec, err := gomp3.NewDecoder(r)
	if err != nil {
		return nil, 0, 0, err
	}

	// Read the entire stream. go-mp3 has no streaming-friendly API for our
	// use case (whisper consumes the full clip in one Full() call anyway).
	raw, err := io.ReadAll(dec)
	if err != nil {
		return nil, 0, 0, err
	}

	n := len(raw) / 2
	out := make([]float32, n)
	for i := range n {
		v := int16(binary.LittleEndian.Uint16(raw[i*2:]))
		out[i] = float32(v) / 32768.0
	}
	return out, dec.SampleRate(), 2, nil
}

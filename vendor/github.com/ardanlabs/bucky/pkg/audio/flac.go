package audio

import (
	"errors"
	"io"

	"github.com/mewkiz/flac"
)

// DecodeFLAC decodes a FLAC stream into interleaved float32 samples in
// [-1, 1].
func DecodeFLAC(r io.Reader) ([]float32, int, int, error) {
	stream, err := flac.New(r)
	if err != nil {
		return nil, 0, 0, err
	}
	defer stream.Close()

	info := stream.Info
	if info == nil {
		return nil, 0, 0, errors.New("audio: FLAC stream has no StreamInfo")
	}
	channels := int(info.NChannels)
	sampleRate := int(info.SampleRate)
	bitsPerSample := info.BitsPerSample
	if channels < 1 {
		return nil, 0, 0, errors.New("audio: FLAC stream has zero channels")
	}
	scale := float32(int64(1) << (bitsPerSample - 1))

	var out []float32
	for {
		frame, err := stream.ParseNext()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, 0, 0, err
		}
		// Each subframe corresponds to one channel; samples are aligned by
		// position. Interleave into the output slice.
		nSamples := frame.Subframes[0].NSamples
		for s := range nSamples {
			for c := range channels {
				v := frame.Subframes[c].Samples[s]
				out = append(out, float32(v)/scale)
			}
		}
	}
	return out, sampleRate, channels, nil
}

// Package speech implements speech conditioning and execution protocols.
// Scheduling and public request/response types belong to model.
package speech

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// ErrInvalidConfig indicates model-specific settings rejected during loading.
var ErrInvalidConfig = errors.New("invalid speech configuration")

// Execution advances a slot's native protocol without owning jobs or scheduling.
// Calls sharing native contexts must be serialized by the engine.
type Execution interface {
	Start(input *mtmd.HelperGenAudioInput) error
	Step(ctx context.Context, promptTokens int) (Output, bool, bool, error)
	Reset() error
	Free()
}

// ExecutionConfig supplies borrowed native handles and immutable slot settings.
// The handles and default speaker reference must outlive the execution.
type ExecutionConfig struct {
	Model          llama.Model
	Context        llama.Context
	Memory         llama.Memory
	Decoder        mtmd.Context
	Input          mtmd.HelperGenAudioInput
	SuppressTokens []llama.Token
	MaxFrames      int
}

// Output contains caller-owned WAV audio and its per-channel sample metadata.
type Output struct {
	Audio      []byte
	SampleRate int
	Samples    int
	Stereo     bool
}

// Prompt supplies text and conditioning to the model's input preparation.
// Voice is the resolved preset name, including the decoder's default voice.
type Prompt struct {
	Text          string
	Language      string
	Voice         string
	ReferenceText string
	MaxFrames     int
}

// readMonoOutput copies and validates the native helper's canonical WAV output.
func readMonoOutput(helper mtmd.HelperGenAudio) (Output, error) {
	output, err := mtmd.HelperGenAudioGetOutput(helper)
	if err != nil {
		return Output{}, err
	}
	data, rate, samples := output.Data, output.SampleRate, output.NSamples
	if len(data) < 44 || samples <= 0 || rate <= 0 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" ||
		binary.LittleEndian.Uint16(data[20:22]) != 1 || binary.LittleEndian.Uint16(data[22:24]) != 1 ||
		binary.LittleEndian.Uint32(data[24:28]) != uint32(rate) || binary.LittleEndian.Uint16(data[34:36]) != 16 ||
		samples != int64((len(data)-44)/2) || int64(binary.LittleEndian.Uint32(data[40:44])) != samples*2 {
		return Output{}, errors.New("speech decoder returned invalid PCM16 mono WAV")
	}
	return Output{Audio: data, SampleRate: int(rate), Samples: int(samples)}, nil
}

package vram

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// SpeechConfig carries resolved speech limits into memory calculation. SDK and
// pool callers populate it automatically; it does not select a model runtime.
type SpeechConfig struct {
	// MaxFrames is the model-specific generated-frame limit. Zero uses 512.
	MaxFrames int64
}

func speechFrames(cfg *SpeechConfig) (int64, error) {
	frames := int64(512)
	if cfg != nil && cfg.MaxFrames != 0 {
		frames = cfg.MaxFrames
	}
	if frames < 1 || frames > 512 {
		return 0, fmt.Errorf("speech max frames must be between 1 and 512, got %d", frames)
	}
	return frames, nil
}

// chatterboxTurboBackboneMemory includes the native helper's persistent host table and
// bounded request buffers in addition to the shared backbone execution arena.
// The table is F32 regardless of the GGUF weight quantization. Native vector
// capacity and sampler storage are allowances, not exact allocator predictions.
func chatterboxTurboBackboneMemory(input Input) ComputeMemory {
	memory := llamaComputeMemory(input)
	frames := int64(512)
	if input.Speech != nil && input.Speech.MaxFrames != 0 {
		frames = input.Speech.MaxFrames
	}
	slots := max(input.Slots, 1)
	embedding := max(input.EmbeddingLength, 0)
	context := max(input.ContextWindow, 1)
	vocabulary := max(input.VocabularySize, 0)
	// A growing prompt vector may coexist with its replacement and the copied
	// final prefill row. Samplers also retain vocabulary-sized candidate data.
	perSlot := 4*vocabulary*embedding + 12*context*embedding + 32*vocabulary + 512*1024
	// Turbo adds three silence tokens. Each produces 960 samples. Reserve the
	// native PCM, helper PCM/WAV, copied Go WAV and vector-capacity overlap.
	perSlot += 24*(frames+3)*960 + 128 + 8*(frames+3)
	memory.WorkspaceCPU += slots * perSlot
	if input.GPULayers == -1 {
		memory.WorkspaceCPU += memory.WorkspaceGPU + memory.RuntimeAllowanceGPU
		memory.WorkspaceGPU, memory.RuntimeAllowanceGPU, memory.MarginGPU = 0, 0, 0
	}
	memory.MarginCPU = memory.WorkspaceCPU / 10
	return memory
}

// chatterboxTurboDecoderResult normalizes the supported family into calculator
// input. Non-Metal backends use provisional allowances based on Metal measurements.
func chatterboxTurboDecoderResult(metadata map[string]string, tensors []gguf.TensorInfo, size int64, cfg Config) (Result, error) {
	for key, want := range map[string]string{
		"clip.gen.audio.embedding_length":     "512",
		"clip.gen.audio.feed_forward_length":  "2048",
		"clip.gen.audio.block_count":          "6",
		"clip.gen.audio.attention.head_count": "8",
		"clip.gen.audio.projection_dim":       "80",
	} {
		if metadata[key] != want {
			return Result{}, fmt.Errorf("speech decoder estimate requires %s=%s, got %q", key, want, metadata[key])
		}
	}
	frames, err := speechFrames(cfg.Speech)
	if err != nil {
		return Result{}, err
	}
	var referenceTokens, voiceBytes int64
	for _, tensor := range tensors {
		if !strings.HasPrefix(tensor.Name, "voice.") {
			continue
		}
		elements := int64(1)
		for _, dimension := range tensor.Dims {
			elements *= dimension
		}
		voiceBytes += 4 * elements
		if strings.HasSuffix(tensor.Name, ".ref_tokens") {
			referenceTokens = max(referenceTokens, elements)
		}
	}
	if referenceTokens <= 0 {
		return Result{}, fmt.Errorf("speech decoder estimate requires preset voice references")
	}
	// The measured envelope covers reference lengths through 250 tokens. Do
	// not extrapolate this calibration to arbitrary embedded reference clips.
	if referenceTokens > 250 {
		return Result{}, fmt.Errorf("speech decoder reference exceeds the calibrated 250-token limit")
	}
	input := Input{
		Architecture: "clip", ModelSizeBytes: size, ComputeBackend: cfg.ComputeBackend,
		ComputeKind:       modelprofile.ComputeChatterboxTurboDecoder,
		NoPersistentCache: true, Slots: max(cfg.Slots, 1), GPULayers: cfg.GPULayers,
		Speech:                &SpeechConfig{MaxFrames: frames},
		speechReferenceTokens: referenceTokens, speechVoiceBytes: voiceBytes,
	}
	return Calculate(input), nil
}

// chatterboxTurboDecoderMemory reserves shared retained flow/vocoder arenas, not an
// arena per sequence. b12223 Metal measurements found 657 MiB at decoder load
// (including approximately 530 MiB of weights), then 531 MiB retained after
// a 512-frame decode. Round the load overhead up to 128 MiB and retain the
// entire measured decode allowance even for smaller frame limits: the source
// establishes quadratic tensors, but not a safe peak-allocation formula.
// This calibrated estimate with margin is not a portable upper bound.
func chatterboxTurboDecoderMemory(input Input) ComputeMemory {
	frames := int64(512)
	if input.Speech != nil && input.Speech.MaxFrames != 0 {
		frames = input.Speech.MaxFrames
	}
	generated := frames + 3
	flowTokens := generated + input.speechReferenceTokens
	samples := 960 * generated
	// Source-derived host waveform arrays coexist with retained graph arenas.
	// At mel=2*generated, STFT/output frames are at most 240*generated+1.
	mel, stft := 2*generated, 240*generated+1
	hostWaveform := 4*(80*mel+mel+samples+36*stft+samples) + 16*(4*stft+16)
	references := input.speechVoiceBytes + max(input.Slots, 1)*4*(161*input.speechReferenceTokens+192)
	// Positional/noise upload scratch and already materialized preset rows.
	host := hostWaveform + 4*512*(4*flowTokens-1) + references
	workspace := int64(531 * 1024 * 1024)
	runtime := int64(128 * 1024 * 1024)
	// Other backends have not been measured. Provisionally add 50% to shared
	// arenas and host scratch before applying the normal 10% margin.
	if !strings.EqualFold(input.ComputeBackend, "metal") {
		workspace += workspace / 2
		runtime += runtime / 2
		host += host / 2
	}
	if input.GPULayers == -1 {
		host += workspace + runtime
		workspace, runtime = 0, 0
	}
	return ComputeMemory{
		WorkspaceGPU: workspace, RuntimeAllowanceGPU: runtime,
		WorkspaceCPU: host, MarginGPU: (workspace + runtime) / 10,
		MarginCPU: host / 10,
	}
}

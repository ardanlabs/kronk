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

// Speech accepts at most 30 seconds of PCM16 mono, 8–48 kHz, in an 8 MiB
// WAV. Keep these fixed admission limits in the reservation even for requests
// without a reference. Each slot retains the caller's WAV and native F32 bitmap.
const speechReferenceSeconds int64 = 30

func speechReferenceAudioMemory(slots, sampleRate int64) int64 {
	// Decode/resample scratch may coexist with the retained bitmap. Reserve
	// two source-rate F32 clips plus a native-rate clip for serialized decoding
	// and preprocessing, independently of the per-slot retained payload.
	return max(slots, 1)*(8*1024*1024+4*speechReferenceSeconds*sampleRate) +
		8*speechReferenceSeconds*48000 + 4*speechReferenceSeconds*sampleRate
}

// speechReferenceConvGraph is an unmeasured graph allowance, not an allocator
// bound. F32 im2col plus two input-sized copies and eight output-sized arrays
// provision convolution, norm/activation, residual and layout intermediates.
// Callers sum stages instead of relying on scheduler lifetime reuse. Backend
// scratch/alignment and non-convolution graph operations still need validation.
func speechReferenceConvGraph(dims []int64, steps int64) int64 {
	return 4 * steps * (dims[0]*dims[1] + 2*dims[1] + 8*dims[2])
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
func chatterboxTurboDecoderResult(tensors []gguf.TensorInfo, size int64, cfg Config) (Result, error) {
	frames, err := speechFrames(cfg.Speech)
	if err != nil {
		return Result{}, err
	}
	var referenceTokens, voiceBytes, referenceGraph, referenceHost int64
	var readbackTemporary int64
	referenceBytes := make(map[string]int64)
	for _, tensor := range tensors {
		voice := strings.HasPrefix(tensor.Name, "voice.")
		readback := strings.HasPrefix(tensor.Name, "a.ve.") || strings.HasPrefix(tensor.Name, "a.cenc.spkr_enc.")
		speaker := strings.HasPrefix(tensor.Name, "a.spk.") && strings.HasSuffix(tensor.Name, ".weight")
		if !voice && !readback && !speaker {
			continue
		}
		elements := int64(1)
		for _, dimension := range tensor.Dims {
			if dimension <= 0 {
				return Result{}, fmt.Errorf("speech reference estimate requires positive %s dimensions, got %v", tensor.Name, tensor.Dims)
			}
			elements *= dimension
		}
		if readback {
			// cbx_ref_cond reads all LSTM/projection weights and biases into
			// coexisting F32 vectors on every encode. cbx_read_tensor also
			// holds one temporary F16 tensor while converting it.
			referenceHost += 4 * elements
			if tensor.GGMLType == 1 {
				readbackTemporary = max(readbackTemporary, 2*elements)
			}
		}
		if speaker && len(tensor.Dims) == 3 {
			// CAMPPlus TDNN/CAM runs after stride-2 time downsampling.
			referenceGraph += speechReferenceConvGraph(tensor.Dims, 1501)
		}
		if speaker && len(tensor.Dims) == 4 {
			// FCM conv2d strides frequency only: 80 -> 40 -> 20 -> 10.
			// Each block's output rate sizes im2col/output arrays; the two
			// input-sized copies also cover a stride-2 block's larger input.
			frequency := int64(80)
			switch {
			case strings.Contains(tensor.Name, ".head.layer1."):
				frequency = 40
			case strings.Contains(tensor.Name, ".head.layer2."):
				frequency = 20
			case tensor.Name == "a.spk.head.conv2.weight":
				frequency = 10
			}
			dims := tensor.Dims
			referenceGraph += 4 * 3001 * frequency * (dims[0]*dims[1]*dims[2] + 2*dims[2] + 8*dims[3])
		}
		if !voice {
			continue
		}
		voiceBytes += 4 * elements
		if strings.HasSuffix(tensor.Name, ".ref_tokens") {
			referenceTokens = max(referenceTokens, elements)
		}
		for _, suffix := range []string{".ref_tokens", ".ref_feat", ".ref_spk"} {
			if voice, ok := strings.CutSuffix(tensor.Name, suffix); ok {
				referenceBytes[voice] += 4 * elements
				break
			}
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
	var largestReference int64
	for _, bytes := range referenceBytes {
		largestReference = max(largestReference, bytes)
	}
	// b12503 caps the flow reference at 10 s (250 codes, 500x80 mel),
	// but Turbo's S3 talker reference uses 15 s (375 codes). Reserve both
	// even if the embedded voice is shorter. S3 has 20 heads of width 64;
	// provision two dense attention arrays and 32 activation rows per width
	// at 375 positions. This added graph allowance is unmeasured, not a
	// reuse of the old flow/vocoder calibration or an allocator upper bound.
	referenceGraph += 4 * (2*20*375*375 + 32*1280*375)
	// CAMPPlus dense concatenations, context gates and statistics pooling:
	// provision 32 arrays at the 1024-channel pooled width, at the TDNN
	// rate. Convolution allowances above additionally cover each CAM layer.
	referenceGraph += 4 * 1501 * 32 * 1024
	// Preprocessing retains five entries: full-clip fbank and voice mel,
	// two S3 mels and flow mel. Allow four copies for materialization and
	// graph uploads; voice mel includes one padded 160-frame partial.
	referenceHost += readbackTemporary + 16*(80*3001+40*(3001+160)+128*(1000+1500)+80*500)
	// CPU LSTM working vectors, speech rows and their output/helper copies.
	referenceHost += 4*(3*256*2+256+4*256) + 16*376*1024
	referenceTokens = max(referenceTokens, 250)
	largestReference = max(largestReference, 4*(250+80*500+192))
	input := Input{
		Architecture: "clip", ModelSizeBytes: size, ComputeBackend: cfg.ComputeBackend,
		ComputeKind:       modelprofile.ComputeChatterboxTurboDecoder,
		NoPersistentCache: true, Slots: max(cfg.Slots, 1), GPULayers: cfg.GPULayers,
		Speech:                &SpeechConfig{MaxFrames: frames},
		speechReferenceTokens: referenceTokens, speechVoiceBytes: voiceBytes,
		speechReferenceBytes: largestReference,
		speechReferenceGraph: referenceGraph, speechReferenceHost: referenceHost,
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
	// b12223 fixes the flow positional width at 512 and mel width at 80,
	// independent of clip.gen.audio metadata. Reference payloads, unlike
	// those graph constants, follow the embedded voice tensor dimensions.
	// Source-derived host waveform arrays coexist with retained graph arenas.
	// At mel=2*generated, STFT/output frames are at most 240*generated+1.
	mel, stft := 2*generated, 240*generated+1
	hostWaveform := 4*(80*mel+mel+samples+36*stft+samples) + 16*(4*stft+16)
	// Presets remain materialized; the shared last-reference payload coexists
	// with the deep copy in every active helper (mtmd_gen_state_set_ref).
	references := input.speechVoiceBytes + (max(input.Slots, 1)+1)*input.speechReferenceBytes
	// Positional/noise upload scratch and already materialized preset rows.
	host := hostWaveform + 4*512*(4*flowTokens-1) + references + input.speechReferenceHost
	host += speechReferenceAudioMemory(input.Slots, 16000)
	workspace := int64(531*1024*1024) + input.speechReferenceGraph
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

package vram

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// qwen3TTSBackboneMemory includes the helper's combined text/codec embedding
// table, retained prompt/output capacities and serialized embedding readback.
// b12223 dequantizes token_embd.weight into an F32 host table per helper.
func qwen3TTSBackboneMemory(input Input) ComputeMemory {
	memory := llamaComputeMemory(input)
	frames := int64(512)
	if input.Speech != nil && input.Speech.MaxFrames != 0 {
		frames = input.Speech.MaxFrames
	}
	embedding, vocabulary := max(input.EmbeddingLength, 0), max(input.VocabularySize, 0)
	// Prompt row vectors coexist with their contiguous copy. Output allowances
	// cover native PCM/vector growth, WAV16, and the caller-owned Go WAV copy.
	// These capacity and sampler allowances are estimates, not allocator bounds.
	perSlot := 4*vocabulary*embedding + 12*max(input.ContextWindow, 1)*embedding
	perSlot += 32*vocabulary + 512*1024 + 24*frames*1920 + 128
	memory.WorkspaceCPU += max(input.Slots, 1)*perSlot + input.speechEmbeddingBytes
	if input.GPULayers == -1 {
		memory.WorkspaceCPU += memory.WorkspaceGPU + memory.RuntimeAllowanceGPU
		memory.WorkspaceGPU, memory.RuntimeAllowanceGPU, memory.MarginGPU = 0, 0, 0
	}
	memory.MarginCPU = memory.WorkspaceCPU / 10
	return memory
}

// qwen3TTSDecoderResult checks the b12223 1.7B Base decoder layout used for
// calibration. Its 72-frame codec graph is fixed regardless of MaxFrames.
func qwen3TTSDecoderResult(metadata map[string]string, tensors []gguf.TensorInfo, size int64, cfg Config) (Result, error) {
	for key, want := range map[string]string{
		"clip.audio.projector_type": "qwen3tts_spkenc", "clip.has_audio_encoder": "true",
		"clip.audio.embedding_length": "1536", "clip.audio.block_count": "3",
		"clip.audio.attention.head_count": "1", "clip.audio.projection_dim": "2048",
		"clip.gen.audio.embedding_length": "1024", "clip.gen.audio.block_count": "5",
		"clip.gen.audio.feed_forward_length": "3072", "clip.gen.audio.projection_dim": "2048",
		"clip.gen.audio.attention.head_count": "16", "clip.gen.audio.attention.head_count_kv": "8",
	} {
		if metadata[key] != want {
			return Result{}, fmt.Errorf("Qwen3-TTS decoder estimate requires %s=%s, got %q", key, want, metadata[key])
		}
	}
	frames, err := speechFrames(cfg.Speech)
	if err != nil {
		return Result{}, err
	}
	dimensions := make(map[string][]int64, len(tensors))
	for _, tensor := range tensors {
		dimensions[tensor.Name] = tensor.Dims
	}
	// Each entry checks a calibrated graph shape and gives its persistent
	// F32 state payload in elements. Native backend slots are aligned below.
	type codecState struct {
		name     string
		dims     []int64
		elements int64
	}
	states := []codecState{
		{"a.gen.code.head.weight", []int64{1024, 2048, 15}, 0},
		{"a.gen.code.out_embd.weight", []int64{2048, 3072}, 0},
		{"a.gen.wav.tfm.blk.0.attn_q.weight", []int64{512, 1024}, 0},
		{"a.gen.wav.tfm.blk.7.attn_q.weight", []int64{512, 1024}, 0},
		{"a.gen.wav.pre_conv.weight", []int64{3, 512, 1024}, 2 * 512},
		{"a.gen.wav.up.blk.0.dwconv.weight", []int64{7, 1, 1024}, 6 * 1024},
		{"a.gen.wav.up.blk.1.dwconv.weight", []int64{7, 1, 1024}, 6 * 1024},
		{"a.gen.wav.dac.entry.weight", []int64{7, 1024, 1536}, 6 * 1024},
		{"a.gen.wav.dac.post_conv.weight", []int64{7, 96, 1}, 6 * 96},
	}
	stateBytes := int64(4 * (1 + 2*8*1024*71)) // position counter and eight F32 K/V pairs
	for block, stride := range []int64{8, 5, 4, 3} {
		width := int64(768) >> block
		states = append(states, codecState{
			fmt.Sprintf("a.gen.wav.dac.blk.%d.conv.weight", block), []int64{2 * stride, width, 2 * width}, stride * width,
		})
		for residual, dilation := range []int64{1, 3, 9} {
			states = append(states, codecState{
				fmt.Sprintf("a.gen.wav.dac.blk.%d.res.%d.conv1.weight", block, residual), []int64{7, width, width}, 6 * dilation * width,
			})
		}
	}
	for _, state := range states {
		if !slices.Equal(dimensions[state.name], state.dims) {
			return Result{}, fmt.Errorf("Qwen3-TTS decoder estimate requires %s dimensions %v, got %v", state.name, state.dims, dimensions[state.name])
		}
		stateBytes += 4 * state.elements
	}
	// Native allocates 38 F32 slots; allow alignment and tensor bookkeeping.
	stateBytes += 16 * 1024
	return Calculate(Input{
		Architecture: "clip", ModelSizeBytes: size, ComputeBackend: cfg.ComputeBackend,
		ComputeKind: modelprofile.ComputeQwen3TTSDecoder, NoPersistentCache: true,
		Slots: max(cfg.Slots, 1), GPULayers: cfg.GPULayers,
		Speech: &SpeechConfig{MaxFrames: frames}, speechCodecStateBytes: stateBytes,
	}), nil
}

// qwen3TTSDecoderMemory reserves shared predictor/codec and speaker-encoder
// arenas, plus per-helper codec state. Predictor KV is graph-local, not another
// per-helper cache. b12223 Metal measured 869.28 MiB generation and 140.66 MiB
// speaker arenas; the 512-frame run also had transient allocation peaks.
// Reserve a 1536 MiB shared envelope plus 128 MiB runtime and 10% margin.
// This calibration is not a portable allocator upper bound. A smaller frame
// cap cannot shrink the fixed 72-frame graph or retained scheduler buffers.
func qwen3TTSDecoderMemory(input Input) ComputeMemory {
	workspace := int64(1536 * 1024 * 1024)
	runtime := int64(128 * 1024 * 1024)
	// Both subcontexts retain host workspace, and each active state retains
	// one full-window waveform readback even when the last window is partial.
	host := int64(32*1024*1024) + max(input.Slots, 1)*(4*72*1920+256*1024)
	// Other backends have not been measured. Provisionally add 50% to shared
	// arenas and host scratch before applying the normal 10% margin.
	// Codec state has source-derived dimensions and is not scaled with arenas.
	if !strings.EqualFold(input.ComputeBackend, "metal") {
		workspace += workspace / 2
		runtime += runtime / 2
		host += host / 2
	}
	workspace += max(input.Slots, 1) * input.speechCodecStateBytes
	if input.GPULayers == -1 {
		host += workspace + runtime
		workspace, runtime = 0, 0
	}
	return ComputeMemory{
		WorkspaceGPU: workspace, RuntimeAllowanceGPU: runtime, WorkspaceCPU: host,
		MarginGPU: (workspace + runtime) / 10, MarginCPU: host / 10,
	}
}

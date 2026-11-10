package vram

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// qwen3TTSWindowFrames matches b12223's native codec sliding window and helper flush.
const qwen3TTSWindowFrames int64 = 72

// qwen3TTSBackboneMemory includes the helper's combined text/codec embedding
// table, retained prompt capacities and serialized embedding readback.
// b12223 dequantizes token_embd.weight into an F32 host table per helper.
func qwen3TTSBackboneMemory(input Input) ComputeMemory {
	memory := llamaComputeMemory(input)
	embedding, vocabulary := max(input.EmbeddingLength, 0), max(input.VocabularySize, 0)
	// Prompt row vectors coexist with their contiguous copy. These capacity
	// and sampler allowances are estimates, not allocator bounds. Waveform
	// buffers are sized with the decoder, which determines samples per frame.
	perSlot := 4*vocabulary*embedding + 12*max(input.ContextWindow, 1)*embedding
	perSlot += 32*vocabulary + 512*1024
	memory.WorkspaceCPU += max(input.Slots, 1)*perSlot + input.speechEmbeddingBytes
	if input.GPULayers == -1 {
		memory.WorkspaceCPU += memory.WorkspaceGPU + memory.RuntimeAllowanceGPU
		memory.WorkspaceGPU, memory.RuntimeAllowanceGPU, memory.MarginGPU = 0, 0, 0
	}
	memory.MarginCPU = memory.WorkspaceCPU / 10
	return memory
}

// qwen3TTSDecoderResult derives codec state and waveform sizes from tensors.
// b12223 fixes the codec topology and window, not its allocation tensor widths.
func qwen3TTSDecoderResult(tensors []gguf.TensorInfo, size int64, cfg Config) (Result, error) {
	frames, err := speechFrames(cfg.Speech)
	if err != nil {
		return Result{}, err
	}
	dimensions := make(map[string][]int64, len(tensors))
	for _, tensor := range tensors {
		dimensions[tensor.Name] = tensor.Dims
	}
	// Only rank and positive dimensions are required for sizing. Do not turn
	// the inspected model's widths/kernels or predictor metadata into an allowlist.
	required := map[string]int{
		"a.gen.wav.tfm.blk.0.attn_q.weight": 2,
		"a.gen.wav.pre_conv.weight":         3,
		"a.gen.wav.dac.entry.weight":        3,
		"a.gen.wav.dac.post_conv.weight":    3,
	}
	for block := range 2 {
		required[fmt.Sprintf("a.gen.wav.up.blk.%d.dwconv.weight", block)] = 3
	}
	for block := range 4 {
		required[fmt.Sprintf("a.gen.wav.dac.blk.%d.conv.weight", block)] = 3
		for residual := range 3 {
			required[fmt.Sprintf("a.gen.wav.dac.blk.%d.res.%d.conv1.weight", block, residual)] = 3
		}
	}
	for name, rank := range required {
		dims := dimensions[name]
		if len(dims) != rank {
			return Result{}, fmt.Errorf("Qwen3-TTS decoder estimate requires %s with %d dimensions, got %v", name, rank, dims)
		}
		for _, dim := range dims {
			if dim <= 0 {
				return Result{}, fmt.Errorf("Qwen3-TTS decoder estimate requires positive %s dimensions, got %v", name, dims)
			}
		}
	}
	// Mirror list_c2w_state_slots: b12223 fixes eight layers and 16 query/KV
	// heads; channel width comes from the query tensor, not predictor metadata.
	kvChannels := dimensions["a.gen.wav.tfm.blk.0.attn_q.weight"][1] / 16 * 16
	stateBytes := 4 * (1 + 2*8*kvChannels*(qwen3TTSWindowFrames-1))
	for _, name := range []string{"a.gen.wav.pre_conv.weight", "a.gen.wav.dac.entry.weight", "a.gen.wav.dac.post_conv.weight"} {
		dims := dimensions[name]
		stateBytes += 4 * (dims[0] - 1) * dims[1]
	}
	samplesPerFrame := int64(1)
	for block := range 2 {
		dims := dimensions[fmt.Sprintf("a.gen.wav.up.blk.%d.dwconv.weight", block)]
		stateBytes += 4 * (dims[0] - 1) * dims[2]
		samplesPerFrame *= 2 // native upsample stride
	}
	for block := range 4 {
		dims := dimensions[fmt.Sprintf("a.gen.wav.dac.blk.%d.conv.weight", block)]
		stride := dims[0] / 2
		stateBytes += 4 * stride * dims[1]
		samplesPerFrame *= stride
		for residual, dilation := range []int64{1, 3, 9} {
			dims := dimensions[fmt.Sprintf("a.gen.wav.dac.blk.%d.res.%d.conv1.weight", block, residual)]
			stateBytes += 4 * (dims[0] - 1) * dilation * dims[1]
		}
	}
	// Native allocates 38 F32 slots; allow alignment and tensor bookkeeping.
	stateBytes += 16 * 1024
	return Calculate(Input{
		Architecture: "clip", ModelSizeBytes: size, ComputeBackend: cfg.ComputeBackend,
		ComputeKind: modelprofile.ComputeQwen3TTSDecoder, NoPersistentCache: true,
		Slots: max(cfg.Slots, 1), GPULayers: cfg.GPULayers,
		Speech: &SpeechConfig{MaxFrames: frames}, speechCodecStateBytes: stateBytes,
		speechSamplesPerFrame: samplesPerFrame,
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
	frames := int64(512)
	if input.Speech != nil && input.Speech.MaxFrames != 0 {
		frames = input.Speech.MaxFrames
	}
	workspace := int64(1536 * 1024 * 1024)
	runtime := int64(128 * 1024 * 1024)
	// Both subcontexts retain host workspace, and each active state retains
	// one full-window waveform readback even when the last window is partial.
	host := int64(32*1024*1024) + max(input.Slots, 1)*(4*qwen3TTSWindowFrames*input.speechSamplesPerFrame+256*1024)
	// Other backends have not been measured. Provisionally add 50% to shared
	// arenas and host scratch before applying the normal 10% margin.
	// Codec state has source-derived dimensions and is not scaled with arenas.
	if !strings.EqualFold(input.ComputeBackend, "metal") {
		workspace += workspace / 2
		runtime += runtime / 2
		host += host / 2
	}
	// Native PCM/vector growth, WAV16, and the caller-owned Go WAV copy.
	// Output scales with the requested frame cap, not the fixed codec window.
	host += max(input.Slots, 1) * (24*frames*input.speechSamplesPerFrame + 128)
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

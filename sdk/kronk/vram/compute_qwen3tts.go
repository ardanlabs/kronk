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
	// b12503 speaker mel uses a 256-sample hop at 24 kHz. Reserve the
	// entire accepted clip, not the old speaker warmup length. Pooling has
	// repeated mean/std, concatenation, attention and weighted-stat arrays;
	// allow 32 full-width F32 arrays in addition to convolution intermediates.
	melFrames := speechReferenceSeconds*24000/256 + 1
	speakerWidth := int64(1536)
	if dims := dimensions["a.conv_out.weight"]; len(dims) == 3 {
		speakerWidth = max(speakerWidth, dims[2])
	}
	referenceGraph := 4 * melFrames * 32 * speakerWidth
	for name, dims := range dimensions {
		if strings.HasPrefix(name, "a.") && !strings.HasPrefix(name, "a.gen.") &&
			!strings.HasPrefix(name, "a.seanet.") && !strings.HasPrefix(name, "a.downsample.") &&
			strings.HasSuffix(name, ".weight") && len(dims) == 3 {
			referenceGraph += speechReferenceConvGraph(dims, melFrames)
		}
	}
	// Four mel copies plus padded waveform/STFT preprocessing scratch. The
	// audio decoder/bitmap and per-slot accepted WAV are reserved separately.
	referenceHost := 16*128*melFrames + 8*speechReferenceSeconds*24000
	var referenceBytes int64
	if marker, ok := dimensions["a.gen.enc.quant.rest.codebook.weight"]; ok {
		// Native has_ref_encoder() uses this marker, not has_audio_encoder.
		// Old projectors still pay for speaker conditioning, but never for
		// the absent SEANet/transformer/RVQ reference encoder graph.
		if len(marker) != 3 || marker[0] <= 0 || marker[1] <= 0 || marker[2] <= 0 {
			return Result{}, fmt.Errorf("Qwen3-TTS reference estimate requires a positive rank-3 codebook, got %v", marker)
		}
		// SEANet strides 4,5,6,8: 720000 samples -> 750 transformer
		// positions -> 375 reference frames after stride-2 downsampling.
		// Sum im2col/layout/activation allowances at each stage's own rate,
		// without assuming graph allocator lifetime reuse. These are
		// unmeasured graph allowances, not calibrated allocator bounds.
		steps := speechReferenceSeconds * 24000
		convs := map[string]int64{"a.seanet.conv_in.weight": steps}
		for block, stride := range []int64{4, 5, 6, 8} {
			for _, kind := range []string{"res_conv1", "res_conv2"} {
				convs[fmt.Sprintf("a.seanet.blk.%d.%s.weight", block, kind)] = steps
			}
			steps = (steps + stride - 1) / stride
			convs[fmt.Sprintf("a.seanet.blk.%d.scale_conv.weight", block)] = steps
		}
		convs["a.seanet.conv_out.weight"] = steps
		convs["a.downsample.conv.weight"] = (steps + 1) / 2
		for name, length := range convs {
			dims := dimensions[name]
			if len(dims) != 3 || dims[0] <= 0 || dims[1] <= 0 || dims[2] <= 0 {
				return Result{}, fmt.Errorf("Qwen3-TTS reference estimate requires positive rank-3 %s, got %v", name, dims)
			}
			referenceGraph += speechReferenceConvGraph(dims, length)
		}
		for name, dims := range dimensions {
			if strings.HasPrefix(name, "a.gen.enc.tfm.") && strings.HasSuffix(name, ".weight") && len(dims) == 2 {
				// Four F32 arrays per projected width provision projection,
				// normalization/activation and residual/layout intermediates.
				referenceGraph += 16 * steps * dims[1]
				if strings.HasSuffix(name, ".attn_q.weight") {
					// Eight heads; explicitly allow score and softmax arrays
					// even on backends that can fuse attention.
					referenceGraph += 8 * 8 * steps * steps
				}
			}
		}
		refFrames := (steps + 1) / 2
		// RVQ distances and reductions across all codebooks, plus readback
		// groups and their frame-major copy. Include native output, helper
		// speaker/ref rows and prompt-construction overlap in host scratch.
		first := dimensions["a.gen.enc.quant.first.codebook.weight"]
		if len(first) != 2 || first[0] <= 0 || first[1] <= 0 {
			return Result{}, fmt.Errorf("Qwen3-TTS reference estimate requires a positive rank-2 first codebook, got %v", first)
		}
		referenceGraph += 8*refFrames*(first[1]+marker[1]*marker[2]) + 4*steps*steps
		referenceBytes = 4 * refFrames * (marker[2] + 1)
		width := int64(2048)
		if dims := dimensions["a.gen.code.out_embd.weight"]; len(dims) == 2 {
			width = dims[0]
		}
		referenceGraph += 8 * refFrames * width
		referenceHost += 4*steps*steps + 16*(refFrames+1)*width + 2*referenceBytes + 8*speechReferenceSeconds*24000
	}
	return Calculate(Input{
		Architecture: "clip", ModelSizeBytes: size, ComputeBackend: cfg.ComputeBackend,
		ComputeKind: modelprofile.ComputeQwen3TTSDecoder, NoPersistentCache: true,
		Slots: max(cfg.Slots, 1), GPULayers: cfg.GPULayers,
		Speech: &SpeechConfig{MaxFrames: frames}, speechCodecStateBytes: stateBytes,
		speechSamplesPerFrame: samplesPerFrame,
		speechReferenceGraph:  referenceGraph, speechReferenceHost: referenceHost,
		speechReferenceBytes: referenceBytes,
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
	workspace := int64(1536*1024*1024) + input.speechReferenceGraph
	runtime := int64(128 * 1024 * 1024)
	// Both subcontexts retain host workspace, and each active state retains
	// one full-window waveform readback even when the last window is partial.
	host := int64(32*1024*1024) + max(input.Slots, 1)*(4*qwen3TTSWindowFrames*input.speechSamplesPerFrame+256*1024)
	// Shared last reference plus deep-copied reference codes per helper.
	// The retained input bitmap and WAV are independent of generated output.
	host += input.speechReferenceHost + (max(input.Slots, 1)+1)*input.speechReferenceBytes
	host += speechReferenceAudioMemory(input.Slots, 24000)
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

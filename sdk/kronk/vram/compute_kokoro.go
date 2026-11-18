package vram

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// kokoroSegmentTokens includes both BOS pads. b12503 runs whole, noncausal
// segments; runtime admission must limit each request to 510 mapped phonemes.
const kokoroSegmentTokens int64 = 512

func kokoroBackboneMemory(input Input) ComputeMemory {
	tokens := min(kokoroSegmentTokens, max(input.ContextWindow, 1))
	if input.NUBatch > 0 {
		tokens = min(tokens, input.NUBatch)
	}
	embedding, vocabulary := max(input.EmbeddingLength, 1), max(input.VocabularySize, 1)
	ffn := input.kokoroFFNLength
	if ffn <= 0 {
		ffn = 4 * embedding
	}
	// Sum all layer intermediates without relying on allocator lifetime reuse:
	// 16 hidden arrays, four FFN arrays, F32 attention scores + softmax per
	// head, and a full mask. Tied ALBERT weights do not tie activations.
	graph := max(input.BlockCount, 1) * (4*tokens*(16*embedding+4*ffn) + 8*max(input.HeadCount, 1)*tokens*tokens)
	graph += 4*tokens*tokens + 4*tokens*(embedding+vocabulary)
	// Native all-token embeddings/logits, helper segment vector growth and
	// its copied decoder input coexist. No dequantized vocabulary cache or
	// reference-audio encoder exists in this pipeline.
	host := 4*tokens*(embedding+vocabulary) + max(input.Slots, 1)*(16*tokens*embedding+16*tokens+4096)
	return kokoroMemory(input, graph, host)
}

// kokoroDecoderResult uses b12503's three serialized graphs and host prosody.
// The graph allowance is provisional: a Metal observation plus source-based
// liveness scaling, not a guarantee for every possible duration or backend.
func kokoroDecoderResult(tensors []gguf.TensorInfo, size int64, cfg Config) (Result, error) {
	dimensions := make(map[string][]int64, len(tensors))
	for _, tensor := range tensors {
		dimensions[tensor.Name] = tensor.Dims
	}
	for name, rank := range map[string]int{
		"a.gen.pred.duration_proj.linear_layer.weight": 2,
		"a.gen.bert_enc.weight":                        2,
		"a.gen.text.embedding.weight":                  2,
		"a.gen.dec.generator.ups.0.weight":             3,
		"a.gen.dec.generator.ups.1.weight":             3,
		"a.gen.dec.generator.conv_post.weight":         3,
	} {
		dims := dimensions[name]
		if len(dims) != rank {
			return Result{}, fmt.Errorf("kokoro decoder estimate requires %s with %d dimensions, got %v", name, rank, dims)
		}
	}
	for _, tensor := range tensors {
		for _, dim := range tensor.Dims {
			if dim <= 0 {
				return Result{}, fmt.Errorf("kokoro decoder estimate requires positive %s dimensions, got %v", tensor.Name, tensor.Dims)
			}
		}
	}
	// Kronk leaves HelperGenAudioInputDefault's speed at exactly 1.0.
	// Sum(sigmoid(duration logits)) <= n_bins, rounded per padded token.
	// Keep this bound for retained output and host buffers, independently
	// of the calibrated graph estimate. MaxFrames is autoregressive-only.
	frames := kokoroSegmentTokens * dimensions["a.gen.pred.duration_proj.linear_layer.weight"][1]
	// F0 has 2*frames steps, 300 samples per step, STFT hop 5. Native
	// generator strides come from the two transposed-convolution kernels.
	up0 := dimensions["a.gen.dec.generator.ups.0.weight"][0] / 2
	up1 := dimensions["a.gen.dec.generator.ups.1.weight"][0] / 2
	if up0 != 10 || up1 != 6 {
		return Result{}, fmt.Errorf("kokoro decoder estimate requires native generator strides 10 and 6, got %d and %d", up0, up1)
	}
	samples := 2 * frames * 300
	harmonicRows := samples/5 + 1
	var readback, voice int64
	for _, tensor := range tensors {
		name, dims := tensor.Name, tensor.Dims
		if strings.HasPrefix(name, "voice.") && len(dims) == 2 {
			voice = max(voice, 4*dims[0]*dims[1])
		}
		if !strings.HasPrefix(name, "a.gen.") {
			continue
		}
		if len(dims) != 3 {
			// Host LSTMs/linear layers read weights as F32 even when the
			// model is quantized. Summing all reads overestimates overlap.
			elements := int64(1)
			for _, dim := range dims {
				elements *= dim
			}
			readback += 4 * elements
		}
	}
	// b12503 Metal standalone smoke: two slots, default US and bf_emma,
	// numbers/POS/unknown words and 509 mapped phonemes. The largest output
	// was 610200 samples = 1017 duration frames; process peak RSS was
	// 711032832 bytes. This includes both models, not an isolated decoder
	// arena. Use it as a conservative floor, not as a worst-case proof.
	// Evaluate source liveness at twice that observed duration for headroom.
	// Longer predicted durations can exceed this provisional graph estimate.
	const observedFrames int64 = 610200 / 600
	const observedRSS int64 = 711032832
	graph := max(observedRSS, kokoroDecoderWorkspace(dimensions, min(frames, 2*observedFrames)))
	// encode_kokoro retains text CNN, shared LSTM/ASR and F0/energy arrays
	// through the call. Add the maximum host phase, not all temporaries from
	// prosody, harmonic synthesis and ISTFT simultaneously. Waveform output
	// is accounted separately per helper, including vector growth.
	// ASR alignment uses push_back/insert growth, unlike the exact-sized
	// shared LSTM output: reserve two ASR capacities and one shared slab.
	retained := 4 * (kokoroSegmentTokens*512 + frames*(3*512+4))
	prosody := retained + readback + 8*frames*640 + 4*kokoroSegmentTokens*(640+512)
	harmonic := retained + 4*18*frames + 4*samples + 4*22*harmonicRows
	vocode := retained + 8*22*harmonicRows
	istft := vocode + 8*(samples+20)
	host := max(prosody, harmonic, vocode, istft)
	return Calculate(Input{
		Architecture: "clip", ModelSizeBytes: size, ComputeBackend: cfg.ComputeBackend,
		ComputeKind: modelprofile.ComputeKokoroDecoder, NoPersistentCache: true,
		Slots: max(cfg.Slots, 1), GPULayers: cfg.GPULayers, Speech: &SpeechConfig{},
		kokoroGraphBytes: graph, kokoroHostBytes: host, kokoroOutputSamples: samples,
		speechVoiceBytes: voice,
	}), nil
}

// kokoroDecoderWorkspace approximates the peak live arena at a specified
// duration. ggml-alloc.c frees parents after their last consumer; the same
// scheduler resets between TEXT/F0N/DEC. Earlier layers and stages are not
// simultaneously live. Fragmentation/backend differences remain approximate.
func kokoroDecoderWorkspace(dimensions map[string][]int64, frames int64) int64 {
	// ggml_conv_1d uses F16 im2col (F32 only for BF16 weights, which Kokoro
	// does not use). Allow six F32 channel slabs: residual, branch input,
	// branch accumulator, normalization/snake intermediates and output/layout.
	// These local slabs cover a residual unit; they are not charged once for
	// every convolution. Only actual graph convolution weights are visited.
	var peak int64
	conv := func(name string, length int64) {
		dims := dimensions[name+".weight"]
		if len(dims) == 3 {
			peak = max(peak, 2*length*dims[0]*dims[1]+24*length*max(dims[1], dims[2]))
		}
	}
	residual := func(name string, length int64) int64 {
		if _, ok := dimensions[name+".pool.weight"]; ok {
			length *= 2
		}
		for _, suffix := range []string{".conv1", ".conv2", ".conv1x1"} {
			conv(name+suffix, length)
		}
		return length
	}
	for i := 0; ; i++ {
		name := fmt.Sprintf("a.gen.text.cnn.%d.0", i)
		if _, ok := dimensions[name+".weight"]; !ok {
			break
		}
		conv(name, kokoroSegmentTokens)
	}
	text := peak + 4*kokoroSegmentTokens
	peak = 0
	for _, curve := range []string{"F0", "N"} {
		length := frames
		for block := range 3 {
			length = residual(fmt.Sprintf("a.gen.pred.%s.%d", curve, block), length)
		}
		conv("a.gen.pred."+curve+"_proj", length)
	}
	// Input and transposed input coexist; both F0/N outputs remain live.
	f0n := peak + 8*frames*512 + 16*frames
	peak = 0
	for _, name := range []string{"F0_conv", "N_conv", "asr_res.0"} {
		conv("a.gen.dec."+name, frames)
	}
	length := residual("a.gen.dec.encode", frames)
	for block := 0; ; block++ {
		name := fmt.Sprintf("a.gen.dec.decode.%d", block)
		if _, ok := dimensions[name+".conv1.weight"]; !ok {
			break
		}
		length = residual(name, length)
	}
	for block := range 2 {
		prefix := "a.gen.dec.generator."
		dims := dimensions[fmt.Sprintf("%sups.%d.weight", prefix, block)]
		stride := dims[0] / 2
		raw := (length-1)*stride + dims[0]
		// ggml_conv_transpose_1d is a direct op, not im2col/mul_mat.
		// Account input, raw output, trimmed contiguous output and bias copy.
		peak = max(peak, 4*(length*dims[2]+3*raw*dims[1]))
		length = raw - 2*(stride/2)
		if block == 1 {
			length++ // final generator stage adds left reflect padding
		}
		conv(fmt.Sprintf("%snoise_convs.%d", prefix, block), length)
		for branch := range 4 {
			name := fmt.Sprintf("%snoise_res.%d", prefix, block)
			if branch > 0 {
				name = fmt.Sprintf("%sresblocks.%d", prefix, block*3+branch-1)
			}
			for unit := range 3 {
				conv(fmt.Sprintf("%s.convs1.%d", name, unit), length)
				conv(fmt.Sprintf("%s.convs2.%d", name, unit), length)
			}
		}
	}
	conv("a.gen.dec.generator.conv_post", length)
	// Explicit DEC inputs plus their contiguous layouts. Har has 22 channels,
	// 120*frames+1 rows. ASR is 512-wide; F0 and energy each have 2*frames.
	decoder := peak + 8*(512*frames+4*frames+22*(120*frames+1))
	return max(text, f0n, decoder)
}

func kokoroDecoderMemory(input Input) ComputeMemory {
	// Each helper retains PCM (including vector growth), WAV16 and the
	// caller's WAV copy; native gen_state retains a separate waveform.
	// The largest complete voice table is read while constructing input.
	host := input.kokoroHostBytes + input.speechVoiceBytes
	host += max(input.Slots, 1) * (24*input.kokoroOutputSamples + 256*1024)
	graph := input.kokoroGraphBytes
	if !strings.EqualFold(input.ComputeBackend, "metal") {
		// Only Metal has observational evidence. Other backends receive
		// provisional 50% graph headroom, not a claim of equivalent liveness.
		graph += graph / 2
	}
	return kokoroMemory(input, graph, host)
}

func kokoroMemory(input Input, graph, host int64) ComputeMemory {
	// Provisional backend/bookkeeping allowance, independent of Qwen/Turbo.
	runtime := int64(32 * 1024 * 1024)
	if input.GPULayers == -1 {
		host += graph + runtime
		graph, runtime = 0, 0
	}
	if input.ComputeKind == modelprofile.ComputeKokoroBackbone && input.GPULayers > 0 && input.GPULayers <= input.BlockCount {
		// The tied layer remains on CPU during partial offload. Retain
		// both arenas rather than assuming a backend can share storage.
		host += graph + runtime
	}
	return ComputeMemory{
		WorkspaceGPU: graph, RuntimeAllowanceGPU: runtime, WorkspaceCPU: host,
		MarginGPU: (graph + runtime) / 10, MarginCPU: host / 10,
	}
}

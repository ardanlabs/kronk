package vram

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// b12503 (4577932fab59), models/models.h and mtmd.cpp: duration <= 400
// frames/token at speed 1; successful utterances have <= 3000 frames; hop 256.
// Runtime admission must enforce BOTH padded bounds, including sentence pads
// and any inserted full stops, not merely count unpadded input symbols.
const (
	tinyVocaSegmentTokens int64 = 512
	tinyVocaRequestTokens int64 = 3 * 509
	tinyVocaMaxFrames     int64 = 3000
	tinyVocaOutputSamples int64 = tinyVocaRequestTokens * 400 * 256
)

func tinyVocaSpeechLimits(cfg *SpeechConfig) error {
	if cfg != nil && cfg.MaxFrames != 0 && cfg.MaxFrames != 512 {
		return fmt.Errorf("tiny-voca requires default speech max frames 512; it is not an autoregressive output cap")
	}
	return nil
}

// tinyVocaStackGraph sums per-layer intermediates rather than assuming arena
// reuse or flash attention. The 7-tap depthwise gather, product and contiguous
// permutation need three 7*width slabs, unlike Kokoro's ALBERT graph.
// This is an unmeasured allowance, not an allocator high-water guarantee.
func tinyVocaStackGraph(rows, width, ffn, heads, layers, vocabulary int64) int64 {
	graph := layers * (4*rows*((16+3*7)*width+4*ffn) + 8*heads*rows*rows)
	return graph + 4*rows*rows + 4*rows*(width+vocabulary) + 4*7*rows
}

func tinyVocaBackboneMemory(input Input) ComputeMemory {
	width, vocabulary := max(input.EmbeddingLength, 1), max(input.VocabularySize, 1)
	ffn := input.tinyVocaFFNLength
	if ffn <= 0 {
		ffn = 4 * width
	}
	graph := tinyVocaStackGraph(tinyVocaSegmentTokens, width, ffn, max(input.HeadCount, 1), max(input.BlockCount, 1), vocabulary)
	// All-token logits/embedding readout and segment vector capacities. clear()
	// retains input storage while feats grows; include two embedding capacities.
	host := 4*tinyVocaSegmentTokens*(width+vocabulary) + max(input.Slots, 1)*(24*tinyVocaRequestTokens*width+32*tinyVocaRequestTokens)
	return tinyVocaMemory(input, graph, host)
}

func tinyVocaDecoderResult(metadata map[string]string, tensors []gguf.TensorInfo, size int64, cfg Config) (Result, error) {
	if err := tinyVocaSpeechLimits(cfg.Speech); err != nil {
		return Result{}, err
	}
	var width, ffn, heads int64
	for key, dst := range map[string]*int64{
		"clip.gen.audio.embedding_length":     &width,
		"clip.gen.audio.feed_forward_length":  &ffn,
		"clip.gen.audio.attention.head_count": &heads,
	} {
		value, err := gguf.ParseInt64(metadata, key)
		if err != nil || value <= 0 {
			return Result{}, fmt.Errorf("tiny-voca decoder estimate requires positive %s", key)
		}
		*dst = value
	}
	dimensions := make(map[string][]int64, len(tensors))
	var hostTables int64
	for _, tensor := range tensors {
		elements := int64(1)
		for _, dim := range tensor.Dims {
			if dim <= 0 {
				return Result{}, fmt.Errorf("tiny-voca decoder estimate requires positive %s dimensions", tensor.Name)
			}
			elements *= dim
		}
		dimensions[tensor.Name] = tensor.Dims
		// Conservatively charge an F32 host copy of every tensor. This
		// covers shared G2P and prosody tables even for quantized fixtures.
		hostTables += 4 * elements
	}
	// The installed ggml-org F32 fixture has 76 symbols and a 128-wide
	// duration predictor. Reject other layouts rather than silently apply
	// these provisional head allowances to an unverified decoder family.
	for name, want := range map[string][]int64{
		"a.gen.tok_embd.weight":     {width, 76},
		"a.gen.stress_embd.weight":  {width, 3},
		"a.gen.dur.conv1.weight":    {3, width, 128},
		"a.gen.dur.conv2.weight":    {3, 128, 128},
		"a.gen.dur.out.weight":      {128, 2},
		"a.gen.voc.harmonic.weight": {width, 98},
		"a.gen.voc.noise.weight":    {width, 36},
	} {
		got := dimensions[name]
		if len(got) != len(want) {
			return Result{}, fmt.Errorf("tiny-voca decoder estimate requires %s dimensions %v, got %v", name, want, got)
		}
		for i, dim := range want {
			if got[i] != dim {
				return Result{}, fmt.Errorf("tiny-voca decoder estimate requires %s dimensions %v, got %v", name, want, got)
			}
		}
	}
	// Two prosody and five acoustic layers run in fresh, serialized contexts
	// with a whole 3000-row ubatch. Their arenas need not coexist. Reserve the
	// larger stack, plus both scheduler heads even though those are serialized.
	graph := tinyVocaStackGraph(tinyVocaMaxFrames, width, ffn, heads, 5, 1)
	graph += 4*tinyVocaSegmentTokens*(3*width+3*128+8*128+2) + 4*tinyVocaMaxFrames*(2*width+2*134)
	// mtmd.cpp retains x, h_pro, h_ac, voc, F0, owner and hidden through
	// source-filter synthesis. models/tiny-voca.cpp has pulses/noise and two
	// filtered outputs, three padded filter slabs, then the output waveform.
	// Charge eight waveform slabs and FFT scratch at the largest FFT (512).
	segmentSamples := tinyVocaMaxFrames * 256
	host := hostTables + 4*tinyVocaSegmentTokens*width + tinyVocaMaxFrames*(12*width+4*134+12)
	host += 32*segmentSamples + 4*(3*768+40*512)
	// The helper copies token and stress tables; voice rows and segment
	// metadata capacities are charged per admitted padded token.
	tables := 4*(76+3)*width + tinyVocaRequestTokens*(16*width+128)
	return Calculate(Input{
		Architecture: "clip", ModelSizeBytes: size, ComputeBackend: cfg.ComputeBackend,
		ComputeKind: modelprofile.ComputeTinyVocaDecoder, NoPersistentCache: true,
		Slots: max(cfg.Slots, 1), GPULayers: cfg.GPULayers, Speech: &SpeechConfig{MaxFrames: 512},
		tinyVocaGraphBytes: graph, tinyVocaHostBytes: host, tinyVocaTableBytes: tables,
	}), nil
}

func tinyVocaDecoderMemory(input Input) ComputeMemory {
	// Helper PCM vector capacity (2x), native gen_state output, WAV16 and
	// caller-owned WAV copies coexist. Round to six F32 waveform equivalents
	// per slot, bounding the sum across sentences, not just one utterance.
	host := input.tinyVocaHostBytes + max(input.Slots, 1)*(24*tinyVocaOutputSamples+input.tinyVocaTableBytes)
	return tinyVocaMemory(input, input.tinyVocaGraphBytes, host)
}

func tinyVocaMemory(input Input, graph, host int64) ComputeMemory {
	// Unmeasured backend/bookkeeping allowance, separate from source-shaped
	// tensor storage. Partial offload conservatively reserves both arenas.
	runtime := int64(32 * 1024 * 1024)
	switch {
	case input.GPULayers == -1:
		host += graph + runtime
		graph, runtime = 0, 0
	case input.GPULayers > 0:
		host += graph + runtime
	}
	return ComputeMemory{
		WorkspaceGPU: graph, RuntimeAllowanceGPU: runtime, WorkspaceCPU: host,
		MarginGPU: (graph + runtime) / 10, MarginCPU: host / 10,
	}
}

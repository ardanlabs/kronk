package vram

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

func TestFromBytesSpeechMemory(t *testing.T) {
	metadata := map[string]string{
		"general.architecture": "gpt2", "general.tags": "[text-to-speech]",
		"gpt2.block_count": "24", "gpt2.embedding_length": "1024",
		"gpt2.attention.head_count": "16", "gpt2.attention.head_count_kv": "16",
		"gpt2.vocab_size": "56839", "gpt2.feed_forward_length": "4096",
		"tokenizer.ggml.bos_token_id": "56837", "tokenizer.ggml.eos_token_id": "56838",
	}
	tensors := []gguf.TensorInfo{
		{Name: "token_embd.weight", Dims: []int64{1024, 56839}},
		{Name: "position_embd.weight", Dims: []int64{1024, 8196}},
	}
	header := deepseek41Header(t, metadata, tensors)
	var single Result
	for _, slots := range []int64{1, 3} {
		result, err := FromBytes(header, 481791712, Config{ComputeBackend: "metal", Slots: slots})
		if err != nil {
			t.Fatal(err)
		}
		if result.Input.Speech == nil || result.Input.Speech.MaxFrames != 512 || result.Input.NUBatch != 512 || result.Input.ContextWindow != 2048 {
			t.Fatalf("speech limits not detected: %+v", result.Input)
		}
		if result.Input.ComputeKind != modelprofile.ComputeChatterboxTurboBackbone {
			t.Fatal("backbone allocation family not selected")
		}
		if result.SlotMemory != slots*192<<20 {
			t.Errorf("KV: got %d, want %d", result.SlotMemory, slots*192<<20)
		}
		// The native helper owns a complete F32 copy of the embedding table
		// per slot, independent of quantized model-weight residency.
		if result.ComputeBufferCPU < slots*232812544 {
			t.Error("host budget omitted per-helper embedding copies")
		}
		if slots == 1 {
			single = result
			continue
		}
		if result.ComputeBufferCPU-single.ComputeBufferCPU < 2*232812544 {
			t.Error("additional helpers did not receive independent host tables")
		}
	}
	short, err := FromBytes(header, 481791712, Config{Speech: &SpeechConfig{MaxFrames: 137}})
	if err != nil {
		t.Fatal(err)
	}
	if short.Input.Speech.MaxFrames != 137 || short.ComputeBufferCPU >= single.ComputeBufferCPU {
		t.Error("resolved frame limit did not reduce bounded host outputs")
	}
	for _, frames := range []int64{-1, 513} {
		if _, err := FromBytes(header, 481791712, Config{Speech: &SpeechConfig{MaxFrames: frames}}); err == nil {
			t.Errorf("accepted invalid frame limit %d", frames)
		}
	}
	delete(metadata, "general.tags")
	untagged, err := FromBytes(deepseek41Header(t, metadata, tensors), 481791712, Config{})
	if err != nil || untagged.Input.ComputeKind != modelprofile.ComputeChatterboxTurboBackbone {
		t.Fatalf("untagged Turbo graph not identified: %+v, %v", untagged.Input, err)
	}
	chat, err := FromBytes(deepseek41Header(t, metadata, nil), 481791712, Config{})
	if err != nil || chat.Input.Speech != nil {
		t.Fatalf("ordinary GPT2 classified as speech: %+v, %v", chat.Input, err)
	}
	configuredChat, err := FromBytes(deepseek41Header(t, metadata, nil), 481791712, Config{Speech: &SpeechConfig{MaxFrames: 137}})
	if err != nil || configuredChat.ComputeBufferCPU != chat.ComputeBufferCPU || configuredChat.ComputeBufferEst != chat.ComputeBufferEst || configuredChat.Input.Speech != nil {
		t.Fatalf("SpeechConfig selected a policy for ordinary GPT2: %+v, %v", configuredChat, err)
	}
	input := single.Input
	input.Speech = nil
	if withoutConfig := Calculate(input); withoutConfig.ComputeBufferCPU != single.ComputeBufferCPU || withoutConfig.ComputeBufferEst != single.ComputeBufferEst {
		t.Error("allocation family must work without an optional SpeechConfig")
	}
	metadata["general.tags"] = "[text-to-speech]"
	if _, err := FromBytes(deepseek41Header(t, metadata, nil), 481791712, Config{}); err == nil {
		t.Error("speech tags alone selected a Turbo memory policy")
	}
}

func TestFromBytesSpeechDecoderMemory(t *testing.T) {
	metadata := map[string]string{
		"general.architecture": "clip", "clip.has_gen_audio_encoder": "true",
		"clip.gen.audio.projector_type": "chatterbox", "clip.gen.audio.embedding_length": "512",
		"clip.gen.audio.feed_forward_length": "2048", "clip.gen.audio.block_count": "6",
		"clip.gen.audio.attention.head_count": "8", "clip.gen.audio.projection_dim": "80",
	}
	tensors := []gguf.TensorInfo{
		{Name: "a.gen.est.time_embed_mixer.weight", Dims: []int64{2048, 1024}},
		{Name: "voice.default.ref_tokens", Dims: []int64{250}},
		{Name: "voice.default.ref_feat", Dims: []int64{80, 500}},
		{Name: "voice.default.ref_spk", Dims: []int64{192}},
		{Name: "voice.default", Dims: []int64{1024, 376}},
	}
	header := deepseek41Header(t, metadata, tensors)
	cfg := Config{ComputeBackend: "metal", Slots: 1}
	single, err := FromBytes(header, 555371392, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if single.Input.ComputeKind != modelprofile.ComputeChatterboxTurboDecoder {
		t.Fatal("decoder allocation family not selected")
	}
	if single.SlotMemory != 0 || single.ModelWeightsGPU != 555371392 || single.ComputeBufferEst < 659<<20 {
		t.Fatalf("decoder omitted shared retained memory or allocated KV: %+v", single)
	}
	cfg.Slots = 4
	multiple, err := FromBytes(header, 555371392, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if multiple.ComputeBufferEst != single.ComputeBufferEst || multiple.ModelWeightsGPU != single.ModelWeightsGPU || multiple.ComputeBufferCPU <= single.ComputeBufferCPU {
		t.Error("decoder weights/workspace must be shared, while helper reference copies scale with slots")
	}
	cfg.Speech = &SpeechConfig{MaxFrames: 1}
	short, err := FromBytes(header, 555371392, cfg)
	if err != nil || short.ComputeBufferEst != multiple.ComputeBufferEst || short.ComputeBufferCPU >= multiple.ComputeBufferCPU {
		t.Fatalf("short output must retain calibrated GPU envelope but reduce host outputs: %+v, %v", short, err)
	}
	for _, backend := range []string{"vulkan", "cuda", "rocm", "", "opencl"} {
		cfg.ComputeBackend, cfg.Speech, cfg.Slots = backend, nil, 4
		provisional, err := FromBytes(header, 555371392, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if provisional.ComputeBufferEst <= multiple.ComputeBufferEst || provisional.ComputeBufferCPU <= multiple.ComputeBufferCPU || provisional.ModelWeightsGPU != single.ModelWeightsGPU || provisional.SlotMemory != 0 {
			t.Fatalf("%s must retain shared weights and add provisional workspace allowance without KV: %+v", backend, provisional)
		}
		cfg.Slots = 1
		provisionalSingle, err := FromBytes(header, 555371392, cfg)
		if err != nil || provisionalSingle.ComputeBufferEst != provisional.ComputeBufferEst || provisionalSingle.ComputeBufferCPU >= provisional.ComputeBufferCPU {
			t.Fatalf("%s decoder arenas must be shared while host references scale: %+v, %v", backend, provisionalSingle, err)
		}
	}
	for _, backend := range []string{"metal", "cpu"} {
		cfg.ComputeBackend, cfg.GPULayers = backend, 0
		gpu, err := FromBytes(header, 555371392, cfg)
		if err != nil {
			t.Fatal(err)
		}
		cfg.GPULayers = -1
		cpu, err := FromBytes(header, 555371392, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if cpu.TotalVRAM != 0 || cpu.ModelWeightsCPU != 555371392 || cpu.SlotMemory != 0 {
			t.Fatalf("%s CPU placement must reserve decoder weights and workspace in RAM only: %+v", backend, cpu)
		}
		// Moving the same estimate to RAM preserves the total, apart from
		// one byte from rounding the combined safety margin.
		difference := cpu.ComputeBufferCPU - gpu.ComputeBufferCPU - gpu.ComputeBufferEst
		if difference < 0 || difference > 1 {
			t.Fatalf("%s CPU placement lost decoder workspace: difference %d", backend, difference)
		}
	}
	cfg.ComputeBackend = "metal"
	cfg.GPULayers = 0
	tensors[1].Dims[0] = 251
	if _, err := FromBytes(deepseek41Header(t, metadata, tensors), 555371392, cfg); err == nil {
		t.Error("accepted reference beyond calibrated length")
	}
	tensors[1].Dims[0] = 250
	tensors = append(tensors, gguf.TensorInfo{Name: "a.gen.t3.text_pos_emb", Dims: []int64{1024, 2048}})
	if _, err := FromBytes(deepseek41Header(t, metadata, tensors), 555371392, cfg); err == nil {
		t.Error("accepted multilingual decoder as Turbo")
	}
}

func TestFromBytesQwen3TTSMemory(t *testing.T) {
	metadata := map[string]string{
		"general.architecture": "qwen3tts", "qwen3tts.block_count": "28",
		"qwen3tts.embedding_length": "2048", "qwen3tts.feed_forward_length": "6144",
		"qwen3tts.attention.head_count": "16", "qwen3tts.attention.head_count_kv": "8",
		"qwen3tts.attention.key_length": "128", "qwen3tts.attention.value_length": "128",
		"qwen3tts.vocab_size": "3072",
	}
	tensors := []gguf.TensorInfo{
		{Name: "token_embd.weight", Dims: []int64{2048, 155008}, GGMLType: 8},
		{Name: "output.weight", Dims: []int64{2048, 3072}, GGMLType: 8},
	}
	header := deepseek41Header(t, metadata, tensors)
	single, err := FromBytes(header, 1847874400, Config{ComputeBackend: "metal"})
	if err != nil {
		t.Fatal(err)
	}
	if single.Input.Speech == nil || single.Input.ContextWindow != 2048 || single.Input.NUBatch != 512 || single.SlotMemory != 224<<20 {
		t.Fatalf("Qwen speech limits/KV: %+v", single)
	}
	// The helper copies the combined 155008-row embedding table, not just the
	// 3072-row output vocabulary. Initialization also reads the encoded table.
	if single.ComputeBufferCPU < 1269825536+337297408 {
		t.Fatalf("Qwen helper table/readback omitted: got %d", single.ComputeBufferCPU)
	}
	multiple, err := FromBytes(header, 1847874400, Config{ComputeBackend: "metal", Slots: 4})
	if err != nil {
		t.Fatal(err)
	}
	if multiple.SlotMemory != 896<<20 || multiple.ComputeBufferCPU-single.ComputeBufferCPU < 3*1269825536 {
		t.Fatal("Qwen per-helper tables or backbone KV did not scale with slots")
	}
	short, err := FromBytes(header, 1847874400, Config{ComputeBackend: "metal", Speech: &SpeechConfig{MaxFrames: 71}})
	if err != nil || short.ComputeBufferCPU >= single.ComputeBufferCPU || short.SlotMemory != single.SlotMemory {
		t.Fatalf("frame cap must reduce output allowance, not backbone KV: %+v, %v", short, err)
	}
	if _, err := FromBytes(deepseek41Header(t, metadata, tensors[1:]), 1847874400, Config{}); err == nil {
		t.Fatal("accepted missing combined embedding table")
	}
	for _, frames := range []int64{-1, 513} {
		if _, err := FromBytes(header, 1847874400, Config{Speech: &SpeechConfig{MaxFrames: frames}}); err == nil {
			t.Errorf("accepted invalid Qwen frame limit %d", frames)
		}
	}
}

func TestFromBytesQwen3TTSDecoderMemory(t *testing.T) {
	metadata := map[string]string{
		"general.architecture": "clip", "clip.has_gen_audio_encoder": "true",
		"clip.gen.audio.projector_type": "qwen3tts_gen",
		"clip.has_audio_encoder":        "true", "clip.audio.projector_type": "qwen3tts_spkenc",
		"clip.audio.embedding_length": "1536", "clip.audio.block_count": "3",
		"clip.audio.attention.head_count": "1", "clip.audio.projection_dim": "2048",
		"clip.gen.audio.embedding_length": "1024", "clip.gen.audio.block_count": "5",
		"clip.gen.audio.feed_forward_length": "3072", "clip.gen.audio.projection_dim": "2048",
		"clip.gen.audio.attention.head_count": "16", "clip.gen.audio.attention.head_count_kv": "8",
	}
	tensors := []gguf.TensorInfo{
		{Name: "a.gen.code.head.weight", Dims: []int64{1024, 2048, 15}},
		{Name: "a.gen.code.out_embd.weight", Dims: []int64{2048, 3072}},
		{Name: "a.gen.wav.tfm.blk.0.attn_q.weight", Dims: []int64{512, 1024}},
		{Name: "a.gen.wav.tfm.blk.7.attn_q.weight", Dims: []int64{512, 1024}},
		{Name: "a.gen.wav.pre_conv.weight", Dims: []int64{3, 512, 1024}},
		{Name: "a.gen.wav.up.blk.0.dwconv.weight", Dims: []int64{7, 1, 1024}},
		{Name: "a.gen.wav.up.blk.1.dwconv.weight", Dims: []int64{7, 1, 1024}},
		{Name: "a.gen.wav.dac.entry.weight", Dims: []int64{7, 1024, 1536}},
		{Name: "a.gen.wav.dac.post_conv.weight", Dims: []int64{7, 96, 1}},
	}
	for block, shape := range [][]int64{{16, 768, 1536}, {10, 384, 768}, {8, 192, 384}, {6, 96, 192}} {
		tensors = append(tensors, gguf.TensorInfo{Name: fmt.Sprintf("a.gen.wav.dac.blk.%d.conv.weight", block), Dims: shape})
		for residual := range 3 {
			tensors = append(tensors, gguf.TensorInfo{Name: fmt.Sprintf("a.gen.wav.dac.blk.%d.res.%d.conv1.weight", block, residual), Dims: []int64{7, shape[1], shape[1]}})
		}
	}
	header := deepseek41Header(t, metadata, tensors)
	cfg := Config{ComputeBackend: "metal"}
	single, err := FromBytes(header, 669081472, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if single.SlotMemory != 0 || single.ModelWeightsGPU != 669081472 || single.ComputeBufferEst < 1536<<20 {
		t.Fatalf("Qwen decoder shared weights/workspace or graph-local KV: %+v", single)
	}
	cfg.Slots = 4
	multiple, err := FromBytes(header, 669081472, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Native's 38 F32 state slots hold 5,218,948 payload bytes per helper.
	// Only this state scales on GPU: shared arenas must not be multiplied.
	delta := multiple.ComputeBufferEst - single.ComputeBufferEst
	if delta < 3*5218948 || delta > 20<<20 || multiple.ModelWeightsGPU != single.ModelWeightsGPU {
		t.Fatalf("Qwen decoder slot scaling: got GPU delta %d", delta)
	}
	cfg.Speech = &SpeechConfig{MaxFrames: 71}
	short, err := FromBytes(header, 669081472, cfg)
	if err != nil || short.ComputeBufferEst != multiple.ComputeBufferEst {
		t.Fatalf("frame cap changed the fixed 72-frame decoder graph: %+v, %v", short, err)
	}
	for _, backend := range []string{"vulkan", "cuda", "rocm", "", "opencl"} {
		cfg.ComputeBackend, cfg.Speech, cfg.Slots = backend, nil, 4
		provisional, err := FromBytes(header, 669081472, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if provisional.ComputeBufferEst <= multiple.ComputeBufferEst || provisional.ComputeBufferCPU <= multiple.ComputeBufferCPU || provisional.ModelWeightsGPU != single.ModelWeightsGPU || provisional.SlotMemory != 0 {
			t.Fatalf("%s must add provisional shared workspace without duplicating weights or KV: %+v", backend, provisional)
		}
		cfg.Slots = 1
		provisionalSingle, err := FromBytes(header, 669081472, cfg)
		if err != nil {
			t.Fatal(err)
		}
		delta := provisional.ComputeBufferEst - provisionalSingle.ComputeBufferEst
		if delta < 3*5218948 || delta > 20<<20 {
			t.Fatalf("%s must scale codec state, not multiply shared arenas: got GPU delta %d", backend, delta)
		}
	}
	for _, backend := range []string{"metal", "cpu"} {
		cfg.ComputeBackend, cfg.GPULayers = backend, 0
		gpu, err := FromBytes(header, 669081472, cfg)
		if err != nil {
			t.Fatal(err)
		}
		cfg.GPULayers = -1
		cpu, err := FromBytes(header, 669081472, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if cpu.TotalVRAM != 0 || cpu.ModelWeightsCPU != 669081472 || cpu.SlotMemory != 0 {
			t.Fatalf("%s CPU placement must reserve Qwen weights and codec state in RAM only: %+v", backend, cpu)
		}
		difference := cpu.ComputeBufferCPU - gpu.ComputeBufferCPU - gpu.ComputeBufferEst
		if difference < 0 || difference > 1 {
			t.Fatalf("%s CPU placement lost Qwen decoder workspace/state: difference %d", backend, difference)
		}
	}
	cfg.ComputeBackend = "metal"
	cfg.GPULayers = 0
	tensors[0].Dims[2] = 14
	if _, err := FromBytes(deepseek41Header(t, metadata, tensors), 669081472, cfg); err == nil {
		t.Fatal("accepted uncalibrated Qwen codebook layout")
	}
}

func TestCalculateUnifiedLFM2Reservation(t *testing.T) {
	input := Input{
		Architecture: "lfm2", ComputeKind: modelprofile.ComputeLFM2, ComputeBackend: "metal",
		ContextWindow: 4096, Slots: 3, NUBatch: 256, OutputRows: 3,
		BlockCount: 16, EmbeddingLength: 1536, VocabularySize: 65536,
		HeadCount: 24, HeadCountKV: 8, KeyLength: 64, ValueLength: 64,
		BytesPerElement: 2, KVUnified: true,
	}
	result := Calculate(input)
	// b12125 reserves 129.3531 MiB for this public configuration. The
	// previous 123.2 MiB reservation missed Metal's per-slice row gathers.
	const nativeArena int64 = 135636557
	if result.ComputeBufferEst < nativeArena {
		t.Fatalf("GPU compute reservation: got %d, must cover native arena %d", result.ComputeBufferEst, nativeArena)
	}
}

func TestCalculateUnifiedAttentionWorkspace(t *testing.T) {
	// With three slices, 95 queries reserve 96 rows. K and V have
	// different widths, so confusing their extents changes the result.
	// Full extent = 1536; original K/V scratch = 1179648 bytes;
	// gathered K/V = 3538944; extra sliced reduction = 8519680;
	// row lists + rounded mask query = 21504, on host and device.
	for _, tt := range []struct {
		name, backend                string
		unified, nonFlash, recurrent bool
		layers, contexts, gpu, cpu   int64
	}{
		{"shared Metal", "metal", true, false, false, 0, 1, 13843456, 410624},
		{"independent contexts", "metal", true, false, false, 0, 2, 27686912, 821248},
		{"partitioned control", "metal", false, false, false, 0, 1, 1568768, 194560},
		{"dense control", "metal", true, true, false, 0, 1, 10214400, 680960},
		{"CUDA has no Metal gathers", "cuda", true, false, false, 0, 1, 605184, 410624},
		{"host layers", "metal", true, false, false, -1, 1, 583680, 410624},
		{"output layer only", "metal", true, false, false, 1, 1, 583680, 410624},
		{"recurrent layers", "metal", true, false, true, 0, 1, 583680, 389120},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Calculate(Input{
				ComputeKind: modelprofile.ComputeLFM2, ComputeBackend: tt.backend,
				ContextWindow: 512, Slots: 3, NUBatch: 95, OutputRows: 1,
				BlockCount: 3, EmbeddingLength: 64, VocabularySize: 16,
				HeadCount: 8, HeadCountKV: 2, KeyLength: 64, ValueLength: 128,
				RecurrentPattern: []bool{tt.recurrent, tt.recurrent, tt.recurrent},
				GPULayers:        tt.layers, ComputeContexts: tt.contexts,
				KVUnified: tt.unified, VTransposed: tt.nonFlash, BytesPerElement: 2,
			})
			if result.ComputeMemory.WorkspaceGPU != tt.gpu || result.ComputeMemory.WorkspaceCPU != tt.cpu {
				t.Fatalf("GPU/CPU workspace: got %d/%d, want %d/%d", result.ComputeMemory.WorkspaceGPU, result.ComputeMemory.WorkspaceCPU, tt.gpu, tt.cpu)
			}
		})
	}
}

func TestCalculateGemmaWorkspaceScaling(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name                         string
		context, batch, vocab, slots int64
		compact, nonFlash            bool
		wantGPU, wantCPU             int64
	}{
		// Small vocabulary makes attention-mask storage the GPU peak.
		{"base", 4096, 128, 1024, 1, false, false, 259 * mib, 17 * mib / 8},
		{"double batch", 4096, 256, 1024, 1, false, false, 262 * mib, 17 * mib / 4},
		{"double context", 8192, 128, 1024, 1, false, false, 261 * mib, 33 * mib / 8},
		{"compact SWA", 4096, 128, 1024, 1, true, false, 258*mib + 5*mib/16, 23 * mib / 16},
		{"two streams, same batch", 4096, 128, 1024, 2, false, false, 259*mib + mib/4, 17 * mib / 8},
		{"without flash attention", 4096, 128, 1024, 1, false, true, 325 * mib, 33 * mib / 8},
		// A larger vocabulary makes logits, not masks, the GPU peak.
		{"logits peak", 4096, 128, 8192, 1, false, false, 261 * mib, 17 * mib / 8},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := Calculate(Input{ComputeKind: modelprofile.ComputeGemma4, ContextWindow: tt.context, Slots: tt.slots, NUBatch: tt.batch, VocabularySize: tt.vocab, EmbeddingLength: 256, HeadCount: 16, SlidingWindow: 1024, SWAFull: !tt.compact, VTransposed: tt.nonFlash})
			wantGPU, wantCPU := tt.wantGPU+tt.wantGPU/10, tt.wantCPU+tt.wantCPU/10
			if r.ComputeBufferEst != wantGPU || r.ComputeBufferCPU != wantCPU {
				t.Fatalf("GPU/CPU compute: got %d/%d, want %d/%d", r.ComputeBufferEst, r.ComputeBufferCPU, wantGPU, wantCPU)
			}
			wantMemory := ComputeMemory{
				WorkspaceGPU:        tt.wantGPU - 256*mib,
				WorkspaceCPU:        tt.wantCPU,
				RuntimeAllowanceGPU: 256 * mib,
				MarginGPU:           tt.wantGPU / 10,
				MarginCPU:           tt.wantCPU / 10,
			}
			if r.ComputeMemory != wantMemory {
				t.Errorf("compute breakdown: got %+v, want %+v", r.ComputeMemory, wantMemory)
			}
		})
	}
}

func TestCalculateGemmaMetalWorkspace(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name, backend                 string
		slots, batch, swaValue        int64
		phaseMiB                      float64
		unified, cpuWeights, cpuCache bool
	}{
		{"one slot logits peak", "metal", 1, 2052, 256, 2052, false, false, false},
		{"two slots attention peak", "metal", 2, 2056, 256, 3076, false, false, false},
		{"three slots attention peak", "metal", 3, 2060, 256, 4102, false, false, false},
		{"unequal K and V", "metal", 2, 2056, 128, 2564, false, false, false},
		// Aggregate cache: 2056 MiB masks + 2048 MiB original scratch.
		// Rows add 4096 MiB gathers, 16.125 MiB reduction, 4 MiB lists.
		{"unified KV pool", "metal", 2, 2056, 256, 8220.125, true, false, false},
		{"CUDA control", "cuda", 2, 2056, 256, 2056, false, false, false},
		{"CPU control", "metal", 2, 2056, 256, 2056, false, true, false},
		{"CPU KV control", "metal", 2, 2056, 256, 2056, false, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := Input{
				ComputeBackend: tt.backend, ComputeKind: modelprofile.ComputeGemma4,
				ContextWindow: 131072, Slots: tt.slots, NUBatch: tt.batch,
				VocabularySize: 262144, EmbeddingLength: 2816,
				BlockCount: 3, HeadCount: 16, HeadCountKV: 2,
				KeyLength: 512, ValueLength: 512,
				SWAHeadCountKV: 8, SWAKeyLength: 256, SWAValueLength: tt.swaValue,
				HeadCountKVByLayer: []int64{8, 8, 2},
				SWAPattern:         []bool{true, true, false}, SlidingWindowLayers: 2,
				SlidingWindow: 1024, SWAFull: true,
				KVUnified: tt.unified, KVCacheOnCPU: tt.cpuCache,
				BytesPerElement: 2,
			}
			if tt.cpuWeights {
				input.GPULayers = -1
			}
			result := Calculate(input)
			// Each 262144-row F32 logits token is 1 MiB. Attention keeps
			// two F16 masks plus scratch for the largest layer, not all
			// layers. SWA has 8 heads; full attention has only 2.
			activation := tt.batch * 2816 * (32 + 8*(tt.slots-1))
			wantWorkspace := activation + int64(tt.phaseMiB*float64(mib))
			wantGPU := wantWorkspace + 256*mib
			wantGPU += wantGPU / 10
			if result.ComputeMemory.WorkspaceGPU != wantWorkspace || result.ComputeBufferEst != wantGPU {
				t.Fatalf("workspace/reservation: got %d/%d, want %d/%d", result.ComputeMemory.WorkspaceGPU, result.ComputeBufferEst, wantWorkspace, wantGPU)
			}
			// The assistant borrows KV but still reserves the same scratch.
			input.SharedKVLayers = input.BlockCount
			assistant := Calculate(input)
			if assistant.SlotMemory != 0 || assistant.ComputeBufferEst != wantGPU {
				t.Fatalf("shared KV cache/compute: got %d/%d, want 0/%d", assistant.SlotMemory, assistant.ComputeBufferEst, wantGPU)
			}
			if tt.name == "two slots attention peak" {
				for _, control := range []string{"compact SWA", "partial offload", "without flash attention"} {
					t.Run(control, func(t *testing.T) {
						controlInput := input
						controlInput.SharedKVLayers = 0
						switch control {
						case "compact SWA":
							controlInput.SWAFull = false
						case "partial offload":
							controlInput.GPULayers = 2 // Output and the last full-attention layer.
						case "without flash attention":
							controlInput.VTransposed = true
						}
						metal := Calculate(controlInput)
						controlInput.ComputeBackend = "cuda"
						cuda := Calculate(controlInput)
						if metal.ComputeBufferEst != cuda.ComputeBufferEst {
							t.Errorf("Metal/CUDA control: got %d/%d, want equal", metal.ComputeBufferEst, cuda.ComputeBufferEst)
						}
					})
				}
			}
		})
	}
}

func TestCalculateMistral3Workspace(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name, backend        string
		slots, value, gpuMiB int64
	}{
		{"one stream", "metal", 1, 128, 1600},
		{"two streams", "metal", 2, 128, 2192},
		{"unequal K/V widths", "metal", 1, 64, 1472},
		{"CUDA control", "cuda", 1, 128, 1088},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Calculate(Input{
				ComputeKind: modelprofile.ComputeMistral3, ComputeBackend: tt.backend,
				ContextWindow: 131072, Slots: tt.slots, NUBatch: 2048, OutputRows: tt.slots,
				EmbeddingLength: 5120, VocabularySize: 196608,
				BlockCount: 40, HeadCount: 32, HeadCountKV: 8, KeyLength: 128, ValueLength: tt.value,
			})
			// One 512 MiB mask, one layer's K/V scratch, and activation
			// storage coexist. Host allowance is the mask + 160 MiB.
			gpu, cpu := tt.gpuMiB*mib, 672*mib
			gpu, cpu = gpu+gpu/10, cpu+cpu/10
			if result.ComputeBufferEst != gpu || result.ComputeBufferCPU != cpu {
				t.Fatalf("GPU/CPU compute: got %d/%d, want %d/%d", result.ComputeBufferEst, result.ComputeBufferCPU, gpu, cpu)
			}
		})
	}
}

func TestCalculateGPTOSSWorkspace(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name, backend          string
		slots, layers, context int64
		compact, unified       bool
		wantGPU, wantCPU       int64
	}{
		{"full SWA", "metal", 1, 0, 131072, false, false, 1716 * mib, 1114 * mib},
		{"compact SWA", "metal", 1, 0, 131072, true, false, 1213 * mib, 611 * mib},
		{"two streams", "metal", 2, 0, 131072, false, false, 2017 * mib, 1114 * mib},
		{"padded per-stream context", "metal", 2, 0, 131073, false, false, 2020 * mib, 1116 * mib},
		// Rows add 1024 MiB gathers, 16.5 MiB sliced reduction and
		// 4 MiB host/device row lists to the dense-fallback reserve.
		{"unified pool", "metal", 2, 0, 131072, false, true, 4085*mib + mib/2, 2142 * mib},
		{"CUDA control", "cuda", 1, 0, 131072, false, false, 1460 * mib, 1114 * mib},
		// Only the trailing SWA layer is offloaded: scratch uses its
		// compact extent, not the host full-attention layer's extent.
		{"only SWA offloaded", "metal", 1, 2, 131072, true, false, 961*mib + mib/2, 611 * mib},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := Calculate(Input{
				ComputeKind: modelprofile.ComputeGPTOSS, ComputeBackend: tt.backend,
				ContextWindow: tt.context, Slots: tt.slots, NUBatch: 2048, OutputRows: tt.slots,
				EmbeddingLength: 2880, VocabularySize: 201088,
				BlockCount: 3, HeadCount: 64, HeadCountKV: 8, KeyLength: 64, ValueLength: 64,
				SlidingWindow: 128, SlidingWindowLayers: 2, SWAPattern: []bool{true, false, true},
				SWAFull: !tt.compact, GPULayers: tt.layers, KVUnified: tt.unified,
			})
			wantGPU, wantCPU := tt.wantGPU+tt.wantGPU/10, tt.wantCPU+tt.wantCPU/10
			if result.ComputeBufferEst != wantGPU || result.ComputeBufferCPU != wantCPU {
				t.Fatalf("GPU/CPU compute: got %d/%d, want %d/%d", result.ComputeBufferEst, result.ComputeBufferCPU, wantGPU, wantCPU)
			}
		})
	}
}

func TestCalculateQwen35Workspace(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name                  string
		contexts, rows, slots int64
		backend               string
		wantGPU, wantCPU      int64
	}{
		// Ornith's 256K attention mask is 1026 MiB; Metal K/V scratch
		// adds 512 MiB once per arena, not once per attention layer.
		{"target", 1, 4, 1, "metal", 1922*mib + mib/4, 1090*mib + mib/8},
		{"target and embedded MTP", 2, 4, 1, "metal", 4356*mib + mib/2, 2148*mib + 3*mib/16},
		{"uncapped logits", 1, 0, 1, "metal", 2328*mib + 40*mib/1024, 1090*mib + mib/8},
		{"two streams", 1, 8, 2, "metal", 2466*mib + 5*mib/16, 1090*mib + mib/8},
		{"CUDA control", 1, 4, 1, "cuda", 1410*mib + mib/4, 1090*mib + mib/8},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pattern := make([]bool, 41)
			heads := make([]int64, 41)
			for layer := range 40 {
				pattern[layer] = (layer+1)%4 != 0
				heads[layer] = 2
			}
			// A wider appended layer must affect only the MTP arena,
			// not the target's peak or the other attention layers.
			heads[40] = 4
			result := Calculate(Input{
				ComputeKind: modelprofile.ComputeQwen35, ComputeBackend: tt.backend,
				ContextWindow: 262144, Slots: tt.slots, NUBatch: 2052,
				OutputRows: tt.rows, VocabularySize: 248320, EmbeddingLength: 2048,
				BlockCount: 41, HeadCount: 16, HeadCountKV: 2, KeyLength: 256, ValueLength: 256,
				HeadCountKVByLayer: heads,
				RecurrentPattern:   pattern, NextNPredictLayers: 1, ComputeContexts: tt.contexts,
			})
			wantGPU := tt.wantGPU + tt.wantGPU/10
			// Margins are rounded per context, not after combining contexts.
			wantCPU := tt.wantCPU + tt.wantCPU/10
			if tt.contexts == 2 {
				wantGPU--
			}
			if result.ComputeBufferEst != wantGPU || result.ComputeBufferCPU != wantCPU {
				t.Fatalf("GPU/CPU compute: got %d/%d, want %d/%d", result.ComputeBufferEst, result.ComputeBufferCPU, wantGPU, wantCPU)
			}
		})
	}
}

func TestEstimateComputeBufferUsesNUBatch(t *testing.T) {
	base := EstimateComputeBuffer(Input{
		NUBatch:         DefaultNUBatch,
		EmbeddingLength: 4096,
	})
	doubled := EstimateComputeBuffer(Input{
		NUBatch:         2 * DefaultNUBatch,
		EmbeddingLength: 4096,
	})
	defaulted := EstimateComputeBuffer(Input{
		EmbeddingLength: 4096,
	})

	if doubled <= base {
		t.Errorf("doubled NUBatch estimate: got %d, want greater than %d", doubled, base)
	}
	if defaulted != base {
		t.Errorf("default NUBatch estimate: got %d, want %d", defaulted, base)
	}
}

func TestCalculateChargesEveryComputeContext(t *testing.T) {
	input := Input{
		ModelSizeBytes:  100,
		NUBatch:         DefaultNUBatch,
		EmbeddingLength: 4096,
		ComputeContexts: 2,
	}

	result := Calculate(input)
	want := 2 * EstimateComputeBuffer(input)
	if result.ComputeBufferEst != want {
		t.Errorf("ComputeBufferEst: got %d, want %d", result.ComputeBufferEst, want)
	}
	wantMemory := ComputeMemory{
		WorkspaceGPU:        512 << 20,
		RuntimeAllowanceGPU: 512 << 20,
		MarginGPU:           2 * ((512 << 20) / 10),
	}
	if result.ComputeMemory != wantMemory {
		t.Errorf("combined compute breakdown: got %+v, want %+v", result.ComputeMemory, wantMemory)
	}
}

func TestAutoFitCPUOnly(t *testing.T) {
	const mib = 1024 * 1024

	tests := []struct {
		name  string
		input Input
	}{
		{
			name: "dense",
			input: Input{
				ModelSizeBytes: 100 * mib,
				BlockCount:     1,
			},
		},
		{
			name: "moe",
			input: Input{
				ModelSizeBytes: 200 * mib,
				BlockCount:     1,
				MoE:            &gguf.MoEInfo{IsMoE: true},
				Weights: &gguf.WeightBreakdown{
					AlwaysActiveBytes:  100 * mib,
					ExpertBytesTotal:   100 * mib,
					ExpertBytesByLayer: []int64{100 * mib},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpuLayers, expertLayers, result, fit := AutoFit(tt.input, FitConstraints{
				CombinedFreeBytes: 320 * mib,
				SystemRAMBytes:    300 * mib,
			})

			if !fit {
				t.Fatal("fit: got false, want true")
			}
			if gpuLayers != -1 {
				t.Errorf("gpuLayers: got %d, want -1", gpuLayers)
			}
			if expertLayers != 0 {
				t.Errorf("expertLayers: got %d, want 0", expertLayers)
			}
			if result.ModelWeightsGPU != 0 {
				t.Errorf("ModelWeightsGPU: got %d, want 0", result.ModelWeightsGPU)
			}
		})
	}
}

func TestAutoFitDenseNoFitPreservesCPUOnlySentinel(t *testing.T) {
	gpuLayers, _, result, fit := AutoFit(Input{
		ModelSizeBytes: 100 * 1024 * 1024,
		BlockCount:     1,
	}, FitConstraints{
		CombinedFreeBytes: 1,
		SystemRAMBytes:    1,
	})

	if fit {
		t.Fatal("fit: got true, want false")
	}
	if gpuLayers != -1 {
		t.Errorf("gpuLayers: got %d, want -1", gpuLayers)
	}
	if result.Input.GPULayers != -1 {
		t.Errorf("result.Input.GPULayers: got %d, want -1", result.Input.GPULayers)
	}
}

func TestAutoFitUnifiedMemory(t *testing.T) {
	input := Input{
		ModelSizeBytes: 1000,
		BlockCount:     2,
		MoE: &gguf.MoEInfo{
			IsMoE: true,
		},
		Weights: &gguf.WeightBreakdown{
			AlwaysActiveBytes:  100,
			ExpertBytesTotal:   900,
			ExpertBytesByLayer: []int64{450, 450},
		},
	}
	wantFootprint := Calculate(input).UnifiedFootprint()

	tests := []struct {
		name     string
		ramBytes int64
		wantFit  bool
	}{
		{name: "fits", ramBytes: wantFootprint * 2, wantFit: true},
		{name: "does not fit", ramBytes: 1, wantFit: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpuLayers, expertLayers, result, fit := AutoFit(input, FitConstraints{
				SystemRAMBytes: tt.ramBytes,
				UnifiedMemory:  true,
			})

			if gpuLayers != input.BlockCount+1 {
				t.Errorf("gpuLayers: got %d, want %d", gpuLayers, input.BlockCount+1)
			}
			if expertLayers != input.BlockCount {
				t.Errorf("expertLayers: got %d, want %d", expertLayers, input.BlockCount)
			}
			if fit != tt.wantFit {
				t.Errorf("fit: got %t, want %t", fit, tt.wantFit)
			}
			if got := result.UnifiedFootprint(); got != wantFootprint {
				t.Errorf("UnifiedFootprint: got %d, want %d", got, wantFootprint)
			}
			if result.ExpertGPUBytes != input.Weights.ExpertBytesTotal {
				t.Errorf("ExpertGPUBytes: got %d, want %d", result.ExpertGPUBytes, input.Weights.ExpertBytesTotal)
			}
		})
	}
}

func TestAssessFitUnifiedMemory(t *testing.T) {
	result := Result{Input: Input{ModelSizeBytes: 90}}

	tests := []struct {
		name       string
		capacity   int64
		wantStatus FitStatus
		wantFit    bool
	}{
		{name: "comfortable", capacity: 200, wantStatus: FitStatusFits, wantFit: true},
		{name: "tight", capacity: 100, wantStatus: FitStatusTight, wantFit: true},
		{name: "outside safe budget", capacity: 94, wantStatus: FitStatusDoesNotFit, wantFit: false},
		{name: "unknown capacity", wantStatus: FitStatusUnknown, wantFit: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment := AssessFit(result, FitConstraints{
				SystemRAMBytes: tt.capacity,
				UnifiedMemory:  true,
			})

			if assessment.Status != tt.wantStatus {
				t.Errorf("Status: got %q, want %q", assessment.Status, tt.wantStatus)
			}
			if assessment.Fits != tt.wantFit {
				t.Errorf("Fits: got %t, want %t", assessment.Fits, tt.wantFit)
			}
			if assessment.Unified.HeadroomBytes != tt.capacity-result.UnifiedFootprint() {
				t.Errorf("HeadroomBytes: got %d, want %d", assessment.Unified.HeadroomBytes, tt.capacity-result.UnifiedFootprint())
			}
		})
	}
}

func TestAssessFitRequiresCapacityForRequiredSystemRAM(t *testing.T) {
	assessment := AssessFit(Result{
		TotalVRAM:         50,
		TotalSystemRAMEst: 25,
	}, FitConstraints{CombinedFreeBytes: 100})

	if assessment.Status != FitStatusUnknown {
		t.Errorf("Status: got %q, want %q", assessment.Status, FitStatusUnknown)
	}
	if assessment.Fits {
		t.Error("Fits: got true, want false")
	}
}

func TestCalculateWeightPlacement(t *testing.T) {
	for _, tt := range []struct {
		name             string
		layers           int64
		tied             bool
		wantGPU, wantCPU int64
	}{
		{name: "untied CPU", layers: -1, wantCPU: 1000},
		{name: "untied output only", layers: 1, wantGPU: 300, wantCPU: 700},
		{name: "untied partial", layers: 3, wantGPU: 550, wantCPU: 450},
		{name: "untied all but first block", layers: 4, wantGPU: 675, wantCPU: 325},
		{name: "untied full", layers: 5, wantGPU: 800, wantCPU: 200},
		{name: "untied default", wantGPU: 800, wantCPU: 200},
		{name: "tied CPU aliases", layers: -1, tied: true, wantCPU: 700},
		{name: "tied output only duplicates", layers: 1, tied: true, wantGPU: 200, wantCPU: 700},
		{name: "tied partial duplicates", layers: 3, tied: true, wantGPU: 450, wantCPU: 450},
		{name: "tied full duplicates", layers: 5, tied: true, wantGPU: 700, wantCPU: 200},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tensors := []gguf.TensorInfo{{Name: "token_embd.weight", Bytes: 200}, {Name: "blk.0.attn_q.weight", Bytes: 500}}
			if !tt.tied {
				tensors = append(tensors, gguf.TensorInfo{Name: "output.weight", Bytes: 300})
			}
			weights := gguf.CategorizeWeights(tensors, 4)
			result := Calculate(Input{ModelSizeBytes: weights.TotalBytes, BlockCount: 4, GPULayers: tt.layers, Weights: &weights, TiedOutputFallback: true})
			if result.ModelWeightsGPU != tt.wantGPU || result.ModelWeightsCPU != tt.wantCPU {
				t.Fatalf("weights: got GPU/CPU %d/%d, want %d/%d", result.ModelWeightsGPU, result.ModelWeightsCPU, tt.wantGPU, tt.wantCPU)
			}
			if got := result.UnifiedFootprint() - result.ComputeBufferEst; got != weights.TotalBytes {
				t.Errorf("unified weights: got %d, want %d", got, weights.TotalBytes)
			}
		})
	}
}

func TestCalculateExpertsFollowOffloadedLayers(t *testing.T) {
	weights := gguf.WeightBreakdown{AlwaysActiveBytes: 400, ExpertBytesTotal: 1000, ExpertBytesByLayer: []int64{100, 200, 300, 400}}
	for _, tt := range []struct {
		layers, experts, wantGPU int64
	}{
		{-1, 4, 0}, {1, 4, 0}, {3, 4, 700}, {3, 1, 400}, {5, 0, 0}, {5, 4, 1000},
	} {
		result := Calculate(Input{ModelSizeBytes: 1400, BlockCount: 4, Weights: &weights, MoE: &gguf.MoEInfo{IsMoE: true}, GPULayers: tt.layers, ExpertLayersOnGPU: tt.experts})
		if result.ExpertGPUBytes != tt.wantGPU || result.ExpertCPUBytes != 1000-tt.wantGPU {
			t.Errorf("layers=%d experts=%d: got GPU/CPU %d/%d, want %d/%d", tt.layers, tt.experts, result.ExpertGPUBytes, result.ExpertCPUBytes, tt.wantGPU, 1000-tt.wantGPU)
		}
	}
}

func TestAutoFitIncludesOutputLayer(t *testing.T) {
	layers, _, result, fit := AutoFit(Input{ModelSizeBytes: 100, BlockCount: 4}, FitConstraints{CombinedFreeBytes: 1 << 30, SystemRAMBytes: 1 << 30})
	if !fit || layers != 5 || result.ModelWeightsCPU != 0 {
		t.Fatalf("full offload: got fit=%t layers=%d CPU=%d, want true/5/0", fit, layers, result.ModelWeightsCPU)
	}
}

func TestCalculateLazyWeightResidency(t *testing.T) {
	const gib int64 = 1 << 30
	for _, tt := range []struct {
		name, mode, load              string
		tableBytes                    int64
		platform, auto, isolated, cpu bool
		wantResident                  int64
	}{
		{"off", "off", "none", 5 * gib, true, true, false, false, 5 * gib},
		{"auto exactly threshold", "auto", "none", 4 * gib, true, true, false, false, 4 * gib},
		{"auto above threshold", "auto", "none", 4*gib + 1, true, true, false, false, gib / 4},
		{"auto unknown devices", "auto", "none", 5 * gib, true, false, false, false, 5 * gib},
		{"on bypasses device gate", "on", "none", gib, true, false, false, false, gib / 4},
		{"on unsupported platform", "on", "none", gib, false, true, false, false, gib},
		{"Metal overlapping mmap", "on", "mmap", gib, true, true, false, false, gib},
		{"Metal overlapping auto", "on", "auto", gib, true, true, false, false, gib},
		{"Metal isolated mmap", "on", "mmap", gib, true, true, true, false, gib / 4},
		{"CPU only mmap", "on", "mmap", gib, true, true, false, true, gib / 4},
		{"mlock", "on", "mlock", gib, true, true, true, false, gib},
		{"mmap mlock", "on", "mmap+mlock", gib, true, true, true, false, gib},
		{"small table capped", "on", "none", gib / 8, true, true, false, false, gib / 8},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := Input{
				ModelSizeBytes: tt.tableBytes + 2*gib, BlockCount: 2,
				ComputeBackend: "metal", Slots: 2, NUBatch: 8,
				Weights:       &gguf.WeightBreakdown{InputBytes: tt.tableBytes, OutputBytes: gib},
				LazyTable:     LazyTable{Bytes: tt.tableBytes, RowBytes: 1024, RowsPerToken: 1, Isolated: tt.isolated},
				WeightLoading: WeightLoading{LazyMode: tt.mode, LoadMode: tt.load, MMapSupported: tt.platform, AutoMMapSupported: tt.auto},
			}
			if tt.cpu {
				input.GPULayers = -1
			}
			baselineInput := input
			baselineInput.WeightLoading.LazyMode = "off"
			baseline := Calculate(baselineInput)
			result := Calculate(input)
			wantSaving := tt.tableBytes - tt.wantResident
			if result.LazyTableBytes != tt.tableBytes || result.LazyResidentBytes != tt.wantResident || baseline.UnifiedFootprint()-result.UnifiedFootprint() != wantSaving || baseline.TotalSystemRAMEst-result.TotalSystemRAMEst != wantSaving {
				t.Fatalf("lazy table/resident/unified saving/host saving: got %d/%d/%d/%d, want %d/%d/%d/%d", result.LazyTableBytes, result.LazyResidentBytes, baseline.UnifiedFootprint()-result.UnifiedFootprint(), baseline.TotalSystemRAMEst-result.TotalSystemRAMEst, tt.tableBytes, tt.wantResident, wantSaving, wantSaving)
			}
			if result.TotalVRAM != baseline.TotalVRAM || result.SlotMemory != baseline.SlotMemory || result.ComputeMemory != baseline.ComputeMemory {
				t.Fatal("lazy weight residency changed GPU, slot, or compute memory")
			}
			if wantSaving > 0 {
				constraints := FitConstraints{UnifiedMemory: true, SystemRAMBytes: result.UnifiedFootprint() + wantSaving/2, Threshold: 1}
				if !AssessFit(result, constraints).Fits || AssessFit(baseline, constraints).Fits {
					t.Fatal("unified fitting did not use the lazy working set")
				}
				_, _, fitted, fits := AutoFit(input, constraints)
				wantFit, wantFittedResident := true, tt.wantResident
				if tt.cpu {
					// Unified auto-fit chooses GPU offload, reintroducing the
					// overlapping Metal mapping. Re-evaluate, do not reuse savings.
					wantFit, wantFittedResident = false, tt.tableBytes
				}
				if fits != wantFit || fitted.LazyResidentBytes != wantFittedResident {
					t.Fatalf("auto-fit lazy resident: got fit=%t bytes=%d, want %t/%d", fits, fitted.LazyResidentBytes, wantFit, wantFittedResident)
				}
			}
		})
	}
}

func TestCalculateLazyRowWorkingSet(t *testing.T) {
	page := int64(os.Getpagesize())
	input := Input{
		ModelSizeBytes: 10 << 30, BlockCount: 2, NUBatch: 8193, Slots: 1,
		Weights:       &gguf.WeightBreakdown{InputBytes: 8 << 30},
		LazyTable:     LazyTable{Bytes: 8 << 30, RowBytes: 3*page + 1, RowsPerToken: 3},
		WeightLoading: WeightLoading{LazyMode: "on", LoadMode: "none", MMapSupported: true},
	}
	// A row spanning three pages plus one byte can touch five pages at
	// an arbitrary offset. Three gathers/token, two batches of 8193 tokens.
	want := int64(245790) * page
	result := Calculate(input)
	if result.LazyResidentBytes != want {
		t.Fatalf("page-rounded row working set: got %d, want %d", result.LazyResidentBytes, want)
	}
	input.Slots = 2
	if got := Calculate(input).LazyResidentBytes; got != want {
		t.Errorf("slots sharing one physical batch: got %d, want %d", got, want)
	}
	input.ComputeContexts = 2
	if got := Calculate(input).LazyResidentBytes; got != min(8<<30, 2*want) {
		t.Errorf("two native contexts: got %d, want %d", got, min(8<<30, 2*want))
	}
	input.NUBatch = 1 << 20
	if got := Calculate(input).LazyResidentBytes; got != 8<<30 {
		t.Errorf("table cap: got %d, want %d", got, int64(8<<30))
	}
}

// Metadata extracted from unsloth/DeepSeek-V4.1-Flash-1017-GGUF UD-IQ1_S,
// revision 60ad3651641b5c97679c807e064ab1d7a3ca1455. Tokenizer/hash constants
// and tensor payloads are unnecessary for sizing. The source has 40 target
// blocks, despite the trailing three zero ratios reserved for separate DSpark.
func deepseek41Metadata() map[string]string {
	return map[string]string{
		"general.architecture":                                "deepseek41",
		"deepseek41.block_count":                              "40",
		"deepseek41.embedding_length":                         "5120",
		"deepseek41.embedding_length_out":                     "20480",
		"deepseek41.attention.head_count":                     "64",
		"deepseek41.attention.head_count_kv":                  "1",
		"deepseek41.attention.key_length":                     "512",
		"deepseek41.attention.value_length":                   "512",
		"deepseek41.attention.sliding_window":                 "128",
		"deepseek41.attention.q_lora_rank":                    "1280",
		"deepseek41.attention.output_lora_rank":               "1024",
		"deepseek41.attention.output_group_count":             "8",
		"deepseek41.attention.indexer.head_count":             "32",
		"deepseek41.attention.indexer.key_length":             "128",
		"deepseek41.attention.indexer.top_k":                  "512",
		"deepseek41.attention.compress_ratios":                "[0 0 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 2 1 1 1 1 1 1 1 1 1 1 1 1 1 1 1 1 1 1 1 1 0 0 0]",
		"deepseek41.attention.kv_source_layers":               "[2 8 14 20]",
		"deepseek41.attention.indexer.source_layers":          "[2 8 14 20 24 28 32 36]",
		"deepseek41.attention.indexer.candidate_source_layer": "20",
		"deepseek41.attention.indexer.candidate_top_k_blocks": "2048",
		"deepseek41.attention.indexer.candidate_block_size":   "8",
		"deepseek41.hyper_connection.count":                   "4",
		"deepseek41.ple.layers":                               "[1 14]",
		"deepseek41.ple.ngram_size":                           "4",
		"deepseek41.ple.heads_per_ngram":                      "8",
		"deepseek41.embedding_length_per_layer_input":         "256",
		"deepseek41.expert_count":                             "384",
		"deepseek41.expert_used_count":                        "6",
		"deepseek41.expert_shared_count":                      "1",
	}
}

func deepseek41Header(t *testing.T, metadata map[string]string, tensors []gguf.TensorInfo) []byte {
	t.Helper()
	var data bytes.Buffer
	write := func(value any) {
		t.Helper()
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	writeString := func(value string) {
		write(uint64(len(value)))
		if _, err := data.WriteString(value); err != nil {
			t.Fatal(err)
		}
	}
	write(gguf.Magic)
	write(uint32(3))
	write(uint64(len(tensors)))
	write(uint64(len(metadata)))
	for key, value := range metadata {
		writeString(key)
		write(gguf.MetadataValueTypeString)
		writeString(value)
	}
	for _, tensor := range tensors {
		writeString(tensor.Name)
		write(uint32(len(tensor.Dims)))
		for _, dim := range tensor.Dims {
			write(uint64(dim))
		}
		write(tensor.GGMLType)
		write(tensor.Offset)
	}
	return data.Bytes()
}

func TestFromBytesDeepSeek41Cache(t *testing.T) {
	// Native payload expectations: 40 raw K caches × 768 cells × 1024 bytes,
	// three ratio-2 and one ratio-1 compressed source (1280 bytes/row),
	// and 28 KiB of F32 compressor state per current-state copy and stream.
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name           string
		context, slots int64
		unified, full  bool
		typeK          int32
		copies         int64
		want           int64
	}{
		{"8K", 8192, 1, false, false, 1, 1, 55*mib + 28672},
		{"32K", 32768, 1, false, false, 1, 1, 130*mib + 28672},
		{"1M", 1048576, 1, false, false, 1, 1, 3230*mib + 28672},
		{"padding boundary", 32769, 1, false, false, 1, 1, 130*mib + 1310720 + 28672},
		{"two streams", 32768, 2, false, false, 1, 1, 260*mib + 57344},
		{"unified capacity with two streams", 32768, 2, true, false, 1, 1, 460*mib + 57344},
		{"full raw cache", 32768, 1, false, true, 1, 1, 1380*mib + 28672},
		{"quantized K only", 32768, 1, false, false, 8, 1, 72417280 + 28672},
		{"rollback state copies", 32768, 1, false, false, 1, 4, 130*mib + 114688},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FromBytes(deepseek41Header(t, deepseek41Metadata(), nil), 200<<30, Config{
				ContextWindow: tt.context, Slots: tt.slots, NUBatch: 512,
				TypeK: tt.typeK, TypeV: 50, BytesPerElement: 4, // F32 V must not allocate a V cache.
				KVUnified: tt.unified, SWAFull: tt.full, RecurrentStateCopies: tt.copies,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.SlotMemory != tt.want || result.KVPerSlot != tt.want/tt.slots {
				t.Fatalf("cache total/per slot: got %d/%d, want %d/%d", result.SlotMemory, result.KVPerSlot, tt.want, tt.want/tt.slots)
			}
			if result.Input.ComputeKind != modelprofile.ComputeDeepSeek41 || result.Input.OutputEmbeddingLength != 5120 {
				t.Fatalf("V4.1 compute/output width: got %v/%d", result.Input.ComputeKind, result.Input.OutputEmbeddingLength)
			}
			if result.MoE == nil || !result.MoE.HasSharedExperts {
				t.Fatal("missing shared expert classification")
			}
		})
	}
}

func TestFromBytesDeepSeek41SourceValidation(t *testing.T) {
	for _, tt := range []struct{ key, value string }{
		{"attention.compress_ratios", "[0 0 2]"},
		{"attention.kv_source_layers", "[0 8 14 20]"},
		{"attention.kv_source_layers", "[2 8 14 40]"},
		{"attention.indexer.source_layers", "[8 14 20]"},
		{"attention.indexer.candidate_source_layer", "21"},
		{"ple.layers", "[14 1]"},
		{"ple.ngram_size", "1"},
	} {
		t.Run(tt.key+tt.value, func(t *testing.T) {
			metadata := deepseek41Metadata()
			metadata["deepseek41."+tt.key] = tt.value
			if _, err := FromBytes(deepseek41Header(t, metadata, nil), 200<<30, Config{}); err == nil {
				t.Fatal("invalid source metadata accepted")
			}
		})
	}
}

func TestFromFilesDeepSeek41Engram(t *testing.T) {
	const tableBytes int64 = 110595290400 // 768022850 IQ4_NL rows × 144 encoded bytes.
	var paths []string
	for i, tensors := range [][]gguf.TensorInfo{
		nil, // Actual first shard is metadata-only.
		{{Name: "token_embd.weight", Dims: []int64{5120, 129280}, GGMLType: 14}},
		{{Name: "per_layer_token_embd.weight", Dims: []int64{256, 768022850}, GGMLType: 20}},
	} {
		metadata := map[string]string{}
		if i == 0 {
			metadata = deepseek41Metadata()
		}
		path := filepath.Join(t.TempDir(), "shard.gguf")
		if err := os.WriteFile(path, deepseek41Header(t, metadata, tensors), 0600); err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			// Sparse: preserve the real table extent without allocating its payload.
			if err := os.Truncate(path, tableBytes+4096); err != nil {
				t.Fatal(err)
			}
		}
		paths = append(paths, path)
	}
	cfg := Config{ComputeBackend: "metal", ContextWindow: 32768, Slots: 1, TypeK: 1, NUBatch: 512,
		WeightLoading: WeightLoading{LazyMode: "on", LoadMode: "mmap", MMapSupported: true, Metal: true}}
	result, err := FromFiles(paths, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// 48 gathers/token, two batches, at most two host pages per encoded row.
	wantResident := max(256<<20, 98304*int64(os.Getpagesize()))
	if result.LazyTableBytes != tableBytes || result.LazyResidentBytes != wantResident {
		t.Fatalf("table/residency: got %d/%d, want %d/%d", result.LazyTableBytes, result.LazyResidentBytes, tableBytes, wantResident)
	}
	cfg.WeightLoading.LazyMode = "off"
	full, err := FromFiles(paths, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if full.LazyResidentBytes != tableBytes || full.UnifiedFootprint()-result.UnifiedFootprint() != tableBytes-wantResident {
		t.Fatal("disabled lazy loading did not restore full table residency")
	}
	cfg.WeightLoading.LazyMode = "on"
	cfg.WeightLoading.MMapSupported = false
	unsupported, err := FromFiles(paths, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.LazyResidentBytes != tableBytes {
		t.Fatal("unsupported lazy mapping did not retain full residency")
	}
}

func TestDeepSeek41CachePlacement(t *testing.T) {
	base, err := FromBytes(deepseek41Header(t, deepseek41Metadata(), nil), 200<<30, Config{
		ContextWindow: 32768, Slots: 1, NUBatch: 512, TypeK: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := base.Input
	input.GPULayers = 11 // Ten blocks, including consumers of CPU source 20.
	partial := Calculate(input)
	// CPU owns all four compressed sources and their state; GPU gets one
	// ratio-1 mirror, not a copy at every one of its ten consumer blocks.
	if partial.KVVRAMBytes != 49807360 || partial.KVCPUBytes != 128479232 || partial.SlotMemory != 178286592 {
		t.Fatalf("partial cache GPU/CPU/total: got %d/%d/%d", partial.KVVRAMBytes, partial.KVCPUBytes, partial.SlotMemory)
	}
	devices := base.CalculatePerDevice(2, []float64{1, 3}, nil, 0)
	// Native's N+1 split places blocks 0..10 on GPU0. Source 8 is mirrored
	// once on GPU1; it is not merely a 1:3 split of a single cache total.
	if devices[0].KVBytes != 50610176 || devices[1].KVBytes != 106704896 {
		t.Fatalf("two-device cache: got %d/%d, want 50610176/106704896", devices[0].KVBytes, devices[1].KVBytes)
	}
	assessment := AssessFit(base, FitConstraints{DeviceCount: 2, GPUFreeBytes: []int64{300 << 30, 300 << 30}, TensorSplit: []float64{1, 3}, SystemRAMBytes: 300 << 30})
	if assessment.GPU.RequiredBytes != devices[0].TotalBytes+devices[1].TotalBytes {
		t.Fatal("fit assessment omitted compressed-cache mirrors")
	}
	aggregate := AssessFit(base, FitConstraints{DeviceCount: 2, CombinedFreeBytes: 600 << 30, TensorSplit: []float64{1, 3}, SystemRAMBytes: 300 << 30})
	if aggregate.GPU.RequiredBytes != assessment.GPU.RequiredBytes {
		t.Fatal("aggregate capacity omitted compressed-cache mirrors")
	}
	input.KVCacheOnCPU = true
	cpu := Calculate(input)
	if cpu.KVVRAMBytes != 0 || cpu.SlotMemory != base.SlotMemory {
		t.Fatal("CPU-only cache retained a device mirror")
	}
}

func TestFromBytesDeepSeek41Compute(t *testing.T) {
	header := deepseek41Header(t, deepseek41Metadata(), nil)
	cfg := Config{ContextWindow: 32768, Slots: 1, NUBatch: 512, TypeK: 1}
	base, err := FromBytes(header, 200<<30, cfg)
	if err != nil {
		t.Fatal(err)
	}
	metadata := deepseek41Metadata()
	delete(metadata, "deepseek41.ple.layers")
	withoutTable, err := FromBytes(deepseek41Header(t, metadata, nil), 200<<30, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Forty-eight F32 gathers/token coexist on both sides. Host also owns
	// the I32 IDs; GPU adds one layer's projection/gating scratch.
	const gathered = 4 * 48 * 256 * 512
	if got := base.ComputeMemory.WorkspaceCPU - withoutTable.ComputeMemory.WorkspaceCPU; got != gathered+4*48*512 {
		t.Fatalf("host embedding workspace: got %d, want %d", got, gathered+4*48*512)
	}
	if got := base.ComputeMemory.WorkspaceGPU - withoutTable.ComputeMemory.WorkspaceGPU; got != gathered+8*5120*5*512 {
		t.Fatalf("GPU embedding workspace: got %d, want %d", got, gathered+8*5120*5*512)
	}
	for _, backend := range []string{"metal", "non-flash"} {
		t.Run(backend, func(t *testing.T) {
			cfg.ComputeBackend = backend
			cfg.VTransposed = backend == "non-flash"
			result, err := FromBytes(header, 200<<30, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if result.ComputeBufferEst <= base.ComputeBufferEst || result.SlotMemory != base.SlotMemory {
				t.Fatal("attention workspace did not grow independently of persistent K-only cache")
			}
		})
	}
	// Force vocabulary projection to dominate attention, then check that
	// the public row cap changes workspace, not persistent state.
	input := base.Input
	input.NUBatch = 2
	input.VocabularySize = 1 << 28
	input.OutputRows = 1
	one := Calculate(input)
	input.OutputRows = 2
	two := Calculate(input)
	if got := two.ComputeMemory.WorkspaceGPU - one.ComputeMemory.WorkspaceGPU; got != 4*input.VocabularySize || two.SlotMemory != one.SlotMemory {
		t.Fatalf("logits row workspace delta: got %d, want %d, cache %d/%d", got, 4*input.VocabularySize, two.SlotMemory, one.SlotMemory)
	}
}

func TestFromBytesKimiK31Memory(t *testing.T) {
	for _, tt := range []struct {
		name         string
		unified, cpu bool
		typeK        int32
		want         int64
	}{
		{"partitioned F16", false, false, 1, 921600},
		{"unified F16", true, false, 1, 790528},
		{"partitioned Q8", false, false, 8, 552960},
		{"unified Q8", true, false, 8, 483328},
		{"CPU cache", false, true, 1, 921600},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]string{
				"general.architecture": "kimi-k31",
				"kimi-k31.block_count": "5", "kimi-k31.embedding_length": "256",
				"kimi-k31.attention.head_count": "8", "kimi-k31.attention.head_count_kv": "[0 1 0 1 1]",
				"kimi-k31.attention.key_length": "64", "kimi-k31.attention.kv_lora_rank": "32",
				"kimi-k31.ssm.conv_kernel": "3", "kimi-k31.kda.head_dim": "16",
				"kimi-k31.attention.indexer.key_length": "32", "kimi-k31.attention.indexer.head_count": "4",
				"kimi-k31.attention.q_lora_rank": "64", "kimi-k31.attention.indexer.top_k": "64",
				"kimi-k31.attention.indexer.types": "[true true false false true]",
			}
			// Three K-only 64-wide MLA rows, two 32-wide indexer owners,
			// and two KDA states of 11264 bytes × 2 sequences × 3 copies.
			// 513 × 2 pads to 1280 shared cells or 768 × 2 separate cells.
			// Q8_0 rows occupy 34 bytes per 32 values, not one byte/value.
			result, err := FromBytes(deepseek41Header(t, metadata, nil), 1<<30, Config{
				ContextWindow: 513, Slots: 2, NUBatch: 128, BytesPerElement: 2,
				TypeK: tt.typeK, TypeV: 1, RecurrentStateCopies: 3,
				KVUnified: tt.unified, KVCacheOnCPU: tt.cpu,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.SlotMemory != tt.want || result.KVPerSlot != tt.want/2 {
				t.Errorf("persistent total/per slot: got %d/%d, want %d/%d", result.SlotMemory, result.KVPerSlot, tt.want, tt.want/2)
			}
			if tt.cpu && result.KVVRAMBytes != 0 {
				t.Errorf("CPU cache GPU allocation: got %d, want 0", result.KVVRAMBytes)
			}
			input := result.Input
			input.GPULayers = 2 // Output plus the final MLA/indexer owner.
			partial := Calculate(input)
			if !tt.cpu && partial.KVVRAMBytes != (tt.want-135168)*3/8 {
				t.Errorf("partial GPU cache: got %d, want %d", partial.KVVRAMBytes, (tt.want-135168)*3/8)
			}
		})
	}
}

func TestFromBytesLFM3Memory(t *testing.T) {
	for _, tt := range []struct {
		name, causal string
		full, onCPU  bool
		wantCache    int64
		wantErr      bool
	}{
		{"causal default", "", false, false, 4612096, false},
		{"causal explicit", "true", false, false, 4612096, false},
		{"full SWA", "true", true, false, 6316032, false},
		{"CPU cache", "true", false, true, 4612096, false},
		{"noncausal has no persistent memory", "false", false, false, 0, false},
		{"noncausal CPU cache flag", "false", false, true, 0, false},
		{"invalid causal flag", "invalid", false, false, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]string{
				"general.architecture": "lfm3",
				"lfm3.block_count":     "4", "lfm3.embedding_length": "256", "lfm3.vocab_size": "1024",
				"lfm3.attention.head_count": "8", "lfm3.attention.key_length": "32", "lfm3.attention.value_length": "32",
				"lfm3.attention.head_count_kv": "[0 2 0 4]", "lfm3.attention.sliding_window_pattern": "[false true false false]",
				"lfm3.attention.sliding_window": "512", "lfm3.shortconv.l_cache": "3",
			}
			if tt.causal != "" {
				metadata["lfm3.attention.causal"] = tt.causal
			}
			cfg := Config{ContextWindow: 4096, Slots: 2, NUBatch: 128, BytesPerElement: 2, RecurrentStateCopies: 3, SWAFull: tt.full, KVCacheOnCPU: tt.onCPU}
			result, err := FromBytes(deepseek41Header(t, metadata, nil), 1<<30, cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("FromBytes: got error %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if result.SlotMemory != tt.wantCache || result.KVPerSlot != tt.wantCache/2 {
				t.Errorf("persistent total/per slot: got %d/%d, want %d/%d", result.SlotMemory, result.KVPerSlot, tt.wantCache, tt.wantCache/2)
			}
			if tt.onCPU && result.KVVRAMBytes != 0 {
				t.Errorf("GPU cache: got %d, want 0", result.KVVRAMBytes)
			}
			if result.Input.ComputeKind != modelprofile.ComputeLFM3 || result.ComputeMemory.RuntimeAllowanceGPU != 256<<20 {
				t.Errorf("compute policy: got %v, allowance %d", result.Input.ComputeKind, result.ComputeMemory.RuntimeAllowanceGPU)
			}
			input := result.Input
			input.ComputeBackend = "metal"
			metal := Calculate(input)
			if (!tt.onCPU || input.NoPersistentCache) && metal.ComputeBufferEst <= result.ComputeBufferEst {
				t.Error("Metal attention scratch did not increase workspace")
			}
			input.ContextWindow *= 2
			larger := Calculate(input)
			if input.NoPersistentCache && (larger.SlotMemory != 0 || larger.ComputeBufferEst != metal.ComputeBufferEst) {
				t.Error("batch-local decision memory depends on configured cache capacity")
			}
			if !input.NoPersistentCache && !tt.onCPU && larger.ComputeBufferEst <= metal.ComputeBufferEst {
				t.Error("causal attention workspace did not grow with context")
			}
			input = result.Input
			input.GPULayers = 2 // Output plus the last full-attention block.
			partial := Calculate(input)
			if !input.NoPersistentCache && !tt.onCPU && partial.KVVRAMBytes != 4194304 {
				t.Errorf("partial-offload GPU cache: got %d, want 4194304", partial.KVVRAMBytes)
			}
			if tt.causal == "false" {
				metadata["lfm3.decision.type"] = "lfm3-d1"
				d1, err := FromBytes(deepseek41Header(t, metadata, nil), 1<<30, cfg)
				if err != nil {
					t.Fatal(err)
				}
				if d1.Input.NUBatch != 4096 || d1.Input.Slots != 1 || d1.Input.OutputRows != 1 || d1.SlotMemory != 0 {
					t.Errorf("d1 must budget one whole-context physical batch without cache: %+v", d1.Input)
				}
				if d1.ComputeBufferEst <= result.ComputeBufferEst {
					t.Error("whole-prompt d1 workspace did not exceed the 128-token batch allowance")
				}
			}
		})
	}
}

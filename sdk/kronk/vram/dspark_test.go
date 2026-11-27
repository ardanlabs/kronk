package vram

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

func dsparkMetadata() map[string]string {
	return map[string]string{
		"general.architecture": "dflash",
		"dflash.block_count":   "3", "dflash.embedding_length": "2304",
		"dflash.attention.head_count": "24", "dflash.attention.head_count_kv": "8",
		"dflash.attention.key_length": "128", "dflash.attention.value_length": "128",
		"dflash.attention.sliding_window":         "1025",
		"dflash.attention.sliding_window_pattern": "[true false true]",
		"dflash.block_size":                       "8", "dflash.target_layers": "[5, 23, 41, 48]",
		"dflash.has_markov_context": "true", "dflash.has_confidence_head": "false",
		"dflash.sample_from_anchor": "false",
	}
}

func TestDSparkFromFiles(t *testing.T) {
	// Header-only fixtures retain real dimensions, not multi-GB model data.
	tensors := []gguf.TensorInfo{
		{Name: "markov_w1.weight", Dims: []int64{256, 200058}},
		{Name: "markov_w2.weight", Dims: []int64{256, 200058}},
	}
	data := deepseek41Header(t, dsparkMetadata(), tensors)
	path := filepath.Join(t.TempDir(), "dspark.gguf")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{ContextWindow: 4096, Slots: 3, NBatch: 97, NUBatch: 13, BytesPerElement: 2,
		RecurrentStateCopies: 9, ComputeContexts: 2, MTPContext: true, OutputRows: 1}
	gpu, err := FromFiles([]string{path}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	in := gpu.Input
	if in.dsparkFeatureWidth != 9216 || in.OutputRows != 24 || in.VocabularySize != 200058 ||
		in.ComputeContexts != 1 || in.RecurrentStateCopies != 1 || in.MTPContext {
		t.Fatalf("sidecar normalization: got %+v", in)
	}
	if gpu.ModelWeightsGPU+gpu.ModelWeightsCPU != int64(len(data)) || in.TiedOutputFallback {
		t.Errorf("weights: got %d, want own file only %d", gpu.ModelWeightsGPU+gpu.ModelWeightsCPU, len(data))
	}
	if gpu.SlotMemory == 0 || gpu.SlotMemory != genericCache(in).SlotMemory {
		t.Errorf("independent KV: got %d, want %d", gpu.SlotMemory, genericCache(in).SlotMemory)
	}
	base := llamaComputeMemory(in)
	wantHost := int64(12*97*9216 + 4*97*2304 + 4*8*2304 + 28*200058 + 4*2304 + 4*3*7*2304 + 3*9*(4*2304+8) + 4*24*200058)
	if got := gpu.ComputeMemory.WorkspaceCPU - base.WorkspaceCPU; got != wantHost {
		t.Errorf("host allocations: got %d, want %d", got, wantHost)
	}
	// Graph allowance must exceed even the padded borrowed projection's rows.
	if got := gpu.ComputeMemory.WorkspaceGPU - base.WorkspaceGPU; got < 4*13*201024 {
		t.Errorf("padded projection allowance: got %d, want >= %d", got, 4*13*201024)
	}
	if gpu.UnifiedFootprint() <= int64(len(data))+gpu.SlotMemory+wantHost {
		t.Error("footprint omitted compute or synchronized buffers")
	}
	cfg.GPULayers = -1
	cpu, err := FromFiles([]string{path}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cpu.TotalVRAM != 0 || cpu.KVCPUBytes != cpu.SlotMemory || cpu.ModelWeightsCPU != int64(len(data)) {
		t.Errorf("CPU placement: got VRAM/KV/weights %d/%d/%d", cpu.TotalVRAM, cpu.KVCPUBytes, cpu.ModelWeightsCPU)
	}
	// One combined CPU margin can round a byte above the split CPU/GPU margins.
	if delta := cpu.UnifiedFootprint() - gpu.UnifiedFootprint(); delta < 0 || delta > 1 {
		t.Errorf("placement footprint: got CPU %d, GPU %d", cpu.UnifiedFootprint(), gpu.UnifiedFootprint())
	}
	cfg.GPULayers, cfg.KVCacheOnCPU = 0, true
	hostKV, err := FromFiles([]string{path}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if hostKV.KVVRAMBytes != 0 || hostKV.KVCPUBytes != gpu.SlotMemory {
		t.Errorf("host KV: got %d/%d, want 0/%d", hostKV.KVVRAMBytes, hostKV.KVCPUBytes, gpu.SlotMemory)
	}
}

func TestDSparkAllocationAxes(t *testing.T) {
	input := Input{Architecture: "dflash", EmbeddingLength: 2304, VocabularySize: 200058,
		ContextWindow: 4096, Slots: 3, NBatch: 97, NUBatch: 13,
		dsparkBlockSize: 8, dsparkFeatureWidth: 9216, OutputRows: 24}
	base := dsparkComputeMemory(input)
	for _, tt := range []struct {
		name                   string
		batch, slots, features int64
		wantHost               int64
	}{
		{"logical rows", 98, 3, 9216, 12*9216 + 4*2304},
		{"feature layers", 97, 3, 11520, 12 * 97 * 2304},
		{"retained slots", 97, 4, 9216, 9*(4*2304+8) + 4*7*2304},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changed := input
			changed.NBatch, changed.Slots, changed.dsparkFeatureWidth = tt.batch, tt.slots, tt.features
			// Remove the common graph heuristic to isolate allocation formulas.
			got := dsparkComputeMemory(changed).WorkspaceCPU - base.WorkspaceCPU
			got -= llamaComputeMemory(changed).WorkspaceCPU - llamaComputeMemory(input).WorkspaceCPU
			if got != tt.wantHost {
				t.Errorf("host allocation delta: got %d, want %d", got, tt.wantHost)
			}
		})
	}
	input.NBatch = 5
	if err := configureDSpark(&input, dsparkMetadata(), []gguf.TensorInfo{{Name: "markov_w1.weight", Dims: []int64{256, 200058}}}); err != nil {
		t.Fatal(err)
	}
	if input.OutputRows != 5 {
		t.Errorf("batch-bounded logits rows: got %d, want 5", input.OutputRows)
	}
}

func TestDSparkEstimateScope(t *testing.T) {
	for _, architecture := range []string{"inkling2_loop", "llama", "dflash"} {
		in := Input{Architecture: architecture, EmbeddingLength: 2304, NBatch: 97, NUBatch: 13, Slots: 3}
		before := Calculate(in)
		tensors := []gguf.TensorInfo{{Name: "markov_w1.weight", Dims: []int64{256, 200058}}}
		if architecture == "dflash" {
			tensors = nil
		}
		if err := configureDSpark(&in, dsparkMetadata(), tensors); err != nil {
			t.Fatal(err)
		}
		after := Calculate(in)
		if after.UnifiedFootprint() != before.UnifiedFootprint() || in.dsparkBlockSize != 0 {
			t.Errorf("%s without qualified sidecar: estimate changed", architecture)
		}
	}
}

func TestDSparkMetadataLimits(t *testing.T) {
	for _, tt := range []struct {
		key, value string
	}{
		{"dflash.block_size", "0"},
		{"dflash.block_size", "bad"},
		{"dflash.target_layers", "[]"},
		{"dflash.target_layers", "bad"},
	} {
		t.Run(tt.key+"/"+tt.value, func(t *testing.T) {
			metadata := dsparkMetadata()
			metadata[tt.key] = tt.value
			_, err := buildFromMetadata(metadata, [][]gguf.TensorInfo{{{Name: "markov_w1.weight", Dims: []int64{256, 200058}}}}, 1024, Config{})
			if err == nil {
				t.Fatal("invalid sidecar dimensions: got nil error")
			}
		})
	}
	for _, tt := range []struct {
		batch, physical, want int64
	}{
		{0, 17, 17}, {19, 17, 19}, {0, 0, DefaultNUBatch},
	} {
		if got := dsparkBatch(Input{NBatch: tt.batch, NUBatch: tt.physical}); got != tt.want {
			t.Errorf("logical batch fallback: got %d, want %d", got, tt.want)
		}
	}
}

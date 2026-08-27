package vram

import "testing"

func TestCalculateSWAFull(t *testing.T) {
	input := Input{
		ContextWindow:       1024,
		BlockCount:          4,
		HeadCountKV:         2,
		KeyLength:           8,
		ValueLength:         8,
		BytesPerElement:     1,
		Slots:               2,
		SlidingWindow:       16,
		SlidingWindowLayers: 3,
		NUBatch:             32,
		KVUnified:           true,
	}

	compact := Calculate(input)
	input.SWAFull = true
	full := Calculate(input)

	const kvPerTokenPerLayer int64 = 2 * (8 + 8)
	wantCompact := (int64(2)*1024 + 3*256) * kvPerTokenPerLayer
	wantFull := int64(2) * 4 * 1024 * kvPerTokenPerLayer

	if compact.SlotMemory != wantCompact {
		t.Errorf("compact SlotMemory: got %d, want %d", compact.SlotMemory, wantCompact)
	}
	if full.SlotMemory != wantFull {
		t.Errorf("full SlotMemory: got %d, want %d", full.SlotMemory, wantFull)
	}
}

func TestCalculateSWAFullPreservesSWADimensions(t *testing.T) {
	input := Input{
		ContextWindow:       128,
		BlockCount:          6,
		HeadCountKV:         2,
		KeyLength:           8,
		ValueLength:         8,
		SWAHeadCountKV:      4,
		SWAKeyLength:        4,
		SWAValueLength:      4,
		BytesPerElement:     1,
		Slots:               1,
		SlidingWindow:       16,
		SlidingWindowLayers: 5,
		SWAFull:             true,
	}

	got := Calculate(input)
	want := int64(256) * (2*(8+8) + 5*4*(4+4))
	if got.SlotMemory != want {
		t.Fatalf("SlotMemory: got %d, want %d", got.SlotMemory, want)
	}
}

func TestCalculatePlacement(t *testing.T) {
	input := Input{
		ModelSizeBytes:  600,
		ContextWindow:   10,
		BlockCount:      6,
		HeadCountKV:     1,
		KeyLength:       1,
		ValueLength:     1,
		BytesPerElement: 1,
		Slots:           1,
	}

	allGPU := Calculate(input)
	if allGPU.ModelWeightsGPU != 600 || allGPU.ModelWeightsCPU != 0 {
		t.Fatalf("default weights: got gpu=%d cpu=%d, want 600/0", allGPU.ModelWeightsGPU, allGPU.ModelWeightsCPU)
	}

	input.GPULayers = -1
	cpuOnly := Calculate(input)
	if cpuOnly.ModelWeightsGPU != 0 || cpuOnly.ModelWeightsCPU != 600 {
		t.Fatalf("CPU-only weights: got gpu=%d cpu=%d, want 0/600", cpuOnly.ModelWeightsGPU, cpuOnly.ModelWeightsCPU)
	}
	if cpuOnly.KVVRAMBytes != 0 || cpuOnly.KVCPUBytes != 3072 {
		t.Fatalf("CPU KV: got gpu=%d cpu=%d, want 0/3072", cpuOnly.KVVRAMBytes, cpuOnly.KVCPUBytes)
	}
}

func TestCalculatePartialOffloadPlacesKVByLayer(t *testing.T) {
	input := Input{
		ContextWindow:       1024,
		BlockCount:          6,
		HeadCountKV:         1,
		KeyLength:           1,
		ValueLength:         1,
		SWAHeadCountKV:      1,
		SWAKeyLength:        1,
		SWAValueLength:      0,
		BytesPerElement:     1,
		Slots:               1,
		SlidingWindow:       256,
		SlidingWindowLayers: 3,
		SWAPattern:          []bool{true, false, true, false, true, false},
		GPULayers:           3,
	}

	got := Calculate(input)
	// Full layers use 1024*(1+1)=2048 bytes. SWA layers use
	// PAD256(256)*(1+1)=512 bytes. The trailing GPU half contains
	// full/SWA/full; the CPU half contains SWA/full/SWA.
	if got.KVVRAMBytes != 2048+512+2048 {
		t.Fatalf("KVVRAMBytes: got %d, want %d", got.KVVRAMBytes, 2048+512+2048)
	}
	if got.KVCPUBytes != 512+2048+512 {
		t.Fatalf("KVCPUBytes: got %d, want %d", got.KVCPUBytes, 512+2048+512)
	}
}

func TestBuildFromMetadataQwen35Hybrid(t *testing.T) {
	metadata := map[string]string{
		"general.architecture":              "qwen35moe",
		"qwen35moe.block_count":             "5",
		"qwen35moe.embedding_length":        "8",
		"qwen35moe.attention.head_count":    "1",
		"qwen35moe.attention.head_count_kv": "1",
		"qwen35moe.attention.key_length":    "8",
		"qwen35moe.attention.value_length":  "8",
		"qwen35moe.nextn_predict_layers":    "1",
		"qwen35moe.ssm.conv_kernel":         "2",
		"qwen35moe.ssm.inner_size":          "8",
		"qwen35moe.ssm.state_size":          "4",
		"qwen35moe.ssm.group_count":         "1",
	}

	got, err := buildFromMetadata(metadata, nil, 1000, Config{
		ContextWindow:          256,
		BytesPerElement:        2,
		Slots:                  1,
		EmbeddedMTPStateCopies: 3,
	})
	if err != nil {
		t.Fatalf("buildFromMetadata: %v", err)
	}

	const recurrentStateBytes int64 = 4 * ((8 + 2*1*4) + 4*8)
	want := int64(256*(8+8)*2) + 3*recurrentStateBytes*3
	if got.SlotMemory != want {
		t.Fatalf("SlotMemory: got %d, want %d", got.SlotMemory, want)
	}
	if got.Input.RecurrentStateCopies != 3 {
		t.Fatalf("RecurrentStateCopies: got %d, want 3", got.Input.RecurrentStateCopies)
	}
	if got.Input.ComputeContexts != 2 {
		t.Fatalf("ComputeContexts: got %d, want 2", got.Input.ComputeContexts)
	}
}

func TestIsFolderURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "shorthand-with-tag",
			url:  "bartowski/Qwen3-8B-GGUF:Q4_K_M",
			want: false,
		},
		{
			name: "shorthand-with-revision",
			url:  "bartowski/Qwen3-8B-GGUF:Q4_K_M@main",
			want: false,
		},
		{
			name: "shorthand-with-hf-prefix",
			url:  "hf.co/bartowski/Qwen3-8B-GGUF:Q4_K_M",
			want: false,
		},
		{
			name: "full-gguf-url",
			url:  "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf",
			want: false,
		},
		{
			name: "short-form-gguf",
			url:  "Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf",
			want: false,
		},
		{
			name: "blob-url",
			url:  "https://huggingface.co/Qwen/Qwen3-8B-GGUF/blob/main/Qwen3-8B-Q8_0.gguf",
			want: false,
		},
		{
			name: "folder-tree-url",
			url:  "https://huggingface.co/unsloth/Qwen3-Coder-Next-GGUF/tree/main/UD-Q5_K_XL",
			want: true,
		},
		{
			name: "short-form-folder",
			url:  "owner/repo/subfolder",
			want: true,
		},
		{
			name: "owner-repo-only",
			url:  "owner/repo",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isFolderURL(tt.url)
			if got != tt.want {
				t.Errorf("isFolderURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

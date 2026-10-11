package vram

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

func TestFromFilesOutputPlacement(t *testing.T) {
	for _, tt := range []struct {
		name, arch, output string
		wantGPU            int64
	}{
		{"output in later shard", "qwen3", "output.weight", 68},
		{"tied language output", "qwen3", "", 100},
		{"embedding without output", "bert", "", 0},
		{"classifier output", "bert", "cls.weight", 68},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var paths []string
			for shard, tensorName := range []string{"token_embd.weight", tt.output} {
				if tensorName == "" {
					continue
				}
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
				write(uint64(1))
				write(uint64(5))
				writeString("general.architecture")
				write(gguf.MetadataValueTypeString)
				writeString(tt.arch)
				for key, value := range map[string]uint32{"block_count": 2, "attention.head_count": 1, "attention.key_length": 8, "attention.value_length": 8} {
					writeString(tt.arch + "." + key)
					write(gguf.MetadataValueTypeUInt32)
					write(value)
				}
				writeString(tensorName)
				write(uint32(1))
				if shard == 0 {
					write(uint64(25)) // F32 input table: 100 bytes.
				} else {
					write(uint64(17)) // F32 output table: 68 bytes.
				}
				write(uint32(0))
				write(uint64(0))
				path := filepath.Join(t.TempDir(), "shard.gguf")
				if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
			}
			result, err := FromFiles(paths, Config{GPULayers: 1})
			if err != nil {
				t.Fatal(err)
			}
			if result.ModelWeightsGPU != tt.wantGPU {
				t.Errorf("output-only GPU weights: got %d, want %d", result.ModelWeightsGPU, tt.wantGPU)
			}
			if result.ModelWeightsCPU != result.Input.ModelSizeBytes-68 && tt.output != "" {
				t.Errorf("CPU weights: got %d, want %d", result.ModelWeightsCPU, result.Input.ModelSizeBytes-68)
			}
		})
	}
}

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
	// PAD256(256)*(1+1)=512 bytes. Output uses the first offload
	// unit, leaving just the trailing SWA/full blocks on GPU.
	if got.KVVRAMBytes != 512+2048 {
		t.Fatalf("KVVRAMBytes: got %d, want %d", got.KVVRAMBytes, 512+2048)
	}
	if got.KVCPUBytes != 512+2048+512+2048 {
		t.Fatalf("KVCPUBytes: got %d, want %d", got.KVCPUBytes, 512+2048+512+2048)
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
	// Target attention plus appended draft attention, and three target
	// recurrent layers with current state and two rollback planes.
	want := int64(2*256*(8+8)*2) + 3*recurrentStateBytes*3
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

func TestBuildFromMetadataGLM5Next(t *testing.T) {
	headCountKV := "[" + strings.TrimSpace(strings.Repeat("0 ", 34)+strings.Repeat("1 ", 12)) + "]"
	metadata := map[string]string{
		"general.architecture":              "glm5-next",
		"glm5-next.block_count":             "46",
		"glm5-next.embedding_length":        "6144",
		"glm5-next.expert_count":            "256",
		"glm5-next.nextn_predict_layers":    "1",
		"glm5-next.attention.head_count":    "64",
		"glm5-next.attention.head_count_kv": headCountKV,
		"glm5-next.attention.key_length":    "512",
		"glm5-next.attention.value_length":  "512",
		"glm5-next.ssm.conv_kernel":         "4",
		"glm5-next.kda.head_dim":            "128",
	}

	got, err := buildFromMetadata(metadata, nil, 1000, Config{
		ContextWindow:        256,
		BytesPerElement:      2,
		Slots:                2,
		RecurrentStateCopies: 3,
	})
	if err != nil {
		t.Fatalf("buildFromMetadata: %v", err)
	}

	const recurrentStateBytes int64 = 4 * (3*(4-1)*(64*128) + 128*128*64)
	wantRecurrent := int64(34) * recurrentStateBytes * 2 * 3
	wantAttention := int64(11) * 512 * (512 + 512) * 2
	want := wantRecurrent + wantAttention
	if got.SlotMemory != want {
		t.Fatalf("SlotMemory = %d, want %d", got.SlotMemory, want)
	}
	if got.Input.RecurrentStateBytes != recurrentStateBytes || got.Input.RecurrentStateCopies != 3 {
		t.Fatalf("recurrent state = bytes %d copies %d, want %d/3",
			got.Input.RecurrentStateBytes, got.Input.RecurrentStateCopies, recurrentStateBytes)
	}
}

func TestFromBytesIndexedHybridMemory(t *testing.T) {
	// 36 recurrent layers × four state planes. Each F32 plane holds
	// (4−1)×(6144+2×16×128) convolution values and 128×6144 state values.
	const recurrentBytes int64 = 36 * 4 * 4 * (3*(6144+2*16*128) + 128*6144)
	for _, tt := range []struct {
		name           string
		context, slots int64
		mtp, embedded  bool
		indexed        bool
		typeK          int32
		wantCache      int64
	}{
		{"target f16", 131072, 1, false, false, true, 1, 3840*1024*1024 + recurrentBytes},
		{"companion f16", 131072, 1, true, false, true, 1, 320 * 1024 * 1024},
		{"embedded f16", 131072, 1, false, true, true, 1, 4160*1024*1024 + recurrentBytes},
		{"quantized K", 131072, 1, false, false, true, 8, 2760*1024*1024 + recurrentBytes},
		{"padded two slots", 32769, 2, false, false, true, 1, 12*33024*2*2560 + 2*recurrentBytes},
		{"non indexed control", 131072, 1, false, false, false, 1, 3072*1024*1024 + recurrentBytes},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]string{
				"general.architecture":             "qwen4exp",
				"qwen4exp.block_count":             "49",
				"qwen4exp.embedding_length":        "2560",
				"qwen4exp.attention.head_count":    "24",
				"qwen4exp.attention.head_count_kv": "2",
				"qwen4exp.attention.key_length":    "256",
				"qwen4exp.attention.value_length":  "256",
				"qwen4exp.nextn_predict_layers":    "1",
				"qwen4exp.ssm.conv_kernel":         "4",
				"qwen4exp.ssm.inner_size":          "6144",
				"qwen4exp.ssm.state_size":          "128",
				"qwen4exp.ssm.group_count":         "16",
				"qwen4exp.hyper_connection.count":  "4",
			}
			if tt.indexed {
				metadata["qwen4exp.attention.indexer.key_length"] = "128"
				metadata["qwen4exp.attention.indexer.head_count"] = "4"
				metadata["qwen4exp.attention.indexer.top_k"] = "2048"
				metadata["qwen4exp.attention.compress_ratios"] = "[0, 0, 0, 4]"
			}
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
			write(uint64(0))
			write(uint64(len(metadata)))
			for key, value := range metadata {
				writeString(key)
				write(gguf.MetadataValueTypeString)
				writeString(value)
			}
			cfg := Config{ContextWindow: tt.context, Slots: tt.slots, TypeK: tt.typeK, TypeV: 1, BytesPerElement: 2, NUBatch: 2052, RecurrentStateCopies: 4, MTPContext: tt.mtp}
			if tt.embedded {
				cfg.ComputeContexts = 2
			}
			result, err := FromBytes(data.Bytes(), 80<<30, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if result.SlotMemory != tt.wantCache {
				t.Errorf("cache: got %d, want %d", result.SlotMemory, tt.wantCache)
			}
			if tt.indexed {
				if result.ComputeBufferEst < 4*tt.context*2052 || result.ComputeBufferCPU < 2*tt.context*2052 {
					t.Errorf("indexed workspaces not reserved: GPU=%d CPU=%d", result.ComputeBufferEst, result.ComputeBufferCPU)
				}
				if tt.name == "target f16" && (result.ComputeBufferEst != 2264583296 || result.ComputeBufferCPU != 832960972) {
					t.Errorf("Flash Next workspace regression: got GPU/CPU %d/%d, want 2264583296/832960972", result.ComputeBufferEst, result.ComputeBufferCPU)
				}
			} else if result.ComputeBufferCPU != 0 {
				t.Errorf("non-indexed host workspace: got %d, want 0", result.ComputeBufferCPU)
			}
			wantUnified := int64(80<<30) + tt.wantCache + result.ComputeBufferEst + result.ComputeBufferCPU
			if result.UnifiedFootprint() != wantUnified {
				t.Errorf("unified memory: got %d, want %d", result.UnifiedFootprint(), wantUnified)
			}
			if result.TotalSystemRAMEst != result.ComputeBufferCPU {
				t.Errorf("host workspace missing from system memory: got %d, want %d", result.TotalSystemRAMEst, result.ComputeBufferCPU)
			}
		})
	}
}

func TestFromBytesGemmaWorkspacesAndSharedKV(t *testing.T) {
	for _, tt := range []struct {
		arch, embedding    string
		shared, nextn      string
		wantCache, wantGPU int64
	}{
		// The target has four ordinary KV layers in this synthetic fixture.
		{"gemma4", "2816", "0", "0", 2 << 30, 2865525555},
		// The assistant borrows all four layers, but retains its own arena.
		{"gemma4-assistant", "1024", "4", "4", 0, 2736088678},
	} {
		t.Run(tt.arch, func(t *testing.T) {
			metadata := map[string]string{
				"general.architecture":                  tt.arch,
				tt.arch + ".block_count":                "4",
				tt.arch + ".embedding_length":           tt.embedding,
				tt.arch + ".embedding_length_out":       "2816",
				tt.arch + ".attention.head_count":       "16",
				tt.arch + ".attention.head_count_kv":    "2",
				tt.arch + ".attention.key_length":       "512",
				tt.arch + ".attention.value_length":     "512",
				tt.arch + ".attention.shared_kv_layers": tt.shared,
				tt.arch + ".nextn_predict_layers":       tt.nextn,
			}
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
			write(uint64(1))
			write(uint64(len(metadata)))
			for key, value := range metadata {
				writeString(key)
				write(gguf.MetadataValueTypeString)
				writeString(value)
			}
			// No vocab_size metadata: recover the 262144 output rows from
			// the embedding descriptor, as the real Gemma files require.
			writeString("token_embd.weight")
			write(uint32(2))
			if tt.arch == "gemma4" {
				write(uint64(2816))
			} else {
				write(uint64(1024))
			}
			write(uint64(262144))
			write(uint32(1))
			write(uint64(0))
			result, err := FromBytes(data.Bytes(), 4<<30, Config{ComputeBackend: "metal", ContextWindow: 131072, Slots: 1, TypeK: 1, TypeV: 1, BytesPerElement: 2, NUBatch: 2052, ComputeContexts: 1, SWAFull: true})
			if err != nil {
				t.Fatal(err)
			}
			if result.Input.ComputeBackend != "metal" {
				t.Fatalf("compute backend: got %q, want metal", result.Input.ComputeBackend)
			}
			// 1026 MiB for the two F16 masks plus 22.04296875 MiB
			// hidden rows; the existing ten-percent margin applies once.
			const wantCPU int64 = 1208847974
			if result.SlotMemory != tt.wantCache || result.ComputeBufferEst != tt.wantGPU || result.ComputeBufferCPU != wantCPU {
				t.Fatalf("cache/GPU compute/CPU compute: got %d/%d/%d, want %d/%d/%d", result.SlotMemory, result.ComputeBufferEst, result.ComputeBufferCPU, tt.wantCache, tt.wantGPU, wantCPU)
			}
			if result.UnifiedFootprint() != 4<<30+tt.wantCache+tt.wantGPU+wantCPU {
				t.Fatal("unified footprint omitted a workspace or counted shared KV twice")
			}
			if result.TotalSystemRAMEst != result.ModelWeightsCPU+wantCPU {
				t.Fatal("system memory omitted host workspace")
			}
		})
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

func TestParseFolderURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantOwner    string
		wantRepo     string
		wantRevision string
		wantPath     string
	}{
		{
			name:         "hf short host and branch",
			url:          "https://hf.co/owner/repo/tree/feature-branch/quantized/Q4_K_M",
			wantOwner:    "owner",
			wantRepo:     "repo",
			wantRevision: "feature-branch",
			wantPath:     "quantized/Q4_K_M",
		},
		{
			name:         "shorthand defaults to main",
			url:          "owner/repo/quantized/Q4_K_M",
			wantOwner:    "owner",
			wantRepo:     "repo",
			wantRevision: "main",
			wantPath:     "quantized/Q4_K_M",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, revision, path, err := parseFolderURL(tt.url)
			if err != nil {
				t.Fatalf("parseFolderURL: %v", err)
			}
			if owner != tt.wantOwner || repo != tt.wantRepo || revision != tt.wantRevision || path != tt.wantPath {
				t.Errorf("parseFolderURL: got %q/%q/%q/%q, want %q/%q/%q/%q",
					owner, repo, revision, path, tt.wantOwner, tt.wantRepo, tt.wantRevision, tt.wantPath)
			}
		})
	}
}

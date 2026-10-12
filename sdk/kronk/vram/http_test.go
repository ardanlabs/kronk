package vram

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
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

func TestFromFilesLazyTables(t *testing.T) {
	for _, arch := range []string{"gemma4", "qwen4exp", "gemma4-assistant", "llama"} {
		for _, isolated := range []bool{false, true} {
			t.Run(arch+" isolated="+map[bool]string{false: "false", true: "true"}[isolated], func(t *testing.T) {
				var paths []string
				groups := [][]string{{"per_layer_token_embd.weight", "token_embd.weight"}}
				if isolated {
					groups = [][]string{{"token_embd.weight"}, {"per_layer_token_embd.weight"}}
				}
				for _, names := range groups {
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
					write(uint64(len(names)))
					write(uint64(9))
					writeString("general.architecture")
					write(gguf.MetadataValueTypeString)
					writeString(arch)
					for key, value := range map[string]uint32{"block_count": 2, "attention.head_count": 1, "attention.key_length": 8, "attention.value_length": 8, "embedding_length_per_layer_input": 512, "ple.ngram_size": 4, "ple.heads_per_ngram": 2} {
						writeString(arch + "." + key)
						write(gguf.MetadataValueTypeUInt32)
						write(value)
					}
					writeString(arch + ".ple.layers")
					write(gguf.MetadataValueTypeArray)
					write(gguf.MetadataValueTypeUInt32)
					write(uint64(1))
					write(uint32(0))
					var offset uint64
					for _, name := range names {
						writeString(name)
						write(uint32(2))
						write(uint64(512))
						rows := uint64(8)
						if name == "per_layer_token_embd.weight" {
							rows = 1 << 22
						}
						write(rows)
						write(uint32(8)) // Q8_0: each 512-element row is 544 bytes.
						write(offset)
						offset += 544 * rows
					}
					path := filepath.Join(t.TempDir(), "model.gguf")
					if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Truncate(path, int64(offset)+(1<<20)); err != nil {
						t.Fatal(err)
					}
					paths = append(paths, path)
				}
				cfg := Config{ComputeBackend: "metal", NUBatch: 8193, WeightLoading: WeightLoading{LazyMode: "on", LoadMode: "none", MMapSupported: true}}
				result, err := FromFiles(paths, cfg)
				if err != nil {
					t.Fatal(err)
				}
				const tableBytes int64 = 544 * (1 << 22)
				wantTable, wantResident := tableBytes, max(256<<20, 32772*int64(os.Getpagesize()))
				switch arch {
				case "qwen4exp":
					// Six hashed gathers/token, each up to two pages, two batches.
					wantResident = min(tableBytes, max(256<<20, 196632*int64(os.Getpagesize())))
				case "gemma4-assistant", "llama":
					wantTable, wantResident = 0, 0
				}
				if result.LazyTableBytes != wantTable || result.LazyResidentBytes != wantResident {
					t.Fatalf("encoded/resident table: got %d/%d, want %d/%d", result.LazyTableBytes, result.LazyResidentBytes, wantTable, wantResident)
				}
				cfg.WeightLoading.LoadMode = "mmap"
				mapped, err := FromFiles(paths, cfg)
				if err != nil {
					t.Fatal(err)
				}
				if !isolated {
					wantResident = wantTable
				}
				if mapped.LazyResidentBytes != wantResident {
					t.Errorf("Metal mmap resident table: got %d, want %d", mapped.LazyResidentBytes, wantResident)
				}
			})
		}
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
	if got.Input.ComputeKind != modelprofile.ComputeQwen35 || got.Input.OutputRows != 3 {
		t.Fatalf("compute kind/output rows: got %d/%d, want %d/3", got.Input.ComputeKind, got.Input.OutputRows, modelprofile.ComputeQwen35)
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

func TestFromBytesLlamaWorkspace(t *testing.T) {
	const mib int64 = 1 << 20
	for _, tt := range []struct {
		name, backend, blocks, hidden, width string
		slots, cacheMiB, gpuMiB, cpuMiB      int64
	}{
		{"8B", "metal", "32", "4096", "128", 1, 16384, 1536, 640},
		{"1B", "metal", "16", "2048", "64", 1, 4096, 1152, 576},
		{"8B two slots", "metal", "32", "4096", "128", 2, 32768, 2112, 640},
		{"1B CUDA control", "cuda", "16", "2048", "64", 1, 4096, 896, 576},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]string{
				"general.architecture": "llama", "llama.block_count": tt.blocks,
				"llama.embedding_length": tt.hidden, "llama.vocab_size": "128256",
				"llama.attention.head_count": "32", "llama.attention.head_count_kv": "8",
				"llama.attention.key_length": tt.width, "llama.attention.value_length": tt.width,
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
			result, err := FromBytes(data.Bytes(), 4<<30, Config{ComputeBackend: tt.backend, ContextWindow: 131072, Slots: tt.slots, TypeK: 1, TypeV: 1, BytesPerElement: 2, NUBatch: 2048})
			if err != nil {
				t.Fatal(err)
			}
			gpu, cpu := tt.gpuMiB*mib, tt.cpuMiB*mib
			gpu, cpu = gpu+gpu/10, cpu+cpu/10
			if result.SlotMemory != tt.cacheMiB*mib || result.ComputeBufferEst != gpu || result.ComputeBufferCPU != cpu {
				t.Fatalf("cache/GPU compute/CPU compute: got %d/%d/%d, want %d/%d/%d", result.SlotMemory, result.ComputeBufferEst, result.ComputeBufferCPU, tt.cacheMiB*mib, gpu, cpu)
			}
			if result.UnifiedFootprint() != 4<<30+tt.cacheMiB*mib+gpu+cpu {
				t.Fatal("unified footprint omitted cache or workspace")
			}
		})
	}
}

func TestFromBytesGemma3RNJWorkspace(t *testing.T) {
	const mib int64 = 1 << 20
	metadata := map[string]string{
		"general.architecture": "gemma3", "gemma3.block_count": "32",
		"gemma3.embedding_length": "4096", "gemma3.vocab_size": "128256",
		"gemma3.attention.head_count": "32", "gemma3.attention.head_count_kv": "8",
		"gemma3.attention.key_length": "128", "gemma3.attention.value_length": "128",
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
	for _, tt := range []struct {
		name, backend            string
		context, slots           int64
		cacheType                int32
		cacheMiB, gpuMiB, cpuMiB int64
	}{
		{"RNJ one slot", "metal", 32768, 1, 1, 4096, 768, 256},
		{"two slots", "metal", 32768, 2, 1, 8192, 960, 256},
		{"larger context", "metal", 65536, 1, 1, 8192, 1024, 384},
		{"quantized cache", "metal", 32768, 1, 8, 2176, 768, 256},
		{"CUDA control", "cuda", 32768, 1, 1, 4096, 640, 256},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FromBytes(data.Bytes(), 8<<30, Config{ComputeBackend: tt.backend, ContextWindow: tt.context, Slots: tt.slots, TypeK: tt.cacheType, TypeV: tt.cacheType, BytesPerElement: 2, NUBatch: 2048})
			if err != nil {
				t.Fatal(err)
			}
			gpu, cpu := tt.gpuMiB*mib, tt.cpuMiB*mib
			gpu, cpu = gpu+gpu/10, cpu+cpu/10
			if result.Input.OutputRows != tt.slots {
				t.Fatalf("output rows: got %d, want %d", result.Input.OutputRows, tt.slots)
			}
			if result.SlotMemory != tt.cacheMiB*mib || result.ComputeBufferEst != gpu || result.ComputeBufferCPU != cpu {
				t.Fatalf("cache/GPU compute/CPU compute: got %d/%d/%d, want %d/%d/%d", result.SlotMemory, result.ComputeBufferEst, result.ComputeBufferCPU, tt.cacheMiB*mib, gpu, cpu)
			}
			if result.UnifiedFootprint() != 8<<30+tt.cacheMiB*mib+gpu+cpu {
				t.Fatal("unified footprint omitted cache or workspace")
			}
		})
	}
}

func TestFromBytesGLM4MoEWorkspace(t *testing.T) {
	const mib int64 = 1 << 20
	metadata := map[string]string{
		"general.architecture": "glm4moe", "glm4moe.block_count": "47",
		"glm4moe.embedding_length": "4096", "glm4moe.vocab_size": "151552",
		"glm4moe.attention.head_count": "96", "glm4moe.attention.head_count_kv": "8",
		"glm4moe.attention.key_length": "128", "glm4moe.attention.value_length": "128",
		"glm4moe.expert_count": "128", "glm4moe.expert_used_count": "8",
		"glm4moe.nextn_predict_layers": "1",
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
	for _, tt := range []struct {
		name                  string
		backend               string
		slots, gpuMiB, cpuMiB int64
	}{
		{"one slot", "metal", 1, 1792, 640},
		{"two slots", "metal", 2, 2368, 640},
		{"CUDA control", "cuda", 1, 1280, 640},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FromBytes(data.Bytes(), 68<<30, Config{ComputeBackend: tt.backend, ContextWindow: 131072, Slots: tt.slots, TypeK: 1, TypeV: 1, BytesPerElement: 2, NUBatch: 2048, EmbeddedMTPStateCopies: 4})
			if err != nil {
				t.Fatal(err)
			}
			// Only 46 trunk layers allocate KV. The appended NextN head
			// is not an enabled Kronk MTP architecture and adds no context.
			cache := tt.slots * 23 << 30
			gpu, cpu := tt.gpuMiB*mib, tt.cpuMiB*mib
			gpu, cpu = gpu+gpu/10, cpu+cpu/10
			if result.Input.ComputeContexts != 1 || result.Input.RecurrentStateCopies != 1 || result.Input.OutputRows != tt.slots {
				t.Fatalf("contexts/state copies/output rows: got %d/%d/%d, want 1/1/%d", result.Input.ComputeContexts, result.Input.RecurrentStateCopies, result.Input.OutputRows, tt.slots)
			}
			if result.SlotMemory != cache || result.ComputeBufferEst != gpu || result.ComputeBufferCPU != cpu {
				t.Fatalf("cache/GPU compute/CPU compute: got %d/%d/%d, want %d/%d/%d", result.SlotMemory, result.ComputeBufferEst, result.ComputeBufferCPU, cache, gpu, cpu)
			}
			if result.UnifiedFootprint() != 68<<30+cache+gpu+cpu {
				t.Fatal("unified footprint omitted host or GPU workspace")
			}
		})
	}
}

func TestFromBytesLFM2Workspace(t *testing.T) {
	const mib int64 = 1 << 20
	metadata := map[string]string{
		"general.architecture": "lfm2", "lfm2.block_count": "16",
		"lfm2.embedding_length": "1536", "lfm2.vocab_size": "65536",
		"lfm2.attention.head_count":    "24",
		"lfm2.attention.head_count_kv": "[0,0,8,0,0,8,0,0,8,0,8,0,8,0,8,0]",
		"lfm2.shortconv.l_cache":       "3",
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
	for _, tt := range []struct {
		name                     string
		context, slots, copies   int64
		cacheType                int32
		cacheMiB, gpuMiB, cpuMiB int64
	}{
		{"one slot", 32768, 1, 1, 1, 384, 352, 176},
		{"two slots", 32768, 2, 1, 1, 768, 440, 176},
		{"larger context", 65536, 1, 1, 1, 768, 544, 304},
		{"quantized cache with rollback", 32768, 1, 3, 8, 204, 352, 176},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FromBytes(data.Bytes(), 4<<30, Config{ComputeBackend: "metal", ContextWindow: tt.context, Slots: tt.slots, RecurrentStateCopies: tt.copies, TypeK: tt.cacheType, TypeV: tt.cacheType, BytesPerElement: 2, NUBatch: 2048})
			if err != nil {
				t.Fatal(err)
			}
			// Six attention layers; ten F32 two-tap history buffers of
			// 1536 values each. State is independent of context/cache type.
			cache := tt.cacheMiB*mib + 122880*tt.slots*tt.copies
			gpu, cpu := tt.gpuMiB*mib, tt.cpuMiB*mib
			gpu, cpu = gpu+gpu/10, cpu+cpu/10
			if result.SlotMemory != cache || result.ComputeBufferEst != gpu || result.ComputeBufferCPU != cpu {
				t.Fatalf("cache/GPU compute/CPU compute: got %d/%d/%d, want %d/%d/%d", result.SlotMemory, result.ComputeBufferEst, result.ComputeBufferCPU, cache, gpu, cpu)
			}
			if result.UnifiedFootprint() != 4<<30+cache+gpu+cpu {
				t.Fatal("unified footprint omitted cache or workspace")
			}
		})
	}
}

func TestFromBytesMistral3Workspace(t *testing.T) {
	const mib int64 = 1 << 20
	metadata := map[string]string{
		"general.architecture": "mistral3", "mistral3.block_count": "40",
		"mistral3.embedding_length": "5120", "mistral3.vocab_size": "196608",
		"mistral3.attention.head_count": "32", "mistral3.attention.head_count_kv": "8",
		"mistral3.attention.key_length": "128", "mistral3.attention.value_length": "128",
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
	for _, tt := range []struct {
		slots, gpuMiB int64
	}{
		{1, 1600},
		{2, 2192},
	} {
		result, err := FromBytes(data.Bytes(), 4<<30, Config{ComputeBackend: "metal", ContextWindow: 131072, Slots: tt.slots, TypeK: 1, TypeV: 1, BytesPerElement: 2, NUBatch: 2048})
		if err != nil {
			t.Fatal(err)
		}
		gpu, cpu := tt.gpuMiB*mib, 672*mib
		gpu, cpu = gpu+gpu/10, cpu+cpu/10
		cache := tt.slots * 20 << 30
		if result.Input.ComputeKind != modelprofile.ComputeMistral3 || result.Input.OutputRows != tt.slots {
			t.Fatalf("compute kind/output rows: got %d/%d, want %d/%d", result.Input.ComputeKind, result.Input.OutputRows, modelprofile.ComputeMistral3, tt.slots)
		}
		if result.SlotMemory != cache || result.ComputeBufferEst != gpu || result.ComputeBufferCPU != cpu {
			t.Fatalf("cache/GPU compute/CPU compute: got %d/%d/%d, want %d/%d/%d", result.SlotMemory, result.ComputeBufferEst, result.ComputeBufferCPU, cache, gpu, cpu)
		}
		if result.UnifiedFootprint() != 4<<30+cache+gpu+cpu {
			t.Fatal("unified footprint omitted host or GPU workspace")
		}
	}
}

func TestFromBytesGPTOSSWorkspaces(t *testing.T) {
	const mib int64 = 1 << 20
	metadata := map[string]string{
		"general.architecture": "gpt-oss", "gpt-oss.block_count": "24",
		"gpt-oss.embedding_length": "2880", "gpt-oss.vocab_size": "201088",
		"gpt-oss.attention.head_count": "64", "gpt-oss.attention.head_count_kv": "8",
		"gpt-oss.attention.key_length": "64", "gpt-oss.attention.value_length": "64",
		"gpt-oss.attention.sliding_window": "128", "gpt-oss.expert_count": "32",
		"gpt-oss.expert_used_count": "4",
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
	for _, tt := range []struct {
		name            string
		slots           int64
		full            bool
		cache, gpu, cpu int64
	}{
		{"full SWA", 1, true, 6 << 30, 1716 * mib, 1114 * mib},
		// Twelve full layers retain 3 GiB; twelve compact SWA layers
		// have PAD256(128+2048)=2304 cells, adding 54 MiB.
		{"compact SWA", 1, false, 3<<30 + 54*mib, 1213 * mib, 611 * mib},
		{"two slots", 2, true, 12 << 30, 2017 * mib, 1114 * mib},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FromBytes(data.Bytes(), 4<<30, Config{ComputeBackend: "metal", ContextWindow: 131072, Slots: tt.slots, TypeK: 1, TypeV: 1, BytesPerElement: 2, NUBatch: 2048, SWAFull: tt.full})
			if err != nil {
				t.Fatal(err)
			}
			gpu, cpu := tt.gpu+tt.gpu/10, tt.cpu+tt.cpu/10
			if result.Input.ComputeKind != modelprofile.ComputeGPTOSS || result.Input.OutputRows != tt.slots {
				t.Fatalf("compute kind/output rows: got %d/%d, want %d/%d", result.Input.ComputeKind, result.Input.OutputRows, modelprofile.ComputeGPTOSS, tt.slots)
			}
			if result.SlotMemory != tt.cache || result.ComputeBufferEst != gpu || result.ComputeBufferCPU != cpu {
				t.Fatalf("cache/GPU compute/CPU compute: got %d/%d/%d, want %d/%d/%d", result.SlotMemory, result.ComputeBufferEst, result.ComputeBufferCPU, tt.cache, gpu, cpu)
			}
			if result.UnifiedFootprint() != 4<<30+tt.cache+gpu+cpu {
				t.Fatal("unified footprint omitted host or GPU workspace")
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

func TestFromBytesClefMemory(t *testing.T) {
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
	write(uint64(8))
	writeString("general.architecture")
	write(gguf.MetadataValueTypeString)
	writeString("clef")
	for key, value := range map[string]uint32{
		"block_count": 32, "embedding_length": 4096,
		"attention.head_count": 16, "attention.head_count_kv": 4,
		"attention.key_length": 256, "attention.value_length": 256,
		"ssm.state_size": 128,
	} {
		writeString("clef." + key)
		write(gguf.MetadataValueTypeUInt32)
		write(value)
	}
	var first Result
	for _, context := range []int64{65536, 131072} {
		result, err := FromBytes(data.Bytes(), 6486448192, Config{
			ComputeBackend: "metal", ContextWindow: context, Slots: 1,
			NUBatch: 2048, BytesPerElement: 2, SWAFull: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.SlotMemory != 0 || result.KVPerSlot != 0 || result.KVVRAMBytes != 0 || result.KVCPUBytes != 0 {
			t.Fatalf("Clef persistent cache: got %+v, want zero", result)
		}
		// The observed graph reserves 474.32 MiB GPU and 136.09 MiB CPU.
		// Cover those buffers without reserving a context-sized cache/mask.
		if result.ComputeBufferEst < 475<<20 || result.ComputeBufferCPU < 137<<20 || result.UnifiedFootprint() > 7<<30 {
			t.Fatalf("Clef reservation does not cover its graph within 7 GiB: %+v", result)
		}
		if first.Input.ContextWindow != 0 && result.UnifiedFootprint() != first.UnifiedFootprint() {
			t.Fatal("Clef reservation grew with context despite batch-local attention")
		}
		first = result
	}
	for _, cfg := range []Config{
		{ComputeBackend: "metal", ContextWindow: 131072, Slots: 2, NUBatch: 2048, BytesPerElement: 2},
		{ComputeBackend: "metal", ContextWindow: 131072, Slots: 1, NUBatch: 4096, BytesPerElement: 2},
		{ComputeBackend: "metal", ContextWindow: 131072, Slots: 1, NUBatch: 2048, BytesPerElement: 2, VTransposed: true},
	} {
		result, err := FromBytes(data.Bytes(), 6486448192, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if result.SlotMemory != 0 || result.ComputeBufferEst <= first.ComputeBufferEst {
			t.Fatalf("Clef larger batch/slot/non-flash workspace: got %+v, want larger compute without cache", result)
		}
	}
}

func TestFromBytesModernBERTMemory(t *testing.T) {
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
	write(uint64(9))
	writeString("general.architecture")
	write(gguf.MetadataValueTypeString)
	writeString("modern-bert")
	for key, value := range map[string]uint32{
		"block_count": 24, "embedding_length": 384,
		"attention.head_count": 6, "attention.key_length": 64,
		"attention.value_length": 64, "attention.sliding_window": 128,
		"attention.sliding_window_pattern": 3, "vocab_size": 256000,
	} {
		writeString("modern-bert." + key)
		write(gguf.MetadataValueTypeUInt32)
		write(value)
	}
	var first Result
	for _, context := range []int64{8192, 16384} {
		result, err := FromBytes(data.Bytes(), 168166496, Config{
			ComputeBackend: "metal", ContextWindow: context, Slots: 1,
			NUBatch: 2048, BytesPerElement: 2, SWAFull: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.SlotMemory != 0 || result.KVPerSlot != 0 || result.KVVRAMBytes != 0 || result.KVCPUBytes != 0 {
			t.Fatalf("ModernBERT persistent cache: got %+v, want zero", result)
		}
		// Julia's native graph reserves 64.83 MiB GPU and 28.04 MiB CPU,
		// plus 0.98 MiB output. Leave room without charging generation logits.
		if result.ComputeBufferEst < 65<<20 || result.ComputeBufferCPU < 29<<20 || result.UnifiedFootprint() > 300<<20 {
			t.Fatalf("ModernBERT reservation does not cover Julia within 300 MiB: %+v", result)
		}
		if first.Input.ContextWindow != 0 && result.UnifiedFootprint() != first.UnifiedFootprint() {
			t.Fatal("ModernBERT reservation grew with context despite batch-local attention")
		}
		first = result
	}
	for _, cfg := range []Config{
		{ComputeBackend: "metal", ContextWindow: 8192, Slots: 2, NUBatch: 2048, BytesPerElement: 2},
		{ComputeBackend: "metal", ContextWindow: 8192, Slots: 1, NUBatch: 4096, BytesPerElement: 2},
		{ComputeBackend: "metal", ContextWindow: 8192, Slots: 1, NUBatch: 2048, BytesPerElement: 2, VTransposed: true},
	} {
		result, err := FromBytes(data.Bytes(), 168166496, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if result.SlotMemory != 0 || result.ComputeBufferEst <= first.ComputeBufferEst {
			t.Fatalf("ModernBERT larger batch/slot/non-flash workspace: got %+v, want larger compute without cache", result)
		}
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

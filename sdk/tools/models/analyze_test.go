package models

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

func TestAnalyzeModelDense(t *testing.T) {

	// Simulate a Qwen3-8B-Q8_0 model.
	info := ModelInfo{
		ID:   "Qwen3-8B-Q8_0",
		Desc: "Qwen3 8B Instruct",
		Size: 8_700_000_000, // ~8.7 GiB
		Metadata: map[string]string{
			"general.architecture":          "qwen3",
			"general.name":                  "Qwen3 8B Instruct",
			"general.file_type":             "7",
			"qwen3.block_count":             "36",
			"qwen3.context_length":          "40960",
			"qwen3.embedding_length":        "4096",
			"qwen3.feed_forward_length":     "12288",
			"qwen3.attention.head_count":    "32",
			"qwen3.attention.head_count_kv": "8",
			"qwen3.attention.key_length":    "128",
			"qwen3.attention.value_length":  "128",
			"qwen3.rope.freq_base":          "1e+06",
			"qwen3.rope.dimension_count":    "128",
			"tokenizer.ggml.model":          "gpt2",
			"tokenizer.ggml.pre":            "qwen2",
		},
	}

	// Simulate an Apple M5 Max with ~110 GiB free.
	devs := devices.Devices{
		Devices: []devices.DeviceInfo{
			{
				Index:      0,
				Name:       "Apple M5 Max",
				Type:       "gpu_metal",
				FreeBytes:  115_000_000_000,
				TotalBytes: 128_000_000_000,
			},
		},
		GPUCount:           1,
		GPUTotalBytes:      128_000_000_000,
		SupportsGPUOffload: true,
		SystemRAMBytes:     128_000_000_000,
	}

	a, err := analyzeModel(info, devs)
	if err != nil {
		t.Fatalf("analyzeModel failed: %v", err)
	}

	// Model facts.
	if a.Model.Architecture != "qwen3" {
		t.Errorf("Architecture = %q, want %q", a.Model.Architecture, "qwen3")
	}

	if a.Model.Class != "dense" {
		t.Errorf("Class = %q, want %q", a.Model.Class, "dense")
	}

	if a.Model.Quantization != "Q8_0" {
		t.Errorf("Quantization = %q, want %q", a.Model.Quantization, "Q8_0")
	}

	if a.Model.BlockCount != 36 {
		t.Errorf("BlockCount = %d, want %d", a.Model.BlockCount, 36)
	}

	if a.Model.TrainingContext != 40960 {
		t.Errorf("TrainingContext = %d, want %d", a.Model.TrainingContext, 40960)
	}

	if a.Model.HeadCountKV != 8 {
		t.Errorf("HeadCountKV = %d, want %d", a.Model.HeadCountKV, 8)
	}

	// System facts.
	if a.System.GPUName != "Apple M5 Max" {
		t.Errorf("GPUName = %q, want %q", a.System.GPUName, "Apple M5 Max")
	}

	if !a.System.SupportsGPUOffload {
		t.Error("SupportsGPUOffload should be true")
	}

	// Memory.
	if a.Memory.KVBytesPerTokenF16 <= 0 {
		t.Errorf("KVBytesPerTokenF16 = %d, want > 0", a.Memory.KVBytesPerTokenF16)
	}

	if !a.Memory.FullGPUFit {
		t.Error("FullGPUFit should be true for 8B model on 110 GiB GPU")
	}

	// Recommended profile.
	if a.Recommended.Name != "balanced" {
		t.Errorf("Recommended.Name = %q, want %q", a.Recommended.Name, "balanced")
	}

	if a.Recommended.NSeqMax != 1 {
		t.Errorf("Recommended.NSeqMax = %d, want %d", a.Recommended.NSeqMax, 1)
	}

	if a.Recommended.FlashAttention != "auto" {
		t.Errorf("Recommended.FlashAttention = %q, want %q", a.Recommended.FlashAttention, "auto")
	}
	if a.Recommended.SplitMode != "layer" {
		t.Errorf("Recommended.SplitMode = %q, want %q", a.Recommended.SplitMode, "layer")
	}
	if a.Recommended.NGPULayers != 0 {
		t.Errorf("Recommended.NGPULayers = %d, want 0", a.Recommended.NGPULayers)
	}

	if a.Recommended.ContextWindow < vram.ContextWindow8K {
		t.Errorf("Recommended.ContextWindow = %d, want >= %d", a.Recommended.ContextWindow, vram.ContextWindow8K)
	}

	if !a.Recommended.Fits {
		t.Error("Recommended should fit on 110 GiB GPU")
	}

	// Profiles should exist.
	if len(a.Profiles) != 3 {
		t.Fatalf("len(Profiles) = %d, want 3", len(a.Profiles))
	}

	// max_context profile should have 1 slot.
	maxCtx := a.Profiles[1]
	if maxCtx.Name != "max_context" {
		t.Errorf("Profiles[1].Name = %q, want %q", maxCtx.Name, "max_context")
	}
	if maxCtx.NSeqMax != 1 {
		t.Errorf("max_context.NSeqMax = %d, want 1", maxCtx.NSeqMax)
	}

	// max_concurrency profile should have more slots.
	maxConc := a.Profiles[2]
	if maxConc.Name != "max_concurrency" {
		t.Errorf("Profiles[2].Name = %q, want %q", maxConc.Name, "max_concurrency")
	}
	if maxConc.NSeqMax < 2 {
		t.Errorf("max_concurrency.NSeqMax = %d, want >= 2", maxConc.NSeqMax)
	}

	// At the same recommendation, a known CPU input table must be removed
	// from GPU weights rather than discarded by the analysis input builder.
	info.Weights = &gguf.WeightBreakdown{
		AlwaysActiveBytes: int64(info.Size), InputBytes: 400_000_000,
		TokenEmbeddingBytes: 400_000_000, OutputBytes: 600_000_000, HasOutputWeight: true,
	}
	withWeights, err := Analyze(info, devs)
	if err != nil {
		t.Fatal(err)
	}
	if withWeights.Recommended.ContextWindow != a.Recommended.ContextWindow || withWeights.Recommended.NSeqMax != a.Recommended.NSeqMax {
		t.Fatal("ample-memory weight correction unexpectedly changed context/concurrency")
	}
	if delta := a.Recommended.EstimatedVRAMBytes - withWeights.Recommended.EstimatedVRAMBytes; delta != 400_000_000 {
		t.Errorf("CPU input-table VRAM reduction: got %d, want 400000000", delta)
	}
}

func TestBuildProfileUsesLayerSplitDefault(t *testing.T) {
	rec := buildProfile("balanced", profileInput{gpuCount: 2}, 0, 0)
	if rec.SplitMode != "layer" {
		t.Errorf("SplitMode = %q, want %q", rec.SplitMode, "layer")
	}
}

func TestAnalyzeIndexedWorkspaceMatchesCalculator(t *testing.T) {
	for _, indexed := range []bool{false, true} {
		name := "non-indexed control"
		if indexed {
			name = "indexed"
		}
		t.Run(name, func(t *testing.T) {
			info := ModelInfo{
				ID: "Qwen4Exp", Size: 80 << 30,
				Metadata: map[string]string{
					"general.architecture":             "qwen4exp",
					"qwen4exp.block_count":             "48",
					"qwen4exp.context_length":          "131072",
					"qwen4exp.embedding_length":        "2560",
					"qwen4exp.attention.head_count":    "24",
					"qwen4exp.attention.head_count_kv": "2",
					"qwen4exp.attention.key_length":    "256",
					"qwen4exp.attention.value_length":  "256",
					"qwen4exp.ssm.conv_kernel":         "4",
					"qwen4exp.ssm.inner_size":          "6144",
					"qwen4exp.ssm.state_size":          "128",
					"qwen4exp.ssm.group_count":         "16",
					"qwen4exp.hyper_connection.count":  "4",
				},
			}
			input := vram.Input{
				ModelSizeBytes: 80 << 30, BlockCount: 48, EmbeddingLength: 2560,
				HeadCount: 24, HeadCountKV: 2, KeyLength: 256, ValueLength: 256,
				OutputEmbeddingLength: 10240, TypeK: 1, TypeV: 1, BytesPerElement: 2,
				RecurrentStateBytes: 4 * (3*(6144+2*16*128) + 128*6144),
				RecurrentPattern:    make([]bool, 48),
			}
			for layer := range input.RecurrentPattern {
				input.RecurrentPattern[layer] = (layer+1)%4 != 0
			}
			if indexed {
				info.Metadata["qwen4exp.attention.indexer.key_length"] = "128"
				info.Metadata["qwen4exp.attention.indexer.top_k"] = "2048"
				info.Metadata["qwen4exp.attention.compress_ratios"] = "[0 0 0 4]"
				input.ComputeKind = modelprofile.ComputeIndexed
				input.IndexerKeyLength = 256 // One raw and one pooled 128-value key.
				input.IndexerCompression = 4
				input.IndexerTopK = 2048
			}
			devs := devices.Devices{
				GPUCount: 1, GPUTotalBytes: 256 << 30, SystemRAMBytes: 256 << 30,
				SupportsGPUOffload: true,
				Devices:            []devices.DeviceInfo{{Name: "GPU", Type: "gpu_cuda", FreeBytes: 240 << 30, TotalBytes: 256 << 30}},
			}
			analysis, err := Analyze(info, devs)
			if err != nil {
				t.Fatal(err)
			}
			for _, rec := range analysis.Profiles {
				if !rec.Fits || rec.CacheTypeK != "f16" || rec.CacheTypeV != "f16" {
					t.Fatalf("%s recommendation: got %+v, want fitting F16 cache", rec.Name, rec)
				}
				input.ContextWindow = rec.ContextWindow
				input.Slots = rec.NSeqMax
				cfg := model.Config{PtrNSeqMax: new(int(rec.NSeqMax))}
				_, physical := cfg.GenerationBatchSizes(1, false)
				input.NUBatch = int64(physical)
				want := vram.Calculate(input)
				if rec.EstimatedVRAMBytes != want.TotalVRAM {
					t.Errorf("%s VRAM: got %d, want direct calculator %d", rec.Name, rec.EstimatedVRAMBytes, want.TotalVRAM)
				}
			}
		})
	}
}

func TestAnalyzeKimiK31MatchesCalculator(t *testing.T) {
	info := ModelInfo{
		ID: "Kimi-K3.1-Pro", Size: 1 << 30,
		Metadata: map[string]string{
			"general.architecture": "kimi-k31",
			"kimi-k31.block_count": "5", "kimi-k31.embedding_length": "256", "kimi-k31.context_length": "8192",
			"kimi-k31.attention.head_count": "8", "kimi-k31.attention.head_count_kv": "[0 1 0 1 1]",
			"kimi-k31.attention.key_length": "64", "kimi-k31.attention.kv_lora_rank": "32",
			"kimi-k31.ssm.conv_kernel": "3", "kimi-k31.kda.head_dim": "16",
			"kimi-k31.attention.indexer.key_length": "32", "kimi-k31.attention.indexer.head_count": "4",
			"kimi-k31.attention.q_lora_rank": "64", "kimi-k31.attention.indexer.top_k": "64",
			"kimi-k31.attention.indexer.types": "[true true false false true]",
		},
	}
	analysis, err := Analyze(info, devices.Devices{
		GPUCount: 1, GPUTotalBytes: 64 << 30, SystemRAMBytes: 64 << 30, SupportsGPUOffload: true,
		Devices: []devices.DeviceInfo{{Name: "GPU", Type: "gpu_cuda", FreeBytes: 60 << 30, TotalBytes: 64 << 30}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Warnings) == 0 || !contains(analysis.Warnings[0], "compute workspace is uncalibrated") {
		t.Errorf("warnings: got %v, want uncalibrated workspace warning", analysis.Warnings)
	}
	for _, rec := range analysis.Profiles {
		cfg := model.Config{PtrNSeqMax: new(int(rec.NSeqMax))}
		_, physical := cfg.GenerationBatchSizes(1, false)
		want := vram.Calculate(vram.Input{
			Architecture: "kimi-k31", ModelSizeBytes: 1 << 30, BlockCount: 5, EmbeddingLength: 256,
			ComputeKind: modelprofile.ComputeKimiK3,
			HeadCount:   8, HeadCountKV: 1, KeyLength: 64, ValueLength: 32,
			HeadCountKVByLayer: []int64{0, 1, 0, 1, 1}, RecurrentPattern: []bool{true, false, true, false, false},
			RecurrentStateBytes: 11264, IndexerKeyLength: 32, IndexerHeads: 4, IndexerTopK: 64, QueryRank: 64,
			IndexerSources: []int64{-1, 1, -1, 1, 4}, ContextWindow: rec.ContextWindow, Slots: rec.NSeqMax,
			NUBatch: int64(physical), TypeK: 1, TypeV: 1, BytesPerElement: 2,
		})
		if !rec.Fits || rec.CacheTypeK != "f16" || rec.EstimatedVRAMBytes != want.TotalVRAM {
			t.Errorf("%s: got %+v, want fitting F16 reservation %d", rec.Name, rec, want.TotalVRAM)
		}
	}
	rocm, err := Analyze(info, devices.Devices{
		GPUCount: 2, SystemRAMBytes: 64 << 30, SupportsGPUOffload: true,
		Devices: []devices.DeviceInfo{
			{Name: "ROCm0", Type: "gpu_rocm", FreeBytes: 32 << 30},
			{Name: "ROCm1", Type: "gpu_rocm", FreeBytes: 32 << 30},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range rocm.Profiles {
		if rec.SplitMode == "tensor" {
			t.Errorf("%s selected unsupported tensor parallelism", rec.Name)
		}
	}
}

func TestAnalyzeModelMoE(t *testing.T) {

	// Simulate a Qwen3-30B-A3B MoE model.
	info := ModelInfo{
		ID:   "Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL",
		Desc: "Qwen3 Coder 30B A3B Instruct",
		Size: 16_500_000_000,
		Metadata: map[string]string{
			"general.architecture":             "qwen3moe",
			"general.name":                     "Qwen3 Coder 30B A3B Instruct",
			"general.file_type":                "7",
			"qwen3moe.block_count":             "48",
			"qwen3moe.context_length":          "131072",
			"qwen3moe.embedding_length":        "2048",
			"qwen3moe.feed_forward_length":     "8192",
			"qwen3moe.attention.head_count":    "16",
			"qwen3moe.attention.head_count_kv": "4",
			"qwen3moe.attention.key_length":    "128",
			"qwen3moe.attention.value_length":  "128",
			"qwen3moe.expert_count":            "128",
			"qwen3moe.expert_used_count":       "8",
			"qwen3moe.rope.freq_base":          "1e+06",
		},
	}

	devs := devices.Devices{
		Devices: []devices.DeviceInfo{
			{
				Index:      0,
				Name:       "Apple M5 Max",
				Type:       "gpu_metal",
				FreeBytes:  110_000_000_000,
				TotalBytes: 128_000_000_000,
			},
		},
		GPUCount:           1,
		GPUTotalBytes:      128_000_000_000,
		SupportsGPUOffload: true,
		SystemRAMBytes:     128_000_000_000,
	}

	a, err := analyzeModel(info, devs)
	if err != nil {
		t.Fatalf("analyzeModel failed: %v", err)
	}

	if a.Model.Class != "moe" {
		t.Errorf("Class = %q, want %q", a.Model.Class, "moe")
	}

	if a.Model.MoE == nil {
		t.Fatal("MoE should not be nil")
	}

	if a.Model.MoE.ExpertCount != 128 {
		t.Errorf("ExpertCount = %d, want 128", a.Model.MoE.ExpertCount)
	}

	if a.Model.MoE.ExpertUsedCount != 8 {
		t.Errorf("ExpertUsedCount = %d, want 8", a.Model.MoE.ExpertUsedCount)
	}
}

func TestAnalyzeModelSlidingWindow(t *testing.T) {

	// Simulate a Gemma4 model with SWA.
	info := ModelInfo{
		ID:   "gemma-4-31B-it-UD-Q8_K_XL",
		Desc: "Gemma-4-31B-It",
		Size: 33_000_000_000,
		Metadata: map[string]string{
			"general.architecture":                    "gemma4",
			"general.name":                            "Gemma-4-31B-It",
			"general.file_type":                       "7",
			"gemma4.block_count":                      "60",
			"gemma4.context_length":                   "262144",
			"gemma4.embedding_length":                 "5376",
			"gemma4.feed_forward_length":              "21504",
			"gemma4.attention.head_count":             "32",
			"gemma4.attention.head_count_kv":          "[16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4]",
			"gemma4.attention.key_length":             "512",
			"gemma4.attention.value_length":           "512",
			"gemma4.attention.sliding_window":         "1024",
			"gemma4.attention.sliding_window_pattern": "[true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false]",
			"gemma4.rope.freq_base":                   "1e+06",
			"gemma4.rope.dimension_count":             "512",
			"gemma4.final_logit_softcapping":          "30",
		},
	}

	devs := devices.Devices{
		Devices: []devices.DeviceInfo{
			{
				Index:      0,
				Name:       "Apple M5 Max",
				Type:       "gpu_metal",
				FreeBytes:  110_000_000_000,
				TotalBytes: 128_000_000_000,
			},
		},
		GPUCount:           1,
		GPUTotalBytes:      128_000_000_000,
		SupportsGPUOffload: true,
		SystemRAMBytes:     128_000_000_000,
	}

	// Fix batch capacity so this backend-workspace comparison does not also
	// compare different AutoTune batch/precision tradeoffs.
	constraints := ModelConfig{PtrPhysicalBatchSize: new(2048)}
	a, err := AnalyzeWithConfig(info, devs, constraints)
	if err != nil {
		t.Fatalf("analyzeModel failed: %v", err)
	}

	if a.Model.Attention.SlidingWindow != 1024 {
		t.Errorf("SlidingWindow = %d, want 1024", a.Model.Attention.SlidingWindow)
	}

	// 50 SWA layers, 10 full attention layers.
	if a.Model.Attention.SlidingWindowLayers != 50 {
		t.Errorf("SlidingWindowLayers = %d, want 50", a.Model.Attention.SlidingWindowLayers)
	}

	if a.Model.Attention.FullAttentionLayers != 10 {
		t.Errorf("FullAttentionLayers = %d, want 10", a.Model.Attention.FullAttentionLayers)
	}

	if a.Model.Attention.LogitSoftcapping != 30 {
		t.Errorf("LogitSoftcapping = %v, want 30", a.Model.Attention.LogitSoftcapping)
	}

	// With the same budget both backends select the same context/slots, but
	// Metal must also reserve F16 K/V scratch for this Gemma attention graph.
	devs.Devices[0].Type = "gpu_cuda"
	cuda, err := AnalyzeWithConfig(info, devs, constraints)
	if err != nil {
		t.Fatal(err)
	}
	if a.Recommended.ContextWindow != cuda.Recommended.ContextWindow || a.Recommended.NSeqMax != cuda.Recommended.NSeqMax {
		t.Fatal("backend control selected different context/slots")
	}
	if a.Recommended.EstimatedVRAMBytes-cuda.Recommended.EstimatedVRAMBytes < 512<<20 {
		t.Errorf("Metal/CUDA estimates: got %d/%d, want Metal at least 512 MiB higher", a.Recommended.EstimatedVRAMBytes, cuda.Recommended.EstimatedVRAMBytes)
	}
}

func TestAnalyzeModelNoGPU(t *testing.T) {

	info := ModelInfo{
		ID:   "Qwen3-8B-Q8_0",
		Desc: "Qwen3 8B Instruct",
		Size: 8_700_000_000,
		Metadata: map[string]string{
			"general.architecture":          "qwen3",
			"general.file_type":             "7",
			"qwen3.block_count":             "36",
			"qwen3.context_length":          "40960",
			"qwen3.embedding_length":        "4096",
			"qwen3.attention.head_count":    "32",
			"qwen3.attention.head_count_kv": "8",
			"qwen3.attention.key_length":    "128",
			"qwen3.attention.value_length":  "128",
		},
	}

	devs := devices.Devices{
		Devices:            []devices.DeviceInfo{},
		SupportsGPUOffload: false,
		SystemRAMBytes:     32_000_000_000,
	}

	a, err := analyzeModel(info, devs)
	if err != nil {
		t.Fatalf("analyzeModel failed: %v", err)
	}

	if a.Recommended.FlashAttention != "disabled" {
		t.Errorf("FlashAttention = %q, want %q", a.Recommended.FlashAttention, "disabled")
	}

	if a.Recommended.NGPULayers != -1 {
		t.Errorf("NGPULayers = %d, want -1 (CPU only)", a.Recommended.NGPULayers)
	}

	found := false
	for _, w := range a.Warnings {
		if contains(w, "No GPU") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected a no-GPU warning")
	}
}

func TestAnalyzeModelTightMemory(t *testing.T) {

	// A large model on a small GPU — should fall back to q8_0 cache.
	info := ModelInfo{
		ID:   "BigModel-70B-Q8_0",
		Desc: "Big Model 70B",
		Size: 70_000_000_000,
		Metadata: map[string]string{
			"general.architecture":          "llama",
			"general.file_type":             "7",
			"llama.block_count":             "80",
			"llama.context_length":          "131072",
			"llama.embedding_length":        "8192",
			"llama.attention.head_count":    "64",
			"llama.attention.head_count_kv": "8",
			"llama.attention.key_length":    "128",
			"llama.attention.value_length":  "128",
		},
	}

	// Only 80 GiB free — tight for a 70B model with KV cache.
	devs := devices.Devices{
		Devices: []devices.DeviceInfo{
			{
				Index:      0,
				Name:       "Apple M4 Max",
				Type:       "gpu_metal",
				FreeBytes:  80_000_000_000,
				TotalBytes: 96_000_000_000,
			},
		},
		GPUCount:           1,
		GPUTotalBytes:      96_000_000_000,
		SupportsGPUOffload: true,
		SystemRAMBytes:     96_000_000_000,
	}

	a, err := analyzeModel(info, devs)
	if err != nil {
		t.Fatalf("analyzeModel failed: %v", err)
	}

	// With tight memory the recommendation should still produce a valid config.
	if a.Recommended.ContextWindow < vram.ContextWindow4K {
		t.Errorf("ContextWindow = %d, want >= %d", a.Recommended.ContextWindow, vram.ContextWindow4K)
	}

	if a.Recommended.EstimatedVRAMBytes <= 0 {
		t.Error("EstimatedVRAMBytes should be > 0")
	}
}

func TestBuildProfileContextAndCachePriority(t *testing.T) {
	tests := []struct {
		name        string
		gpuBudget   int64
		wantContext int64
		wantCache   string
	}{
		{"f16 at target context", 3_000_000_000, vram.ContextWindow64K, "f16"},
		{"q8 preserves target context", 2_200_000_000, vram.ContextWindow64K, "q8_0"},
		{"q8 before reducing context again", 1_800_000_000, vram.ContextWindow32K, "q8_0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := profileInput{
				modelSize:   1_000_000_000,
				blockCount:  10,
				headCountKV: 4,
				keyLength:   128,
				valueLength: 128,
				trainingCtx: vram.ContextWindow64K,
				class:       "dense",
				gpuBudget:   tt.gpuBudget,
				hasGPU:      true,
				gpuCount:    1,
			}

			rec := buildProfile("balanced", p, 0, 0)

			if rec.ContextWindow != tt.wantContext {
				t.Errorf("ContextWindow: got %d, want %d", rec.ContextWindow, tt.wantContext)
			}
			if rec.CacheTypeK != tt.wantCache || rec.CacheTypeV != tt.wantCache {
				t.Errorf("cache types: got %s/%s, want %s/%s", rec.CacheTypeK, rec.CacheTypeV, tt.wantCache, tt.wantCache)
			}
			if !rec.Fits {
				t.Error("Fits: got false, want true")
			}
		})
	}

}

func TestBuildProfileZeroGPUBudgetUsesFallback(t *testing.T) {
	p := profileInput{
		modelSize:   1_000_000_000,
		blockCount:  10,
		headCountKV: 4,
		keyLength:   128,
		valueLength: 128,
		trainingCtx: vram.ContextWindow128K,
		class:       "dense",
		hasGPU:      true,
		gpuCount:    1,
	}

	rec := buildProfile("balanced", p, 0, 0)

	if rec.ContextWindow != vram.ContextWindow4K {
		t.Errorf("ContextWindow: got %d, want %d", rec.ContextWindow, vram.ContextWindow4K)
	}
	if rec.CacheTypeK != "q8_0" || rec.CacheTypeV != "q8_0" {
		t.Errorf("cache types: got %s/%s, want q8_0/q8_0", rec.CacheTypeK, rec.CacheTypeV)
	}
	if rec.Fits {
		t.Error("Fits: got true, want false")
	}
}

func TestAutoTuneBudgetIsIndependentOfLiveFreeVRAM(t *testing.T) {
	info := ModelInfo{
		ID:   "Qwen3-1B-Q8_0",
		Size: 1_000_000_000,
		Metadata: map[string]string{
			"general.architecture":          "qwen3",
			"qwen3.block_count":             "10",
			"qwen3.context_length":          "131072",
			"qwen3.embedding_length":        "1024",
			"qwen3.attention.head_count":    "8",
			"qwen3.attention.head_count_kv": "4",
			"qwen3.attention.key_length":    "128",
			"qwen3.attention.value_length":  "128",
		},
	}
	budget := AutoTuneBudget{
		GPUBytes:       3_000_000_000,
		SystemRAMBytes: 16_000_000_000,
	}
	device := devices.DeviceInfo{
		Name:       "CUDA0",
		Type:       "gpu_cuda",
		TotalBytes: 8_000_000_000,
	}
	devs := devices.Devices{
		Devices: []devices.DeviceInfo{
			device,
			{Name: "CUDA1", Type: "gpu_cuda", FreeBytes: 7_000_000_000, TotalBytes: 12_000_000_000},
		},
		GPUCount:           2,
		SupportsGPUOffload: true,
		SystemRAMBytes:     16_000_000_000,
	}

	cuda1Primary, err := autoTuneWithConfigAndBudget(info, devs, ModelConfig{}, &budget)
	if err != nil {
		t.Fatalf("CUDA1-primary AutoTune: %v", err)
	}
	devs.Devices[0].FreeBytes = 7_000_000_000
	devs.Devices[1].FreeBytes = 0
	cuda0Primary, err := autoTuneWithConfigAndBudget(info, devs, ModelConfig{}, &budget)
	if err != nil {
		t.Fatalf("CUDA0-primary AutoTune: %v", err)
	}

	if cuda1Primary.PtrContextWindow == nil || cuda0Primary.PtrContextWindow == nil {
		t.Fatalf("PtrContextWindow: CUDA1-primary=%v CUDA0-primary=%v, want non-nil", cuda1Primary.PtrContextWindow, cuda0Primary.PtrContextWindow)
	}
	if *cuda1Primary.PtrContextWindow != *cuda0Primary.PtrContextWindow {
		t.Errorf("ContextWindow: CUDA1-primary=%d CUDA0-primary=%d, want equal", *cuda1Primary.PtrContextWindow, *cuda0Primary.PtrContextWindow)
	}
	if cuda1Primary.CacheTypeK != cuda0Primary.CacheTypeK || cuda1Primary.CacheTypeV != cuda0Primary.CacheTypeV {
		t.Errorf("cache types: CUDA1-primary=%s/%s CUDA0-primary=%s/%s, want equal", cuda1Primary.CacheTypeK, cuda1Primary.CacheTypeV, cuda0Primary.CacheTypeK, cuda0Primary.CacheTypeV)
	}
}

func TestBuildProfileExplicitCPUUsesRAMBudget(t *testing.T) {
	cpuOnly := -1
	p := profileInput{
		modelSize:   1_000_000_000,
		blockCount:  10,
		headCountKV: 4,
		keyLength:   128,
		valueLength: 128,
		trainingCtx: vram.ContextWindow64K,
		class:       "dense",
		ramBudget:   4_000_000_000,
		hasGPU:      true,
		gpuCount:    1,
		nGpuLayers:  &cpuOnly,
	}

	rec := buildProfile("balanced", p, 0, 0)

	if !rec.Fits {
		t.Error("Fits: got false, want true")
	}
}

func TestBuildProfileExplicitConstraints(t *testing.T) {
	context64K := int(vram.ContextWindow64K)
	context256K := int(vram.ContextWindow256K)
	twoSlots := 2

	tests := []struct {
		name           string
		gpuBudget      int64
		contextWindow  *int
		nSeqMax        *int
		cacheTypeK     model.GGMLType
		cacheTypeV     model.GGMLType
		wantContext    int64
		wantNSeqMax    int64
		wantCacheTypeK string
		wantCacheTypeV string
		wantFits       bool
	}{
		{
			name:           "preserve explicit context with q8",
			gpuBudget:      2_200_000_000,
			contextWindow:  &context64K,
			wantContext:    vram.ContextWindow64K,
			wantNSeqMax:    1,
			wantCacheTypeK: "q8_0",
			wantCacheTypeV: "q8_0",
			wantFits:       true,
		},
		{
			name:           "explicit context can exceed profile cap",
			gpuBudget:      5_000_000_000,
			contextWindow:  &context256K,
			wantContext:    vram.ContextWindow256K,
			wantNSeqMax:    1,
			wantCacheTypeK: "q8_0",
			wantCacheTypeV: "q8_0",
			wantFits:       true,
		},
		{
			name:           "preserve explicit f16 when it does not fit",
			gpuBudget:      2_200_000_000,
			contextWindow:  &context64K,
			cacheTypeK:     model.GGMLTypeF16,
			cacheTypeV:     model.GGMLTypeF16,
			wantContext:    vram.ContextWindow64K,
			wantNSeqMax:    1,
			wantCacheTypeK: "f16",
			wantCacheTypeV: "f16",
			wantFits:       false,
		},
		{
			name:           "q8 is the quantization floor",
			gpuBudget:      1_200_000_000,
			contextWindow:  &context64K,
			wantContext:    vram.ContextWindow64K,
			wantNSeqMax:    1,
			wantCacheTypeK: "q8_0",
			wantCacheTypeV: "q8_0",
			wantFits:       false,
		},
		{
			name:           "size explicit concurrency",
			gpuBudget:      3_000_000_000,
			contextWindow:  &context64K,
			nSeqMax:        &twoSlots,
			wantContext:    vram.ContextWindow64K,
			wantNSeqMax:    2,
			wantCacheTypeK: "q8_0",
			wantCacheTypeV: "q8_0",
			wantFits:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := profileInput{
				modelSize:     1_000_000_000,
				blockCount:    10,
				headCountKV:   4,
				keyLength:     128,
				valueLength:   128,
				embLen:        1024,
				trainingCtx:   vram.ContextWindow256K,
				class:         "dense",
				gpuBudget:     tt.gpuBudget,
				hasGPU:        true,
				gpuCount:      1,
				contextWindow: tt.contextWindow,
				nSeqMax:       tt.nSeqMax,
				cacheTypeK:    tt.cacheTypeK,
				cacheTypeV:    tt.cacheTypeV,
			}

			rec := buildProfile("balanced", p, 0, 0)
			if rec.ContextWindow != tt.wantContext {
				t.Errorf("ContextWindow: got %d, want %d", rec.ContextWindow, tt.wantContext)
			}
			if rec.NSeqMax != tt.wantNSeqMax {
				t.Errorf("NSeqMax: got %d, want %d", rec.NSeqMax, tt.wantNSeqMax)
			}
			if rec.CacheTypeK != tt.wantCacheTypeK {
				t.Errorf("CacheTypeK: got %q, want %q", rec.CacheTypeK, tt.wantCacheTypeK)
			}
			if rec.CacheTypeV != tt.wantCacheTypeV {
				t.Errorf("CacheTypeV: got %q, want %q", rec.CacheTypeV, tt.wantCacheTypeV)
			}
			if rec.Fits != tt.wantFits {
				t.Errorf("Fits: got %t, want %t", rec.Fits, tt.wantFits)
			}
		})
	}

	p := profileInput{
		modelSize:     1_000_000_000,
		blockCount:    10,
		headCountKV:   4,
		keyLength:     128,
		valueLength:   128,
		embLen:        1024,
		trainingCtx:   vram.ContextWindow64K,
		class:         "dense",
		gpuBudget:     10_000_000_000,
		hasGPU:        true,
		gpuCount:      1,
		contextWindow: &context64K,
		nSeqMax:       &twoSlots,
	}
	rec := buildProfile("max_concurrency", p, 0, 1)
	if rec.NSeqMax != int64(twoSlots) {
		t.Errorf("explicit max-concurrency NSeqMax: got %d, want %d", rec.NSeqMax, twoSlots)
	}
}

func TestAutoTuneWithConfigPreservesExplicitSizing(t *testing.T) {
	info := ModelInfo{
		ID:   "mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL",
		Desc: "Qwen3.6 35B A3B",
		Size: 40_000_000_000,
		Metadata: map[string]string{
			"general.architecture":              "qwen35moe",
			"general.file_type":                 "7",
			"qwen35moe.block_count":             "41",
			"qwen35moe.context_length":          "262144",
			"qwen35moe.embedding_length":        "2048",
			"qwen35moe.attention.head_count":    "16",
			"qwen35moe.attention.head_count_kv": "2",
			"qwen35moe.attention.key_length":    "256",
			"qwen35moe.attention.value_length":  "256",
		},
	}
	devs := devices.Devices{
		Devices: []devices.DeviceInfo{
			{
				Index:      0,
				Name:       "Apple M5 Max",
				Type:       "gpu_metal",
				FreeBytes:  115_000_000_000,
				TotalBytes: 128_000_000_000,
			},
		},
		GPUCount:           1,
		GPUTotalBytes:      128_000_000_000,
		SupportsGPUOffload: true,
		SystemRAMBytes:     128_000_000_000,
	}
	contextWindow := 131072
	nSeqMax := 2
	flashAttention := model.FlashAttentionEnabled
	splitMode := model.SplitModeNone
	nGpuLayers := 0
	constraints := ModelConfig{
		PtrContextWindow: &contextWindow,
		PtrNSeqMax:       &nSeqMax,
		FlashAttention:   &flashAttention,
		PtrSplitMode:     &splitMode,
		PtrNGpuLayers:    &nGpuLayers,
	}

	cfg, err := AutoTuneWithConfig(info, devs, constraints)
	if err != nil {
		t.Fatalf("AutoTuneWithConfig failed: %v", err)
	}

	if cfg.PtrContextWindow == nil || *cfg.PtrContextWindow != contextWindow {
		t.Errorf("PtrContextWindow: got %v, want %d", cfg.PtrContextWindow, contextWindow)
	}
	if cfg.PtrNSeqMax == nil || *cfg.PtrNSeqMax != nSeqMax {
		t.Errorf("PtrNSeqMax: got %v, want %d", cfg.PtrNSeqMax, nSeqMax)
	}
	if cfg.PtrPrefillBatchSize != nil {
		t.Errorf("PtrPrefillBatchSize: got %v, want nil", cfg.PtrPrefillBatchSize)
	}
	if cfg.FlashAttention == nil || *cfg.FlashAttention != flashAttention {
		t.Errorf("FlashAttention: got %v, want %s", cfg.FlashAttention, flashAttention)
	}
	if cfg.PtrSplitMode == nil || *cfg.PtrSplitMode != splitMode {
		t.Errorf("PtrSplitMode: got %v, want %s", cfg.PtrSplitMode, splitMode)
	}
	if cfg.PtrNGpuLayers == nil || *cfg.PtrNGpuLayers != nGpuLayers {
		t.Errorf("PtrNGpuLayers: got %v, want %d", cfg.PtrNGpuLayers, nGpuLayers)
	}
	if cfg.CacheTypeK == model.GGMLTypeAuto || cfg.CacheTypeV == model.GGMLTypeAuto {
		t.Errorf("cache types: got %s/%s, want AutoTune recommendation", cfg.CacheTypeK, cfg.CacheTypeV)
	}
}

func TestAutoTunePhysicalBatchFit(t *testing.T) {
	// This model has 1 GiB of weights and 32 MiB of F16 KV at 8K.
	// Its 2K/1K/512 compute estimates are about 563/422/352 MiB.
	// These budgets distinguish reducing batch from quantizing KV first.
	info := ModelInfo{
		ID: "Qwen3-1B-Q8_0", Size: 1 << 30,
		Metadata: map[string]string{
			"general.architecture": "qwen3", "qwen3.block_count": "4",
			"qwen3.context_length": "8192", "qwen3.embedding_length": "4096",
			"qwen3.attention.head_count": "16", "qwen3.attention.head_count_kv": "4",
			"qwen3.attention.key_length": "64", "qwen3.attention.value_length": "64",
		},
	}
	for _, tt := range []struct {
		name        string
		budgetMiB   int64
		gpus        int
		constraints ModelConfig
		wantBatch   int
		wantContext int
		wantCache   model.GGMLType
	}{
		{"preferred batch", 1700, 1, ModelConfig{}, 0, 8192, model.GGMLTypeF16},
		{"F16 before Q8", 1500, 1, ModelConfig{}, 1024, 8192, model.GGMLTypeF16},
		{"512 floor", 1450, 1, ModelConfig{}, 512, 8192, model.GGMLTypeF16},
		{"Q8 before context", 1400, 1, ModelConfig{}, 512, 8192, model.GGMLTypeQ8_0},
		{"context last", 1390, 1, ModelConfig{}, 512, 4096, model.GGMLTypeQ8_0},
		{"no fit below floor", 1370, 1, ModelConfig{}, 512, 4096, model.GGMLTypeQ8_0},
		{"unfittable explicit context", 1390, 1, ModelConfig{PtrContextWindow: new(8192)}, 512, 8192, model.GGMLTypeQ8_0},
		{"explicit batch", 1500, 1, ModelConfig{PtrPhysicalBatchSize: new(2048)}, 2048, 4096, model.GGMLTypeQ8_0},
		{"explicit below floor", 1400, 1, ModelConfig{PtrPhysicalBatchSize: new(256)}, 256, 8192, model.GGMLTypeF16},
		{"explicit precision", 1400, 1, ModelConfig{CacheTypeK: model.GGMLTypeF16, CacheTypeV: model.GGMLTypeF16}, 512, 4096, model.GGMLTypeF16},
		{"explicit context and concurrency", 1450, 1, ModelConfig{PtrContextWindow: new(8192), PtrNSeqMax: new(1)}, 512, 8192, model.GGMLTypeF16},
		{"multi GPU tensor unchanged", 1500, 2, ModelConfig{PtrSplitMode: new(model.SplitModeTensor)}, 0, 4096, model.GGMLTypeQ8_0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			devs := devices.Devices{GPUCount: tt.gpus, SupportsGPUOffload: true, SystemRAMBytes: 16 << 30}
			for i := range tt.gpus {
				devs.Devices = append(devs.Devices, devices.DeviceInfo{Index: i, Type: "gpu_cuda", TotalBytes: 8 << 30})
			}
			budget := AutoTuneBudget{GPUBytes: tt.budgetMiB << 20, SystemRAMBytes: 16 << 30}
			cfg, err := AutoTuneWithConfigAndBudget(info, devs, tt.constraints, budget)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.ToKronkConfig().PhysicalBatchSize(); got != tt.wantBatch {
				t.Errorf("physical batch: got %d, want %d", got, tt.wantBatch)
			}
			if cfg.PtrContextWindow == nil || *cfg.PtrContextWindow != tt.wantContext {
				t.Errorf("context: got %v, want %d", cfg.PtrContextWindow, tt.wantContext)
			}
			if cfg.CacheTypeK != tt.wantCache || cfg.CacheTypeV != tt.wantCache {
				t.Errorf("KV types: got %s/%s, want %s", cfg.CacheTypeK, cfg.CacheTypeV, tt.wantCache)
			}
			if cfg.PtrPrefillBatchSize != nil || cfg.PtrSWAFull != nil {
				t.Fatal("batch tuning changed the prefill contribution or SWA mode")
			}
		})
	}
}

func TestAnalyzeInklingFitsWeightsButNeedsSmallerBatch(t *testing.T) {
	info := ModelInfo{
		ID: "Inkling-2-Q8_0", Size: 8 << 30, HasProjection: true,
		Metadata: map[string]string{
			"general.architecture": "inkling2", "inkling2.block_count": "4",
			"inkling2.context_length": "131072", "inkling2.embedding_length": "2048",
			"inkling2.attention.head_count": "24", "inkling2.attention.head_count_kv": "4",
			"inkling2.attention.key_length": "128", "inkling2.attention.value_length": "128",
			"inkling2.attention.sliding_window": "512", "inkling2.shortconv.l_cache": "3",
			"inkling2.attention.sliding_window_pattern": "[false true true true]",
		},
	}
	devs := devices.Devices{
		GPUCount: 1, SupportsGPUOffload: true, SystemRAMBytes: 128 << 30,
		Devices: []devices.DeviceInfo{{Type: "gpu_cuda", FreeBytes: 32 << 30, TotalBytes: 32 << 30}},
	}
	constraints := ModelConfig{PtrContextWindow: new(131072), PtrNSeqMax: new(1)}
	a, err := AnalyzeWithConfig(info, devs, constraints)
	if err != nil {
		t.Fatal(err)
	}
	// Weights fit comfortably, but two 24 GiB attention-bias tensors at 2K
	// exceed this budget. F16 with batch 512 fits without shrinking context.
	if !a.Memory.FullGPUFit {
		t.Fatal("fixture weights do not fit on GPU")
	}
	if !a.Recommended.Fits || a.Recommended.PhysicalBatchSize != 512 || a.Recommended.ContextWindow != 131072 ||
		a.Recommended.CacheTypeK != "f16" || a.Recommended.CacheTypeV != "f16" {
		t.Fatalf("recommendation: got %+v, want fitting 128K/F16/512", a.Recommended)
	}

	for _, tt := range []struct {
		name     string
		capacity *int
		budget   *int
		wantSize int
		wantFits bool
	}{
		{"image capacity", new(640), nil, 640, true},
		{"image token budget", nil, new(640), 640, true},
		{"unfittable image capacity", new(1536), nil, 1536, false},
		{"unfittable image token budget", nil, new(1536), 1536, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			constraints := constraints
			constraints.PtrImageBatchTokenCapacity = tt.capacity
			constraints.PtrImageTokenBudget = tt.budget
			a, err := AnalyzeWithConfig(info, devs, constraints)
			if err != nil {
				t.Fatal(err)
			}
			if a.Recommended.Fits != tt.wantFits || a.Recommended.PhysicalBatchSize != 512 {
				t.Fatalf("recommendation: got %+v, want fits %t and requested batch 512", a.Recommended, tt.wantFits)
			}
			tuned, err := AutoTuneWithConfig(info, devs, constraints)
			if err != nil {
				t.Fatal(err)
			}
			if tuned.PtrPhysicalBatchSize == nil || *tuned.PtrPhysicalBatchSize != 512 {
				t.Fatalf("physical batch: got %v, want 512", tuned.PtrPhysicalBatchSize)
			}
			cfg := constraints.ToKronkConfig()
			cfg.PtrPhysicalBatchSize = tuned.PtrPhysicalBatchSize
			_, physical := cfg.GenerationBatchSizes(1, false)
			if physical != tt.wantSize {
				t.Errorf("effective image capacity: got %d, want %d", physical, tt.wantSize)
			}
		})
	}

	// With ample memory the architecture preference still wins, rather than
	// choosing 2K merely because it fits. Explicit requests remain authoritative.
	devs.Devices[0].FreeBytes = 128 << 30
	devs.Devices[0].TotalBytes = 128 << 30
	for _, tt := range []struct {
		name     string
		slots    int
		physical *int
		wantSize int
	}{
		{"one slot preference", 1, nil, 512},
		{"two slot preference", 2, nil, 512},
		{"explicit larger batch", 2, new(1024), 1024},
		{"explicit smaller batch", 2, new(256), 256},
	} {
		t.Run(tt.name, func(t *testing.T) {
			constraints := constraints
			constraints.PtrNSeqMax = new(tt.slots)
			constraints.PtrPhysicalBatchSize = tt.physical
			tuned, err := AutoTuneWithConfig(info, devs, constraints)
			if err != nil {
				t.Fatal(err)
			}
			cfg := tuned.ToKronkConfig()
			_, physical := cfg.GenerationBatchSizes(1, false)
			if cfg.PhysicalBatchSize() != tt.wantSize || physical != tt.wantSize {
				t.Errorf("requested/effective physical batch: got %d/%d, want %d", cfg.PhysicalBatchSize(), physical, tt.wantSize)
			}
			if cfg.BatchBaseSize() != 2048 || cfg.NSeqMax() != tt.slots || cfg.ContextWindow() != 131072 ||
				cfg.CacheTypeK != model.GGMLTypeF16 || cfg.CacheTypeV != model.GGMLTypeF16 {
				t.Errorf("preference changed other sizing: got %s", cfg)
			}
		})
	}
}

func TestAutoTuneSWA(t *testing.T) {
	devs := devices.Devices{
		GPUCount: 1, SupportsGPUOffload: true, SystemRAMBytes: 128 << 30,
		Devices: []devices.DeviceInfo{{Type: "gpu_cuda", FreeBytes: 128 << 30, TotalBytes: 128 << 30}},
	}
	for _, architecture := range []string{"inkling2", "inkling2_loop", "gemma4", "qwen2"} {
		info := ModelInfo{ID: "SWA-Q8_0", Size: 8 << 30, Metadata: map[string]string{
			"general.architecture": architecture,
		}}
		for key, value := range map[string]string{
			"block_count": "4", "context_length": "131072", "embedding_length": "2048",
			"attention.head_count": "24", "attention.head_count_kv": "4",
			"attention.key_length": "128", "attention.value_length": "128",
			"attention.sliding_window": "512", "shortconv.l_cache": "3",
			"attention.sliding_window_pattern": "[false true true true]",
			"loop.start":                       "1", "loop.end": "3", "loop.count": "3",
			"attn_res.block_size": "1", "loop.gate_rank": "8",
		} {
			info.Metadata[architecture+"."+key] = value
		}
		compact, err := AnalyzeWithConfig(info, devs, ModelConfig{
			PtrContextWindow: new(131072), PtrNSeqMax: new(1), PtrSWAFull: new(false),
		})
		if err != nil {
			t.Fatal(err)
		}
		compactBytes := compact.Recommended.EstimatedVRAMBytes
		for _, tt := range []struct {
			name string
			full *bool
			want bool
		}{
			{"compact default", nil, false},
			{"explicit compact", new(false), false},
			{"explicit full", new(true), true},
		} {
			t.Run(architecture+"/"+tt.name, func(t *testing.T) {
				constraints := ModelConfig{PtrContextWindow: new(131072), PtrNSeqMax: new(1), PtrSWAFull: tt.full}
				cfg, err := AutoTuneWithConfig(info, devs, constraints)
				if err != nil {
					t.Fatal(err)
				}
				if got := cfg.ToKronkConfig().SWAFull(); got != tt.want {
					t.Errorf("SWA full: got %t, want %t", got, tt.want)
				}
				a, err := AnalyzeWithConfig(info, devs, constraints)
				if err != nil {
					t.Fatal(err)
				}
				switch {
				case tt.want && a.Recommended.EstimatedVRAMBytes <= compactBytes:
					t.Errorf("full estimate: got %d, want greater than compact %d", a.Recommended.EstimatedVRAMBytes, compactBytes)
				case !tt.want && a.Recommended.EstimatedVRAMBytes != compactBytes:
					t.Errorf("compact estimate: got %d, want %d", a.Recommended.EstimatedVRAMBytes, compactBytes)
				}
			})
		}
	}
}

func TestAnalyzeBatchLocalSWA(t *testing.T) {
	devs := devices.Devices{
		GPUCount: 1, SupportsGPUOffload: true, UnifiedMemory: true, SystemRAMBytes: 128 << 30,
		Devices: []devices.DeviceInfo{{Type: "gpu_metal", FreeBytes: 128 << 30, TotalBytes: 128 << 30}},
	}
	for _, tt := range []struct {
		name, architecture, causal, decision string
		noCache                              bool
	}{
		{"embedding encoder", "gemma-embedding", "", "", true},
		{"noncausal LFM3", "lfm3", "false", "", true},
		{"LFM3 decision", "lfm3", "false", "lfm3-d1", true},
		{"LFM3 decision contract", "lfm3", "", "lfm3-d1-omni", true},
		{"causal LFM3 default", "lfm3", "", "", false},
		{"causal LFM3 explicit", "lfm3", "true", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := ModelInfo{ID: tt.name, Size: 1 << 30, Metadata: map[string]string{
				"general.architecture": tt.architecture,
			}}
			for key, value := range map[string]string{
				"block_count": "4", "context_length": "32768", "embedding_length": "256", "vocab_size": "1024",
				"attention.head_count": "8", "attention.head_count_kv": "2",
				"attention.key_length": "32", "attention.value_length": "32", "attention.sliding_window": "512",
			} {
				info.Metadata[tt.architecture+"."+key] = value
			}
			if tt.architecture == "lfm3" {
				info.Metadata["lfm3.attention.head_count_kv"] = "[0 2 0 4]"
				info.Metadata["lfm3.attention.sliding_window_pattern"] = "[false true false false]"
				info.Metadata["lfm3.shortconv.l_cache"] = "3"
			}
			if tt.causal != "" {
				info.Metadata["lfm3.attention.causal"] = tt.causal
			}
			if tt.decision != "" {
				info.Metadata["lfm3.decision.type"] = tt.decision
			}
			constraints := ModelConfig{PtrContextWindow: new(8192), PtrNSeqMax: new(1), PtrSWAFull: new(false)}
			compact, err := AnalyzeWithConfig(info, devs, constraints)
			if err != nil {
				t.Fatal(err)
			}
			constraints.PtrSWAFull = new(true)
			full, err := AnalyzeWithConfig(info, devs, constraints)
			if err != nil {
				t.Fatal(err)
			}
			if tt.noCache && full.Recommended.EstimatedVRAMBytes != compact.Recommended.EstimatedVRAMBytes {
				t.Errorf("batch-local estimate depends on SWA cache mode: got %d/%d", compact.Recommended.EstimatedVRAMBytes, full.Recommended.EstimatedVRAMBytes)
			}
			if !tt.noCache && full.Recommended.EstimatedVRAMBytes <= compact.Recommended.EstimatedVRAMBytes {
				t.Error("causal LFM3 lost its full/compact cache distinction")
			}
			if tt.decision != "" {
				constraints.PtrPrefillBatchSize = new(1024)
				smaller, err := AnalyzeWithConfig(info, devs, constraints)
				if err != nil {
					t.Fatal(err)
				}
				if smaller.Recommended.EstimatedVRAMBytes != full.Recommended.EstimatedVRAMBytes {
					t.Error("whole-prompt decision estimate depends on generation prefill size")
				}
			}
		})
	}
}

func TestAutoTuneLeavesSplitModeUnset(t *testing.T) {
	info := ModelInfo{
		ID:   "Qwen3-8B-Q8_0",
		Size: 8_000_000_000,
		Metadata: map[string]string{
			"general.architecture":          "qwen3",
			"qwen3.block_count":             "24",
			"qwen3.context_length":          "32768",
			"qwen3.embedding_length":        "2048",
			"qwen3.attention.head_count":    "16",
			"qwen3.attention.head_count_kv": "8",
			"qwen3.attention.key_length":    "128",
			"qwen3.attention.value_length":  "128",
		},
	}
	devs := devices.Devices{
		GPUCount:           2,
		SupportsGPUOffload: true,
		Devices: []devices.DeviceInfo{
			{Name: "CUDA0", Type: "gpu_cuda", FreeBytes: 24_000_000_000},
			{Name: "CUDA1", Type: "gpu_cuda", FreeBytes: 24_000_000_000},
		},
	}

	cfg, err := AutoTuneWithConfig(info, devs, ModelConfig{Devices: []string{"CUDA0"}})
	if err != nil {
		t.Fatalf("AutoTuneWithConfig failed: %v", err)
	}
	if cfg.PtrSplitMode != nil {
		t.Fatalf("PtrSplitMode: got %s, want nil for load-time device resolution", cfg.PtrSplitMode.String())
	}
	for _, tt := range []struct {
		name        string
		constraints ModelConfig
		want        int
	}{
		{"one selected GPU", ModelConfig{Devices: []string{"CUDA0"}}, 0},
		{"multi GPU layer", ModelConfig{}, 512},
		{"explicit request", ModelConfig{PtrPhysicalBatchSize: new(1024)}, 1024},
		{"explicit one GPU", ModelConfig{Devices: []string{"CUDA0"}, PtrPhysicalBatchSize: new(256)}, 256},
		{"tensor split", ModelConfig{PtrSplitMode: new(model.SplitModeTensor)}, 0},
		{"tensor placement override", ModelConfig{TensorBuftOverrides: []string{"blk.*=CPU"}}, 0},
		{"CPU only", ModelConfig{PtrNGpuLayers: new(-1)}, 0},
		{"host KV", ModelConfig{PtrOffloadKQV: new(false)}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := AutoTuneWithConfig(info, devs, tt.constraints)
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.ToKronkConfig().PhysicalBatchSize(); got != tt.want {
				t.Errorf("PhysicalBatchSize: got %d, want %d", got, tt.want)
			}
			if cfg.PtrPrefillBatchSize != nil {
				t.Fatal("physical tuning materialized a prompt contribution override")
			}
		})
	}
	// Both ordinary text and companion MTP honor the physical request in
	// compute estimates without reducing their logical generation reserves.
	constraints := ModelConfig{PtrContextWindow: new(8192), PtrNSeqMax: new(3), PtrPhysicalBatchSize: new(512)}
	small, err := AnalyzeWithConfig(info, devs, constraints)
	if err != nil {
		t.Fatal(err)
	}
	constraints.PtrPhysicalBatchSize = new(2048)
	large, err := AnalyzeWithConfig(info, devs, constraints)
	if err != nil {
		t.Fatal(err)
	}
	if small.Recommended.EstimatedVRAMBytes >= large.Recommended.EstimatedVRAMBytes {
		t.Fatal("ordinary text physical size did not reduce the memory estimate")
	}
	info.HasMTP = true
	mtpLarge, err := AnalyzeWithConfig(info, devs, constraints)
	if err != nil {
		t.Fatal(err)
	}
	constraints.PtrPhysicalBatchSize = new(512)
	mtpSmall, err := AnalyzeWithConfig(info, devs, constraints)
	if err != nil {
		t.Fatal(err)
	}
	if mtpSmall.Recommended.EstimatedVRAMBytes >= mtpLarge.Recommended.EstimatedVRAMBytes {
		t.Fatal("MTP physical size did not reduce the memory estimate")
	}
}

func TestAutoTuneSelectsTensorParallelForTwoROCmGPUs(t *testing.T) {
	info := ModelInfo{
		ID:   "Qwen3-8B-Q8_0",
		Size: 8_000_000_000,
		Metadata: map[string]string{
			"general.architecture":          "qwen3",
			"qwen3.block_count":             "24",
			"qwen3.context_length":          "32768",
			"qwen3.embedding_length":        "2048",
			"qwen3.attention.head_count":    "16",
			"qwen3.attention.head_count_kv": "8",
			"qwen3.attention.key_length":    "128",
			"qwen3.attention.value_length":  "128",
		},
	}
	glm := ModelInfo{
		ID: "GLM5-Next", Size: info.Size,
		Metadata: map[string]string{
			"general.architecture":              "glm5-next",
			"glm5-next.block_count":             "4",
			"glm5-next.context_length":          "32768",
			"glm5-next.embedding_length":        "6144",
			"glm5-next.attention.head_count":    "64",
			"glm5-next.attention.head_count_kv": "[0 0 0 1]",
			"glm5-next.attention.key_length":    "512",
			"glm5-next.attention.value_length":  "512",
			"glm5-next.ssm.conv_kernel":         "4",
			"glm5-next.kda.head_dim":            "128",
		},
	}
	rocm := devices.Devices{
		GPUCount:           2,
		SupportsGPUOffload: true,
		Devices: []devices.DeviceInfo{
			{Name: "ROCm0", Type: "gpu_rocm", FreeBytes: 24_000_000_000},
			{Name: "ROCm1", Type: "gpu_rocm", FreeBytes: 24_000_000_000},
		},
	}

	tests := []struct {
		name        string
		info        ModelInfo
		devs        devices.Devices
		constraints ModelConfig
		want        *model.SplitMode
	}{
		{"two ROCm GPUs", info, rocm, ModelConfig{}, new(model.SplitModeTensor)},
		{"GLM cache replication needs explicit TP", glm, rocm, ModelConfig{}, nil},
		{"explicit GLM TP remains a constraint", glm, rocm, ModelConfig{
			PtrSplitMode: new(model.SplitModeTensor),
		}, new(model.SplitModeTensor)},
		{"one selected ROCm GPU", info, rocm, ModelConfig{Devices: []string{"ROCm0"}}, nil},
		{"three ROCm GPUs", info, devices.Devices{
			GPUCount: 3, SupportsGPUOffload: true,
			Devices: append(append([]devices.DeviceInfo{}, rocm.Devices...),
				devices.DeviceInfo{Name: "ROCm2", Type: "gpu_rocm", FreeBytes: 24_000_000_000}),
		}, ModelConfig{}, nil},
		{"two CUDA GPUs", info, devices.Devices{
			GPUCount: 2, SupportsGPUOffload: true,
			Devices: []devices.DeviceInfo{
				{Name: "CUDA0", Type: "gpu_cuda", FreeBytes: 24_000_000_000},
				{Name: "CUDA1", Type: "gpu_cuda", FreeBytes: 24_000_000_000},
			},
		}, ModelConfig{}, nil},
		{"unsupported architecture", ModelInfo{
			ID: "Mamba", Size: info.Size,
			Metadata: map[string]string{
				"general.architecture": "mamba",
				"mamba.block_count":    "24",
			},
		}, rocm, ModelConfig{}, nil},
		{"flash attention disabled", info, rocm, ModelConfig{
			FlashAttention: new(model.FlashAttentionDisabled),
		}, nil},
		{"CPU-only placement", info, rocm, ModelConfig{PtrNGpuLayers: new(-1)}, nil},
		{"explicit layer mode", info, rocm, ModelConfig{
			PtrSplitMode: new(model.SplitModeLayer),
		}, new(model.SplitModeLayer)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := AutoTuneWithConfig(tt.info, tt.devs, tt.constraints)
			if err != nil {
				t.Fatalf("AutoTuneWithConfig failed: %v", err)
			}
			if tt.want == nil {
				if cfg.PtrSplitMode != nil {
					t.Errorf("PtrSplitMode: got %s, want nil", cfg.PtrSplitMode.String())
				}
				return
			}
			if cfg.PtrSplitMode == nil || *cfg.PtrSplitMode != *tt.want {
				t.Errorf("PtrSplitMode: got %v, want %s", cfg.PtrSplitMode, tt.want.String())
			}
		})
	}
}

func TestAnalyzeUsesSelectedDevices(t *testing.T) {
	info := ModelInfo{
		ID:   "Qwen3-8B-Q8_0",
		Size: 8_000_000_000,
		Metadata: map[string]string{
			"general.architecture":          "qwen3",
			"qwen3.block_count":             "24",
			"qwen3.context_length":          "32768",
			"qwen3.embedding_length":        "2048",
			"qwen3.attention.head_count":    "16",
			"qwen3.attention.head_count_kv": "8",
			"qwen3.attention.key_length":    "128",
			"qwen3.attention.value_length":  "128",
		},
	}
	devs := devices.Devices{
		GPUCount:           2,
		SupportsGPUOffload: true,
		Devices: []devices.DeviceInfo{
			{Name: "CUDA0", Type: "gpu_cuda", FreeBytes: 8_000_000_000, TotalBytes: 8_000_000_000},
			{Name: "CUDA1", Type: "gpu_cuda", FreeBytes: 48_000_000_000, TotalBytes: 48_000_000_000},
		},
	}

	analysis, err := AnalyzeWithConfig(info, devs, ModelConfig{Devices: []string{"CUDA0"}})
	if err != nil {
		t.Fatalf("AnalyzeWithConfig failed: %v", err)
	}
	if analysis.System.GPUName != "CUDA0" || analysis.System.GPUFreeBytes != 8_000_000_000 {
		t.Fatalf("selected GPU: got %s/%d, want CUDA0/8000000000", analysis.System.GPUName, analysis.System.GPUFreeBytes)
	}
}

func TestProfileVTransposed(t *testing.T) {
	auto := model.FlashAttentionAuto
	disabled := model.FlashAttentionDisabled

	tests := []struct {
		name string
		p    profileInput
		want bool
	}{
		{name: "GPU default recommends auto", p: profileInput{hasGPU: true}, want: false},
		{name: "explicit auto", p: profileInput{hasGPU: true, flashAttention: &auto}, want: false},
		{name: "explicit disabled", p: profileInput{hasGPU: true, flashAttention: &disabled}, want: true},
		{name: "CPU default disables flash attention", p: profileInput{}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := profileVTransposed(tt.p); got != tt.want {
				t.Fatalf("profileVTransposed: got %t, want %t", got, tt.want)
			}
		})
	}
}

func TestNormalizeAnalysisConfig(t *testing.T) {
	zero := 0
	negative := -1

	tests := []struct {
		name            string
		contextWindow   *int
		nSeqMax         *int
		trainingContext int64
		wantContext     int
		wantNSeqMax     int
	}{
		{"zero uses runtime defaults", &zero, &zero, vram.ContextWindow64K, int(vram.ContextWindow8K), 1},
		{"negative uses short training context", &negative, &negative, 2048, 2048, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := normalizeAnalysisConfig(ModelConfig{
				PtrContextWindow: tt.contextWindow,
				PtrNSeqMax:       tt.nSeqMax,
			}, tt.trainingContext)

			if cfg.PtrContextWindow == nil || *cfg.PtrContextWindow != tt.wantContext {
				t.Errorf("PtrContextWindow: got %v, want %d", cfg.PtrContextWindow, tt.wantContext)
			}
			if cfg.PtrNSeqMax == nil || *cfg.PtrNSeqMax != tt.wantNSeqMax {
				t.Errorf("PtrNSeqMax: got %v, want %d", cfg.PtrNSeqMax, tt.wantNSeqMax)
			}
		})
	}
}

func TestAnalyzeBalancedContextCap(t *testing.T) {

	// A small model whose training context exceeds 128K, on a GPU with ample
	// free VRAM. The balanced/Recommended profile should climb to the 128K cap
	// (no longer clamped to 32K) while staying within the GPU budget.
	info := ModelInfo{
		ID:   "SmallLongCtx-3B-Q8_0",
		Desc: "Small Long Context 3B",
		Size: 3_000_000_000,
		Metadata: map[string]string{
			"general.architecture":          "qwen3",
			"general.file_type":             "7",
			"qwen3.block_count":             "24",
			"qwen3.context_length":          "262144",
			"qwen3.embedding_length":        "2048",
			"qwen3.attention.head_count":    "16",
			"qwen3.attention.head_count_kv": "8",
			"qwen3.attention.key_length":    "128",
			"qwen3.attention.value_length":  "128",
		},
	}

	devs := devices.Devices{
		Devices: []devices.DeviceInfo{
			{
				Index:      0,
				Name:       "Apple M5 Max",
				Type:       "gpu_metal",
				FreeBytes:  115_000_000_000,
				TotalBytes: 128_000_000_000,
			},
		},
		GPUCount:           1,
		GPUTotalBytes:      128_000_000_000,
		SupportsGPUOffload: true,
		SystemRAMBytes:     128_000_000_000,
	}

	a, err := analyzeModel(info, devs)
	if err != nil {
		t.Fatalf("analyzeModel failed: %v", err)
	}

	if a.Recommended.Name != "balanced" {
		t.Fatalf("Recommended.Name = %q, want %q", a.Recommended.Name, "balanced")
	}

	if a.Recommended.ContextWindow != vram.ContextWindow128K {
		t.Errorf("Recommended.ContextWindow = %d, want %d (128K cap)", a.Recommended.ContextWindow, vram.ContextWindow128K)
	}

	if !a.Recommended.Fits {
		t.Error("Recommended should fit within the GPU budget")
	}
}

func TestClassifyModel(t *testing.T) {
	tests := []struct {
		name string
		info ModelInfo
		moe  MoEInfo
		role modelprofile.Role
		want string
	}{
		{
			name: "dense",
			info: ModelInfo{ID: "model"},
			role: modelprofile.RoleLanguage,
			want: "dense",
		},
		{
			name: "moe",
			info: ModelInfo{ID: "model"},
			moe:  MoEInfo{IsMoE: true, ExpertCount: 8},
			role: modelprofile.RoleLanguage,
			want: "moe",
		},
		{
			name: "vision-projection",
			info: ModelInfo{ID: "model", HasProjection: true},
			role: modelprofile.RoleLanguage,
			want: "vision",
		},
		{
			name: "embedding",
			info: ModelInfo{ID: "embed-model", IsEmbedModel: true},
			role: modelprofile.RoleLanguage,
			want: "embedding",
		},
		{
			name: "rerank",
			info: ModelInfo{ID: "rerank-model", IsRerankModel: true},
			role: modelprofile.RoleLanguage,
			want: "rerank",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyModel(tt.info, tt.moe, modelprofile.Profile{Role: tt.role})
			if got != tt.want {
				t.Errorf("classifyModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}

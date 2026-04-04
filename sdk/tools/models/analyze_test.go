package models

import (
	"testing"

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

	if a.Recommended.NSeqMax != 2 {
		t.Errorf("Recommended.NSeqMax = %d, want %d", a.Recommended.NSeqMax, 2)
	}

	if a.Recommended.FlashAttention != "auto" {
		t.Errorf("Recommended.FlashAttention = %q, want %q", a.Recommended.FlashAttention, "auto")
	}

	if a.Recommended.ContextWindow < ContextWindow8K {
		t.Errorf("Recommended.ContextWindow = %d, want >= %d", a.Recommended.ContextWindow, ContextWindow8K)
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

	a, err := analyzeModel(info, devs)
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

	// Should have a warning about SWA.
	found := false
	for _, w := range a.Warnings {
		if contains(w, "sliding window") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected a sliding window warning")
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
	if a.Recommended.ContextWindow < ContextWindow4K {
		t.Errorf("ContextWindow = %d, want >= %d", a.Recommended.ContextWindow, ContextWindow4K)
	}

	if a.Recommended.EstimatedVRAMBytes <= 0 {
		t.Error("EstimatedVRAMBytes should be > 0")
	}
}

func TestToModelConfig(t *testing.T) {
	rec := RuntimeRecommendation{
		Name:           "balanced",
		ContextWindow:  32768,
		NSeqMax:        2,
		CacheTypeK:     "f16",
		CacheTypeV:     "f16",
		FlashAttention: "auto",
		NGPULayers:     0, // 0 = all on GPU in model.Config
	}

	cfg := rec.ToModelConfig()

	if cfg.ContextWindow != 32768 {
		t.Errorf("ContextWindow = %d, want 32768", cfg.ContextWindow)
	}

	if cfg.NSeqMax != 2 {
		t.Errorf("NSeqMax = %d, want 2", cfg.NSeqMax)
	}

	if cfg.CacheTypeK != 1 { // model.GGMLTypeF16
		t.Errorf("CacheTypeK = %d, want 1 (f16)", cfg.CacheTypeK)
	}

	if cfg.CacheTypeV != 1 { // model.GGMLTypeF16
		t.Errorf("CacheTypeV = %d, want 1 (f16)", cfg.CacheTypeV)
	}

	if cfg.FlashAttention != 2 { // model.FlashAttentionAuto
		t.Errorf("FlashAttention = %d, want 2 (auto)", cfg.FlashAttention)
	}

	// NGPULayers=0 means all on GPU, which is the default (nil).
	if cfg.NGpuLayers != nil {
		t.Errorf("NGpuLayers = %v, want nil (all on GPU)", cfg.NGpuLayers)
	}

	if cfg.NBatch != 2048 {
		t.Errorf("NBatch = %d, want 2048", cfg.NBatch)
	}

	if cfg.NUBatch != 512 {
		t.Errorf("NUBatch = %d, want 512", cfg.NUBatch)
	}
}

func TestToModelConfigQ8(t *testing.T) {
	rec := RuntimeRecommendation{
		Name:           "balanced",
		ContextWindow:  8192,
		NSeqMax:        1,
		CacheTypeK:     "q8_0",
		CacheTypeV:     "q8_0",
		FlashAttention: "disabled",
		NGPULayers:     0,
	}

	cfg := rec.ToModelConfig()

	if cfg.CacheTypeK != 8 { // model.GGMLTypeQ8_0
		t.Errorf("CacheTypeK = %d, want 8 (q8_0)", cfg.CacheTypeK)
	}

	if cfg.FlashAttention != 1 { // model.FlashAttentionDisabled
		t.Errorf("FlashAttention = %d, want 1 (disabled)", cfg.FlashAttention)
	}

	if cfg.NGpuLayers != nil {
		t.Errorf("NGpuLayers = %v, want nil", cfg.NGpuLayers)
	}
}

func TestGGUFFileTypeName(t *testing.T) {
	tests := []struct {
		ft   int64
		want string
	}{
		{0, "F32"},
		{1, "F16"},
		{7, "Q8_0"},
		{15, "Q4_K_M"},
		{17, "Q5_K_M"},
		{28, "BF16"},
		{999, "unknown(999)"},
	}

	for _, tt := range tests {
		got := ggufFileTypeName(tt.ft)
		if got != tt.want {
			t.Errorf("ggufFileTypeName(%d) = %q, want %q", tt.ft, got, tt.want)
		}
	}
}

func TestCountSWALayers(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    int64
	}{
		{
			name:    "gemma4-60-layers",
			pattern: "[true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false]",
			want:    50,
		},
		{
			name:    "all-true",
			pattern: "[true true true]",
			want:    3,
		},
		{
			name:    "all-false",
			pattern: "[false false false]",
			want:    0,
		},
		{
			name:    "empty",
			pattern: "[]",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countSWALayers(tt.pattern)
			if got != tt.want {
				t.Errorf("countSWALayers() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClassifyModel(t *testing.T) {
	tests := []struct {
		name string
		info ModelInfo
		moe  MoEInfo
		arch string
		want string
	}{
		{
			name: "dense",
			info: ModelInfo{ID: "model"},
			arch: "llama",
			want: "dense",
		},
		{
			name: "moe",
			info: ModelInfo{ID: "model"},
			moe:  MoEInfo{IsMoE: true, ExpertCount: 8},
			arch: "qwen3moe",
			want: "moe",
		},
		{
			name: "vision-projection",
			info: ModelInfo{ID: "model", HasProjection: true},
			arch: "llama",
			want: "vision",
		},
		{
			name: "embedding",
			info: ModelInfo{ID: "embed-model", IsEmbedModel: true},
			arch: "bert",
			want: "embedding",
		},
		{
			name: "rerank",
			info: ModelInfo{ID: "rerank-model", IsRerankModel: true},
			arch: "bert",
			want: "rerank",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyModel(tt.info, tt.moe, tt.arch)
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

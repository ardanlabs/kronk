package models

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
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

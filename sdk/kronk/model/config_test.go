package model

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
	"go.yaml.in/yaml/v2"
)

func TestMoEMode(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    MoEMode
		wantErr bool
	}{
		{"auto", "auto", MoEModeAuto, false},
		{"experts CPU", "experts_cpu", MoEModeExpertsCPU, false},
		{"experts GPU", "experts_gpu", MoEModeExpertsGPU, false},
		{"keep top N", "keep_top_n", MoEModeKeepTopN, false},
		{"custom", "custom", MoEModeCustom, false},
		{"unknown", "unknown", MoEMode{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMoEMode(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMoEMode() error = %v, wantErr %t", err, tt.wantErr)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseMoEMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultSplitMode(t *testing.T) {
	tests := []struct {
		name     string
		gpuCount int
	}{
		{"no GPU", 0},
		{"single GPU", 1},
		{"multiple GPUs", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultSplitMode(tt.gpuCount); got != SplitModeLayer {
				t.Errorf("DefaultSplitMode() = %s, want %s", got, SplitModeLayer)
			}
		})
	}
}

func TestMoEModeYAML(t *testing.T) {
	var cfg MoEConfig
	if err := yaml.Unmarshal([]byte("mode: experts_cpu\n"), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v, want nil", err)
	}
	if !cfg.Mode.Equal(MoEModeExpertsCPU) {
		t.Errorf("yaml.Unmarshal() mode = %q, want %q", cfg.Mode, MoEModeExpertsCPU)
	}

	if err := yaml.Unmarshal([]byte("mode: unknown\n"), &cfg); err == nil {
		t.Fatal("yaml.Unmarshal() error = nil, want error")
	}
}

func TestConfigStringIncludesCompleteDiagnostics(t *testing.T) {
	cfg := Config{
		AutoTune: true,
		ChatTemplateKwargs: D{
			"preserve_thinking": true,
			"secret":            "do not log",
		},
		DefaultParams:         Params{Grammar: "secret grammar"},
		PtrAdmissionTimeout:   new(5 * time.Minute),
		PtrIMCSessionCapacity: new(9),
		PtrQueueDepth:         new(7),
	}

	got := cfg.String()
	for _, want := range []string{
		"AutoTune[true]",
		"AdmissionTimeout[5m0s]",
		"ChatTemplateKwargs[preserve_thinking=true secret=configured]",
		"DefaultParams[",
		"grammar[true]",
		"IMCSessionCapacity[9]",
		"QueueDepth[7]",
		"ProjDevice[]",
		"Speculation[auto]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Config.String() missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, cfg.DefaultParams.Grammar) {
		t.Errorf("Config.String() exposed grammar contents in %q", got)
	}
	if strings.Contains(got, cfg.ChatTemplateKwargs["secret"].(string)) {
		t.Errorf("Config.String() exposed chat template kwarg contents in %q", got)
	}
}

func TestChatTemplateKwargsOwnership(t *testing.T) {
	source := D{
		"preserve_thinking": true,
		"nested": D{
			"mode": "source",
		},
	}
	cfg := NewConfig(WithChatTemplateKwargs(source))

	source["preserve_thinking"] = false
	source["nested"].(D)["mode"] = "mutated"
	if got := cfg.ChatTemplateKwargs["preserve_thinking"]; got != true {
		t.Errorf("configured preserve_thinking: got %v, want true", got)
	}
	if got := cfg.ChatTemplateKwargs["nested"].(D)["mode"]; got != "source" {
		t.Errorf("configured nested mode: got %v, want source", got)
	}

	m := Model{cfg: cfg}
	exposed := m.Config()
	exposed.ChatTemplateKwargs["preserve_thinking"] = false
	exposed.ChatTemplateKwargs["nested"].(D)["mode"] = "mutated"
	if got := m.cfg.ChatTemplateKwargs["preserve_thinking"]; got != true {
		t.Errorf("model preserve_thinking: got %v, want true", got)
	}
	if got := m.cfg.ChatTemplateKwargs["nested"].(D)["mode"]; got != "source" {
		t.Errorf("model nested mode: got %v, want source", got)
	}
}

func TestAdjustAdmissionTimeout(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want time.Duration
	}{
		{"unset defaults", Config{}, defaultAdmissionTimeout},
		{"zero defaults", NewConfig(WithAdmissionTimeout(0)), defaultAdmissionTimeout},
		{"override preserved", NewConfig(WithAdmissionTimeout(45 * time.Second)), 45 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := adjustAdmissionTimeout(tt.cfg)
			if got := cfg.AdmissionTimeout(); got != tt.want {
				t.Errorf("AdmissionTimeout: got %s, want %s", got, tt.want)
			}
		})
	}

	if got := (Config{}).String(); !strings.Contains(got, "AdmissionTimeout[nil]") {
		t.Errorf("unadjusted Config.String() = %q, want nil admission timeout diagnostic", got)
	}
}

func TestAdjustQueueDepthDefault(t *testing.T) {
	cfg := adjustQueueDepth(Config{})
	if got := cfg.QueueDepth(); got != DefaultQueueDepth {
		t.Errorf("QueueDepth: got %d, want %d", got, DefaultQueueDepth)
	}
	if !strings.Contains(cfg.String(), "QueueDepth[2]") {
		t.Errorf("adjusted Config.String() missing effective queue-depth default in %q", cfg.String())
	}
}

func TestAdjustIMCSessionCapacity(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want int
	}{
		{"derived default", NewConfig(WithNSeqMax(2), WithQueueDepth(2)), 6},
		{"queue expansion", NewConfig(WithNSeqMax(2), WithQueueDepth(4)), 8},
		{"explicit capacity preserved", NewConfig(WithNSeqMax(2), WithQueueDepth(2), WithIMCSessionCapacity(8)), 8},
		{"embedding has no session pool", NewConfig(WithModelFiles([]string{"Qwen3-Embedding-0.6B-Q8_0.gguf"}), WithNSeqMax(2), WithQueueDepth(2)), 0},
		{"rerank has no session pool", NewConfig(WithModelFiles([]string{"bge-reranker-v2-m3-Q8_0.gguf"}), WithNSeqMax(2), WithQueueDepth(2)), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := adjustIMCSessionCapacity(tt.cfg)
			if got := cfg.IMCSessionCapacity(); got != tt.want {
				t.Errorf("IMCSessionCapacity: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSWAFull(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"unset is not explicitly enabled", Config{}, false},
		{"enabled", NewConfig(WithSWAFull(true)), true},
		{"disabled", NewConfig(WithSWAFull(false)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.SWAFull(); got != tt.want {
				t.Errorf("SWAFull: got %t, want %t", got, tt.want)
			}
		})
	}
}

func TestEffectiveSWAFull(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name         string
		configured   *bool
		llamaDefault bool
		want         bool
	}{
		{"unset uses enabled llama default", nil, true, true},
		{"unset uses disabled llama default", nil, false, false},
		{"explicit enabled overrides default", &enabled, false, true},
		{"explicit disabled overrides default", &disabled, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveSWAFull(tt.configured, tt.llamaDefault); got != tt.want {
				t.Errorf("effectiveSWAFull: got %t, want %t", got, tt.want)
			}
		})
	}
}

func TestModelInfoStringIncludesNSWA(t *testing.T) {
	const nSWA = 4096

	got := (ModelInfo{NSWA: nSWA, FileType: 15, Quantization: "Q4_K - Medium"}).String()
	if !strings.Contains(got, "NSWA[4096]") {
		t.Errorf("ModelInfo.String: got %q, want NSWA[%d]", got, nSWA)
	}
	if !strings.Contains(got, "FileType[15]") || !strings.Contains(got, "Quantization[Q4_K - Medium]") {
		t.Errorf("ModelInfo.String: got %q, want friendly quantization", got)
	}
}

func TestFlashAttentionPresence(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantSet   bool
		wantValue FlashAttentionType
	}{
		{"unset", NewConfig(), false, FlashAttentionAuto},
		{"explicit enabled", NewConfig(WithFlashAttention(FlashAttentionEnabled)), true, FlashAttentionEnabled},
		{"explicit disabled", NewConfig(WithFlashAttention(FlashAttentionDisabled)), true, FlashAttentionDisabled},
		{"explicit auto", NewConfig(WithFlashAttention(FlashAttentionAuto)), true, FlashAttentionAuto},
		{"with config preserves explicit", NewConfig(WithConfig(NewConfig(WithFlashAttention(FlashAttentionEnabled)))), true, FlashAttentionEnabled},
		{"with config preserves unset", NewConfig(WithConfig(Config{})), false, FlashAttentionAuto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.PtrFlashAttention != nil; got != tt.wantSet {
				t.Errorf("PtrFlashAttention presence: got %t, want %t", got, tt.wantSet)
			}
			if got := tt.cfg.FlashAttention(); got != tt.wantValue {
				t.Errorf("FlashAttention: got %s, want %s", got, tt.wantValue)
			}
		})
	}
}

func TestFlashAttentionToYZMAType(t *testing.T) {
	tests := []struct {
		name  string
		value FlashAttentionType
		want  llama.FlashAttentionType
	}{
		{"enabled", FlashAttentionEnabled, llama.FlashAttentionTypeEnabled},
		{"disabled", FlashAttentionDisabled, llama.FlashAttentionTypeDisabled},
		{"auto", FlashAttentionAuto, llama.FlashAttentionTypeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.value.toYZMAType(); got != tt.want {
				t.Errorf("toYZMAType: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAdjustConfigUsesOneBatchDefault(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		wantNBatch  int
		wantNUBatch int
	}{
		{"text", NewConfig(WithContextWindow(8192), WithNSeqMax(4)), DefaultPrefillBatchSize + 4, DefaultPrefillBatchSize},
		{"projection", NewConfig(WithContextWindow(8192), WithNSeqMax(4), WithProjFile("mmproj.gguf")), DefaultPrefillBatchSize + 4, DefaultPrefillBatchSize},
		{"MoE CPU experts", NewConfig(WithContextWindow(8192), WithNSeqMax(4), WithMoE(&MoEConfig{Mode: MoEModeExpertsCPU})), DefaultPrefillBatchSize + 4, DefaultPrefillBatchSize},
		{"embedding", NewConfig(WithModelFiles([]string{"Qwen3-Embedding-0.6B-Q8_0.gguf"}), WithContextWindow(8192), WithNSeqMax(4)), DefaultPrefillBatchSize, DefaultPrefillBatchSize},
		{"rerank", NewConfig(WithModelFiles([]string{"bge-reranker-v2-m3-Q8_0.gguf"}), WithContextWindow(8192), WithNSeqMax(4)), DefaultPrefillBatchSize, DefaultPrefillBatchSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := adjustConfig(tt.cfg, 0)
			if got := cfg.EffectiveNUBatch(); got != tt.wantNUBatch {
				t.Errorf("NUBatch: got %d, want %d", got, tt.wantNUBatch)
			}
			if got := cfg.EffectiveNBatch(); got != tt.wantNBatch {
				t.Errorf("NBatch: got %d, want %d", got, tt.wantNBatch)
			}
			if got := cfg.PrefillBatchSize(); got != DefaultPrefillBatchSize {
				t.Errorf("PrefillBatchSize: got %d, want %d", got, DefaultPrefillBatchSize)
			}
		})
	}
}

func TestAdjustConfigUsesConfiguredPrefillBatchSize(t *testing.T) {
	cfg := NewConfig(
		WithContextWindow(8192),
		WithNSeqMax(4),
		WithPrefillBatchSize(4096),
	)

	got := adjustConfig(cfg, 0)

	if got.PrefillBatchSize() != 4096 {
		t.Errorf("PrefillBatchSize: got %d, want %d", got.PrefillBatchSize(), 4096)
	}
	if got.EffectiveNBatch() != 4100 || got.EffectiveNUBatch() != 4096 {
		t.Errorf("effective batch: got %d/%d, want 4100/4096", got.EffectiveNBatch(), got.EffectiveNUBatch())
	}
}

func TestAdjustGenerationBatchUsesPrefillBatchSize(t *testing.T) {
	tests := []struct {
		name                string
		generationRows      int
		singlePhysicalBatch bool
		wantNBatch          int
		wantNUBatch         int
	}{
		{"non-MTP", 1, false, 1028, 1024},
		{"MTP", 1 + defMTPNDraft, true, 1040, 1040},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig(WithNSeqMax(4), WithPrefillBatchSize(1024))
			got := adjustGenerationBatch(cfg, tt.generationRows, tt.singlePhysicalBatch)

			if got.EffectiveNBatch() != tt.wantNBatch || got.EffectiveNUBatch() != tt.wantNUBatch {
				t.Errorf("batch = %d/%d, want %d/%d", got.EffectiveNBatch(), got.EffectiveNUBatch(), tt.wantNBatch, tt.wantNUBatch)
			}
		})
	}
}

func TestAdjustDefaultGenerationBatchBySlotCount(t *testing.T) {
	tests := []struct {
		nSeqMax   int
		wantPlain int
		wantMTP   int
	}{
		{nSeqMax: 1, wantPlain: 2049, wantMTP: 2052},
		{nSeqMax: 2, wantPlain: 2050, wantMTP: 2056},
		{nSeqMax: 4, wantPlain: 2052, wantMTP: 2064},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("slots-%d", tt.nSeqMax), func(t *testing.T) {
			plain := adjustConfig(NewConfig(
				WithContextWindow(8192),
				WithNSeqMax(tt.nSeqMax),
			), 0)
			if plain.EffectiveNBatch() != tt.wantPlain || plain.EffectiveNUBatch() != DefaultPrefillBatchSize {
				t.Errorf("plain batch = %d/%d, want %d/%d", plain.EffectiveNBatch(), plain.EffectiveNUBatch(), tt.wantPlain, DefaultPrefillBatchSize)
			}

			mtp := adjustGenerationBatch(plain, 1+defMTPNDraft, true)
			if mtp.EffectiveNBatch() != tt.wantMTP || mtp.EffectiveNUBatch() != tt.wantMTP {
				t.Errorf("MTP batch = %d/%d, want %d/%d", mtp.EffectiveNBatch(), mtp.EffectiveNUBatch(), tt.wantMTP, tt.wantMTP)
			}
		})
	}
}

func TestValidateGenerationBatchCapacity(t *testing.T) {
	tests := []struct {
		name              string
		nBatch            int
		generationReserve int
		wantErr           bool
	}{
		{"capacity available", 2056, 8, false},
		{"exact capacity", 8, 8, false},
		{"insufficient capacity", 7, 8, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGenerationBatchCapacity(tt.nBatch, tt.generationReserve)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGenerationBatchCapacity() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestContextTopologyParamsProvidesFullContextPerSequence(t *testing.T) {
	got := contextTopologyParams(llama.ContextParams{}, 32_768, 4)

	if got.NCtx != 131_072 {
		t.Errorf("NCtx: got %d, want %d", got.NCtx, 131_072)
	}
	if got.NSeqMax != 4 {
		t.Errorf("NSeqMax: got %d, want %d", got.NSeqMax, 4)
	}
	if got.KVUnified != 0 {
		t.Errorf("KVUnified: got %d, want %d", got.KVUnified, 0)
	}
}

func TestContextParamsTraceDistinguishesTotalAndPerSequenceContext(t *testing.T) {
	params := contextTopologyParams(llama.ContextParams{}, 131_072, 2)

	var got string
	log := func(_ context.Context, _ string, args ...any) {
		got = fmt.Sprint(args[1])
	}
	logContextParamsTrace(t.Context(), params, 131_072, log)

	for _, want := range []string{"NCtxTotal[262144]", "NCtxPerSeq[131072]"} {
		if !strings.Contains(got, want) {
			t.Errorf("context params trace: got %q, want it to contain %q", got, want)
		}
	}
}

func TestBatchSeqTokenLimit(t *testing.T) {
	tests := []struct {
		name    string
		nBatch  int
		nUBatch int
		want    int
	}{
		{"physical limit smaller", 8192, 2048, 2048},
		{"logical limit smaller", 1024, 2048, 1024},
		{"equal limits", 2048, 2048, 2048},
		{"logical limit unavailable", 0, 2048, 2048},
		{"physical limit unavailable", 2048, 0, 2048},
		{"both unavailable", 0, 0, 0},
		{"negative limits", -1, -1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := batchSeqTokenLimit(tt.nBatch, tt.nUBatch); got != tt.want {
				t.Errorf("batchSeqTokenLimit: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParamsStringIncludesZeroAndFalseValues(t *testing.T) {
	got := (Params{Grammar: "secret grammar"}).String()
	for _, want := range []string{
		"adaptive_p_decay[0]",
		"frequency_penalty[0]",
		"grammar[true]",
		"include_usage[false]",
		"logprobs[false]",
		"min_p[0]",
		"stream[false]",
		"top_logprobs[0]",
		"xtc_probability[0]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Params.String() missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "secret grammar") {
		t.Errorf("Params.String() exposed grammar contents in %q", got)
	}
}

func TestGGMLTypeString(t *testing.T) {
	tests := []struct {
		typ  GGMLType
		want string
	}{
		{GGMLTypeF32, "f32"},
		{GGMLTypeF16, "f16"},
		{GGMLTypeQ4_0, "q4_0"},
		{GGMLTypeQ4_1, "q4_1"},
		{GGMLTypeQ5_0, "q5_0"},
		{GGMLTypeQ5_1, "q5_1"},
		{GGMLTypeQ8_0, "q8_0"},
		{GGMLTypeBF16, "bf16"},
		{GGMLTypeAuto, "auto"},
		{GGMLType(999), "unknown(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.typ.String(); got != tt.want {
				t.Errorf("GGMLType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseGGMLType(t *testing.T) {
	tests := []struct {
		input   string
		want    GGMLType
		wantErr bool
	}{
		{"f32", GGMLTypeF32, false},
		{"fp32", GGMLTypeF32, false},
		{"F32", GGMLTypeF32, false},
		{"f16", GGMLTypeF16, false},
		{"fp16", GGMLTypeF16, false},
		{"F16", GGMLTypeF16, false},
		{"q4_0", GGMLTypeQ4_0, false},
		{"q4", GGMLTypeQ4_0, false},
		{"Q4_0", GGMLTypeQ4_0, false},
		{"q4_1", GGMLTypeQ4_1, false},
		{"q5_0", GGMLTypeQ5_0, false},
		{"q5", GGMLTypeQ5_0, false},
		{"q5_1", GGMLTypeQ5_1, false},
		{"q8_0", GGMLTypeQ8_0, false},
		{"q8", GGMLTypeQ8_0, false},
		{"Q8_0", GGMLTypeQ8_0, false},
		{"bf16", GGMLTypeBF16, false},
		{"bfloat16", GGMLTypeBF16, false},
		{"BF16", GGMLTypeBF16, false},
		{"auto", GGMLTypeAuto, false},
		{"", GGMLTypeAuto, false},
		{"  auto  ", GGMLTypeAuto, false},
		{"invalid", GGMLTypeAuto, true},
		{"q3", GGMLTypeAuto, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseGGMLType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGGMLType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseGGMLType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadMode(t *testing.T) {
	if got := LoadMode(0); got != LoadModeAuto {
		t.Errorf("LoadMode zero value = %v, want %v", got, LoadModeAuto)
	}
	if got := DerefLoadMode(nil); got != LoadModeAuto {
		t.Errorf("DerefLoadMode(nil) = %v, want %v", got, LoadModeAuto)
	}

	tests := []struct {
		name    string
		input   string
		want    LoadMode
		wantStr string
		wantErr bool
	}{
		{"default", "", LoadModeAuto, "auto", false},
		{"auto", "auto", LoadModeAuto, "auto", false},
		{"mmap", "mmap", LoadModeMMap, "mmap", false},
		{"none", "none", LoadModeNone, "none", false},
		{"mlock", "mlock", LoadModeMLock, "mlock", false},
		{"mmap mlock", "mmap+mlock", LoadModeMMapMLock, "mmap+mlock", false},
		{"direct io", "direct-io", LoadModeDirectIO, "direct-io", false},
		{"dio alias", "dio", LoadModeDirectIO, "direct-io", false},
		{"invalid", "buffered", LoadModeAuto, "auto", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLoadMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseLoadMode() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseLoadMode() = %v, want %v", got, tt.want)
			}
			if got.String() != tt.wantStr {
				t.Errorf("LoadMode.String() = %q, want %q", got.String(), tt.wantStr)
			}
		})
	}
}

func TestLoadModeToYZMAType(t *testing.T) {
	tests := []struct {
		mode LoadMode
		want llama.LoadMode
	}{
		{LoadModeAuto, llama.LoadModeAuto},
		{LoadModeMMap, llama.LoadModeMmap},
		{LoadModeNone, llama.LoadModeNone},
		{LoadModeMLock, llama.LoadModeMlock},
		{LoadModeMMapMLock, llama.LoadModeMmapMlock},
		{LoadModeDirectIO, llama.LoadModeDirectIO},
	}

	for _, tt := range tests {
		if got := tt.mode.ToYZMAType(); got != tt.want {
			t.Errorf("LoadMode(%s).ToYZMAType() = %v, want %v", tt.mode, got, tt.want)
		}
	}
}

func TestDraftModelIsSeparate(t *testing.T) {
	if (DraftModelConfig{NDraft: 4}).IsSeparate() {
		t.Errorf("IsSeparate() = true for config with no model files, want false")
	}
	if !(DraftModelConfig{ModelFiles: []string{"draft.gguf"}}).IsSeparate() {
		t.Errorf("IsSeparate() = false for config with model files, want true")
	}
}

func TestSpeculationMode(t *testing.T) {
	if got := NewConfig().SpeculationMode(); got != SpeculationAuto {
		t.Errorf("default SpeculationMode() = %q, want %q", got, SpeculationAuto)
	}
	if got := NewConfig(WithSpeculationMode(SpeculationDisabled)).SpeculationMode(); got != SpeculationDisabled {
		t.Errorf("SpeculationMode() = %q, want %q", got, SpeculationDisabled)
	}
}

func TestMTPNDraft(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want int
	}{
		{"no draft model uses default", NewConfig(), 3},
		{"separate-GGUF draft ignored for MTP, uses default", NewConfig(
			WithDraftModel(&DraftModelConfig{ModelFiles: []string{"d.gguf"}, NDraft: 9}),
		), 3},
		{"MTP override uses configured value", NewConfig(
			WithDraftModel(&DraftModelConfig{NDraft: 7}),
		), 7},
		{"MTP override with zero falls back to default", NewConfig(
			WithDraftModel(&DraftModelConfig{}),
		), 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mtpNDraft(tt.cfg); got != tt.want {
				t.Errorf("mtpNDraft() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	discardLogger := func(ctx context.Context, msg string, args ...any) {}
	tempDir := t.TempDir()
	adapterFile := filepath.Join(tempDir, "adapter.gguf")
	if err := os.WriteFile(adapterFile, []byte("adapter"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		want    string
		cfg     Config
		wantErr bool
	}{
		{"multi GPU setup is valid", NewConfig(
			WithDevices([]string{"CUDA0", "CUDA1"}),
			WithModelFiles([]string{"dummy.gguf"}),
		), false},
		{"mmap mlock load mode is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithLoadMode(LoadModeMMapMLock),
		), false},
		{"MTP nDraft override (no draft files) is valid even with NSeqMax>1", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithNSeqMax(4),
			WithDraftModel(&DraftModelConfig{NDraft: 6}),
		), false},
		{"MTP nDraft override with zero is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithDraftModel(&DraftModelConfig{}),
		), false},
		{"MTP nDraft override rejects negative ndraft", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithDraftModel(&DraftModelConfig{NDraft: -1}),
		), true},
		{"adapter is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithAdapters([]AdapterConfig{{Path: adapterFile, Scale: 1}}),
		), false},
		{"zero adapter scale is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithAdapters([]AdapterConfig{{Path: adapterFile, Scale: 0}}),
		), false},
		{"relative adapter path is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithAdapters([]AdapterConfig{{Path: "adapter.gguf", Scale: 1}}),
		), true},
		{"negative adapter scale is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithAdapters([]AdapterConfig{{Path: adapterFile, Scale: -1}}),
		), true},
		{"non-finite adapter scale is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithAdapters([]AdapterConfig{{Path: adapterFile, Scale: float32(math.Inf(1))}}),
		), true},
		{"duplicate adapter path is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithAdapters([]AdapterConfig{{Path: adapterFile, Scale: 1}, {Path: adapterFile, Scale: 0.5}}),
		), true},
		{"negative queue depth is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithQueueDepth(-1),
		), true},
		{"negative IMC session capacity is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithIMCSessionCapacity(-1),
		), true},
		{"IMC session capacity below admission is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithNSeqMax(2),
			WithQueueDepth(2),
			WithIMCSessionCapacity(3),
		), true},
		{"IMC session capacity equal to admission is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithNSeqMax(2),
			WithQueueDepth(2),
			WithIMCSessionCapacity(4),
		), false},
		{"embedding ignores generation admission floor", NewConfig(
			WithModelFiles([]string{"Qwen3-Embedding-0.6B-Q8_0.gguf"}),
			WithNSeqMax(4),
			WithQueueDepth(2),
			WithIMCSessionCapacity(4),
		), false},
		{"negative admission timeout is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithAdmissionTimeout(-time.Second),
		), true},
		{"zero prefill batch size is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithPrefillBatchSize(0),
		), true},
		{"negative prefill batch size is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithPrefillBatchSize(-1),
		), true},
		{"quantized V cache with Flash Attention disabled is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithCacheTypeV(GGMLTypeQ8_0),
			WithFlashAttention(FlashAttentionDisabled),
		), true},
		{"quantized V cache with Flash Attention auto is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithCacheTypeV(GGMLTypeQ8_0),
			WithFlashAttention(FlashAttentionAuto),
		), false},
		{"quantized V cache with Flash Attention enabled is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithCacheTypeV(GGMLTypeQ8_0),
			WithFlashAttention(FlashAttentionEnabled),
		), false},
		{"F16 V cache with Flash Attention disabled is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithCacheTypeV(GGMLTypeF16),
			WithFlashAttention(FlashAttentionDisabled),
		), false},
		{"projector device is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithProjDevice("CUDA1"),
		), false},
		{"projector device with explicit GPU offload is valid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithProjDevice("CUDA1"),
			WithProjOnCPU(false),
		), false},
		{"projector device conflicts with CPU placement", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithProjDevice("CUDA1"),
			WithProjOnCPU(true),
		), true},
	}
	{
		for _, tt := range tests {
			t.Run(tt.want, func(t *testing.T) {
				err := validateConfig(context.Background(), tt.cfg, discardLogger)
				if (err != nil) != tt.wantErr {
					t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
			})
		}
	}
}

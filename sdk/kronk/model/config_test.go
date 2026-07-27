package model

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestConfigStringIncludesCompleteDiagnostics(t *testing.T) {
	cfg := Config{
		AutoTune:            true,
		DefaultParams:       Params{Grammar: "secret grammar"},
		PtrAdmissionTimeout: new(5 * time.Minute),
		PtrQueueDepth:       new(7),
	}

	got := cfg.String()
	for _, want := range []string{
		"AutoTune[true]",
		"AdmissionTimeout[5m0s]",
		"DefaultParams[",
		"grammar[true]",
		"QueueDepth[7]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Config.String() missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, cfg.DefaultParams.Grammar) {
		t.Errorf("Config.String() exposed grammar contents in %q", got)
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
	if got := cfg.QueueDepth(); got != defaultQueueDepth {
		t.Errorf("QueueDepth: got %d, want %d", got, defaultQueueDepth)
	}
	if !strings.Contains(cfg.String(), "QueueDepth[2]") {
		t.Errorf("adjusted Config.String() missing effective queue-depth default in %q", cfg.String())
	}
}

func TestAdjustConfigUsesOneBatchDefault(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"text", NewConfig(WithContextWindow(8192), WithNSeqMax(4))},
		{"projection", NewConfig(WithContextWindow(8192), WithNSeqMax(4), WithProjFile("mmproj.gguf"))},
		{"MoE CPU experts", NewConfig(WithContextWindow(8192), WithNSeqMax(4), WithMoE(&MoEConfig{Mode: MoEModeExpertsCPU}))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := adjustConfig(tt.cfg, 0)
			if got := cfg.NUBatch(); got != defNUBatch {
				t.Errorf("NUBatch: got %d, want %d", got, defNUBatch)
			}
			if got, want := cfg.NBatch(), defNUBatch*cfg.NSeqMax(); got != want {
				t.Errorf("NBatch: got %d, want %d", got, want)
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
		"return_prompt[false]",
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
	tests := []struct {
		name    string
		input   string
		want    LoadMode
		wantStr string
		wantErr bool
	}{
		{"default", "", LoadModeMMap, "mmap", false},
		{"mmap", "mmap", LoadModeMMap, "mmap", false},
		{"none", "none", LoadModeNone, "none", false},
		{"mlock", "mlock", LoadModeMLock, "mlock", false},
		{"direct io", "direct-io", LoadModeDirectIO, "direct-io", false},
		{"dio alias", "dio", LoadModeDirectIO, "direct-io", false},
		{"invalid", "buffered", LoadModeMMap, "mmap", true},
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
		{LoadModeMMap, llama.LoadModeMmap},
		{LoadModeNone, llama.LoadModeNone},
		{LoadModeMLock, llama.LoadModeMlock},
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

func TestMTPNDraft(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want int
	}{
		{"no draft model uses default", NewConfig(), defMTPNDraft},
		{"separate-GGUF draft ignored for MTP, uses default", NewConfig(
			WithDraftModel(&DraftModelConfig{ModelFiles: []string{"d.gguf"}, NDraft: 9}),
		), defMTPNDraft},
		{"MTP override uses configured value", NewConfig(
			WithDraftModel(&DraftModelConfig{NDraft: 7}),
		), 7},
		{"MTP override with zero falls back to default", NewConfig(
			WithDraftModel(&DraftModelConfig{}),
		), defMTPNDraft},
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
		{"negative admission timeout is invalid", NewConfig(
			WithModelFiles([]string{"dummy.gguf"}),
			WithAdmissionTimeout(-time.Second),
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

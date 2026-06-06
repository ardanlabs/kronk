package model

import (
	"context"
	"testing"
)

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

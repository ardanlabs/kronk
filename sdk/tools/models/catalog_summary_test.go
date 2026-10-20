package models_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestCatalogModelDimensions(t *testing.T) {
	tests := []struct {
		name         string
		metadata     map[string]string
		wantType     string
		wantSequence string
	}{
		{"dense attention", map[string]string{"general.architecture": "llama"}, "Dense", "Attention-only"},
		{"moe attention", map[string]string{"general.architecture": "qwen3moe", "qwen3moe.expert_count": "128"}, "MoE", "Attention-only"},
		{"dense hybrid", map[string]string{"general.architecture": "qwen35", "qwen35.block_count": "8"}, "Dense", "Hybrid"},
		{"moe hybrid", map[string]string{"general.architecture": "qwen35moe", "qwen35moe.block_count": "8", "qwen35moe.expert_count": "256"}, "MoE", "Hybrid"},
		{"recurrent only", map[string]string{"general.architecture": "mamba"}, "Dense", "Recurrent"},
		{"all recurrent pattern", map[string]string{"general.architecture": "qwen35", "qwen35.block_count": "3", "qwen35.attention.recurrent_layers": "[true,true,true]"}, "Dense", "Recurrent"},
		{"all attention pattern", map[string]string{"general.architecture": "qwen35", "qwen35.block_count": "3", "qwen35.attention.recurrent_layers": "[false,false,false]"}, "Dense", "Attention-only"},
		{"generic explicit hybrid", map[string]string{"general.architecture": "clef", "clef.block_count": "3", "clef.attention.recurrent_layers": "[true,false,true]"}, "Dense", "Hybrid"},
		{"exclude appended mtp", map[string]string{"general.architecture": "clef", "clef.block_count": "3", "clef.nextn_predict_layers": "1", "clef.attention.recurrent_layers": "[true,true,false]"}, "Dense", "Recurrent"},
		{"unknown recurrent topology", map[string]string{"general.architecture": "custom", "custom.ssm.state_size": "16"}, "Dense", "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := modelprofile.Resolve(tt.metadata)
			if got := profile.ParameterActivation(); got != tt.wantType {
				t.Errorf("profile parameter activation: got %q, want %q", got, tt.wantType)
			}
			if got := profile.SequenceProcessing; got != tt.wantSequence {
				t.Errorf("sequence processing: got %q, want %q", got, tt.wantSequence)
			}
			architecture := tt.metadata["general.architecture"]
			entry := models.CatalogEntry{ModelType: tt.wantType, Architecture: architecture, SequenceProcessing: tt.wantSequence}
			summary := models.NewSummary("test/model", entry, nil, nil)
			if summary.ModelType != tt.wantType || summary.Architecture != architecture || summary.SequenceProcessing != tt.wantSequence {
				t.Errorf("summary dimensions: got %q/%q/%q, want %q/%q/%q", summary.ModelType, summary.Architecture, summary.SequenceProcessing, tt.wantType, architecture, tt.wantSequence)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{name: "negative", input: -1, want: ""},
		{name: "zero", input: 0, want: ""},
		{name: "bytes", input: 1023, want: "1023 B"},
		{name: "kibibyte", input: 1 << 10, want: "1 KiB"},
		{name: "mebibyte", input: 1 << 20, want: "1 MiB"},
		{name: "gibibyte", input: 1 << 30, want: "1.00 GiB"},
		{name: "fractional gibibyte", input: 3 << 29, want: "1.50 GiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := models.FormatBytes(tt.input); got != tt.want {
				t.Errorf("FormatBytes: got %q, want %q", got, tt.want)
			}
		})
	}
}

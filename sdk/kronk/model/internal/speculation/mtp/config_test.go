package mtp

import (
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

func TestMetadataHasMTP(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     bool
	}{
		{"no MTP key", map[string]string{"general.architecture": "qwen35moe"}, false},
		{"zero layers", map[string]string{"general.architecture": "qwen35moe", "qwen35moe.nextn_predict_layers": "0"}, false},
		{"positive layers", map[string]string{"general.architecture": "qwen35moe", "qwen35moe.nextn_predict_layers": "1"}, true},
		{"malformed layers", map[string]string{"general.architecture": "qwen35moe", "qwen35moe.nextn_predict_layers": "invalid"}, false},
		{"unsupported architecture", map[string]string{"general.architecture": "cohere2moe", "cohere2moe.nextn_predict_layers": " 2 "}, false},
		{"matching text within unrelated key", map[string]string{"general.architecture": "vendor", "vendor.optional_nextn_predict_layers.count": "3"}, false},
		{"qwen4exp architecture", map[string]string{"general.architecture": "qwen4exp", "qwen4exp.nextn_predict_layers": "1"}, true},
		{"Nemotron architecture", map[string]string{"general.architecture": "nemotron_h_moe", "nemotron_h_moe.nextn_predict_layers": "1"}, true},
		{"glm5-next unsupported runtime", map[string]string{"general.architecture": "glm5-next", "glm5-next.nextn_predict_layers": "1"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MetadataArchitecture(tt.metadata) != modelprofile.MTPArchitectureNone
			if got != tt.want {
				t.Errorf("metadataHasMTP() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestMetadataHasAssistantMTP(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     bool
	}{
		{"assistant with MTP", map[string]string{"general.architecture": "gemma4-assistant", "gemma4.nextn_predict_layers": "1"}, true},
		{"assistant without MTP", map[string]string{"general.architecture": "gemma4-assistant"}, false},
		{"target with MTP", map[string]string{"general.architecture": "gemma4", "gemma4.nextn_predict_layers": "1"}, false},
		{"malformed MTP", map[string]string{"general.architecture": "gemma4-assistant", "gemma4.nextn_predict_layers": "invalid"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MetadataHasAssistant(tt.metadata)
			if got != tt.want {
				t.Errorf("metadataHasAssistantMTP() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestResolveEmbeddedMTPCompatibility(t *testing.T) {
	tests := []struct {
		name            string
		plan            speculation.Plan
		targetWidth     int32
		mtpOutputWidth  int32
		wantSource      speculation.Source
		wantErrContains string
	}{
		{"matching widths preserve embedded MTP", speculation.Plan{Mode: speculation.ModeAuto, Source: speculation.SourceMTP, MTPArchitecture: speculation.MTPArchitectureQwen35OwnKV, MTPArtifact: speculation.MTPArtifactEmbedded, Available: true}, 4096, 4096, speculation.SourceMTP, ""},
		{"automatic mode falls back on mismatch", speculation.Plan{Mode: speculation.ModeAuto, Source: speculation.SourceMTP, MTPArchitecture: speculation.MTPArchitectureQwen35OwnKV, MTPArtifact: speculation.MTPArtifactEmbedded, Available: true}, 4096, 3584, speculation.SourceNone, ""},
		{"explicit MTP rejects mismatch", speculation.Plan{Mode: speculation.ModeMTP, Source: speculation.SourceMTP, MTPArchitecture: speculation.MTPArchitectureQwen35OwnKV, MTPArtifact: speculation.MTPArtifactEmbedded, Available: true}, 4096, 3584, speculation.SourceNone, "output width 3584 does not match target embedding width 4096"},
		{"Qwen4Exp accepts wide output", speculation.Plan{Mode: speculation.ModeAuto, Source: speculation.SourceMTP, MTPArchitecture: speculation.MTPArchitectureQwen4ExpOwnKV, MTPArtifact: speculation.MTPArtifactEmbedded, Available: true}, 4096, 16384, speculation.SourceMTP, ""},
		{"Qwen4Exp rejects non-integral output width", speculation.Plan{Mode: speculation.ModeAuto, Source: speculation.SourceMTP, MTPArchitecture: speculation.MTPArchitectureQwen4ExpOwnKV, MTPArtifact: speculation.MTPArtifactEmbedded, Available: true}, 4096, 15000, speculation.SourceNone, ""},
		{"non-embedded source is unchanged", speculation.Plan{Mode: speculation.ModeAuto, Source: speculation.SourceMTP, MTPArchitecture: speculation.MTPArchitectureGemmaSharedKV, MTPArtifact: speculation.MTPArtifactCompanion, Available: true}, 4096, 3584, speculation.SourceMTP, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveEmbeddedCompatibility(tt.plan, tt.targetWidth, tt.mtpOutputWidth)
			if tt.wantErrContains == "" && err != nil {
				t.Fatalf("resolveEmbeddedMTPCompatibility() error = %v", err)
			}
			if tt.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContains)) {
				t.Fatalf("resolveEmbeddedMTPCompatibility() error = %v, want containing %q", err, tt.wantErrContains)
			}
			if got.Source != tt.wantSource {
				t.Errorf("source = %s, want %s", got.Source, tt.wantSource)
			}
		})
	}
}

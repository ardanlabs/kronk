package model

import (
	"strings"
	"testing"

	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
)

func TestResolveEmbeddedMTPCompatibility(t *testing.T) {
	tests := []struct {
		name            string
		plan            internalspec.Plan
		targetWidth     int32
		mtpOutputWidth  int32
		wantSource      internalspec.Source
		wantErrContains string
	}{
		{"matching widths preserve embedded MTP", internalspec.Plan{Mode: SpeculationAuto, Source: speculationSourceMTP, MTPArchitecture: mtpArchitectureQwen35OwnKV, MTPArtifact: mtpArtifactEmbedded, Available: true}, 4096, 4096, speculationSourceMTP, ""},
		{"automatic mode falls back on mismatch", internalspec.Plan{Mode: SpeculationAuto, Source: speculationSourceMTP, MTPArchitecture: mtpArchitectureQwen35OwnKV, MTPArtifact: mtpArtifactEmbedded, Available: true}, 4096, 3584, speculationSourceNone, ""},
		{"explicit MTP rejects mismatch", internalspec.Plan{Mode: SpeculationMTP, Source: speculationSourceMTP, MTPArchitecture: mtpArchitectureQwen35OwnKV, MTPArtifact: mtpArtifactEmbedded, Available: true}, 4096, 3584, speculationSourceNone, "output width 3584 does not match target embedding width 4096"},
		{"Qwen4Exp accepts wide output", internalspec.Plan{Mode: SpeculationAuto, Source: speculationSourceMTP, MTPArchitecture: mtpArchitectureQwen4ExpOwnKV, MTPArtifact: mtpArtifactEmbedded, Available: true}, 4096, 16384, speculationSourceMTP, ""},
		{"Qwen4Exp rejects non-integral output width", internalspec.Plan{Mode: SpeculationAuto, Source: speculationSourceMTP, MTPArchitecture: mtpArchitectureQwen4ExpOwnKV, MTPArtifact: mtpArtifactEmbedded, Available: true}, 4096, 15000, speculationSourceNone, ""},
		{"non-embedded source is unchanged", internalspec.Plan{Mode: SpeculationAuto, Source: speculationSourceMTP, MTPArchitecture: mtpArchitectureGemmaSharedKV, MTPArtifact: mtpArtifactCompanion, Available: true}, 4096, 3584, speculationSourceMTP, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveEmbeddedMTPCompatibility(tt.plan, tt.targetWidth, tt.mtpOutputWidth)
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

package model

import (
	"strings"
	"testing"

	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
)

func TestResolveEmbeddedMTPCompatibility(t *testing.T) {
	tests := []struct {
		name            string
		plan            speculationPlan
		targetWidth     int32
		mtpOutputWidth  int32
		wantSource      internalspec.Source
		wantErrContains string
	}{
		{"matching widths preserve embedded MTP", speculationPlan{Mode: SpeculationAuto, Source: speculationSourceMTPEmbedded, Available: true}, 4096, 4096, speculationSourceMTPEmbedded, ""},
		{"automatic mode falls back on mismatch", speculationPlan{Mode: SpeculationAuto, Source: speculationSourceMTPEmbedded, Available: true}, 4096, 3584, speculationSourceNone, ""},
		{"explicit MTP rejects mismatch", speculationPlan{Mode: SpeculationMTP, Source: speculationSourceMTPEmbedded, Available: true}, 4096, 3584, speculationSourceNone, "output width 3584 does not match target embedding width 4096"},
		{"non-embedded source is unchanged", speculationPlan{Mode: SpeculationAuto, Source: speculationSourceMTPCompanion, Available: true}, 4096, 3584, speculationSourceMTPCompanion, ""},
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
				t.Errorf("source = %d, want %d", got.Source, tt.wantSource)
			}
		})
	}
}

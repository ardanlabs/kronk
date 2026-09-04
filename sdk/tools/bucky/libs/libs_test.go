package libs

import (
	"testing"

	"github.com/ardanlabs/bucky/pkg/download"
)

func TestDefaultVersionIsPinned(t *testing.T) {
	tag, digest, err := download.ParsePinnedVersion(defaultVersion)
	if err != nil {
		t.Fatalf("parse default version: %v", err)
	}
	if tag != "v1.9.3" {
		t.Errorf("tag: got %q, want %q", tag, "v1.9.3")
	}
	if digest == "" {
		t.Error("digest: got empty value")
	}
}

func TestVersionGreater(t *testing.T) {
	tests := []struct {
		name   string
		v1, v2 string
		want   bool
	}{
		{"equal", "v1.8.6", "v1.8.6", false},
		{"patch greater", "v1.8.6", "v1.8.4", true},
		{"patch lesser", "v1.8.4", "v1.8.6", false},
		{"minor greater", "v1.9.0", "v1.8.6", true},
		{"major greater", "v2.0.0", "v1.8.6", true},
		{"two-digit minor greater", "v1.10.0", "v1.8.6", true},
		{"two-digit minor lesser", "v1.8.6", "v1.10.0", false},
		{"two-digit patch greater", "v1.8.10", "v1.8.6", true},
		{"missing segment lesser", "v1.8", "v1.8.1", false},
		{"missing segment greater", "v1.8.1", "v1.8", true},
		{"missing segment equal", "v1.8", "v1.8.0", false},
		{"no prefix numeric", "1.8.6", "1.8.4", true},
		{"pin ignored when equal", "v1.9.3@sha256:abc", "v1.9.3", false},
		{"pin ignored when greater", "v1.9.4@sha256:abc", "v1.9.3@sha256:def", true},
		{"empty v1", "", "v1.8.6", false},
		{"empty v2", "v1.8.6", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionGreater(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("versionGreater(%q, %q) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestChooseVersionPreservesDefaultPin(t *testing.T) {
	got := chooseVersion("", true, "", "v1.9.3", defaultVersion)
	if got != defaultVersion {
		t.Errorf("chooseVersion: got %q, want %q", got, defaultVersion)
	}
}

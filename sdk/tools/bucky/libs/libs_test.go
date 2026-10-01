package libs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWithValidation(t *testing.T) {
	var options Options
	WithValidation(true)(&options)

	if !options.Validation {
		t.Error("Validation: got false, want true")
	}
}

func TestDownloadAcceptsNilLogger(t *testing.T) {
	root := t.TempDir()
	if err := writeVersionFile(root, defaultVersion, "arm64", "darwin", "metal"); err != nil {
		t.Fatalf("writeVersionFile: %v", err)
	}

	lib := Libs{path: root, readOnly: true}
	if _, err := lib.Download(context.Background(), nil); err != nil {
		t.Fatalf("Download: %v", err)
	}
}

func TestSwapInstallRestoresExistingInstallWhenActivationFails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "cpu")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	existing := filepath.Join(path, "libwhisper.dylib")
	if err := os.WriteFile(existing, []byte("working"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := swapInstall(path, filepath.Join(root, "missing-stage")); err == nil {
		t.Fatal("swapInstall: got nil error, want activation failure")
	}

	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("ReadFile existing install: %v", err)
	}
	if string(data) != "working" {
		t.Errorf("existing install: got %q, want working", data)
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

func TestChooseVersionPreservesAuthenticatedDefault(t *testing.T) {
	got := chooseVersion("", true, "", bareVersion(defaultVersion), defaultVersion)
	if got != defaultVersion {
		t.Errorf("chooseVersion: got %q, want %q", got, defaultVersion)
	}
}

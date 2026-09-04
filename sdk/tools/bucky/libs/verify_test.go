package libs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/bucky/pkg/download"
)

func TestVerifyAllowsKronkVersionMetadata(t *testing.T) {
	libPath := t.TempDir()
	lib, err := New(
		WithLibPath(libPath),
		WithArch("arm64"),
		WithOS("darwin"),
		WithProcessor("metal"),
	)
	if err != nil {
		t.Fatalf("new libs: %v", err)
	}

	const library = "libwhisper.dylib"
	contents := []byte("whisper")
	if err := os.WriteFile(filepath.Join(libPath, library), contents, 0o755); err != nil {
		t.Fatalf("write library: %v", err)
	}
	if err := writeVersionFile(libPath, defaultVersion, "arm64", "darwin", "metal"); err != nil {
		t.Fatalf("write version: %v", err)
	}

	sum := sha256.Sum256(contents)
	record := download.InstallRecord{
		Tag:       "v1.9.3",
		Arch:      "arm64",
		OS:        "darwin",
		Processor: "metal",
		Asset: download.InstallAsset{
			Files: map[string]string{library: hex.EncodeToString(sum[:])},
		},
	}
	if err := download.WriteInstallRecord(libPath, record); err != nil {
		t.Fatalf("write install record: %v", err)
	}

	report, err := lib.Verify(context.Background(), "")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !report.OK() {
		t.Errorf("report: got changed=%d missing=%d unexpected=%d, want all zero", report.Changed, report.Missing, report.Unexpected)
	}
	if len(report.Files) != 1 || report.Files[0].Name != library {
		t.Errorf("files: got %+v, want only %s", report.Files, library)
	}

	if err := os.WriteFile(filepath.Join(libPath, "unexpected.txt"), []byte("unexpected"), 0o644); err != nil {
		t.Fatalf("write unexpected file: %v", err)
	}

	report, err = lib.Verify(context.Background(), "")
	if err != nil {
		t.Fatalf("verify with unexpected file: %v", err)
	}
	if report.OK() || report.Unexpected != 1 {
		t.Errorf("unexpected file report: got ok=%t unexpected=%d, want false/1", report.OK(), report.Unexpected)
	}
}

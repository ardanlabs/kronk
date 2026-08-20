package defaults

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogFilePreservesExistingCatalog(t *testing.T) {
	basePath := t.TempDir()
	catalogDir := filepath.Join(basePath, catalogDirName)
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("MkdirAll: unexpected error: %v", err)
	}

	want := []byte("models:\n  example/custom-model:\n    provider: example\n")
	target := filepath.Join(catalogDir, catalogFileName)
	if err := os.WriteFile(target, want, 0644); err != nil {
		t.Fatalf("WriteFile: unexpected error: %v", err)
	}

	gotPath, err := CatalogFile("", basePath)
	if err != nil {
		t.Fatalf("CatalogFile: unexpected error: %v", err)
	}
	if gotPath != target {
		t.Errorf("CatalogFile path: got %q, want %q", gotPath, target)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: unexpected error: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("catalog contents: got %q, want %q", got, want)
	}
}

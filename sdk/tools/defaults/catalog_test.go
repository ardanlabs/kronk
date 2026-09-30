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

func TestCatalogFileDoesNotMoveRootFile(t *testing.T) {
	basePath := t.TempDir()
	rootFile := filepath.Join(basePath, catalogFileName)
	wantRoot := []byte("models:\n  example/old-location: {}\n")
	if err := os.WriteFile(rootFile, wantRoot, 0644); err != nil {
		t.Fatalf("WriteFile root catalog: %v", err)
	}

	target, err := CatalogFile("", basePath)
	if err != nil {
		t.Fatalf("CatalogFile: %v", err)
	}
	if target != filepath.Join(basePath, catalogDirName, catalogFileName) {
		t.Errorf("CatalogFile path: got %q, want canonical path", target)
	}
	if got, err := os.ReadFile(rootFile); err != nil || string(got) != string(wantRoot) {
		t.Errorf("root catalog: got %q, %v; want unchanged", got, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) == string(wantRoot) {
		t.Errorf("canonical catalog: got root contents %q, %v; want embedded default", got, err)
	}
}

func TestModelConfigFileDoesNotMoveRootFile(t *testing.T) {
	basePath := t.TempDir()
	rootFile := filepath.Join(basePath, modelConfigFileName)
	wantRoot := []byte("owner/old-model: {}\n")
	if err := os.WriteFile(rootFile, wantRoot, 0644); err != nil {
		t.Fatalf("WriteFile root model config: %v", err)
	}

	target, err := ModelConfigFile("", basePath)
	if err != nil {
		t.Fatalf("ModelConfigFile: %v", err)
	}
	if target != filepath.Join(basePath, modelsDirName, modelConfigFileName) {
		t.Errorf("ModelConfigFile path: got %q, want canonical path", target)
	}
	if got, err := os.ReadFile(rootFile); err != nil || string(got) != string(wantRoot) {
		t.Errorf("root model config: got %q, %v; want unchanged", got, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) == string(wantRoot) {
		t.Errorf("canonical model config: got root contents %q, %v; want embedded default", got, err)
	}
}

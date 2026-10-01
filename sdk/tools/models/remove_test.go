package models

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemove_ErrorOnFailedDelete(t *testing.T) {
	m := newTestModels(t)

	mp := Path{
		ModelFiles: []string{filepath.Join(t.TempDir(), "does-not-exist.gguf")},
	}

	if err := m.Remove(mp, testLog); err == nil {
		t.Fatal("Remove returned nil error after a failed deletion; want non-nil error")
	}
}

func TestRemoveAllowsMissingSHAPointers(t *testing.T) {
	m := newTestModels(t)
	dir := filepath.Join(m.Path(), "provider", "family")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	mp := Path{
		ModelFiles: []string{filepath.Join(dir, "model.gguf")},
		ProjFile:   filepath.Join(dir, "mmproj-model.gguf"),
		MTPFile:    filepath.Join(dir, "mtp-model.gguf"),
	}
	for _, file := range []string{mp.ModelFiles[0], mp.ProjFile, mp.MTPFile} {
		if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", file, err)
		}
	}

	if err := m.Remove(mp, testLog); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

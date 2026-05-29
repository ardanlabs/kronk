package models

import (
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

package models

import (
	"errors"
	"testing"
)

func TestFullPathModelNotFound(t *testing.T) {
	models, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	_, err = models.FullPath("unknown")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("FullPath error: got %v, want ErrModelNotFound", err)
	}
}

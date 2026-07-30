package models

import (
	"errors"
	"testing"
)

func TestFullPathNotFound(t *testing.T) {
	var m Models

	_, err := m.FullPath("missing")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("FullPath: got %v, want %v", err, ErrModelNotFound)
	}
}

func TestCatalogHeaderNotFound(t *testing.T) {
	var m Models

	_, err := m.CatalogHeader(t.Context(), "missing")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("CatalogHeader: got %v, want %v", err, ErrModelNotFound)
	}
}

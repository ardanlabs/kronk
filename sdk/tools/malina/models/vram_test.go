package models

import (
	"errors"
	"testing"

	malinavram "github.com/ardanlabs/kronk/sdk/malina/vram"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
)

func TestCalculateVRAM(t *testing.T) {
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths() error = %v", err)
	}

	const (
		diffusionSize = int64(2_500_000_000)
		vaeSize       = int64(335_000_000)
		llmSize       = int64(2_500_000_000)
		contexts      = int64(2)
	)
	m.index[BundleFlux2Klein4B.String()] = backend.ModelPath{
		FileSizes: []int64{diffusionSize, vaeSize, llmSize},
	}

	got, err := m.CalculateVRAM(BundleFlux2Klein4B.String(), malinavram.Config{Contexts: contexts})
	if err != nil {
		t.Fatalf("CalculateVRAM() error = %v", err)
	}

	wantModel := (diffusionSize + vaeSize + llmSize) * contexts
	if got.ModelBytes != wantModel {
		t.Errorf("ModelBytes: got %d, want %d", got.ModelBytes, wantModel)
	}
	wantTotal := wantModel + malinavram.RuntimeOverhead*contexts
	if got.TotalVRAM != wantTotal {
		t.Errorf("TotalVRAM: got %d, want %d", got.TotalVRAM, wantTotal)
	}
}

func TestCalculateVRAMRejectsMissingModel(t *testing.T) {
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths() error = %v", err)
	}

	_, err = m.CalculateVRAM(BundleSD15.String(), malinavram.Config{})
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("CalculateVRAM() error = %v, want ErrModelNotFound", err)
	}
}

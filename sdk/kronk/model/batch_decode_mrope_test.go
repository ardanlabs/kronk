package model

import (
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestFillMRoPETextPositions(t *testing.T) {
	positions := make([]llama.Pos, 12)
	fillMRoPETextPositions(positions, 3, 7)

	want := []llama.Pos{
		7, 8, 9,
		7, 8, 9,
		7, 8, 9,
		7, 8, 9,
	}
	if !slices.Equal(positions, want) {
		t.Errorf("positions = %v, want %v", positions, want)
	}
}

func TestFillMRoPEImagePositions(t *testing.T) {
	positions := make([]llama.Pos, 24)
	fillMRoPEImagePositions(positions, 6, 3, 2, 10)

	want := []llama.Pos{
		10, 10, 10, 10, 10, 10,
		10, 10, 10, 11, 11, 11,
		10, 11, 12, 10, 11, 12,
		0, 0, 0, 0, 0, 0,
	}
	if !slices.Equal(positions, want) {
		t.Errorf("positions = %v, want %v", positions, want)
	}
}

func TestIMCSessionLogicalPosition(t *testing.T) {
	tests := []struct {
		name    string
		session imcSession
		want    int
	}{
		{name: "linear uses physical count", session: imcSession{totalTokensCached: 12}, want: 12},
		{name: "mrope uses logical position", session: imcSession{totalTokensCached: 12, nextLogicalPos: 5}, want: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.session.logicalPosition(); got != tt.want {
				t.Errorf("logicalPosition() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMRoPERejectsNonRectangularLayout(t *testing.T) {
	engine := batchEngine{}
	if err := engine.decodeEmbeddingsMRoPE(&slot{}, nil, 0, 5, 2, 2); err == nil {
		t.Fatal("decodeEmbeddingsMRoPE() error = nil, want unsupported layout error")
	}

	m := Model{}
	if _, err := m.decodeEmbeddingsMRoPEIntoCache(nil, 0, 5, 2, 2, 0, 0, false); err == nil {
		t.Fatal("decodeEmbeddingsMRoPEIntoCache() error = nil, want unsupported layout error")
	}
}

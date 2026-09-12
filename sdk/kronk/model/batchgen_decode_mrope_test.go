package model

import (
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// TestFillMRoPETextPositions verifies generation-batch M-RoPE positions.
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

func TestMRoPERejectsWrongPositionCount(t *testing.T) {
	engine := batchEngine{}
	if err := engine.decodeEmbeddingsMRoPE(&slot{}, nil, 0, 5, make([]llama.Pos, 19), 0); err == nil {
		t.Fatal("decodeEmbeddingsMRoPE() error = nil, want position count error")
	}

	m := Model{}
	if _, err := m.decodeEmbeddingsMRoPEIntoCache(nil, 0, 5, make([]llama.Pos, 19), 0, false); err == nil {
		t.Fatal("decodeEmbeddingsMRoPEIntoCache() error = nil, want position count error")
	}
}

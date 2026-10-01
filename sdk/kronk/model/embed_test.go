package model

import (
	"math"
	"slices"
	"testing"
)

func TestEmbeddingVector(t *testing.T) {
	raw := []float32{3, 4, 12}

	got := embeddingVector(raw, 2)
	want := []float32{0.6, 0.8}
	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(float64(got[i]-want[i])) > 1e-6 {
			t.Errorf("embedding[%d]: got %f, want %f", i, got[i], want[i])
		}
	}

	if !slices.Equal(raw, []float32{3, 4, 12}) {
		t.Errorf("raw vector: got %v, want [3 4 12]", raw)
	}
}

func TestEmbeddingsRejectsOperationDuringUnload(t *testing.T) {
	m := Model{modelInfo: ModelInfo{IsEmbedModel: true}}
	m.unloaded.Store(true)

	if _, err := m.Embeddings(t.Context(), D{"input": "hello"}); err == nil {
		t.Fatal("Embeddings() error = nil, want unloading error")
	}
	if got := m.activeStreams.Load(); got != 0 {
		t.Errorf("active streams: got %d, want 0", got)
	}
}

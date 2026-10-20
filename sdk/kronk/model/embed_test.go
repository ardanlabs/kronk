package model

import (
	"context"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Supply an embedding GGUF with different hidden/output widths to exercise
// projected embeddings. The same test also covers ordinary equal-width models.
func TestEmbeddingsOutputWidth(t *testing.T) {
	modelPath := os.Getenv("KRONK_TEST_EMBED_MODEL")
	if modelPath == "" {
		t.Skip("KRONK_TEST_EMBED_MODEL is required")
	}
	if err := llama.Load(libs.Path(os.Getenv("KRONK_TEST_LIB_PATH"))); err != nil {
		t.Fatalf("load llama.cpp: %v", err)
	}
	if err := llama.Init(); err != nil {
		t.Fatalf("initialize llama.cpp: %v", err)
	}
	llama.LogSet(llama.LogSilent())

	m, err := NewModel(t.Context(), NewConfig(
		WithModelFiles([]string{modelPath}), WithNGpuLayers(0),
		WithContextWindow(256), WithPrefillBatchSize(64),
		WithPhysicalBatchSize(64), WithNSeqMax(2),
		WithFlashAttention(FlashAttentionDisabled),
	))
	if err != nil {
		t.Fatalf("load embedding model: %v", err)
	}
	t.Cleanup(func() {
		if err := m.Unload(context.Background()); err != nil {
			t.Errorf("unload embedding model: %v", err)
		}
	})
	wantDim := int(llama.ModelNEmbdOut(m.model))
	if wantDim < 2 {
		t.Fatalf("output width: got %d, want at least 2", wantDim)
	}
	t.Logf("hidden width %d, output width %d", llama.ModelNEmbd(m.model), wantDim)

	// Reject this before inference: accepting hidden width here could allow
	// reading past a smaller projected native output. Cancel the context so
	// a regression cannot reach an unsafe native read.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = m.Embeddings(ctx, D{"input": "search_query: ocean", "dimensions": float64(wantDim + 1)})
	if err == nil || !strings.Contains(err.Error(), "dimensions but model only has") {
		t.Fatalf("oversized dimensions: got %v, want dimension-limit error", err)
	}

	inputs := []string{"search_query: ocean", "search_query: mountain trails in autumn"}
	full, err := m.Embeddings(t.Context(), D{"input": inputs})
	if err != nil {
		t.Fatalf("full embeddings: %v", err)
	}
	reduced, err := m.Embeddings(t.Context(), D{"input": inputs, "dimensions": float64(wantDim - 1)})
	if err != nil {
		t.Fatalf("reduced embeddings: %v", err)
	}
	if len(full.Data) != len(inputs) || len(reduced.Data) != len(inputs) {
		t.Fatalf("response counts: got %d and %d, want %d", len(full.Data), len(reduced.Data), len(inputs))
	}
	for i := range inputs {
		if len(full.Data[i].Embedding) != wantDim || len(reduced.Data[i].Embedding) != wantDim-1 {
			t.Fatalf("input[%d] dimensions: got %d and %d, want %d and %d", i,
				len(full.Data[i].Embedding), len(reduced.Data[i].Embedding), wantDim, wantDim-1)
		}
		var fullNorm, prefixNorm float64
		for j, v := range full.Data[i].Embedding {
			fullNorm += float64(v) * float64(v)
			if j < wantDim-1 {
				prefixNorm += float64(v) * float64(v)
			}
		}
		if math.IsNaN(fullNorm) || math.Abs(fullNorm-1) > 1e-5 {
			t.Fatalf("input[%d] squared norm: got %v, want 1", i, fullNorm)
		}
		for j, v := range reduced.Data[i].Embedding {
			want := float64(full.Data[i].Embedding[j]) / math.Sqrt(prefixNorm)
			if math.IsNaN(float64(v)) || math.Abs(float64(v)-want) > 1e-5 {
				t.Errorf("input[%d] reduced[%d]: got %v, want %v", i, j, v, want)
			}
		}
	}
}

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

package model

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestDecisionSharedLen(t *testing.T) {
	work := []decisionWork{
		{tokens: []llama.Token{1, 2, 3, 4, 5}, prefixLen: 4},
		{tokens: []llama.Token{1, 2, 9, 4, 5}, prefixLen: 4},
		{tokens: []llama.Token{1, 2, 3, 4, 6}, prefixLen: 3},
	}
	if got := decisionSharedLen(work); got != 2 {
		t.Fatalf("shared length: got %d, want 2", got)
	}
}

func TestDecisionContextParams(t *testing.T) {
	cfg := NewConfig(WithContextWindow(16_384))
	cfg.nUBatch = 768
	base := llama.ContextParams{TypeK: llama.GGMLTypeQ8_0}

	got := decisionContextParams(base, cfg)
	if got.NCtx != 16_384 || got.NBatch != 16_384 || got.NUbatch != 768 {
		t.Fatalf("batch dimensions: got ctx=%d batch=%d ubatch=%d", got.NCtx, got.NBatch, got.NUbatch)
	}
	if got.NSeqMax != 2 || got.KVUnified != 1 {
		t.Fatalf("topology: got sequences=%d unified=%d", got.NSeqMax, got.KVUnified)
	}
	if got.NOutputsMax != decisionMaxReadouts || got.NOutputsMaxPerSeq != decisionMaxReadouts {
		t.Fatalf("outputs: got max=%d per-sequence=%d", got.NOutputsMax, got.NOutputsMaxPerSeq)
	}
	if got.TypeK != llama.GGMLTypeQ8_0 {
		t.Fatalf("cache type: got %v, want q8_0", got.TypeK)
	}
}

package model

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestDecisionContextParams(t *testing.T) {
	cfg := NewConfig(WithContextWindow(16_384), WithNSeqMax(4))
	cfg.nUBatch = 768
	base := llama.ContextParams{TypeK: llama.GGMLTypeQ8_0}

	got := decisionContextParams(base, cfg)
	if got.NCtx != 65_536 || got.NBatch != 16_384 || got.NUbatch != 768 {
		t.Fatalf("batch dimensions: got ctx=%d batch=%d ubatch=%d", got.NCtx, got.NBatch, got.NUbatch)
	}
	if got.NSeqMax != 4 || got.KVUnified != 1 {
		t.Fatalf("topology: got sequences=%d unified=%d", got.NSeqMax, got.KVUnified)
	}
	if got.NOutputsMax != decisionMaxReadouts || got.NOutputsMaxPerSeq != decisionMaxReadouts {
		t.Fatalf("outputs: got max=%d per-sequence=%d", got.NOutputsMax, got.NOutputsMaxPerSeq)
	}
	if got.TypeK != llama.GGMLTypeQ8_0 {
		t.Fatalf("cache type: got %v, want q8_0", got.TypeK)
	}
}

package model

import (
	"slices"
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

func TestStageDecisionParts(t *testing.T) {
	batch := extendedBatch{capacity: 5}
	if _, err := batch.addToken(99, 99, []llama.SeqId{99}, extendedBatchOutputLogits); err != nil {
		t.Fatalf("seed batch: %v", err)
	}

	parts := []decisionPart{
		{
			tokens:   []llama.Token{11, 12, 13},
			position: 4,
			sequence: 2,
			readouts: []decisionReadout{{position: 4}, {position: 6}},
		},
		{
			tokens:   []llama.Token{21, 22},
			position: 9,
			sequence: 7,
			readouts: []decisionReadout{{position: 10}},
		},
	}

	indices, err := stageDecisionParts(&batch, parts)
	if err != nil {
		t.Fatalf("stage decision parts: %v", err)
	}

	wantIndices := [][]int32{{0, 2}, {4}}
	wantTokens := []llama.Token{11, 12, 13, 21, 22}
	wantPositions := []llama.Pos{4, 5, 6, 9, 10}
	wantSequences := []llama.SeqId{2, 2, 2, 7, 7}
	wantOutputs := []extendedBatchOutput{
		extendedBatchOutputLogits,
		extendedBatchOutputNone,
		extendedBatchOutputLogits,
		extendedBatchOutputNone,
		extendedBatchOutputLogits,
	}

	if !slices.Equal(indices[0], wantIndices[0]) || !slices.Equal(indices[1], wantIndices[1]) {
		t.Fatalf("readout indices: got %v, want %v", indices, wantIndices)
	}
	if len(batch.entries) != len(wantTokens) {
		t.Fatalf("entry count: got %d, want %d", len(batch.entries), len(wantTokens))
	}
	for i, entry := range batch.entries {
		if entry.token != wantTokens[i] {
			t.Errorf("entry[%d] token: got %d, want %d", i, entry.token, wantTokens[i])
		}
		if entry.positionCount != 1 || entry.positions[0] != wantPositions[i] {
			t.Errorf("entry[%d] position: got %v, want [%d]", i, entry.positions[:entry.positionCount], wantPositions[i])
		}
		if entry.sequenceID != wantSequences[i] {
			t.Errorf("entry[%d] sequence: got %d, want %d", i, entry.sequenceID, wantSequences[i])
		}
		if entry.output != wantOutputs[i] {
			t.Errorf("entry[%d] output: got %d, want %d", i, entry.output, wantOutputs[i])
		}
	}
}

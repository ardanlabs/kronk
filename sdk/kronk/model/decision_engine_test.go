package model

import (
	"errors"
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

func TestDecisionContextParamsWholePrompt(t *testing.T) {
	for _, protocol := range []DecisionProtocol{DecisionProtocolVllmD3, DecisionProtocolVllmDecision2} {
		t.Run(protocol.String(), func(t *testing.T) {
			cfg := NewConfig(WithContextWindow(4096), WithNSeqMax(4), WithPrefillBatchSize(768), WithDecisionProtocol(protocol))
			got := decisionContextParams(llama.ContextParams{}, cfg)
			if got.NBatch != 4096 || got.NUbatch != 4096 {
				t.Errorf("whole prompt: got batch=%d ubatch=%d, want 4096/4096", got.NBatch, got.NUbatch)
			}
			if protocol == DecisionProtocolVllmD3 {
				if got.NSeqMax != 1 || got.NCtx != 4096 || got.AttentionType != llama.AttentionTypeNonCausal || got.Embeddings != 0 {
					t.Errorf("D3 isolation: got %+v", got)
				}
			} else if got.Embeddings != 1 || got.NOutputsMax != 4096 || got.NOutputsMaxPerSeq != 4096 {
				t.Errorf("decision2 all-token outputs: got %+v", got)
			}
		})
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
			readouts: []decisionReadout{{position: 10, embeddingWidth: 6}},
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
		extendedBatchOutputEmbeddings,
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
			t.Errorf("entry[%d] output: got %d, want %d", i, entry.output.value, wantOutputs[i].value)
		}
	}
}

func TestDecisionContextParamsEmbeddingProtocol(t *testing.T) {
	tests := []struct {
		name      string
		protocol  DecisionProtocol
		prefill   int
		wantBatch uint32
	}{
		{"Laya default", DecisionProtocolLaya, 0, uint32(DefaultPrefillBatchSize)},
		{"Kev configured", DecisionProtocolKev, 768, 768},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := []Option{
				WithContextWindow(4096),
				WithDecisionProtocol(tt.protocol),
			}
			if tt.prefill > 0 {
				options = append(options, WithPrefillBatchSize(tt.prefill))
			}
			cfg := adjustConfig(NewConfig(options...), 0)

			got := decisionContextParams(llama.ContextParams{}, cfg)
			if got.Embeddings != 1 {
				t.Errorf("Embeddings: got %d, want 1", got.Embeddings)
			}
			if got.PoolingType != llama.PoolingTypeNone {
				t.Errorf("PoolingType: got %d, want %d", got.PoolingType, llama.PoolingTypeNone)
			}
			if got.NBatch != tt.wantBatch || got.NUbatch != tt.wantBatch {
				t.Errorf("embedding batch dimensions: got batch=%d ubatch=%d, want %d", got.NBatch, got.NUbatch, tt.wantBatch)
			}
		})
	}
}

func TestDecisionContextParamsClefOutputsEveryToken(t *testing.T) {
	cfg := NewConfig(
		WithContextWindow(4096),
		WithDecisionProtocol(DecisionProtocolClef),
	)
	cfg.nUBatch = 768

	got := decisionContextParams(llama.ContextParams{}, cfg)
	if got.NBatch != 768 || got.NOutputsMax != 768 || got.NOutputsMaxPerSeq != 768 {
		t.Fatalf("Clef dimensions: got batch=%d outputs=%d per-sequence=%d, want 768", got.NBatch, got.NOutputsMax, got.NOutputsMaxPerSeq)
	}
}

func TestStageDecisionJointPart(t *testing.T) {
	batch := extendedBatch{capacity: 3}
	part := decisionPart{
		tokens:        []llama.Token{11, 12, 13},
		sequence:      2,
		decisionOrder: []llama.DecisionOrder{llama.DecisionOrderQuestionChoice, llama.DecisionOrderQuestionChoice, llama.DecisionOrderOption},
		jointScores:   2,
	}

	indices, err := stageDecisionParts(&batch, []decisionPart{part})
	if err != nil {
		t.Fatalf("stage decision joint part: %v", err)
	}
	if !slices.Equal(indices[0], []int32{0, 1, 2}) {
		t.Fatalf("output indices: got %v, want [0 1 2]", indices[0])
	}
	for i, entry := range batch.entries {
		if entry.output != extendedBatchOutputEmbeddings {
			t.Errorf("entry[%d] output: got %d, want embeddings", i, entry.output.value)
		}
		if entry.decisionOrder != part.decisionOrder[i] {
			t.Errorf("entry[%d] decision order: got %d, want %d", i, entry.decisionOrder, part.decisionOrder[i])
		}
	}
}

func TestStageDecisionMediaPart(t *testing.T) {
	batch := extendedBatch{capacity: 5}
	part := decisionPart{
		prefixRows: []extendedBatchEntry{
			{hasToken: true, token: 41, positions: [4]llama.Pos{0}, positionCount: 1},
			{embedding: []float32{1, 2}, embdWidth: 2, positions: [4]llama.Pos{1, 3, 5, 7}, positionCount: 4},
			{embedding: []float32{8, 9}, embdWidth: 2, positions: [4]llama.Pos{1, 4, 6, 7}, positionCount: 4},
		},
		// Three physical prefix rows advance only two logical positions.
		position: 2, sequence: 7, tokens: []llama.Token{42, 43},
		decisionOrder: []llama.DecisionOrder{llama.DecisionOrderQuestionNoul, llama.DecisionOrderOption},
		jointScores:   1,
	}
	indices, err := stageDecisionParts(&batch, []decisionPart{part})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(indices[0], []int32{0, 1, 2, 3, 4}) {
		t.Fatalf("outputs: got %v, want all five rows", indices)
	}
	for i, row := range batch.entries {
		if row.sequenceID != 7 || row.output != extendedBatchOutputEmbeddings {
			t.Errorf("row[%d]: got sequence=%d output=%v, want 7/embeddings", i, row.sequenceID, row.output)
		}
		if i < 3 {
			if row.decisionOrder != llama.DecisionOrderNone || row.positions != part.prefixRows[i].positions {
				t.Errorf("prefix[%d]: got order=%v positions=%v", i, row.decisionOrder, row.positions)
			}
		} else if row.positions[0] != llama.Pos(i-1) || row.decisionOrder != part.decisionOrder[i-3] {
			t.Errorf("text[%d]: got position=%d order=%v", i, row.positions[0], row.decisionOrder)
		}
	}
	part.prefixRows[1].embedding[0] = 99
	if !slices.Equal(batch.entries[1].embedding, []float32{1, 2}) {
		t.Fatalf("staged embeddings: got %v, want owned [1 2]", batch.entries[1].embedding)
	}
}

func TestDecisionMediaBudget(t *testing.T) {
	engine := decisionEngine{contextWindow: 10, maxTokens: 5}
	work := decisionWork{
		tokens: []llama.Token{1, 2}, prefixRows: make([]extendedBatchEntry, 3),
		decisionOrder: []llama.DecisionOrder{llama.DecisionOrderQuestionNoul, llama.DecisionOrderOption}, jointScores: 1,
	}
	if err := engine.validate([]decisionWork{work}); err != nil {
		t.Fatalf("exact fit: %v", err)
	}
	work.prefixRows = append(work.prefixRows, extendedBatchEntry{})
	if err := engine.validate([]decisionWork{work}); !errors.Is(err, ErrDecisionBudget) {
		t.Fatalf("one row over: got %v, want ErrDecisionBudget", err)
	}
}

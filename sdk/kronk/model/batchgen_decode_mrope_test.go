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

func TestLinearMRoPEPositions(t *testing.T) {
	positions := linearMRoPEPositions(3, 7)

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

func TestStageMRoPEText(t *testing.T) {
	batch := extendedBatch{capacity: 3}
	tokens := []llama.Token{11, 22, 33}
	sequenceIDs := []llama.SeqId{4, 7}

	if err := stageMRoPEText(&batch, tokens, 9, sequenceIDs, extendedBatchOutputLogits); err != nil {
		t.Fatalf("stage M-RoPE text: %v", err)
	}

	for i, entry := range batch.entries {
		position := llama.Pos(9 + i)
		wantPositions := []llama.Pos{position}
		if entry.token != tokens[i] || !entry.hasToken {
			t.Errorf("entry %d token = %d, %t; want %d, true", i, entry.token, entry.hasToken, tokens[i])
		}
		if !slices.Equal(entry.positions[:entry.positionCount], wantPositions) {
			t.Errorf("entry %d positions = %v, want %v", i, entry.positions[:entry.positionCount], wantPositions)
		}
		if entry.sequenceID != 4 || !slices.Equal(entry.extraSequenceIDs, []llama.SeqId{7}) {
			t.Errorf("entry %d sequence IDs = %d + %v, want 4 + [7]", i, entry.sequenceID, entry.extraSequenceIDs)
		}

		wantOutput := extendedBatchOutputNone
		if i == len(tokens)-1 {
			wantOutput = extendedBatchOutputLogits
		}
		if entry.output != wantOutput {
			t.Errorf("entry %d output = %d, want %d", i, entry.output, wantOutput)
		}
	}
}

func TestStageEmbeddingRows(t *testing.T) {
	batch := extendedBatch{capacity: 3}
	embeddings := []float32{
		1, 2,
		3, 4,
		5, 6,
	}
	positions := []llama.Pos{
		10, 11, 12,
		20, 21, 22,
		30, 31, 32,
		40, 41, 42,
	}

	if err := stageEmbeddingRows(&batch, embeddings, 2, 3, positions, []llama.SeqId{5}, extendedBatchOutputLogits); err != nil {
		t.Fatalf("stage embedding rows: %v", err)
	}

	wantEmbeddings := [][]float32{{1, 2}, {3, 4}, {5, 6}}
	wantPositions := [][]llama.Pos{{10, 20, 30, 40}, {11, 21, 31, 41}, {12, 22, 32, 42}}
	for i, entry := range batch.entries {
		if !slices.Equal(entry.embedding, wantEmbeddings[i]) {
			t.Errorf("entry %d embedding = %v, want %v", i, entry.embedding, wantEmbeddings[i])
		}
		if !slices.Equal(entry.positions[:entry.positionCount], wantPositions[i]) {
			t.Errorf("entry %d positions = %v, want %v", i, entry.positions[:entry.positionCount], wantPositions[i])
		}
		if entry.sequenceID != 5 {
			t.Errorf("entry %d sequence ID = %d, want 5", i, entry.sequenceID)
		}

		wantOutput := extendedBatchOutputNone
		if i == len(batch.entries)-1 {
			wantOutput = extendedBatchOutputLogits
		}
		if entry.output != wantOutput {
			t.Errorf("entry %d output = %d, want %d", i, entry.output, wantOutput)
		}
	}

	embeddings[0] = 99
	positions[0] = 99
	if batch.entries[0].embedding[0] != 1 || batch.entries[0].positions[0] != 10 {
		t.Fatal("staged embedding row retained mutable input")
	}
}

func TestStageEmbeddingRowsWithoutOutput(t *testing.T) {
	batch := extendedBatch{capacity: 2}
	if err := stageEmbeddingRows(&batch, []float32{1, 2}, 1, 2, []llama.Pos{7, 8}, []llama.SeqId{0}, extendedBatchOutputNone); err != nil {
		t.Fatalf("stage embedding rows: %v", err)
	}

	for i, entry := range batch.entries {
		if entry.output != extendedBatchOutputNone {
			t.Errorf("entry %d output = %d, want none", i, entry.output)
		}
	}
}

func TestAppendEmbeddingRowsPreservesExistingTokens(t *testing.T) {
	batch := extendedBatch{capacity: 3}
	if _, err := batch.addToken(7, 3, []llama.SeqId{0}, extendedBatchOutputLogits); err != nil {
		t.Fatalf("add token: %v", err)
	}
	if err := appendEmbeddingRows(&batch, []float32{1, 2, 3, 4}, 2, 2, []llama.Pos{4, 5}, []llama.SeqId{1}, extendedBatchOutputLogits); err != nil {
		t.Fatalf("append embedding rows: %v", err)
	}

	if len(batch.entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(batch.entries))
	}
	if !batch.entries[0].hasToken || batch.entries[0].token != 7 {
		t.Errorf("first entry = %+v, want original token 7", batch.entries[0])
	}
	if batch.entries[1].hasToken || batch.entries[2].hasToken {
		t.Fatal("embedding entries unexpectedly contain token IDs")
	}
	if !slices.Equal(batch.entries[1].embedding, []float32{1, 2}) || !slices.Equal(batch.entries[2].embedding, []float32{3, 4}) {
		t.Errorf("embedding rows = %v/%v, want [1 2]/[3 4]", batch.entries[1].embedding, batch.entries[2].embedding)
	}
}

func TestStageEmbeddingRowsRejectsInvalidDimensions(t *testing.T) {
	tests := []struct {
		name       string
		embeddings []float32
		nEmbd      int32
		nTokens    int32
		positions  []llama.Pos
	}{
		{name: "missing embedding value", embeddings: []float32{1, 2, 3}, nEmbd: 2, nTokens: 2, positions: []llama.Pos{0, 1}},
		{name: "ragged positions", embeddings: []float32{1, 2}, nEmbd: 1, nTokens: 2, positions: []llama.Pos{0, 1, 2}},
		{name: "too many position planes", embeddings: []float32{1}, nEmbd: 1, nTokens: 1, positions: []llama.Pos{0, 1, 2, 3, 4}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := extendedBatch{capacity: 2}
			if err := stageEmbeddingRows(&batch, tt.embeddings, tt.nEmbd, tt.nTokens, tt.positions, []llama.SeqId{0}, extendedBatchOutputNone); err == nil {
				t.Fatal("error = nil, want invalid dimensions error")
			}
		})
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

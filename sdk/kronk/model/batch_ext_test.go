package model

import (
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestExtendedBatchTracksEntriesAndRollback(t *testing.T) {
	batch := extendedBatch{capacity: 4}
	sequenceIDs := []llama.SeqId{2, 5}

	first, err := batch.addToken(11, 7, sequenceIDs, extendedBatchOutputNone)
	if err != nil {
		t.Fatalf("add first token: %v", err)
	}
	second, err := batch.addToken(22, 8, []llama.SeqId{3}, extendedBatchOutputLogits)
	if err != nil {
		t.Fatalf("add second token: %v", err)
	}
	if first != 0 || second != 1 {
		t.Fatalf("indices = (%d, %d), want (0, 1)", first, second)
	}

	sequenceIDs[0] = 99
	entry := batch.entries[0]
	if entry.sequenceID != 2 || !slices.Equal(entry.extraSequenceIDs, []llama.SeqId{5}) {
		t.Fatalf("stored sequence IDs = %d + %v, want 2 + [5]", entry.sequenceID, entry.extraSequenceIDs)
	}

	tokens, ok := batch.tokens(0, 2)
	if !ok || !slices.Equal(tokens, []llama.Token{11, 22}) {
		t.Fatalf("tokens = %v, %t, want [11 22], true", tokens, ok)
	}
	tokens[0] = 99
	if batch.entries[0].token != 11 {
		t.Fatal("tokens returned mutable batch metadata")
	}

	if err := batch.truncate(1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if batch.len() != 1 || batch.entries[0].token != 11 {
		t.Fatalf("batch after truncate = %+v, want first entry only", batch.entries)
	}
}

func TestExtendedBatchClonesEmbeddingAndMRoPEPositions(t *testing.T) {
	batch := extendedBatch{capacity: 2}
	embedding := []float32{1, 2, 3, 4}
	positions := []llama.Pos{10, 20, 30, 40}

	idx, err := batch.addEmbedding(embedding, 4, positions, []llama.SeqId{1}, extendedBatchOutputEmbeddings)
	if err != nil {
		t.Fatalf("add embedding: %v", err)
	}
	if idx != 0 {
		t.Fatalf("index = %d, want 0", idx)
	}

	embedding[0] = 99
	positions[0] = 99
	entry := batch.entries[0]
	if !slices.Equal(entry.embedding, []float32{1, 2, 3, 4}) {
		t.Fatalf("stored embedding = %v, want cloned input", entry.embedding)
	}
	if !slices.Equal(entry.positions[:entry.positionCount], []llama.Pos{10, 20, 30, 40}) {
		t.Fatalf("stored positions = %v, want cloned input", entry.positions[:entry.positionCount])
	}
	if _, ok := batch.tokens(0, 1); ok {
		t.Fatal("embedding-only entry reported token metadata")
	}
}

func TestExtendedBatchClonesTokenPositions(t *testing.T) {
	batch := extendedBatch{capacity: 1}
	positions := []llama.Pos{10, 20, 30, 40}

	if _, err := batch.addTokenPositions(7, positions, []llama.SeqId{3}, extendedBatchOutputLogits); err != nil {
		t.Fatalf("add token positions: %v", err)
	}
	positions[0] = 99

	entry := batch.entries[0]
	if !slices.Equal(entry.positions[:entry.positionCount], []llama.Pos{10, 20, 30, 40}) {
		t.Fatalf("stored positions = %v, want cloned input", entry.positions[:entry.positionCount])
	}
}

func TestExtendedBatchRejectsInvalidEntries(t *testing.T) {
	tests := []struct {
		name string
		add  func(*extendedBatch) error
	}{
		{
			name: "missing sequence",
			add: func(batch *extendedBatch) error {
				_, err := batch.addToken(1, 0, nil, extendedBatchOutputNone)
				return err
			},
		},
		{
			name: "too many positions",
			add: func(batch *extendedBatch) error {
				_, err := batch.addEmbedding([]float32{1}, 1, []llama.Pos{0, 0, 0, 0, 0}, []llama.SeqId{0}, extendedBatchOutputNone)
				return err
			},
		},
		{
			name: "token missing positions",
			add: func(batch *extendedBatch) error {
				_, err := batch.addTokenPositions(1, nil, []llama.SeqId{0}, extendedBatchOutputNone)
				return err
			},
		},
		{
			name: "ragged embedding",
			add: func(batch *extendedBatch) error {
				_, err := batch.addEmbedding([]float32{1, 2, 3}, 2, []llama.Pos{0}, []llama.SeqId{0}, extendedBatchOutputNone)
				return err
			},
		},
		{
			name: "full",
			add: func(batch *extendedBatch) error {
				if _, err := batch.addToken(1, 0, []llama.SeqId{0}, extendedBatchOutputNone); err != nil {
					return err
				}
				_, err := batch.addToken(2, 1, []llama.SeqId{0}, extendedBatchOutputNone)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := extendedBatch{capacity: 1}
			if err := tt.add(&batch); err == nil {
				t.Fatal("error = nil, want invalid entry error")
			}
		})
	}
}

func TestExtendedBatchRejectsInvalidTruncation(t *testing.T) {
	batch := extendedBatch{capacity: 1}
	if _, err := batch.addTokenEmbedding(1, []float32{1, 2}, 2, 0, []llama.SeqId{0}, extendedBatchOutputLogits); err != nil {
		t.Fatalf("add token embedding: %v", err)
	}

	for _, length := range []int{-1, 2} {
		if err := batch.truncate(length); err == nil {
			t.Fatalf("truncate(%d) error = nil, want bounds error", length)
		}
	}
}

func TestExtendedBatchRejectsMultipleTokenEmbeddingRows(t *testing.T) {
	batch := extendedBatch{capacity: 1}
	if _, err := batch.addTokenEmbedding(1, []float32{1, 2, 3, 4}, 2, 0, []llama.SeqId{0}, extendedBatchOutputLogits); err == nil {
		t.Fatal("error = nil, want one-row token embedding error")
	}
}

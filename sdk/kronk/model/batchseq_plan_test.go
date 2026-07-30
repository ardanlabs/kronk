package model

import (
	"context"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestPlanBatchSeqItems(t *testing.T) {
	items := []batchSeqItem{
		{index: 10, tokens: []llama.Token{1, 2}},
		{index: 20, tokens: []llama.Token{3, 4, 5}},
		{index: 30, tokens: []llama.Token{6}},
	}

	tests := []struct {
		name         string
		start        int
		maxSequences int
		maxTokens    int
		wantEntries  int
		wantTokens   int
		wantNext     int
	}{
		{"all items", 0, 3, 6, 3, 6, 3},
		{"sequence limit", 0, 2, 6, 2, 5, 2},
		{"token limit", 0, 3, 4, 1, 2, 1},
		{"continue request", 1, 3, 4, 2, 4, 3},
		{"request complete", 3, 3, 6, 0, 0, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := planBatchSeqItems(items, tt.start, tt.maxSequences, tt.maxTokens)
			if err != nil {
				t.Fatalf("planBatchSeqItems: unexpected error: %v", err)
			}
			if len(got.entries) != tt.wantEntries {
				t.Errorf("entries: got %d, want %d", len(got.entries), tt.wantEntries)
			}
			if got.nTokens != tt.wantTokens {
				t.Errorf("nTokens: got %d, want %d", got.nTokens, tt.wantTokens)
			}
			if got.next != tt.wantNext {
				t.Errorf("next: got %d, want %d", got.next, tt.wantNext)
			}
			for i, entry := range got.entries {
				if entry.seqID != llama.SeqId(i) {
					t.Errorf("entry[%d].seqID: got %d, want %d", i, entry.seqID, i)
				}
				if entry.itemOffset != tt.start+i {
					t.Errorf("entry[%d].itemOffset: got %d, want %d", i, entry.itemOffset, tt.start+i)
				}
			}
		})
	}
}

func TestPlanBatchSeqItemsErrors(t *testing.T) {
	tests := []struct {
		name         string
		items        []batchSeqItem
		start        int
		maxSequences int
		maxTokens    int
	}{
		{"negative start", nil, -1, 1, 1},
		{"start past end", nil, 1, 1, 1},
		{"no sequence capacity", nil, 0, 0, 1},
		{"no token capacity", nil, 0, 1, 0},
		{"empty item", []batchSeqItem{{index: 4}}, 0, 1, 1},
		{"oversized item", []batchSeqItem{{index: 4, tokens: []llama.Token{1, 2}}}, 0, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := planBatchSeqItems(tt.items, tt.start, tt.maxSequences, tt.maxTokens); err == nil {
				t.Fatal("planBatchSeqItems: expected error")
			}
		})
	}
}

func TestScheduleBatchSeqRoundRobin(t *testing.T) {
	first := newBatchSeqJob(context.Background(), []batchSeqItem{
		{index: 10, tokens: []llama.Token{1}},
		{index: 11, tokens: []llama.Token{2}},
		{index: 12, tokens: []llama.Token{3}},
	}, 2)
	second := newBatchSeqJob(context.Background(), []batchSeqItem{
		{index: 20, tokens: []llama.Token{4}},
	}, 2)

	got, remaining, err := scheduleBatchSeq([]*batchSeqJob{first, second}, 3, 3)
	if err != nil {
		t.Fatalf("scheduleBatchSeq: unexpected error: %v", err)
	}

	wantIndexes := []int{10, 20, 11}
	for i, want := range wantIndexes {
		if got.entries[i].item.index != want {
			t.Errorf("entry[%d].index: got %d, want %d", i, got.entries[i].item.index, want)
		}
	}
	if len(got.done) != 1 || got.done[0] != second {
		t.Errorf("done: got %v, want second job", got.done)
	}
	if len(remaining) != 1 || remaining[0] != first {
		t.Errorf("remaining: got %v, want first job", remaining)
	}
	if first.next != 2 {
		t.Errorf("first.next: got %d, want %d", first.next, 2)
	}
}

func TestScheduleBatchSeqRejectsOversizedItem(t *testing.T) {
	large := newBatchSeqJob(context.Background(), []batchSeqItem{
		{index: 10, tokens: []llama.Token{1, 2, 3}},
	}, 1)
	small := newBatchSeqJob(context.Background(), []batchSeqItem{
		{index: 20, tokens: []llama.Token{4}},
	}, 1)

	got, remaining, err := scheduleBatchSeq([]*batchSeqJob{large, small}, 2, 2)
	if err != nil {
		t.Fatalf("scheduleBatchSeq: unexpected error: %v", err)
	}
	if len(got.failed) != 1 || got.failed[0].job != large {
		t.Fatalf("failed: got %v, want large job", got.failed)
	}
	if len(got.entries) != 1 || got.entries[0].job != small {
		t.Errorf("entries: got %v, want small job", got.entries)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining: got %v, want empty", remaining)
	}
}

func TestScheduleBatchSeqDefersItemFromFullBatch(t *testing.T) {
	first := newBatchSeqJob(context.Background(), []batchSeqItem{
		{index: 10, tokens: []llama.Token{1}},
	}, 1)
	second := newBatchSeqJob(context.Background(), []batchSeqItem{
		{index: 20, tokens: []llama.Token{2, 3}},
	}, 1)

	got, remaining, err := scheduleBatchSeq([]*batchSeqJob{first, second}, 2, 2)
	if err != nil {
		t.Fatalf("scheduleBatchSeq: unexpected error: %v", err)
	}
	if len(got.entries) != 1 || got.entries[0].job != first {
		t.Errorf("entries: got %v, want first job", got.entries)
	}
	if got.nTokens != 1 {
		t.Errorf("nTokens: got %d, want %d", got.nTokens, 1)
	}
	if len(remaining) != 1 || remaining[0] != second {
		t.Errorf("remaining: got %v, want second job", remaining)
	}
}

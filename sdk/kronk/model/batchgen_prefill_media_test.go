package model

import (
	"context"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/trace"
)

func TestNextMediaSlotStartsAtCursorAndSkipsIneligibleSlots(t *testing.T) {
	e := batchEngine{
		mediaNext: 2,
		slots: []*slot{
			{id: 0, active: true, inputChunks: 1},
			{id: 1, active: false, inputChunks: 1},
			{id: 2, active: true, inputChunks: 1, mediaPrefillDone: true},
			{id: 3, active: true, inputChunks: 1},
		},
	}

	s, idx := e.nextMediaSlot()
	if s == nil {
		t.Fatal("nextMediaSlot() slot = nil, want slot 3")
	}
	if idx != 3 || s.id != 3 {
		t.Errorf("nextMediaSlot() = slot %d at %d, want slot 3 at 3", s.id, idx)
	}

	e.mediaNext = 0
	s, idx = e.nextMediaSlot()
	if s == nil {
		t.Fatal("nextMediaSlot() after wrap slot = nil, want slot 0")
	}
	if idx != 0 || s.id != 0 {
		t.Errorf("nextMediaSlot() after wrap = slot %d at %d, want slot 0 at 0", s.id, idx)
	}
}

func TestMediaTextContributionSizeUsesRemainingTrayCapacity(t *testing.T) {
	tests := []struct {
		name               string
		remaining          int
		availableAfterRows int
		chunkLimit         int
		want               int
	}{
		{"generation leaves full prefill unit", 4096, 2049, 2048, 2048},
		{"generation reduces media prefill", 4096, 2044, 2048, 2044},
		{"remaining media text limits prefill", 512, 2044, 2048, 512},
		{"full tray defers media text", 512, 0, 2048, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mediaTextContributionSize(tt.remaining, tt.availableAfterRows, tt.chunkLimit)
			if got != tt.want {
				t.Errorf("media text contribution: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStagedMediaContributionCommitsOnlyAfterDecode(t *testing.T) {
	s := slot{
		chunkIdx:    2,
		chunkTokIdx: 3,
		nPast:       11,
		iBatch:      -1,
		span:        trace.SpanFromContext(context.Background()),
	}
	contribution := stagedMediaContribution{
		slot:            &s,
		nextChunkIdx:    3,
		nextChunkTokIdx: 0,
		nextPast:        llama.Pos(19),
		outputIndex:     7,
		complete:        true,
		started:         time.Now(),
	}

	if s.chunkIdx != 2 || s.chunkTokIdx != 3 || s.nPast != 11 || s.iBatch != -1 || s.mediaPrefillDone {
		t.Fatalf("slot changed before commit: %+v", s)
	}

	contribution.commit()
	if s.chunkIdx != 3 || s.chunkTokIdx != 0 || s.nPast != 19 || s.iBatch != 7 || !s.mediaPrefillDone {
		t.Errorf("slot after commit = chunk %d/%d past %d output %d complete %t, want 3/0 19 7 true",
			s.chunkIdx, s.chunkTokIdx, s.nPast, s.iBatch, s.mediaPrefillDone)
	}
}

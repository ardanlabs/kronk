package model

import "testing"

func TestNextMediaSlotStartsAtCursorAndSkipsIneligibleSlots(t *testing.T) {
	e := batchEngine{
		mediaNext: 2,
		slots: []*slot{
			{id: 0, active: true, inputChunks: 1},
			{id: 1, active: false, inputChunks: 1},
			{id: 2, active: true},
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

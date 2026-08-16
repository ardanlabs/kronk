package model

import "testing"

func TestIMCPreparationUsesPrefillBase(t *testing.T) {
	e := batchEngine{model: &Model{
		cfg: adjustGenerationBatch(adjustConfig(NewConfig(WithNSeqMax(4), WithPrefillBatchSize(2048)), 0), 1+defMTPNDraft, true),
	}}

	if got := e.imcPreparationChunkSize(); got != 2048 {
		t.Errorf("IMC preparation chunk: got %d, want 2048", got)
	}
}

func TestIMCPreparationChunkEndStopsAtCheckpoint(t *testing.T) {
	got := imcPreparationChunkEnd(0, 1000, 5000, 2048, 2500, 6000)
	if got != 1500 {
		t.Fatalf("chunk end: got %d, want 1500", got)
	}
}

func TestNextIMCPreparationSlotStartsAtCursorAndWraps(t *testing.T) {
	e := batchEngine{
		slots: []*slot{
			{active: true, imcPrep: &imcPreparation{}},
			{active: false, imcPrep: &imcPreparation{}},
			{active: true, imcPrep: &imcPreparation{}},
		},
	}

	_, idx := e.nextIMCPreparationSlot()
	if idx != 0 {
		t.Fatalf("first slot: got %d, want 0", idx)
	}

	e.imcPrepNext = idx + 1
	_, idx = e.nextIMCPreparationSlot()
	if idx != 2 {
		t.Fatalf("rotated slot: got %d, want 2", idx)
	}
}

func TestIMCPreparationSlotIDsReturnsEveryEligibleSlot(t *testing.T) {
	e := batchEngine{slots: []*slot{
		{id: 0, active: true, imcPrep: &imcPreparation{}},
		{id: 1, active: true},
		{id: 2, active: false, imcPrep: &imcPreparation{}},
		{id: 3, active: true, imcPrep: &imcPreparation{}},
	}}

	got := e.imcPreparationSlotIDs()
	if len(got) != 2 || got[0] != 0 || got[1] != 3 {
		t.Errorf("imcPreparationSlotIDs() = %v, want [0 3]", got)
	}
}

func TestIMCPreparationNextRetainsOwnerUntilComplete(t *testing.T) {
	tests := []struct {
		name     string
		selected int
		slots    int
		complete bool
		want     int
	}{
		{"incomplete retains owner", 1, 4, false, 1},
		{"complete advances", 1, 4, true, 2},
		{"complete wraps", 3, 4, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imcPreparationNext(tt.selected, tt.slots, tt.complete)
			if got != tt.want {
				t.Errorf("next cursor = %d, want %d", got, tt.want)
			}
		})
	}
}

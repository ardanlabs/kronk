package model

import (
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestBatchEngineSnapshotPublishesSlotAndSelectorState(t *testing.T) {
	now := time.Now()
	m := Model{cfg: adjustConfig(NewConfig(WithNSeqMax(2), WithPrefillBatchSize(2048)), 0)}
	e := batchEngine{
		model:                     &m,
		prefillNext:               1,
		imcPrepNext:               0,
		diagnosticPrefillStart:    0,
		diagnosticPrefillSelected: 1,
		diagnosticIMCStart:        1,
		diagnosticIMCSelected:     0,
		diagnosticGenerationRows:  1,
		diagnosticGeneration: []BatchGenerationContribution{
			{SlotID: 2, Rows: 1, Mode: "ordinary"},
		},
		slots: []*slot{
			{
				id:      0,
				active:  true,
				job:     &chatJob{id: "imc-request", requestStart: now.Add(-time.Second), imcNewCacheTokens: make([]llama.Token, 100)},
				imcPrep: &imcPreparation{nextToken: 40},
			},
			{
				id:            1,
				active:        true,
				job:           &chatJob{id: "prefill-request", requestStart: now.Add(-time.Second)},
				prefillTokens: make([]llama.Token, 80),
				nPrefilled:    20,
			},
			{
				id:               2,
				active:           true,
				job:              &chatJob{id: "generation-request", requestStart: now.Add(-time.Second)},
				prefillDone:      true,
				completionTokens: 7,
			},
			{
				id:           3,
				active:       true,
				job:          &chatJob{id: "restore-request", requestStart: now.Add(-time.Second)},
				imcRestoring: true,
			},
		},
	}
	e.batch.NTokens = 41
	m.batch = &e

	e.publishDiagnostics(true)
	got, ok := m.BatchEngineSnapshot()
	if !ok {
		t.Fatal("BatchEngineSnapshot() available = false, want true")
	}
	if got.PrefillBatchSize != 2048 || got.NUBatch != 2048 || got.NBatch != 2050 {
		t.Errorf("batch sizing = %d/%d/%d, want 2048/2048/2050", got.PrefillBatchSize, got.NUBatch, got.NBatch)
	}
	if got.GenerationRows != 1 || got.PrefillRows != 40 || got.TotalRows != 41 {
		t.Errorf("tray rows = generation %d, prefill %d, total %d; want 1, 40, 41",
			got.GenerationRows, got.PrefillRows, got.TotalRows)
	}
	if got.PrefillSelectorSelected != 1 || !got.Slots[1].PrefillOwner {
		t.Errorf("prefill selector = %d and owner = %t, want 1 and true",
			got.PrefillSelectorSelected, got.Slots[1].PrefillOwner)
	}
	if got.Slots[0].Phase != "prefill-imc" || got.Slots[0].IMCPreparationRemaining != 60 {
		t.Errorf("slot 0 = phase %q, remaining %d; want prefill-imc, 60",
			got.Slots[0].Phase, got.Slots[0].IMCPreparationRemaining)
	}
	if got.Slots[1].Phase != "prefill" || got.Slots[1].PrefillRemaining != 60 {
		t.Errorf("slot 1 = phase %q, remaining %d; want prefill, 60",
			got.Slots[1].Phase, got.Slots[1].PrefillRemaining)
	}
	if got.Slots[2].Phase != "generation" || got.Slots[2].GenerationRows != 1 {
		t.Errorf("slot 2 = phase %q, rows %d; want generation, 1",
			got.Slots[2].Phase, got.Slots[2].GenerationRows)
	}
	if got.Slots[3].Phase != "imc-restore" {
		t.Errorf("slot 3 phase = %q, want imc-restore", got.Slots[3].Phase)
	}

	got.EligiblePrefillSlots[0] = 99
	again, _ := m.BatchEngineSnapshot()
	if again.EligiblePrefillSlots[0] != 1 {
		t.Errorf("snapshot mutation changed stored eligible slots: got %v, want [1]", again.EligiblePrefillSlots)
	}
}

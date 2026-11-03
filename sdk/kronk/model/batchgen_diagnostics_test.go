package model

import (
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestBatchEngineSnapshotPublishesSlotAndSelectorState(t *testing.T) {
	now := time.Now()
	m := Model{cfg: adjustConfig(NewConfig(WithNSeqMax(2), WithPrefillBatchSize(2048)), 0, false)}
	e := batchEngine{
		model:                     &m,
		batch:                     &extendedBatch{capacity: 64},
		prefillNext:               1,
		imcPrepNext:               0,
		specTargetRollbacks:       map[string]uint64{"attention-suffix": 7, "recurrent-native": 2},
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
				id:                  2,
				active:              true,
				job:                 &chatJob{id: "generation-request", requestStart: now.Add(-time.Second)},
				prefillDone:         true,
				completionTokens:    7,
				specTargetRollbacks: map[string]uint64{"attention-suffix": 3},
			},
			{
				id:           3,
				active:       true,
				job:          &chatJob{id: "restore-request", requestStart: now.Add(-time.Second)},
				imcRestoring: true,
			},
			{
				id:              4,
				active:          true,
				job:             &chatJob{id: "media-request", requestStart: now.Add(-time.Second)},
				mediaPrefilling: true,
			},
		},
	}
	for i := range 41 {
		if _, err := e.batch.addToken(llama.Token(i), llama.Pos(i), []llama.SeqId{0}, extendedBatchOutputNone); err != nil {
			t.Fatalf("add diagnostic batch row %d: %v", i, err)
		}
	}
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
	if got.Slots[4].Phase != "media-prefill" {
		t.Errorf("slot 4 phase = %q, want media-prefill", got.Slots[4].Phase)
	}
	if got.SpeculativeRollbacks["attention-suffix"] != 7 || got.SpeculativeRollbacks["recurrent-native"] != 2 ||
		got.Slots[2].SpeculativeRollbacks["attention-suffix"] != 3 || len(got.Slots[1].SpeculativeRollbacks) != 0 {
		t.Errorf("rollback counts: got engine %v, generating slot %v, prefill slot %v; want 7/2, 3, empty",
			got.SpeculativeRollbacks, got.Slots[2].SpeculativeRollbacks, got.Slots[1].SpeculativeRollbacks)
	}

	got.EligiblePrefillSlots[0] = 99
	got.SpeculativeRollbacks["attention-suffix"] = 99
	got.Slots[2].SpeculativeRollbacks["attention-suffix"] = 99
	// Engine progress must not mutate an already published snapshot either.
	e.specTargetRollbacks["attention-suffix"]++
	e.slots[2].specTargetRollbacks["attention-suffix"]++
	again, _ := m.BatchEngineSnapshot()
	if again.EligiblePrefillSlots[0] != 1 {
		t.Errorf("snapshot mutation changed stored eligible slots: got %v, want [1]", again.EligiblePrefillSlots)
	}
	if again.SpeculativeRollbacks["attention-suffix"] != 7 || again.Slots[2].SpeculativeRollbacks["attention-suffix"] != 3 {
		t.Errorf("published rollback counts mutated: got engine %v, slot %v; want 7, 3",
			again.SpeculativeRollbacks, again.Slots[2].SpeculativeRollbacks)
	}
	e.slots[2].reset()
	e.publishDiagnostics(true)
	completed, _ := m.BatchEngineSnapshot()
	if completed.SpeculativeRollbacks["attention-suffix"] != 8 || len(completed.Slots[2].SpeculativeRollbacks) != 0 {
		t.Errorf("completed rollback counts: got engine %v, slot %v; want 8, empty",
			completed.SpeculativeRollbacks, completed.Slots[2].SpeculativeRollbacks)
	}
}

package toolapp

import (
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/pool"
)

func TestToBatchEngineSnapshots(t *testing.T) {
	snapshots := []pool.BatchEngineDetail{{
		ModelID: "test/model",
		BatchEngineSnapshot: model.BatchEngineSnapshot{
			Iteration:        7,
			PrefillBatchSize: 2048,
			NBatch:           2056,
			NUBatch:          2056,
			MTP:              true,
			NDraft:           3,
			Slots: []model.BatchSlotSnapshot{{
				ID:         1,
				Phase:      "generation",
				RequestID:  "request-1",
				RequestAge: 1500 * time.Millisecond,
				PastTokens: 100,
			}},
		},
	}}

	got := toBatchEngineSnapshots(snapshots)
	if len(got) != 1 {
		t.Fatalf("len(response) = %d, want 1", len(got))
	}
	if got[0].ModelID != "test/model" || !got[0].MTP || got[0].NDraft != 3 {
		t.Errorf("model detail = %#v, want test/model MTP nDraft 3", got[0])
	}
	if got[0].PrefillBatchSize != 2048 || got[0].NUBatch != 2056 || got[0].NBatch != 2056 {
		t.Errorf("batch sizing = %d/%d/%d, want 2048/2056/2056", got[0].PrefillBatchSize, got[0].NUBatch, got[0].NBatch)
	}
	if got[0].Slots[0].RequestAgeMS != 1500 {
		t.Errorf("request age = %dms, want 1500ms", got[0].Slots[0].RequestAgeMS)
	}
	if got[0].EligiblePrefillSlots == nil || got[0].EligibleIMCSlots == nil {
		t.Errorf("eligible slot lists are nil, want empty arrays")
	}
}

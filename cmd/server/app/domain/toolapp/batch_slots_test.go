package toolapp

import (
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/pool"
)

func TestToBatchEngineSnapshotsNormalizesWireValues(t *testing.T) {
	snapshots := []pool.BatchEngineDetail{{
		Slots: []model.BatchSlotSnapshot{{
			RequestAge: 1500 * time.Millisecond,
		}},
	}}

	got := toBatchEngineSnapshots(snapshots)
	if len(got) != 1 {
		t.Fatalf("len(response) = %d, want 1", len(got))
	}
	if got[0].Slots[0].RequestAgeMS != 1500 {
		t.Errorf("request age = %dms, want 1500ms", got[0].Slots[0].RequestAgeMS)
	}
	if got[0].EligiblePrefillSlots == nil || got[0].EligibleIMCSlots == nil || got[0].GenerationContributions == nil {
		t.Errorf("diagnostic lists contain nil, want empty arrays")
	}
}

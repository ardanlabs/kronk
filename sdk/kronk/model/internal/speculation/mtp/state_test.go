package mtp

import (
	"slices"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
)

func TestSlotStateResetRetainsBuffersAndClearsLifecycle(t *testing.T) {
	state := SlotState{
		PendingHidden: []float32{1},
		DraftHidden:   []float32{2},
		VerifyHidden:  []float32{3},
		TargetRange:   speculation.TargetRange{Count: 1},
		HasTargetRows: true,
		Disabled:      true,
		DisableReason: "test",
		ResumeSource:  "test",
	}
	pendingCapacity := cap(state.PendingHidden)

	state.Reset()

	if len(state.PendingHidden) != 0 || cap(state.PendingHidden) != pendingCapacity {
		t.Fatalf("pending hidden after reset: len=%d cap=%d, want len=0 cap=%d", len(state.PendingHidden), cap(state.PendingHidden), pendingCapacity)
	}
	if len(state.DraftHidden) != 0 || len(state.VerifyHidden) != 0 {
		t.Fatalf("transient hidden buffers not reset")
	}
	if state.HasTargetRows || state.TargetRange.Count != 0 || state.Disabled || state.DisableReason != "" || state.ResumeSource != "" {
		t.Fatalf("lifecycle state not reset: %+v", state)
	}
}

func TestSlotStateDisable(t *testing.T) {
	state := SlotState{PendingHidden: []float32{1}, HasTargetRows: true}
	state.Disable("sync-error")

	if !state.Disabled || state.DisableReason != "sync-error" || state.HasTargetRows || len(state.PendingHidden) != 0 {
		t.Fatalf("disabled state = %+v", state)
	}
}

func TestCheckpointState(t *testing.T) {
	for _, test := range []struct {
		name  string
		state []float32
		width int
		want  bool
	}{
		{"complete", []float32{3, -7}, 2, true},
		{"missing", nil, 2, false},
		{"wrong width", []float32{3}, 2, false},
		{"invalid width", nil, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var restored SlotState
			if got := restored.RestoreCheckpoint(test.state, test.width); got != test.want {
				t.Fatalf("restore: got %t, want %t", got, test.want)
			}
			if !test.want {
				return
			}
			test.state[0] = 99
			if got := restored.CheckpointState(test.width); !slices.Equal(got, []float32{3, -7}) {
				t.Errorf("owned restored state: got %v, want [3 -7]", got)
			}
			restored.Disable("imc-hit")
			if got := restored.CheckpointState(test.width); got != nil {
				t.Errorf("disabled checkpoint: got %v, want nil", got)
			}
		})
	}
}

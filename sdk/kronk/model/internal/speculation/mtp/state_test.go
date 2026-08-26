package mtp

import (
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
)

func TestSlotStateResetRetainsBuffersAndClearsLifecycle(t *testing.T) {
	state := SlotState{
		PendingHidden: []float32{1},
		DraftHidden:   []float32{2},
		VerifyHidden:  []float32{3},
		AcceptanceEMA: 0.2,
		Rounds:        4,
		LowWindows:    1,
		RoundDraft:    2,
		RoundStarted:  time.Now(),
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
	if state.AcceptanceEMA != 1 || state.Rounds != 0 || state.LowWindows != 0 || state.RoundDraft != 0 || !state.RoundStarted.IsZero() {
		t.Fatalf("acceptance state not reset: %+v", state)
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

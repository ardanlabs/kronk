package mtp

import (
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
)

// SlotState contains all request-local state owned by MTP. The batch engine
// stores one value per execution slot but does not interpret its invariants.
type SlotState struct {
	PendingHidden []float32
	DraftHidden   []float32
	VerifyHidden  []float32
	AcceptanceEMA float64
	Rounds        int
	LowWindows    int
	RoundDraft    int
	RoundStarted  time.Time
	TargetRange   speculation.TargetRange
	HasTargetRows bool
	Disabled      bool
	DisableReason string
	ResumeSource  string
}

// Reset begins a new request while retaining reusable buffer capacity.
func (s *SlotState) Reset() {
	s.PendingHidden = s.PendingHidden[:0]
	s.DraftHidden = s.DraftHidden[:0]
	s.VerifyHidden = s.VerifyHidden[:0]
	s.AcceptanceEMA = 1
	s.Rounds = 0
	s.LowWindows = 0
	s.RoundDraft = 0
	s.RoundStarted = time.Time{}
	s.TargetRange = speculation.TargetRange{}
	s.HasTargetRows = false
	s.Disabled = false
	s.DisableReason = ""
	s.ResumeSource = ""
}

// TrackTargetRange records target rows awaiting synchronization.
func (s *SlotState) TrackTargetRange(targetRange speculation.TargetRange) {
	s.TargetRange = targetRange
	s.HasTargetRows = targetRange.Count > 0
}

// ClearTargetRange marks all staged target rows as consumed.
func (s *SlotState) ClearTargetRange() {
	s.HasTargetRows = false
	s.TargetRange.Count = 0
}

// Disable stops MTP for the remainder of the current request.
func (s *SlotState) Disable(reason string) {
	s.PendingHidden = s.PendingHidden[:0]
	s.HasTargetRows = false
	s.Disabled = true
	s.DisableReason = reason
}

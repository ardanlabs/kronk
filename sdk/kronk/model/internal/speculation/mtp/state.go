package mtp

import "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"

// SlotState contains all request-local state owned by MTP. The batch engine
// stores one value per execution slot but does not interpret its invariants.
type SlotState struct {
	PendingHidden []float32
	DraftHidden   []float32
	VerifyHidden  []float32
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

// CheckpointState returns the borrowed hidden vector needed to resume an
// independently owned MTP prefix, or nil when it is unavailable.
func (s *SlotState) CheckpointState(width int) []float32 {
	if s.Disabled || width <= 0 || len(s.PendingHidden) != width {
		return nil
	}
	return s.PendingHidden
}

// RestoreCheckpoint copies the hidden vector paired with restored draft KV.
func (s *SlotState) RestoreCheckpoint(state []float32, width int) bool {
	if width <= 0 || len(state) != width {
		return false
	}
	s.PendingHidden = append(s.PendingHidden[:0], state...)
	s.ResumeSource = "restored-draft-kv"
	return true
}

// Disable stops MTP for the remainder of the current request.
func (s *SlotState) Disable(reason string) {
	s.PendingHidden = s.PendingHidden[:0]
	s.HasTargetRows = false
	s.Disabled = true
	s.DisableReason = reason
}

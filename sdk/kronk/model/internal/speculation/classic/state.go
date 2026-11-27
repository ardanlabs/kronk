package classic

import "github.com/hybridgroup/yzma/pkg/llama"

// SlotState contains request-local state owned by classic speculation.
type SlotState struct {
	AcceptanceEMA      float64
	ProbeTick          int
	Rounds             int
	DraftStartPosition llama.Pos
	DraftDistributions [][]llama.DraftCandidate
}

// Reset begins a request.
func (s *SlotState) Reset() {
	s.AcceptanceEMA = 1
	s.ProbeTick = 0
	s.Rounds = 0
	s.DraftStartPosition = 0
	s.DraftDistributions = nil
}

package observapp

import (
	"encoding/json"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/session"
)

// Sessions contains the latest in-memory session summaries.
type Sessions struct {
	Enabled  bool              `json:"enabled"`
	Sessions []session.Summary `json:"sessions"`
}

// Encode implements web.Encoder.
func (s Sessions) Encode() ([]byte, string, error) {
	data, err := json.Marshal(s)
	return data, "application/json", err
}

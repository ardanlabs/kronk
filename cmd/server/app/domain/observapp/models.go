package observapp

import (
	"encoding/json"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/session"
)

// Status reports whether context session observability is enabled.
type Status struct {
	Enabled bool `json:"enabled"`
}

// Encode implements web.Encoder.
func (s Status) Encode() ([]byte, string, error) {
	data, err := json.Marshal(s)
	return data, "application/json", err
}

// Summary reports distributions across Active, Idle, and Completed sessions.
type Summary session.Overview

// Encode implements web.Encoder.
func (s Summary) Encode() ([]byte, string, error) {
	data, err := json.Marshal(s)
	return data, "application/json", err
}

// Page contains one offset-paginated session summary page.
type Page session.Page

// Encode implements web.Encoder.
func (p Page) Encode() ([]byte, string, error) {
	data, err := json.Marshal(p)
	return data, "application/json", err
}

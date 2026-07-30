// Package session tracks model context use over the lifetime of inference sessions.
package session

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrClosed is returned when an operation is attempted after shutdown.
	ErrClosed = errors.New("session tracker is closed")

	// ErrNotFound is returned when a live session does not exist.
	ErrNotFound = errors.New("session not found")

	// ErrRequestActive is returned when a second request starts for a session
	// that is already executing a different request.
	ErrRequestActive = errors.New("session already has an active request")

	// ErrRequestMismatch is returned when a completion does not match the
	// request currently executing for a session.
	ErrRequestMismatch = errors.New("request does not match active session request")
)

// State identifies the lifecycle group containing a session summary.
type State string

const (
	// StateActive identifies a session currently executing a request.
	StateActive State = "active"

	// StateIdle identifies a reusable IMC session with no executing request.
	StateIdle State = "idle"

	// StateCompleted identifies a session that has finished.
	StateCompleted State = "completed"
)

// Key identifies a session by model and session ID.
type Key struct {
	ModelID   string
	SessionID string
}

// RequestStart describes a request beginning execution for a session.
type RequestStart struct {
	Key           Key
	RequestID     string
	StartedAt     time.Time
	ContextWindow int
	PromptTokens  int
}

// RequestCompletion describes the final token accounting for one request.
type RequestCompletion struct {
	Key          Key
	RequestID    string
	CompletedAt  time.Time
	PromptTokens int
	CachedTokens int
	OutputTokens int
	ContextFull  bool
	Reusable     bool
}

// Summary contains context-use metadata for a session. It intentionally stores
// no prompt, response, media, user identity, or network address.
type Summary struct {
	ModelID   string `json:"model_id"`
	SessionID string `json:"session_id"`
	State     State  `json:"state"`

	StartedAt    time.Time  `json:"started_at"`
	LastActiveAt time.Time  `json:"last_active_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`

	RequestCount   int `json:"request_count"`
	ContextWindow  int `json:"context_window"`
	CurrentContext int `json:"current_context,omitempty"`
	PeakPrompt     int `json:"peak_prompt"`
	PeakOutput     int `json:"peak_output"`
	PeakContext    int `json:"peak_context"`
	CachedTokens   int `json:"cached_tokens,omitempty"`

	TotalCachedTokens int64 `json:"total_cached_tokens,omitempty"`

	ContextFull bool `json:"context_full,omitempty"`
	Incomplete  bool `json:"incomplete,omitempty"`
}

// Utilization returns the peak fraction of the configured context window used
// by the session.
func (s Summary) Utilization() float64 {
	if s.ContextWindow <= 0 {
		return 0
	}

	return float64(s.PeakContext) / float64(s.ContextWindow)
}

// Observer is the lifecycle contract used by inference and IMC code.
type Observer interface {
	RequestStarted(RequestStart) error
	RequestCompleted(context.Context, RequestCompletion) error
	SessionCompleted(context.Context, Key) error
}

func eventTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}

	return value.UTC()
}

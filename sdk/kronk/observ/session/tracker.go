package session

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

type trackedSession struct {
	currentRequestID string
	summary          Summary
}

// Tracker keeps the latest summary for each session in memory.
type Tracker struct {
	mu       sync.RWMutex
	sessions map[Key]*trackedSession
	closed   bool
}

// New constructs an in-memory session tracker.
func New() *Tracker {
	return &Tracker{sessions: make(map[Key]*trackedSession)}
}

// RequestStarted marks a session Active and records the request's initial
// prompt occupancy.
func (t *Tracker) RequestStarted(event RequestStart) error {
	if err := validateKey(event.Key); err != nil {
		return fmt.Errorf("request started: %w", err)
	}
	if event.RequestID == "" {
		return errors.New("request started: request ID is required")
	}

	now := eventTime(event.StartedAt)
	promptTokens := max(event.PromptTokens, 0)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}

	tracked, exists := t.sessions[event.Key]
	if !exists {
		tracked = &trackedSession{
			summary: Summary{
				ModelID:      event.Key.ModelID,
				SessionID:    event.Key.SessionID,
				StartedAt:    now,
				LastActiveAt: now,
			},
		}
		t.sessions[event.Key] = tracked
	}

	if tracked.summary.State == StateActive {
		if tracked.currentRequestID == event.RequestID {
			return nil
		}
		return ErrRequestActive
	}

	tracked.currentRequestID = event.RequestID
	tracked.summary.State = StateActive
	tracked.summary.LastActiveAt = now
	tracked.summary.EndedAt = nil
	tracked.summary.Incomplete = false
	tracked.summary.RequestCount++
	tracked.summary.CurrentContext = promptTokens
	tracked.summary.ContextWindow = event.ContextWindow
	tracked.summary.PeakPrompt = max(tracked.summary.PeakPrompt, promptTokens)
	tracked.summary.PeakContext = max(tracked.summary.PeakContext, promptTokens)

	return nil
}

// RequestCompleted applies final token accounting and updates the session's
// latest state.
func (t *Tracker) RequestCompleted(ctx context.Context, event RequestCompletion) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateKey(event.Key); err != nil {
		return fmt.Errorf("request completed: %w", err)
	}
	if event.RequestID == "" {
		return errors.New("request completed: request ID is required")
	}

	now := eventTime(event.CompletedAt)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}

	tracked, exists := t.sessions[event.Key]
	if !exists {
		return ErrNotFound
	}
	if tracked.summary.State != StateActive || tracked.currentRequestID != event.RequestID {
		return ErrRequestMismatch
	}

	promptTokens := max(event.PromptTokens, 0)
	cachedTokens := min(max(event.CachedTokens, 0), promptTokens)
	outputTokens := max(event.OutputTokens, 0)

	tracked.currentRequestID = ""
	tracked.summary.LastActiveAt = now
	tracked.summary.CurrentContext = 0
	tracked.summary.PeakPrompt = max(tracked.summary.PeakPrompt, promptTokens)
	tracked.summary.PeakOutput = max(tracked.summary.PeakOutput, outputTokens)
	tracked.summary.PeakContext = max(tracked.summary.PeakContext, promptTokens+outputTokens)
	tracked.summary.CachedTokens = cachedTokens
	tracked.summary.TotalCachedTokens += int64(cachedTokens)
	tracked.summary.TotalProcessedTokens += int64(promptTokens - cachedTokens)
	tracked.summary.ContextFull = tracked.summary.ContextFull || event.ContextFull
	if event.Reusable {
		tracked.summary.State = StateIdle
		return nil
	}

	complete(tracked, now)
	return nil
}

// SessionCompleted marks the latest in-memory summary as Completed.
func (t *Tracker) SessionCompleted(ctx context.Context, key Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateKey(key); err != nil {
		return fmt.Errorf("session completed: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrClosed
	}

	tracked, exists := t.sessions[key]
	if !exists {
		return ErrNotFound
	}

	complete(tracked, time.Now().UTC())
	return nil
}

func complete(tracked *trackedSession, endedAt time.Time) {
	tracked.summary.Incomplete = tracked.summary.State == StateActive
	tracked.summary.State = StateCompleted
	tracked.summary.EndedAt = &endedAt
	tracked.summary.CurrentContext = 0
	tracked.currentRequestID = ""
}

// List returns every in-memory session, ordered by peak context use descending.
func (t *Tracker) List() ([]Summary, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return nil, ErrClosed
	}

	summaries := make([]Summary, 0, len(t.sessions))
	for _, tracked := range t.sessions {
		summaries = append(summaries, tracked.summary)
	}
	slices.SortFunc(summaries, func(a, b Summary) int {
		if a.PeakContext != b.PeakContext {
			return b.PeakContext - a.PeakContext
		}
		if cmp := b.LastActiveAt.Compare(a.LastActiveAt); cmp != 0 {
			return cmp
		}
		if a.ModelID != b.ModelID {
			return compareString(a.ModelID, b.ModelID)
		}
		return compareString(a.SessionID, b.SessionID)
	})

	return summaries, nil
}

func compareString(a string, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// Shutdown discards all in-memory session data and closes the tracker.
func (t *Tracker) Shutdown(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true
	clear(t.sessions)

	return nil
}

func validateKey(key Key) error {
	if key.ModelID == "" {
		return errors.New("model ID is required")
	}
	if key.SessionID == "" {
		return errors.New("session ID is required")
	}

	return nil
}

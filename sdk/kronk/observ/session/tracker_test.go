package session

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTrackerLifecycle(t *testing.T) {
	tracker := New()
	ctx := context.Background()
	key := Key{ModelID: "model", SessionID: "session"}
	start := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)

	if err := tracker.RequestStarted(RequestStart{
		Key:           key,
		RequestID:     "request-1",
		StartedAt:     start,
		ContextWindow: 262_144,
		PromptTokens:  100,
	}); err != nil {
		t.Fatalf("start first request: %v", err)
	}
	if err := tracker.RequestCompleted(ctx, RequestCompletion{
		Key:          key,
		RequestID:    "request-1",
		CompletedAt:  start.Add(time.Minute),
		PromptTokens: 100,
		OutputTokens: 20,
		Reusable:     true,
	}); err != nil {
		t.Fatalf("complete first request: %v", err)
	}
	if err := tracker.RequestStarted(RequestStart{
		Key:           key,
		RequestID:     "request-2",
		StartedAt:     start.Add(2 * time.Minute),
		ContextWindow: 262_144,
		PromptTokens:  180,
	}); err != nil {
		t.Fatalf("start second request: %v", err)
	}
	if err := tracker.RequestCompleted(ctx, RequestCompletion{
		Key:          key,
		RequestID:    "request-2",
		CompletedAt:  start.Add(3 * time.Minute),
		PromptTokens: 180,
		CachedTokens: 100,
		OutputTokens: 30,
		Reusable:     true,
	}); err != nil {
		t.Fatalf("complete second request: %v", err)
	}

	sessions, err := tracker.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions: got %d, want 1", len(sessions))
	}
	summary := sessions[0]
	if summary.State != StateIdle {
		t.Errorf("state: got %q, want %q", summary.State, StateIdle)
	}
	if summary.RequestCount != 2 || summary.PeakContext != 210 {
		t.Errorf("summary accounting: got requests=%d peak=%d", summary.RequestCount, summary.PeakContext)
	}
	if summary.TotalCachedTokens != 100 {
		t.Errorf("cached token accounting: got %d, want 100", summary.TotalCachedTokens)
	}

	if err := tracker.SessionCompleted(ctx, key); err != nil {
		t.Fatalf("complete session: %v", err)
	}
	sessions, err = tracker.List()
	if err != nil {
		t.Fatalf("list completed: %v", err)
	}
	if sessions[0].State != StateCompleted || sessions[0].EndedAt == nil {
		t.Fatalf("completed summary: %+v", sessions[0])
	}
}

func TestTrackerListSortsByPeakContext(t *testing.T) {
	tracker := New()
	for _, test := range []struct {
		id     string
		prompt int
	}{
		{id: "small", prompt: 100},
		{id: "large", prompt: 500},
		{id: "medium", prompt: 250},
	} {
		if err := tracker.RequestStarted(RequestStart{
			Key:          Key{ModelID: "model", SessionID: test.id},
			RequestID:    "request",
			PromptTokens: test.prompt,
		}); err != nil {
			t.Fatalf("start %s: %v", test.id, err)
		}
	}

	sessions, err := tracker.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []string{"large", "medium", "small"}
	for i, id := range want {
		if sessions[i].SessionID != id {
			t.Errorf("session %d: got %q, want %q", i, sessions[i].SessionID, id)
		}
	}
}

func TestTrackerRejectsConcurrentRequest(t *testing.T) {
	tracker := New()
	key := Key{ModelID: "model", SessionID: "session"}
	if err := tracker.RequestStarted(RequestStart{Key: key, RequestID: "one"}); err != nil {
		t.Fatalf("start request: %v", err)
	}
	if err := tracker.RequestStarted(RequestStart{Key: key, RequestID: "two"}); !errors.Is(err, ErrRequestActive) {
		t.Fatalf("second request error: got %v, want ErrRequestActive", err)
	}
}

func TestTrackerCanceledCompletionDoesNotChangeSession(t *testing.T) {
	tracker := New()
	key := Key{ModelID: "model", SessionID: "session"}
	if err := tracker.RequestStarted(RequestStart{Key: key, RequestID: "request"}); err != nil {
		t.Fatalf("start request: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := tracker.RequestCompleted(ctx, RequestCompletion{Key: key, RequestID: "request"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("completion error: got %v, want context.Canceled", err)
	}

	sessions, err := tracker.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if sessions[0].State != StateActive {
		t.Errorf("state: got %q, want %q", sessions[0].State, StateActive)
	}
}

func TestTrackerShutdownDiscardsSessions(t *testing.T) {
	tracker := New()
	if err := tracker.RequestStarted(RequestStart{
		Key:       Key{ModelID: "model", SessionID: "session"},
		RequestID: "request",
	}); err != nil {
		t.Fatalf("start request: %v", err)
	}
	if err := tracker.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if _, err := tracker.List(); !errors.Is(err, ErrClosed) {
		t.Fatalf("list after shutdown: got %v, want ErrClosed", err)
	}
	if len(tracker.sessions) != 0 {
		t.Fatalf("sessions after shutdown: got %d, want 0", len(tracker.sessions))
	}
}

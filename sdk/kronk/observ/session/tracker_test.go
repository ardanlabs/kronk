package session

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestTrackerLifecycle(t *testing.T) {
	tracker := newTestTracker(t, 10)
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
		CachedTokens: 0,
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

	page, err := tracker.List(ctx, StateIdle, Query{})
	if err != nil {
		t.Fatalf("list idle: %v", err)
	}
	if len(page.Sessions) != 1 {
		t.Fatalf("idle summaries: got %d, want 1", len(page.Sessions))
	}

	summary := page.Sessions[0]
	if summary.RequestCount != 2 {
		t.Errorf("request count: got %d, want 2", summary.RequestCount)
	}
	if summary.PeakContext != 210 {
		t.Errorf("peak context: got %d, want 210", summary.PeakContext)
	}
	if summary.TotalCachedTokens != 100 {
		t.Errorf("cached tokens: got %d, want 100", summary.TotalCachedTokens)
	}
	if summary.TotalProcessedTokens != 180 {
		t.Errorf("processed tokens: got %d, want 180", summary.TotalProcessedTokens)
	}

	if err := tracker.SessionCompleted(ctx, key); err != nil {
		t.Fatalf("finalize session: %v", err)
	}

	completed, err := tracker.List(ctx, StateCompleted, Query{})
	if err != nil {
		t.Fatalf("list completed: %v", err)
	}
	if len(completed.Sessions) != 1 {
		t.Fatalf("completed summaries: got %d, want 1", len(completed.Sessions))
	}
	if completed.Sessions[0].SessionID != key.SessionID {
		t.Errorf("session ID: got %q, want %q", completed.Sessions[0].SessionID, key.SessionID)
	}
}

func TestTrackerNonReusableRequestCompletesImmediately(t *testing.T) {
	tracker := newTestTracker(t, 10)
	ctx := context.Background()
	key := Key{ModelID: "model", SessionID: "request-session"}

	if err := tracker.RequestStarted(RequestStart{
		Key:           key,
		RequestID:     "request",
		ContextWindow: 4_096,
		PromptTokens:  500,
	}); err != nil {
		t.Fatalf("start request: %v", err)
	}
	if err := tracker.RequestCompleted(ctx, RequestCompletion{
		Key:          key,
		RequestID:    "request",
		PromptTokens: 500,
		OutputTokens: 100,
		Reusable:     false,
	}); err != nil {
		t.Fatalf("complete request: %v", err)
	}

	counts := tracker.Counts()
	if counts.Active != 0 || counts.Idle != 0 || counts.Completed != 1 {
		t.Fatalf("counts: got %+v, want only one completed", counts)
	}
}

func TestTrackerRejectsConcurrentRequest(t *testing.T) {
	tracker := newTestTracker(t, 10)
	key := Key{ModelID: "model", SessionID: "session"}

	if err := tracker.RequestStarted(RequestStart{Key: key, RequestID: "one"}); err != nil {
		t.Fatalf("start request: %v", err)
	}
	if err := tracker.RequestStarted(RequestStart{Key: key, RequestID: "two"}); !errors.Is(err, ErrRequestActive) {
		t.Fatalf("second request error: got %v, want ErrRequestActive", err)
	}
}

func TestTrackerRetriesCanceledCompletionWithoutDoubleAccounting(t *testing.T) {
	tracker := newTestTracker(t, 10)
	key := Key{ModelID: "model", SessionID: "session"}
	event := RequestCompletion{
		Key:          key,
		RequestID:    "request",
		PromptTokens: 100,
		CachedTokens: 40,
		OutputTokens: 20,
		Reusable:     false,
	}

	if err := tracker.RequestStarted(RequestStart{Key: key, RequestID: "request"}); err != nil {
		t.Fatalf("start request: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tracker.RequestCompleted(canceled, event); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled completion: got %v, want context.Canceled", err)
	}
	if err := tracker.RequestCompleted(context.Background(), event); err != nil {
		t.Fatalf("retry completion: %v", err)
	}

	page, err := tracker.List(context.Background(), StateCompleted, Query{})
	if err != nil {
		t.Fatalf("list completed: %v", err)
	}
	if len(page.Sessions) != 1 {
		t.Fatalf("completed summaries: got %d, want 1", len(page.Sessions))
	}
	if got := page.Sessions[0].TotalCachedTokens; got != 40 {
		t.Errorf("cached tokens: got %d, want 40", got)
	}
	if got := page.Sessions[0].TotalProcessedTokens; got != 60 {
		t.Errorf("processed tokens: got %d, want 60", got)
	}
}

func TestTrackerCurrentSummary(t *testing.T) {
	tracker := newTestTracker(t, 10)
	ctx := context.Background()

	for i, prompt := range []int{100, 200, 300, 400} {
		key := Key{ModelID: "model", SessionID: fmt.Sprintf("session-%d", i)}
		if err := tracker.RequestStarted(RequestStart{
			Key:           key,
			RequestID:     "request",
			ContextWindow: 1_000,
			PromptTokens:  prompt,
		}); err != nil {
			t.Fatalf("start request %d: %v", i, err)
		}
		switch i {
		case 0:
			if err := tracker.RequestCompleted(ctx, RequestCompletion{
				Key:          key,
				RequestID:    "request",
				PromptTokens: prompt,
				Reusable:     true,
			}); err != nil {
				t.Fatalf("complete request %d: %v", i, err)
			}
		case 1:
			if err := tracker.RequestCompleted(ctx, RequestCompletion{
				Key:          key,
				RequestID:    "request",
				PromptTokens: prompt,
				Reusable:     false,
			}); err != nil {
				t.Fatalf("complete request %d: %v", i, err)
			}
		}
	}

	summary, err := tracker.Summary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Active != 2 || summary.Idle != 1 || summary.Completed != 1 || summary.Total != 4 {
		t.Fatalf("session counts: got active=%d idle=%d completed=%d total=%d", summary.Active, summary.Idle, summary.Completed, summary.Total)
	}
	if summary.Context.P50 != 200 || summary.Context.P90 != 400 || summary.Context.P99 != 400 || summary.Context.Max != 400 {
		t.Fatalf("context percentiles: got %+v", summary.Context)
	}
	if summary.Utilization.P50 != 0.2 || summary.Utilization.P90 != 0.4 || summary.Utilization.Max != 0.4 {
		t.Fatalf("utilization percentiles: got %+v", summary.Utilization)
	}
}

func TestTrackerShutdownPersistsLiveSessions(t *testing.T) {
	path := t.TempDir()
	tracker, err := New(Config{StorePath: path, MaxCompleted: 10})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}

	for i, reusable := range []bool{false, true} {
		key := Key{ModelID: "model", SessionID: fmt.Sprintf("session-%d", i)}
		if err := tracker.RequestStarted(RequestStart{Key: key, RequestID: "request"}); err != nil {
			t.Fatalf("start request %d: %v", i, err)
		}
		if reusable {
			if err := tracker.RequestCompleted(context.Background(), RequestCompletion{
				Key:       key,
				RequestID: "request",
				Reusable:  true,
			}); err != nil {
				t.Fatalf("idle request %d: %v", i, err)
			}
		}
	}

	if err := tracker.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	reopened, err := New(Config{StorePath: path, MaxCompleted: 10})
	if err != nil {
		t.Fatalf("reopen tracker: %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Shutdown(context.Background()); err != nil {
			t.Errorf("close reopened tracker: %v", err)
		}
	})

	page, err := reopened.List(context.Background(), StateCompleted, Query{})
	if err != nil {
		t.Fatalf("list completed: %v", err)
	}
	if len(page.Sessions) != 2 {
		t.Fatalf("completed summaries: got %d, want 2", len(page.Sessions))
	}
}

func newTestTracker(t *testing.T, maxCompleted int) *Tracker {
	t.Helper()

	tracker, err := New(Config{
		StorePath:    t.TempDir(),
		MaxCompleted: maxCompleted,
	})
	if err != nil {
		t.Fatalf("new tracker: %v", err)
	}
	t.Cleanup(func() {
		if err := tracker.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracker: %v", err)
		}
	})

	return tracker
}

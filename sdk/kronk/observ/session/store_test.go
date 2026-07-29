package session

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestStorePrunesOldestCompletedSummaries(t *testing.T) {
	tracker := newTestTracker(t, 20)
	ctx := context.Background()
	base := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)

	for i := range 21 {
		key := Key{ModelID: "model", SessionID: fmt.Sprintf("session-%02d", i)}
		started := base.Add(time.Duration(i) * time.Minute)
		if err := tracker.RequestStarted(RequestStart{
			Key:       key,
			RequestID: "request",
			StartedAt: started,
		}); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		if err := tracker.RequestCompleted(ctx, RequestCompletion{
			Key:         key,
			RequestID:   "request",
			CompletedAt: started,
			Reusable:    false,
		}); err != nil {
			t.Fatalf("complete %d: %v", i, err)
		}
	}

	counts := tracker.Counts()
	if counts.Completed != 19 {
		t.Fatalf("completed count after batched prune: got %d, want 19", counts.Completed)
	}

	page, err := tracker.List(ctx, StateCompleted, Query{Limit: 100})
	if err != nil {
		t.Fatalf("list completed: %v", err)
	}
	if len(page.Sessions) != 19 {
		t.Fatalf("completed page size: got %d, want 19", len(page.Sessions))
	}
	for _, summary := range page.Sessions {
		if summary.LastActiveAt.Equal(base) || summary.LastActiveAt.Equal(base.Add(time.Minute)) {
			t.Fatalf("old summary was not pruned: %s", summary.LastActiveAt)
		}
	}
}

func TestCompletedPagination(t *testing.T) {
	tracker := newTestTracker(t, 20)
	ctx := context.Background()
	base := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)

	for i := range 5 {
		key := Key{ModelID: "model", SessionID: fmt.Sprintf("session-%d", i)}
		started := base.Add(time.Duration(i) * time.Minute)
		if err := tracker.RequestStarted(RequestStart{Key: key, RequestID: "request", StartedAt: started}); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		if err := tracker.RequestCompleted(ctx, RequestCompletion{
			Key:         key,
			RequestID:   "request",
			CompletedAt: started,
			Reusable:    false,
		}); err != nil {
			t.Fatalf("complete %d: %v", i, err)
		}
	}

	first, err := tracker.List(ctx, StateCompleted, Query{Limit: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Sessions) != 2 || !first.HasMore || first.NextOffset != 2 {
		t.Fatalf("first page metadata: %+v", first)
	}

	second, err := tracker.List(ctx, StateCompleted, Query{Limit: 2, Offset: first.NextOffset})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Sessions) != 2 || !second.HasMore {
		t.Fatalf("second page metadata: %+v", second)
	}
	if first.Sessions[1].SessionID == second.Sessions[0].SessionID {
		t.Fatal("offset repeated the last item from the previous page")
	}

	third, err := tracker.List(ctx, StateCompleted, Query{Limit: 2, Offset: second.NextOffset})
	if err != nil {
		t.Fatalf("third page: %v", err)
	}
	if len(third.Sessions) != 1 || third.HasMore || third.NextOffset != 0 {
		t.Fatalf("third page metadata: %+v", third)
	}
}

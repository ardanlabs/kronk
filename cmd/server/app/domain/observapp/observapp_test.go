package observapp

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/session"
)

func TestStatusDisabled(t *testing.T) {
	resp := newApp(nil).status(context.Background(), httptest.NewRequest("GET", "/", nil))
	status, ok := resp.(Status)
	if !ok {
		t.Fatalf("response type: got %T, want Status", resp)
	}
	if status.Enabled {
		t.Fatal("enabled: got true, want false")
	}
}

func TestList(t *testing.T) {
	tracker := newTracker(t)
	key := session.Key{ModelID: "model", SessionID: "session"}
	if err := tracker.RequestStarted(session.RequestStart{
		Key:           key,
		RequestID:     "request",
		ContextWindow: 262_144,
		PromptTokens:  1_024,
	}); err != nil {
		t.Fatalf("start request: %v", err)
	}

	req := httptest.NewRequest("GET", "/?limit=25&offset=0&model_id=model&min_utilization=0", nil)
	req.SetPathValue("state", "active")
	resp := newApp(tracker).list(context.Background(), req)
	page, ok := resp.(Page)
	if !ok {
		t.Fatalf("response type: got %T, want Page", resp)
	}
	if len(page.Sessions) != 1 {
		t.Fatalf("sessions: got %d, want 1", len(page.Sessions))
	}
	if page.Sessions[0].SessionID != key.SessionID {
		t.Errorf("session ID: got %q, want %q", page.Sessions[0].SessionID, key.SessionID)
	}
}

func TestListInvalidQuery(t *testing.T) {
	tracker := newTracker(t)
	req := httptest.NewRequest("GET", "/?limit=101", nil)
	req.SetPathValue("state", "active")

	resp := newApp(tracker).list(context.Background(), req)
	appErr, ok := resp.(*errs.Error)
	if !ok {
		t.Fatalf("response type: got %T, want *errs.Error", resp)
	}
	if appErr.Code != errs.InvalidArgument {
		t.Errorf("error code: got %s, want %s", appErr.Code.String(), errs.InvalidArgument.String())
	}
}

func TestSummary(t *testing.T) {
	tracker := newTracker(t)
	key := session.Key{ModelID: "model", SessionID: "session"}
	if err := tracker.RequestStarted(session.RequestStart{
		Key:           key,
		RequestID:     "request",
		ContextWindow: 1_000,
		PromptTokens:  500,
	}); err != nil {
		t.Fatalf("start request: %v", err)
	}

	resp := newApp(tracker).summary(context.Background(), httptest.NewRequest("GET", "/", nil))
	summary, ok := resp.(Summary)
	if !ok {
		t.Fatalf("response type: got %T, want Summary", resp)
	}
	if summary.Active != 1 || summary.Idle != 0 || summary.Completed != 0 || summary.Total != 1 {
		t.Fatalf("session counts: got active=%d idle=%d completed=%d total=%d", summary.Active, summary.Idle, summary.Completed, summary.Total)
	}
	if summary.Context.P50 != 500 || summary.Context.P99 != 500 || summary.Context.Max != 500 {
		t.Fatalf("context percentiles: got %+v, want 500", summary.Context)
	}
	if summary.Utilization.P50 != 0.5 || summary.Utilization.P99 != 0.5 || summary.Utilization.Max != 0.5 {
		t.Fatalf("utilization percentiles: got %+v, want 0.5", summary.Utilization)
	}
}

func newTracker(t *testing.T) *session.Tracker {
	t.Helper()

	tracker, err := session.New(session.Config{StorePath: t.TempDir()})
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

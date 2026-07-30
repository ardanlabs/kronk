package observapp

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/session"
)

func TestStatusDisabled(t *testing.T) {
	resp := newApp(nil).list(context.Background(), httptest.NewRequest("GET", "/", nil))
	sessions, ok := resp.(Sessions)
	if !ok {
		t.Fatalf("response type: got %T, want Sessions", resp)
	}
	if sessions.Enabled {
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

	req := httptest.NewRequest("GET", "/", nil)
	resp := newApp(tracker).list(context.Background(), req)
	result, ok := resp.(Sessions)
	if !ok {
		t.Fatalf("response type: got %T, want Sessions", resp)
	}
	if !result.Enabled {
		t.Fatal("enabled: got false, want true")
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("sessions: got %d, want 1", len(result.Sessions))
	}
	if result.Sessions[0].SessionID != key.SessionID {
		t.Errorf("session ID: got %q, want %q", result.Sessions[0].SessionID, key.SessionID)
	}
}

func newTracker(t *testing.T) *session.Tracker {
	t.Helper()

	tracker := session.New()
	t.Cleanup(func() {
		if err := tracker.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracker: %v", err)
		}
	})

	return tracker
}

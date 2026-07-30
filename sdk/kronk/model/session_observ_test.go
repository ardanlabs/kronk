package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/session"
)

type recordingSessionObserver struct {
	starts           []session.RequestStart
	completions      []session.RequestCompletion
	completed        []session.Key
	completionCtxErr error
}

func (o *recordingSessionObserver) RequestStarted(event session.RequestStart) error {
	o.starts = append(o.starts, event)
	return nil
}

func (o *recordingSessionObserver) RequestCompleted(ctx context.Context, event session.RequestCompletion) error {
	o.completionCtxErr = ctx.Err()
	o.completions = append(o.completions, event)
	return nil
}

func (o *recordingSessionObserver) SessionCompleted(_ context.Context, key session.Key) error {
	o.completed = append(o.completed, key)
	return nil
}

func TestSessionObservationIMCLifecycle(t *testing.T) {
	observer := &recordingSessionObserver{}
	contextWindow := 1_000
	m := Model{
		cfg: Config{
			PtrContextWindow: new(contextWindow),
			SessionObserver:  observer,
		},
		modelInfo: ModelInfo{ID: "model"},
		log:       func(context.Context, string, ...any) {},
	}
	imc := &imcSession{
		id:                0,
		totalTokensCached: 100,
		kvState:           populatedTestSessionStore(),
		reserved:          true,
	}
	m.imcSessions = []*imcSession{imc}

	first := &slot{
		nPrompt: 150,
		job: &chatJob{
			id:                "request-1",
			ctx:               context.Background(),
			requestStart:      time.Now(),
			imcSession:        imc,
			imcSessionID:      imc.id,
			imcExpectedTokens: 100,
		},
	}
	m.observeRequestStarted(first)
	m.observeRequestCompleted(first, nil, 25)

	second := &slot{
		nPrompt: 230,
		job: &chatJob{
			id:                "request-2",
			ctx:               context.Background(),
			requestStart:      time.Now(),
			imcSession:        imc,
			imcSessionID:      imc.id,
			imcExpectedTokens: 150,
		},
	}
	m.observeRequestStarted(second)
	m.observeRequestCompleted(second, nil, 30)
	m.completeObservedIMCSessions(context.Background())

	if len(observer.starts) != 2 || len(observer.completions) != 2 {
		t.Fatalf("events: got starts=%d completions=%d, want 2 each", len(observer.starts), len(observer.completions))
	}
	if observer.starts[0].Key.SessionID != "request-1" || observer.starts[1].Key.SessionID != "request-1" {
		t.Fatalf("session IDs: got %q and %q, want request-1", observer.starts[0].Key.SessionID, observer.starts[1].Key.SessionID)
	}
	if got := observer.completions[0].CachedTokens; got != 100 {
		t.Errorf("first cached tokens: got %d, want 100", got)
	}
	if got := observer.completions[1].CachedTokens; got != 150 {
		t.Errorf("second cached tokens: got %d, want 150", got)
	}
	if !observer.completions[0].Reusable || !observer.completions[1].Reusable {
		t.Fatal("IMC completions should remain reusable")
	}
	if len(observer.completed) != 1 || observer.completed[0].SessionID != "request-1" {
		t.Fatalf("completed sessions: got %+v, want request-1", observer.completed)
	}
}

func TestSessionObservationCompletionIgnoresRequestCancellation(t *testing.T) {
	observer := &recordingSessionObserver{}
	contextWindow := 1_000
	m := Model{
		cfg: Config{
			PtrContextWindow: new(contextWindow),
			SessionObserver:  observer,
		},
		modelInfo: ModelInfo{ID: "model"},
		log:       func(context.Context, string, ...any) {},
	}

	ctx, cancel := context.WithCancel(context.Background())
	job := &chatJob{id: "request", ctx: ctx, requestStart: time.Now()}
	s := &slot{job: job, nPrompt: 700}
	m.observeRequestStarted(s)
	cancel()
	m.observeRequestCompleted(s, context.Canceled, 25)

	if observer.completionCtxErr != nil {
		t.Fatalf("completion context: got %v, want nil", observer.completionCtxErr)
	}
	if len(observer.completions) != 1 {
		t.Fatalf("completions: got %d, want 1", len(observer.completions))
	}
	if observer.completions[0].Reusable {
		t.Fatal("non-IMC request should not be reusable")
	}
	if observer.completions[0].ContextFull {
		t.Fatal("canceled request should not report context full")
	}
}

func TestDecodeErrorContextFull(t *testing.T) {
	if err := decodeError(1, nil); !errors.Is(err, errContextFull) {
		t.Fatalf("decode error: got %v, want errContextFull", err)
	}
	if err := decodeError(2, nil); errors.Is(err, errContextFull) {
		t.Fatalf("cancel decode error should not match errContextFull: %v", err)
	}
}

package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/ram"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// TestNewSessionStore_DefaultIsRAM verifies that an empty Config uses the RAM
// factory.
func TestNewSessionStore_DefaultIsRAM(t *testing.T) {
	store, err := newSessionStore(context.Background(), Config{})
	if err != nil {
		t.Fatalf("newSessionStore(Config{}) returned err = %v, want nil", err)
	}
	if _, ok := store.(*ram.Store); !ok {
		t.Errorf("newSessionStore(Config{}) returned %T, want *ram.Store", store)
	}
}

// TestNewSessionStore_InjectedRAM verifies that an injected RAM factory uses
// the same construction path as a custom factory.
func TestNewSessionStore_InjectedRAM(t *testing.T) {
	store, err := newSessionStore(context.Background(), Config{SessionStoreFactory: ram.NewFactory()})
	if err != nil {
		t.Fatalf("newSessionStore returned err = %v, want nil", err)
	}
	if _, ok := store.(*ram.Store); !ok {
		t.Errorf("newSessionStore returned %T, want *ram.Store", store)
	}
}

// TestNewSessionStore_CustomFactory verifies that an SDK-provided factory is
// invoked for each store Kronk needs.
func TestNewSessionStore_CustomFactory(t *testing.T) {
	var calls int
	cfg := NewConfig(
		WithSessionStoreFactory(func(ctx context.Context) (SessionStore, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			calls++
			return ram.New(), nil
		}),
	)

	store1, err := newSessionStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("newSessionStore(custom) err = %v, want nil", err)
	}
	store2, err := newSessionStore(context.Background(), cfg)
	if err != nil {
		t.Fatalf("newSessionStore(custom) second err = %v, want nil", err)
	}
	if store1 == store2 {
		t.Errorf("newSessionStore(custom) returned the same store %p twice", store1)
	}
	if calls != 2 {
		t.Errorf("factory calls = %d, want 2", calls)
	}
}

// TestNewSessionStore_CustomFactoryError verifies that construction failures
// from an SDK-provided factory are returned to the model loader.
func TestNewSessionStore_CustomFactoryError(t *testing.T) {
	wantErr := errors.New("unavailable")
	cfg := Config{
		SessionStoreFactory: func(context.Context) (SessionStore, error) {
			return nil, wantErr
		},
	}

	store, err := newSessionStore(context.Background(), cfg)
	if !errors.Is(err, wantErr) {
		t.Fatalf("newSessionStore(custom) err = %v, want %v", err, wantErr)
	}
	if store != nil {
		t.Errorf("newSessionStore(custom) store = %T, want nil", store)
	}
}

// TestNewSessionStore_CustomFactoryNilStore verifies that an invalid custom
// factory result fails during construction rather than panicking during use.
func TestNewSessionStore_CustomFactoryNilStore(t *testing.T) {
	cfg := Config{
		SessionStoreFactory: func(context.Context) (SessionStore, error) {
			return nil, nil
		},
	}

	store, err := newSessionStore(context.Background(), cfg)
	if err == nil {
		t.Fatal("newSessionStore(custom) err = nil, want non-nil")
	}
	if store != nil {
		t.Errorf("newSessionStore(custom) store = %T, want nil", store)
	}
}

func TestNewSessionStorePassesContext(t *testing.T) {
	type contextKey struct{}
	want := "request"
	ctx := context.WithValue(context.Background(), contextKey{}, want)
	cfg := Config{
		SessionStoreFactory: func(ctx context.Context) (SessionStore, error) {
			if got := ctx.Value(contextKey{}); got != want {
				return nil, errors.New("factory did not receive caller context")
			}
			return ram.New(), nil
		},
	}

	if _, err := newSessionStore(ctx, cfg); err != nil {
		t.Fatalf("newSessionStore() error = %v, want nil", err)
	}
}

func TestProcessIMCTokenPlanReturnsResetError(t *testing.T) {
	wantErr := errors.New("reset failed")
	m := Model{
		cfg: Config{PtrCacheMinTokens: new(1)},
		imcSessions: []*imcSession{{
			id:      0,
			kvState: &resetErrorStore{err: wantErr},
		}},
	}

	result := m.processIMCTokenPlan(context.Background(), D{"messages": []D{{"role": "user", "content": "hello"}}}, []llama.Token{1, 2, 3}, []llama.Token{1, 2}, nil, time.Now())
	if !errors.Is(result.err, wantErr) {
		t.Fatalf("processIMCTokenPlan() error = %v, want %v", result.err, wantErr)
	}
	if m.imcSessions[0].reserved {
		t.Error("session remained reserved after reset failure")
	}
}

func TestPrepareSessionSnapshotRejectsWrongLength(t *testing.T) {
	store := shortPrepareStore{}

	buf, err := prepareSessionSnapshot(context.Background(), &store, 4)
	if err == nil {
		t.Fatal("prepareSessionSnapshot() error = nil, want non-nil")
	}
	if buf != nil {
		t.Errorf("prepareSessionSnapshot() buffer length = %d, want nil", len(buf))
	}
}

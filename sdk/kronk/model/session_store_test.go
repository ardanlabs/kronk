package model

import (
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/ram"
)

// TestNewSessionStore_DefaultIsRAM verifies that an empty Config uses the RAM
// factory.
func TestNewSessionStore_DefaultIsRAM(t *testing.T) {
	store, err := newSessionStore(Config{})
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
	store, err := newSessionStore(Config{SessionStoreFactory: ram.NewFactory()})
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
		WithSessionStoreFactory(func() (SessionStore, error) {
			calls++
			return ram.New(), nil
		}),
	)

	store1, err := newSessionStore(cfg)
	if err != nil {
		t.Fatalf("newSessionStore(custom) err = %v, want nil", err)
	}
	store2, err := newSessionStore(cfg)
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
		SessionStoreFactory: func() (SessionStore, error) {
			return nil, wantErr
		},
	}

	store, err := newSessionStore(cfg)
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
		SessionStoreFactory: func() (SessionStore, error) {
			return nil, nil
		},
	}

	store, err := newSessionStore(cfg)
	if err == nil {
		t.Fatal("newSessionStore(custom) err = nil, want non-nil")
	}
	if store != nil {
		t.Errorf("newSessionStore(custom) store = %T, want nil", store)
	}
}

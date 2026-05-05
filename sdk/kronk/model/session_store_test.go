package model

import (
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/disk"
	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage/ram"
)

// TestNewSessionStore_DefaultIsRAM verifies that an empty
// Config.SessionStoreKind dispatches to the RAM backend.
func TestNewSessionStore_DefaultIsRAM(t *testing.T) {
	store, err := newSessionStore(Config{})
	if err != nil {
		t.Fatalf("newSessionStore(Config{}) returned err = %v, want nil", err)
	}
	if _, ok := store.(*ram.Store); !ok {
		t.Errorf("newSessionStore(Config{}) returned %T, want *ram.Store", store)
	}
}

// TestNewSessionStore_ExplicitRAM verifies that an explicit
// SessionStoreKindRAM dispatches to the RAM backend.
func TestNewSessionStore_ExplicitRAM(t *testing.T) {
	store, err := newSessionStore(Config{SessionStoreKind: SessionStoreKindRAM})
	if err != nil {
		t.Fatalf("newSessionStore returned err = %v, want nil", err)
	}
	if _, ok := store.(*ram.Store); !ok {
		t.Errorf("newSessionStore returned %T, want *ram.Store", store)
	}
}

// TestNewSessionStore_UnknownKindErrors verifies that an unrecognized
// SessionStoreKind value returns an error rather than silently
// falling back to RAM.
func TestNewSessionStore_UnknownKindErrors(t *testing.T) {
	store, err := newSessionStore(Config{SessionStoreKind: "bogus"})
	if err == nil {
		t.Fatalf("newSessionStore(bogus) err = nil, want non-nil")
	}
	if store != nil {
		t.Errorf("newSessionStore(bogus) store = %T, want nil", store)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("err = %v, want it to mention the offending kind", err)
	}
}

// TestNewSessionStore_Disk verifies that SessionStoreKindDisk
// dispatches to the disk backend when SessionStoreDir is set.
func TestNewSessionStore_Disk(t *testing.T) {
	cfg := Config{
		SessionStoreKind: SessionStoreKindDisk,
		SessionStoreDir:  t.TempDir(),
	}
	store, err := newSessionStore(cfg)
	if err != nil {
		t.Fatalf("newSessionStore(disk) err = %v, want nil", err)
	}
	defer func() {
		_ = store.Close()
	}()
	if _, ok := store.(*disk.Store); !ok {
		t.Errorf("newSessionStore(disk) returned %T, want *disk.Store", store)
	}
}

// TestNewSessionStore_DiskRequiresDir verifies that selecting the
// disk backend without SessionStoreDir surfaces an error from the
// disk constructor.
func TestNewSessionStore_DiskRequiresDir(t *testing.T) {
	store, err := newSessionStore(Config{SessionStoreKind: SessionStoreKindDisk})
	if err == nil {
		t.Fatalf("newSessionStore(disk, no dir) err = nil, want non-nil")
	}
	if store != nil {
		t.Errorf("newSessionStore(disk, no dir) store = %T, want nil", store)
	}
}

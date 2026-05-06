package pool_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/resman"
)

// Test_FailedLoadReleasesReservation verifies that when kronk.NewWithContext
// fails after the resman reservation succeeds, the reservation is released
// (no resource leak), the cache key is not populated, the documented log
// status is emitted, and a subsequent valid load using the same memory
// budget still succeeds.
func Test_FailedLoadReleasesReservation(t *testing.T) {
	// initKronk installs/validates libs; it is idempotent and shared with
	// the main Test_Pool entry point. Use it so this test is runnable on
	// its own.
	_ = initKronk(t)

	// Use a real catalog model ID so planRequest's VRAM lookup succeeds.
	// The cfg.ModelFiles below points to a nonexistent path so
	// kronk.NewWithContext will fail in validateConfig, AFTER the resman
	// reservation has been made.
	modelID := findAvailableModel(t, "")
	key := fmt.Sprintf("%s/playground/failed-load-test", modelID)

	var logBuf bytes.Buffer
	captureLog := func(ctx context.Context, msg string, args ...any) {
		sl := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		sl.InfoContext(ctx, msg, args...)
	}

	cfg := pool.Config{
		Log:          captureLog,
		ModelsInPool: 2,
		TTL:          5 * time.Minute,
	}

	mgr, err := pool.New(cfg)
	if err != nil {
		t.Fatalf("pool.New: %v", err)
	}
	defer mgr.Shutdown(context.Background())

	t.Cleanup(func() {
		t.Log("=====================")
		t.Log(logBuf.String())
		t.Log("=====================")
	})

	// ------------------------------------------------------------------
	// Snapshot resman usage BEFORE the failing load.

	before := mgr.ResourceManager().Usage()
	beforeReservations := len(before.Reservations)
	beforeRAMUsed := before.RAMUsed
	beforeDeviceUsed := snapshotDeviceUsed(before)

	// ------------------------------------------------------------------
	// Trigger a failed load: real modelID (so planRequest succeeds) but a
	// nonexistent model file (so NewWithContext fails in validateConfig).

	badCfg := model.Config{
		ModelFiles: []string{"/nonexistent/path/does-not-exist.gguf"},
	}

	ctx := context.Background()
	_, err = mgr.AquireCustom(ctx, key, badCfg)
	if err == nil {
		t.Fatal("expected error from AquireCustom with bad model file path, got nil")
	}
	if !strings.Contains(err.Error(), "unable to create inference model") {
		t.Errorf("expected error to mention 'unable to create inference model', got: %v", err)
	}

	// ------------------------------------------------------------------
	// Snapshot resman usage AFTER. Nothing should have leaked.

	after := mgr.ResourceManager().Usage()

	if got, want := len(after.Reservations), beforeReservations; got != want {
		t.Errorf("reservations leaked: before=%d after=%d", want, got)
	}

	for _, r := range after.Reservations {
		if r.Key == key {
			t.Errorf("failing key %q is still in resman.Reservations: %+v", key, r)
		}
	}

	if after.RAMUsed != beforeRAMUsed {
		t.Errorf("RAMUsed leaked: before=%d after=%d (delta=%d)", beforeRAMUsed, after.RAMUsed, after.RAMUsed-beforeRAMUsed)
	}

	afterDeviceUsed := snapshotDeviceUsed(after)
	for name, want := range beforeDeviceUsed {
		got := afterDeviceUsed[name]
		if got != want {
			t.Errorf("device %q VRAM leaked: before=%d after=%d (delta=%d)", name, want, got, got-want)
		}
	}

	// ------------------------------------------------------------------
	// The failing key must not be cached.

	if _, ok := mgr.GetExisting(key); ok {
		t.Errorf("failing key %q was cached; expected absent", key)
	}

	// ------------------------------------------------------------------
	// The pool must log the documented release status.

	logs := logBuf.String()
	if !strings.Contains(logs, "load-failed-reservation-released") {
		t.Errorf("expected log to contain 'load-failed-reservation-released'; pool log:\n%s", logs)
	}

	// ------------------------------------------------------------------
	// A subsequent successful load with the same model must still work.
	// If the failed reservation had leaked, this could fail (or be forced
	// into eviction); either way the assertion proves the budget is clean.

	krn, err := mgr.AquireModel(ctx, modelID)
	if err != nil {
		t.Fatalf("subsequent AquireModel after failed load failed: %v", err)
	}
	if krn == nil {
		t.Fatal("subsequent AquireModel returned nil kronk")
	}
}

// snapshotDeviceUsed returns a map of device-name → UsedBytes for easy
// before/after comparison.
func snapshotDeviceUsed(u resman.Usage) map[string]int64 {
	out := make(map[string]int64, len(u.Devices))
	for _, d := range u.Devices {
		out[d.Name] = d.UsedBytes
	}
	return out
}

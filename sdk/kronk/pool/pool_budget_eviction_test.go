package pool_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

// MiB is a readable byte-size constant for sizing synthetic snapshots.
const MiB int64 = 1 << 20

// newSyntheticPool builds a pool wired to a resman seeded with the
// supplied synthetic snapshot at 100% budget. Test-specific Models
// come from the global testModels initialized by initKronk.
func newSyntheticPool(t *testing.T, snap resman.Snapshot, modelConfigFile string) *pool.Pool {
	t.Helper()
	startup := devices.Devices{
		SystemRAMBytes: uint64(max(snap.RAMBytes, 0)),
		UnifiedMemory:  snap.UnifiedMemory,
	}
	for i, device := range snap.Devices {
		startup.Devices = append(startup.Devices, devices.DeviceInfo{
			Index:      i,
			Name:       device.Name,
			Type:       device.Type,
			FreeBytes:  uint64(max(device.TotalBytes, 0)),
			TotalBytes: uint64(max(device.TotalBytes, 0)),
		})
		startup.GPUCount++
		startup.GPUTotalBytes += uint64(max(device.TotalBytes, 0))
		startup.SupportsGPUOffload = true
	}
	if snap.UnifiedMemory && len(startup.Devices) == 0 {
		memory := uint64(max(snap.RAMBytes, 0))
		startup.Devices = []devices.DeviceInfo{{
			Name:       "Metal",
			Type:       "gpu_metal",
			FreeBytes:  memory,
			TotalBytes: memory,
		}}
		startup.GPUCount = 1
		startup.GPUTotalBytes = memory
		startup.SupportsGPUOffload = true
	}

	rm, err := resman.New(resman.Config{
		Snapshot:      snap,
		BudgetPercent: 100,
	})
	if err != nil {
		t.Fatalf("resman.New: %v", err)
	}
	mgr, err := pool.New(pool.Config{
		Log:             log,
		Models:          testModels,
		Resman:          rm,
		StartupDevices:  &startup,
		ModelConfigFile: modelConfigFile,
		ModelsInPool:    10,
		TTL:             5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("pool.New: %v", err)
	}
	return mgr
}

// probeRAMBudget is sized far above any realistic CI model footprint so
// the probe phase never triggers eviction; the real reservation bytes
// come back unchanged from resman.Usage.
const probeRAMBudget int64 = 1 << 40

// Test_BudgetEviction verifies the end-to-end budget-driven eviction
// path in pool.reserveWithEviction by injecting a synthetic
// resman.Snapshot. CI's small models fit comfortably inside the real
// host's budget, so without a synthetic snapshot these scenarios are
// unreachable on real hardware.
//
// The injected snapshot uses UnifiedMemory=true with no GPU devices so
// the resman charges the entire predicted footprint to a single RAM
// pool, regardless of the host's actual device topology. This keeps the
// budget math trivial and the test platform-agnostic.
//
// The test does a one-time probe under a permissive snapshot to learn
// each model's predicted reservation, then sizes the per-subtest
// snapshot relative to those measured values. This makes the tests
// self-tune to whatever model files CI happens to have installed.
func Test_BudgetEviction(t *testing.T) {
	log = initKronk(t)

	modelA := findAvailableModel(t, "")
	modelB := findAvailableModel(t, modelA)
	modelConfigFile := fixedSizingConfig(t, modelA, modelB)

	sizeA, sizeB := probeFootprints(t, modelConfigFile, modelA, modelB)

	t.Logf("probed footprints: %s=%s, %s=%s",
		modelA, pool.HumanBytes(sizeA),
		modelB, pool.HumanBytes(sizeB))

	t.Run("evicts-loaded-model-when-second-load-exceeds-budget", func(t *testing.T) {
		evictsOnSecondLoad(t, modelConfigFile, modelA, modelB, sizeA, sizeB)
	})

	t.Run("rejects-infeasible-request-without-eviction", func(t *testing.T) {
		rejectsInfeasibleRequest(t, modelConfigFile, modelA)
	})

	t.Run("release-restores-budget", func(t *testing.T) {
		releaseRestoresBudget(t, modelConfigFile, modelA, sizeA)
	})
}

// fixedSizingConfig prevents AutoTune from changing the memory-affecting
// settings when the synthetic budget changes between the probe and the
// admission scenarios. AutoTune's budget behavior has its own unit tests;
// this test needs stable footprints to isolate resource-manager eviction.
func fixedSizingConfig(t *testing.T, modelA, modelB string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "model_config.yaml")
	data := fmt.Appendf(nil, `%q:
  context-window: 4096
  cache-type-k: f16
  cache-type-v: f16
%q:
  context-window: 4096
  cache-type-k: f16
  cache-type-v: f16
`, modelA, modelB)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write model config: %v", err)
	}

	return path
}

// probeFootprints loads each model under a permissive synthetic
// snapshot and returns the total predicted footprint
// (RAMBytes+VRAMBytes) the resman charged for the reservation. The pool
// is shut down before returning so the probe holds no models in memory
// during the subtests that follow.
func probeFootprints(t *testing.T, modelConfigFile, modelA, modelB string) (int64, int64) {
	t.Helper()

	snap := resman.Snapshot{UnifiedMemory: true, RAMBytes: probeRAMBudget}
	mgr := newSyntheticPool(t, snap, modelConfigFile)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := mgr.Shutdown(ctx); err != nil {
			t.Logf("probe: shutdown: %v", err)
		}
	}()

	ctx := context.Background()
	if _, err := mgr.AquireModel(ctx, modelA); err != nil {
		t.Fatalf("probe: acquire %s: %v", modelA, err)
	}
	if _, err := mgr.AquireModel(ctx, modelB); err != nil {
		t.Fatalf("probe: acquire %s: %v", modelB, err)
	}

	var a, b int64
	for _, r := range mgr.ResourceManager().Usage().Reservations {
		switch r.Key {
		case modelA:
			a = r.RAMBytes + r.VRAMBytes
		case modelB:
			b = r.RAMBytes + r.VRAMBytes
		}
	}
	if a == 0 || b == 0 {
		t.Fatalf("probe: missing reservation a=%d b=%d", a, b)
	}
	return a, b
}

// evictsOnSecondLoad sizes the budget so either model fits alone but
// not both. Loading B after A must trigger reserveWithEviction's
// budget-driven eviction loop, unload A, then admit B.
func evictsOnSecondLoad(t *testing.T, modelConfigFile, modelA, modelB string, sizeA, sizeB int64) {
	// Size the snapshot so the *effective* RAM budget (snapshot minus the
	// resman's 5% headroom) is just large enough to hold the larger of
	// the two models alone, with a 64 MiB slack for plan-time variance.
	// Loading the second model must therefore push the pool over budget
	// and trigger reserveWithEviction.
	maxFootprint := max(sizeA, sizeB)
	snap := resman.Snapshot{
		UnifiedMemory: true,
		RAMBytes:      maxFootprint*100/95 + 64*MiB,
	}
	// ModelsInPool cap is well above 2; budget drives eviction.
	mgr := newSyntheticPool(t, snap, modelConfigFile)
	defer mgr.Shutdown(context.Background())

	ctx := context.Background()

	if _, err := mgr.AquireModel(ctx, modelA); err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	if got := numReservations(mgr); got != 1 {
		t.Fatalf("after A: reservations=%d, want 1", got)
	}

	if _, err := mgr.AquireModel(ctx, modelB); err != nil {
		t.Fatalf("acquire B (must evict A): %v", err)
	}

	// Otter eviction is asynchronous — the callback releases A's
	// ticket on its own goroutine. Allow a bounded window for it to
	// drain before asserting on the resman's view.
	if err := waitForReservations(mgr, []string{modelB}, 30*time.Second); err != nil {
		t.Fatalf("post-eviction reservations: %v", err)
	}

	u := mgr.ResourceManager().Usage()
	if u.RAMUsed > u.RAMBudget {
		t.Errorf("RAMUsed=%d > RAMBudget=%d after eviction", u.RAMUsed, u.RAMBudget)
	}
}

// rejectsInfeasibleRequest gives the pool a budget smaller than the
// model's predicted footprint. checkRequestFitsBudget must reject the
// request up front; no eviction should run because there is no
// reservation that could free enough budget.
func rejectsInfeasibleRequest(t *testing.T, modelConfigFile, modelA string) {
	snap := resman.Snapshot{UnifiedMemory: true, RAMBytes: 1024}
	mgr := newSyntheticPool(t, snap, modelConfigFile)
	defer mgr.Shutdown(context.Background())

	_, err := mgr.AquireModel(context.Background(), modelA)
	if err == nil {
		t.Fatal("expected error for infeasible request, got nil")
	}
	// checkRequestFitsBudget produces "...max ram budget is..." for RAM
	// rejections. We assert on the substring to stay resilient to
	// future wrapping changes.
	if !strings.Contains(err.Error(), "ram budget") {
		t.Errorf("unexpected error: %v", err)
	}

	if got := numReservations(mgr); got != 0 {
		t.Errorf("reservations after rejection: got %d, want 0", got)
	}
}

// releaseRestoresBudget verifies that invalidating a loaded model
// returns its bytes to the budget so a re-acquire under the same tight
// snapshot succeeds.
func releaseRestoresBudget(t *testing.T, modelConfigFile, modelA string, sizeA int64) {
	// Account for the resman's 5% headroom so its effective budget can hold
	// the probed footprint plus a small allowance for plan-time variance.
	snap := resman.Snapshot{UnifiedMemory: true, RAMBytes: sizeA*100/95 + 64*MiB}
	mgr := newSyntheticPool(t, snap, modelConfigFile)
	defer mgr.Shutdown(context.Background())

	ctx := context.Background()
	if _, err := mgr.AquireModel(ctx, modelA); err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	if mgr.ResourceManager().Usage().RAMUsed == 0 {
		t.Fatal("RAMUsed=0 after acquire")
	}

	if err := mgr.InvalidateSync(ctx, modelA); err != nil {
		t.Fatalf("invalidate-sync: %v", err)
	}

	if u := mgr.ResourceManager().Usage(); u.RAMUsed != 0 {
		t.Errorf("RAMUsed=%d after invalidate-sync, want 0", u.RAMUsed)
	}

	// Re-acquire to confirm the budget is fully restored.
	if _, err := mgr.AquireModel(ctx, modelA); err != nil {
		t.Fatalf("re-acquire A after release: %v", err)
	}
}

// =============================================================================

// numReservations returns the count of reservations the resource
// manager is currently tracking.
func numReservations(mgr *pool.Pool) int {
	return len(mgr.ResourceManager().Usage().Reservations)
}

// waitForReservations polls the resource manager until the set of
// reserved keys exactly matches want, or returns an error on timeout.
func waitForReservations(mgr *pool.Pool, want []string, timeout time.Duration) error {
	wantSet := make(map[string]struct{}, len(want))
	for _, k := range want {
		wantSet[k] = struct{}{}
	}

	deadline := time.Now().Add(timeout)
	for {
		got := make(map[string]struct{})
		for _, r := range mgr.ResourceManager().Usage().Reservations {
			got[r.Key] = struct{}{}
		}

		if equalSets(got, wantSet) {
			return nil
		}

		if time.Now().After(deadline) {
			actual := make([]string, 0, len(got))
			for k := range got {
				actual = append(actual, k)
			}
			return fmt.Errorf("timeout: have %v want %v", actual, want)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func equalSets(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

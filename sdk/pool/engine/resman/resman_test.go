package resman_test

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
)

// Common byte-size constants for readable test scenarios.
const (
	GiB int64 = 1 << 30
	MiB int64 = 1 << 20
)

// snapshot24_12 returns a snapshot with two GPUs of asymmetric VRAM (24 GB
// and 12 GB) and 64 GB of system RAM.
func snapshot24_12() resman.Snapshot {
	return resman.Snapshot{
		Devices: []resman.Device{
			{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: 24 * GiB},
			{Name: "CUDA1", Type: "gpu_cuda", TotalBytes: 12 * GiB},
		},
		RAMBytes: 64 * GiB,
	}
}

// snapshotSingle returns a snapshot with one 16 GB GPU and 32 GB of RAM.
func snapshotSingle() resman.Snapshot {
	return resman.Snapshot{
		Devices: []resman.Device{
			{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: 16 * GiB},
		},
		RAMBytes: 32 * GiB,
	}
}

// noHeadroom returns a Config that disables headroom so budget math is
// exactly BudgetPercent of TotalBytes.
func noHeadroom(snap resman.Snapshot, pct int) resman.Config {
	return resman.Config{
		Snapshot:      snap,
		BudgetPercent: pct,
		HeadroomBytes: -1, // negative is clamped to 0 in New.
	}
}

func Test_New_Defaults(t *testing.T) {
	m, err := resman.New(resman.Config{Snapshot: snapshotSingle()})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	u := m.Usage()
	if u.BudgetPercent != resman.DefaultBudgetPercent {
		t.Errorf("BudgetPercent: want %d, got %d", resman.DefaultBudgetPercent, u.BudgetPercent)
	}
	if u.HeadroomBytes != resman.DefaultHeadroomBytes {
		t.Errorf("HeadroomBytes: want %d, got %d", resman.DefaultHeadroomBytes, u.HeadroomBytes)
	}
	if len(u.Devices) != 1 {
		t.Fatalf("Devices: want 1, got %d", len(u.Devices))
	}

	// 80% of 16 GiB minus 256 MiB headroom.
	total := float64(16 * GiB)
	wantBudget := int64(total*0.8) - int64(resman.DefaultHeadroomBytes)
	if u.Devices[0].BudgetBytes != wantBudget {
		t.Errorf("BudgetBytes: want %d, got %d", wantBudget, u.Devices[0].BudgetBytes)
	}
}

func Test_New_BadConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  resman.Config
	}{
		{"too low", resman.Config{Snapshot: snapshotSingle(), BudgetPercent: -5}},
		{"too high", resman.Config{Snapshot: snapshotSingle(), BudgetPercent: 101}},
		{"duplicate device", resman.Config{
			Snapshot: resman.Snapshot{
				Devices: []resman.Device{
					{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: GiB},
					{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: GiB},
				},
			},
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := resman.New(c.cfg); err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func Test_New_IgnoresCPU(t *testing.T) {
	snap := resman.Snapshot{
		Devices: []resman.Device{
			{Name: "CPU", Type: "cpu", TotalBytes: 1 * GiB},
			{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: 8 * GiB},
		},
	}

	m, err := resman.New(resman.Config{Snapshot: snap, BudgetPercent: 100, HeadroomBytes: -1})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	u := m.Usage()
	if len(u.Devices) != 1 {
		t.Fatalf("only the GPU should be tracked, got %d devices", len(u.Devices))
	}
	if u.Devices[0].Name != "CUDA0" {
		t.Errorf("device: want CUDA0, got %s", u.Devices[0].Name)
	}
}

func Test_Reserve_PreventsOOM_SingleGPU(t *testing.T) {
	// 80% of 16 GiB, no headroom = 12.8 GiB budget.
	m, err := resman.New(noHeadroom(snapshotSingle(), 80))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	total := float64(16 * GiB)
	wantBudget := int64(total * 0.8)
	if got := m.Usage().Devices[0].BudgetBytes; got != wantBudget {
		t.Fatalf("BudgetBytes: want %d, got %d", wantBudget, got)
	}

	// A 13 GiB model exceeds the 12.8 GiB budget — must be rejected even
	// though the device physically holds 16 GiB.
	_, _, err = m.Reserve(resman.PlanRequest{Key: "big", VRAMBytes: 13 * GiB})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity, got: %v", err)
	}

	if got := m.Usage().Devices[0].UsedBytes; got != 0 {
		t.Errorf("a failed reservation must not consume budget; got UsedBytes=%d", got)
	}

	// A 12 GiB model fits.
	_, _, err = m.Reserve(resman.PlanRequest{Key: "ok", VRAMBytes: 12 * GiB})
	if err != nil {
		t.Fatalf("expected fit, got: %v", err)
	}
}

func Test_Reserve_NeverExceedsBudget(t *testing.T) {
	// Two GPUs, 100% budget so we can fill exactly to TotalBytes.
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Fill CUDA0 to exactly its budget.
	_, _, err = m.Reserve(resman.PlanRequest{
		Key: "fill0", VRAMBytes: 24 * GiB, Devices: []string{"CUDA0"},
	})
	if err != nil {
		t.Fatalf("fill CUDA0: %v", err)
	}

	// One byte more on CUDA0 must fail.
	_, _, err = m.Reserve(resman.PlanRequest{
		Key: "overflow0", VRAMBytes: 1, Devices: []string{"CUDA0"},
	})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity on saturated CUDA0, got: %v", err)
	}

	// CUDA1 still has its full 12 GiB budget.
	_, _, err = m.Reserve(resman.PlanRequest{
		Key: "fill1", VRAMBytes: 12 * GiB, Devices: []string{"CUDA1"},
	})
	if err != nil {
		t.Fatalf("fill CUDA1: %v", err)
	}

	u := m.Usage()
	for _, d := range u.Devices {
		if d.UsedBytes > d.BudgetBytes {
			t.Errorf("device[%s] used=%d > budget=%d", d.Name, d.UsedBytes, d.BudgetBytes)
		}
	}
}

func Test_Reserve_DoesNotSumAcrossGPUs(t *testing.T) {
	// 24 GiB + 12 GiB = 36 GiB total VRAM, but a single 20 GiB model still
	// has to fit on ONE card. A 30 GiB model that "would fit" if we summed
	// the cards must still be rejected.
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	_, _, err = m.Reserve(resman.PlanRequest{Key: "huge", VRAMBytes: 30 * GiB})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("a 30 GiB model must not be admitted across 24+12 GiB cards: %v", err)
	}
}

func Test_Reserve_FreeChoicePicksLargestRoom(t *testing.T) {
	// With 24 GiB free on CUDA0 and 12 GiB free on CUDA1, a 10 GiB model
	// should land on CUDA0 (the card with most headroom).
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	_, plan, err := m.Reserve(resman.PlanRequest{Key: "m1", VRAMBytes: 10 * GiB})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if len(plan.Per) != 1 || plan.Per[0].Name != "CUDA0" {
		t.Fatalf("expected placement on CUDA0, got %+v", plan.Per)
	}

	// Now CUDA0 has 14 GiB free, CUDA1 has 12 GiB. Next 10 GiB model still
	// goes to CUDA0 (14 > 12).
	_, plan, err = m.Reserve(resman.PlanRequest{Key: "m2", VRAMBytes: 10 * GiB})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if len(plan.Per) != 1 || plan.Per[0].Name != "CUDA0" {
		t.Fatalf("expected placement on CUDA0, got %+v", plan.Per)
	}

	// Now CUDA0 has 4 GiB free, CUDA1 has 12 GiB. A 10 GiB model now goes
	// to CUDA1.
	_, plan, err = m.Reserve(resman.PlanRequest{Key: "m3", VRAMBytes: 10 * GiB})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if len(plan.Per) != 1 || plan.Per[0].Name != "CUDA1" {
		t.Fatalf("expected placement on CUDA1, got %+v", plan.Per)
	}
}

func Test_Reserve_PinnedDevice(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Pin to the smaller card; a 16 GiB model must fail even though
	// CUDA0 has plenty of room.
	_, _, err = m.Reserve(resman.PlanRequest{
		Key: "pinned-fail", VRAMBytes: 16 * GiB, Devices: []string{"CUDA1"},
	})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("pinned to 12 GiB card, 16 GiB request must fail: %v", err)
	}

	// CUDA0 must still be untouched.
	if m.Usage().Devices[0].UsedBytes != 0 {
		t.Errorf("failed pinned reservation must not touch other devices")
	}

	// Unknown device.
	_, _, err = m.Reserve(resman.PlanRequest{
		Key: "ghost", VRAMBytes: GiB, Devices: []string{"CUDA9"},
	})
	if !errors.Is(err, resman.ErrUnknownDevice) {
		t.Fatalf("expected ErrUnknownDevice, got: %v", err)
	}
}

func Test_Reserve_TensorSplit(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// 30 GiB model split 60/40 across CUDA0/CUDA1: 18 GiB / 12 GiB.
	// CUDA1 has exactly 12 GiB budget — should fit.
	_, plan, err := m.Reserve(resman.PlanRequest{
		Key:         "split",
		VRAMBytes:   30 * GiB,
		Devices:     []string{"CUDA0", "CUDA1"},
		TensorSplit: []float32{0.6, 0.4},
	})
	if err != nil {
		t.Fatalf("split reserve: %v", err)
	}

	var sum int64
	for _, a := range plan.Per {
		sum += a.Bytes
	}
	if sum != 30*GiB {
		t.Errorf("split allocations must sum to VRAMBytes; got %d want %d", sum, 30*GiB)
	}

	// Verify neither bucket overflows.
	for _, d := range m.Usage().Devices {
		if d.UsedBytes > d.BudgetBytes {
			t.Errorf("device[%s] over budget after split: used=%d budget=%d",
				d.Name, d.UsedBytes, d.BudgetBytes)
		}
	}

	// Now a split that would need >12 GiB on CUDA1 must fail.
	_, _, err = m.Reserve(resman.PlanRequest{
		Key:         "split-fail",
		VRAMBytes:   20 * GiB,
		Devices:     []string{"CUDA0", "CUDA1"},
		TensorSplit: []float32{0.5, 0.5}, // 10 GiB each — but CUDA1 only has 0 GiB free now.
	})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity on saturated split, got: %v", err)
	}
}

func Test_Reserve_TensorSplitMismatch(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	_, _, err = m.Reserve(resman.PlanRequest{
		Key:         "bad",
		VRAMBytes:   GiB,
		Devices:     []string{"CUDA0", "CUDA1"},
		TensorSplit: []float32{1.0},
	})
	if !errors.Is(err, resman.ErrInvalidPlan) {
		t.Fatalf("expected ErrInvalidPlan, got: %v", err)
	}
}

func Test_Release_RestoresCapacity(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshotSingle(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	t1, _, err := m.Reserve(resman.PlanRequest{Key: "a", VRAMBytes: 14 * GiB})
	if err != nil {
		t.Fatalf("reserve a: %v", err)
	}

	// 4 GiB more would overflow (14+4 > 16).
	_, _, err = m.Reserve(resman.PlanRequest{Key: "b", VRAMBytes: 4 * GiB})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity before release, got: %v", err)
	}

	m.Release(t1)
	if got := m.Usage().Devices[0].UsedBytes; got != 0 {
		t.Errorf("UsedBytes after release: want 0, got %d", got)
	}

	// After release the same request fits.
	_, _, err = m.Reserve(resman.PlanRequest{Key: "b", VRAMBytes: 4 * GiB})
	if err != nil {
		t.Fatalf("reserve b after release: %v", err)
	}
}

func Test_Release_UnknownTicket(t *testing.T) {
	m, err := resman.New(resman.Config{Snapshot: snapshotSingle()})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Should not panic or change state.
	m.Release(resman.Ticket{Key: "nope"})
	m.Release(resman.Ticket{})
}

func Test_Reserve_DuplicateKey(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshotSingle(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if _, _, err := m.Reserve(resman.PlanRequest{Key: "dup", VRAMBytes: GiB}); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if _, _, err := m.Reserve(resman.PlanRequest{Key: "dup", VRAMBytes: GiB}); !errors.Is(err, resman.ErrDuplicateKey) {
		t.Fatalf("expected ErrDuplicateKey, got: %v", err)
	}
}

func Test_Reserve_RAMBudget(t *testing.T) {
	snap := resman.Snapshot{
		Devices:  []resman.Device{{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: 8 * GiB}},
		RAMBytes: 10 * GiB,
	}
	m, err := resman.New(resman.Config{Snapshot: snap, BudgetPercent: 100, HeadroomBytes: -1})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if _, _, err := m.Reserve(resman.PlanRequest{Key: "a", RAMBytes: 6 * GiB}); err != nil {
		t.Fatalf("reserve a: %v", err)
	}
	if _, _, err := m.Reserve(resman.PlanRequest{Key: "b", RAMBytes: 5 * GiB}); !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected RAM ErrNoCapacity, got: %v", err)
	}
}

func Test_Reserve_NoVRAMNeeded(t *testing.T) {
	// A model with VRAMBytes=0 (e.g. CPU-only embedding) should always
	// reserve successfully even when no GPUs are present.
	m, err := resman.New(resman.Config{
		Snapshot: resman.Snapshot{RAMBytes: 8 * GiB},
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if _, _, err := m.Reserve(resman.PlanRequest{Key: "cpu-only"}); err != nil {
		t.Fatalf("reserve: %v", err)
	}
}

func Test_Reserve_NoGPUsButVRAMNeeded(t *testing.T) {
	m, err := resman.New(resman.Config{Snapshot: resman.Snapshot{RAMBytes: 8 * GiB}})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	_, _, err = m.Reserve(resman.PlanRequest{Key: "needs-gpu", VRAMBytes: GiB})
	if !errors.Is(err, resman.ErrNoGPUs) {
		t.Fatalf("expected ErrNoGPUs, got: %v", err)
	}
}

// Test_Reserve_ConcurrentNeverExceedsBudget hammers the manager with many
// goroutines all trying to reserve. The total successful allocation on each
// device must never exceed its budget — this is the OOM-prevention invariant
// under concurrency.
func Test_Reserve_ConcurrentNeverExceedsBudget(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	const (
		workers      = 64
		perWorker    = 50
		modelVRAM    = 1 * GiB // each successful reservation costs 1 GiB.
		expectedFits = 24 + 12 // total budget across both cards in GiB.
	)

	var (
		wg       sync.WaitGroup
		attempts atomic.Int64
		fits     atomic.Int64
		mu       sync.Mutex
		tickets  []resman.Ticket
	)

	for w := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range perWorker {
				key := fmt.Sprintf("w%d-i%d", id, i)
				attempts.Add(1)
				ticket, _, err := m.Reserve(resman.PlanRequest{Key: key, VRAMBytes: modelVRAM})
				if err == nil {
					fits.Add(1)
					mu.Lock()
					tickets = append(tickets, ticket)
					mu.Unlock()
					continue
				}
				if !errors.Is(err, resman.ErrNoCapacity) {
					t.Errorf("unexpected error on %s: %v", key, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	// Invariant 1: per-device used bytes never exceeds budget.
	u := m.Usage()
	for _, d := range u.Devices {
		if d.UsedBytes > d.BudgetBytes {
			t.Errorf("device[%s] used=%d > budget=%d", d.Name, d.UsedBytes, d.BudgetBytes)
		}
	}

	// Invariant 2: exactly the budget's worth of 1 GiB models was admitted.
	// 24 + 12 = 36 reservations of 1 GiB each.
	if int64(fits.Load()) != int64(expectedFits) {
		t.Errorf("admitted=%d, want=%d (attempts=%d)", fits.Load(), expectedFits, attempts.Load())
	}

	// Invariant 3: total used == total admitted.
	var totalUsed int64
	for _, d := range u.Devices {
		totalUsed += d.UsedBytes
	}
	if totalUsed != int64(fits.Load())*modelVRAM {
		t.Errorf("totalUsed=%d, want=%d", totalUsed, int64(fits.Load())*modelVRAM)
	}

	// Release everything and confirm we land exactly at zero.
	for _, ticket := range tickets {
		m.Release(ticket)
	}
	for _, d := range m.Usage().Devices {
		if d.UsedBytes != 0 {
			t.Errorf("after full release device[%s].UsedBytes=%d, want 0", d.Name, d.UsedBytes)
		}
	}
}

// Test_Reserve_FailedDoesNotMutate verifies that no partial state is committed
// when a multi-device split fails on the second device.
func Test_Reserve_FailedDoesNotMutate(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Saturate CUDA1 first.
	if _, _, err := m.Reserve(resman.PlanRequest{
		Key: "saturate1", VRAMBytes: 12 * GiB, Devices: []string{"CUDA1"},
	}); err != nil {
		t.Fatalf("saturate: %v", err)
	}

	before := m.Usage()

	// Try a split that needs CUDA1 — must fail without touching CUDA0.
	_, _, err = m.Reserve(resman.PlanRequest{
		Key:         "split-bad",
		VRAMBytes:   10 * GiB,
		Devices:     []string{"CUDA0", "CUDA1"},
		TensorSplit: []float32{0.5, 0.5},
	})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity, got: %v", err)
	}

	after := m.Usage()
	for i, d := range after.Devices {
		if d.UsedBytes != before.Devices[i].UsedBytes {
			t.Errorf("device[%s] mutated by failed split: before=%d after=%d",
				d.Name, before.Devices[i].UsedBytes, d.UsedBytes)
		}
	}
}

// snapshot24_16_8 returns a snapshot with three GPUs of asymmetric VRAM
// (24 GB, 16 GB and 8 GB) and 64 GB of system RAM. Used to exercise
// free-choice placement when more than two cards have unequal headroom.
func snapshot24_16_8() resman.Snapshot {
	return resman.Snapshot{
		Devices: []resman.Device{
			{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: 24 * GiB},
			{Name: "CUDA1", Type: "gpu_cuda", TotalBytes: 16 * GiB},
			{Name: "CUDA2", Type: "gpu_cuda", TotalBytes: 8 * GiB},
		},
		RAMBytes: 64 * GiB,
	}
}

// Test_Reserve_FreeChoice_ThreeWayAsymmetric verifies that free-choice
// placement walks down a 3-card asymmetric topology in the correct order:
// each unpinned reservation must land on whichever card currently has the
// most remaining headroom.
func Test_Reserve_FreeChoice_ThreeWayAsymmetric(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshot24_16_8(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Each step reserves a 6 GiB model and asserts the chosen card.
	//
	// Free state after each step (in GiB):
	//
	//   start                CUDA0=24 CUDA1=16 CUDA2= 8
	//   step1 → CUDA0 (24)   CUDA0=18 CUDA1=16 CUDA2= 8
	//   step2 → CUDA0 (18)   CUDA0=12 CUDA1=16 CUDA2= 8
	//   step3 → CUDA1 (16)   CUDA0=12 CUDA1=10 CUDA2= 8
	//   step4 → CUDA0 (12)   CUDA0= 6 CUDA1=10 CUDA2= 8
	//   step5 → CUDA1 (10)   CUDA0= 6 CUDA1= 4 CUDA2= 8
	//   step6 → CUDA2  (8)   CUDA0= 6 CUDA1= 4 CUDA2= 2
	steps := []struct {
		key  string
		want string
	}{
		{"s1", "CUDA0"},
		{"s2", "CUDA0"},
		{"s3", "CUDA1"},
		{"s4", "CUDA0"},
		{"s5", "CUDA1"},
		{"s6", "CUDA2"},
	}

	for _, s := range steps {
		_, plan, err := m.Reserve(resman.PlanRequest{Key: s.key, VRAMBytes: 6 * GiB})
		if err != nil {
			t.Fatalf("%s reserve: %v", s.key, err)
		}
		if len(plan.Per) != 1 || plan.Per[0].Name != s.want {
			t.Fatalf("%s placement: want %s, got %+v", s.key, s.want, plan.Per)
		}
	}

	// State is now CUDA0=6, CUDA1=4, CUDA2=2 free. A 7 GiB request must
	// fail because no single card can hold it (placement never sums across
	// GPUs).
	_, _, err = m.Reserve(resman.PlanRequest{Key: "overflow", VRAMBytes: 7 * GiB})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity when no single card has 7 GiB free, got: %v", err)
	}

	// Sanity: no card may ever exceed its budget.
	for _, d := range m.Usage().Devices {
		if d.UsedBytes > d.BudgetBytes {
			t.Errorf("device[%s] used=%d > budget=%d", d.Name, d.UsedBytes, d.BudgetBytes)
		}
	}
}

// Test_Reserve_TensorSplit_RatioOverflowsSmallCard verifies that a tensor
// split whose proportional share exceeds the smaller card's budget is
// rejected with ErrNoCapacity, even though the request would fit if summed
// across both cards.
func Test_Reserve_TensorSplit_RatioOverflowsSmallCard(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// 30 GiB total VRAM exists across the cards (24+12), but a 50/50 split
	// puts 15 GiB on CUDA1 — overflowing its 12 GiB budget.
	_, _, err = m.Reserve(resman.PlanRequest{
		Key:         "ratio-bad",
		VRAMBytes:   30 * GiB,
		Devices:     []string{"CUDA0", "CUDA1"},
		TensorSplit: []float32{0.5, 0.5},
	})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity when split share overflows smaller card, got: %v", err)
	}

	// Failed split must not have committed any bytes on either card.
	for _, d := range m.Usage().Devices {
		if d.UsedBytes != 0 {
			t.Errorf("device[%s] mutated by failed split: used=%d", d.Name, d.UsedBytes)
		}
	}

	// A skewed split that places the larger share on the larger card fits.
	// 30 GiB at 0.7/0.3 → 21 GiB on CUDA0, 9 GiB on CUDA1.
	_, plan, err := m.Reserve(resman.PlanRequest{
		Key:         "ratio-ok",
		VRAMBytes:   30 * GiB,
		Devices:     []string{"CUDA0", "CUDA1"},
		TensorSplit: []float32{0.7, 0.3},
	})
	if err != nil {
		t.Fatalf("skewed split must fit: %v", err)
	}

	// Allocation sum must equal VRAMBytes (rounding remainder lands on last).
	var sum int64
	for _, a := range plan.Per {
		sum += a.Bytes
	}
	if sum != 30*GiB {
		t.Errorf("split sum: want %d, got %d", 30*GiB, sum)
	}
}

// Test_Reserve_TensorSplit_HeadroomShrinksShare verifies the interaction
// between HeadroomBytes and asymmetric tensor split. With a 2 GiB headroom
// applied to each card, CUDA1's effective budget drops from 12 GiB to
// 10 GiB. A 50/50 split that would barely fit without headroom must now be
// rejected once headroom is accounted for.
func Test_Reserve_TensorSplit_HeadroomShrinksShare(t *testing.T) {
	cfg := resman.Config{
		Snapshot:      snapshot24_12(),
		BudgetPercent: 100,
		HeadroomBytes: 2 * GiB,
	}

	m, err := resman.New(cfg)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Effective budgets: CUDA0=22 GiB, CUDA1=10 GiB.
	u := m.Usage()
	if u.Devices[0].BudgetBytes != 22*GiB {
		t.Fatalf("CUDA0 budget: want %d, got %d", 22*GiB, u.Devices[0].BudgetBytes)
	}
	if u.Devices[1].BudgetBytes != 10*GiB {
		t.Fatalf("CUDA1 budget: want %d, got %d", 10*GiB, u.Devices[1].BudgetBytes)
	}

	// 20 GiB at 50/50 → 10 GiB each. CUDA1 is exactly at its post-headroom
	// budget, so this fits.
	if _, _, err := m.Reserve(resman.PlanRequest{
		Key:         "split-at-budget",
		VRAMBytes:   20 * GiB,
		Devices:     []string{"CUDA0", "CUDA1"},
		TensorSplit: []float32{0.5, 0.5},
	}); err != nil {
		t.Fatalf("at-budget split must fit: %v", err)
	}

	// 22 GiB at 50/50 → 11 GiB each. Without headroom CUDA1 (12 GiB) would
	// fit; with the 2 GiB headroom (10 GiB effective) the share now
	// overflows and the split must be rejected.
	_, _, err = m.Reserve(resman.PlanRequest{
		Key:         "split-over-headroom",
		VRAMBytes:   22 * GiB,
		Devices:     []string{"CUDA0", "CUDA1"},
		TensorSplit: []float32{0.5, 0.5},
	})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity once headroom shrinks CUDA1 below split share, got: %v", err)
	}
}

// Test_Reserve_FreeChoice_FollowsHeadroomAfterRelease verifies that
// free-choice placement re-evaluates remaining headroom after each release:
// the next unpinned reservation must move to whichever card currently has
// the most room, even if a previous round placed work elsewhere.
func Test_Reserve_FreeChoice_FollowsHeadroomAfterRelease(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Step 1: 18 GiB lands on CUDA0 (free 24 > 12). After: CUDA0=6 free,
	// CUDA1=12 free → CUDA1 now has more headroom.
	t1, plan, err := m.Reserve(resman.PlanRequest{Key: "big", VRAMBytes: 18 * GiB})
	if err != nil {
		t.Fatalf("big reserve: %v", err)
	}
	if plan.Per[0].Name != "CUDA0" {
		t.Fatalf("step1: want CUDA0, got %s", plan.Per[0].Name)
	}

	// Step 2: a 5 GiB unpinned reservation must follow the new headroom and
	// land on CUDA1 (12 > 6).
	t2, plan, err := m.Reserve(resman.PlanRequest{Key: "small1", VRAMBytes: 5 * GiB})
	if err != nil {
		t.Fatalf("small1 reserve: %v", err)
	}
	if plan.Per[0].Name != "CUDA1" {
		t.Fatalf("step2: want CUDA1, got %s", plan.Per[0].Name)
	}

	// Step 3: release the big CUDA0 ticket. State: CUDA0=24 free, CUDA1=7
	// free → headroom flips back to CUDA0.
	m.Release(t1)

	// Step 4: a fresh 5 GiB unpinned reservation must now return to CUDA0.
	_, plan, err = m.Reserve(resman.PlanRequest{Key: "small2", VRAMBytes: 5 * GiB})
	if err != nil {
		t.Fatalf("small2 reserve: %v", err)
	}
	if plan.Per[0].Name != "CUDA0" {
		t.Fatalf("step4: want CUDA0 after release, got %s", plan.Per[0].Name)
	}

	// Step 5: release the CUDA1 ticket. CUDA0=19 free, CUDA1=12 free →
	// CUDA0 still has more headroom; the next unpinned 5 GiB stays on CUDA0.
	m.Release(t2)
	_, plan, err = m.Reserve(resman.PlanRequest{Key: "small3", VRAMBytes: 5 * GiB})
	if err != nil {
		t.Fatalf("small3 reserve: %v", err)
	}
	if plan.Per[0].Name != "CUDA0" {
		t.Fatalf("step5: want CUDA0, got %s", plan.Per[0].Name)
	}
}

// Test_Reserve_PinBeatsFreeChoice verifies that pinning to a smaller card
// is honored even when the larger card is empty, and that an over-budget
// pin to the smaller card is rejected with ErrNoCapacity rather than
// silently overflowing onto the bigger card.
func Test_Reserve_PinBeatsFreeChoice(t *testing.T) {
	m, err := resman.New(noHeadroom(snapshot24_12(), 100))
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Both cards are empty; free-choice would pick CUDA0 (24 > 12). A pin
	// to CUDA1 must override that and place on the smaller card.
	_, plan, err := m.Reserve(resman.PlanRequest{
		Key: "pin-small", VRAMBytes: 5 * GiB, Devices: []string{"CUDA1"},
	})
	if err != nil {
		t.Fatalf("pinned reserve: %v", err)
	}
	if len(plan.Per) != 1 || plan.Per[0].Name != "CUDA1" {
		t.Fatalf("pin must override free-choice; got %+v", plan.Per)
	}

	// CUDA0 must still be empty — pin must not "spill" onto a bigger card.
	u := m.Usage()
	if u.Devices[0].UsedBytes != 0 {
		t.Errorf("CUDA0 must be untouched by pin to CUDA1, got %d", u.Devices[0].UsedBytes)
	}
	if u.Devices[1].UsedBytes != 5*GiB {
		t.Errorf("CUDA1 used: want %d, got %d", 5*GiB, u.Devices[1].UsedBytes)
	}

	// A pin that exceeds the smaller card's budget must fail with
	// ErrNoCapacity even though CUDA0 has more than enough headroom — pins
	// must never silently relocate.
	before := m.Usage()
	_, _, err = m.Reserve(resman.PlanRequest{
		Key: "pin-overflow", VRAMBytes: 16 * GiB, Devices: []string{"CUDA1"},
	})
	if !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity on over-budget pin, got: %v", err)
	}

	// Failed pin must not mutate either card.
	after := m.Usage()
	for i, d := range after.Devices {
		if d.UsedBytes != before.Devices[i].UsedBytes {
			t.Errorf("device[%s] mutated by failed pin: before=%d after=%d",
				d.Name, before.Devices[i].UsedBytes, d.UsedBytes)
		}
	}
}

// Test_Headroom verifies the headroom is subtracted from the budget. A model
// that would fit at BudgetPercent without headroom must be rejected once
// headroom is accounted for.
func Test_Headroom(t *testing.T) {
	cfg := resman.Config{
		Snapshot:      snapshotSingle(),
		BudgetPercent: 100,
		HeadroomBytes: 2 * GiB,
	}

	m, err := resman.New(cfg)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	// Budget is 16 - 2 = 14 GiB.
	if got := m.Usage().Devices[0].BudgetBytes; got != 14*GiB {
		t.Fatalf("BudgetBytes: want %d, got %d", 14*GiB, got)
	}

	if _, _, err := m.Reserve(resman.PlanRequest{Key: "fit", VRAMBytes: 14 * GiB}); err != nil {
		t.Fatalf("at-budget reserve: %v", err)
	}
	if _, _, err := m.Reserve(resman.PlanRequest{Key: "over", VRAMBytes: 1}); !errors.Is(err, resman.ErrNoCapacity) {
		t.Fatalf("expected ErrNoCapacity past headroom, got: %v", err)
	}
}

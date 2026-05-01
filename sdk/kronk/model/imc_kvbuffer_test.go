package model

import (
	"testing"
	"unsafe"
)

// TestKVBuffer_ZeroValue verifies that a zero-value kvBuffer is usable
// without initialization.
func TestKVBuffer_ZeroValue(t *testing.T) {
	var k kvBuffer

	if got := k.Len(); got != 0 {
		t.Errorf("zero value Len() = %d, want 0", got)
	}
	if got := k.Cap(); got != 0 {
		t.Errorf("zero value Cap() = %d, want 0", got)
	}
	if got := k.Bytes(); got != nil {
		t.Errorf("zero value Bytes() = %v, want nil", got)
	}
}

// TestKVBuffer_PrepareGrows verifies that Prepare allocates when the
// requested size exceeds the current capacity.
func TestKVBuffer_PrepareGrows(t *testing.T) {
	var k kvBuffer

	buf := k.Prepare(1024)
	if len(buf) != 1024 {
		t.Errorf("Prepare(1024) returned slice of len %d, want 1024", len(buf))
	}
	if k.Len() != 1024 {
		t.Errorf("after Prepare(1024) Len() = %d, want 1024", k.Len())
	}
	if k.Cap() < 1024 {
		t.Errorf("after Prepare(1024) Cap() = %d, want ≥ 1024", k.Cap())
	}
}

// TestKVBuffer_PrepareReusesBackingArray is the central invariant: when
// the requested size fits in the existing capacity, no new allocation
// happens and the same backing array is reused.
func TestKVBuffer_PrepareReusesBackingArray(t *testing.T) {
	var k kvBuffer

	first := k.Prepare(2048)
	firstAddr := unsafe.SliceData(first)

	// A smaller subsequent Prepare should reuse the same backing array.
	second := k.Prepare(1024)
	secondAddr := unsafe.SliceData(second)

	if firstAddr != secondAddr {
		t.Errorf("Prepare(1024) after Prepare(2048) reallocated; "+
			"backing array address changed (%p → %p)", firstAddr, secondAddr)
	}
	if k.Cap() != 2048 {
		t.Errorf("Cap() shrank to %d after Prepare(1024); want 2048 retained", k.Cap())
	}
	if k.Len() != 1024 {
		t.Errorf("Len() = %d after Prepare(1024), want 1024", k.Len())
	}
}

// TestKVBuffer_PrepareGrowsWhenExceeding verifies that Prepare allocates
// a fresh backing array when the requested size exceeds current capacity,
// and the new capacity is at least the requested size.
func TestKVBuffer_PrepareGrowsWhenExceeding(t *testing.T) {
	var k kvBuffer

	first := k.Prepare(1024)
	firstAddr := unsafe.SliceData(first)

	second := k.Prepare(4096)
	secondAddr := unsafe.SliceData(second)

	if firstAddr == secondAddr {
		t.Errorf("Prepare(4096) after Prepare(1024) did not reallocate; " +
			"expected new backing array")
	}
	if k.Cap() < 4096 {
		t.Errorf("after Prepare(4096) Cap() = %d, want ≥ 4096", k.Cap())
	}
	if k.Len() != 4096 {
		t.Errorf("after Prepare(4096) Len() = %d, want 4096", k.Len())
	}
}

// TestKVBuffer_PrepareNegativeSize verifies that a negative size is
// clamped to zero rather than panicking.
func TestKVBuffer_PrepareNegativeSize(t *testing.T) {
	var k kvBuffer
	_ = k.Prepare(1024)

	buf := k.Prepare(-1)
	if len(buf) != 0 {
		t.Errorf("Prepare(-1) returned slice of len %d, want 0", len(buf))
	}
	if k.Cap() != 1024 {
		t.Errorf("Cap() = %d after Prepare(-1); want 1024 retained", k.Cap())
	}
}

// TestKVBuffer_Commit verifies that Commit truncates Len without
// affecting Cap.
func TestKVBuffer_Commit(t *testing.T) {
	var k kvBuffer
	_ = k.Prepare(1024)

	k.Commit(512)

	if k.Len() != 512 {
		t.Errorf("after Commit(512) Len() = %d, want 512", k.Len())
	}
	if k.Cap() != 1024 {
		t.Errorf("after Commit(512) Cap() = %d, want 1024 retained", k.Cap())
	}
}

// TestKVBuffer_CommitClamps verifies that Commit clamps to [0, cap].
func TestKVBuffer_CommitClamps(t *testing.T) {
	var k kvBuffer
	_ = k.Prepare(1024)

	k.Commit(-50)
	if k.Len() != 0 {
		t.Errorf("Commit(-50) → Len() = %d, want 0", k.Len())
	}

	k.Commit(99999)
	if k.Len() != 1024 {
		t.Errorf("Commit(99999) → Len() = %d, want 1024 (clamped to cap)", k.Len())
	}
}

// TestKVBuffer_Reset verifies that Reset clears Len but retains Cap and
// the same backing array — that's the whole point of the never-shrink
// design.
func TestKVBuffer_Reset(t *testing.T) {
	var k kvBuffer
	first := k.Prepare(2048)
	firstAddr := unsafe.SliceData(first)

	k.Reset()

	if k.Len() != 0 {
		t.Errorf("after Reset() Len() = %d, want 0", k.Len())
	}
	if k.Cap() != 2048 {
		t.Errorf("after Reset() Cap() = %d, want 2048 retained", k.Cap())
	}

	// Next Prepare should reuse the same backing array.
	second := k.Prepare(1024)
	secondAddr := unsafe.SliceData(second)
	if firstAddr != secondAddr {
		t.Errorf("Prepare after Reset reallocated; backing array changed")
	}
}

// TestKVBuffer_BytesAliasesBuffer verifies that Bytes returns a slice
// that aliases the internal buffer (mutating the returned slice is
// observable via Bytes again, and via Len which reflects the slice
// length).
func TestKVBuffer_BytesAliasesBuffer(t *testing.T) {
	var k kvBuffer
	buf := k.Prepare(4)
	buf[0] = 0xAA
	buf[1] = 0xBB
	buf[2] = 0xCC
	buf[3] = 0xDD

	got := k.Bytes()
	want := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	if len(got) != 4 {
		t.Fatalf("Bytes() len = %d, want 4", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Bytes()[%d] = 0x%02X, want 0x%02X", i, got[i], want[i])
		}
	}
}

// TestKVBuffer_PrepareAddsHeadroomOnGrow verifies that a small grow
// over the previous capacity provisions at least 25% headroom. This
// is the central optimization that eliminates per-turn reallocations
// when conversations grow monotonically by small deltas.
func TestKVBuffer_PrepareAddsHeadroomOnGrow(t *testing.T) {
	var k kvBuffer

	// Establish an initial capacity.
	_ = k.Prepare(1000)
	if k.Cap() != 1000 {
		t.Fatalf("after Prepare(1000) Cap() = %d, want 1000", k.Cap())
	}

	// A small grow (within +25%) should provision the 25% headroom,
	// not just the requested size.
	_ = k.Prepare(1100)

	wantMin := 1000 + 1000/4 // 1250
	if k.Cap() < wantMin {
		t.Errorf("after small grow Cap() = %d, want ≥ %d (25%% headroom)", k.Cap(), wantMin)
	}
	if k.Len() != 1100 {
		t.Errorf("after Prepare(1100) Len() = %d, want 1100", k.Len())
	}
}

// TestKVBuffer_PrepareReusesAfterHeadroomGrow verifies that subsequent
// Prepares within the headroom slack reuse the backing array. This is
// the practical payoff: after one headroom grow, many turns of small
// deltas hit the reuse path with zero allocation.
func TestKVBuffer_PrepareReusesAfterHeadroomGrow(t *testing.T) {
	var k kvBuffer
	_ = k.Prepare(1000)

	// Small grow allocates with 25% headroom (cap becomes ≥ 1250).
	first := k.Prepare(1100)
	firstAddr := unsafe.SliceData(first)
	growCap := k.Cap()

	if growCap < 1250 {
		t.Fatalf("after small grow Cap() = %d, want ≥ 1250", growCap)
	}

	// Successive small grows within the headroom must reuse the
	// backing array.
	for _, size := range []int{1150, 1200, 1240, growCap} {
		buf := k.Prepare(size)
		if got := unsafe.SliceData(buf); got != firstAddr {
			t.Errorf("Prepare(%d) within headroom reallocated; backing array changed", size)
		}
		if k.Cap() != growCap {
			t.Errorf("Prepare(%d) within headroom changed Cap() to %d, want %d retained",
				size, k.Cap(), growCap)
		}
	}
}

// TestKVBuffer_PrepareLargeJumpHonorsExactSize verifies that when the
// requested size exceeds oldCap + 25%, the new capacity is exactly the
// requested size (no extra headroom). Mirrors Go's "newLen > 2*oldCap"
// shortcut and prevents accidental over-allocation when matching into
// a session whose previous conversation was much smaller.
func TestKVBuffer_PrepareLargeJumpHonorsExactSize(t *testing.T) {
	var k kvBuffer
	_ = k.Prepare(1000)

	// 5000 is well over 1000 + 250 = 1250, so the size wins and
	// Cap() must be exactly 5000 (no extra headroom).
	_ = k.Prepare(5000)

	if k.Cap() != 5000 {
		t.Errorf("after large-jump Prepare(5000) over Cap=1000, Cap() = %d, want 5000",
			k.Cap())
	}
}

// TestKVBuffer_PrepareFirstAllocNoHeadroom verifies that the very
// first allocation (oldCap == 0) is sized exactly to the request.
// 0 + 0/4 = 0 < size, so size wins.
func TestKVBuffer_PrepareFirstAllocNoHeadroom(t *testing.T) {
	var k kvBuffer

	_ = k.Prepare(1024)

	if k.Cap() != 1024 {
		t.Errorf("first Prepare(1024) Cap() = %d, want 1024 (no headroom on cold start)",
			k.Cap())
	}
}

// TestKVBuffer_PrepareLogarithmicGrowCount verifies that under a
// monotonically growing workload (each turn larger than the last by
// a small delta), the number of actual reallocations is O(log_1.25(N))
// rather than O(N). This is the practical performance guarantee of the
// 25% headroom strategy.
func TestKVBuffer_PrepareLogarithmicGrowCount(t *testing.T) {
	var k kvBuffer

	// Simulate 200 turns where each turn is 1% larger than the last,
	// starting at 1 MiB and growing to ~7.3 MiB. Without headroom this
	// would be 200 reallocations; with 25% headroom it should be far
	// fewer (a 1.25× factor over a 7.3× total range is log_1.25(7.3)
	// ≈ 9 grows).
	size := 1 << 20
	growCount := 0
	prevAddr := unsafe.SliceData(k.Prepare(size))
	growCount++
	k.Commit(size)

	for range 200 {
		size = size + size/100 // +1% per turn
		buf := k.Prepare(size)
		if addr := unsafe.SliceData(buf); addr != prevAddr {
			growCount++
			prevAddr = addr
		}
		k.Commit(size)
	}

	if growCount > 15 {
		t.Errorf("grow count = %d over 201 turns, want ≤ 15 (logarithmic with 25%% headroom)", growCount)
	}
}

// TestKVBuffer_NoChurnUnderRepeatedSnapshots simulates the IMC
// per-turn snapshot pattern: many Prepare/Commit cycles whose sizes
// stay below an established peak. After warm-up, no further allocation
// should happen — assert by comparing the backing array address across
// many iterations.
func TestKVBuffer_NoChurnUnderRepeatedSnapshots(t *testing.T) {
	var k kvBuffer

	// Establish peak capacity.
	first := k.Prepare(1 << 20) // 1 MiB
	firstAddr := unsafe.SliceData(first)
	k.Commit(1 << 20)

	// Simulate 100 turns; each turn snapshots a size between 50% and 100%
	// of the established peak. Backing array must never change.
	for i := range 100 {
		size := max(
			// shrinks slowly, stays under peak
			(1<<20)-(i*1024), 0)
		buf := k.Prepare(size)
		if got := unsafe.SliceData(buf); got != firstAddr {
			t.Fatalf("iteration %d: backing array reallocated (peak=%d, size=%d)",
				i, 1<<20, size)
		}
		k.Commit(size)
	}
}

package model

// kvBuffer encapsulates a session's externalized KV cache state with
// lazy-grow / never-shrink semantics.
//
// The backing byte array is allocated on first Prepare, grown only when a
// snapshot exceeds the current capacity, and retained across snapshots
// (and across session rebinds) so per-turn allocation churn is eliminated.
// Conversations grow monotonically and are bounded by the model context
// window, so each session's buffer reaches steady-state after a small
// number of Prepare calls and never reallocates again.
//
// kvBuffer is NOT safe for concurrent use. Callers must serialize access
// via the existing per-session invariants — at most one in-flight request
// touches a given session's kvBuffer at a time.
//
// The zero value is a usable, empty buffer.
type kvBuffer struct {
	buf []byte
}

// Len returns the number of valid bytes currently held in the buffer.
// This is the size of the most recently Commit'ed snapshot.
func (k *kvBuffer) Len() int {
	return len(k.buf)
}

// Cap returns the current backing-array capacity. Useful for diagnostics
// and tests that want to verify the never-shrink invariant.
func (k *kvBuffer) Cap() int {
	return cap(k.buf)
}

// Bytes returns the valid byte slice for read access (e.g., to pass to
// llama.StateSeqSetData when restoring KV state). The returned slice
// aliases the internal buffer; callers must not retain it past the next
// Prepare/Commit/Reset call.
func (k *kvBuffer) Bytes() []byte {
	return k.buf
}

// Prepare returns a slice of length size, ready to be filled (e.g., by
// llama.StateSeqGetData). The backing array is reused if its capacity
// is already sufficient, otherwise a new array is allocated and the old
// one is released. The previous contents are not preserved across a
// resize.
//
// On grow, the new capacity is max(size, oldCap + oldCap/4) — i.e. at
// least 25% headroom over the previous capacity. This mirrors Go's
// runtime policy for large slices (see nextslicecap in
// runtime/slice.go) and amortizes the cost of the small per-turn
// monotonic growth pattern produced by IMC: each turn typically adds
// only a few MB of KV state, so allocating exactly the requested size
// would force a reallocation every turn. With 25% headroom, a snapshot
// of N bytes provisions capacity for the next ~25% of conversation
// growth, after which the buffer grows again by 25%. Total grows over
// a session lifetime are O(log_1.25(peak)).
//
// When size > oldCap + oldCap/4 (large jump, e.g. matched into a
// session whose previous conversation was much smaller, or a media
// build), the requested size is honored directly with no extra
// headroom. This mirrors Go's "newLen > 2*oldCap" shortcut.
//
// A negative size is treated as 0.
func (k *kvBuffer) Prepare(size int) []byte {
	if size < 0 {
		size = 0
	}

	oldCap := cap(k.buf)
	if oldCap < size {
		// Add 25% headroom over the previous capacity, but never less
		// than the requested size. Integer math: oldCap/4 is +25%.
		newCap := max(oldCap+oldCap/4, size)
		k.buf = make([]byte, size, newCap)
	} else {
		k.buf = k.buf[:size]
	}

	return k.buf
}

// Commit truncates the buffer to the actual length n after a fill
// operation (e.g., when llama.StateSeqGetData returns fewer bytes than
// the prepared size, or zero on failure). The backing array is retained.
//
// n is clamped to [0, cap(buf)].
func (k *kvBuffer) Commit(n int) {
	switch {
	case n < 0:
		n = 0
	case n > cap(k.buf):
		n = cap(k.buf)
	}
	k.buf = k.buf[:n]
}

// Reset clears the valid contents (Len becomes 0) but retains the
// backing array for reuse on the next Prepare. Called when a session
// is rebound to a different conversation: the old conversation's bytes
// become irrelevant but the buffer itself stays attached to the session
// to avoid a fresh allocation on the next snapshot.
func (k *kvBuffer) Reset() {
	k.buf = k.buf[:0]
}

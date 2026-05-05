// Package ram provides the in-process RAM implementation of the
// model.SessionStore contract used by IMC (Incremental Message Cache)
// to externalize per-session KV cache bytes between requests.
//
// This is the default session store. It keeps each session's bytes in a
// single Go-allocated []byte with lazy-grow / never-shrink semantics —
// the backing array is allocated on first Prepare, grown only when a
// snapshot exceeds the current capacity, and retained across snapshots
// (and across session rebinds) so per-turn allocation churn is
// eliminated. Conversations grow monotonically and are bounded by the
// model context window, so each session's buffer reaches steady-state
// after a small number of Prepare calls and never reallocates again.
//
// Future kvstorage subpackages (e.g. kvstorage/disk) provide alternative
// backends behind the same model.SessionStore contract.
package ram

// Store is the in-process RAM session store. It is NOT safe for
// concurrent use; the IMC scheduler serializes access via the
// per-session pending invariant — at most one in-flight request
// touches a given session's Store at a time.
//
// The zero value is a usable, empty Store.
type Store struct {
	buf []byte
}

// New returns a new, empty Store ready for use.
func New() *Store {
	return &Store{}
}

// Len returns the number of valid bytes currently held in the store.
// This is the size of the most recently Commit'ed snapshot.
func (s *Store) Len() int {
	return len(s.buf)
}

// Cap returns the current backing-array capacity. Useful for
// diagnostics and tests that want to verify the never-shrink invariant.
func (s *Store) Cap() int {
	return cap(s.buf)
}

// Bytes returns the valid byte slice for read access (e.g., to pass to
// llama.StateSeqSetData when restoring KV state). The returned slice
// aliases the internal buffer; callers must not retain it past the next
// Prepare/Commit/Reset call.
func (s *Store) Bytes() []byte {
	return s.buf
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
func (s *Store) Prepare(size int) []byte {
	if size < 0 {
		size = 0
	}

	oldCap := cap(s.buf)
	if oldCap < size {
		// Add 25% headroom over the previous capacity, but never less
		// than the requested size. Integer math: oldCap/4 is +25%.
		newCap := max(oldCap+oldCap/4, size)
		s.buf = make([]byte, size, newCap)
	} else {
		s.buf = s.buf[:size]
	}

	return s.buf
}

// Commit truncates the store to the actual length n after a fill
// operation (e.g., when llama.StateSeqGetData returns fewer bytes than
// the prepared size, or zero on failure). The backing array is retained.
//
// n is clamped to [0, cap(buf)].
func (s *Store) Commit(n int) {
	switch {
	case n < 0:
		n = 0
	case n > cap(s.buf):
		n = cap(s.buf)
	}
	s.buf = s.buf[:n]
}

// Reset clears the valid contents (Len becomes 0) but retains the
// backing array for reuse on the next Prepare. Called when a session
// is rebound to a different conversation: the old conversation's bytes
// become irrelevant but the buffer itself stays attached to the session
// to avoid a fresh allocation on the next snapshot.
func (s *Store) Reset() {
	s.buf = s.buf[:0]
}

// Close releases the backing array. Always returns nil; satisfies the
// SessionStore.Close contract. The RAM impl has no file descriptors
// or external resources to release — Go's garbage collector reclaims
// the backing array once no caller retains a reference.
func (s *Store) Close() error {
	s.buf = nil
	return nil
}

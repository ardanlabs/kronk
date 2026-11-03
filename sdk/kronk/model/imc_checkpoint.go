package model

import (
	"context"
	"errors"
	"fmt"
)

// imcCheckpoint holds native sequence bytes in RAM. Its backing array grows
// lazily and normally retains capacity across snapshots and session rebinds.
// It is not safe for concurrent use; the IMC scheduler serializes access
// through session reservations and System-cache build/restore ownership.
//
// The zero value is a usable, empty checkpoint.
type imcCheckpoint struct {
	buf      []byte
	length   int
	prepared int

	// Allocation-owned evidence survives session metadata resets, but never
	// moves to a replacement array. Only published working checkpoints count.
	baselineBytes    int
	baselineMessages int
	lastMessages     int
	smallerSnapshots int
	replacement      bool
}

// Len returns the number of valid bytes currently held in the store.
// This is the size of the most recently Commit'ed snapshot.
func (s *imcCheckpoint) Len() int {
	return s.length
}

// Cap returns the current backing-array capacity. Useful for
// diagnostics and allocation-retention decisions.
func (s *imcCheckpoint) Cap() int {
	return cap(s.buf)
}

// Bytes returns the valid byte slice for read access (e.g., to pass to
// llama.StateSeqSetData when restoring KV state). The returned read-only slice
// aliases the internal buffer; callers must not modify it or retain it past the
// next Prepare, Commit, Reset, or Close call.
func (s *imcCheckpoint) Bytes(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return s.buf[:s.length], nil
}

// Prepare returns a borrowed slice of length size, ready to be filled (e.g.,
// by llama.StateSeqGetData). The caller may write it only until the next
// Prepare, Commit, Reset, or Close call. The backing array is reused if its
// capacity is already sufficient, otherwise a new array is allocated and the
// old one is released. The previous contents are not preserved across a
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
// A negative size returns an error.
func (s *imcCheckpoint) Prepare(ctx context.Context, size int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if size < 0 {
		return nil, errors.New("imc-checkpoint: prepare size must not be negative")
	}

	s.length = 0
	s.prepared = size
	oldCap := cap(s.buf)
	if oldCap < size {
		// Add 25% headroom over the previous capacity, but never less
		// than the requested size. Integer math: oldCap/4 is +25%.
		newCap := max(oldCap+oldCap/4, size)
		*s = imcCheckpoint{buf: make([]byte, size, newCap), prepared: size}
	} else {
		s.buf = s.buf[:size]
	}

	return s.buf, nil
}

// Commit truncates the store to the actual length n after a fill
// operation (e.g., when llama.StateSeqGetData returns fewer bytes than
// the prepared size, or zero on failure). The backing array is retained.
//
// n must be within the slice returned by Prepare.
func (s *imcCheckpoint) Commit(ctx context.Context, n int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.length = 0
	if n < 0 || n > s.prepared {
		return fmt.Errorf("imc-checkpoint: commit length %d outside prepared length %d", n, s.prepared)
	}

	s.length = n
	s.prepared = 0
	return nil
}

// Reset zeroes the entire backing array and clears the valid contents
// (Len becomes 0), while retaining capacity for the next Prepare. Called
// when a session is rebound to a different conversation so no bytes from
// the prior conversation survive reuse.
func (s *imcCheckpoint) Reset(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	clear(s.buf[:cap(s.buf)])
	s.buf = s.buf[:0]
	s.length = 0
	s.prepared = 0
	return nil
}

// observe records successful working-prefix publication. A replacement history
// must reach the allocation's original message count and demonstrate 25% lower
// requirements at three increasing message counts. Repeats cannot qualify it.
func (s *imcCheckpoint) observe(messages int) {
	if s == nil || s.length == 0 {
		return
	}
	if s.baselineBytes == 0 {
		s.baselineBytes = s.length
		s.baselineMessages = messages
		return
	}
	// Round 75% down without overflowing. The original requirement is no
	// larger than capacity, so this also guarantees at least 25% spare capacity.
	limit := s.baselineBytes/4*3 + s.baselineBytes%4*3/4
	if s.length > limit {
		s.smallerSnapshots = 0
		s.lastMessages = 0
		return
	}
	if s.replacement && messages >= s.baselineMessages && messages > s.lastMessages {
		s.smallerSnapshots = min(s.smallerSnapshots+1, 3)
		s.lastMessages = messages
	}
}

// reuse runs only after a successful planner reset, while the session is
// reserved. Discard qualified spare capacity only for a shorter incoming
// history. Otherwise keep the array, but do not combine unfinished evidence
// from different histories.
func (s *imcCheckpoint) reuse(incomingMessages, outgoingMessages int) {
	if s == nil {
		return
	}
	if s.smallerSnapshots == 3 && incomingMessages < s.baselineMessages && incomingMessages < outgoingMessages {
		*s = imcCheckpoint{}
		return
	}
	if s.smallerSnapshots < 3 {
		s.smallerSnapshots = 0
	}
	s.lastMessages = 0
	s.replacement = true
}

// Close releases the backing array. Always returns nil. There are no file
// descriptors or external resources to release — Go's garbage collector reclaims
// the backing array once no caller retains a reference.
func (s *imcCheckpoint) Close() error {
	*s = imcCheckpoint{}
	return nil
}

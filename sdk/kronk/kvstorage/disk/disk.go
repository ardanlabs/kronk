// Package disk provides a disk-backed implementation of the
// model.SessionStore contract used by IMC (Incremental Message Cache)
// to externalize per-session KV cache bytes between requests.
//
// Each Store owns one regular file under the directory passed to New.
// The file is created on construction (via os.CreateTemp, so the name
// is unique within the directory) and removed on Close. The Store
// keeps a small RAM scratch buffer used as the cgo target for
// snapshot writes (Prepare/Commit) and a separate read buffer that is
// populated lazily on Bytes — both buffers are released on the next
// Prepare/Commit/Reset/Close so the steady-state RAM footprint is
// O(peak snapshot size) per active session, vs O(N × peak) for the
// RAM backend across N sessions.
//
// The disk backend trades RAM for I/O. Snapshot writes and restore
// reads each touch the file once per request; on NVMe class storage
// this is on the order of milliseconds per GB. Use this backend when
// the host cannot spare (NSeqMax × peak-conversation-KV) bytes of
// RAM for IMC.
//
// Crash safety: per-session files leak on process crash because the
// Store's Close cleanup never runs. They live under
// Config.SessionStoreDir and are named "kronk-sess-*.kv"; an external
// cleanup (cron, systemd-tmpfiles, or a future startup-time sweep)
// can reclaim them.
package disk

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// filePattern is the os.CreateTemp pattern used for per-session
// files. The "*" is replaced by a random suffix that makes the name
// unique within the configured directory.
const filePattern = "kronk-sess-*.kv"

// Store is the disk-backed session store. One instance owns one file.
// It is NOT safe for concurrent use; the IMC scheduler serializes
// access via the per-session pending invariant — at most one in-flight
// request touches a given session's Store at a time.
type Store struct {
	file    *os.File // open handle to the per-session file
	length  int      // bytes committed to the file (= file size)
	scratch []byte   // RAM buffer used by Prepare; written to file in Commit
	read    []byte   // RAM buffer used by Bytes; lazily filled from the file
}

// New creates a new disk-backed session store. A fresh per-session
// file is created under dir via os.CreateTemp, which gives each
// session a name unique within dir. The directory must already exist
// and be writable; New does not create it.
//
// The returned *Store owns the file; the caller must invoke Close to
// remove it. On Close failure the file is leaked under dir and must be
// reclaimed out-of-band.
func New(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("disk: SessionStoreDir is required for the disk backend")
	}

	f, err := os.CreateTemp(dir, filePattern)
	if err != nil {
		return nil, fmt.Errorf("disk: create session file in %q: %w", dir, err)
	}

	return &Store{file: f}, nil
}

// Len returns the number of valid bytes currently held by the store
// (i.e., the size of the most recently committed snapshot on disk).
func (s *Store) Len() int {
	return s.length
}

// Cap returns the current scratch-buffer capacity. The disk store has
// no persistent backing-array notion; this is a diagnostic indicator
// of the largest in-flight snapshot the scratch has ever held.
func (s *Store) Cap() int {
	return cap(s.scratch)
}

// Bytes returns a slice containing the most recently committed
// snapshot bytes, lazily reading them from disk into a RAM buffer the
// first time it is called after a Commit. The returned slice is valid
// until the next Prepare/Commit/Reset/Close call on this store; the
// store must not be used concurrently while a caller holds the slice.
//
// Returns nil when the store is empty.
func (s *Store) Bytes() []byte {
	if s.length == 0 {
		return nil
	}

	// Reuse the read buffer when it already holds the latest snapshot.
	if len(s.read) == s.length {
		return s.read
	}

	// Grow the read buffer if needed; otherwise reuse the backing
	// array. ReadAt is used over Read so we don't have to track the
	// file offset.
	if cap(s.read) < s.length {
		s.read = make([]byte, s.length)
	} else {
		s.read = s.read[:s.length]
	}

	if _, err := s.file.ReadAt(s.read, 0); err != nil && !errors.Is(err, io.EOF) {
		// Surface the failure as an empty result; callers treat zero
		// bytes as "nothing to restore" and rebuild from scratch.
		s.read = s.read[:0]
		return nil
	}

	return s.read
}

// Prepare returns a writable scratch buffer of length size, ready to
// be filled (typically by cgo via llama.StateSeqGetData). The scratch
// is reused when its capacity is sufficient; otherwise a new array is
// allocated. Calling Prepare invalidates any slice previously returned
// by Bytes.
//
// On grow, the new capacity is max(size, oldCap + oldCap/4) — the same
// 25% headroom policy as the RAM backend, amortizing per-turn churn
// when conversations grow monotonically by small deltas.
//
// A negative size is treated as 0.
func (s *Store) Prepare(size int) []byte {
	if size < 0 {
		size = 0
	}

	// Drop the read cache; the next Bytes call will re-read from disk.
	s.read = nil

	oldCap := cap(s.scratch)
	if oldCap < size {
		newCap := max(oldCap+oldCap/4, size)
		s.scratch = make([]byte, size, newCap)
	} else {
		s.scratch = s.scratch[:size]
	}

	return s.scratch
}

// Commit writes the first n bytes of the scratch buffer to the
// per-session file, truncating the file to exactly n bytes. n is
// clamped to [0, cap(scratch)]. After Commit the on-disk file
// contains the new snapshot and Len returns n.
//
// On write failure the file's contents are undefined; the next Bytes
// call will return whatever the partial write left behind. Callers
// detect the inconsistency via Len() == 0 and rebuild from scratch.
func (s *Store) Commit(n int) {
	switch {
	case n < 0:
		n = 0
	case n > cap(s.scratch):
		n = cap(s.scratch)
	}

	// Drop the read cache; it's about to be stale.
	s.read = nil

	if n == 0 {
		// Truncate to zero so Bytes returns nil and Len reports 0.
		_ = s.file.Truncate(0)
		s.length = 0
		return
	}

	if _, err := s.file.WriteAt(s.scratch[:n], 0); err != nil {
		_ = s.file.Truncate(0)
		s.length = 0
		return
	}

	if err := s.file.Truncate(int64(n)); err != nil {
		s.length = 0
		return
	}

	s.length = n
}

// Reset truncates the on-disk file to zero bytes and clears the read
// cache. The scratch buffer is retained for reuse on the next
// Prepare. Called when a session is rebound to a different
// conversation.
func (s *Store) Reset() {
	s.read = nil
	if err := s.file.Truncate(0); err == nil {
		s.length = 0
	}
}

// Close releases the file descriptor and removes the per-session
// file. After Close the store must not be used again.
//
// Returns the first non-nil error from close-or-remove; both
// operations are attempted regardless of intermediate failure so a
// failed Close still removes the file when possible.
func (s *Store) Close() error {
	if s.file == nil {
		return nil
	}

	name := s.file.Name()
	closeErr := s.file.Close()
	removeErr := os.Remove(name)

	s.file = nil
	s.scratch = nil
	s.read = nil
	s.length = 0

	switch {
	case closeErr != nil:
		return fmt.Errorf("disk: close session file %q: %w", name, closeErr)
	case removeErr != nil:
		return fmt.Errorf("disk: remove session file %q: %w", name, removeErr)
	}

	return nil
}

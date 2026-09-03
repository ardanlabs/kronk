// This file provides a temporary disk-backed implementation of the
// kvstorage.Store contract as an SDK extension example. It is not durable
// storage: files have no stable identity and Close removes them.
//
// Each Store owns one regular file under the directory passed to New.
// The file is created on construction (via os.CreateTemp, so the name
// is unique within the directory) and removed on Close. The Store
// keeps a RAM scratch buffer used as the cgo target for snapshot writes
// (Prepare/Commit) and a separate read buffer populated lazily on Bytes.
// The scratch allocation is retained for reuse, so this example does not
// eliminate snapshot-sized RAM allocations.
//
// Crash safety: per-session files leak on process crash because the
// Store's Close cleanup never runs. They live under the directory passed to
// NewFactory and are named "kronk-sess-*.kv"; an external
// cleanup can reclaim them. The implementation also cannot report I/O errors
// caused by process termination. It is an API example only, not a production
// storage recommendation.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/kronk/kvstorage"
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
	file     *os.File // open handle to the per-session file
	length   int      // bytes committed to the file (= file size)
	prepared int      // writable bytes handed out by the latest Prepare
	scratch  []byte   // RAM buffer used by Prepare; written to file in Commit
	read     []byte   // RAM buffer used by Bytes; lazily filled from the file
}

var _ kvstorage.Store = (*Store)(nil)

// NewFactory constructs a factory for independent disk stores rooted in dir.
// The directory must already exist and be a directory. Each factory call
// creates a new per-session file under dir.
func NewFactory(ctx context.Context, dir string) (kvstorage.Factory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, errors.New("disk: directory is required")
	}

	fi, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("disk: stat directory %q: %w", dir, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("disk: path %q is not a directory", dir)
	}

	return func(ctx context.Context) (kvstorage.Store, error) {
		return New(ctx, dir)
	}, nil
}

// New creates a new disk-backed session store. A fresh per-session
// file is created under dir via os.CreateTemp, which gives each
// session a name unique within dir. The directory must already exist
// and be writable; New does not create it.
//
// The returned *Store owns the file; the caller must invoke Close to
// remove it. On Close failure the file is leaked under dir and must be
// reclaimed out-of-band.
func New(ctx context.Context, dir string) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, errors.New("disk: directory is required")
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
func (s *Store) Bytes(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.length == 0 {
		return nil, nil
	}

	// Reuse the read buffer when it already holds the latest snapshot.
	if len(s.read) == s.length {
		return s.read, nil
	}

	// Grow the read buffer if needed; otherwise reuse the backing
	// array. ReadAt is used over Read so we don't have to track the
	// file offset.
	if cap(s.read) < s.length {
		s.read = make([]byte, s.length)
	} else {
		s.read = s.read[:s.length]
	}

	n, err := s.file.ReadAt(s.read, 0)
	if err != nil {
		s.read = s.read[:0]
		return nil, fmt.Errorf("disk: read session snapshot: %w", err)
	}
	if n != s.length {
		s.read = s.read[:0]
		return nil, fmt.Errorf("disk: read session snapshot: got %d bytes, want %d", n, s.length)
	}

	return s.read, nil
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
// A negative size returns an error.
func (s *Store) Prepare(ctx context.Context, size int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if size < 0 {
		return nil, errors.New("disk: prepare size must not be negative")
	}

	// Drop the read cache; the next Bytes call will re-read from disk.
	s.read = nil
	s.length = 0
	s.prepared = size

	oldCap := cap(s.scratch)
	if oldCap < size {
		newCap := max(oldCap+oldCap/4, size)
		s.scratch = make([]byte, size, newCap)
	} else {
		s.scratch = s.scratch[:size]
	}

	return s.scratch, nil
}

// Commit writes the first n bytes of the scratch buffer to the
// per-session file, truncating the file to exactly n bytes. n must be
// within the slice returned by Prepare. After Commit the on-disk file
// contains the new snapshot and Len returns n.
//
// After Prepare, a failure leaves Len at zero so callers cannot restore
// partial contents.
func (s *Store) Commit(ctx context.Context, n int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.length = 0

	if n < 0 || n > s.prepared {
		return fmt.Errorf("disk: commit length %d outside prepared length %d", n, s.prepared)
	}

	// Drop the read cache; it's about to be stale.
	s.read = nil
	s.prepared = 0

	if n == 0 {
		// Truncate to zero so Bytes returns nil and Len reports 0.
		if err := s.file.Truncate(0); err != nil {
			return fmt.Errorf("disk: truncate session snapshot: %w", err)
		}
		return nil
	}

	written, err := s.file.WriteAt(s.scratch[:n], 0)
	if err != nil {
		writeErr := fmt.Errorf("disk: write session snapshot: %w", err)
		if truncateErr := s.file.Truncate(0); truncateErr != nil {
			return errors.Join(writeErr, fmt.Errorf("disk: truncate failed snapshot: %w", truncateErr))
		}
		return writeErr
	}
	if written != n {
		writeErr := fmt.Errorf("disk: write session snapshot: wrote %d bytes, want %d", written, n)
		if truncateErr := s.file.Truncate(0); truncateErr != nil {
			return errors.Join(writeErr, fmt.Errorf("disk: truncate failed snapshot: %w", truncateErr))
		}
		return writeErr
	}

	if err := s.file.Truncate(int64(n)); err != nil {
		return fmt.Errorf("disk: truncate session snapshot to %d bytes: %w", n, err)
	}

	s.length = n
	return nil
}

// Reset truncates the on-disk file to zero bytes and zeroes all retained
// scratch and read-buffer capacity. The scratch allocation is retained for
// reuse on the next Prepare, but no bytes from the prior conversation survive.
func (s *Store) Reset(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	clear(s.scratch[:cap(s.scratch)])
	clear(s.read[:cap(s.read)])
	s.read = nil
	s.length = 0
	s.prepared = 0
	if err := s.file.Truncate(0); err != nil {
		return fmt.Errorf("disk: reset session snapshot: %w", err)
	}

	return nil
}

// Close releases the file descriptor and removes the per-session
// file. After Close the store must not be used again.
//
// Both operations are attempted regardless of intermediate failure so a
// failed Close still removes the file when possible. Close joins both errors
// when both operations fail.
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
	s.prepared = 0

	if closeErr != nil {
		closeErr = fmt.Errorf("disk: close session file %q: %w", name, closeErr)
	}
	if removeErr != nil {
		removeErr = fmt.Errorf("disk: remove session file %q: %w", name, removeErr)
	}

	return errors.Join(closeErr, removeErr)
}

package disk

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// newTestStore creates a Store rooted under t.TempDir(); the dir is
// auto-cleaned by the test framework so leftover files don't matter.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New(tempdir) err = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

// TestNewRequiresDir verifies that an empty dir returns an error
// rather than silently writing to the process CWD.
func TestNewRequiresDir(t *testing.T) {
	store, err := New("")
	if err == nil {
		t.Fatalf("New(\"\") err = nil, want non-nil")
	}
	if store != nil {
		t.Errorf("New(\"\") store = %v, want nil", store)
	}
}

// TestNewMissingDirErrors verifies that pointing at a nonexistent
// directory returns an error rather than creating it.
func TestNewMissingDirErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	store, err := New(missing)
	if err == nil {
		t.Fatalf("New(missing) err = nil, want non-nil")
	}
	if store != nil {
		t.Errorf("New(missing) store = %v, want nil", store)
	}
}

// TestZeroState verifies the empty store reports zero length and
// returns a nil Bytes slice — Len() and Bytes() must agree on
// "nothing to restore" so the IMC restore path skips the no-op call
// to llama.StateSeqSetData.
func TestZeroState(t *testing.T) {
	s := newTestStore(t)

	if got := s.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0", got)
	}
	if got := s.Bytes(); got != nil {
		t.Errorf("Bytes() = %v, want nil", got)
	}
}

// TestPrepareCommitBytes verifies the snapshot/restore round trip:
// Prepare hands out a writable scratch slice, Commit persists it to
// disk, and Bytes reads it back exactly.
func TestPrepareCommitBytes(t *testing.T) {
	s := newTestStore(t)

	want := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x11, 0x22, 0x33}
	buf := s.Prepare(len(want))
	copy(buf, want)
	s.Commit(len(want))

	if got := s.Len(); got != len(want) {
		t.Errorf("Len() = %d, want %d", got, len(want))
	}

	got := s.Bytes()
	if !bytes.Equal(got, want) {
		t.Errorf("Bytes() = %x, want %x", got, want)
	}
}

// TestBytesIsCachedAfterFirstRead verifies that successive Bytes
// calls reuse the same backing array — repeated restores in the IMC
// hot path must not pay the I/O cost twice.
func TestBytesIsCachedAfterFirstRead(t *testing.T) {
	s := newTestStore(t)

	buf := s.Prepare(16)
	for i := range buf {
		buf[i] = byte(i)
	}
	s.Commit(16)

	first := s.Bytes()
	second := s.Bytes()

	if &first[0] != &second[0] {
		t.Errorf("Bytes() returned different backing arrays on successive calls; expected reuse")
	}
}

// TestPrepareInvalidatesReadCache verifies that the contract on
// Bytes — slice valid until next Prepare/Commit/Reset — is honored.
// After Prepare the read buffer is cleared so the next Bytes re-reads
// from disk.
func TestPrepareInvalidatesReadCache(t *testing.T) {
	s := newTestStore(t)

	buf := s.Prepare(4)
	copy(buf, []byte{1, 2, 3, 4})
	s.Commit(4)

	first := s.Bytes()
	if !bytes.Equal(first, []byte{1, 2, 3, 4}) {
		t.Fatalf("first Bytes() = %v, want [1 2 3 4]", first)
	}

	// Calling Prepare invalidates the previously returned slice.
	_ = s.Prepare(4)

	// The new Bytes call should re-read from disk.
	got := s.Bytes()
	if !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Errorf("Bytes() after Prepare = %v, want [1 2 3 4] (re-read from disk)", got)
	}
}

// TestCommitTruncatesShorter verifies that committing fewer bytes
// shrinks the on-disk snapshot — the file must reflect exactly the
// committed length so a partial-write recovery doesn't leave stale
// trailing bytes that StateSeqSetData would interpret as KV data.
func TestCommitTruncatesShorter(t *testing.T) {
	s := newTestStore(t)

	buf := s.Prepare(16)
	for i := range buf {
		buf[i] = byte(i + 1)
	}
	s.Commit(16)

	// Commit a shorter snapshot.
	buf = s.Prepare(4)
	copy(buf, []byte{0xAA, 0xBB, 0xCC, 0xDD})
	s.Commit(4)

	if got := s.Len(); got != 4 {
		t.Errorf("Len() = %d, want 4", got)
	}

	got := s.Bytes()
	want := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	if !bytes.Equal(got, want) {
		t.Errorf("Bytes() = %x, want %x (file should be truncated)", got, want)
	}
}

// TestCommitClampsTooLarge verifies that an oversize Commit is
// clamped to the scratch capacity rather than panicking on the out-of-
// bounds slice.
func TestCommitClampsTooLarge(t *testing.T) {
	s := newTestStore(t)

	_ = s.Prepare(8)
	s.Commit(99999)

	if got := s.Len(); got != 8 {
		t.Errorf("Len() after oversize Commit = %d, want 8 (clamped to cap)", got)
	}
}

// TestCommitClampsNegative verifies that a negative Commit is
// treated as zero rather than panicking.
func TestCommitClampsNegative(t *testing.T) {
	s := newTestStore(t)

	buf := s.Prepare(8)
	copy(buf, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	s.Commit(8)

	s.Commit(-1)

	if got := s.Len(); got != 0 {
		t.Errorf("Len() after negative Commit = %d, want 0", got)
	}
	if got := s.Bytes(); got != nil {
		t.Errorf("Bytes() after negative Commit = %v, want nil", got)
	}
}

// TestPrepareNegativeSize verifies the negative-size path matches the
// RAM impl — a negative size becomes zero, no panic.
func TestPrepareNegativeSize(t *testing.T) {
	s := newTestStore(t)

	_ = s.Prepare(1024)
	buf := s.Prepare(-1)

	if len(buf) != 0 {
		t.Errorf("Prepare(-1) returned slice of len %d, want 0", len(buf))
	}
	if s.Cap() != 1024 {
		t.Errorf("Cap() = %d after Prepare(-1); want 1024 retained", s.Cap())
	}
}

// TestReset verifies that Reset clears the on-disk snapshot, the
// reported length, and the read cache while keeping the file usable.
func TestReset(t *testing.T) {
	s := newTestStore(t)

	buf := s.Prepare(8)
	copy(buf, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	s.Commit(8)
	_ = s.Bytes() // populate the read cache

	s.Reset()

	if got := s.Len(); got != 0 {
		t.Errorf("Len() after Reset = %d, want 0", got)
	}
	if got := s.Bytes(); got != nil {
		t.Errorf("Bytes() after Reset = %v, want nil", got)
	}

	// Store remains usable after Reset.
	buf = s.Prepare(2)
	copy(buf, []byte{0xAB, 0xCD})
	s.Commit(2)
	if got := s.Bytes(); !bytes.Equal(got, []byte{0xAB, 0xCD}) {
		t.Errorf("Bytes() after Reset+Prepare+Commit = %x, want [AB CD]", got)
	}
}

// TestCloseRemovesFile verifies that Close removes the per-session
// file from disk — production calls Close in Model.Unload to keep
// SessionStoreDir from accumulating leaked files.
func TestCloseRemovesFile(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatalf("New err = %v, want nil", err)
	}

	name := store.file.Name()
	if _, err := os.Stat(name); err != nil {
		t.Fatalf("stat before Close: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Errorf("Close err = %v, want nil", err)
	}

	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Errorf("stat after Close: err = %v, want IsNotExist", err)
	}
}

// TestCloseIdempotent verifies that calling Close twice does not
// return an error — the second call is a no-op.
func TestCloseIdempotent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New err = %v, want nil", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first Close err = %v, want nil", err)
	}
	if err := store.Close(); err != nil {
		t.Errorf("second Close err = %v, want nil (idempotent)", err)
	}
}

// TestRoundTripGrowAndShrink simulates the IMC per-turn pattern:
// snapshots that grow over time, then a smaller snapshot. Verifies
// the on-disk file size always matches the committed length, not the
// scratch capacity.
func TestRoundTripGrowAndShrink(t *testing.T) {
	s := newTestStore(t)

	for _, size := range []int{1024, 2048, 4096, 1024} {
		buf := s.Prepare(size)
		for i := range buf {
			buf[i] = byte(i % 251)
		}
		s.Commit(size)

		got := s.Bytes()
		if len(got) != size {
			t.Errorf("size=%d: Bytes len = %d, want %d", size, len(got), size)
		}
		for i := range got {
			if got[i] != byte(i%251) {
				t.Errorf("size=%d: byte %d = %x, want %x", size, i, got[i], byte(i%251))
				break
			}
		}
	}
}

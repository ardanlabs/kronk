package model

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFakeModel creates a fake model file plus its companion sha file
// (in <dir>/sha/<base>) and returns (modelPath, computedSHA).
func writeFakeModel(t *testing.T, dir string, name string, payload []byte) (string, string) {
	t.Helper()

	modelPath := filepath.Join(dir, name)
	if err := os.WriteFile(modelPath, payload, 0o644); err != nil {
		t.Fatalf("write model: %v", err)
	}

	sum := sha256.Sum256(payload)
	sha := fmt.Sprintf("%x", sum)

	shaDir := filepath.Join(dir, "sha")
	if err := os.MkdirAll(shaDir, 0o755); err != nil {
		t.Fatalf("mkdir sha: %v", err)
	}
	shaContents := fmt.Sprintf("oid sha256:%s\nsize %d\n", sha, len(payload))
	if err := os.WriteFile(filepath.Join(shaDir, name), []byte(shaContents), 0o644); err != nil {
		t.Fatalf("write sha file: %v", err)
	}

	return modelPath, sha
}

// TestCheckModel_FirstCallWritesSentinel verifies that a successful full
// sha-check leaves a sentinel behind so subsequent calls can short-circuit.
func TestCheckModel_FirstCallWritesSentinel(t *testing.T) {
	dir := t.TempDir()
	modelPath, _ := writeFakeModel(t, dir, "fake.gguf", []byte("hello kronk"))

	sentinel := verifiedFilePath(modelPath)
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("sentinel should not exist before first check; got err=%v", err)
	}

	if err := CheckModel(modelPath, true); err != nil {
		t.Fatalf("first CheckModel: unexpected error: %v", err)
	}

	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel should exist after first successful check: %v", err)
	}
}

// TestCheckModel_SecondCallSkipsRehash verifies the fast-path: if the
// sentinel matches the file's expected sha + size + mtime, CheckModel
// must NOT re-open the file body. We prove this by removing the model
// file body after the first verify and confirming the second call still
// succeeds (it would fail with ENOENT if it tried to re-hash).
func TestCheckModel_SecondCallSkipsRehash(t *testing.T) {
	dir := t.TempDir()
	modelPath, _ := writeFakeModel(t, dir, "fake.gguf", []byte("hello kronk"))

	if err := CheckModel(modelPath, true); err != nil {
		t.Fatalf("first CheckModel: %v", err)
	}

	// Capture the stat info we'd need to "lie" to CheckModel about the
	// file body. We can't actually delete it (CheckModel calls os.Stat
	// up front) but we can truncate-then-restore-mtime/size to confirm
	// the sentinel is what's deciding the outcome.
	info, err := os.Stat(modelPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Replace the body with garbage of the same size and restore mtime.
	garbage := make([]byte, info.Size())
	for i := range garbage {
		garbage[i] = 0xff
	}
	if err := os.WriteFile(modelPath, garbage, 0o644); err != nil {
		t.Fatalf("rewrite body: %v", err)
	}
	if err := os.Chtimes(modelPath, time.Now(), info.ModTime()); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// Second call must succeed because the sentinel says we already
	// verified — even though the body now hashes to something else.
	if err := CheckModel(modelPath, true); err != nil {
		t.Fatalf("second CheckModel: expected fast-path success, got: %v", err)
	}
}

// TestCheckModel_MTimeChangeInvalidatesSentinel verifies that a changed
// mtime forces a re-hash (and therefore detects the changed body).
func TestCheckModel_MTimeChangeInvalidatesSentinel(t *testing.T) {
	dir := t.TempDir()
	modelPath, _ := writeFakeModel(t, dir, "fake.gguf", []byte("hello kronk"))

	if err := CheckModel(modelPath, true); err != nil {
		t.Fatalf("first CheckModel: %v", err)
	}

	// Replace body with same-size garbage and bump mtime to "now".
	info, err := os.Stat(modelPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	garbage := make([]byte, info.Size())
	for i := range garbage {
		garbage[i] = 0xff
	}
	if err := os.WriteFile(modelPath, garbage, 0o644); err != nil {
		t.Fatalf("rewrite body: %v", err)
	}
	// Default mtime from WriteFile is "now" which differs from sentinel's mtime.
	if err := CheckModel(modelPath, true); err == nil {
		t.Fatalf("expected sha mismatch after mtime change + body change, got nil")
	}
}

// TestCheckModel_SizeChangeFailsBeforeFastPath verifies that a size
// change is caught by the existing size-mismatch guard well before the
// sentinel has a chance to short-circuit.
func TestCheckModel_SizeChangeFailsBeforeFastPath(t *testing.T) {
	dir := t.TempDir()
	modelPath, _ := writeFakeModel(t, dir, "fake.gguf", []byte("hello kronk"))

	if err := CheckModel(modelPath, true); err != nil {
		t.Fatalf("first CheckModel: %v", err)
	}

	if err := os.WriteFile(modelPath, []byte("different size"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if err := CheckModel(modelPath, true); err == nil {
		t.Fatalf("expected size-mismatch error, got nil")
	}
}

// TestCheckModel_SHAFileChangeInvalidatesSentinel verifies that if the
// expected sha in the sha file changes (e.g. the model was repackaged
// with the same byte length but different contents and the sha file got
// updated to match), we re-hash rather than trusting the stale sentinel.
func TestCheckModel_SHAFileChangeInvalidatesSentinel(t *testing.T) {
	dir := t.TempDir()
	modelPath, _ := writeFakeModel(t, dir, "fake.gguf", []byte("hello kronk"))

	if err := CheckModel(modelPath, true); err != nil {
		t.Fatalf("first CheckModel: %v", err)
	}

	// Overwrite the sha file with a different expected sha but the same
	// size, simulating a "repacked-to-same-size" scenario where the
	// sentinel must NOT short-circuit.
	shaPath := filepath.Join(dir, "sha", "fake.gguf")
	bogus := fmt.Sprintf("oid sha256:%s\nsize %d\n",
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		len("hello kronk"))
	if err := os.WriteFile(shaPath, []byte(bogus), 0o644); err != nil {
		t.Fatalf("rewrite sha file: %v", err)
	}

	if err := CheckModel(modelPath, true); err == nil {
		t.Fatalf("expected sha mismatch from re-hash, got nil")
	}
}

// TestRemoveVerifiedSentinel verifies the cleanup helper removes the
// sentinel and treats a missing sentinel as a no-op.
func TestRemoveVerifiedSentinel(t *testing.T) {
	dir := t.TempDir()
	modelPath, _ := writeFakeModel(t, dir, "fake.gguf", []byte("hello kronk"))

	if err := CheckModel(modelPath, true); err != nil {
		t.Fatalf("CheckModel: %v", err)
	}

	sentinel := verifiedFilePath(modelPath)
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("sentinel should exist: %v", err)
	}

	if err := RemoveVerifiedSentinel(modelPath); err != nil {
		t.Fatalf("RemoveVerifiedSentinel: %v", err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("sentinel should have been removed; got err=%v", err)
	}

	// Second call must be a no-op (missing file is not an error).
	if err := RemoveVerifiedSentinel(modelPath); err != nil {
		t.Fatalf("RemoveVerifiedSentinel on missing sentinel should not error: %v", err)
	}
}

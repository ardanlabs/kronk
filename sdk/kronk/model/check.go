package model

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// verifiedSuffix is appended to the basename of the sha file to form the
// verified sentinel path. The sentinel records that a full sha256
// verification of the model file has already succeeded so subsequent loads
// can skip the (very expensive) re-hash when the file is unchanged.
const verifiedSuffix = ".verified"

// verifiedSentinel is the JSON payload written to <dir>/sha/<base>.verified
// after a successful full-file sha256 verification. The combination of
// SHA256 + Size + MTimeNS lets a later call detect any change to the
// expected hash, the file size, or the file's modification time and
// re-verify when needed.
type verifiedSentinel struct {
	SHA256       string `json:"sha256"`
	Size         int64  `json:"size"`
	MTimeNS      int64  `json:"mtime_ns"`
	VerifiedAt   int64  `json:"verified_at"`
	KronkVersion string `json:"kronk_version,omitempty"`
}

// CheckModel is check if the downloaded model is valid based on it's sha
// file. If no sha file exists, this check will return with no error.
func CheckModel(modelFile string, checkSHA bool) error {
	dir := filepath.Dir(modelFile)
	base := filepath.Base(modelFile)
	shaFile := filepath.Join(dir, "sha", base)

	data, err := os.Open(shaFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("check-model: opening sha file: %w", err)
	}
	defer data.Close()

	var expectedSHA string
	var expectedSize int64

	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "oid sha256:"):
			expectedSHA = strings.TrimPrefix(line, "oid sha256:")

		case strings.HasPrefix(line, "size "):
			sizeStr := strings.TrimPrefix(line, "size ")
			expectedSize, err = strconv.ParseInt(sizeStr, 10, 64)
			if err != nil {
				return fmt.Errorf("check-model: parsing size from sha file: %w", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("check-model: reading sha file: %w", err)
	}

	info, err := os.Stat(modelFile)
	if err != nil {
		return fmt.Errorf("check-model: stat model file: %w", err)
	}

	if info.Size() != expectedSize {
		return fmt.Errorf("check-model: size mismatch: expected %d, got %d", expectedSize, info.Size())
	}

	if checkSHA {

		// Fast path: if a verified sentinel says we've already hashed this
		// exact file (same expected sha + same size + same mtime), skip the
		// multi-second re-hash.
		if isAlreadyVerified(modelFile, expectedSHA, info) {
			return nil
		}

		f, err := os.Open(modelFile)
		if err != nil {
			return fmt.Errorf("check-model: opening model file for sha check: %w", err)
		}
		defer f.Close()

		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return fmt.Errorf("check-model: computing sha256: %w", err)
		}

		actualSHA := fmt.Sprintf("%x", h.Sum(nil))
		if actualSHA != expectedSHA {
			return fmt.Errorf("check-model: sha256 mismatch: expected %s, got %s", expectedSHA, actualSHA)
		}

		// Best-effort: record success so future loads can skip the re-hash.
		// Failure to write the sentinel must not block the model load —
		// the worst case is we re-verify next time.
		_ = markVerified(modelFile, expectedSHA, info)
	}

	return nil
}

// verifiedFilePath returns the path of the sentinel file that records a
// successful full sha256 verification for modelFile. It lives next to the
// existing sha file so the two are managed together.
func verifiedFilePath(modelFile string) string {
	dir := filepath.Dir(modelFile)
	base := filepath.Base(modelFile)
	return filepath.Join(dir, "sha", base+verifiedSuffix)
}

// isAlreadyVerified returns true when a sentinel file exists for modelFile
// AND its recorded expected sha, file size, and file mtime all match the
// values for the current file on disk. Any mismatch (or any error reading
// the sentinel) returns false so the caller falls back to a full re-hash.
func isAlreadyVerified(modelFile string, expectedSHA string, info os.FileInfo) bool {
	path := verifiedFilePath(modelFile)

	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	var s verifiedSentinel
	if err := json.Unmarshal(raw, &s); err != nil {
		return false
	}

	if !strings.EqualFold(s.SHA256, expectedSHA) {
		return false
	}
	if s.Size != info.Size() {
		return false
	}
	if s.MTimeNS != info.ModTime().UnixNano() {
		return false
	}

	return true
}

// markVerified writes a sentinel file recording a successful full sha256
// verification of modelFile. It uses an atomic temp-file + rename so a
// concurrent reader never observes a partially-written sentinel. Errors
// are returned but the caller may safely ignore them — a missing sentinel
// just means the next load re-verifies.
func markVerified(modelFile string, expectedSHA string, info os.FileInfo) error {
	path := verifiedFilePath(modelFile)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mark-verified: mkdir: %w", err)
	}

	s := verifiedSentinel{
		SHA256:     expectedSHA,
		Size:       info.Size(),
		MTimeNS:    info.ModTime().UnixNano(),
		VerifiedAt: time.Now().Unix(),
	}

	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("mark-verified: marshal: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("mark-verified: tempfile: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("mark-verified: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("mark-verified: close: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("mark-verified: rename: %w", err)
	}

	return nil
}

// RemoveVerifiedSentinel deletes the sentinel file for modelFile if it
// exists. Used by the model-removal paths so a deleted model doesn't
// leave behind a stale verified marker. A non-existent sentinel is not
// an error.
func RemoveVerifiedSentinel(modelFile string) error {
	if err := os.Remove(verifiedFilePath(modelFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

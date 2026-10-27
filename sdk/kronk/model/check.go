package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
)

// ErrArtifactSizeMismatch indicates an incomplete or invalid model artifact.
var ErrArtifactSizeMismatch = errors.New("model artifact size mismatch; wait for any active pull to finish or run `kronk model pull` again")

// ArtifactDigest provides the expected SHA256 and size for an artifact.
type ArtifactDigest struct {
	SHA256 string
	Size   int64
}

// ArtifactVerification records a successful full-file verification of an
// artifact.
type ArtifactVerification struct {
	SHA256       string
	Size         int64
	MTimeNS      int64
	VerifiedAt   int64
	KronkVersion string
}

// ArtifactIntegrity provides the parsed digest and optional prior verification
// for an artifact path.
type ArtifactIntegrity struct {
	Digest       ArtifactDigest
	Verification *ArtifactVerification
}

// ArtifactVerificationRecorder persists a newly produced verification record.
// The model package does not prescribe where or how the record is stored.
type ArtifactVerificationRecorder func(modelFile string, verification ArtifactVerification) error

// VerifyArtifact checks modelFile against parsed integrity values. It reads the
// artifact itself but performs no sidecar I/O. The returned bool reports
// whether a new verification was produced and should be persisted by the
// caller.
func VerifyArtifact(modelFile string, digest ArtifactDigest, previous *ArtifactVerification, checkSHA bool) (ArtifactVerification, bool, error) {
	return VerifyArtifactWithLogger(context.Background(), applog.DiscardLogger, modelFile, digest, previous, checkSHA)
}

// VerifyArtifactWithLogger performs VerifyArtifact and logs the start and
// successful completion of full-file hashing. Cached and size-only checks
// produce no verification logs.
func VerifyArtifactWithLogger(ctx context.Context, log applog.Logger, modelFile string, digest ArtifactDigest, previous *ArtifactVerification, checkSHA bool) (ArtifactVerification, bool, error) {
	expectedSHA := strings.ToLower(digest.SHA256)
	decoded, err := hex.DecodeString(expectedSHA)
	if err != nil || len(decoded) != sha256.Size {
		return ArtifactVerification{}, false, fmt.Errorf("verify-artifact: invalid sha256")
	}

	if digest.Size < 0 {
		return ArtifactVerification{}, false, fmt.Errorf("verify-artifact: invalid size")
	}

	info, err := os.Stat(modelFile)
	if err != nil {
		return ArtifactVerification{}, false, fmt.Errorf("verify-artifact: stat model file: %w", err)
	}

	if info.Size() != digest.Size {
		return ArtifactVerification{}, false, fmt.Errorf("verify-artifact: %w: expected %d, got %d", ErrArtifactSizeMismatch, digest.Size, info.Size())
	}

	if !checkSHA {
		return ArtifactVerification{}, false, nil
	}

	if verificationMatches(previous, expectedSHA, info) {
		return *previous, false, nil
	}

	f, err := os.Open(modelFile)
	if err != nil {
		return ArtifactVerification{}, false, fmt.Errorf("verify-artifact: open model file: %w", err)
	}
	defer f.Close()

	log(ctx, fmt.Sprintf("verify-artifact: verifying SHA-256 for %s (%d MB)...", filepath.Base(modelFile), info.Size()/(1000*1000)))
	start := time.Now()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ArtifactVerification{}, false, fmt.Errorf("verify-artifact: compute sha256: %w", err)
	}

	actualSHA := fmt.Sprintf("%x", h.Sum(nil))
	if actualSHA != expectedSHA {
		return ArtifactVerification{}, false, fmt.Errorf("verify-artifact: sha256 mismatch: expected %s, got %s", expectedSHA, actualSHA)
	}

	verification := ArtifactVerification{
		SHA256:     expectedSHA,
		Size:       info.Size(),
		MTimeNS:    info.ModTime().UnixNano(),
		VerifiedAt: time.Now().Unix(),
	}

	log(ctx, fmt.Sprintf("verify-artifact: SHA-256 verified for %s in %s", filepath.Base(modelFile), time.Since(start).Round(time.Millisecond)))

	return verification, true, nil
}

func verificationMatches(verification *ArtifactVerification, expectedSHA string, info os.FileInfo) bool {
	if verification == nil {
		return false
	}
	if !strings.EqualFold(verification.SHA256, expectedSHA) {
		return false
	}
	if verification.Size != info.Size() {
		return false
	}
	if verification.MTimeNS != info.ModTime().UnixNano() {
		return false
	}

	return true
}

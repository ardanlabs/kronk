package libs

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/download"
)

var (
	// ErrInvalidDigest means a pinned manifest digest is malformed.
	ErrInvalidDigest = download.ErrInvalidDigest

	// ErrInvalidVersion means a pinned digest does not name an exact release.
	ErrInvalidVersion = download.ErrInvalidVersion
)

// VerifyReport describes the files checked in an installed llama.cpp bundle.
type VerifyReport = download.VerifyReport

// Verify checks the selected library bundle against Yzma's published file
// digests. An empty version trusts the release recorded during installation. A
// version may include an externally trusted manifest digest in the form
// VERSION@sha256:DIGEST.
func (lib *Libs) Verify(ctx context.Context, version string) (*VerifyReport, error) {
	report, err := download.VerifyInstall(ctx, lib.path, version)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	return report, nil
}

func verifyRuntime(ctx context.Context, candidate runtimeCandidate, version string) error {
	report, err := download.VerifyInstall(ctx, candidate.path, version)
	if err != nil {
		return fmt.Errorf("verify runtime %q: %w", candidate.path, err)
	}
	if !report.OK() {
		return fmt.Errorf("verify runtime %q: %d changed and %d missing files", candidate.path, report.Changed, report.Missing)
	}

	return nil
}

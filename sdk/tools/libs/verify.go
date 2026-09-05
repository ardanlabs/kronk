package libs

import (
	"context"
	"errors"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/download"
)

var (
	// ErrInvalidDigest means a pinned manifest digest is malformed.
	ErrInvalidDigest = download.ErrInvalidDigest

	// ErrInvalidVersion means a pinned digest does not name an exact release.
	ErrInvalidVersion = download.ErrInvalidVersion

	// ErrNoFileDigests means the available release metadata provides archive
	// digests but not the per-file digests needed for post-install validation.
	ErrNoFileDigests = download.ErrNoFileDigests
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

func (lib *Libs) validateDownload(ctx context.Context, log Logger, tag VersionTag) error {
	version := lib.verificationVersion(tag.Version)

	report, err := lib.Verify(ctx, version)
	err = allowUnavailableFileValidation(ctx, log, version, err)
	if err != nil {
		return fmt.Errorf("download: validate libraries: %w", err)
	}
	if report == nil {
		return nil
	}
	if !report.OK() {
		return fmt.Errorf("download: validate libraries: %d changed, %d missing, and %d unexpected files", report.Changed, report.Missing, report.Unexpected)
	}

	return nil
}

func allowUnavailableFileValidation(ctx context.Context, log Logger, version string, err error) error {
	if !errors.Is(err, ErrNoFileDigests) {
		return err
	}

	log(ctx, "validate-libraries: post-install validation unavailable", "version", version, "WARNING", err)
	return nil
}

func (lib *Libs) verificationVersion(version string) string {
	if lib.version != "" {
		return lib.version
	}
	if version == versionTag(defaultVersion) {
		return defaultVersion
	}

	return version
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

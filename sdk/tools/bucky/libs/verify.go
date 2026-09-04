package libs

import (
	"context"
	"fmt"
	"slices"

	"github.com/ardanlabs/bucky/pkg/download"
)

var (
	// ErrInvalidDigest means a pinned manifest digest is malformed.
	ErrInvalidDigest = download.ErrInvalidDigest

	// ErrInvalidVersion means a pinned digest does not name an exact release.
	ErrInvalidVersion = download.ErrInvalidVersion
)

// VerifyReport describes the files checked in an installed whisper.cpp bundle.
type VerifyReport = download.VerifyReport

// Verify checks the selected library bundle against Bucky's published file
// digests. An empty version trusts the metadata recorded during installation.
// A version may include an externally trusted manifest digest in the form
// VERSION@sha256:DIGEST.
func (lib *Libs) Verify(ctx context.Context, version string) (*VerifyReport, error) {
	report, err := download.VerifyInstall(ctx, lib.path, version)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

	// Bucky verifies a dedicated library directory and therefore rejects every
	// unexpected path. Kronk owns version.json in that directory; remove only
	// that known metadata file while preserving all other unexpected paths.
	report.Files = slices.DeleteFunc(report.Files, func(file download.FileReport) bool {
		if file.Name != versionFile || file.State != download.FileUnexpected {
			return false
		}

		report.Unexpected--
		return true
	})

	return report, nil
}

func (lib *Libs) validateDownload(ctx context.Context, tag VersionTag) error {
	version := tag.Version
	if lib.version != "" {
		version = lib.version
	}

	report, err := lib.Verify(ctx, version)
	if err != nil {
		return fmt.Errorf("download: validate libraries: %w", err)
	}
	if !report.OK() {
		return fmt.Errorf("download: validate libraries: %d changed, %d missing, and %d unexpected files", report.Changed, report.Missing, report.Unexpected)
	}

	return nil
}

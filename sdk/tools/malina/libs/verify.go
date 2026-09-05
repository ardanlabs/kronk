package libs

import (
	"context"
	"fmt"

	"github.com/ardanlabs/malina/pkg/download"
)

// VerifyReport describes the files checked in an installed
// stable-diffusion.cpp bundle.
type VerifyReport = download.VerifyReport

// Verify checks the selected library bundle against Malina's embedded file
// digests. An empty version uses the release recorded during installation. A
// version may include the embedded manifest digest in the form
// VERSION@sha256:DIGEST.
func (lib *Libs) Verify(ctx context.Context, version string) (*VerifyReport, error) {
	report, err := download.VerifyInstall(ctx, lib.path, version)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}

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
		return fmt.Errorf("download: validate libraries: %d changed and %d missing files", report.Changed, report.Missing)
	}

	return nil
}

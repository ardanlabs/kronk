package toolapp

import (
	"context"
	"errors"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	malinalibs "github.com/ardanlabs/kronk/sdk/tools/malina/libs"
)

func (a *app) verifyMalinaLibs(ctx context.Context, r *http.Request) web.Encoder {
	version := r.URL.Query().Get("version")
	if version == "" {
		tag, err := a.malinaLibs.InstalledVersion()
		if err != nil {
			return errs.Errorf(errs.Internal, "unable to read stable-diffusion.cpp library version: %s", err)
		}
		version = tag.Version
	}

	report, err := a.malinaLibs.Verify(ctx, version)
	if err != nil {
		if errors.Is(err, malinalibs.ErrInvalidDigest) || errors.Is(err, malinalibs.ErrInvalidVersion) {
			return errs.Errorf(errs.InvalidArgument, "invalid library version: %s", err)
		}
		return errs.Errorf(errs.Internal, "unable to verify stable-diffusion.cpp libraries: %s", err)
	}

	var verifiedPaths []string
	for _, file := range report.Files {
		if file.State.String() == "verified" {
			verifiedPaths = append(verifiedPaths, file.Name)
		}
	}
	manifest, verifiedAt, err := buildRuntimeBundleManifest(ctx, a.malinaLibs.LibsPath(), verifiedPaths, report.OK())
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to identify stable-diffusion.cpp libraries: %s", err)
	}

	return toAppMalinaLibIntegrity(report, a.malinaLibs, manifest, verifiedAt)
}

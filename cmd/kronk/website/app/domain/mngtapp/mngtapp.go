// Package mngtapp provides endpoints to handle server managment.
package mngtapp

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/krn"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/web"
	"github.com/ardanlabs/kronk/install"
)

type app struct {
	build  string
	log    *logger.Logger
	krnMgr *krn.Manager
}

func newApp(log *logger.Logger, krnMgr *krn.Manager) *app {
	return &app{
		log:    log,
		krnMgr: krnMgr,
	}
}

func (a *app) libs(ctx context.Context, r *http.Request) web.Encoder {
	libPath := a.krnMgr.LibsPath()
	processor := a.krnMgr.Processor()

	orgVI, err := install.VersionInformation(libPath)
	if err != nil {
		return errs.Newf(errs.Internal, "error retrieving version info: %s", err)
	}

	a.log.Info(ctx, "mngtapp-libs", "status", "check llama.cpp installation", "libPath", libPath, "processor", processor, "latest", orgVI.Latest, "current", orgVI.Current)

	if orgVI.Current == orgVI.Latest {
		a.log.Info(ctx, "mngtapp-libs", "status", "current already installed", "latest", orgVI.Latest, "current", orgVI.Current)
		return toAppVersion("latest already installed", libPath, processor, orgVI)
	}

	a.log.Info(ctx, "mngtapp-libs", "status", "llama.cpp installation", "libPath", libPath, "processor", processor)

	vi, err := install.Libraries(libPath, processor, true)
	if err != nil {
		a.log.Info(ctx, "mngtapp-libs", "status", "llama.cpp installation", "ERROR", err)

		if _, err := install.InstalledVersion(libPath); err != nil {
			return errs.Newf(errs.Internal, "failed to install llama: %q: error: %s", libPath, err)
		}

		a.log.Info(ctx, "mngtapp-libs", "status", "failed to install new version, using current version")
	}

	a.log.Info(ctx, "mngtapp-libs", "status", "updated llama.cpp installation", "libPath", "old version", orgVI.Current, "current", vi.Current)

	return toAppVersion("new version installed", libPath, processor, vi)
}

func (a *app) list(ctx context.Context, r *http.Request) web.Encoder {
	return nil
}

func (a *app) pull(ctx context.Context, r *http.Request) web.Encoder {
	return nil
}

func (a *app) remove(ctx context.Context, r *http.Request) web.Encoder {
	return nil
}

func (a *app) show(ctx context.Context, r *http.Request) web.Encoder {
	return nil
}

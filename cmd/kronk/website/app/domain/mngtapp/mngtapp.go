// Package mngtapp provides endpoints to handle server managment.
package mngtapp

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/krn"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/web"
	"github.com/ardanlabs/kronk/tools"
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
	libPath := a.krnMgr.LibPath()
	processor := a.krnMgr.Processor()

	vi, err := tools.DownloadLibraries(ctx, tools.FmtLogger, libPath, processor, true)
	if err != nil {
		return errs.Newf(errs.Internal, "unable to install llama.cpp: %s", err)
	}

	return toAppVersion("installed", libPath, processor, vi)
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

// Package toolapp provides endpoints to handle tool management.
package toolapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
		return errs.Errorf(errs.Internal, "unable to install llama.cpp: %s", err)
	}

	return toAppVersion("installed", libPath, processor, vi)
}

func (a *app) list(ctx context.Context, r *http.Request) web.Encoder {
	modelPath := a.krnMgr.ModelPath()

	models, err := tools.ListModels(modelPath)
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to retrieve model list: %s", err)
	}

	return toListModelsInfo(models)
}

func (a *app) pull(ctx context.Context, r *http.Request) web.Encoder {
	var req PullRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if _, err := url.ParseRequestURI(req.ModelURL); err != nil {
		return errs.Errorf(errs.InvalidArgument, "invalid model URL: %s", req.ModelURL)
	}

	if req.ProjURL != "" {
		if _, err := url.ParseRequestURI(req.ProjURL); err != nil {
			return errs.Errorf(errs.InvalidArgument, "invalid project URL: %s", req.ProjURL)
		}
	}

	// -------------------------------------------------------------------------

	w := web.GetWriter(ctx)

	f, ok := w.(http.Flusher)
	if !ok {
		return errs.Errorf(errs.Internal, "streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	// -------------------------------------------------------------------------

	modelPath := a.krnMgr.ModelPath()

	logger := func(ctx context.Context, msg string, args ...any) {
		var sb strings.Builder
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				sb.WriteString(fmt.Sprintf(" %v[%v]", args[i], args[i+1]))
			}
		}

		fmt.Fprintf(w, "%s:%s\n", msg, sb.String())
		f.Flush()
	}

	_, err := tools.DownloadModel(ctx, logger, req.ModelURL, req.ProjURL, modelPath)
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to install model: %s", err)
	}

	return web.NewNoResponse()
}

func (a *app) remove(ctx context.Context, r *http.Request) web.Encoder {
	modelPath := a.krnMgr.ModelPath()
	modelName := web.Param(r, "model")

	a.log.Info(ctx, "tool-remove", "modelName", modelName)

	mp, err := tools.FindModel(modelPath, modelName)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if err := tools.RemoveModel(mp); err != nil {
		return errs.Errorf(errs.Internal, "failed to remove model: %s", err)
	}

	return nil
}

func (a *app) show(ctx context.Context, r *http.Request) web.Encoder {
	libPath := a.krnMgr.LibPath()
	modelPath := a.krnMgr.ModelPath()
	modelName := web.Param(r, "model")

	mi, err := tools.ShowModel(libPath, modelPath, modelName)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toModelInfo(mi)
}

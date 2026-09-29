// Package decisionapp provides the decision API endpoints.
package decisionapp

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/pool"
)

type app struct {
	log  *logger.Logger
	pool *pool.Pool
}

func newApp(cfg Config) *app {
	return &app{
		log:  cfg.Log,
		pool: cfg.Pool,
	}
}

func (a *app) decide(ctx context.Context, r *http.Request) web.Encoder {
	req, err := decodeDecisionRequest(r.Body)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	krn, err := a.pool.Kronk.AquireModel(ctx, req.Model)
	if err != nil {
		return errs.FromSDK(err)
	}

	if !krn.ModelInfo().IsDecisionModel {
		return errs.Errorf(errs.InvalidArgument, "model doesn't support decisions")
	}

	a.log.Info(ctx, "decision", "MODEL", req.Model, "QUESTIONS", len(req.Questions))

	if _, err := krn.DecisionHTTP(ctx, a.log.Info, web.GetWriter(ctx), req.toSDK()); err != nil {
		return errs.FromSDK(err)
	}

	return web.NewNoResponse()
}

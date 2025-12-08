// Package chatapp provides the chat api endpoints.
package chatapp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/krn"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/web"
	"github.com/ardanlabs/kronk/model"
)

type app struct {
	log    *logger.Logger
	krnMgr *krn.Manager
}

func newApp(log *logger.Logger, krnMgr *krn.Manager) *app {
	return &app{
		log:    log,
		krnMgr: krnMgr,
	}
}

func (a *app) chatCompletions(ctx context.Context, r *http.Request) web.Encoder {
	var req model.D
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	modelNameReq, exists := req["model"]
	if !exists {
		return errs.Errorf(errs.InvalidArgument, "missing model field")
	}

	modelName, ok := modelNameReq.(string)
	if !ok {
		return errs.Errorf(errs.InvalidArgument, "model name must be a string")
	}

	krn, err := a.krnMgr.AquireModel(ctx, modelName)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	ctx, cancel := context.WithTimeout(ctx, 180*time.Minute)
	defer cancel()

	d := toD(req)

	delete(req, "messages")
	delete(req, "tools")
	a.log.Info(ctx, "chat-completions:REQUEST", "req", req)

	if _, err := krn.ChatCompletions(ctx, a.log.Info, web.GetWriter(ctx), d); err != nil {
		return errs.New(errs.Internal, err)
	}

	return web.NewNoResponse()
}

// Package chatapp provides the chat api endpoints.
package chatapp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

type app struct {
	log   *logger.Logger
	cache *cache.Cache
}

func newApp(cfg Config) *app {
	return &app{
		log:   cfg.Log,
		cache: cfg.Cache,
	}
}

func (a *app) chatCompletions(ctx context.Context, r *http.Request) web.Encoder {
	var req model.D
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	modelIDReq, exists := req["model"]
	if !exists {
		return errs.Errorf(errs.InvalidArgument, "missing model field")
	}

	modelID, ok := modelIDReq.(string)
	if !ok {
		return errs.Errorf(errs.InvalidArgument, "model name must be a string")
	}

	krn, err := a.cache.AquireModel(ctx, modelID)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	a.log.Info(ctx, "chat-completions", "REQUEST-PARAMS", req.String())

	ctx, cancel := context.WithTimeout(ctx, 180*time.Minute)
	defer cancel()

	d := model.MapToModelD(req)

	if imcID := r.Header.Get("KRONK_IMC_ID"); imcID != "" {
		d["imc_id"] = imcID
	}

	if _, err := krn.ChatStreamingHTTP(ctx, web.GetWriter(ctx), d); err != nil {
		return errs.New(errs.Internal, err)
	}

	return web.NewNoResponse()
}

// Package embedapp provides the embedding api endpoints.
package embedapp

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

func (a *app) embeddings(ctx context.Context, r *http.Request) web.Encoder {
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

	if !krn.ModelInfo().IsEmbedModel {
		return errs.Errorf(errs.InvalidArgument, "model doesn't support embedding")
	}

	a.log.Info(ctx, "embedding", "req", req)

	ctx, cancel := context.WithTimeout(ctx, 180*time.Minute)
	defer cancel()

	d := model.MapToModelD(req)

	if _, err := krn.EmbeddingsHTTP(ctx, a.log.Info, web.GetWriter(ctx), d); err != nil {
		return errs.New(errs.Internal, err)
	}

	return web.NewNoResponse()
}

package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// selectAndLoadDraft dispatches the resolved plan to its model-facing loader.
// Algorithm loaders own initialization and cleanup of partially built resources;
// the model owns the returned drafter and unloads it with the target.
func selectAndLoadDraft(ctx context.Context, log applog.Logger, cfg Config, targetCtx llama.Context, targetModel llama.Model, targetCtxParams llama.ContextParams, plan internalspec.Plan) (drafter, error) {
	switch plan.Source {
	case speculationSourceNone:
		return nil, nil

	case internalspec.SourceDSpark:
		return loadDSpark(ctx, log, cfg, targetCtx, targetModel, targetCtxParams, plan)

	case speculationSourceClassic:
		d, err := loadDraftModel(ctx, log, cfg, targetModel, targetCtxParams)
		if err != nil {
			return nil, err
		}
		log(ctx, "draft-model", "status", "loaded",
			"source", "explicit-separate",
			"nDraft", d.c.nDraft, "devices", cfg.PtrDraftModel.Devices,
			"nCtx", llama.NCtx(d.c.lctx))
		return d, nil

	case speculationSourceMTP:
		return loadMTP(ctx, log, cfg, targetCtx, targetModel, targetCtxParams, plan)
	}

	return nil, fmt.Errorf("unsupported speculation source %d", plan.Source)
}

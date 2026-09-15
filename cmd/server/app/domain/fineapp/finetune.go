package fineapp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

func (a *app) startRun(ctx context.Context, r *http.Request) web.Encoder {
	if a.finetuner == nil {
		return errs.Errorf(errs.FailedPrecondition, "fine-tuning is not configured")
	}

	var req StartRunRequest
	if err := web.Decode(r, &req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	dataset, err := a.datasetFile(req.DatasetID, "dataset.jsonl")
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	model, err := a.modelFile(req.ModelID)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	run, err := a.finetuner.Start(context.WithoutCancel(ctx), finetune.Request{
		Dataset: dataset,
		Model:   model,
		Runtime: a.runtime,
	}, nil)
	if err != nil {
		return errs.New(errs.FailedPrecondition, fmt.Errorf("start fine-tuning run: %w", err))
	}

	return jsonResponse{value: runFromBusiness(run)}
}

func (a *app) currentRun(_ context.Context, _ *http.Request) web.Encoder {
	if a.finetuner == nil {
		return errs.Errorf(errs.FailedPrecondition, "fine-tuning is not configured")
	}

	run, exists := a.finetuner.Current()
	if !exists {
		return errs.Errorf(errs.NotFound, "no fine-tuning run exists")
	}
	return jsonResponse{value: runFromBusiness(run)}
}

func (a *app) history(_ context.Context, _ *http.Request) web.Encoder {
	if a.finetuner == nil {
		return errs.Errorf(errs.FailedPrecondition, "fine-tuning is not configured")
	}

	runs, err := a.finetuner.History()
	if err != nil {
		return errs.FromSDK(fmt.Errorf("load fine-tuning history: %w", err))
	}
	return jsonResponse{value: HistoryResponse{Runs: runsFromBusiness(runs)}}
}

func (a *app) runByID(_ context.Context, r *http.Request) web.Encoder {
	if a.finetuner == nil {
		return errs.Errorf(errs.FailedPrecondition, "fine-tuning is not configured")
	}

	id := web.Param(r, "id")
	if err := validateResourceID(id); err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("run id %w", err))
	}
	runs, err := a.finetuner.History()
	if err != nil {
		return errs.FromSDK(fmt.Errorf("load fine-tuning history: %w", err))
	}
	for _, run := range runs {
		if run.ID == id {
			return jsonResponse{value: runFromBusiness(run)}
		}
	}
	return errs.Errorf(errs.NotFound, "fine-tuning run %q was not found", id)
}

func (a *app) resumeRun(ctx context.Context, r *http.Request) web.Encoder {
	if a.finetuner == nil {
		return errs.Errorf(errs.FailedPrecondition, "fine-tuning is not configured")
	}

	id := web.Param(r, "id")
	if err := validateResourceID(id); err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("run id %w", err))
	}
	run, err := a.finetuner.Resume(context.WithoutCancel(ctx), id, nil)
	if err != nil {
		return errs.New(errs.FailedPrecondition, fmt.Errorf("resume fine-tuning run: %w", err))
	}
	return jsonResponse{value: runFromBusiness(run)}
}

func (a *app) stopRun(_ context.Context, _ *http.Request) web.Encoder {
	if a.finetuner == nil {
		return errs.Errorf(errs.FailedPrecondition, "fine-tuning is not configured")
	}
	if err := a.finetuner.Stop(); err != nil {
		return errs.New(errs.FailedPrecondition, fmt.Errorf("stop fine-tuning run: %w", err))
	}
	return jsonResponse{value: ActionResponse{Status: "stopping"}}
}

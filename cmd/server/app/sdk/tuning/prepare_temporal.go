package tuning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	preparationWorkflowName = "moxie.tuning.Preparation"
	preparationActivityName = "moxie.tuning.PrepareDataset"
)

func preparationWorkflow(ctx workflow.Context, input preparationInput) (PreparationRun, error) {
	info := workflow.GetInfo(ctx)
	run := PreparationRun{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID, Request: input.Request, Kind: input.Kind, Status: StatusRunning, StartedAt: workflow.Now(ctx)}
	if err := workflow.SetQueryHandler(ctx, stateQuery, func() (PreparationRun, error) { return run, nil }); err != nil {
		return PreparationRun{}, err
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: input.Timeout, HeartbeatTimeout: 5 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3}})
	var result PreparationResult
	if err := workflow.ExecuteActivity(ctx, preparationActivityName, input, run.Execution).Get(ctx, &result); err != nil {
		return PreparationRun{}, err
	}
	run.Result = &result
	run.Status = StatusCompleted
	run.ClosedAt = workflow.Now(ctx)
	return run, nil
}

func (b *Business) prepareDataset(ctx context.Context, input preparationInput, e Execution) (PreparationResult, error) {
	// The execution identity, not caller-supplied paths, owns this workspace.
	info := activity.GetInfo(ctx)
	if e.WorkflowID != info.WorkflowExecution.ID || e.RunID != info.WorkflowExecution.RunID {
		return PreparationResult{}, temporal.NewNonRetryableApplicationError("invalid output for tuning stage prepare", "invalid-stage-output", nil)
	}
	publish, stop := activityProgress(ctx, e, StagePrepare, b.progress.Publish)
	defer stop()
	dataset, err := b.DatasetSource(ctx, input.Request.DatasetID)
	if err != nil {
		if errors.Is(err, ErrInvalidArgument) || errors.Is(err, ErrNotFound) {
			return PreparationResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-dataset", err)
		}
		return PreparationResult{}, err
	}
	root, err := os.OpenRoot(b.cfg.DatasetRoot)
	if err != nil {
		return PreparationResult{}, err
	}
	defer root.Close()
	path := filepath.Join("preparations", e.RunID)
	if err := root.MkdirAll(path, 0o700); err != nil {
		return PreparationResult{}, err
	}
	workspace, err := root.OpenRoot(path)
	if err != nil {
		return PreparationResult{}, err
	}
	defer workspace.Close()
	resultPath := "result.json"
	if data, err := workspace.ReadFile(resultPath); err == nil {
		var result PreparationResult
		if err := json.Unmarshal(data, &result); err != nil {
			return PreparationResult{}, err
		}
		if result.DatasetID != dataset.ID {
			return PreparationResult{}, fmt.Errorf("preparation result identity mismatch")
		}
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return PreparationResult{}, err
	}
	engine := preparationEngine{Extractor: b.cfg.Extractor, Completer: b.cfg.Completer}
	result, err := engine.Build(ctx, input, dataset, root, workspace, publish)
	if err != nil {
		if errors.Is(err, ErrInvalidArgument) {
			return PreparationResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-dataset", err)
		}
		return PreparationResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return PreparationResult{}, err
	}
	if err := writeJSON(workspace, resultPath, result); err != nil {
		return PreparationResult{}, err
	}
	return result, nil
}

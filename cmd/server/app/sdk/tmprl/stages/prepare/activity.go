package prepare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

const preparationWorkflowName = WorkflowName

func (b *Business) prepareDataset(ctx context.Context, input preparationInput, e Execution) (PreparationResult, error) {
	// The execution identity, not caller-supplied paths, owns this workspace.
	info := activity.GetInfo(ctx)
	if e.WorkflowID != info.WorkflowExecution.ID || e.RunID != info.WorkflowExecution.RunID {
		return PreparationResult{}, temporal.NewNonRetryableApplicationError("invalid output for tuning stage prepare", "invalid-stage-output", nil)
	}
	publish, stop := model.ActivityProgress(ctx, e, StagePrepare, b.publishProgress)
	defer stop()
	source, err := b.cfg.Datasets.DatasetSource(ctx, input.Request.DatasetID)
	if err != nil {
		if errors.Is(err, ErrInvalidArgument) || errors.Is(err, ErrNotFound) {
			return PreparationResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-dataset", err)
		}
		return PreparationResult{}, err
	}
	root, err := os.OpenRoot(b.cfg.Datasets.Root())
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
		if result.DatasetID != source.ID {
			return PreparationResult{}, fmt.Errorf("preparation result identity mismatch")
		}
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return PreparationResult{}, err
	}
	engine := Engine{Extractor: b.cfg.Extractor, Completer: b.cfg.Completer}
	result, err := engine.Build(ctx, input, source, root, workspace, publish)
	if err != nil {
		if errors.Is(err, ErrInvalidArgument) {
			return PreparationResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-dataset", err)
		}
		return PreparationResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return PreparationResult{}, err
	}
	if err := dataset.WriteJSON(workspace, resultPath, result); err != nil {
		return PreparationResult{}, err
	}
	return result, nil
}

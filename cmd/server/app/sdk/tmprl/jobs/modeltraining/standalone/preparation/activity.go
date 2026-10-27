package prepare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

const preparationWorkflowName = WorkflowName

// prepareDataset is the Temporal activity boundary: it validates execution
// identity, supplies a confined workspace and heartbeats, and reuses a completed
// result on retry. The local Engine coordinates the preparation steps.
func (b *Business) prepareDataset(ctx context.Context, input preparation.PreparationInput, e tmprl.Execution) (output preparation.PreparationResult, err error) {
	// The execution identity, not caller-supplied paths, owns this workspace.
	info := activity.GetInfo(ctx)
	log := activity.GetLogger(ctx)
	started := time.Now()
	log.Info("preparation job started", "dataset_id", input.Request.DatasetID, "kind", input.Kind, "attempt", info.Attempt, "worker_queue", info.TaskQueue, "deadline", info.Deadline)
	defer func() {
		if err != nil {
			log.Error("preparation job failed", "attempt", info.Attempt, "elapsed", time.Since(started), "error", err)
			return
		}
		log.Info("preparation job finished", "attempt", info.Attempt, "elapsed", time.Since(started), "records", output.RecordCount, "training_ready", output.TrainingReady, "prompt_tokens", output.PromptTokens, "completion_tokens", output.CompletionTokens, "warnings", len(output.Warnings))
	}()
	if e.WorkflowID != info.WorkflowExecution.ID || e.RunID != info.WorkflowExecution.RunID {
		return preparation.PreparationResult{}, temporal.NewNonRetryableApplicationError("invalid output for tuning stage prepare", "invalid-stage-output", nil)
	}
	publish, stop := activityProgress(ctx, e, preparation.StagePrepare, b.publishProgress)
	defer stop()
	source, err := b.cfg.Datasets.DatasetSource(ctx, input.Request.DatasetID)
	if err != nil {
		if errors.Is(err, ErrInvalidArgument) || errors.Is(err, ErrNotFound) {
			return preparation.PreparationResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-dataset", err)
		}
		return preparation.PreparationResult{}, err
	}
	root, err := os.OpenRoot(b.cfg.Datasets.Root())
	if err != nil {
		return preparation.PreparationResult{}, err
	}
	defer root.Close()
	path := filepath.Join("preparations", e.RunID)
	if err := root.MkdirAll(path, 0o700); err != nil {
		return preparation.PreparationResult{}, err
	}
	workspace, err := root.OpenRoot(path)
	if err != nil {
		return preparation.PreparationResult{}, err
	}
	defer workspace.Close()
	resultPath := "result.json"
	if data, err := workspace.ReadFile(resultPath); err == nil {
		var result preparation.PreparationResult
		if err := json.Unmarshal(data, &result); err != nil {
			return preparation.PreparationResult{}, err
		}
		if result.DatasetID != source.ID {
			return preparation.PreparationResult{}, fmt.Errorf("preparation result identity mismatch")
		}
		log.Info("preparation checkpoint reused", "dataset_id", source.ID, "records", result.RecordCount)
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return preparation.PreparationResult{}, err
	}
	engine := Engine{Extractor: b.cfg.Extractor, Completer: b.cfg.Completer}
	result, err := engine.Build(ctx, input, source, root, workspace, func(update tmprl.Progress) {
		log.Info("preparation progress", "phase", update.Message, "completed", update.Completed, "total", update.Total)
		publish(update)
	})
	if err != nil {
		if errors.Is(err, ErrInvalidArgument) {
			return preparation.PreparationResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-dataset", err)
		}
		return preparation.PreparationResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return preparation.PreparationResult{}, err
	}
	if err := tmprl.WriteJSON(workspace, resultPath, result); err != nil {
		return preparation.PreparationResult{}, err
	}
	return result, nil
}

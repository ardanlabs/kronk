package modeltraining

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

const (
	workflowName  = WorkflowName
	stateQuery    = StateQuery
	controlSignal = ControlSignal
)

type workflowInput = Input

// ExecuteStage preserves the activity boundary for legacy flat-pipeline steps.
// New jobs execute steps through ExecuteJobStep; their stages are YAML groups.
func (b *Business) ExecuteStage(ctx context.Context, input LegacyStepRequest) (result tmprl.StepResult[model.Artifacts], err error) {
	info := activity.GetInfo(ctx)
	log := activity.GetLogger(ctx)
	started := time.Now()
	log.Info("training stage started", "stage", input.Stage, "operation_key", input.Key, "attempt", info.Attempt, "worker_queue", info.TaskQueue, "scheduled_at", info.ScheduledTime, "deadline", info.Deadline)
	defer func() {
		result.Attempt = info.Attempt
		if err != nil {
			log.Error("training stage failed", "stage", input.Stage, "operation_key", input.Key, "attempt", info.Attempt, "elapsed", time.Since(started), "error", err)
			return
		}
		log.Info("training stage finished", "stage", input.Stage, "attempt", info.Attempt, "elapsed", time.Since(started), "outcome", result.Status, "accepted", result.Artifacts.Accepted)
	}()
	if input.Execution != (tmprl.Execution{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID}) || input.Key != input.RunID+"/"+string(input.Stage) {
		return tmprl.StepResult[model.Artifacts]{}, model.InvalidOutput(string(input.Stage))
	}
	stage, exists := b.legacySteps[input.Stage]
	if !exists {
		return tmprl.StepResult[model.Artifacts]{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("stage %q is not registered on this worker", input.Stage), "unregistered-stage", nil)
	}
	publish, stop := model.ActivityProgress(ctx, input.Execution, input.Stage, b.publishProgress)
	defer stop()
	report := func(update tmprl.Progress) {
		log.Debug("training stage progress", "stage", input.Stage, "completed", update.Completed, "total", update.Total)
		publish(update)
	}
	switch {
	case input.Request.DatasetID != "" || input.Request.Preparation != (tmprl.Execution{}):
		result, err = b.managedLegacyStep(ctx, input, report)
	case b.cfg.Backend != nil:
		// Compatibility for the original Backend API. New pipelines register
		// StepHandler implementations in Config.Steps and order them in Config.Job.
		var output Artifacts
		output, err = b.cfg.Backend.RunStage(ctx, input, report)
		if err == nil {
			result, err = legacyStepResult(input, output)
		}
	case b.customLegacySteps:
		result, err = stage.Execute(ctx, model.StepContext{Request: input, Publish: report})
	default:
		err = temporal.NewNonRetryableApplicationError("training requires managed input or a configured backend", "invalid-training", nil)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && result.Status != "" && result.Status != tmprl.StatusCompleted && result.Status != model.StatusNeedsImprovement {
		return tmprl.StepResult[model.Artifacts]{}, model.InvalidOutput(string(input.Stage))
	}
	return result, err
}

func legacyStepResult(input LegacyStepRequest, output Artifacts) (tmprl.StepResult[model.Artifacts], error) {
	artifacts, err := legacyStepArtifacts(input, output)
	result := tmprl.StepResult[model.Artifacts]{Artifacts: artifacts}
	if input.Stage == StageEvaluate && !artifacts.Accepted {
		result.Status = model.StatusNeedsImprovement
	}
	return result, err
}

// RunStage is the registered training activity entrypoint. It supplies progress
// heartbeats, delegates managed input to trainingStage in pipeline.go (or uses
// the configured Backend), and validates the returned stage artifacts.
func (b *Business) RunStage(ctx context.Context, input LegacyStepRequest) (Artifacts, error) {
	publish, stop := model.ActivityProgress(ctx, input.Execution, input.Stage, b.publishProgress)
	defer stop()
	var output Artifacts
	var err error
	switch {
	case input.Request.DatasetID != "" || input.Request.Preparation != (tmprl.Execution{}):
		output, err = b.trainingLegacyStep(ctx, input, publish)
	case b.cfg.Backend != nil:
		output, err = b.cfg.Backend.RunStage(ctx, input, publish)
	default:
		return Artifacts{}, temporal.NewNonRetryableApplicationError("training backend is not configured", "invalid-training", nil)
	}
	if err != nil {
		return Artifacts{}, err
	}
	if err := ctx.Err(); err != nil {
		return Artifacts{}, err
	}
	return legacyStepArtifacts(input, output)
}

func legacyStepArtifacts(input LegacyStepRequest, output Artifacts) (Artifacts, error) {
	result := input.Artifacts
	switch input.Stage {
	case StageSplit:
		result.Dataset = output.Dataset
		if result.Dataset == "" {
			return Artifacts{}, invalidOutput(input.Stage)
		}
	case StageTrain:
		result.Checkpoint = output.Checkpoint
		if result.Checkpoint == "" {
			return Artifacts{}, invalidOutput(input.Stage)
		}
	case StageEvaluate:
		result.Report = output.Report
		result.Accepted = output.Accepted
		if result.Report == "" {
			return Artifacts{}, invalidOutput(input.Stage)
		}
	case StagePublish:
		if !result.Accepted {
			return Artifacts{}, invalidOutput(input.Stage)
		}
		result.Adapter = output.Adapter
		if result.Adapter == "" {
			return Artifacts{}, invalidOutput(input.Stage)
		}
	default:
		return Artifacts{}, invalidOutput(input.Stage)
	}
	return result, nil
}

func invalidOutput(stage tmprl.Stage) error {
	return model.InvalidOutput(string(stage))
}

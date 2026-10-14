package tuning

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	workflowName  = "moxie.tuning.Run"
	activityName  = "moxie.tuning.Stage"
	stateQuery    = "tuning-state"
	controlSignal = "tuning-control"
)

type workflowInput struct {
	Request      Request       `json:"request"`
	StageTimeout time.Duration `json:"stage_timeout"`
}

func runWorkflow(ctx workflow.Context, input workflowInput) (Run, error) {
	info := workflow.GetInfo(ctx)
	state := Run{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID, Request: input.Request, Status: StatusRunning, StartedAt: workflow.Now(ctx)}
	if err := workflow.SetQueryHandler(ctx, stateQuery, func() (Run, error) { return state, nil }); err != nil {
		return Run{}, err
	}
	paused := false
	controls := workflow.GetSignalChannel(ctx, controlSignal)
	workflow.Go(ctx, func(ctx workflow.Context) {
		for {
			var command string
			if !controls.Receive(ctx, &command) {
				return
			}
			switch command {
			case "pause":
				paused = true
				state.Status = StatusPauseRequested
			case "resume":
				paused = false
				state.Status = StatusRunning
			}
		}
	})
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: input.StageTimeout, HeartbeatTimeout: 5 * time.Second,
		WaitForCancellation: true,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	})
	// This prepares the training splits, not the source documents. Document
	// preparation has its own execution in prepare_temporal.go.
	for _, stage := range []Stage{StageSplit, StageTrain, StageEvaluate, StagePublish} {
		if paused {
			state.Status = StatusPaused
			state.Stage = ""
			if err := workflow.Await(ctx, func() bool { return !paused }); err != nil {
				return Run{}, err
			}
		}
		state.Stage = stage
		request := StageRequest{Execution: state.Execution, Key: state.RunID + "/" + string(stage), Stage: stage, Request: input.Request, Artifacts: state.Artifacts}
		if err := workflow.ExecuteActivity(ctx, activityName, request).Get(ctx, &state.Artifacts); err != nil {
			return Run{}, err
		}
		if stage == StageEvaluate && !state.Artifacts.Accepted {
			state.Status = StatusNeedsImprovement
			state.Stage = ""
			state.ClosedAt = workflow.Now(ctx)
			return state, nil
		}
	}
	state.Status = StatusCompleted
	state.Stage = ""
	state.ClosedAt = workflow.Now(ctx)
	return state, nil
}

func (b *Business) runStage(ctx context.Context, input StageRequest) (Artifacts, error) {
	publish, stop := activityProgress(ctx, input.Execution, input.Stage, b.progress.Publish)
	defer stop()
	var output Artifacts
	var err error
	switch {
	case input.Request.DatasetID != "" || input.Request.Preparation != (Execution{}):
		output, err = b.trainingStage(ctx, input, publish)
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
	return stageArtifacts(input, output)
}

func stageArtifacts(input StageRequest, output Artifacts) (Artifacts, error) {
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

func invalidOutput(stage Stage) error {
	return temporal.NewNonRetryableApplicationError(fmt.Sprintf("invalid output for tuning stage %s", stage), "invalid-stage-output", nil)
}

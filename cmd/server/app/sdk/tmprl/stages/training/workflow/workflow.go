// Package training owns the main training workflow, separate from document
// preparation. Its independent substages perform split, train, evaluate, and
// publish computations; the provider package supplies computation adapters only.
package training

import (
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Temporal names remain stable for existing clients and workflow histories.
const (
	WorkflowName  = "moxie.tuning.Run"
	ActivityName  = "moxie.tuning.Stage"
	StateQuery    = "tuning-state"
	ControlSignal = "tuning-control"
)

// Input is the durable training workflow payload.
type Input struct {
	Request      model.Request `json:"request"`
	StageTimeout time.Duration `json:"stage_timeout"`
}

// Workflow orchestrates the main training execution, not source preparation.
// It owns stage order, retries, cooperative cancellation, state queries, and
// pause/resume at stage boundaries. Rejected evaluations never reach publication.
// Each stage schedules RunStage in activity.go as a separate activity execution;
// managed input is dispatched to the stage's Run function through pipeline.go.
func Workflow(ctx workflow.Context, input Input) (model.Run, error) {
	info := workflow.GetInfo(ctx)
	state := model.Run{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID, Request: input.Request, Status: model.StatusRunning, StartedAt: workflow.Now(ctx)}
	if err := workflow.SetQueryHandler(ctx, StateQuery, func() (model.Run, error) { return state, nil }); err != nil {
		return model.Run{}, err
	}
	paused := false
	controls := workflow.GetSignalChannel(ctx, ControlSignal)
	workflow.Go(ctx, func(ctx workflow.Context) {
		for {
			var command string
			if !controls.Receive(ctx, &command) {
				return
			}
			switch command {
			case "pause":
				paused = true
				state.Status = model.StatusPauseRequested
			case "resume":
				paused = false
				state.Status = model.StatusRunning
			}
		}
	})
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: input.StageTimeout, HeartbeatTimeout: 5 * time.Second,
		WaitForCancellation: true,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	})
	// Split prepares training splits, not the source documents.
	for _, stage := range []model.Stage{model.StageSplit, model.StageTrain, model.StageEvaluate, model.StagePublish} {
		if paused {
			state.Status = model.StatusPaused
			state.Stage = ""
			if err := workflow.Await(ctx, func() bool { return !paused }); err != nil {
				return model.Run{}, err
			}
		}
		state.Stage = stage
		request := model.StageRequest{Execution: state.Execution, Key: state.RunID + "/" + string(stage), Stage: stage, Request: input.Request, Artifacts: state.Artifacts}
		if err := workflow.ExecuteActivity(ctx, ActivityName, request).Get(ctx, &state.Artifacts); err != nil {
			return model.Run{}, err
		}
		if stage == model.StageEvaluate && !state.Artifacts.Accepted {
			state.Status = model.StatusNeedsImprovement
			state.Stage = ""
			state.ClosedAt = workflow.Now(ctx)
			return state, nil
		}
	}
	state.Status = model.StatusCompleted
	state.Stage = ""
	state.ClosedAt = workflow.Now(ctx)
	return state, nil
}

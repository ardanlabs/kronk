// Package modeltraining owns model-training jobs: one workflow and reserved worker
// execute the YAML-defined stages and their steps. Preparation and training are
// stages of that job; computation providers never schedule Temporal work.
package modeltraining

import (
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Temporal names remain stable for existing clients and workflow histories.
const (
	WorkflowName = "moxie.tuning.Run"
	ActivityName = "moxie.tuning.Stage"
	// SessionActivityName preserves the legacy flat-pipeline step activity name.
	SessionActivityName = "moxie.tuning.SessionStage"
	StateQuery          = "tuning-state"
	ControlSignal       = "tuning-control"
)

// Input is the durable training workflow payload.
type Input struct {
	Job          *tmprl.Job        `json:"job,omitzero"`
	Request      Request           `json:"request"`
	StageTimeout time.Duration     `json:"stage_timeout"`
	Stages       []tmprl.StageName `json:"stages,omitzero"`
	JobTimeout   time.Duration     `json:"job_timeout,omitzero"`
	QueueTimeout time.Duration     `json:"queue_timeout,omitzero"`
}

// Workflow executes the submitted Job -> Stage -> Step definition, including
// preparation when requested. Old payloads retain their original flat training
// orchestration for replay. New jobs pause between steps, not inside a step.
func Workflow(ctx workflow.Context, input Input) (tmprl.Run[Request, Artifacts], error) {
	if input.Job != nil {
		return tmprl.RunJob[Request, Artifacts](ctx, tmprl.JobInput[Request]{Job: input.Job, Request: input.Request}, jobNames())
	}
	if workflow.GetVersion(ctx, "whole-job-session", workflow.DefaultVersion, 1) == workflow.DefaultVersion {
		return legacyWorkflow(ctx, input)
	}
	return managedWorkflow(ctx, input)
}

// legacyWorkflow replays histories created before whole-job sessions. Its
// activity names, payloads, and scheduling order must remain unchanged.
func legacyWorkflow(ctx workflow.Context, input Input) (tmprl.Run[Request, Artifacts], error) {
	info := workflow.GetInfo(ctx)
	state := tmprl.Run[Request, Artifacts]{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID, Request: input.Request, Status: tmprl.StatusRunning, StartedAt: workflow.Now(ctx)}
	if err := workflow.SetQueryHandler(ctx, StateQuery, func() (tmprl.Run[Request, Artifacts], error) { return state, nil }); err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
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
				if paused {
					continue
				}
				paused = true
				state.Status = tmprl.StatusPauseRequested
			case "resume":
				if !paused {
					continue
				}
				paused = false
				state.Status = tmprl.StatusRunning
			}
		}
	})
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: input.StageTimeout, HeartbeatTimeout: 5 * time.Second,
		WaitForCancellation: true,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	})
	// Split prepares training splits, not the source documents.
	for _, stage := range []tmprl.StageName{StageSplit, StageTrain, StageEvaluate, StagePublish} {
		if paused {
			state.Status = tmprl.StatusPaused
			state.Stage = ""
			if err := workflow.Await(ctx, func() bool { return !paused }); err != nil {
				return tmprl.Run[Request, Artifacts]{}, err
			}
		}
		state.Stage = stage
		request := LegacyStepRequest{Execution: state.Execution, Key: state.RunID + "/" + string(stage), Stage: stage, Request: input.Request, Artifacts: state.Artifacts}
		if err := workflow.ExecuteActivity(ctx, ActivityName, request).Get(ctx, &state.Artifacts); err != nil {
			return tmprl.Run[Request, Artifacts]{}, err
		}
		if stage == StageEvaluate && !state.Artifacts.Accepted {
			state.Status = training.StatusNeedsImprovement
			state.Stage = ""
			state.ClosedAt = workflow.Now(ctx)
			return state, nil
		}
	}
	state.Status = tmprl.StatusCompleted
	state.Stage = ""
	state.ClosedAt = workflow.Now(ctx)
	return state, nil
}

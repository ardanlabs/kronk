package prepare

import (
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	WorkflowName = "moxie.tuning.Preparation"
	ActivityName = "moxie.tuning.PrepareDataset"
	StateQuery   = "tuning-state"
)

func Workflow(ctx workflow.Context, input model.PreparationInput) (model.PreparationRun, error) {
	info := workflow.GetInfo(ctx)
	run := model.PreparationRun{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID, Request: input.Request, Kind: input.Kind, Status: model.StatusRunning, StartedAt: workflow.Now(ctx)}
	if err := workflow.SetQueryHandler(ctx, StateQuery, func() (model.PreparationRun, error) { return run, nil }); err != nil {
		return model.PreparationRun{}, err
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: input.Timeout, HeartbeatTimeout: 5 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3}})
	var result model.PreparationResult
	if err := workflow.ExecuteActivity(ctx, ActivityName, input, run.Execution).Get(ctx, &result); err != nil {
		return model.PreparationRun{}, err
	}
	run.Result = &result
	run.Status = model.StatusCompleted
	run.ClosedAt = workflow.Now(ctx)
	return run, nil
}

package prepare

import (
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	WorkflowName = "moxie.tuning.Preparation"
	ActivityName = "moxie.tuning.PrepareDataset"
	StateQuery   = "tuning-state"
)

// Workflow schedules one preparation activity, prepareDataset in activity.go.
// That activity calls the local Engine.Build to coordinate extraction,
// generation, and verification, including batching and checkpoint recovery.
// These steps are not separate Temporal activities in the standalone workflow.
func Workflow(ctx workflow.Context, input preparation.PreparationInput) (preparation.PreparationRun, error) {
	info := workflow.GetInfo(ctx)
	run := preparation.PreparationRun{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID, Request: input.Request, Kind: input.Kind, Status: tmprl.StatusRunning, StartedAt: workflow.Now(ctx)}
	if err := workflow.SetQueryHandler(ctx, StateQuery, func() (preparation.PreparationRun, error) { return run, nil }); err != nil {
		return preparation.PreparationRun{}, err
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: input.Timeout, HeartbeatTimeout: 5 * time.Second, WaitForCancellation: true, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3}})
	var result preparation.PreparationResult
	if err := workflow.ExecuteActivity(ctx, ActivityName, input, run.Execution).Get(ctx, &result); err != nil {
		return preparation.PreparationRun{}, err
	}
	run.Result = &result
	run.Status = tmprl.StatusCompleted
	run.ClosedAt = workflow.Now(ctx)
	return run, nil
}

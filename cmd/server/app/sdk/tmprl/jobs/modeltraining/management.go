package modeltraining

import (
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/workflow"
)

const runMemoKey = "moxie.training.run"

func queuedRun(input Input) tmprl.Run[model.Request, model.Artifacts] {
	return tmprl.QueuedLegacyRun[Request, Artifacts](legacyInput(input))
}

// managedWorkflow preserves the session protocol for historical flat-stage jobs.
func managedWorkflow(ctx workflow.Context, input Input) (tmprl.Run[model.Request, model.Artifacts], error) {
	names := tmprl.Names{Activity: SessionActivityName, Memo: runMemoKey, Query: StateQuery, Control: ControlSignal}
	return tmprl.RunLegacyStages[Request, Artifacts](ctx, legacyInput(input), names, "invalid-training")
}

func legacyInput(input Input) tmprl.LegacyInput[Request] {
	return tmprl.LegacyInput[Request]{
		Request: input.Request, Stages: input.Stages, StageTimeout: input.StageTimeout,
		JobTimeout: input.JobTimeout, QueueTimeout: input.QueueTimeout,
	}
}

package modeltraining

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/publish"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/split"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/train"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// DefaultSteps supplies the legacy Start API's flat training-step plan.
// New jobs use Config.Steps for handlers and Config.Job for stage/step ordering.
func DefaultSteps() []TrainingStep {
	return []TrainingStep{trainingAdapter{split.Step{}}, trainingAdapter{train.Step{}}, trainingAdapter{evaluate.Step{}}, trainingAdapter{publish.Step{}}}
}

// trainingAdapter translates whole-job artifacts to the training stage's contract.
// Preparation artifacts remain owned by the job and never reach training handlers.
type trainingAdapter struct {
	implementation training.TrainingStep
}

func (a trainingAdapter) Name() string { return a.implementation.Name() }

func (a trainingAdapter) Execute(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
	incoming := input.Request.Artifacts
	output, err := a.implementation.Execute(ctx, training.StepContext{
		Artifacts: training.Artifacts{Dataset: incoming.Dataset, Checkpoint: incoming.Checkpoint, Report: incoming.Report, Adapter: incoming.Adapter, Accepted: incoming.Accepted},
		Workspace: input.Workspace, Trainer: input.Trainer, Job: input.Job,
		Records: input.Records, Fingerprint: input.Fingerprint, Publish: input.Publish,
	})
	if err != nil {
		return tmprl.StepResult[Artifacts]{}, err
	}
	incoming.Dataset, incoming.Checkpoint = output.Artifacts.Dataset, output.Artifacts.Checkpoint
	incoming.Report, incoming.Adapter, incoming.Accepted = output.Artifacts.Report, output.Artifacts.Adapter, output.Artifacts.Accepted
	return tmprl.StepResult[Artifacts]{Artifacts: incoming, Status: output.Status, Attempt: output.Attempt}, nil
}

func validStepName(name string) bool {
	return tmprl.ValidName(name)
}

// legacyStepID preserves the original split activity's serialized "prepare"
// identity. Only the old payload boundary uses this naming convention.
func legacyStepID(name string) tmprl.StageName {
	if name == "split" {
		return StageSplit
	}
	return tmprl.StageName(name)
}

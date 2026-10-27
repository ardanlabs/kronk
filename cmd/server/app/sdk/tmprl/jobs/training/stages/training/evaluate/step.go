package evaluate

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step evaluates a candidate and owns the job's acceptance decision.
type Step struct{}

// Name identifies the evaluation step within the training stage.
func (Step) Name() string { return "evaluate" }

// Execute merges the evaluation report and ends rejected pipelines.
func (s Step) Execute(ctx context.Context, input training.StepContext) (tmprl.StepResult[training.Artifacts], error) {
	output, err := Run(ctx, input.Workspace, input.Trainer, input.Job)
	if err != nil {
		return tmprl.StepResult[training.Artifacts]{}, err
	}
	if output.Report == "" {
		return tmprl.StepResult[training.Artifacts]{}, training.InvalidOutput(s.Name())
	}
	artifacts := input.Artifacts
	artifacts.Report = output.Report
	artifacts.Accepted = output.Accepted
	result := tmprl.StepResult[training.Artifacts]{Artifacts: artifacts}
	if !output.Accepted {
		result.Status = training.StatusNeedsImprovement
	}
	return result, nil
}

package evaluate

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step evaluates a candidate and owns the job's acceptance decision.
type Step struct{}

// Name identifies the evaluation step within the training stage.
func (Step) Name() string { return "evaluate" }

// Execute merges the evaluation report and ends rejected pipelines.
func (s Step) Execute(ctx context.Context, input model.StepContext) (tmprl.StepResult[model.Artifacts], error) {
	output, err := Run(ctx, input.Workspace, input.Trainer, input.Job)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if output.Report == "" {
		return tmprl.StepResult[model.Artifacts]{}, model.InvalidOutput(s.Name())
	}
	artifacts := input.Request.Artifacts
	artifacts.Report = output.Report
	artifacts.Accepted = output.Accepted
	result := tmprl.StepResult[model.Artifacts]{Artifacts: artifacts}
	if !output.Accepted {
		result.Status = model.StatusNeedsImprovement
	}
	return result, nil
}

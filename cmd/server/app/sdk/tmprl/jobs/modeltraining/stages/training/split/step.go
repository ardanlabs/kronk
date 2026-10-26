package split

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step locks the training splits and preserves earlier artifacts.
type Step struct{}

// Name identifies the split step within the training stage.
func (Step) Name() string { return "split" }

// Execute validates and merges the locked dataset artifact.
func (s Step) Execute(ctx context.Context, input model.StepContext) (tmprl.StepResult[model.Artifacts], error) {
	output, err := Run(ctx, input.Workspace, input.Trainer, input.Job, input.Records, input.Fingerprint)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if output.Dataset == "" {
		return tmprl.StepResult[model.Artifacts]{}, model.InvalidOutput(s.Name())
	}
	artifacts := input.Request.Artifacts
	artifacts.Dataset = output.Dataset
	return tmprl.StepResult[model.Artifacts]{Artifacts: artifacts}, nil
}

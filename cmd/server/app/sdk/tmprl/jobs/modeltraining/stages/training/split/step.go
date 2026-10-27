package split

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step locks the training splits and preserves earlier artifacts.
type Step struct{}

// Name identifies the split step within the training stage.
func (Step) Name() string { return "split" }

// Execute validates and merges the locked dataset artifact.
func (s Step) Execute(ctx context.Context, input training.StepContext) (tmprl.StepResult[training.Artifacts], error) {
	output, err := Run(ctx, input.Workspace, input.Trainer, input.Job, input.Records, input.Fingerprint)
	if err != nil {
		return tmprl.StepResult[training.Artifacts]{}, err
	}
	if output.Dataset == "" {
		return tmprl.StepResult[training.Artifacts]{}, training.InvalidOutput(s.Name())
	}
	artifacts := input.Artifacts
	artifacts.Dataset = output.Dataset
	return tmprl.StepResult[training.Artifacts]{Artifacts: artifacts}, nil
}

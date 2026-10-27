package train

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step produces a candidate and preserves earlier artifacts.
type Step struct{}

// Name identifies the training step within the training stage.
func (Step) Name() string { return "train" }

// Execute validates and merges the candidate checkpoint artifact.
func (s Step) Execute(ctx context.Context, input training.StepContext) (tmprl.StepResult[training.Artifacts], error) {
	output, err := Run(ctx, input.Workspace, input.Trainer, input.Job, input.Publish)
	if err != nil {
		return tmprl.StepResult[training.Artifacts]{}, err
	}
	if output.Checkpoint == "" {
		return tmprl.StepResult[training.Artifacts]{}, training.InvalidOutput(s.Name())
	}
	artifacts := input.Artifacts
	artifacts.Checkpoint = output.Checkpoint
	return tmprl.StepResult[training.Artifacts]{Artifacts: artifacts}, nil
}

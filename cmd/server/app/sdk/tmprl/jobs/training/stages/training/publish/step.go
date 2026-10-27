package publish

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step releases accepted candidates and preserves earlier artifacts.
type Step struct{}

// Name identifies the publication step within the training stage.
func (Step) Name() string { return "publish" }

// Execute validates acceptance and merges the published adapter artifact.
// Pipeline completion remains the orchestrator's responsibility.
func (s Step) Execute(ctx context.Context, input training.StepContext) (tmprl.StepResult[training.Artifacts], error) {
	if !input.Artifacts.Accepted {
		return tmprl.StepResult[training.Artifacts]{}, training.InvalidOutput(s.Name())
	}
	output, err := Run(ctx, input.Workspace, input.Artifacts)
	if err != nil {
		return tmprl.StepResult[training.Artifacts]{}, err
	}
	if output.Adapter == "" {
		return tmprl.StepResult[training.Artifacts]{}, training.InvalidOutput(s.Name())
	}
	artifacts := input.Artifacts
	artifacts.Adapter = output.Adapter
	return tmprl.StepResult[training.Artifacts]{Artifacts: artifacts}, nil
}

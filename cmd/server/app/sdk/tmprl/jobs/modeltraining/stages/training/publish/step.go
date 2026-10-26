package publish

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step releases accepted candidates and preserves earlier artifacts.
type Step struct{}

// Name identifies the publication step within the training stage.
func (Step) Name() string { return "publish" }

// Execute validates acceptance and merges the published adapter artifact.
// Pipeline completion remains the orchestrator's responsibility.
func (s Step) Execute(ctx context.Context, input model.StepContext) (tmprl.StepResult[model.Artifacts], error) {
	if !input.Request.Artifacts.Accepted {
		return tmprl.StepResult[model.Artifacts]{}, model.InvalidOutput(s.Name())
	}
	output, err := Run(ctx, input.Workspace, input.Request.Artifacts)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if output.Adapter == "" {
		return tmprl.StepResult[model.Artifacts]{}, model.InvalidOutput(s.Name())
	}
	artifacts := input.Request.Artifacts
	artifacts.Adapter = output.Adapter
	return tmprl.StepResult[model.Artifacts]{Artifacts: artifacts}, nil
}

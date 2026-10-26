package prepare

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// ExecuteJobStep resolves preparation resources and invokes the supplied step.
// It does not dispatch on names, submit, or require a standalone workflow.
// The caller validates Temporal identity and owns heartbeats/retries.
func (b *Business) ExecuteJobStep(ctx context.Context, step model.StepHandler, input model.PreparationInput, execution tmprl.Execution, artifacts model.Artifacts, publish func(tmprl.Progress)) (tmprl.StepResult[model.Artifacts], error) {
	if err := model.ValidateExecution(execution); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if filepath.Base(execution.RunID) != execution.RunID || execution.RunID == "." || execution.RunID == ".." {
		return tmprl.StepResult[model.Artifacts]{}, fmt.Errorf("%w: invalid preparation run ID", model.ErrInvalidArgument)
	}
	if step == nil {
		return tmprl.StepResult[model.Artifacts]{}, fmt.Errorf("%w: preparation step is required", model.ErrInvalidArgument)
	}
	source, err := b.cfg.Datasets.DatasetSource(ctx, input.Request.DatasetID)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	root, err := os.OpenRoot(b.cfg.Datasets.Root())
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	defer root.Close()
	path := filepath.Join("preparations", execution.RunID)
	if err := root.MkdirAll(path, 0o700); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	workspace, err := root.OpenRoot(path)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	defer workspace.Close()
	return step.Execute(ctx, model.StepContext{
		Preparation: &model.PreparationContext{Input: input, Source: source, SourceRoot: root, Extractor: b.cfg.Extractor, Completer: b.cfg.Completer},
		Workspace:   workspace, Publish: publish,
		Request: model.LegacyStepRequest{Execution: execution, Artifacts: artifacts},
	})
}

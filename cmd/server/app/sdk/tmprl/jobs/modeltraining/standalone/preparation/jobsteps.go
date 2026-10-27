package prepare

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// ExecuteJobStep resolves preparation resources and invokes the supplied step.
// It does not dispatch on names, submit, or require a standalone workflow.
// The caller validates Temporal identity and owns heartbeats/retries.
func (b *Business) ExecuteJobStep(ctx context.Context, step preparation.StepHandler, input preparation.PreparationInput, execution tmprl.Execution, artifacts preparation.Artifacts, publish func(tmprl.Progress)) (tmprl.StepResult[preparation.Artifacts], error) {
	if err := preparation.ValidateExecution(execution); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	if filepath.Base(execution.RunID) != execution.RunID || execution.RunID == "." || execution.RunID == ".." {
		return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: invalid preparation run ID", preparation.ErrInvalidArgument)
	}
	if step == nil {
		return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: preparation step is required", preparation.ErrInvalidArgument)
	}
	source, err := b.cfg.Datasets.DatasetSource(ctx, input.Request.DatasetID)
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	root, err := os.OpenRoot(b.cfg.Datasets.Root())
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	defer root.Close()
	path := filepath.Join("preparations", execution.RunID)
	if err := root.MkdirAll(path, 0o700); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	workspace, err := root.OpenRoot(path)
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	defer workspace.Close()
	return step.Execute(ctx, preparation.StepContext{
		Preparation: &preparation.PreparationContext{Input: input, Source: source, SourceRoot: root, Extractor: b.cfg.Extractor, Completer: b.cfg.Completer},
		Workspace:   workspace, Publish: publish,
		Artifacts: artifacts,
	})
}

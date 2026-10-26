package modeltraining

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/verify"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/publish"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/split"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/train"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

// JobStepActivityName is the activity boundary for YAML-defined job steps.
const JobStepActivityName = "moxie.tuning.JobStep"

type stepFunc func(context.Context, model.StepContext) (tmprl.StepResult[model.Artifacts], error)

func (f stepFunc) Execute(ctx context.Context, input model.StepContext) (tmprl.StepResult[model.Artifacts], error) {
	return f(ctx, input)
}

// builtinSteps registers capabilities only. Ordering and all execution policies
// belong to definition/job.yaml, never to this registry or the runner.
func (b *Business) builtinSteps() map[string]model.StepHandler {
	registry := make(map[string]model.StepHandler)
	for name, implementation := range map[string]model.StepHandler{
		"extract": extract.Step{}, "generate": generate.Step{}, "verify": verify.Step{},
	} {
		registry["preparation/"+name] = stepFunc(func(ctx context.Context, input model.StepContext) (tmprl.StepResult[model.Artifacts], error) {
			r := input.JobStep
			if r.Request.Prepare == nil {
				return tmprl.StepResult[model.Artifacts]{}, temporal.NewNonRetryableApplicationError("document preparation input is required", "invalid-preparation", nil)
			}
			return b.cfg.Preparation.ExecuteJobStep(ctx, implementation, model.PreparationInput{Request: *r.Request.Prepare, Kind: model.PreparationFull}, r.Execution, r.Artifacts, input.Publish)
		})
	}
	for name, implementation := range map[string]model.TrainingStep{
		"split": split.Step{}, "train": train.Step{}, "evaluate": evaluate.Step{}, "publish": publish.Step{},
	} {
		registry["training/"+name] = stepFunc(func(ctx context.Context, input model.StepContext) (tmprl.StepResult[model.Artifacts], error) {
			r := input.Request
			// Preserve old artifact identities for existing built-in implementations.
			r.Stage = legacyStepID(implementation.Name())
			return b.managedStep(ctx, r, implementation, input.Publish)
		})
	}
	return registry
}

// jobSteps adapts application resources to the shared runner's computation seam.
func (b *Business) jobSteps() map[string]tmprl.StepHandler[Request, Artifacts] {
	steps := make(map[string]tmprl.StepHandler[Request, Artifacts], len(b.steps))
	for name, handler := range b.steps {
		steps[name] = tmprl.StepFunc[Request, Artifacts](func(ctx context.Context, input tmprl.StepContext[Request, Artifacts]) (tmprl.StepResult[model.Artifacts], error) {
			r := input.Request
			return handler.Execute(ctx, model.StepContext{
				JobStep: &r, Workspace: input.Workspace, Publish: input.Publish,
				Request: LegacyStepRequest{Execution: r.Execution, Key: r.Key, Stage: tmprl.Stage(r.Step), Request: r.Request, Artifacts: r.Artifacts},
			})
		})
	}
	return steps
}

// ExecuteJobStep retains the registered activity API while the foundation owns execution.
func (b *Business) ExecuteJobStep(ctx context.Context, input tmprl.StepRequest[model.Request, model.Artifacts]) (tmprl.StepResult[model.Artifacts], error) {
	return b.jobs.ExecuteStep(ctx, input)
}

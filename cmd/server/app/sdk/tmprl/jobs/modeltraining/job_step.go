package modeltraining

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/verify"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

// JobStepActivityName is the activity boundary for YAML-defined job steps.
const JobStepActivityName = "moxie.tuning.JobStep"

type stepFunc func(context.Context, StepContext) (tmprl.StepResult[Artifacts], error)

func (f stepFunc) Execute(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
	return f(ctx, input)
}

// builtinSteps registers capabilities only. Ordering and all execution policies
// belong to definition/job.yaml, never to this registry or the runner.
func (b *Business) builtinSteps() map[string]StepHandler {
	registry := make(map[string]StepHandler)
	for name, implementation := range map[string]preparation.StepHandler{
		"extract": extract.Step{}, "generate": generate.Step{}, "verify": verify.Step{},
	} {
		registry["preparation/"+name] = stepFunc(func(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
			r := input.JobStep
			if r.Request.Prepare == nil {
				return tmprl.StepResult[Artifacts]{}, temporal.NewNonRetryableApplicationError("document preparation input is required", "invalid-preparation", nil)
			}
			output, err := b.cfg.Preparation.ExecuteJobStep(ctx, implementation, preparation.PreparationInput{Request: *r.Request.Prepare, Kind: preparation.PreparationFull}, r.Execution, preparation.Artifacts{Preparation: r.Artifacts.Preparation}, input.Publish)
			if err != nil {
				return tmprl.StepResult[Artifacts]{}, err
			}
			artifacts := r.Artifacts
			artifacts.Preparation = output.Artifacts.Preparation
			return tmprl.StepResult[Artifacts]{Artifacts: artifacts, Status: output.Status, Attempt: output.Attempt}, nil
		})
	}
	for _, implementation := range DefaultSteps() {
		registry["training/"+implementation.Name()] = stepFunc(func(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
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
		steps[name] = tmprl.StepFunc[Request, Artifacts](func(ctx context.Context, input tmprl.StepContext[Request, Artifacts]) (tmprl.StepResult[Artifacts], error) {
			r := input.Request
			return handler.Execute(ctx, StepContext{
				JobStep: &r, Workspace: input.Workspace, Publish: input.Publish,
				Request: LegacyStepRequest{Execution: r.Execution, Key: r.Key, Stage: tmprl.StageName(r.Step), Request: r.Request, Artifacts: r.Artifacts},
			})
		})
	}
	return steps
}

// ExecuteJobStep retains the registered activity API while the foundation owns execution.
func (b *Business) ExecuteJobStep(ctx context.Context, input tmprl.StepRequest[Request, Artifacts]) (tmprl.StepResult[Artifacts], error) {
	return b.jobs.ExecuteStep(ctx, input)
}

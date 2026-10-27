package training

import (
	"context"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/verify"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/publish"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/split"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/train"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// steps registers capabilities. Only job.yaml specifies their order and policies.
func (b *Business) steps() map[string]tmprl.StepHandler[Request, Artifacts] {
	return map[string]tmprl.StepHandler[Request, Artifacts]{
		"preparation/extract":  b.prepare(extract.Step{}),
		"preparation/generate": b.prepare(generate.Step{}),
		"preparation/verify":   b.prepare(verify.Step{}),
		"training/split":       b.optimize(split.Step{}),
		"training/train":       b.optimize(train.Step{}),
		"training/evaluate":    b.optimize(evaluate.Step{}),
		"training/publish":     b.optimize(publish.Step{}),
	}
}

func (b *Business) prepare(step preparation.StepHandler) tmprl.StepHandler[Request, Artifacts] {
	return tmprl.StepFunc[Request, Artifacts](func(ctx context.Context, input tmprl.StepContext[Request, Artifacts]) (tmprl.StepResult[Artifacts], error) {
		r := input.Request
		if r.Request.Prepare == nil {
			return tmprl.StepResult[Artifacts]{}, invalidInput("document preparation input is required")
		}
		source, err := b.cfg.Imports.DatasetSource(ctx, r.Request.Prepare.DatasetID)
		if err != nil {
			return tmprl.StepResult[Artifacts]{}, stepError(err)
		}
		root, err := os.OpenRoot(b.cfg.Imports.Root())
		if err != nil {
			return tmprl.StepResult[Artifacts]{}, err
		}
		defer root.Close()
		output, err := step.Execute(ctx, preparation.StepContext{
			Preparation: &preparation.PreparationContext{
				Input:  preparation.PreparationInput{Request: *r.Request.Prepare, Kind: preparation.PreparationFull},
				Source: source, SourceRoot: root, Extractor: b.cfg.Extractor, Completer: b.cfg.Completer,
			},
			Workspace: input.Workspace, Artifacts: preparation.Artifacts{Preparation: r.Artifacts.Preparation}, Publish: input.Publish,
		})
		artifacts := r.Artifacts
		artifacts.Preparation = output.Artifacts.Preparation
		return tmprl.StepResult[Artifacts]{Artifacts: artifacts, Status: output.Status}, stepError(err)
	})
}

func (b *Business) optimize(step optimization.StepHandler) tmprl.StepHandler[Request, Artifacts] {
	return tmprl.StepFunc[Request, Artifacts](func(ctx context.Context, input tmprl.StepContext[Request, Artifacts]) (tmprl.StepResult[Artifacts], error) {
		resources, err := b.trainingContext(ctx, input)
		if err != nil {
			return tmprl.StepResult[Artifacts]{}, stepError(err)
		}
		defer os.RemoveAll(resources.Job.Workdir)
		output, err := step.Execute(ctx, resources)
		artifacts := input.Request.Artifacts
		artifacts.Training = output.Artifacts
		return tmprl.StepResult[Artifacts]{Artifacts: artifacts, Status: output.Status}, stepError(err)
	})
}

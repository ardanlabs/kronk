package extract

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step extracts source passages and commits the preparation checkpoint.
type Step struct{}

// Name identifies the extraction step within the preparation stage.
func (Step) Name() string { return "extract" }

// Execute initializes source-grounded records and passages once per execution.
// Replays preserve committed generation and verification progress.
func (Step) Execute(ctx context.Context, input model.StepContext) (tmprl.StepResult[model.Artifacts], error) {
	state, exists, err := checkpoint.Load(ctx, input, true)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if !exists {
		p := input.Preparation
		document, err := Run(ctx, p.Extractor, p.Source, p.SourceRoot, input.Workspace)
		if err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
		passages, excluded := Passages(document.Markdown)
		available := len(passages)
		passages = slices.DeleteFunc(passages, func(passage string) bool { return len(generate.Targets(passage, p.Input.Request.Goal)) == 0 })
		if len(passages) == 0 {
			return tmprl.StepResult[model.Artifacts]{}, fmt.Errorf("document contains no supported source passages for this goal")
		}
		if excluded+available-len(passages) > 0 {
			state.Result.Warnings = append(state.Result.Warnings, fmt.Sprintf("%d source passages excluded: too long or no supported targets for this goal", excluded+available-len(passages)))
		}
		if p.Input.Kind == model.PreparationPreview {
			count := min(p.Input.Request.PreviewRecords, len(passages))
			for i := range count {
				state.Passages = append(state.Passages, passages[i*len(passages)/count])
			}
		} else {
			state.Records = slices.Clone(p.Input.Request.ReviewedRecords)
			for i, r := range state.Records {
				r.Prompt = strings.TrimSpace(r.Prompt)
				r.Target = dataset.Normalized(r.Target)
				r.Evidence = dataset.Normalized(r.Evidence)
				if !strings.Contains(dataset.Normalized(document.Markdown), r.Evidence) {
					return tmprl.StepResult[model.Artifacts]{}, fmt.Errorf("reviewed evidence is not in the imported document")
				}
				if err := generate.ValidateRecord(p.Input.Request.Goal, r); err != nil {
					return tmprl.StepResult[model.Artifacts]{}, err
				}
				r.FactID = dataset.FactID(r.Evidence, r.Target)
				r.SourceDocument = p.Source.ID
				state.Records[i] = r
			}
			state.Passages = slices.DeleteFunc(passages, func(passage string) bool {
				return slices.ContainsFunc(state.Records, func(r model.Record) bool { return r.Evidence == passage })
			})
		}
		if err := dataset.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
	}
	if input.Publish != nil {
		input.Publish(tmprl.Progress{Completed: 1, Total: 1, Message: "Source extraction complete"})
	}
	return tmprl.StepResult[model.Artifacts]{Artifacts: input.Request.Artifacts}, nil
}

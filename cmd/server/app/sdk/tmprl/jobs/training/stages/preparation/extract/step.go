package extract

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"go.temporal.io/sdk/temporal"
)

// Step extracts source passages and commits the preparation checkpoint.
type Step struct{}

// Name identifies the extraction step within the preparation stage.
func (Step) Name() string { return "extract" }

// Execute initializes source-grounded records and passages once per execution.
// Replays preserve committed generation and verification progress.
func (Step) Execute(ctx context.Context, input preparation.StepContext) (tmprl.StepResult[preparation.Artifacts], error) {
	state, exists, err := checkpoint.Load(ctx, input, true)
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	if !exists {
		p := input.Preparation
		pages := providers.DocumentOptions{PageStart: p.Input.Request.PageStart, PageEnd: p.Input.Request.PageEnd}
		// A page range has a known total; a whole document reports pages read without one.
		total := 0
		if pages.PageStart > 0 {
			total = pages.PageEnd - pages.PageStart + 1
		}
		var progress func(int)
		if input.Publish != nil {
			message := "Reading source document"
			switch p.Source.Format {
			case ".pdf":
				message = "Converting PDF pages; waiting for the next completed page batch"
			case ".docx":
				message = "Converting Word document; waiting for extraction results"
			}
			input.Publish(tmprl.Progress{Total: total, Message: message})
			progress = func(read int) {
				input.Publish(tmprl.Progress{Completed: read, Total: total, Message: "Page batch extracted; continuing document conversion"})
			}
		}
		document, err := Run(ctx, p.Extractor, p.Source, p.OpenSource, input.Workspace, progress, pages)
		if err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		// The provider's whole-document result is durable here. This does not
		// claim that an in-flight Docling conversion can be suspended/recovered.
		if input.SafePoint != nil {
			if err := input.SafePoint(ctx, "extraction.json"); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
		}
		units, exclusions, err := SourceUnits(document, p.Input.Request.Goal)
		if err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		if len(units) == 0 {
			state.Result.Exclusions = exclusions
			if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
				return tmprl.StepResult[preparation.Artifacts]{}, err
			}
			return tmprl.StepResult[preparation.Artifacts]{}, temporal.NewNonRetryableApplicationError("document contains no supported source units for this goal; exclusions are saved", "preparation-no-source-units", nil)
		}
		state.Result.Exclusions = exclusions
		if len(exclusions) > 0 {
			state.Result.Warnings = append(state.Result.Warnings, preparation.ExclusionWarning(len(exclusions)))
		}
		if p.Input.Kind == preparation.PreparationPreview {
			count := min(p.Input.Request.PreviewRecords, len(units))
			for i := range count {
				state.Units = append(state.Units, units[i*len(units)/count])
			}
		} else {
			state.Records = slices.Clone(p.Input.Request.ReviewedRecords)
			for i, r := range state.Records {
				r.Prompt = strings.TrimSpace(r.Prompt)
				r.Target = training.Normalized(r.Target)
				if r.SourceDocument != p.Source.ID || !r.UserApproved || !slices.ContainsFunc(units, func(u preparation.SourceUnit) bool { return u.Evidence == r.Evidence }) {
					return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("reviewed evidence must belong to this source-unit plan; obtain a fresh Preview for an older plan")
				}
				if err := generate.ValidateRecord(p.Input.Request.Goal, r); err != nil {
					return tmprl.StepResult[preparation.Artifacts]{}, err
				}
				r.FactID = training.FactID(r.Evidence, r.Target)
				r.SourceDocument = p.Source.ID
				state.Records[i] = r
			}
			// Accepted examples consume capacity, not all facts in their passage.
			state.Units = units
		}
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
	}
	if err := checkpoint.SafePoint(ctx, input); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	if input.Publish != nil {
		input.Publish(tmprl.Progress{Completed: 1, Total: 1, Message: "Source extraction complete"})
	}
	return tmprl.StepResult[preparation.Artifacts]{Artifacts: input.Artifacts}, nil
}

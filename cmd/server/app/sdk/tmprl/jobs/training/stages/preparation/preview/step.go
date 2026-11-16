package preview

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/verify"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
)

// Step produces reviewable examples, never a training-ready dataset.
type Step struct{}

// Name identifies the operation within the preparation stage.
func (Step) Name() string { return "preview" }

// Execute saves generation and verification progress in an isolated workspace.
// Foundation owns activity retries, heartbeats, and completed-step commits.
func (Step) Execute(ctx context.Context, input preparation.StepContext) (tmprl.StepResult[preparation.Artifacts], error) {
	if err := ctx.Err(); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	if input.Preparation == nil || input.Workspace == nil {
		return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: preview resources are required", preparation.ErrInvalidArgument)
	}
	resources := *input.Preparation
	if resources.Completer == nil {
		return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: preview requires a completer", preparation.ErrInvalidArgument)
	}
	if len(resources.Input.Request.ReviewedRecords) != 0 {
		return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: preview cannot contain accepted examples", preparation.ErrInvalidArgument)
	}
	resources.Input.Kind = preparation.PreparationPreview

	// Extract must already have produced this document. Never convert here.
	document, err := extract.Read(ctx, resources.Extractor, resources.Source, input.Workspace, providers.DocumentOptions{PageStart: resources.Input.Request.PageStart, PageEnd: resources.Input.Request.PageEnd})
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	if err := input.Workspace.MkdirAll("preview", 0o700); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	workspace, err := input.Workspace.OpenRoot("preview")
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	defer workspace.Close()

	previewInput := input
	previewInput.Preparation = &resources
	previewInput.Workspace = workspace
	if input.SafePoint != nil {
		previewInput.SafePoint = func(ctx context.Context, path string) error {
			return input.SafePoint(ctx, "preview/"+path)
		}
	}
	state, exists, err := checkpoint.Load(ctx, previewInput, true)
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	report := func(completed, total int, message string) {
		if input.Publish != nil {
			input.Publish(tmprl.Progress{Completed: completed, Total: total, Message: message})
		}
	}

	if !exists {
		units, exclusions, err := Units(document, resources.Input.Request.Goal, resources.Input.Request.PreviewRecords)
		if err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		state.Units = units
		state.Result.Exclusions = exclusions
		if err := ctx.Err(); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		if err := tmprl.WriteJSON(workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
	}

	if !state.Generated {
		report(state.Next, len(state.Units), "Generating preview examples")
		// Reuse the generation owner and the same durable batch-attempt protocol.
		// Preview kind prevents held-out generation; verification remains below.
		if _, err := (generate.Step{}).Execute(ctx, previewInput); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		state, _, err = checkpoint.Load(ctx, previewInput, false)
		if err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
	}

	if state.Verified == 0 && len(state.Records) > resources.Input.Request.PreviewRecords {
		// Sampling is a Preview display bound, not a full-dataset source exclusion.
		state.Records = sampleRecords(state.Units, state.Records, resources.Input.Request.PreviewRecords)
		if err := tmprl.WriteJSON(workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
	}

	if state.Verified < len(state.Records) {
		if err := checkpoint.SafePoint(ctx, previewInput); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		report(0, len(state.Records), "Checking preview examples")
		warnings, usage, err := verify.Run(ctx, resources.Completer, resources.Input.Request, "", state.Records)
		if err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		state.Verified = len(state.Records)
		state.Result.Warnings = append(state.Result.Warnings, warnings...)
		state.Result.PromptTokens += usage.PromptTokens
		state.Result.CompletionTokens += usage.CompletionTokens
		if len(state.Result.Exclusions) > 0 {
			state.Result.Warnings = append(state.Result.Warnings, fmt.Sprintf("%d source exclusions recorded; inspect source_exclusions", len(state.Result.Exclusions)))
		}
	}
	state.Result.Preview = state.Records
	state.Result.RecordCount = len(state.Records)
	state.Result.TrainingReady = false
	if err := ctx.Err(); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	if err := tmprl.WriteJSON(workspace, checkpoint.File, state); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	if err := checkpoint.SafePoint(ctx, previewInput); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}

	report(len(state.Records), len(state.Records), "Preview ready for review")
	artifacts := input.Artifacts
	artifacts.Preview = &state.Result
	return tmprl.StepResult[preparation.Artifacts]{Artifacts: artifacts}, nil
}

package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step verifies generated records and owns publication of preparation artifacts.
type Step struct{}

// Name identifies the verification step within the preparation stage.
func (Step) Name() string { return "verify" }

// Execute checkpoints verification batches and publishes only after all required
// checks succeed. Rejected full datasets are never exposed as training input.
func (Step) Execute(ctx context.Context, input preparation.StepContext) (tmprl.StepResult[preparation.Artifacts], error) {
	state, _, err := checkpoint.Load(ctx, input, false)
	if err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	p := input.Preparation
	if p.Completer == nil {
		return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("%w: preparation completer is required", preparation.ErrInvalidArgument)
	}
	if !state.Generated {
		return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("generate must complete before verify")
	}
	artifacts := input.Artifacts
	if data, err := input.Workspace.ReadFile("result.json"); err == nil {
		var result preparation.PreparationResult
		if err := json.Unmarshal(data, &result); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		if result.DatasetID != p.Source.ID {
			return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("preparation result identity mismatch")
		}
		artifacts.Preparation = &result
		return tmprl.StepResult[preparation.Artifacts]{Artifacts: artifacts}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	report := func() {
		if input.Publish != nil {
			input.Publish(tmprl.Progress{Completed: state.Verified, Total: len(state.Records), Message: "Verifying prepared records"})
		}
	}
	report()
	for state.Verified < len(state.Records) {
		if err := ctx.Err(); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		end := min(state.Verified+5, len(state.Records))
		// Reviewed examples and held-out questions retain independent batches.
		if state.Verified < len(p.Input.Request.ReviewedRecords) {
			end = min(end, len(p.Input.Request.ReviewedRecords))
		} else {
			for i := state.Verified; i < end; i++ {
				if state.Records[i].ValidationPrompt || state.Records[i].IndependentPrompt {
					end = max(state.Verified+1, i)
					break
				}
			}
		}
		warnings, usage, err := Run(ctx, p.Completer, p.Input.Request, state.Records[state.Verified:end])
		if err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		if len(warnings) > 0 && p.Input.Kind == preparation.PreparationFull {
			return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("records failed verification: %s", strings.Join(warnings, "; "))
		}
		state.Result.Warnings = append(state.Result.Warnings, warnings...)
		state.Result.PromptTokens += usage.PromptTokens
		state.Result.CompletionTokens += usage.CompletionTokens
		state.Verified = end
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		report()
	}
	result := state.Result
	name := "dataset"
	if p.Input.Kind == preparation.PreparationPreview {
		name = "preview"
		result.Preview = state.Records
	} else if err := training.TrainingReadiness(state.Records); err == nil {
		result.TrainingReady = true
	} else {
		result.Warnings = append(result.Warnings, err.Error())
	}
	if err := ctx.Err(); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	result.RecordCount = len(state.Records)
	result.RecordsPath = filepath.Join(input.Workspace.Name(), name+".jsonl")
	result.ReportPath = filepath.Join(input.Workspace.Name(), name+"-report.json")
	if err := training.WriteRecords(input.Workspace, name+".jsonl", state.Records); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	if err := tmprl.WriteJSON(input.Workspace, name+"-report.json", struct {
		Result preparation.PreparationResult `json:"result"`
		Goal   preparation.Goal              `json:"goal"`
		Model  string                        `json:"model"`
	}{result, p.Input.Request.Goal, p.Input.Request.Model}); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	if err := tmprl.WriteJSON(input.Workspace, "result.json", result); err != nil {
		return tmprl.StepResult[preparation.Artifacts]{}, err
	}
	artifacts.Preparation = &result
	return tmprl.StepResult[preparation.Artifacts]{Artifacts: artifacts}, nil
}

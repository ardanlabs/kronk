package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Step verifies generated records and owns publication of preparation artifacts.
type Step struct{}

// Name identifies the verification step within the preparation stage.
func (Step) Name() string { return "verify" }

// Execute checkpoints verification batches and publishes only after all required
// checks succeed. Rejected full datasets are never exposed as training input.
func (Step) Execute(ctx context.Context, input model.StepContext) (tmprl.StepResult[model.Artifacts], error) {
	state, _, err := checkpoint.Load(ctx, input, false)
	if err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	p := input.Preparation
	if p.Completer == nil {
		return tmprl.StepResult[model.Artifacts]{}, fmt.Errorf("%w: preparation completer is required", model.ErrInvalidArgument)
	}
	if !state.Generated {
		return tmprl.StepResult[model.Artifacts]{}, fmt.Errorf("generate must complete before verify")
	}
	artifacts := input.Request.Artifacts
	if data, err := input.Workspace.ReadFile("result.json"); err == nil {
		var result model.PreparationResult
		if err := json.Unmarshal(data, &result); err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
		if result.DatasetID != p.Source.ID {
			return tmprl.StepResult[model.Artifacts]{}, fmt.Errorf("preparation result identity mismatch")
		}
		artifacts.Preparation = &result
		return tmprl.StepResult[model.Artifacts]{Artifacts: artifacts}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	report := func() {
		if input.Publish != nil {
			input.Publish(tmprl.Progress{Completed: state.Verified, Total: len(state.Records), Message: "Verifying prepared records"})
		}
	}
	report()
	for state.Verified < len(state.Records) {
		if err := ctx.Err(); err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
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
			return tmprl.StepResult[model.Artifacts]{}, err
		}
		if len(warnings) > 0 && p.Input.Kind == model.PreparationFull {
			return tmprl.StepResult[model.Artifacts]{}, fmt.Errorf("records failed verification: %s", strings.Join(warnings, "; "))
		}
		state.Result.Warnings = append(state.Result.Warnings, warnings...)
		state.Result.PromptTokens += usage.PromptTokens
		state.Result.CompletionTokens += usage.CompletionTokens
		state.Verified = end
		if err := dataset.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[model.Artifacts]{}, err
		}
		report()
	}
	result := state.Result
	name := "dataset"
	if p.Input.Kind == model.PreparationPreview {
		name = "preview"
		result.Preview = state.Records
	} else if err := dataset.TrainingReadiness(state.Records); err == nil {
		result.TrainingReady = true
	} else {
		result.Warnings = append(result.Warnings, err.Error())
	}
	if err := ctx.Err(); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	result.RecordCount = len(state.Records)
	result.RecordsPath = filepath.Join(input.Workspace.Name(), name+".jsonl")
	result.ReportPath = filepath.Join(input.Workspace.Name(), name+"-report.json")
	if err := dataset.WriteRecords(input.Workspace, name+".jsonl", state.Records); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if err := dataset.WriteJSON(input.Workspace, name+"-report.json", struct {
		Result model.PreparationResult `json:"result"`
		Goal   model.Goal              `json:"goal"`
		Model  string                  `json:"model"`
	}{result, p.Input.Request.Goal, p.Input.Request.Model}); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	if err := dataset.WriteJSON(input.Workspace, "result.json", result); err != nil {
		return tmprl.StepResult[model.Artifacts]{}, err
	}
	artifacts.Preparation = &result
	return tmprl.StepResult[model.Artifacts]{Artifacts: artifacts}, nil
}

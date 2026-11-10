package verify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/checkpoint"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"go.temporal.io/sdk/temporal"
)

// Step verifies generated records and owns publication of preparation artifacts.
type Step struct{}

// Name identifies the verification step within the preparation stage.
func (Step) Name() string { return "verify" }

// Execute checkpoints verification batches and publishes only passing full
// records. Rejected approved examples still require review; Preview stays intact.
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
	if state.Verified < len(state.Records) {
		_, err := extract.Read(ctx, p.Extractor, p.Source, input.Workspace, providers.DocumentOptions{PageStart: p.Input.Request.PageStart, PageEnd: p.Input.Request.PageEnd})
		if err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
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
		if state.Verified < state.ReviewedCount {
			end = min(end, state.ReviewedCount)
		} else {
			for i := state.Verified; i < end; i++ {
				if state.Records[i].ValidationPrompt || state.Records[i].IndependentPrompt {
					end = max(state.Verified+1, i)
					break
				}
			}
		}
		outcomes, usage, err := run(ctx, p.Completer, p.Input.Request, "", state.Records[state.Verified:end])
		if err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		if err := ctx.Err(); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		// Check approvals before changing any record, warning or cursor in this batch.
		for i, outcome := range outcomes {
			r := state.Records[state.Verified+i]
			if outcome != "pass" && p.Input.Kind == preparation.PreparationFull && (r.UserApproved || state.Verified+i < state.ReviewedCount) {
				return tmprl.StepResult[preparation.Artifacts]{}, fmt.Errorf("approved example requires review: %q: %s", r.Prompt, outcome)
			}
		}
		kept := state.Records[:state.Verified]
		for i, outcome := range outcomes {
			r := state.Records[state.Verified+i]
			switch {
			case outcome == "pass":
				kept = append(kept, r)
			case p.Input.Kind == preparation.PreparationPreview:
				kept = append(kept, r)
				state.Result.Warnings = append(state.Result.Warnings, fmt.Sprintf("record %d: %s", i+1, outcome))
			default:
				reference := r.FactID
				for _, unit := range state.Units {
					if unit.Evidence == r.Evidence {
						reference = unit.Reference
						break
					}
				}
				reason := fmt.Sprintf("verification rejected example %q: %s", r.Prompt, outcome)
				state.Result.Exclusions = append(state.Result.Exclusions, preparation.SourceExclusion{Reference: reference, Reason: reason})
				state.Result.Warnings = append(state.Result.Warnings, reference+": "+reason)
			}
		}
		state.Result.PromptTokens += usage.PromptTokens
		state.Result.CompletionTokens += usage.CompletionTokens
		state.Verified = len(kept)
		state.Records = append(kept, state.Records[end:]...)
		if err := tmprl.WriteJSON(input.Workspace, checkpoint.File, state); err != nil {
			return tmprl.StepResult[preparation.Artifacts]{}, err
		}
		report()
	}
	if p.Input.Kind == preparation.PreparationFull && !slices.ContainsFunc(state.Records, func(r training.Record) bool { return !r.ValidationPrompt && !r.IndependentPrompt }) {
		return tmprl.StepResult[preparation.Artifacts]{}, temporal.NewNonRetryableApplicationError("no training examples passed verification; rejection reasons are saved in the preparation checkpoint", "preparation-no-verified-examples", nil)
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

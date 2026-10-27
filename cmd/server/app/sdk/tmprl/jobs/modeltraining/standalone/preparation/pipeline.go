package prepare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/imports"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation/verify"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Engine coordinates the preparation pipeline without calling Temporal.
// Job adapters supply its services, workspace, and progress callback.
type Engine struct {
	Extractor preparation.DocumentExtractor
	Completer preparation.ModelCompleter
}

type preparationCheckpoint struct {
	Fingerprint      string            `json:"fingerprint"`
	Next             int               `json:"next"`
	EvaluationNext   int               `json:"evaluation_next"`
	Records          []training.Record `json:"records"`
	PromptTokens     int               `json:"prompt_tokens"`
	CompletionTokens int               `json:"completion_tokens"`
}

// Build executes the extract, generate, and verify Step handlers in order.
// Handlers checkpoint their own progress and verification publishes the records
// and report. Existing v1 workspaces retain their original recovery semantics.
// A nil callback disables progress.
func (b *Engine) Build(ctx context.Context, input preparation.PreparationInput, source imports.PreparationSource, sourceRoot, workspace *os.Root, publish func(tmprl.Progress)) (preparation.PreparationResult, error) {
	// Existing standalone activities may already have verified v1 batches.
	// Keep their original recovery semantics instead of interpreting that state
	// as the new, deliberately unverified generation checkpoint.
	if _, err := workspace.Stat("checkpoint.json"); err == nil {
		return b.buildLegacy(ctx, input, source, sourceRoot, workspace, publish)
	} else if !errors.Is(err, os.ErrNotExist) {
		return preparation.PreparationResult{}, err
	}
	resources := preparation.PreparationContext{Input: input, Source: source, SourceRoot: sourceRoot, Extractor: b.Extractor, Completer: b.Completer}
	var artifacts preparation.Artifacts
	for _, step := range []preparation.StepHandler{extract.Step{}, generate.Step{}, verify.Step{}} {
		result, err := step.Execute(ctx, preparation.StepContext{
			Preparation: &resources, Workspace: workspace, Publish: publish,
			Artifacts: artifacts,
		})
		if err != nil {
			return preparation.PreparationResult{}, err
		}
		artifacts = result.Artifacts
	}
	if artifacts.Preparation == nil {
		return preparation.PreparationResult{}, preparation.InvalidOutput("verify")
	}
	return *artifacts.Preparation, nil
}

func (b *Engine) buildLegacy(ctx context.Context, input preparation.PreparationInput, source imports.PreparationSource, sourceRoot, workspace *os.Root, publish func(tmprl.Progress)) (preparation.PreparationResult, error) {
	if b.Completer == nil {
		return preparation.PreparationResult{}, fmt.Errorf("%w: preparation completer is required", preparation.ErrInvalidArgument)
	}
	if publish == nil {
		publish = func(tmprl.Progress) {}
	}
	publish(tmprl.Progress{Message: "Extracting source document", Total: 1})
	document, err := extract.Run(ctx, b.Extractor, source, sourceRoot, workspace)
	if err != nil {
		return preparation.PreparationResult{}, err
	}
	passages, excluded := extract.Passages(document.Markdown)
	available := len(passages)
	passages = slices.DeleteFunc(passages, func(p string) bool { return len(generate.Targets(p, input.Request.Goal)) == 0 })
	if len(passages) == 0 {
		return preparation.PreparationResult{}, fmt.Errorf("document contains no supported source passages for this goal")
	}
	result := preparation.PreparationResult{DatasetID: source.ID}
	if excluded+available-len(passages) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d source passages excluded: too long or no supported targets for this goal", excluded+available-len(passages)))
	}
	var records []training.Record
	switch input.Kind {
	case preparation.PreparationPreview:
		count := min(input.Request.PreviewRecords, len(passages))
		selected := make([]string, count)
		for i := range count {
			selected[i] = passages[i*len(passages)/count]
		}
		publish(tmprl.Progress{Message: "Generating preview", Total: count})
		generated, usage, err := generate.Run(ctx, b.Completer, input.Request, selected, nil)
		if err != nil {
			return preparation.PreparationResult{}, err
		}
		result.PromptTokens += usage.PromptTokens
		result.CompletionTokens += usage.CompletionTokens
		publish(tmprl.Progress{Message: "Verifying preview", Total: count})
		warnings, usage, err := verify.Run(ctx, b.Completer, input.Request, generated)
		if err != nil {
			return preparation.PreparationResult{}, err
		}
		result.PromptTokens += usage.PromptTokens
		result.CompletionTokens += usage.CompletionTokens
		result.Preview = generated
		result.Warnings = append(result.Warnings, warnings...)
		records = generated
	case preparation.PreparationFull:
		checkpointPath := "checkpoint.json"
		identity, err := json.Marshal(input)
		if err != nil {
			return preparation.PreparationResult{}, err
		}
		hash := sha256.Sum256(append(identity, []byte(document.Markdown)...))
		checkpoint := preparationCheckpoint{Fingerprint: "tuning-preparation-v1/" + hex.EncodeToString(hash[:])}
		if data, err := workspace.ReadFile(checkpointPath); err == nil {
			var saved preparationCheckpoint
			if err := json.Unmarshal(data, &saved); err != nil {
				return preparation.PreparationResult{}, err
			}
			if saved.Fingerprint != checkpoint.Fingerprint {
				return preparation.PreparationResult{}, fmt.Errorf("preparation checkpoint does not match source or configuration")
			}
			checkpoint = saved
		} else if !errors.Is(err, os.ErrNotExist) {
			return preparation.PreparationResult{}, err
		} else {
			checkpoint.Records = slices.Clone(input.Request.ReviewedRecords)
			if len(checkpoint.Records) > 0 {
				documentText := training.Normalized(document.Markdown)
				for i, r := range checkpoint.Records {
					r.Prompt = strings.TrimSpace(r.Prompt)
					r.Target = training.Normalized(r.Target)
					r.Evidence = training.Normalized(r.Evidence)
					if !strings.Contains(documentText, r.Evidence) {
						return preparation.PreparationResult{}, fmt.Errorf("reviewed evidence is not in the imported document")
					}
					if err := generate.ValidateRecord(input.Request.Goal, r); err != nil {
						return preparation.PreparationResult{}, err
					}
					r.FactID = training.FactID(r.Evidence, r.Target)
					r.SourceDocument = source.ID
					checkpoint.Records[i] = r
				}
				warnings, usage, err := verify.Run(ctx, b.Completer, input.Request, checkpoint.Records)
				if err != nil {
					return preparation.PreparationResult{}, err
				}
				if len(warnings) > 0 {
					return preparation.PreparationResult{}, fmt.Errorf("reviewed records failed verification: %s", strings.Join(warnings, "; "))
				}
				checkpoint.PromptTokens += usage.PromptTokens
				checkpoint.CompletionTokens += usage.CompletionTokens
			}
			if err := tmprl.WriteJSON(workspace, checkpointPath, checkpoint); err != nil {
				return preparation.PreparationResult{}, err
			}
		}
		// Reviewed examples already cover their quoted evidence; generate only
		// the remaining passages. Evaluation rows never influence coverage.
		passages = slices.DeleteFunc(passages, func(p string) bool {
			return slices.ContainsFunc(input.Request.ReviewedRecords, func(r training.Record) bool { return training.Normalized(r.Evidence) == p })
		})
		if checkpoint.Next < 0 || checkpoint.Next > len(passages) || checkpoint.EvaluationNext < 0 || checkpoint.EvaluationNext > 30 {
			return preparation.PreparationResult{}, fmt.Errorf("invalid checkpoint bounds")
		}
		if err := training.ValidateRecords(checkpoint.Records); err != nil {
			return preparation.PreparationResult{}, err
		}
		for checkpoint.Next < len(passages) {
			if err := ctx.Err(); err != nil {
				return preparation.PreparationResult{}, err
			}
			end := min(checkpoint.Next+5, len(passages))
			publish(tmprl.Progress{Message: "Generating and verifying dataset", Completed: checkpoint.Next, Total: len(passages) + 1})
			generated, usage, err := generate.Run(ctx, b.Completer, input.Request, passages[checkpoint.Next:end], checkpoint.Records)
			if err != nil {
				return preparation.PreparationResult{}, err
			}
			checkpoint.PromptTokens += usage.PromptTokens
			checkpoint.CompletionTokens += usage.CompletionTokens
			warnings, usage, err := verify.Run(ctx, b.Completer, input.Request, generated)
			if err != nil {
				return preparation.PreparationResult{}, err
			}
			if len(warnings) > 0 {
				return preparation.PreparationResult{}, fmt.Errorf("generated records failed verification: %s", strings.Join(warnings, "; "))
			}
			checkpoint.PromptTokens += usage.PromptTokens
			checkpoint.CompletionTokens += usage.CompletionTokens
			for i := range generated {
				generated[i].SourceDocument = source.ID
			}
			checkpoint.Records = append(checkpoint.Records, generated...)
			checkpoint.Next = end
			if err := tmprl.WriteJSON(workspace, checkpointPath, checkpoint); err != nil {
				return preparation.PreparationResult{}, err
			}
		}
		trainingRows := slices.DeleteFunc(slices.Clone(checkpoint.Records), func(r training.Record) bool { return r.ValidationPrompt || r.IndependentPrompt })
		if input.Request.Goal == preparation.GoalQuestionAnswer && len(trainingRows) >= 20 {
			for checkpoint.EvaluationNext < 30 {
				index := checkpoint.EvaluationNext
				fact := trainingRows[index%20]
				publish(tmprl.Progress{Message: "Creating independent evaluation questions", Completed: index, Total: 31})
				r, usage, err := generate.EvaluationRecord(ctx, b.Completer, input.Request, fact, checkpoint.Records)
				if err != nil {
					return preparation.PreparationResult{}, err
				}
				checkpoint.PromptTokens += usage.PromptTokens
				checkpoint.CompletionTokens += usage.CompletionTokens
				r.UserApproved = false
				r.ValidationPrompt = index < 10
				r.IndependentPrompt = index >= 10
				warnings, usage, err := verify.Run(ctx, b.Completer, input.Request, []training.Record{r})
				if err != nil {
					return preparation.PreparationResult{}, err
				}
				if len(warnings) > 0 {
					return preparation.PreparationResult{}, fmt.Errorf("evaluation question failed verification: %s", strings.Join(warnings, "; "))
				}
				checkpoint.PromptTokens += usage.PromptTokens
				checkpoint.CompletionTokens += usage.CompletionTokens
				checkpoint.Records = append(checkpoint.Records, r)
				checkpoint.EvaluationNext++
				if err := tmprl.WriteJSON(workspace, checkpointPath, checkpoint); err != nil {
					return preparation.PreparationResult{}, err
				}
			}
		}
		records = checkpoint.Records
		result.PromptTokens = checkpoint.PromptTokens
		result.CompletionTokens = checkpoint.CompletionTokens
		if err := training.TrainingReadiness(records); err == nil {
			result.TrainingReady = true
		} else {
			result.Warnings = append(result.Warnings, err.Error())
		}
	default:
		return preparation.PreparationResult{}, fmt.Errorf("invalid preparation kind")
	}
	for i := range records {
		records[i].SourceDocument = source.ID
	}
	if err := ctx.Err(); err != nil {
		return preparation.PreparationResult{}, err
	}
	if err := training.ValidateRecords(records); err != nil {
		return preparation.PreparationResult{}, err
	}
	name := "dataset"
	if input.Kind == preparation.PreparationPreview {
		name = "preview"
	}
	result.RecordsPath = filepath.Join(workspace.Name(), name+".jsonl")
	result.ReportPath = filepath.Join(workspace.Name(), name+"-report.json")
	result.RecordCount = len(records)
	if err := training.WriteRecords(workspace, name+".jsonl", records); err != nil {
		return preparation.PreparationResult{}, err
	}
	if err := tmprl.WriteJSON(workspace, name+"-report.json", struct {
		Result preparation.PreparationResult `json:"result"`
		Goal   preparation.Goal              `json:"goal"`
		Model  string                        `json:"model"`
	}{result, input.Request.Goal, input.Request.Model}); err != nil {
		return preparation.PreparationResult{}, err
	}
	publish(tmprl.Progress{Message: "Dataset preparation complete", Completed: len(records), Total: len(records)})
	return result, nil
}

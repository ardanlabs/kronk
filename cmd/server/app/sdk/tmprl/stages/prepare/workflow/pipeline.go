// Package prepare extracts documents and generates verified training records,
// with persisted checkpoints for retries.
package prepare

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/prepare/extract"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/prepare/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/prepare/verify"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
)

// Engine coordinates the preparation pipeline without calling Temporal.
// prepareDataset in activity.go supplies its services, workspace, and progress callback.
type Engine struct {
	Extractor model.DocumentExtractor
	Completer model.ModelCompleter
}

type preparationCheckpoint struct {
	Fingerprint      string         `json:"fingerprint"`
	Next             int            `json:"next"`
	EvaluationNext   int            `json:"evaluation_next"`
	Records          []model.Record `json:"records"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
}

// Build coordinates extract.Run, generate.Run, and verify.Run, then persists
// the resulting records and report. Preview samples passages to generate and verify;
// full preparation generates and verifies batches, checkpoints their progress,
// and generates and verifies evaluation questions. Retries resume checkpoints
// within the same Temporal activity. A nil callback disables progress.
func (b *Engine) Build(ctx context.Context, input model.PreparationInput, source model.PreparationSource, sourceRoot, workspace *os.Root, publish func(model.Progress)) (model.PreparationResult, error) {
	if b.Completer == nil {
		return model.PreparationResult{}, fmt.Errorf("%w: preparation completer is required", model.ErrInvalidArgument)
	}
	if publish == nil {
		publish = func(model.Progress) {}
	}
	publish(model.Progress{Message: "Extracting source document", Total: 1})
	document, err := extract.Run(ctx, b.Extractor, source, sourceRoot, workspace)
	if err != nil {
		return model.PreparationResult{}, err
	}
	passages, excluded := extract.Passages(document.Markdown)
	available := len(passages)
	passages = slices.DeleteFunc(passages, func(p string) bool { return len(generate.Targets(p, input.Request.Goal)) == 0 })
	if len(passages) == 0 {
		return model.PreparationResult{}, fmt.Errorf("document contains no supported source passages for this goal")
	}
	result := model.PreparationResult{DatasetID: source.ID}
	if excluded+available-len(passages) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d source passages excluded: too long or no supported targets for this goal", excluded+available-len(passages)))
	}
	var records []model.Record
	switch input.Kind {
	case model.PreparationPreview:
		count := min(input.Request.PreviewRecords, len(passages))
		selected := make([]string, count)
		for i := range count {
			selected[i] = passages[i*len(passages)/count]
		}
		publish(model.Progress{Message: "Generating preview", Total: count})
		generated, usage, err := generate.Run(ctx, b.Completer, input.Request, selected, nil)
		if err != nil {
			return model.PreparationResult{}, err
		}
		result.PromptTokens += usage.PromptTokens
		result.CompletionTokens += usage.CompletionTokens
		publish(model.Progress{Message: "Verifying preview", Total: count})
		warnings, usage, err := verify.Run(ctx, b.Completer, input.Request, generated)
		if err != nil {
			return model.PreparationResult{}, err
		}
		result.PromptTokens += usage.PromptTokens
		result.CompletionTokens += usage.CompletionTokens
		result.Preview = generated
		result.Warnings = append(result.Warnings, warnings...)
		records = generated
	case model.PreparationFull:
		checkpointPath := "checkpoint.json"
		identity, err := json.Marshal(input)
		if err != nil {
			return model.PreparationResult{}, err
		}
		hash := sha256.Sum256(append(identity, []byte(document.Markdown)...))
		checkpoint := preparationCheckpoint{Fingerprint: "tuning-preparation-v1/" + hex.EncodeToString(hash[:])}
		if data, err := workspace.ReadFile(checkpointPath); err == nil {
			var saved preparationCheckpoint
			if err := json.Unmarshal(data, &saved); err != nil {
				return model.PreparationResult{}, err
			}
			if saved.Fingerprint != checkpoint.Fingerprint {
				return model.PreparationResult{}, fmt.Errorf("preparation checkpoint does not match source or configuration")
			}
			checkpoint = saved
		} else if !errors.Is(err, os.ErrNotExist) {
			return model.PreparationResult{}, err
		} else {
			checkpoint.Records = slices.Clone(input.Request.ReviewedRecords)
			if len(checkpoint.Records) > 0 {
				documentText := dataset.Normalized(document.Markdown)
				for i, r := range checkpoint.Records {
					r.Prompt = strings.TrimSpace(r.Prompt)
					r.Target = dataset.Normalized(r.Target)
					r.Evidence = dataset.Normalized(r.Evidence)
					if !strings.Contains(documentText, r.Evidence) {
						return model.PreparationResult{}, fmt.Errorf("reviewed evidence is not in the imported document")
					}
					if err := generate.ValidateRecord(input.Request.Goal, r); err != nil {
						return model.PreparationResult{}, err
					}
					r.FactID = dataset.FactID(r.Evidence, r.Target)
					r.SourceDocument = source.ID
					checkpoint.Records[i] = r
				}
				warnings, usage, err := verify.Run(ctx, b.Completer, input.Request, checkpoint.Records)
				if err != nil {
					return model.PreparationResult{}, err
				}
				if len(warnings) > 0 {
					return model.PreparationResult{}, fmt.Errorf("reviewed records failed verification: %s", strings.Join(warnings, "; "))
				}
				checkpoint.PromptTokens += usage.PromptTokens
				checkpoint.CompletionTokens += usage.CompletionTokens
			}
			if err := dataset.WriteJSON(workspace, checkpointPath, checkpoint); err != nil {
				return model.PreparationResult{}, err
			}
		}
		// Reviewed examples already cover their quoted evidence; generate only
		// the remaining passages. Evaluation rows never influence coverage.
		passages = slices.DeleteFunc(passages, func(p string) bool {
			return slices.ContainsFunc(input.Request.ReviewedRecords, func(r model.Record) bool { return dataset.Normalized(r.Evidence) == p })
		})
		if checkpoint.Next < 0 || checkpoint.Next > len(passages) || checkpoint.EvaluationNext < 0 || checkpoint.EvaluationNext > 30 {
			return model.PreparationResult{}, fmt.Errorf("invalid checkpoint bounds")
		}
		if err := dataset.ValidateRecords(checkpoint.Records); err != nil {
			return model.PreparationResult{}, err
		}
		for checkpoint.Next < len(passages) {
			if err := ctx.Err(); err != nil {
				return model.PreparationResult{}, err
			}
			end := min(checkpoint.Next+5, len(passages))
			publish(model.Progress{Message: "Generating and verifying dataset", Completed: checkpoint.Next, Total: len(passages) + 1})
			generated, usage, err := generate.Run(ctx, b.Completer, input.Request, passages[checkpoint.Next:end], checkpoint.Records)
			if err != nil {
				return model.PreparationResult{}, err
			}
			checkpoint.PromptTokens += usage.PromptTokens
			checkpoint.CompletionTokens += usage.CompletionTokens
			warnings, usage, err := verify.Run(ctx, b.Completer, input.Request, generated)
			if err != nil {
				return model.PreparationResult{}, err
			}
			if len(warnings) > 0 {
				return model.PreparationResult{}, fmt.Errorf("generated records failed verification: %s", strings.Join(warnings, "; "))
			}
			checkpoint.PromptTokens += usage.PromptTokens
			checkpoint.CompletionTokens += usage.CompletionTokens
			for i := range generated {
				generated[i].SourceDocument = source.ID
			}
			checkpoint.Records = append(checkpoint.Records, generated...)
			checkpoint.Next = end
			if err := dataset.WriteJSON(workspace, checkpointPath, checkpoint); err != nil {
				return model.PreparationResult{}, err
			}
		}
		training := slices.DeleteFunc(slices.Clone(checkpoint.Records), func(r model.Record) bool { return r.ValidationPrompt || r.IndependentPrompt })
		if input.Request.Goal == model.GoalQuestionAnswer && len(training) >= 20 {
			for checkpoint.EvaluationNext < 30 {
				index := checkpoint.EvaluationNext
				fact := training[index%20]
				publish(model.Progress{Message: "Creating independent evaluation questions", Completed: index, Total: 31})
				r, usage, err := generate.EvaluationRecord(ctx, b.Completer, input.Request, fact, checkpoint.Records)
				if err != nil {
					return model.PreparationResult{}, err
				}
				checkpoint.PromptTokens += usage.PromptTokens
				checkpoint.CompletionTokens += usage.CompletionTokens
				r.UserApproved = false
				r.ValidationPrompt = index < 10
				r.IndependentPrompt = index >= 10
				warnings, usage, err := verify.Run(ctx, b.Completer, input.Request, []model.Record{r})
				if err != nil {
					return model.PreparationResult{}, err
				}
				if len(warnings) > 0 {
					return model.PreparationResult{}, fmt.Errorf("evaluation question failed verification: %s", strings.Join(warnings, "; "))
				}
				checkpoint.PromptTokens += usage.PromptTokens
				checkpoint.CompletionTokens += usage.CompletionTokens
				checkpoint.Records = append(checkpoint.Records, r)
				checkpoint.EvaluationNext++
				if err := dataset.WriteJSON(workspace, checkpointPath, checkpoint); err != nil {
					return model.PreparationResult{}, err
				}
			}
		}
		records = checkpoint.Records
		result.PromptTokens = checkpoint.PromptTokens
		result.CompletionTokens = checkpoint.CompletionTokens
		if err := dataset.TrainingReadiness(records); err == nil {
			result.TrainingReady = true
		} else {
			result.Warnings = append(result.Warnings, err.Error())
		}
	default:
		return model.PreparationResult{}, fmt.Errorf("invalid preparation kind")
	}
	for i := range records {
		records[i].SourceDocument = source.ID
	}
	if err := ctx.Err(); err != nil {
		return model.PreparationResult{}, err
	}
	if err := dataset.ValidateRecords(records); err != nil {
		return model.PreparationResult{}, err
	}
	name := "dataset"
	if input.Kind == model.PreparationPreview {
		name = "preview"
	}
	result.RecordsPath = filepath.Join(workspace.Name(), name+".jsonl")
	result.ReportPath = filepath.Join(workspace.Name(), name+"-report.json")
	result.RecordCount = len(records)
	if err := writeRecords(workspace, name+".jsonl", records); err != nil {
		return model.PreparationResult{}, err
	}
	if err := dataset.WriteJSON(workspace, name+"-report.json", struct {
		Result model.PreparationResult `json:"result"`
		Goal   model.Goal              `json:"goal"`
		Model  string                  `json:"model"`
	}{result, input.Request.Goal, input.Request.Model}); err != nil {
		return model.PreparationResult{}, err
	}
	publish(model.Progress{Message: "Dataset preparation complete", Completed: len(records), Total: len(records)})
	return result, nil
}

func writeRecords(root *os.Root, path string, records []model.Record) error {
	temporary := ".records-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	encoder := json.NewEncoder(file)
	for _, r := range records {
		if err := encoder.Encode(r); err != nil {
			file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return root.Rename(temporary, path)
}

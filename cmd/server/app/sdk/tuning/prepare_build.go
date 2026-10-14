package tuning

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
)

// preparationEngine performs computation through injected services without
// calling Temporal. The activity supplies its progress callback.
type preparationEngine struct {
	Extractor DocumentExtractor
	Completer ModelCompleter
}

type preparationCheckpoint struct {
	Fingerprint      string   `json:"fingerprint"`
	Next             int      `json:"next"`
	EvaluationNext   int      `json:"evaluation_next"`
	Records          []Record `json:"records"`
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
}

// Build extracts an imported source, prepares preview or full records, and
// persists artifacts and retry checkpoints. A nil callback disables progress.
func (b *preparationEngine) Build(ctx context.Context, input preparationInput, dataset PreparationSource, sourceRoot, workspace *os.Root, publish func(Progress)) (PreparationResult, error) {
	if b.Completer == nil {
		return PreparationResult{}, fmt.Errorf("%w: preparation completer is required", ErrInvalidArgument)
	}
	if publish == nil {
		publish = func(Progress) {}
	}
	publish(Progress{Message: "Extracting source document", Total: 1})
	document, err := b.extractDocument(ctx, dataset, sourceRoot, workspace)
	if err != nil {
		return PreparationResult{}, err
	}
	passages, excluded := sourcePassages(document.Markdown)
	available := len(passages)
	passages = slices.DeleteFunc(passages, func(p string) bool { return len(targetCandidates(p, input.Request.Goal)) == 0 })
	if len(passages) == 0 {
		return PreparationResult{}, fmt.Errorf("document contains no supported source passages for this goal")
	}
	result := PreparationResult{DatasetID: dataset.ID}
	if excluded+available-len(passages) > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d source passages excluded: too long or no supported targets for this goal", excluded+available-len(passages)))
	}
	var records []Record
	switch input.Kind {
	case PreparationPreview:
		count := min(input.Request.PreviewRecords, len(passages))
		selected := make([]string, count)
		for i := range count {
			selected[i] = passages[i*len(passages)/count]
		}
		publish(Progress{Message: "Generating preview", Total: count})
		generated, usage, err := b.generateRecords(ctx, input.Request, selected, nil)
		if err != nil {
			return PreparationResult{}, err
		}
		result.PromptTokens += usage.PromptTokens
		result.CompletionTokens += usage.CompletionTokens
		publish(Progress{Message: "Verifying preview", Total: count})
		warnings, usage, err := b.verifyRecords(ctx, input.Request, generated)
		if err != nil {
			return PreparationResult{}, err
		}
		result.PromptTokens += usage.PromptTokens
		result.CompletionTokens += usage.CompletionTokens
		result.Preview = generated
		result.Warnings = append(result.Warnings, warnings...)
		records = generated
	case PreparationFull:
		checkpointPath := "checkpoint.json"
		identity, err := json.Marshal(input)
		if err != nil {
			return PreparationResult{}, err
		}
		hash := sha256.Sum256(append(identity, []byte(document.Markdown)...))
		checkpoint := preparationCheckpoint{Fingerprint: "tuning-preparation-v1/" + hex.EncodeToString(hash[:])}
		if data, err := workspace.ReadFile(checkpointPath); err == nil {
			var saved preparationCheckpoint
			if err := json.Unmarshal(data, &saved); err != nil {
				return PreparationResult{}, err
			}
			if saved.Fingerprint != checkpoint.Fingerprint {
				return PreparationResult{}, fmt.Errorf("preparation checkpoint does not match source or configuration")
			}
			checkpoint = saved
		} else if !errors.Is(err, os.ErrNotExist) {
			return PreparationResult{}, err
		} else {
			checkpoint.Records = slices.Clone(input.Request.ReviewedRecords)
			if len(checkpoint.Records) > 0 {
				documentText := normalized(document.Markdown)
				for i, r := range checkpoint.Records {
					r.Prompt = strings.TrimSpace(r.Prompt)
					r.Target = normalized(r.Target)
					r.Evidence = normalized(r.Evidence)
					if !strings.Contains(documentText, r.Evidence) {
						return PreparationResult{}, fmt.Errorf("reviewed evidence is not in the imported document")
					}
					if err := recordShape(input.Request.Goal, r); err != nil {
						return PreparationResult{}, err
					}
					r.FactID = factID(r.Evidence, r.Target)
					r.SourceDocument = dataset.ID
					checkpoint.Records[i] = r
				}
				warnings, usage, err := b.verifyRecords(ctx, input.Request, checkpoint.Records)
				if err != nil {
					return PreparationResult{}, err
				}
				if len(warnings) > 0 {
					return PreparationResult{}, fmt.Errorf("reviewed records failed verification: %s", strings.Join(warnings, "; "))
				}
				checkpoint.PromptTokens += usage.PromptTokens
				checkpoint.CompletionTokens += usage.CompletionTokens
			}
			if err := writeJSON(workspace, checkpointPath, checkpoint); err != nil {
				return PreparationResult{}, err
			}
		}
		// Reviewed examples already cover their quoted evidence; generate only
		// the remaining passages. Evaluation rows never influence coverage.
		passages = slices.DeleteFunc(passages, func(p string) bool {
			return slices.ContainsFunc(input.Request.ReviewedRecords, func(r Record) bool { return normalized(r.Evidence) == p })
		})
		if checkpoint.Next < 0 || checkpoint.Next > len(passages) || checkpoint.EvaluationNext < 0 || checkpoint.EvaluationNext > 30 {
			return PreparationResult{}, fmt.Errorf("invalid checkpoint bounds")
		}
		if err := validateRecords(checkpoint.Records); err != nil {
			return PreparationResult{}, err
		}
		for checkpoint.Next < len(passages) {
			if err := ctx.Err(); err != nil {
				return PreparationResult{}, err
			}
			end := min(checkpoint.Next+5, len(passages))
			publish(Progress{Message: "Generating and verifying dataset", Completed: checkpoint.Next, Total: len(passages) + 1})
			generated, usage, err := b.generateRecords(ctx, input.Request, passages[checkpoint.Next:end], checkpoint.Records)
			if err != nil {
				return PreparationResult{}, err
			}
			checkpoint.PromptTokens += usage.PromptTokens
			checkpoint.CompletionTokens += usage.CompletionTokens
			warnings, usage, err := b.verifyRecords(ctx, input.Request, generated)
			if err != nil {
				return PreparationResult{}, err
			}
			if len(warnings) > 0 {
				return PreparationResult{}, fmt.Errorf("generated records failed verification: %s", strings.Join(warnings, "; "))
			}
			checkpoint.PromptTokens += usage.PromptTokens
			checkpoint.CompletionTokens += usage.CompletionTokens
			for i := range generated {
				generated[i].SourceDocument = dataset.ID
			}
			checkpoint.Records = append(checkpoint.Records, generated...)
			checkpoint.Next = end
			if err := writeJSON(workspace, checkpointPath, checkpoint); err != nil {
				return PreparationResult{}, err
			}
		}
		training := slices.DeleteFunc(slices.Clone(checkpoint.Records), func(r Record) bool { return r.ValidationPrompt || r.IndependentPrompt })
		if input.Request.Goal == GoalQuestionAnswer && len(training) >= 20 {
			for checkpoint.EvaluationNext < 30 {
				index := checkpoint.EvaluationNext
				fact := training[index%20]
				publish(Progress{Message: "Creating independent evaluation questions", Completed: index, Total: 31})
				r, usage, err := b.evaluationRecord(ctx, input.Request, fact, checkpoint.Records)
				if err != nil {
					return PreparationResult{}, err
				}
				checkpoint.PromptTokens += usage.PromptTokens
				checkpoint.CompletionTokens += usage.CompletionTokens
				r.UserApproved = false
				r.ValidationPrompt = index < 10
				r.IndependentPrompt = index >= 10
				warnings, usage, err := b.verifyRecords(ctx, input.Request, []Record{r})
				if err != nil {
					return PreparationResult{}, err
				}
				if len(warnings) > 0 {
					return PreparationResult{}, fmt.Errorf("evaluation question failed verification: %s", strings.Join(warnings, "; "))
				}
				checkpoint.PromptTokens += usage.PromptTokens
				checkpoint.CompletionTokens += usage.CompletionTokens
				checkpoint.Records = append(checkpoint.Records, r)
				checkpoint.EvaluationNext++
				if err := writeJSON(workspace, checkpointPath, checkpoint); err != nil {
					return PreparationResult{}, err
				}
			}
		}
		records = checkpoint.Records
		result.PromptTokens = checkpoint.PromptTokens
		result.CompletionTokens = checkpoint.CompletionTokens
		if err := TrainingReadiness(records); err == nil {
			result.TrainingReady = true
		} else {
			result.Warnings = append(result.Warnings, err.Error())
		}
	default:
		return PreparationResult{}, fmt.Errorf("invalid preparation kind")
	}
	for i := range records {
		records[i].SourceDocument = dataset.ID
	}
	if err := ctx.Err(); err != nil {
		return PreparationResult{}, err
	}
	if err := validateRecords(records); err != nil {
		return PreparationResult{}, err
	}
	name := "dataset"
	if input.Kind == PreparationPreview {
		name = "preview"
	}
	result.RecordsPath = filepath.Join(workspace.Name(), name+".jsonl")
	result.ReportPath = filepath.Join(workspace.Name(), name+"-report.json")
	result.RecordCount = len(records)
	if err := writeRecords(workspace, name+".jsonl", records); err != nil {
		return PreparationResult{}, err
	}
	if err := writeJSON(workspace, name+"-report.json", struct {
		Result PreparationResult `json:"result"`
		Goal   Goal              `json:"goal"`
		Model  string            `json:"model"`
	}{result, input.Request.Goal, input.Request.Model}); err != nil {
		return PreparationResult{}, err
	}
	publish(Progress{Message: "Dataset preparation complete", Completed: len(records), Total: len(records)})
	return result, nil
}

func (b *preparationEngine) evaluationRecord(ctx context.Context, request PreparationRequest, fact Record, previous []Record) (Record, Completion, error) {
	var excluded strings.Builder
	for _, r := range previous {
		if r.FactID == fact.FactID {
			fmt.Fprintf(&excluded, "\nDo not repeat: %s", r.Prompt)
		}
	}
	prompt := fmt.Sprintf("Write a new standalone natural question ending in ? for independent evaluation. It must ask for exactly the locked target, without revealing it. Return {\"record_1\":\"question\"}. Evidence: %s\nLocked target: %s\nGuidance: %s%s", fact.Evidence, fact.Target, request.Instructions, excluded.String())
	usage, err := b.Completer.Complete(ctx, CompletionRequest{Model: request.Model, Messages: []Message{{Role: "system", Content: preparationSystemPrompt}, {Role: "user", Content: prompt}}, ResponseSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"record_1"}, "properties": map[string]any{"record_1": map[string]any{"type": "string"}}}, MaxTokens: 256, Seed: 3407})
	if err != nil {
		return Record{}, usage, err
	}
	var response struct {
		Prompt string `json:"record_1"`
	}
	if err := decodeModelJSON(usage.Content, &response); err != nil {
		return Record{}, usage, err
	}
	fact.Prompt = strings.TrimSpace(response.Prompt)
	if err := recordShape(request.Goal, fact); err != nil {
		return Record{}, usage, err
	}
	if err := validateRecords(append(slices.Clone(previous), fact)); err != nil {
		return Record{}, usage, err
	}
	return fact, usage, nil
}

func writeRecords(root *os.Root, path string, records []Record) error {
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

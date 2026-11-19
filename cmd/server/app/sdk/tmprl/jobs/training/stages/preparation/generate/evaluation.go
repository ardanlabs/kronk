package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/modeljson"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
)

// duplicateQuestion carries correction feedback without exposing source content
// in activity errors or logs.
type duplicateQuestion struct{ prompt string }

func (*duplicateQuestion) Error() string { return "generated record repeats an existing prompt" }

// EvaluationRecord rephrases an existing question for a locked source fact.
// It requests one candidate, using the last rejected question on retries.
// Temporal still owns all retries.
func EvaluationRecord(ctx context.Context, completer preparation.ModelCompleter, request preparation.PreparationRequest, fact training.Record, previous []training.Record, rejections uint32, rejectedPrompt string) (training.Record, providers.Completion, error) {
	var excluded strings.Builder
	for _, r := range previous {
		if r.FactID == fact.FactID {
			fmt.Fprintf(&excluded, "\nDo not repeat: %s", r.Prompt)
		}
	}
	prompt := fmt.Sprintf("Write a new standalone natural question ending in ? for independent evaluation. It must ask for exactly the locked target, without revealing it. Preserve the original question's requested information, entity, quantity, negation and conditions; a percentage question must still request a percentage, not just name its category. Return {\"record_1\":\"question\"}. Original question: %s\nEvidence: %s\nLocked target: %s\nGuidance: %s%s", fact.Prompt, fact.Evidence, fact.Target, request.Instructions, excluded.String())
	question := fact.Prompt
	if rejections > 0 {
		question = rejectedPrompt
	}
	example, err := json.Marshal(map[string]string{"record_1": question})
	if err != nil {
		return training.Record{}, providers.Completion{}, err
	}
	messages := []providers.Message{
		{Role: "system", Content: preparationSystemPrompt},
		{Role: "user", Content: prompt},
		{Role: "assistant", Content: string(example)},
		{Role: "user", Content: "That question is already in the dataset. Produce a genuinely different wording. Recast the grammatical construction rather than copying it. Use only the original evidence and locked target; the answer must not change. Do not include the answer in the question. Return only the replacement question in the required JSON schema."},
	}
	usage, err := completer.Complete(ctx, providers.CompletionRequest{Model: request.Model, Messages: messages, ResponseSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"record_1"}, "properties": map[string]any{"record_1": map[string]any{"type": "string"}}}, MaxTokens: 256, Seed: 3407 + rejections})
	if err != nil {
		return training.Record{}, usage, err
	}
	var response struct {
		Prompt string `json:"record_1"`
	}
	if err := modeljson.Decode(usage.Content, &response); err != nil {
		return training.Record{}, usage, err
	}
	fact.Prompt = strings.TrimSpace(response.Prompt)
	if err := ValidateRecord(request.Goal, fact); err != nil {
		return training.Record{}, usage, err
	}
	if slices.ContainsFunc(previous, func(r training.Record) bool { return training.Key(r.Prompt) == training.Key(fact.Prompt) }) {
		return training.Record{}, usage, &duplicateQuestion{prompt: fact.Prompt}
	}
	if err := training.ValidateRecords(append(slices.Clone(previous), fact)); err != nil {
		return training.Record{}, usage, err
	}
	return fact, usage, nil
}

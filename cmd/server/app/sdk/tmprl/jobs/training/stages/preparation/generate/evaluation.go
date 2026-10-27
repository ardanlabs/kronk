package generate

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation/internal/modeljson"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
)

// EvaluationRecord generates a distinct question for a locked source fact.
func EvaluationRecord(ctx context.Context, completer preparation.ModelCompleter, request preparation.PreparationRequest, fact training.Record, previous []training.Record) (training.Record, providers.Completion, error) {
	var excluded strings.Builder
	for _, r := range previous {
		if r.FactID == fact.FactID {
			fmt.Fprintf(&excluded, "\nDo not repeat: %s", r.Prompt)
		}
	}
	prompt := fmt.Sprintf("Write a new standalone natural question ending in ? for independent evaluation. It must ask for exactly the locked target, without revealing it. Return {\"record_1\":\"question\"}. Evidence: %s\nLocked target: %s\nGuidance: %s%s", fact.Evidence, fact.Target, request.Instructions, excluded.String())
	usage, err := completer.Complete(ctx, providers.CompletionRequest{Model: request.Model, Messages: []providers.Message{{Role: "system", Content: preparationSystemPrompt}, {Role: "user", Content: prompt}}, ResponseSchema: map[string]any{"type": "object", "additionalProperties": false, "required": []string{"record_1"}, "properties": map[string]any{"record_1": map[string]any{"type": "string"}}}, MaxTokens: 256, Seed: 3407})
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
	if err := training.ValidateRecords(append(slices.Clone(previous), fact)); err != nil {
		return training.Record{}, usage, err
	}
	return fact, usage, nil
}

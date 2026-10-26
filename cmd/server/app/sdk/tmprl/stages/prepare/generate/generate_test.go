package generate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/prepare/generate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
)

type completer struct {
	content string
	err     error
}

func (c completer) Complete(context.Context, model.CompletionRequest) (model.Completion, error) {
	return model.Completion{Content: c.content, PromptTokens: 12, CompletionTokens: 7}, c.err
}

func TestRun(t *testing.T) {
	request := model.PreparationRequest{Goal: model.GoalQuestionAnswer, Model: "controlled"}
	passages := []string{"The harbor opens at dawn.", "The ferry leaves at noon."}
	valid := `{"records":[{"prompt":" When does the ferry leave? ","evidence_id":2,"target_id":1},{"prompt":"When does the harbor open?","evidence_id":1,"target_id":1}]}`
	records, usage, err := generate.Run(t.Context(), completer{content: valid}, request, passages, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Prompt != "When does the ferry leave?" || records[0].Target != passages[1] || records[0].Evidence != passages[1] || records[0].FactID == "" || records[1].Target != passages[0] || records[0].FactID == records[1].FactID {
		t.Fatalf("incorrect grounded records: %+v", records)
	}
	if usage.PromptTokens != 12 || usage.CompletionTokens != 7 {
		t.Fatalf("lost usage: %+v", usage)
	}
	for name, response := range map[string]string{
		"malformed":         `{`,
		"trailing":          valid + `{}`,
		"wrong count":       `{"records":[]}`,
		"repeated evidence": `{"records":[{"prompt":"First question?","evidence_id":1,"target_id":1},{"prompt":"Second question?","evidence_id":1,"target_id":1}]}`,
		"unknown field":     `{"records":[],"extra":true}`,
		"bad evidence":      `{"records":[{"prompt":"First question?","evidence_id":0,"target_id":1},{"prompt":"Second question?","evidence_id":2,"target_id":1}]}`,
		"bad target":        `{"records":[{"prompt":"First question?","evidence_id":1,"target_id":2},{"prompt":"Second question?","evidence_id":2,"target_id":1}]}`,
		"not question":      `{"records":[{"prompt":"Describe harbor","evidence_id":1,"target_id":1},{"prompt":"Second question?","evidence_id":2,"target_id":1}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, _, err := generate.Run(t.Context(), completer{content: response}, request, passages, nil); err == nil || len(got) != 0 {
				t.Fatalf("accepted invalid response: %+v, %v", got, err)
			}
		})
	}
	if _, _, err := generate.Run(t.Context(), completer{content: valid}, request, passages, records); err == nil {
		t.Fatal("accepted repeated previous prompts")
	}
	want := errors.New("provider unavailable")
	if _, usage, err := generate.Run(t.Context(), completer{err: want}, request, passages, nil); !errors.Is(err, want) || usage.PromptTokens != 12 {
		t.Fatalf("provider failure/usage: %+v, %v", usage, err)
	}
}

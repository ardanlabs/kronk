package fineapp

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestModelCompleter(t *testing.T) {
	var gotModel string
	var gotRequest model.D
	completer := modelCompleter{
		chat: func(_ context.Context, modelID string, request model.D) (model.ChatResponse, error) {
			gotModel = modelID
			gotRequest = request
			return model.ChatResponse{
				Choices: []model.Choice{{Message: &model.ResponseMessage{Content: ` {"records":[]} `}}},
				Usage:   &model.Usage{PromptTokens: 17, CompletionTokens: 9},
			}, nil
		},
	}

	completion, err := completer.Complete(t.Context(), prepare.CompletionRequest{
		Model: "owner/preparation-model",
		Messages: []prepare.Message{
			{Role: "system", Content: "Return records."},
			{Role: "user", Content: "Source text"},
		},
		ResponseSchema: map[string]any{"type": "object"},
		MaxTokens:      321,
		Seed:           42,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotModel != "owner/preparation-model" {
		t.Fatalf("model: got %q", gotModel)
	}
	if completion.Content != `{"records":[]}` || completion.PromptTokens != 17 || completion.CompletionTokens != 9 {
		t.Fatalf("completion: got %+v", completion)
	}

	wantMessages := []model.D{
		{"role": "system", "content": "Return records."},
		{"role": "user", "content": "Source text"},
	}
	if !reflect.DeepEqual(gotRequest["messages"], wantMessages) {
		t.Fatalf("messages: got %#v", gotRequest["messages"])
	}
	if gotRequest["max_completion_tokens"] != 321 || gotRequest["seed"] != uint32(42) || gotRequest["enable_thinking"] != false {
		t.Fatalf("controlled request: got %#v", gotRequest)
	}
	responseFormat, ok := gotRequest["response_format"].(model.D)
	if !ok || responseFormat["type"] != "json_schema" {
		t.Fatalf("response format: got %#v", gotRequest["response_format"])
	}
}

func TestModelCompleterRejectsMissingContent(t *testing.T) {
	tests := []struct {
		name     string
		response model.ChatResponse
		want     string
	}{
		{name: "no choices", want: "no choices"},
		{name: "nil message", response: model.ChatResponse{Choices: []model.Choice{{}}}, want: "no choices"},
		{name: "empty content", response: model.ChatResponse{Choices: []model.Choice{{Message: &model.ResponseMessage{}}}}, want: "empty content"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			completer := modelCompleter{chat: func(context.Context, string, model.D) (model.ChatResponse, error) {
				return test.response, nil
			}}
			_, err := completer.Complete(t.Context(), prepare.CompletionRequest{Model: "model"})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error: got %v, want containing %q", err, test.want)
			}
		})
	}
}

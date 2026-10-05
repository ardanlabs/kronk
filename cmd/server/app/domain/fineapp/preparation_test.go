package fineapp

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/tools/models"
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

func TestModelCompleterRejectsLengthTerminatedContentWithoutRetry(t *testing.T) {
	finishReason := model.FinishReasonLength
	var calls int
	completer := modelCompleter{chat: func(context.Context, string, model.D) (model.ChatResponse, error) {
		calls++
		return model.ChatResponse{
			Choices: []model.Choice{{
				Message:         &model.ResponseMessage{Content: `{"record_1":"pass"`},
				FinishReasonPtr: &finishReason,
			}},
		}, nil
	}}

	_, err := completer.Complete(t.Context(), prepare.CompletionRequest{Model: "model", MaxTokens: 464})
	if !errors.Is(err, prepare.ErrCompletionTruncated) {
		t.Fatalf("error = %v, want ErrCompletionTruncated", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want one call because an unchanged retry cannot repair a token limit", calls)
	}
}

func TestModelCompleterRetriesTransientFailures(t *testing.T) {
	var calls int
	completer := modelCompleter{chat: func(context.Context, string, model.D) (model.ChatResponse, error) {
		calls++
		if calls < 3 {
			return model.ChatResponse{}, errors.New("temporary model transport failure")
		}
		return model.ChatResponse{Choices: []model.Choice{{Message: &model.ResponseMessage{Content: `{"records":[]}`}}}}, nil
	}}

	completion, err := completer.Complete(t.Context(), prepare.CompletionRequest{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || completion.Content != `{"records":[]}` {
		t.Fatalf("calls = %d, completion = %+v; want success on third attempt", calls, completion)
	}
}

func TestModelCompleterDoesNotRetryPermanentOrCanceledFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "invalid request", err: model.ErrInvalidRequest},
		{name: "insufficient capacity", err: pool.ErrNoCapacity},
		{name: "model not found", err: models.ErrModelNotFound},
		{name: "canceled", err: context.Canceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls int
			completer := modelCompleter{chat: func(context.Context, string, model.D) (model.ChatResponse, error) {
				calls++
				return model.ChatResponse{}, test.err
			}}
			_, err := completer.Complete(t.Context(), prepare.CompletionRequest{Model: "model"})
			if !errors.Is(err, test.err) || calls != 1 || !strings.Contains(err.Error(), "after 1 attempts") {
				t.Fatalf("error = %v, calls = %d; want one non-retried failure", err, calls)
			}
		})
	}
}

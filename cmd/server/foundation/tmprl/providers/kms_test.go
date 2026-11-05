package providers

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestNewKMSCompleterNilPool(t *testing.T) {
	c, err := NewKMSCompleter(nil)
	if c != nil || !errors.Is(err, tmprl.ErrInvalidArgument) {
		t.Fatalf("NewKMSCompleter(nil): got %v, %v, want nil, ErrInvalidArgument", c, err)
	}
}

func TestKMSCompleterComplete(t *testing.T) {
	request := CompletionRequest{
		Model: "owner/structured-model",
		Messages: []Message{
			{Role: "system", Content: "Return structured records."},
			{Role: "user", Content: "A distinct source passage."},
		},
		ResponseSchema: map[string]any{"type": "object", "required": []string{"records"}, "properties": map[string]any{"records": map[string]any{"type": "array"}}},
		MaxTokens:      739,
		Seed:           53,
	}
	wantRequest := model.D{
		"messages": []model.D{
			{"role": "system", "content": "Return structured records."},
			{"role": "user", "content": "A distinct source passage."},
		},
		"max_completion_tokens": 739,
		"temperature":           0.2,
		"top_p":                 0.9,
		"seed":                  uint32(53),
		"enable_thinking":       false,
		"response_format": model.D{"type": "json_schema", "json_schema": model.D{
			"name": "structured_output", "strict": true, "schema": request.ResponseSchema,
		}},
	}
	ctx := t.Context()
	calls := 0
	c := KMSCompleter{chat: func(gotCtx context.Context, modelID string, input model.D) (model.ChatResponse, int, error) {
		calls++
		if gotCtx != ctx || modelID != request.Model || !reflect.DeepEqual(input, wantRequest) {
			t.Errorf("chat: got context %v, model %q, request %#v, want original context, model %q, request %#v", gotCtx, modelID, input, request.Model, wantRequest)
		}
		finish := model.FinishReasonStop
		return model.ChatResponse{
			Choices: []model.Choice{{Message: &model.ResponseMessage{Content: " \n{\"records\":[1]}\t "}, FinishReasonPtr: &finish}},
			Usage:   &model.Usage{PromptTokens: 127, CompletionTokens: 31},
		}, 8192, nil
	}}
	got, err := c.Complete(ctx, request)
	want := Completion{Content: `{"records":[1]}`, PromptTokens: 127, CompletionTokens: 31}
	if err != nil || got != want || calls != 1 {
		t.Fatalf("Complete: got %+v, %v, %d calls, want %+v, nil, 1 call", got, err, calls, want)
	}
}

func TestKMSCompleterRequestOptions(t *testing.T) {
	for _, test := range []struct {
		name     string
		thinking bool
		schema   map[string]any
	}{
		{name: "unconstrained"},
		{name: "structured", schema: map[string]any{"type": "object"}},
		{name: "thinking unconstrained", thinking: true},
		{name: "thinking structured", thinking: true, schema: map[string]any{"type": "object"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			c := KMSCompleter{chat: func(_ context.Context, _ string, input model.D) (model.ChatResponse, int, error) {
				calls++
				if got := input["enable_thinking"]; got != test.thinking {
					t.Errorf("enable_thinking: got %v, want %v", got, test.thinking)
				}
				format, exists := input["response_format"]
				switch {
				case test.schema == nil:
					if exists {
						t.Errorf("response_format: got %#v, want absent", format)
					}
				default:
					want := model.D{"type": "json_schema", "json_schema": model.D{
						"name": "structured_output", "strict": true, "schema": test.schema,
					}}
					if !reflect.DeepEqual(format, want) {
						t.Errorf("response_format: got %#v, want %#v", format, want)
					}
				}
				return model.ChatResponse{
					Choices: []model.Choice{{Message: &model.ResponseMessage{Content: " \n{\"record_1\":\"pass\"}\t "}, FinishReasonPtr: new(model.FinishReasonStop)}},
					Usage:   &model.Usage{PromptTokens: 127, CompletionTokens: 31},
				}, 8192, nil
			}}
			got, err := c.Complete(t.Context(), CompletionRequest{
				Model: "model", MaxTokens: 4096, EnableThinking: test.thinking, ResponseSchema: test.schema,
			})
			want := Completion{Content: `{"record_1":"pass"}`, PromptTokens: 127, CompletionTokens: 31}
			if err != nil || got != want || calls != 1 {
				t.Fatalf("Complete: got %+v, %v, %d calls, want %+v, nil, 1 call", got, err, calls, want)
			}
		})
	}
}

func TestKMSCompleterThinkingBudget(t *testing.T) {
	for _, test := range []struct {
		name          string
		contextWindow int
		promptTokens  int
		wantErr       bool
	}{
		{name: "below boundary", contextWindow: 8192, promptTokens: 4095},
		{name: "exact boundary", contextWindow: 8192, promptTokens: 4096},
		{name: "over boundary", contextWindow: 8192, promptTokens: 4097, wantErr: true},
		{name: "smaller window exact boundary", contextWindow: 6144, promptTokens: 2048},
		{name: "smaller window over boundary", contextWindow: 6144, promptTokens: 2049, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			c := KMSCompleter{chat: func(context.Context, string, model.D) (model.ChatResponse, int, error) {
				calls++
				return model.ChatResponse{
					Choices: []model.Choice{{Message: &model.ResponseMessage{Content: `{"record_1":"pass"}`}, FinishReasonPtr: new(model.FinishReasonStop)}},
					Usage:   &model.Usage{PromptTokens: test.promptTokens, CompletionTokens: 37},
				}, test.contextWindow, nil
			}}
			got, err := c.Complete(t.Context(), CompletionRequest{Model: "model", MaxTokens: 4096, EnableThinking: true})
			want := Completion{Content: `{"record_1":"pass"}`, PromptTokens: test.promptTokens, CompletionTokens: 37}
			if got != want || calls != 1 {
				t.Errorf("Complete: got %+v, %d calls, want %+v, 1 call", got, calls, want)
			}
			switch {
			case test.wantErr:
				if err == nil || !strings.Contains(err.Error(), "insufficient context budget") {
					t.Fatalf("error: got %v, want insufficient context budget", err)
				}
			case err != nil:
				t.Fatalf("error: got %v, want nil", err)
			}
		})
	}
}

func TestKMSCompleterResponses(t *testing.T) {
	stop, length, tool, failed := model.FinishReasonStop, model.FinishReasonLength, model.FinishReasonTool, model.FinishReasonError
	tests := []struct {
		name      string
		choices   []model.Choice
		want      Completion
		wantErr   string
		wantIs    error
		withUsage bool
	}{
		{name: "no choices", wantErr: "exactly one choice"},
		{name: "multiple choices", choices: []model.Choice{{}, {}}, wantErr: "exactly one choice"},
		{name: "nil message", choices: []model.Choice{{FinishReasonPtr: &stop}}, wantErr: "no message"},
		{name: "empty content", choices: []model.Choice{{Message: &model.ResponseMessage{}, FinishReasonPtr: &stop}}, wantErr: "empty content"},
		{name: "whitespace content", choices: []model.Choice{{Message: &model.ResponseMessage{Content: " \n\t"}, FinishReasonPtr: &stop}}, wantErr: "empty content"},
		{name: "missing finish", choices: []model.Choice{{Message: &model.ResponseMessage{Content: "partial"}}}, want: Completion{Content: "partial"}, wantErr: "did not finish successfully"},
		{name: "tool finish", choices: []model.Choice{{Message: &model.ResponseMessage{Content: "tool"}, FinishReasonPtr: &tool}}, want: Completion{Content: "tool"}, wantErr: "did not finish successfully"},
		{name: "error finish", choices: []model.Choice{{Message: &model.ResponseMessage{Content: "failed"}, FinishReasonPtr: &failed}}, want: Completion{Content: "failed"}, wantErr: "did not finish successfully"},
		{name: "truncated", choices: []model.Choice{{Message: &model.ResponseMessage{Content: "partial"}, FinishReasonPtr: &length}}, withUsage: true, want: Completion{Content: "partial", PromptTokens: 83, CompletionTokens: 47}, wantErr: "after 47 tokens", wantIs: ErrCompletionTruncated},
		{name: "empty truncated", choices: []model.Choice{{Message: &model.ResponseMessage{}, FinishReasonPtr: &length}}, wantErr: "after 47 tokens", wantIs: ErrCompletionTruncated},
		{name: "no usage", choices: []model.Choice{{Message: &model.ResponseMessage{Content: "complete"}, FinishReasonPtr: &stop}}, want: Completion{Content: "complete"}},
	}
	for _, enableThinking := range []bool{false, true} {
		name := "default"
		if enableThinking {
			name = "thinking"
		}
		t.Run(name, func(t *testing.T) {
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					calls := 0
					c := KMSCompleter{chat: func(context.Context, string, model.D) (model.ChatResponse, int, error) {
						calls++
						response := model.ChatResponse{Choices: test.choices}
						if test.withUsage {
							response.Usage = &model.Usage{PromptTokens: 83, CompletionTokens: 47}
						}
						return response, 8192, nil
					}}
					got, err := c.Complete(t.Context(), CompletionRequest{Model: "model", MaxTokens: 47, EnableThinking: enableThinking})
					if got != test.want || calls != 1 {
						t.Errorf("Complete: got %+v, %d calls, want %+v, 1 call", got, calls, test.want)
					}
					wantErr := test.wantErr
					if enableThinking && wantErr == "" && !test.withUsage {
						wantErr = "missing usage for context-budget validation"
					}
					if wantErr == "" {
						if err != nil {
							t.Fatalf("error: got %v, want nil", err)
						}
					} else if err == nil || !strings.Contains(err.Error(), wantErr) {
						t.Fatalf("error: got %v, want containing %q", err, wantErr)
					}
					if test.wantIs != nil && !errors.Is(err, test.wantIs) {
						t.Errorf("error: got %v, want wrapping %v", err, test.wantIs)
					}
				})
			}
		})
	}
}

func TestKMSCompleterFailuresWithoutRetry(t *testing.T) {
	for _, wantErr := range []error{errors.New("temporary backend failure"), model.ErrInvalidRequest, context.Canceled, context.DeadlineExceeded} {
		t.Run(wantErr.Error(), func(t *testing.T) {
			calls := 0
			c := KMSCompleter{chat: func(context.Context, string, model.D) (model.ChatResponse, int, error) {
				calls++
				return model.ChatResponse{}, 0, wantErr
			}}
			got, err := c.Complete(t.Context(), CompletionRequest{Model: "model"})
			if !errors.Is(err, wantErr) || err == wantErr || calls != 1 || got != (Completion{}) {
				t.Fatalf("Complete: got %+v, %v, %d calls, want zero completion, wrapped %v, 1 call", got, err, calls, wantErr)
			}
		})
	}
}

func TestKMSCompleterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	done := make(chan error, 1)
	c := KMSCompleter{chat: func(ctx context.Context, _ string, _ model.D) (model.ChatResponse, int, error) {
		close(started)
		<-ctx.Done()
		return model.ChatResponse{}, 0, ctx.Err()
	}}
	go func() {
		_, err := c.Complete(ctx, CompletionRequest{Model: "model"})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Complete: got %v, want wrapped context.Canceled", err)
	}
}

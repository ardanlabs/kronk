package kronk

import (
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
)

func TestResponseValidatesMessagesBeforeAdmission(t *testing.T) {
	tests := []struct {
		name string
		d    model.D
		want error
	}{
		{name: "missing", d: model.D{}, want: model.ErrMessagesMissing},
		{name: "invalid", d: model.D{"messages": "invalid"}, want: model.ErrMessagesInvalid},
		{name: "empty input", d: model.D{"input": []any{}}, want: model.ErrMessagesMissing},
		{name: "nil input", d: model.D{"input": nil}, want: model.ErrMessagesInvalid},
		{name: "scalar input", d: model.D{"input": 42}, want: model.ErrMessagesInvalid},
		{name: "required without tools", d: model.D{"input": "hello", "tool_choice": "required"}, want: model.ErrInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var krn Kronk

			if _, err := krn.Response(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("Response: got %v, want %v", err, tt.want)
			}
			if _, err := krn.ResponseStreaming(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("ResponseStreaming: got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestResponsesRejectStopWhenPresent(t *testing.T) {
	for _, value := range []any{nil, "END", []any{"END"}} {
		d := model.D{"input": "hello", "stop": value}
		var krn Kronk

		if _, err := krn.Response(t.Context(), d); !errors.Is(err, model.ErrInvalidRequest) {
			t.Errorf("Response stop %v: got %v, want ErrInvalidRequest", value, err)
		}
		if _, err := krn.ResponseStreaming(t.Context(), d); !errors.Is(err, model.ErrInvalidRequest) {
			t.Errorf("ResponseStreaming stop %v: got %v, want ErrInvalidRequest", value, err)
		}
	}
}

func TestToChatResponseToResponsesUsageIncludesReasoning(t *testing.T) {
	chatResp := model.ChatResponse{
		Usage: &model.Usage{
			PromptTokens:        100,
			PromptTokensDetails: model.PromptTokensDetails{CachedTokens: 80},
			CompletionTokens:    25,
			CompletionTokensDetails: model.CompletionTokensDetails{
				ReasoningTokens: 20,
			},
			TotalTokens: 125,
		},
	}

	resp := toChatResponseToResponses(chatResp, model.D{})

	if got, want := resp.Usage.OutputTokens, 25; got != want {
		t.Errorf("OutputTokens: got %d, want %d", got, want)
	}
	if got, want := resp.Usage.InputTokensDetails.CachedTokens, 80; got != want {
		t.Errorf("CachedTokens: got %d, want %d", got, want)
	}
	if got, want := resp.Usage.OutputTokenDetail.ReasoningTokens, 20; got != want {
		t.Errorf("ReasoningTokens: got %d, want %d", got, want)
	}
	if got, want := resp.Usage.TotalTokens, 125; got != want {
		t.Errorf("TotalTokens: got %d, want %d", got, want)
	}
}

func TestToChatResponseToResponsesWithoutUsage(t *testing.T) {
	resp := toChatResponseToResponses(model.ChatResponse{}, model.D{})

	if got := resp.Usage; got != (ResponseUsage{}) {
		t.Errorf("Usage: got %+v, want zero value", got)
	}
}

func TestToChatResponseToResponsesPreservesToolChoice(t *testing.T) {
	toolChoice := model.D{"type": "function", "function": model.D{"name": "get_weather"}}
	want := model.D{"type": "function", "name": "get_weather"}
	chatResp := model.ChatResponse{Usage: &model.Usage{}}

	resp := toChatResponseToResponses(chatResp, model.D{"tool_choice": toolChoice})

	if diff := cmp.Diff(any(want), resp.ToolChoice); diff != "" {
		t.Errorf("tool choice mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertInputToMessagesAcceptsResponsesToolChoice(t *testing.T) {
	toolChoice := model.D{"type": "function", "name": "get_weather"}
	d := model.D{
		"input":       "hello",
		"tool_choice": toolChoice,
		"tools": []model.D{
			{"type": "function", "name": "get_weather"},
		},
	}

	got, err := convertInputToMessages(d)
	if err != nil {
		t.Fatalf("convertInputToMessages: %v", err)
	}
	if err := model.ValidateChatRequest(got); err != nil {
		t.Fatalf("ValidateChatRequest: %v", err)
	}
	want := model.D{"type": "function", "function": model.D{"name": "get_weather"}}
	if diff := cmp.Diff(any(want), got["tool_choice"]); diff != "" {
		t.Errorf("tool choice mismatch (-want +got):\n%s", diff)
	}
	wantTools := []model.D{{
		"type":     "function",
		"function": model.D{"name": "get_weather"},
	}}
	if diff := cmp.Diff(wantTools, got["tools"]); diff != "" {
		t.Errorf("tools mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertInputToMessagesRejectsChatToolChoice(t *testing.T) {
	d := model.D{
		"input": "hello",
		"tool_choice": model.D{
			"type":     "function",
			"function": model.D{"name": "get_weather"},
		},
	}

	_, err := convertInputToMessages(d)
	if !errors.Is(err, model.ErrInvalidRequest) {
		t.Fatalf("convertInputToMessages: got %v, want ErrInvalidRequest", err)
	}
}

func TestToChatResponseToResponsesTokenLimit(t *testing.T) {
	finishReason := model.FinishReasonLength
	chatResp := model.ChatResponse{
		Choices: []model.Choice{{
			Message:         &model.ResponseMessage{Content: "partial"},
			FinishReasonPtr: &finishReason,
		}},
		Usage: &model.Usage{},
	}

	resp := toChatResponseToResponses(chatResp, model.D{})
	if got, want := resp.Status, "incomplete"; got != want {
		t.Errorf("Status: got %q, want %q", got, want)
	}
	if resp.CompletedAt != nil {
		t.Errorf("CompletedAt: got %d, want nil", *resp.CompletedAt)
	}
	if resp.IncompleteDetail == nil {
		t.Fatal("IncompleteDetail: got nil, want max_output_tokens")
	}
	if got, want := resp.IncompleteDetail.Reason, "max_output_tokens"; got != want {
		t.Errorf("IncompleteDetail.Reason: got %q, want %q", got, want)
	}
}

func TestStreamStateCompleteTokenLimit(t *testing.T) {
	finishReason := model.FinishReasonLength
	chatResp := model.ChatResponse{
		Choices: []model.Choice{{FinishReasonPtr: &finishReason}},
		Usage: &model.Usage{
			PromptTokensDetails: model.PromptTokensDetails{CachedTokens: 42},
		},
	}
	ss := streamState{}

	events := ss.complete(chatResp)
	if len(events) == 0 {
		t.Fatal("complete: got no events")
	}
	last := events[len(events)-1]
	if got, want := last.Type, "response.incomplete"; got != want {
		t.Errorf("event type: got %q, want %q", got, want)
	}
	if last.Response == nil {
		t.Fatal("event response: got nil")
	}
	if got, want := last.Response.Status, "incomplete"; got != want {
		t.Errorf("response status: got %q, want %q", got, want)
	}
	if got, want := last.Response.Usage.InputTokensDetails.CachedTokens, 42; got != want {
		t.Errorf("CachedTokens: got %d, want %d", got, want)
	}
}

func TestConvertInputToMessagesRoleShapedImage(t *testing.T) {
	const imageURL = "data:image/jpeg;base64,aW1hZ2U="

	input := model.D{
		"input": []model.D{
			{
				"role": "user",
				"content": []model.D{
					{"type": "input_image", "image_url": imageURL},
					{"type": "input_text", "text": "describe this image"},
				},
			},
		},
	}

	want := model.D{
		"messages": []model.D{
			{
				"role": "user",
				"content": []model.D{
					{
						"type": "image_url",
						"image_url": model.D{
							"url": imageURL,
						},
					},
					{"type": "text", "text": "describe this image"},
				},
			},
		},
	}

	got, err := convertInputToMessages(input)
	if err != nil {
		t.Fatalf("convertInputToMessages: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("convertInputToMessages mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertInputToMessagesMapsMaxOutputTokens(t *testing.T) {
	d := model.D{
		"input":             "hello",
		"max_output_tokens": float64(32),
		"max_tokens":        16,
	}

	got, err := convertInputToMessages(d)
	if err != nil {
		t.Fatalf("convertInputToMessages: %v", err)
	}

	if got["max_tokens"] != float64(32) {
		t.Errorf("max_tokens: got %v, want 32", got["max_tokens"])
	}
	params := extractInputParams(got)
	if params.MaxOutputTokens == nil || *params.MaxOutputTokens != 32 {
		t.Errorf("MaxOutputTokens: got %v, want 32", params.MaxOutputTokens)
	}
}

func TestConvertInputToMessagesRejectsFileInput(t *testing.T) {
	tests := []struct {
		name  string
		input model.D
	}{
		{
			name: "role-shaped input",
			input: model.D{
				"input": []model.D{
					{
						"role": "user",
						"content": []model.D{
							{"type": "input_file", "filename": "document.pdf", "file_data": "data:application/pdf;base64,JVBERg=="},
						},
					},
				},
			},
		},
		{
			name: "flat input",
			input: model.D{
				"input": []model.D{
					{"type": "input_file", "filename": "document.txt", "file_data": "dGV4dA=="},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := convertInputToMessages(tt.input)
			if err == nil {
				t.Fatal("convertInputToMessages: expected error")
			}
			if !errors.Is(err, model.ErrFileInputsUnsupported) {
				t.Errorf("error: got %v, want ErrFileInputsUnsupported", err)
			}
			if got, want := err.Error(), "convert-input-to-messages: file inputs are not currently supported"; got != want {
				t.Errorf("error: got %q, want %q", got, want)
			}
		})
	}
}

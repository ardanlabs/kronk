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

func TestToChatResponseToResponsesUsageIncludesReasoning(t *testing.T) {
	chatResp := model.ChatResponse{
		Usage: &model.Usage{
			PromptTokens:     100,
			ReasoningTokens:  20,
			CompletionTokens: 5,
			OutputTokens:     25,
			TotalTokens:      125,
		},
	}

	resp := toChatResponseToResponses(chatResp, model.D{})

	if got, want := resp.Usage.OutputTokens, 25; got != want {
		t.Errorf("OutputTokens: got %d, want %d", got, want)
	}
	if got, want := resp.Usage.OutputTokenDetail.ReasoningTokens, 20; got != want {
		t.Errorf("ReasoningTokens: got %d, want %d", got, want)
	}
	if got, want := resp.Usage.TotalTokens, 125; got != want {
		t.Errorf("TotalTokens: got %d, want %d", got, want)
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

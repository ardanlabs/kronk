package kronk

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestStreamIncludeUsage(t *testing.T) {
	tests := []struct {
		name string
		d    model.D
		want bool
	}{
		{name: "omitted defaults true", d: model.D{}, want: true},
		{name: "D true", d: model.D{"stream_options": model.D{"include_usage": true}}, want: true},
		{name: "D false", d: model.D{"stream_options": model.D{"include_usage": false}}, want: false},
		{name: "map true", d: model.D{"stream_options": map[string]any{"include_usage": true}}, want: true},
		{name: "map false", d: model.D{"stream_options": map[string]any{"include_usage": false}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := streamIncludeUsage(tt.d); got != tt.want {
				t.Errorf("streamIncludeUsage: got %t, want %t", got, tt.want)
			}
		})
	}
}

func TestMarshalChatStreamError(t *testing.T) {
	resp := model.ChatResponseErr(
		"id",
		model.ObjectChatText,
		"model",
		0,
		"",
		errors.New("inference failed"),
		model.Usage{},
	)

	data, err := marshalChatStreamError(resp)
	if err != nil {
		t.Fatalf("marshalChatStreamError: %v", err)
	}

	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got, want := len(wire), 1; got != want {
		t.Fatalf("top-level fields: got %d, want %d", got, want)
	}

	var apiErr struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(wire["error"], &apiErr); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got, want := apiErr.Message, "inference failed"; got != want {
		t.Errorf("Message: got %q, want %q", got, want)
	}
	if got, want := apiErr.Type, "server_error"; got != want {
		t.Errorf("Type: got %q, want %q", got, want)
	}
	if got, want := apiErr.Code, "server_error"; got != want {
		t.Errorf("Code: got %q, want %q", got, want)
	}
}

func TestChatValidatesMessagesBeforeAdmission(t *testing.T) {
	tests := []struct {
		name string
		d    model.D
		want error
	}{
		{name: "missing", d: model.D{}, want: model.ErrMessagesMissing},
		{name: "invalid", d: model.D{"messages": "invalid"}, want: model.ErrMessagesInvalid},
		{
			name: "required without tools",
			d: model.D{
				"messages":    []model.D{{"role": "user", "content": "hello"}},
				"tool_choice": "required",
			},
			want: model.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var krn Kronk

			if _, err := krn.Chat(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("Chat: got %v, want %v", err, tt.want)
			}
			if _, err := krn.ChatStreaming(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("ChatStreaming: got %v, want %v", err, tt.want)
			}
		})
	}
}

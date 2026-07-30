package model

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestDeserializeToolCallArguments(t *testing.T) {
	want := map[string]any{"location": "New York City, NY"}

	doubleEncoded, err := json.Marshal(ToolCallArguments(want))
	if err != nil {
		t.Fatalf("marshal tool call arguments: %v", err)
	}

	tests := []struct {
		name      string
		arguments string
		want      map[string]any
	}{
		{
			name:      "json object text",
			arguments: `{"location":"New York City, NY"}`,
			want:      want,
		},
		{
			name:      "json string containing object text",
			arguments: string(doubleEncoded),
			want:      want,
		},
		{
			name:      "null",
			arguments: "null",
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{
				"messages": []D{
					{
						"role": "assistant",
						"tool_calls": []D{
							{
								"type": "function",
								"function": D{
									"name":      "get_weather",
									"arguments": tt.arguments,
								},
							},
						},
					},
				},
			}

			got := deserializeToolCallArguments(d)
			messages := got["messages"].([]D)
			toolCalls := messages[0]["tool_calls"].([]D)
			function := toolCalls[0]["function"].(D)
			arguments, ok := function["arguments"].(map[string]any)
			if !ok {
				t.Fatalf("arguments type = %T, want map[string]any", function["arguments"])
			}

			if !reflect.DeepEqual(arguments, tt.want) {
				t.Errorf("arguments = %#v, want %#v", arguments, tt.want)
			}
		})
	}
}

func TestValidateDocumentRejectsFileInput(t *testing.T) {
	tests := []struct {
		name     string
		partType string
	}{
		{name: "chat completions file", partType: "file"},
		{name: "responses input file", partType: "input_file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{
				"messages": []D{
					{
						"role": "user",
						"content": []D{
							{"type": tt.partType},
						},
					},
				},
			}

			var m Model
			_, err := m.validateDocument(d)
			if err == nil {
				t.Fatal("validateDocument: expected error")
			}
			if !errors.Is(err, ErrFileInputsUnsupported) {
				t.Errorf("error: got %v, want ErrFileInputsUnsupported", err)
			}
			if got, want := err.Error(), "validate-document: messages[0].content[0]: file inputs are not currently supported"; got != want {
				t.Errorf("error: got %q, want %q", got, want)
			}
		})
	}
}

func TestValidateMessages(t *testing.T) {
	tests := []struct {
		name string
		d    D
		want error
	}{
		{
			name: "missing messages",
			d:    D{},
			want: ErrMessagesMissing,
		},
		{
			name: "messages wrong type",
			d:    D{"messages": "invalid"},
			want: ErrMessagesInvalid,
		},
		{
			name: "messages empty",
			d:    D{"messages": []D{}},
			want: ErrMessagesMissing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMessages(tt.d)
			if !errors.Is(err, tt.want) {
				t.Fatalf("ValidateMessages: got %v, want %v", err, tt.want)
			}
		})
	}
}

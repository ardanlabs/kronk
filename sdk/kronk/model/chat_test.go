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

func TestChatResponseFinalFinishReason(t *testing.T) {
	toolCalls := []ResponseToolCall{{
		ID:   "call_1",
		Type: "function",
		Function: ResponseToolCallFunction{
			Name:      "get_weather",
			Arguments: ToolCallArguments{},
		},
	}}

	tests := []struct {
		name         string
		finishReason string
		toolCalls    []ResponseToolCall
		want         string
	}{
		{name: "natural stop", want: FinishReasonStop},
		{name: "complete tool call", toolCalls: toolCalls, want: FinishReasonTool},
		{name: "token limit", finishReason: FinishReasonLength, want: FinishReasonLength},
		{name: "truncated tool call", finishReason: FinishReasonLength, toolCalls: toolCalls, want: FinishReasonLength},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := chatResponseFinal("id", ObjectChatTextFinal, "model", 0, "", "", "", tt.toolCalls, nil, tt.finishReason, Usage{})
			if got := resp.Choices[0].FinishReason(); got != tt.want {
				t.Errorf("FinishReason: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseParamsTemperature(t *testing.T) {
	tests := []struct {
		name        string
		temperature any
		include     bool
		want        float32
	}{
		{name: "omitted uses model default", want: 0.6},
		{name: "explicit zero is greedy", temperature: 0, include: true, want: 0},
		{name: "explicit nonzero overrides model default", temperature: 0.2, include: true, want: 0.2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				cfg: Config{DefaultParams: Params{Temperature: 0.6}},
				log: noopLog,
			}
			d := D{}
			if tt.include {
				d["temperature"] = tt.temperature
			}

			params, err := m.parseParams(d)
			if err != nil {
				t.Fatalf("parseParams: %v", err)
			}
			if got := params.Temperature; got != tt.want {
				t.Errorf("Temperature: got %v, want %v", got, tt.want)
			}
		})
	}
}

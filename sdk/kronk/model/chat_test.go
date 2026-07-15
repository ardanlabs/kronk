package model

import (
	"encoding/json"
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

package model

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestFormatLogContentPreservesTextPartBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		content any
		want    string
	}{
		{
			name: "typed parts without separator",
			content: []D{
				{"type": "text", "text": "(no output)"},
				{"type": "text", "text": "Command exited with code 0."},
			},
			want: "(2 parts, full text): (no output)Command exited with code 0.",
		},
		{
			name: "typed parts with explicit newline",
			content: []D{
				{"type": "text", "text": "(no output)\n"},
				{"type": "text", "text": "Command exited with code 0."},
			},
			want: "(2 parts, full text): (no output)\nCommand exited with code 0.",
		},
		{
			name:    "normalized parts",
			content: []any{"before", []byte{1, 2}, "after"},
			want:    "(3 parts, 2 media bytes in 1 media parts, full text): beforeafter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLogContent(tt.content); got != tt.want {
				t.Errorf("formatLogContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

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

func TestDeserializeToolCallArgumentsPreservesToolHistory(t *testing.T) {
	const request = `{
		"messages": [
			{
				"role": "assistant",
				"content": "Checking both locations.",
				"tool_calls": [
					{
						"id": "call_1",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"location\":\"Austin\",\"days\":2}"
						}
					},
					{
						"id": "call_2",
						"type": "function",
						"function": {
							"name": "get_weather",
							"arguments": "{\"location\":\"Seattle\",\"days\":3}"
						}
					}
				]
			},
			{
				"role": "tool",
				"tool_call_id": "call_1",
				"name": "get_weather",
				"content": "sunny"
			}
		]
	}`

	var wire map[string]any
	if err := json.Unmarshal([]byte(request), &wire); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	d := MapToModelD(wire)
	got := deserializeToolCallArguments(d)

	messages, ok := got["messages"].([]D)
	if !ok {
		t.Fatalf("messages type: got %T, want []D", got["messages"])
	}

	toolCalls, ok := messages[0]["tool_calls"].([]D)
	if !ok {
		t.Fatalf("tool_calls type: got %T, want []D", messages[0]["tool_calls"])
	}

	for i, want := range []struct {
		id       string
		name     string
		location string
		days     float64
	}{
		{id: "call_1", name: "get_weather", location: "Austin", days: 2},
		{id: "call_2", name: "get_weather", location: "Seattle", days: 3},
	} {
		if toolCalls[i]["id"] != want.id {
			t.Errorf("tool_calls[%d].id: got %q, want %q", i, toolCalls[i]["id"], want.id)
		}
		if toolCalls[i]["type"] != "function" {
			t.Errorf("tool_calls[%d].type: got %q, want %q", i, toolCalls[i]["type"], "function")
		}

		function := toolCalls[i]["function"].(D)
		if function["name"] != want.name {
			t.Errorf("tool_calls[%d].function.name: got %q, want %q", i, function["name"], want.name)
		}

		arguments, ok := function["arguments"].(map[string]any)
		if !ok {
			t.Fatalf("tool_calls[%d].function.arguments type: got %T, want map[string]any", i, function["arguments"])
		}
		if arguments["location"] != want.location {
			t.Errorf("tool_calls[%d].function.arguments.location: got %q, want %q", i, arguments["location"], want.location)
		}
		if arguments["days"] != want.days {
			t.Errorf("tool_calls[%d].function.arguments.days: got %v, want %v", i, arguments["days"], want.days)
		}
	}

	toolResult := messages[1]
	if toolResult["role"] != "tool" ||
		toolResult["tool_call_id"] != "call_1" ||
		toolResult["name"] != "get_weather" ||
		toolResult["content"] != "sunny" {
		t.Errorf("tool result changed: got %#v", toolResult)
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
			_, err := m.validateDocument(context.Background(), d)
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

func TestValidateChatRequestToolChoice(t *testing.T) {
	tests := []struct {
		name       string
		toolChoice any
		tools      []D
		include    bool
		wantErr    bool
	}{
		{name: "omitted"},
		{name: "auto", toolChoice: "auto", include: true},
		{name: "none", toolChoice: "none", include: true},
		{
			name:       "required",
			toolChoice: "required",
			tools: []D{
				{"type": "function", "function": D{"name": "get_weather"}},
			},
			include: true,
		},
		{
			name:       "chat function",
			toolChoice: D{"type": "function", "function": D{"name": "get_weather"}},
			tools: []D{
				{"type": "function", "function": D{"name": "get_weather"}},
			},
			include: true,
		},
		{
			name:       "different function",
			toolChoice: D{"type": "function", "function": D{"name": "get_weather"}},
			tools: []D{
				{"type": "function", "function": D{"name": "search"}},
			},
			include: true,
			wantErr: true,
		},
		{name: "required without tools", toolChoice: "required", include: true, wantErr: true},
		{name: "required with malformed tool", toolChoice: "required", tools: []D{{"type": "function"}}, include: true, wantErr: true},
		{name: "bare function name", toolChoice: "get_weather", include: true, wantErr: true},
		{name: "unknown string", toolChoice: "sometimes", include: true, wantErr: true},
		{name: "responses function", toolChoice: D{"type": "function", "name": "get_weather"}, include: true, wantErr: true},
		{name: "missing object name", toolChoice: D{"type": "function"}, include: true, wantErr: true},
		{name: "malformed nested function", toolChoice: D{"type": "function", "function": "get_weather"}, include: true, wantErr: true},
		{name: "unsupported object type", toolChoice: D{"type": "custom", "name": "get_weather"}, include: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{"messages": []D{{"role": "user", "content": "hello"}}}
			if tt.tools != nil {
				d["tools"] = tt.tools
			}
			if tt.include {
				d["tool_choice"] = tt.toolChoice
			}

			err := ValidateChatRequest(d)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidRequest) {
					t.Fatalf("ValidateChatRequest: got %v, want ErrInvalidRequest", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateChatRequest: got %v, want nil", err)
			}
		})
	}
}

func TestApplyToolChoice(t *testing.T) {
	tools := []D{
		{"type": "function", "function": D{"name": "get_weather"}},
		{"type": "function", "function": D{"name": "search"}},
	}

	t.Run("none removes tools", func(t *testing.T) {
		d := D{"tool_choice": "none", "tools": tools}
		applyToolChoice(d)
		if _, exists := d["tools"]; exists {
			t.Fatal("tools still present")
		}
	})

	t.Run("required preserves tools", func(t *testing.T) {
		d := D{"tool_choice": "required", "tools": tools}
		applyToolChoice(d)
		if got := len(d["tools"].([]D)); got != 2 {
			t.Fatalf("tools: got %d, want 2", got)
		}
	})

	t.Run("function selects one tool", func(t *testing.T) {
		d := D{
			"tool_choice": D{"type": "function", "function": D{"name": "search"}},
			"tools":       tools,
		}
		applyToolChoice(d)

		gotTools := d["tools"].([]D)
		if got := len(gotTools); got != 1 {
			t.Fatalf("tools: got %d, want 1", got)
		}
		function := gotTools[0]["function"].(D)
		if got, want := function["name"], "search"; got != want {
			t.Errorf("function name: got %v, want %v", got, want)
		}

		choice := d["tool_choice"].(D)
		function = choice["function"].(D)
		if got, want := function["name"], "search"; got != want {
			t.Errorf("normalized tool_choice name: got %v, want %v", got, want)
		}
	})
}

func TestValidateChatRequestRejectsStop(t *testing.T) {
	d := D{
		"messages": []D{{"role": "user", "content": "hello"}},
		"stop":     []string{"END"},
	}

	err := ValidateChatRequest(d)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("ValidateChatRequest: got %v, want ErrInvalidRequest", err)
	}
	if got, want := err.Error(), "stop is not supported"; !strings.Contains(got, want) {
		t.Errorf("error: got %q, want to contain %q", got, want)
	}
	if got := d.String(); strings.Contains(got, "stop") {
		t.Errorf("D.String: got %q, want stop omitted", got)
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
			resp := chatResponseFinal("id", ObjectChatTextFinal, "model", 0, "", "", "", tt.toolCalls, nil, nil, tt.finishReason, Usage{})
			if got := resp.Choices[0].FinishReason(); got != tt.want {
				t.Errorf("FinishReason: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatResponseToolCallDeltaJSON(t *testing.T) {
	resp := chatResponseToolCallDelta("id", ObjectChatText, "model", 0, ResponseToolCallDelta{
		ID:    "call_1",
		Index: 0,
		Type:  "function",
		Function: ResponseToolCallDeltaFunction{
			Name: "get_weather",
		},
	})

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	choices := wire["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	toolCalls := delta["tool_calls"].([]any)
	toolCall := toolCalls[0].(map[string]any)
	function := toolCall["function"].(map[string]any)

	if got, want := toolCall["id"], "call_1"; got != want {
		t.Errorf("tool call ID: got %v, want %v", got, want)
	}
	if got, want := function["name"], "get_weather"; got != want {
		t.Errorf("function name: got %v, want %v", got, want)
	}
	if got, want := function["arguments"], ""; got != want {
		t.Errorf("function arguments: got %v, want %v", got, want)
	}
}

func TestChatResponseFinalRetainsCompletedToolCalls(t *testing.T) {
	toolCalls := []ResponseToolCall{{
		ID:    "call_1",
		Index: 0,
		Type:  "function",
		Function: ResponseToolCallFunction{
			Name:      "get_weather",
			Arguments: ToolCallArguments{"location": "London"},
		},
	}}
	started := []ResponseToolCallDelta{{
		ID:    "call_1",
		Index: 0,
		Type:  "function",
		Function: ResponseToolCallDeltaFunction{
			Name: "get_weather",
		},
	}}
	terminal := reconcileStartedToolCalls(toolCalls, started)

	resp := chatResponseFinal("id", ObjectChatTextFinal, "model", 0, "", "answer", "thought", toolCalls, terminal, nil, "", Usage{})
	if resp.Choices[0].Delta == nil {
		t.Fatal("Delta: got nil, want completed tool calls for streaming compatibility")
	}
	if got := resp.Choices[0].Delta.Content; got != "" {
		t.Errorf("Delta.Content: got %q, want empty terminal delta", got)
	}
	if got := resp.Choices[0].Delta.Reasoning; got != "" {
		t.Errorf("Delta.Reasoning: got %q, want empty terminal delta", got)
	}
	if got := resp.Choices[0].Delta.ToolCalls; len(got) != 1 {
		t.Fatalf("Delta.ToolCalls: got %d calls, want 1", len(got))
	}
	if got, want := resp.Choices[0].Delta.ToolCalls[0].Function.Arguments["location"], "London"; got != want {
		t.Errorf("location: got %v, want %v", got, want)
	}
	if got, want := resp.Choices[0].Message.Content, "answer"; got != want {
		t.Errorf("Message.Content: got %q, want %q", got, want)
	}
	if got, want := resp.Choices[0].Message.Reasoning, "thought"; got != want {
		t.Errorf("Message.Reasoning: got %q, want %q", got, want)
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	choices := wire["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	terminalCalls := delta["tool_calls"].([]any)
	terminalCall := terminalCalls[0].(map[string]any)
	if _, exists := terminalCall["id"]; exists {
		t.Errorf("terminal delta: got repeated id in %v", terminalCall)
	}
	function := terminalCall["function"].(map[string]any)
	if _, exists := function["name"]; exists {
		t.Errorf("terminal delta: got repeated function name in %v", function)
	}
	if got, want := function["arguments"], `{"location":"London"}`; got != want {
		t.Errorf("terminal arguments: got %v, want %v", got, want)
	}
}

func TestChatResponseTextDeltaOmitsToolCalls(t *testing.T) {
	resp := chatResponseDelta("id", ObjectChatText, "model", 0, "hello", false, nil)

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), `"tool_calls"`) {
		t.Errorf("JSON: got %s, want tool_calls omitted", data)
	}
}

func TestReconcileStartedToolCalls(t *testing.T) {
	toolCalls := []ResponseToolCall{
		{
			ID:   "final-id",
			Type: "function",
			Function: ResponseToolCallFunction{
				Name:      "get_weather",
				Arguments: ToolCallArguments{"location": "London"},
			},
		},
	}
	started := []ResponseToolCallDelta{
		{
			ID:    "stream-id",
			Index: 1,
			Type:  "function",
			Function: ResponseToolCallDeltaFunction{
				Name: "get_weather",
			},
		},
	}

	terminal := reconcileStartedToolCalls(toolCalls, started)
	if len(terminal) != 1 {
		t.Fatalf("terminal deltas: got %d, want 1", len(terminal))
	}
	if got, want := toolCalls[0].ID, "stream-id"; got != want {
		t.Errorf("ID: got %q, want %q", got, want)
	}
	if got, want := toolCalls[0].Index, 1; got != want {
		t.Errorf("Index: got %d, want %d", got, want)
	}

	toolCalls[0].ID = "final-id"
	terminal = reconcileStartedToolCalls(toolCalls, nil)
	if terminal != nil {
		t.Errorf("terminal deltas without started calls: got %v, want nil", terminal)
	}
	if got, want := toolCalls[0].ID, "final-id"; got != want {
		t.Errorf("ID without started calls: got %q, want %q", got, want)
	}
}

func TestReconcileStartedToolCallsIndependently(t *testing.T) {
	toolCalls := []ResponseToolCall{
		{
			ID:     "bad-final-id",
			Type:   "function",
			Status: 2,
			Function: ResponseToolCallFunction{
				Name: "broken",
			},
		},
		{
			ID:   "good-final-id",
			Type: "function",
			Function: ResponseToolCallFunction{
				Name:      "working",
				Arguments: ToolCallArguments{"value": "ok"},
			},
		},
	}
	started := []ResponseToolCallDelta{
		{ID: "bad-start-id", Index: 0, Type: "function", Function: ResponseToolCallDeltaFunction{Name: "broken"}},
		{ID: "good-start-id", Index: 1, Type: "function", Function: ResponseToolCallDeltaFunction{Name: "working"}},
	}

	terminal := reconcileStartedToolCalls(toolCalls, started)
	if len(terminal) != 2 {
		t.Fatalf("terminal deltas: got %d, want 2", len(terminal))
	}
	if got, want := toolCalls[0].ID, "bad-final-id"; got != want {
		t.Errorf("malformed call ID: got %q, want %q", got, want)
	}
	if got := terminal[0].Function.Name; got != "" {
		t.Errorf("malformed terminal name: got %q, want empty", got)
	}
	if got, want := toolCalls[1].ID, "good-start-id"; got != want {
		t.Errorf("valid call ID: got %q, want %q", got, want)
	}
	if got := terminal[1].Function.Name; got != "" {
		t.Errorf("valid terminal name: got %q, want empty", got)
	}
}

func TestReconcileStartedToolCallsWithCardinalityMismatch(t *testing.T) {
	toolCalls := []ResponseToolCall{
		{ID: "first-final", Type: "function", Function: ResponseToolCallFunction{Name: "first", Arguments: ToolCallArguments{}}},
		{ID: "second-final", Type: "function", Function: ResponseToolCallFunction{Name: "second", Arguments: ToolCallArguments{}}},
	}
	started := []ResponseToolCallDelta{
		{ID: "first-start", Index: 0, Type: "function", Function: ResponseToolCallDeltaFunction{Name: "first"}},
	}

	terminal := reconcileStartedToolCalls(toolCalls, started)
	if len(terminal) != 2 {
		t.Fatalf("terminal deltas: got %d, want 2", len(terminal))
	}
	if got, want := toolCalls[0].ID, "first-start"; got != want {
		t.Errorf("announced call ID: got %q, want %q", got, want)
	}
	if got := terminal[0].Function.Name; got != "" {
		t.Errorf("announced terminal name: got %q, want empty", got)
	}
	if got, want := terminal[1].Function.Name, "second"; got != want {
		t.Errorf("unannounced terminal name: got %q, want %q", got, want)
	}
}

func TestReconcileStartedToolCallsAfterUnannouncedMalformedCall(t *testing.T) {
	toolCalls := []ResponseToolCall{
		{ID: "bad-final", Type: "function", Status: 2, Function: ResponseToolCallFunction{}},
		{ID: "good-final", Type: "function", Function: ResponseToolCallFunction{Name: "working", Arguments: ToolCallArguments{"value": "ok"}}},
	}
	started := []ResponseToolCallDelta{
		{ID: "good-start", Index: 0, Type: "function", Function: ResponseToolCallDeltaFunction{Name: "working"}},
	}

	terminal := reconcileStartedToolCalls(toolCalls, started)
	if got, want := toolCalls[0].Index, 1; got != want {
		t.Errorf("unannounced malformed index: got %d, want %d", got, want)
	}
	if got, want := terminal[0].ID, "bad-final"; got != want {
		t.Errorf("unannounced malformed terminal ID: got %q, want %q", got, want)
	}
	if got, want := toolCalls[1].ID, "good-start"; got != want {
		t.Errorf("announced valid ID: got %q, want %q", got, want)
	}
	if got := terminal[1].Function.Name; got != "" {
		t.Errorf("announced valid terminal name: got %q, want empty", got)
	}
}

func TestStreamingResponseLoggerStringReportsTotalBytes(t *testing.T) {
	l := StreamingResponseLogger{
		finishReason: FinishReasonStop,
		content:      "answer",
		reasoning:    strings.Repeat("r", 500),
	}

	got := l.String()
	for _, want := range []string{
		"Content (6 bytes total, first 400 characters shown): answer",
		"Reasoning (500 bytes total, first 400 characters shown): " + strings.Repeat("r", 400),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("String: got %q, want substring %q", got, want)
		}
	}
}

func TestFlushStateMachine(t *testing.T) {
	tests := []struct {
		name              string
		result            Result
		wantContent       string
		wantReasoning     string
		wantTooling       string
		wantToolFlag      int
		wantReasonFlag    int
		wantAnswerFlag    int
		initialReasonFlag int
		wantDelta         bool
		suppressTools     bool
	}{
		{name: "answer", result: Result{Channel: ChannelAnswer, Content: "answer"}, wantContent: "answer", wantAnswerFlag: 1, wantDelta: true},
		{name: "answer transition CRLF", result: Result{Channel: ChannelAnswer, Content: "\n\n"}, wantAnswerFlag: 1},
		{name: "reasoning", result: Result{Channel: ChannelReasoning, Content: "thought"}, wantReasoning: "thought", wantReasonFlag: 1, wantDelta: true},
		{name: "reasoning transition CRLF", result: Result{Channel: ChannelReasoning, Content: "\n"}, wantReasonFlag: 1},
		{name: "tool", result: Result{Channel: ChannelTool, Content: `{"name":"lookup","arguments":{}}`}, wantTooling: `{"name":"lookup","arguments":{}}`, wantToolFlag: 1},
		{name: "tool clears reasoning", result: Result{Channel: ChannelTool, Content: `{"name":"lookup","arguments":{}}`}, initialReasonFlag: 3, wantTooling: `{"name":"lookup","arguments":{}}`, wantToolFlag: 1},
		{name: "tool suppressed", result: Result{Channel: ChannelTool, Content: `{"name":"lookup","arguments":{}}`}, suppressTools: true, wantContent: `{"name":"lookup","arguments":{}}`, wantAnswerFlag: 1, wantDelta: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan ChatResponse, 1)
			stateMachine := &flushStateMachine{result: tt.result}
			s := slot{
				stateMachine:     stateMachine,
				job:              &chatJob{ctx: context.Background(), ch: ch, id: "id", object: ObjectChatText},
				reasonFlag:       tt.initialReasonFlag,
				suppressTools:    tt.suppressTools,
				reasonTokens:     2,
				completionTokens: 3,
				finishReason:     FinishReasonLength,
			}
			e := batchEngine{model: &Model{}}

			e.flushStateMachine(&s, stateMachine.Flush())

			if got := s.finalContent.String(); got != tt.wantContent {
				t.Errorf("finalContent: got %q, want %q", got, tt.wantContent)
			}
			if got := s.finalReasoning.String(); got != tt.wantReasoning {
				t.Errorf("finalReasoning: got %q, want %q", got, tt.wantReasoning)
			}
			if got := s.finalTooling.String(); got != tt.wantTooling {
				t.Errorf("finalTooling: got %q, want %q", got, tt.wantTooling)
			}
			if s.toolFlag != tt.wantToolFlag {
				t.Errorf("toolFlag: got %d, want %d", s.toolFlag, tt.wantToolFlag)
			}
			if s.reasonFlag != tt.wantReasonFlag {
				t.Errorf("reasonFlag: got %d, want %d", s.reasonFlag, tt.wantReasonFlag)
			}
			if s.completionFlag != tt.wantAnswerFlag {
				t.Errorf("completionFlag: got %d, want %d", s.completionFlag, tt.wantAnswerFlag)
			}
			if s.finishReason != FinishReasonLength {
				t.Errorf("finishReason: got %q, want %q", s.finishReason, FinishReasonLength)
			}
			if s.reasonTokens != 2 || s.completionTokens != 3 {
				t.Errorf("token counts: got reasoning=%d completion=%d, want reasoning=2 completion=3", s.reasonTokens, s.completionTokens)
			}

			if tt.wantDelta {
				resp := <-ch
				if resp.Choices[0].Delta == nil {
					t.Fatal("delta: got nil, want flushed content")
				}
				if got := resp.Choices[0].Delta.Content; got != tt.wantContent {
					t.Errorf("delta content: got %q, want %q", got, tt.wantContent)
				}
				if got := resp.Choices[0].Delta.Reasoning; got != tt.wantReasoning {
					t.Errorf("delta reasoning: got %q, want %q", got, tt.wantReasoning)
				}
			} else if len(ch) != 0 {
				t.Errorf("tool flush sent %d raw deltas, want 0", len(ch))
			}
		})
	}
}

func TestRetainAndStreamResultUsesCleanedContentConsistently(t *testing.T) {
	tests := []struct {
		name           string
		result         Result
		reasonFlag     int
		completionFlag int
		wantContent    string
		wantReasoning  string
		wantDelta      bool
		wantLogprobs   int
	}{
		{name: "answer transition CRLF", result: Result{Channel: ChannelAnswer, Content: "\n\n"}, completionFlag: 1},
		{name: "reasoning transition CRLF", result: Result{Channel: ChannelReasoning, Content: "\n"}, reasonFlag: 1},
		{name: "answer content", result: Result{Channel: ChannelAnswer, Content: "answer"}, completionFlag: 2, wantContent: "answer", wantDelta: true, wantLogprobs: 1},
		{name: "reasoning content", result: Result{Channel: ChannelReasoning, Content: "thought"}, reasonFlag: 2, wantReasoning: "thought", wantDelta: true, wantLogprobs: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan ChatResponse, 1)
			logprob := ContentLogprob{}
			s := slot{
				job:            &chatJob{ctx: context.Background(), ch: ch, id: "id", object: ObjectChatText},
				reasonFlag:     tt.reasonFlag,
				completionFlag: tt.completionFlag,
				currentLogprob: &logprob,
				logprobsData:   []ContentLogprob{logprob},
			}
			e := batchEngine{model: &Model{}}

			if err := e.retainAndStreamResult(&s, tt.result, 1, &logprob); err != nil {
				t.Fatalf("retainAndStreamResult: %v", err)
			}

			if got := s.finalContent.String(); got != tt.wantContent {
				t.Errorf("finalContent: got %q, want %q", got, tt.wantContent)
			}
			if got := s.finalReasoning.String(); got != tt.wantReasoning {
				t.Errorf("finalReasoning: got %q, want %q", got, tt.wantReasoning)
			}
			if got := len(s.logprobsData); got != tt.wantLogprobs {
				t.Errorf("logprobsData: got %d entries, want %d", got, tt.wantLogprobs)
			}
			if tt.wantDelta {
				resp := <-ch
				if got := resp.Choices[0].Delta.Content; got != tt.wantContent {
					t.Errorf("delta content: got %q, want %q", got, tt.wantContent)
				}
				if got := resp.Choices[0].Delta.Reasoning; got != tt.wantReasoning {
					t.Errorf("delta reasoning: got %q, want %q", got, tt.wantReasoning)
				}
			} else if len(ch) != 0 {
				t.Errorf("deltas: got %d, want 0", len(ch))
			}
		})
	}
}

type flushStateMachine struct {
	result Result
}

func (sm *flushStateMachine) Classify(string) (Result, bool) {
	return Result{}, false
}

func (sm *flushStateMachine) Reset() {}

func (sm *flushStateMachine) Flush() Result {
	result := sm.result
	sm.result = Result{}
	return result
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

			params, err := m.parseParams(context.Background(), d)
			if err != nil {
				t.Fatalf("parseParams: %v", err)
			}
			if got := params.Temperature; got != tt.want {
				t.Errorf("Temperature: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseParamsMaxCompletionTokens(t *testing.T) {
	tests := []struct {
		name string
		doc  D
		want int
	}{
		{name: "omitted uses model default", doc: D{}, want: 256},
		{name: "legacy max tokens remains supported", doc: D{"max_tokens": 64}, want: 64},
		{name: "max completion tokens is supported", doc: D{"max_completion_tokens": 32}, want: 32},
		{
			name: "max completion tokens takes precedence",
			doc:  D{"max_tokens": 64, "max_completion_tokens": 32},
			want: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				cfg: Config{DefaultParams: Params{MaxTokens: 256}},
				log: noopLog,
			}

			params, err := m.parseParams(context.Background(), tt.doc)
			if err != nil {
				t.Fatalf("parseParams: %v", err)
			}
			if got := params.MaxTokens; got != tt.want {
				t.Errorf("MaxTokens: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseParamsTopP(t *testing.T) {
	tests := []struct {
		name    string
		topP    any
		include bool
		want    float32
	}{
		{name: "omitted uses model default", want: 0.95},
		{name: "explicit zero overrides model default", topP: 0.0, include: true, want: 0.0},
		{name: "explicit one overrides model default", topP: 1.0, include: true, want: 1.0},
		{name: "explicit non-default overrides model default", topP: 0.8, include: true, want: 0.8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				cfg: Config{DefaultParams: Params{TopP: 0.95}},
				log: noopLog,
			}
			d := D{}
			if tt.include {
				d["top_p"] = tt.topP
			}

			params, err := m.parseParams(context.Background(), d)
			if err != nil {
				t.Fatalf("parseParams: %v", err)
			}
			if got := params.TopP; got != tt.want {
				t.Errorf("TopP: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseParamsIncludeUsage(t *testing.T) {
	tests := []struct {
		name       string
		streamOpts any
		include    bool
		want       bool
	}{
		{name: "omitted defaults true", want: true},
		{name: "D true", streamOpts: D{"include_usage": true}, include: true, want: true},
		{name: "D false", streamOpts: D{"include_usage": false}, include: true, want: false},
		{name: "map true", streamOpts: map[string]any{"include_usage": true}, include: true, want: true},
		{name: "map false", streamOpts: map[string]any{"include_usage": false}, include: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{log: noopLog}
			d := D{}
			if tt.include {
				d["stream_options"] = tt.streamOpts
			}

			params, err := m.parseParams(context.Background(), d)
			if err != nil {
				t.Fatalf("parseParams: %v", err)
			}
			if got := params.IncludeUsage; got != tt.want {
				t.Errorf("IncludeUsage: got %t, want %t", got, tt.want)
			}
		})
	}
}

func TestParseParamsMinP(t *testing.T) {
	tests := []struct {
		name    string
		minP    any
		include bool
		want    float32
	}{
		{name: "omitted uses model default", want: 0.05},
		{name: "explicit zero disables min-p", minP: 0.0, include: true, want: 0.0},
		{name: "explicit nonzero overrides model default", minP: 0.1, include: true, want: 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				cfg: Config{DefaultParams: Params{MinP: 0.05}},
				log: noopLog,
			}
			d := D{}
			if tt.include {
				d["min_p"] = tt.minP
			}

			params, err := m.parseParams(context.Background(), d)
			if err != nil {
				t.Fatalf("parseParams: %v", err)
			}
			if got := params.MinP; got != tt.want {
				t.Errorf("MinP: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseParamsRepeatPenalty(t *testing.T) {
	tests := []struct {
		name          string
		repeatPenalty any
		include       bool
		want          float32
	}{
		{name: "omitted uses explicit model default", want: 1.1},
		{name: "explicit zero disables penalty", repeatPenalty: 0.0, include: true, want: 1.0},
		{name: "explicit negative disables penalty", repeatPenalty: -1.0, include: true, want: 1.0},
		{name: "explicit positive overrides model default", repeatPenalty: 1.2, include: true, want: 1.2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				cfg: Config{DefaultParams: Params{RepeatPenalty: 1.1}},
				log: noopLog,
			}
			d := D{}
			if tt.include {
				d["repeat_penalty"] = tt.repeatPenalty
			}

			params, err := m.parseParams(context.Background(), d)
			if err != nil {
				t.Fatalf("parseParams: %v", err)
			}
			if got := params.RepeatPenalty; got != tt.want {
				t.Errorf("RepeatPenalty: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveSamplingDefaults(t *testing.T) {
	metadata := map[string]string{
		"general.sampling.temp":           "1.0",
		"general.sampling.top_k":          "20",
		"general.sampling.top_p":          "0.95",
		"general.sampling.min_p":          "0.05",
		"general.sampling.penalty_last_n": "-1",
		"general.sampling.penalty_repeat": "1.1",
	}

	params := resolveSamplingDefaults(Params{Temperature: 0.6}, metadata, 4096)

	if got, want := params.Temperature, float32(0.6); got != want {
		t.Errorf("Temperature: got %v, want configured value %v", got, want)
	}
	if got, want := params.TopK, int32(20); got != want {
		t.Errorf("TopK: got %d, want GGUF value %d", got, want)
	}
	if got, want := params.TopP, float32(0.95); got != want {
		t.Errorf("TopP: got %v, want GGUF value %v", got, want)
	}
	if got, want := params.MinP, float32(0.05); got != want {
		t.Errorf("MinP: got %v, want GGUF value %v", got, want)
	}
	if got, want := params.RepeatLastN, int32(-1); got != want {
		t.Errorf("RepeatLastN: got %d, want GGUF value %d", got, want)
	}
	if got, want := params.RepeatPenalty, float32(DefRepeatPenalty); got != want {
		t.Errorf("RepeatPenalty: got %v, want disabled default %v", got, want)
	}
}

func TestResolveSamplingDefaultsFallback(t *testing.T) {
	metadata := map[string]string{
		"general.sampling.temp":  "NaN",
		"general.sampling.top_k": "invalid",
		"general.sampling.top_p": "+Inf",
	}

	params := resolveSamplingDefaults(Params{}, metadata, 4096)

	if got, want := params.Temperature, float32(DefTemp); got != want {
		t.Errorf("Temperature: got %v, want Kronk fallback %v", got, want)
	}
	if got, want := params.TopK, DefTopK; got != want {
		t.Errorf("TopK: got %d, want Kronk fallback %d", got, want)
	}
	if got, want := params.TopP, float32(DefTopP); got != want {
		t.Errorf("TopP: got %v, want Kronk fallback %v", got, want)
	}
}

func TestParseParamsPreservesResolvedZeroDefaults(t *testing.T) {
	contextWindow := 4096
	defaults := resolveSamplingDefaults(Params{}, map[string]string{
		"general.sampling.temp":            "0",
		"general.sampling.top_p":           "0",
		"general.sampling.penalty_repeat":  "0",
		"general.sampling.xtc.probability": "0",
		"general.sampling.xtc.threshold":   "0",
	}, contextWindow)
	m := Model{
		cfg: Config{
			PtrContextWindow: &contextWindow,
			DefaultParams:    defaults,
		},
		paramsResolved: true,
		log:            noopLog,
	}

	params, err := m.parseParams(context.Background(), D{
		"temperature":     0,
		"top_p":           0,
		"repeat_penalty":  0,
		"xtc_probability": 0,
		"xtc_threshold":   0,
	})
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}

	if params.Temperature != 0 {
		t.Errorf("Temperature: got %v, want resolved 0", params.Temperature)
	}
	if params.TopP != 0 {
		t.Errorf("TopP: got %v, want resolved 0", params.TopP)
	}
	if params.RepeatPenalty != DefRepeatPenalty {
		t.Errorf("RepeatPenalty: got %v, want disabled default %v", params.RepeatPenalty, DefRepeatPenalty)
	}
	if params.XtcProbability != 0 {
		t.Errorf("XtcProbability: got %v, want resolved 0", params.XtcProbability)
	}
	if params.XtcThreshold != 0 {
		t.Errorf("XtcThreshold: got %v, want resolved 0", params.XtcThreshold)
	}
}

func TestParseParamsSamplingSentinels(t *testing.T) {
	contextWindow := 4096
	defaults := resolveSamplingDefaults(Params{}, nil, contextWindow)
	m := Model{
		cfg: Config{
			PtrContextWindow: &contextWindow,
			DefaultParams:    defaults,
		},
		paramsResolved: true,
		log:            noopLog,
	}

	tests := []struct {
		name string
		doc  D
		want func(Params) bool
	}{
		{
			name: "DRY enabled uses full context default",
			doc:  D{"dry_multiplier": 1.0},
			want: func(p Params) bool { return p.DryPenaltyLast == -1 },
		},
		{
			name: "DRY zero disables its window",
			doc:  D{"dry_multiplier": 1.0, "dry_penalty_last_n": 0},
			want: func(p Params) bool { return p.DryPenaltyLast == 0 },
		},
		{
			name: "repeat zero disables its window",
			doc:  D{"repeat_penalty": 1.15, "repeat_last_n": 0},
			want: func(p Params) bool { return p.RepeatLastN == 0 },
		},
		{
			name: "repeat negative one uses full context",
			doc:  D{"repeat_penalty": 1.15, "repeat_last_n": -1},
			want: func(p Params) bool { return p.RepeatLastN == -1 },
		},
		{
			name: "top-k zero disables filtering",
			doc:  D{"top_k": 0},
			want: func(p Params) bool { return p.TopK == 0 },
		},
		{
			name: "negative top-k disables filtering",
			doc:  D{"top_k": -1},
			want: func(p Params) bool { return p.TopK == -1 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := m.parseParams(context.Background(), tt.doc)
			if err != nil {
				t.Fatalf("parseParams: %v", err)
			}
			if !tt.want(params) {
				t.Errorf("Params: got\n%s", params.String())
			}
		})
	}
}

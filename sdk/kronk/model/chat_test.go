package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/applog"
)

func TestChatResponseErrPreservesErrorIdentity(t *testing.T) {
	wantErr := fmt.Errorf("context limit: %w", ErrInvalidRequest)
	resp := ChatResponseErr("id", ObjectChatText, "model", 0, wantErr, Usage{})

	if !errors.Is(resp.internal.cause, ErrInvalidRequest) {
		t.Errorf("response error: got %v, want %v", resp.internal.cause, ErrInvalidRequest)
	}
}

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

func TestDCloneOwnsNestedJSONContainers(t *testing.T) {
	type namedMap map[string]string
	type namedSlice []string

	nested := map[string]any{"mode": "original"}
	labels := map[string]string{"role": "original"}
	choices := []string{"original"}
	namedLabels := namedMap{"role": "original"}
	namedChoices := namedSlice{"original"}
	d := D{
		"chat_template_kwargs": D{
			"custom":        []any{nested},
			"labels":        labels,
			"choices":       choices,
			"named_labels":  namedLabels,
			"named_choices": namedChoices,
		},
	}

	clone := d.Clone()
	nested["mode"] = "changed"
	labels["role"] = "changed"
	choices[0] = "changed"
	namedLabels["role"] = "changed"
	namedChoices[0] = "changed"
	kwargs := clone["chat_template_kwargs"].(D)
	items := kwargs["custom"].([]any)
	got := items[0].(D)["mode"]
	if got != "original" {
		t.Errorf("nested mode: got %q, want %q", got, "original")
	}
	if got := kwargs["labels"].(map[string]string)["role"]; got != "original" {
		t.Errorf("labels role: got %q, want %q", got, "original")
	}
	if got := kwargs["choices"].([]string)[0]; got != "original" {
		t.Errorf("choices[0]: got %q, want %q", got, "original")
	}
	if got := kwargs["named_labels"].(namedMap)["role"]; got != "original" {
		t.Errorf("named_labels role: got %q, want %q", got, "original")
	}
	if got := kwargs["named_choices"].(namedSlice)[0]; got != "original" {
		t.Errorf("named_choices[0]: got %q, want %q", got, "original")
	}
}

func TestNormalizeChatTemplateKwargs(t *testing.T) {
	tests := []struct {
		name         string
		doc          D
		wantThinking bool
		wantCustom   any
		wantErr      bool
	}{
		{
			name: "nested values promoted",
			doc: D{
				"chat_template_kwargs": D{"enable_thinking": false, "custom_mode": "fast"},
			},
			wantThinking: false,
			wantCustom:   "fast",
		},
		{
			name: "top-level value wins",
			doc: D{
				"enable_thinking":      true,
				"chat_template_kwargs": D{"enable_thinking": false},
			},
			wantThinking: true,
		},
		{
			name:    "non-object rejected",
			doc:     D{"chat_template_kwargs": false},
			wantErr: true,
		},
		{
			name:    "self-reference rejected",
			doc:     D{"chat_template_kwargs": D{"chat_template_kwargs": D{}}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.doc.Clone()
			err := normalizeChatTemplateKwargs(d, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("normalizeChatTemplateKwargs: got nil error, want error")
				}
				if !errors.Is(err, ErrInvalidRequest) {
					t.Errorf("normalizeChatTemplateKwargs error: got %v, want ErrInvalidRequest", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeChatTemplateKwargs: %v", err)
			}
			if got := d["enable_thinking"]; got != tt.wantThinking {
				t.Errorf("enable_thinking: got %v, want %t", got, tt.wantThinking)
			}
			kwargs := d["chat_template_kwargs"].(D)
			if tt.wantCustom != nil && kwargs["custom_mode"] != tt.wantCustom {
				t.Errorf("nested custom_mode: got %v, want %v", kwargs["custom_mode"], tt.wantCustom)
			}
		})
	}
}

func TestNormalizeChatTemplateKwargsMergesModelDefaults(t *testing.T) {
	defaults := D{
		"preserve_thinking": true,
		"custom_mode":       "default",
	}
	d := D{
		"messages": []D{{"role": "user", "content": "hello"}},
		"chat_template_kwargs": D{
			"custom_mode": "request",
		},
	}

	if err := normalizeChatTemplateKwargs(d, defaults); err != nil {
		t.Fatalf("normalizeChatTemplateKwargs: %v", err)
	}

	kwargs := d["chat_template_kwargs"].(D)
	if got := kwargs["preserve_thinking"]; got != true {
		t.Errorf("preserve_thinking: got %v, want true", got)
	}
	if got := kwargs["custom_mode"]; got != "request" {
		t.Errorf("custom_mode: got %v, want request", got)
	}
	if got := defaults["custom_mode"]; got != "default" {
		t.Errorf("default custom_mode mutated: got %v, want default", got)
	}
}

func TestNormalizeChatTemplateKwargsTopLevelOverridesModelDefault(t *testing.T) {
	d := D{
		"messages":          []D{{"role": "user", "content": "hello"}},
		"preserve_thinking": false,
	}

	if err := normalizeChatTemplateKwargs(d, D{"preserve_thinking": true}); err != nil {
		t.Fatalf("normalizeChatTemplateKwargs: %v", err)
	}

	m := Model{log: noopLog}
	m.template = Template{FileName: "kwargs-default-test", Script: `{{ preserve_thinking }}`}
	prompt, err := m.applyJinjaTemplate(context.Background(), d)
	if err != nil {
		t.Fatalf("applyJinjaTemplate: %v", err)
	}
	if prompt != "False" {
		t.Errorf("prompt: got %q, want %q", prompt, "False")
	}
}

func TestChatTemplateKwargsAreTemplateOnly(t *testing.T) {
	m := Model{log: noopLog}
	m.template = Template{FileName: "kwargs-test", Script: `{{ custom_mode }}:{{ temperature }}`}
	d := D{
		"messages":              []D{{"role": "user", "content": "hello"}},
		"add_generation_prompt": false,
		"bos_token":             "",
		"eos_token":             "",
		"chat_template_kwargs": D{
			"custom_mode": "fast",
			"temperature": 0.1,
		},
	}

	if err := normalizeChatTemplateKwargs(d, nil); err != nil {
		t.Fatalf("normalizeChatTemplateKwargs: %v", err)
	}
	params, err := m.parseParams(context.Background(), d)
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if params.Temperature == 0.1 {
		t.Error("nested template temperature changed sampling temperature")
	}

	prompt, err := m.applyJinjaTemplate(context.Background(), d)
	if err != nil {
		t.Fatalf("applyJinjaTemplate: %v", err)
	}
	if prompt != "fast:0.1" {
		t.Errorf("prompt: got %q, want %q", prompt, "fast:0.1")
	}
}

func TestParseParamsUsesChatTemplateKwargs(t *testing.T) {
	m := Model{log: noopLog}
	d := D{
		"messages": []D{{"role": "user", "content": "hello"}},
		"chat_template_kwargs": D{
			"enable_thinking": false,
		},
	}

	params, normalized, err := m.validateOwnedDocument(context.Background(), d)
	if err != nil {
		t.Fatalf("validateOwnedDocument: %v", err)
	}
	if params.Thinking != ThinkingDisabled {
		t.Errorf("Thinking: got %q, want %q", params.Thinking, ThinkingDisabled)
	}
	if got := normalized["enable_thinking"]; got != false {
		t.Errorf("normalized enable_thinking: got %v, want false", got)
	}
	if _, exists := normalized["reasoning_effort"]; exists {
		t.Error("normalized reasoning_effort: got framework default, want template default")
	}
}

func TestChatPreservesValidationError(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value any
	}{
		{name: "invalid seed", field: "seed", value: -1},
		{name: "unsupported response format", field: "response_format", value: D{"type": "banana"}},
		{name: "missing response schema", field: "response_format", value: D{"type": "json_schema"}},
		{name: "invalid float parameter", field: "temperature", value: D{"invalid": true}},
		{name: "invalid integer parameter", field: "max_tokens", value: D{"invalid": true}},
		{name: "invalid boolean parameter", field: "logprobs", value: "invalid"},
		{name: "invalid reasoning parameter", field: "reasoning_effort", value: "invalid"},
		{name: "invalid reasoning parameter type", field: "reasoning_effort", value: 1},
		{name: "unsupported choice count", field: "n", value: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{log: noopLog}
			d := D{
				"messages": []D{{"role": "user", "content": "hello"}},
				tt.field:   tt.value,
			}

			if _, err := m.Chat(t.Context(), d); !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("Chat: got %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestChatStreamingReturnsValidationError(t *testing.T) {
	m := Model{log: noopLog}
	d := D{
		"messages": []D{{"role": "user", "content": "hello"}},
		"seed":     -1,
		"chat_template_kwargs": D{
			"enable_thinking": false,
		},
	}

	ch, err := m.ChatStreaming(t.Context(), d)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("ChatStreaming: got %v, want ErrInvalidRequest", err)
	}
	if ch != nil {
		t.Error("ChatStreaming: got non-nil channel, want nil")
	}
	if active := m.activeStreams.Load(); active != 0 {
		t.Errorf("active streams: got %d, want 0", active)
	}
	if _, exists := d["enable_thinking"]; exists {
		t.Error("ChatStreaming modified caller-owned request")
	}
}

func TestChatChoiceCountValidation(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{name: "omitted"},
		{name: "null", value: nil},
		{name: "one decoded", value: json.Number("1")},
		{name: "one decoded decimal", value: json.Number("1.0")},
		{name: "one native", value: 1},
		{name: "multiple", value: json.Number("4"), wantErr: true},
		{name: "zero", value: json.Number("0"), wantErr: true},
		{name: "negative", value: -1, wantErr: true},
		{name: "fractional", value: json.Number("1.5"), wantErr: true},
		{name: "string", value: "1", wantErr: true},
		{name: "boolean", value: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{"messages": []D{{"role": "user", "content": "hello"}}}
			if tt.name != "omitted" {
				d["n"] = tt.value
			}

			err := ValidateChatRequest(d)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidRequest) {
					t.Errorf("ValidateChatRequest: got %v, want ErrInvalidRequest", err)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateChatRequest: %v", err)
			}
		})
	}
}

func TestChatStopValidationAndParsing(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    []string
		wantErr bool
	}{
		{name: "null", value: nil},
		{name: "string", value: "END", want: []string{"END"}},
		{name: "decoded array", value: []any{"A", "B"}, want: []string{"A", "B"}},
		{name: "native array", value: []string{"A", "B"}, want: []string{"A", "B"}},
		{name: "empty", value: "", wantErr: true},
		{name: "too many", value: []any{"1", "2", "3", "4", "5"}, wantErr: true},
		{name: "wrong scalar", value: true, wantErr: true},
		{name: "wrong entry", value: []any{"A", 2}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{"messages": []D{{"role": "user", "content": "hello"}}, "stop": tt.value}
			err := ValidateChatRequest(d)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidRequest) {
					t.Errorf("ValidateChatRequest: got %v, want ErrInvalidRequest", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateChatRequest: %v", err)
			}

			m := Model{log: noopLog}
			params, err := m.parseParams(t.Context(), d)
			if err != nil {
				t.Fatalf("parseParams: %v", err)
			}
			if !reflect.DeepEqual(params.Stop, tt.want) {
				t.Errorf("Stop: got %v, want %v", params.Stop, tt.want)
			}
		})
	}
}

func TestParseParamsUsesDefaultStop(t *testing.T) {
	m := Model{log: noopLog, cfg: Config{DefaultParams: Params{Stop: []string{"default"}}}}
	d := D{"messages": []D{{"role": "user", "content": "hello"}}}

	params, err := m.parseParams(t.Context(), d)
	if err != nil {
		t.Fatalf("parseParams: %v", err)
	}
	if !reflect.DeepEqual(params.Stop, []string{"default"}) {
		t.Errorf("Stop: got %v, want [default]", params.Stop)
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

func TestDeserializeToolCallArgumentsPreservesJSONNumbers(t *testing.T) {
	const arguments = `{"small":120000,"million":1000000,"large":9007199254740993}`

	doubleEncoded, err := json.Marshal(ToolCallArguments{
		"small":   json.Number("120000"),
		"million": json.Number("1000000"),
		"large":   json.Number("9007199254740993"),
	})
	if err != nil {
		t.Fatalf("marshal tool call arguments: %v", err)
	}

	tests := []struct {
		name      string
		arguments string
	}{
		{name: "json object text", arguments: arguments},
		{name: "json string containing object text", arguments: string(doubleEncoded)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{
				"messages": []D{
					{
						"role": "assistant",
						"tool_calls": []D{
							{
								"function": D{
									"name":      "run_job",
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
			gotArguments := function["arguments"].(map[string]any)

			for name, want := range map[string]string{
				"small":   "120000",
				"million": "1000000",
				"large":   "9007199254740993",
			} {
				number, ok := gotArguments[name].(json.Number)
				if !ok {
					t.Fatalf("arguments[%q] type: got %T, want json.Number", name, gotArguments[name])
				}
				if got := number.String(); got != want {
					t.Errorf("arguments[%q]: got %q, want %q", name, got, want)
				}
			}
		})
	}
}

func TestToolCallArgumentsUnmarshalJSONPreservesNumbers(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "string encoded",
			input: `"{\"million\":1000000,\"large\":9007199254740993}"`,
		},
		{
			name:  "object",
			input: `{"million":1000000,"large":9007199254740993}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var arguments ToolCallArguments
			if err := json.Unmarshal([]byte(tt.input), &arguments); err != nil {
				t.Fatalf("unmarshal tool call arguments: %v", err)
			}

			for name, want := range map[string]string{
				"million": "1000000",
				"large":   "9007199254740993",
			} {
				number, ok := arguments[name].(json.Number)
				if !ok {
					t.Fatalf("arguments[%q] type: got %T, want json.Number", name, arguments[name])
				}
				if got := number.String(); got != want {
					t.Errorf("arguments[%q]: got %q, want %q", name, got, want)
				}
			}
		})
	}
}

func TestToolCallArgumentsMarshalNilAsEmptyObject(t *testing.T) {
	var arguments ToolCallArguments

	data, err := json.Marshal(arguments)
	if err != nil {
		t.Fatalf("marshal tool call arguments: %v", err)
	}
	if got, want := string(data), `"{}"`; got != want {
		t.Errorf("arguments: got %q, want %q", got, want)
	}
}

func TestToolNumbersRenderWithoutPrecisionLoss(t *testing.T) {
	m := Model{log: noopLog}
	m.template = Template{
		FileName: "number-preservation-test",
		Script:   `{{ tools[0] | tojson }}|{{ messages[0].tool_calls[0].function.arguments.million | string }}|{{ messages[0].tool_calls[0].function.arguments.large | string }}`,
	}

	d := D{
		"messages": []D{
			{
				"role": "assistant",
				"tool_calls": []D{
					{
						"function": D{
							"name":      "run_job",
							"arguments": `{"million":1000000,"large":9007199254740993}`,
						},
					},
				},
			},
		},
		"tools": []D{
			{
				"type": "function",
				"function": D{
					"name": "run_job",
					"parameters": D{
						"type": "object",
						"properties": D{
							"timeout": D{"type": "integer", "default": json.Number("1000000")},
							"ticket":  D{"type": "integer", "default": json.Number("9007199254740993")},
						},
					},
				},
			},
		},
		"add_generation_prompt": false,
		"bos_token":             "",
		"eos_token":             "",
	}

	d = deserializeToolCallArguments(d)
	prompt, err := m.applyJinjaTemplate(context.Background(), d)
	if err != nil {
		t.Fatalf("applyJinjaTemplate: %v", err)
	}

	for _, want := range []string{"1000000", "9007199254740993"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt: got %q, want value %q", prompt, want)
		}
	}
	for _, unwanted := range []string{"1e+06", "9.007199254740992e+15", "9007199254740992"} {
		if strings.Contains(prompt, unwanted) {
			t.Errorf("prompt: got %q, unwanted value %q", prompt, unwanted)
		}
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
		days     json.Number
	}{
		{id: "call_1", name: "get_weather", location: "Austin", days: json.Number("2")},
		{id: "call_2", name: "get_weather", location: "Seattle", days: json.Number("3")},
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

func TestValidateChatRequestChatTemplateKwargs(t *testing.T) {
	tests := []struct {
		name   string
		kwargs any
	}{
		{name: "non-object", kwargs: false},
		{name: "self-reference", kwargs: D{"chat_template_kwargs": D{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{
				"messages":             []D{{"role": "user", "content": "hello"}},
				"chat_template_kwargs": tt.kwargs,
			}

			err := ValidateChatRequest(d)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("ValidateChatRequest: got %v, want ErrInvalidRequest", err)
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

func TestToolChoiceAvailableToJinjaTemplate(t *testing.T) {
	tools := []D{
		{"type": "function", "function": D{"name": "get_weather"}},
		{"type": "function", "function": D{"name": "search"}},
	}

	tests := []struct {
		name       string
		toolChoice any
		script     string
		want       string
	}{
		{
			name:       "required includes every tool",
			toolChoice: "required",
			script:     `{{ tool_choice }}:{% for tool in tools %}{{ tool.function.name }},{% endfor %}`,
			want:       "required:get_weather,search,",
		},
		{
			name:       "named choice includes selected tool",
			toolChoice: D{"type": "function", "function": D{"name": "search"}},
			script:     `{{ tool_choice.function.name }}:{% for tool in tools %}{{ tool.function.name }},{% endfor %}`,
			want:       "search:search,",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{
				"messages":    []D{{"role": "user", "content": "hello"}},
				"tools":       tools,
				"tool_choice": tt.toolChoice,
			}
			if err := ValidateChatRequest(d); err != nil {
				t.Fatalf("ValidateChatRequest: %v", err)
			}
			applyToolChoice(d)

			m := Model{log: noopLog}
			m.template = Template{FileName: "tool-choice-test", Script: tt.script}
			prompt, err := m.applyJinjaTemplate(t.Context(), d)
			if err != nil {
				t.Fatalf("applyJinjaTemplate: %v", err)
			}
			if prompt != tt.want {
				t.Errorf("prompt: got %q, want %q", prompt, tt.want)
			}
		})
	}
}

func TestValidateChatRequestAcceptsStopWithoutLoggingIt(t *testing.T) {
	d := D{
		"messages": []D{{"role": "user", "content": "hello"}},
		"stop":     []string{"END"},
	}

	if err := ValidateChatRequest(d); err != nil {
		t.Fatalf("ValidateChatRequest: %v", err)
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
			resp := chatResponseFinal("id", ObjectChatTextFinal, "model", 0, "", "", tt.toolCalls, nil, tt.finishReason, true, Usage{})
			if got := resp.Choices[0].FinishReason(); got != tt.want {
				t.Errorf("FinishReason: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRetainLengthTerminatedToolOutput(t *testing.T) {
	toolCall := ResponseToolCall{
		ID:   "call_1",
		Type: "function",
		Function: ResponseToolCallFunction{
			Name:      "lookup",
			Arguments: ToolCallArguments{},
		},
	}
	tests := []struct {
		name         string
		finishReason string
		content      string
		wantContent  string
		wantDelta    string
		wantCalls    int
		wantRetained bool
	}{
		{name: "incomplete call", finishReason: FinishReasonLength, wantContent: lengthTerminatedToolMessage, wantDelta: lengthTerminatedToolMessage, wantRetained: true},
		{name: "complete call", finishReason: FinishReasonLength, wantContent: lengthTerminatedToolMessage, wantDelta: lengthTerminatedToolMessage, wantRetained: true},
		{name: "mixed calls", finishReason: FinishReasonLength, wantContent: lengthTerminatedToolMessage, wantDelta: lengthTerminatedToolMessage, wantRetained: true},
		{name: "existing answer", finishReason: FinishReasonLength, content: "answer: ", wantContent: "answer: " + lengthTerminatedToolMessage, wantDelta: lengthTerminatedToolMessage, wantRetained: true},
		{name: "natural stop", finishReason: FinishReasonStop, content: "answer", wantContent: "answer", wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var content strings.Builder
			content.WriteString(tt.content)
			toolCalls := []ResponseToolCall{toolCall}

			gotDelta, gotRetained := retainLengthTerminatedToolOutput(tt.finishReason, &content, &toolCalls)

			if got := content.String(); got != tt.wantContent {
				t.Errorf("content: got %q, want %q", got, tt.wantContent)
			}
			if gotDelta != tt.wantDelta {
				t.Errorf("delta: got %q, want %q", gotDelta, tt.wantDelta)
			}
			if got := len(toolCalls); got != tt.wantCalls {
				t.Errorf("tool calls: got %d, want %d", got, tt.wantCalls)
			}
			if gotRetained != tt.wantRetained {
				t.Errorf("retained: got %t, want %t", gotRetained, tt.wantRetained)
			}
		})
	}
}

func TestLengthTerminatedToolOutputResponse(t *testing.T) {
	const answer = "I will update the file. "
	const tooling = `<tool_call>{"name":"write_file","arguments":{"path":"main.go","content":"package main</tool_ca`

	var content strings.Builder
	content.WriteString(answer)
	toolCalls := []ResponseToolCall{{ID: "call_1", Type: "function"}}
	deltaContent, retained := retainLengthTerminatedToolOutput(FinishReasonLength, &content, &toolCalls)
	if !retained {
		t.Fatal("retained: got false, want true")
	}
	if got := len(toolCalls); got != 0 {
		t.Fatalf("tool calls: got %d, want 0", got)
	}

	ctx := t.Context()
	ch := make(chan ChatResponse, 3)
	m := Model{log: applog.DiscardLogger}
	if err := m.sendDeltaResponse(ctx, ch, "id", ObjectChatText, 0, answer, ChannelAnswer, 0, 1, nil); err != nil {
		t.Fatalf("send answer delta: %v", err)
	}
	if err := m.sendDeltaResponse(ctx, ch, "id", ObjectChatText, 0, deltaContent, ChannelAnswer, 0, 256, nil); err != nil {
		t.Fatalf("send retained tool delta: %v", err)
	}
	m.sendFinalResponse(ctx, ch, "id", ObjectChatText, 0, &content, &strings.Builder{}, toolCalls, nil, nil, FinishReasonLength, "max-tokens", ChannelAnswer, len(tooling), true, false, Usage{CompletionTokens: 256})

	answerDelta := <-ch
	toolDelta := <-ch
	terminal := <-ch
	if got := len(ch); got != 0 {
		t.Fatalf("responses after terminal: got %d, want 0", got)
	}
	if got, want := answerDelta.Choices[0].Delta.Content+toolDelta.Choices[0].Delta.Content, content.String(); got != want {
		t.Errorf("reconstructed content: got %q, want %q", got, want)
	}
	if got, want := toolDelta.Choices[0].Delta.Role, RoleAssistant; got != want {
		t.Errorf("tool delta role: got %q, want %q", got, want)
	}
	if got := toolDelta.Choices[0].Delta.Content; got != lengthTerminatedToolMessage {
		t.Errorf("tool delta content: got %q, want %q", got, lengthTerminatedToolMessage)
	}

	choice := terminal.Choices[0]
	if got := choice.FinishReason(); got != FinishReasonLength {
		t.Errorf("finish reason: got %q, want %q", got, FinishReasonLength)
	}
	if got := choice.Message.Content; got != content.String() {
		t.Errorf("content: got %q, want %q", got, content.String())
	}
	if got := len(choice.Message.ToolCalls); got != 0 {
		t.Errorf("tool calls: got %d, want 0", got)
	}
	if strings.Contains(choice.Message.Content, "<tool_call>") || strings.Contains(choice.Message.Content, "</tool_ca") {
		t.Errorf("content: got tool markers in %q", choice.Message.Content)
	}
	if got := choice.Delta.Content; got != "" {
		t.Errorf("terminal delta content: got %q, want empty", got)
	}
}

func TestUsageCachedTokensJSON(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{name: "cache miss"},
		{name: "cache hit", want: 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := Usage{
				PromptTokensDetails: PromptTokensDetails{CachedTokens: tt.want},
			}

			data, err := json.Marshal(usage)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var wire struct {
				PromptTokensDetails PromptTokensDetails `json:"prompt_tokens_details"`
			}
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got := wire.PromptTokensDetails.CachedTokens; got != tt.want {
				t.Errorf("CachedTokens: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestUsageCompletionTokensJSON(t *testing.T) {
	usage := Usage{
		PromptTokens:     100,
		CompletionTokens: 25,
		CompletionTokensDetails: CompletionTokensDetails{
			ReasoningTokens: 20,
		},
		TotalTokens: 125,
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got, want := int(wire["completion_tokens"].(float64)), 25; got != want {
		t.Errorf("completion_tokens: got %d, want %d", got, want)
	}
	if got, want := int(wire["total_tokens"].(float64)), 125; got != want {
		t.Errorf("total_tokens: got %d, want %d", got, want)
	}

	details := wire["completion_tokens_details"].(map[string]any)
	if got, want := int(details["reasoning_tokens"].(float64)), 20; got != want {
		t.Errorf("reasoning_tokens: got %d, want %d", got, want)
	}

	for _, field := range []string{"reasoning_tokens", "output_tokens"} {
		if _, exists := wire[field]; exists {
			t.Errorf("%s: got top-level field, want absent", field)
		}
	}
}

func TestChatResponseFinalUsage(t *testing.T) {
	u := Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}

	withUsage := chatResponseFinal("id", ObjectChatTextFinal, "model", 0, "answer", "", nil, nil, FinishReasonStop, true, u)
	if withUsage.Usage == nil {
		t.Fatal("Usage: got nil, want usage")
	}
	if got := withUsage.Usage.TotalTokens; got != u.TotalTokens {
		t.Errorf("TotalTokens: got %d, want %d", got, u.TotalTokens)
	}

	withoutUsage := chatResponseFinal("id", ObjectChatTextFinal, "model", 0, "answer", "", nil, nil, FinishReasonStop, false, u)
	if withoutUsage.Usage != nil {
		t.Errorf("Usage: got %+v, want nil", withoutUsage.Usage)
	}
}

func TestChatResponseUsage(t *testing.T) {
	u := Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
	terminal := chatResponseFinal("id", ObjectChatText, "model", 0, "answer", "", nil, nil, FinishReasonStop, false, u)

	resp := chatResponseUsage(terminal, u)
	if resp.Choices == nil || len(resp.Choices) != 0 {
		t.Fatalf("Choices: got %v, want []", resp.Choices)
	}
	if resp.Usage == nil {
		t.Fatal("Usage: got nil, want usage")
	}
	if got := resp.Usage.TotalTokens; got != u.TotalTokens {
		t.Errorf("TotalTokens: got %d, want %d", got, u.TotalTokens)
	}
	if resp.ID != terminal.ID || resp.Object != terminal.Object || resp.Created != terminal.Created || resp.Model != terminal.Model || resp.SystemFingerprint != terminal.SystemFingerprint {
		t.Error("Metadata: got values that differ from terminal response")
	}
	if got := len(terminal.Choices); got != 1 {
		t.Errorf("terminal Choices: got %d, want 1", got)
	}
	if terminal.Usage != nil {
		t.Errorf("terminal Usage: got %+v, want nil", terminal.Usage)
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
	if got, want := delta["role"], RoleAssistant; got != want {
		t.Errorf("role: got %v, want %v", got, want)
	}
	toolCalls := delta["tool_calls"].([]any)
	toolCall := toolCalls[0].(map[string]any)
	function := toolCall["function"].(map[string]any)

	if got, want := toolCall["id"], "call_1"; got != want {
		t.Errorf("tool call ID: got %v, want %v", got, want)
	}
	if got, want := toolCall["index"], float64(0); got != want {
		t.Errorf("tool call index: got %v, want %v", got, want)
	}
	if got, want := function["name"], "get_weather"; got != want {
		t.Errorf("function name: got %v, want %v", got, want)
	}
	if got, want := function["arguments"], ""; got != want {
		t.Errorf("function arguments: got %v, want %v", got, want)
	}
}

func TestResponseMessageCompletedToolCallsOmitIndex(t *testing.T) {
	msg := ResponseMessage{
		ToolCalls: []ResponseToolCall{
			{ID: "call_1", Index: 0, Type: "function", Function: ResponseToolCallFunction{Name: "first", Arguments: ToolCallArguments{}}},
			{ID: "call_2", Index: 1, Type: "function", Function: ResponseToolCallFunction{Name: "second", Arguments: ToolCallArguments{}}},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var wire struct {
		ToolCalls []map[string]any `json:"tool_calls"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for i, toolCall := range wire.ToolCalls {
		if _, exists := toolCall["index"]; exists {
			t.Errorf("tool call %d: got index in %v, want omitted", i, toolCall)
		}
	}
}

func TestChatResponseFinalSeparatesCompletedToolCallsFromFinishReason(t *testing.T) {
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

	argumentResp := chatResponseToolCallDelta("id", ObjectChatText, "model", 0, terminal[0])
	if got := argumentResp.Choices[0].FinishReason(); got != "" {
		t.Fatalf("argument FinishReason: got %q, want empty", got)
	}
	if got, want := argumentResp.Choices[0].Delta.ToolCallDeltas[0].Function.Arguments, `{"location":"London"}`; got != want {
		t.Errorf("argument delta: got %q, want %q", got, want)
	}

	resp := chatResponseFinal("id", ObjectChatTextFinal, "model", 0, "answer", "thought", toolCalls, nil, "", true, Usage{})
	if resp.Choices[0].Delta == nil {
		t.Fatal("Delta: got nil, want empty terminal delta")
	}
	if got := resp.Choices[0].Delta.Content; got != "" {
		t.Errorf("Delta.Content: got %q, want empty terminal delta", got)
	}
	if got := resp.Choices[0].Delta.Reasoning; got != "" {
		t.Errorf("Delta.Reasoning: got %q, want empty terminal delta", got)
	}
	if got := resp.Choices[0].Delta.ToolCalls; len(got) != 0 {
		t.Fatalf("Delta.ToolCalls: got %d calls, want none in terminal chunk", len(got))
	}
	if got, want := resp.Choices[0].Message.Content, "answer"; got != want {
		t.Errorf("Message.Content: got %q, want %q", got, want)
	}
	if got, want := resp.Choices[0].Message.Reasoning, "thought"; got != want {
		t.Errorf("Message.Reasoning: got %q, want %q", got, want)
	}

	data, err := json.Marshal(argumentResp)
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

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	choices := wire["choices"].([]any)
	delta := choices[0].(map[string]any)["delta"].(map[string]any)
	if got, want := delta["role"], RoleAssistant; got != want {
		t.Errorf("role: got %v, want %v", got, want)
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
	if len(terminal) != 1 {
		t.Fatalf("terminal deltas without started calls: got %d, want 1", len(terminal))
	}
	if got, want := toolCalls[0].ID, "final-id"; got != want {
		t.Errorf("ID without started calls: got %q, want %q", got, want)
	}
	if got, want := terminal[0].Function.Arguments, `{"location":"London"}`; got != want {
		t.Errorf("arguments without started calls: got %q, want %q", got, want)
	}
}

func TestReconcileStartedToolCallWithoutArguments(t *testing.T) {
	toolCalls := []ResponseToolCall{{
		ID:   "final-id",
		Type: "function",
		Function: ResponseToolCallFunction{
			Name: "list_projects",
		},
	}}
	started := []ResponseToolCallDelta{{
		ID:    "stream-id",
		Index: 0,
		Type:  "function",
		Function: ResponseToolCallDeltaFunction{
			Name: "list_projects",
		},
	}}

	terminal := reconcileStartedToolCalls(toolCalls, started)
	if len(terminal) != 1 {
		t.Fatalf("terminal deltas: got %d, want 1", len(terminal))
	}
	if got, want := terminal[0].Function.Arguments, `{}`; got != want {
		t.Errorf("arguments: got %q, want %q", got, want)
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

func TestFlushAllStateMachine(t *testing.T) {
	sm := &flushStateMachine{results: []Result{
		{Channel: ChannelTool, Content: "first"},
		{Channel: ChannelTool, Content: "second"},
		{Channel: ChannelTool, Content: "malformed"},
	}}
	s := slot{
		job:          &chatJob{ctx: context.Background(), ch: make(chan ChatResponse, 1), id: "id", object: ObjectChatText},
		finishReason: FinishReasonStop,
	}
	e := batchEngine{model: &Model{}}

	e.flushAllStateMachine(&s, sm)

	if got := s.finalTooling.String(); got != "firstsecondmalformed" {
		t.Fatalf("finalTooling: got %q, want all queued results", got)
	}
	if got := sm.Flush(); got != (Result{}) {
		t.Fatalf("Flush after drain: got %+v, want zero result", got)
	}
}

func TestProcessDecodedPieceRetainsResultOnParserEOG(t *testing.T) {
	sm := &flushStateMachine{classifyResult: Result{Channel: ChannelTool, Content: "tooling"}, classifyEOG: true}
	s := slot{
		stateMachine: sm,
		job:          &chatJob{ctx: context.Background(), ch: make(chan ChatResponse, 1), id: "id", object: ObjectChatText},
		toolFlag:     1,
	}
	e := batchEngine{model: &Model{}}

	outcome := e.processDecodedPiece(&s, stopPiece{content: "terminal"}, -1, false)

	if !outcome.parserEOG || outcome.err != nil {
		t.Fatalf("outcome: got %+v, want parser EOG without error", outcome)
	}
	if got := s.finalTooling.String(); got != "tooling" {
		t.Fatalf("finalTooling: got %q, want retained EOG result", got)
	}
	if got := s.completionTokens; got != 1 {
		t.Fatalf("completionTokens: got %d, want parser-EOG piece counted", got)
	}
}

func TestReconcileParserEOGRemainder(t *testing.T) {
	released := stopPiece{content: "released", provisionalReason: true}
	pending := stopPiece{content: "pending", provisionalReason: true}
	discarded := stopPiece{content: "discarded", provisionalReason: true}
	s := slot{
		reasonTokens: 3,
		stopGate: &stopGate{
			pending:   []stopPiece{pending},
			discarded: []stopPiece{discarded},
		},
	}

	reconcileParserEOGRemainder(&s, []stopPiece{released})

	if s.reasonTokens != 0 || s.completionTokens != 3 {
		t.Fatalf("tokens: got reasoning=%d completion=%d, want 0/3", s.reasonTokens, s.completionTokens)
	}
	if len(s.stopGate.pending) != 0 || len(s.stopGate.discarded) != 0 {
		t.Fatalf("stop gate was not drained: %+v", s.stopGate)
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

			if err := e.retainAndStreamResult(&s, tt.result, 1, &logprob, 0); err != nil {
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
	result         Result
	results        []Result
	classifyResult Result
	classifyEOG    bool
}

func (sm *flushStateMachine) Classify(string) (Result, bool) {
	return sm.classifyResult, sm.classifyEOG
}

func (sm *flushStateMachine) Reset() {}

func (sm *flushStateMachine) Flush() Result {
	if len(sm.results) > 0 {
		result := sm.results[0]
		sm.results = sm.results[1:]
		return result
	}
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
		{name: "json number overrides model default", temperature: json.Number("0.2"), include: true, want: 0.2},
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
		{name: "json number max completion tokens is supported", doc: D{"max_completion_tokens": json.Number("32")}, want: 32},
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
		defaultVal bool
		want       bool
	}{
		{name: "omitted defaults false", want: false},
		{name: "omitted uses configured default", defaultVal: true, want: true},
		{name: "empty options defaults false", streamOpts: D{}, include: true, want: false},
		{name: "empty options uses configured default", streamOpts: D{}, include: true, defaultVal: true, want: true},
		{name: "D true", streamOpts: D{"include_usage": true}, include: true, want: true},
		{name: "D false overrides configured default", streamOpts: D{"include_usage": false}, include: true, defaultVal: true, want: false},
		{name: "map true", streamOpts: map[string]any{"include_usage": true}, include: true, want: true},
		{name: "map false overrides configured default", streamOpts: map[string]any{"include_usage": false}, include: true, defaultVal: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				cfg: Config{DefaultParams: Params{IncludeUsage: tt.defaultVal}},
				log: noopLog,
			}
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
	if params.ReasoningEffort != "" {
		t.Errorf("ReasoningEffort: got %q, want template default", params.ReasoningEffort)
	}
	explicit := resolveSamplingDefaults(Params{ReasoningEffort: ReasoningEffortHigh}, metadata, 4096)
	if explicit.ReasoningEffort != ReasoningEffortHigh {
		t.Errorf("explicit ReasoningEffort: got %q, want %q", explicit.ReasoningEffort, ReasoningEffortHigh)
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
	if params.ReasoningEffort != "" {
		t.Errorf("ReasoningEffort: got %q, want template default", params.ReasoningEffort)
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

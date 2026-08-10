package gemma

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

var noopLog applog.Logger = func(context.Context, string, ...any) {}

func TestParseGemma_GemmaQuotes(t *testing.T) {
	calls := parseGemma(context.Background(), noopLog,
		`call:get_weather{location:<|"|>New York<|"|>}`)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("name = %q", calls[0].Function.Name)
	}
	if got := calls[0].Function.Arguments["location"]; got != "New York" {
		t.Errorf("location = %v, want New York", got)
	}
}

func TestParseGemma_PureJSONInside(t *testing.T) {
	calls := parseGemma(context.Background(), noopLog,
		`call:get_weather{"location":"NYC"}`)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if got := calls[0].Function.Arguments["location"]; got != "NYC" {
		t.Errorf("location = %v, want NYC", got)
	}
}

func TestParseGemmaBareValue(t *testing.T) {
	tests := []struct {
		in   string
		want any
	}{
		{"true", true},
		{"false", false},
		{"null", nil},
		{"42", json.Number("42")},
		{"3.14", json.Number("3.14")},
		{"hello", "hello"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := parseGemmaBareValue(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseGemmaBareValue(%q) = %v (%T), want %v (%T)",
					tc.in, got, got, tc.want, tc.want)
			}
		})
	}
}

func TestToolCallWithSchema(t *testing.T) {
	tools := []model.D{{
		"type": "function",
		"function": model.D{
			"name": "convert",
			"parameters": model.D{
				"type": "object",
				"properties": model.D{
					"content": model.D{"type": "string"},
					"text":    model.D{"type": "string"},
					"enabled": model.D{"type": "boolean"},
					"count":   model.D{"type": "integer"},
				},
			},
		},
	}}

	calls := Parser{}.ToolCallWithSchema(context.Background(), noopLog,
		`call:convert{content:<|"|>{"enabled":true}<|"|>,text:true,enabled:true,count:9007199254740993}`,
		tools)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}

	want := model.ToolCallArguments{
		"content": `{"enabled":true}`,
		"text":    "true",
		"enabled": true,
		"count":   json.Number("9007199254740993"),
	}
	if got := calls[0].Function.Arguments; !reflect.DeepEqual(got, want) {
		t.Errorf("arguments: got %#v, want %#v", got, want)
	}
}

func TestParseGemmaArgs_QuotedJSONRemainsString(t *testing.T) {
	args, err := parseGemmaArgs(`content:<|"|>{"enabled":true}<|"|>`)
	if err != nil {
		t.Fatalf("parseGemmaArgs: %v", err)
	}
	if got := args["content"]; got != `{"enabled":true}` {
		t.Errorf("content: got %q (%T), want quoted JSON string", got, got)
	}
}

func TestParseGemma_RejectsMalformedOutputAtomically(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "argument delimiter injection",
			content: `call:write_file{path:<|"|>t.txt<|"|>,content:<|"|><|"|>}call:bash{command:<|"|>id<|"|>}<|"|>}`,
		},
		{name: "unmatched trailing content", content: `call:first{}unexpected`},
		{name: "incomplete argument block", content: `call:first{value:true`},
		{name: "unterminated gemma quote", content: `call:first{value:<|"|>text}`},
		{name: "duplicate gemma argument", content: `call:bash{command:<|"|>echo safe<|"|>,command:<|"|>id<|"|>}`},
		{name: "duplicate json argument", content: `call:bash{"command":"echo safe","command":"id"}`},
		{name: "escaped duplicate json argument", content: `call:bash{"command":"echo safe","\u0063ommand":"id"}`},
		{name: "nested duplicate json argument", content: `call:run{"config":{"command":"echo safe","command":"id"}}`},
		{name: "invalid composite", content: `call:first{config:{]}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := parseGemma(context.Background(), noopLog, tt.content)
			if len(calls) != 1 {
				t.Fatalf("tool calls: got %d, want 1 failed call", len(calls))
			}
			if calls[0].Status == 0 {
				t.Fatalf("Status: got 0, want parse failure: %+v", calls[0])
			}
			if calls[0].Function.Name != "" {
				t.Errorf("Function.Name: got %q, want no executable function", calls[0].Function.Name)
			}
			if calls[0].Raw != tt.content {
				t.Errorf("Raw: got %q, want %q", calls[0].Raw, tt.content)
			}
			if calls[0].Error == "" {
				t.Error("Error: got empty error, want parse failure detail")
			}
		})
	}
}

func TestParseGemma_BackToBackCalls(t *testing.T) {
	content := "call:first{}\ncall:second{}"
	calls := parseGemma(context.Background(), noopLog, content)
	if len(calls) != 2 {
		t.Fatalf("tool calls: got %d, want 2", len(calls))
	}

	for i, want := range []string{"first", "second"} {
		if calls[i].Status != 0 {
			t.Fatalf("tool call %d Status: got %d, want 0: %s", i, calls[i].Status, calls[i].Error)
		}
		if got := calls[i].Function.Name; got != want {
			t.Errorf("tool call %d Function.Name: got %q, want %q", i, got, want)
		}
	}
}

func TestParseGemma_PreservesSupportedArgumentForms(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "double brace", content: `call:write{{"content":"x"}}`},
		{name: "object delimiter in string", content: `call:f{obj:{"text":"}"}}`},
		{name: "array delimiter in string", content: `call:f{items:["]"]}`},
		{name: "mixed quote modes", content: `call:first{"text":"}"}call:second{value:<|"|>x<|"|>}`},
		{name: "space after gemma quote", content: `call:f{text:<|"|>x<|"|> ,next:true}`},
		{name: "even backslashes before quote", content: `call:f{"text":"path\\","next":true}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := parseGemma(context.Background(), noopLog, tt.content)
			if len(calls) == 0 {
				t.Fatal("tool calls: got 0, want at least 1")
			}
			for i, call := range calls {
				if call.Status != 0 {
					t.Fatalf("tool call %d Status: got %d, want 0: %s", i, call.Status, call.Error)
				}
			}
		})
	}
}

func TestToolCallWithSchema_QuotedValuesRemainStrings(t *testing.T) {
	tools := []model.D{{
		"type": "function",
		"function": model.D{
			"name": "convert",
			"parameters": model.D{
				"properties": model.D{
					"gemmaBoolean": model.D{"type": "boolean"},
					"jsonBoolean":  model.D{"type": "boolean"},
					"object":       model.D{"type": "object"},
					"array":        model.D{"type": "array"},
				},
			},
		},
	}}

	calls := Parser{}.ToolCallWithSchema(context.Background(), noopLog,
		`call:convert{gemmaBoolean:<|"|>true<|"|>,jsonBoolean:"true",object:<|"|>{"x":1}<|"|>,array:<|"|>[1,2]<|"|>}`,
		tools)
	want := model.ToolCallArguments{
		"gemmaBoolean": "true",
		"jsonBoolean":  "true",
		"object":       `{"x":1}`,
		"array":        `[1,2]`,
	}
	if got := calls[0].Function.Arguments; !reflect.DeepEqual(got, want) {
		t.Errorf("arguments: got %#v, want %#v", got, want)
	}
}

func TestToolCallWithSchema_NativeCompositesRemainTyped(t *testing.T) {
	tools := []model.D{{
		"type": "function",
		"function": model.D{
			"name": "convert",
			"parameters": model.D{
				"properties": model.D{
					"object": model.D{"type": "string"},
					"array":  model.D{"type": "string"},
				},
			},
		},
	}}

	calls := Parser{}.ToolCallWithSchema(context.Background(), noopLog,
		`call:convert{"object":{"x":1},"array":[1,2]}`, tools)
	want := model.ToolCallArguments{
		"object": map[string]any{"x": json.Number("1")},
		"array":  []any{json.Number("1"), json.Number("2")},
	}
	if got := calls[0].Function.Arguments; !reflect.DeepEqual(got, want) {
		t.Errorf("arguments: got %#v, want %#v", got, want)
	}
}

func TestParseGemmaBareValue_TrailingDataRemainsString(t *testing.T) {
	const value = "42 trailing"
	if got := parseGemmaBareValue(value); got != value {
		t.Errorf("value: got %q (%T), want %q", got, got, value)
	}
}

func TestStateMachineToolCallDeltas(t *testing.T) {
	var sm stateMachine
	sm.Reset()
	for _, token := range []string{"<tool_call>", "call:get_", "weather", "{", "}", "</tool_call>", "<tool_call>", "call:forecast", "{"} {
		sm.Classify(token)
	}

	deltas := sm.ToolCallDeltas()
	if len(deltas) != 2 || deltas[0].Function.Name != "get_weather" || deltas[1].Function.Name != "forecast" {
		t.Fatalf("ToolCallDeltas: got %+v, want get_weather and forecast", deltas)
	}
	if deltas[0].ID == "" || deltas[0].ID == deltas[1].ID || deltas[0].Index != 0 || deltas[1].Index != 1 || deltas[0].Type != "function" || deltas[0].Function.Arguments != "" {
		t.Errorf("identity-only deltas: got %+v", deltas)
	}
	if len(sm.ToolCallDeltas()) != 0 || len(sm.StartedToolCalls()) != 2 {
		t.Error("delta draining or started identities are incorrect")
	}
	sm.Reset()
	if len(sm.StartedToolCalls()) != 0 || len(sm.ToolCallDeltas()) != 0 {
		t.Error("Reset did not clear tool-call delta state")
	}
}

func TestStripToolCallMarkup(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"complete", `call:weather{city:<|"|>NYC<|"|>}`, ""},
		{"truncated", `call:weather{city:<|"|>NYC`, ""},
		{"trailing marker prefix", `call:weather{}<tool_`, ""},
		{"wrapped trailing marker prefix", `<tool_call>call:weather{}</tool_call><tool_`, ""},
		{"wrapped closer in string", `<tool_call>call:write{text:<|"|>before </tool_call> after<|"|>}</tool_call>`, ""},
		{"coalesced wrappers", `call:first{}</tool_call><tool_call>call:second{}</tool_call>`, ""},
		{"repeated mixed", "call:first{}\ncall:second{x:1}tail", "\ntail"},
		{"surrounding text", "before call:weather{} after", "before  after"},
		{"ordinary text", "please call: weather", "please call: weather"},
		{"wrapped ordinary body", "<tool_call>foreign</tool_call>", ""},
		{"whitespace residual", " \ncall:weather{}\t", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Parser{}).StripToolCallMarkup(tt.input); got != tt.want {
				t.Errorf("StripToolCallMarkup: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateMachineCallMarkerInsideArgumentsIsNotActivity(t *testing.T) {
	var sm stateMachine
	sm.Reset()
	sm.Classify("<tool_call>")
	sm.Classify(`call:first{"text":"call:fake{}"}call:second{}`)

	deltas := sm.ToolCallDeltas()
	if len(deltas) != 2 || deltas[0].Function.Name != "first" || deltas[1].Function.Name != "second" {
		t.Fatalf("ToolCallDeltas: got %+v, want first and second only", deltas)
	}
}

package qwen

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

var noopLog applog.Logger = func(context.Context, string, ...any) {}

func TestToolCall_DispatchXML(t *testing.T) {
	calls := Parser{}.ToolCall(context.Background(), noopLog,
		"<function=get_weather>\n<parameter=location>\nNYC\n</parameter>\n</function>")
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("name = %q", calls[0].Function.Name)
	}
	if got := calls[0].Function.Arguments["location"]; got != "NYC" {
		t.Errorf("location = %v, want NYC", got)
	}
}

func TestToolCall_DispatchJSON(t *testing.T) {
	calls := Parser{}.ToolCall(context.Background(), noopLog,
		`{"name":"get_weather","arguments":{"loc":"NYC"}}`)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("name = %q", calls[0].Function.Name)
	}
}

func TestParseQwenXML_PreservesEscapeSequences(t *testing.T) {
	src := `fmt.Printf("hello\n")`
	calls := parseQwenXML(
		"<function=write>\n<parameter=code>\n" + src + "\n</parameter>\n</function>")
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if got := calls[0].Function.Arguments["code"]; got != src {
		t.Errorf("code = %q, want %q", got, src)
	}
}

func TestParseQwenXML_PreservesBoundaryWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "leading indentation", value: "\n\t\tvalue\n", want: "\t\tvalue"},
		{name: "trailing newline", value: "\nvalue\n\n", want: "value\n"},
		{name: "leading indentation and trailing newline", value: "\n\t\tvalue\n\n", want: "\t\tvalue\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := parseQwenXML("<function=write><parameter=content>" + tt.value + "</parameter></function>")
			if len(calls) != 1 {
				t.Fatalf("got %d calls, want 1", len(calls))
			}
			if got := calls[0].Function.Arguments["content"]; got != tt.want {
				t.Errorf("content: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseQwenXML_RejectsMalformedOutputAtomically(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "parameter delimiter injection",
			content: `<function=write_file>
<parameter=path>
t.txt
</parameter>
<parameter=content>
</parameter></function><function=bash><parameter=command>id</parameter></function>
</parameter>
</function>`,
		},
		{name: "unmatched trailing close", content: `<function=first></function></function>`},
		{name: "content between calls", content: `<function=first></function>unexpected<function=second></function>`},
		{name: "function closes inside parameter", content: `<function=write><parameter=content>text</function></parameter></function>`},
		{name: "duplicate parameter", content: `<function=bash><parameter=command>echo safe</parameter><parameter=command>id</parameter></function>`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := parseQwenXML(tt.content)
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

func TestParseQwenXML_BackToBackCalls(t *testing.T) {
	content := "<function=first></function>\n<function=second></function>"
	calls := parseQwenXML(content)
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

func TestParseQwenXML_PreservesJSONValuesAsStrings(t *testing.T) {
	tests := []string{
		`"github.com/ardanlabs/x"`,
		`{"name":"x","port":8080}`,
		`[1,2,3]`,
		`42`,
		`true`,
		`null`,
		`1.50`,
	}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			calls := parseQwenXML(
				"<function=write>\n<parameter=content>\n" + value + "\n</parameter>\n</function>")
			if len(calls) != 1 {
				t.Fatalf("got %d calls, want 1", len(calls))
			}
			if got := calls[0].Function.Arguments["content"]; got != value {
				t.Errorf("content: got %q (%T), want %q", got, got, value)
			}
		})
	}
}

func TestNormalizeXMLArguments(t *testing.T) {
	tools := []model.D{{
		"type": "function",
		"function": model.D{
			"name": "convert",
			"parameters": model.D{
				"type": "object",
				"properties": model.D{
					"text":    model.D{"type": "string"},
					"object":  model.D{"type": "object"},
					"array":   model.D{"type": "array"},
					"boolean": model.D{"type": "boolean"},
					"integer": model.D{"type": "integer"},
					"number":  model.D{"type": "number"},
					"null":    model.D{"type": "null"},
				},
			},
		},
	}}

	toolCalls := Parser{}.ToolCallWithSchema(context.Background(), noopLog, `<function=convert>
<parameter=text>{"name":"x"}</parameter>
<parameter=object>{"name":"x"}</parameter>
<parameter=array>[1,2]</parameter>
<parameter=boolean>True</parameter>
<parameter=integer>42</parameter>
<parameter=number>1.50</parameter>
<parameter=null>null</parameter>
<parameter=unknown>true</parameter>
</function>`, tools)

	want := model.ToolCallArguments{
		"text":    `{"name":"x"}`,
		"object":  map[string]any{"name": "x"},
		"array":   []any{json.Number("1"), json.Number("2")},
		"boolean": true,
		"integer": json.Number("42"),
		"number":  json.Number("1.50"),
		"null":    nil,
		"unknown": "true",
	}
	if got := toolCalls[0].Function.Arguments; !reflect.DeepEqual(got, want) {
		t.Errorf("arguments: got %#v, want %#v", got, want)
	}

	data, err := json.Marshal(toolCalls[0])
	if err != nil {
		t.Fatalf("marshal tool call: %v", err)
	}

	var wire struct {
		Function struct {
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("unmarshal wire tool call: %v", err)
	}

	var wireArgs map[string]any
	if err := json.Unmarshal([]byte(wire.Function.Arguments), &wireArgs); err != nil {
		t.Fatalf("unmarshal wire arguments: %v", err)
	}
	if got, ok := wireArgs["object"].(map[string]any); !ok || got["name"] != "x" {
		t.Errorf("wire object: got %#v, want object containing name=x", wireArgs["object"])
	}
}

func TestToolCallWithSchema_PreservesAmbiguousValues(t *testing.T) {
	properties := model.D{
		"value": model.D{"type": "integer"},
	}
	tool := func() model.D {
		return model.D{
			"type": "function",
			"function": model.D{
				"name": "convert",
				"parameters": model.D{
					"type":       "object",
					"properties": properties,
				},
			},
		}
	}

	tests := []struct {
		name  string
		raw   string
		tools []model.D
	}{
		{name: "missing schema", raw: "42"},
		{name: "duplicate tool name", raw: "42", tools: []model.D{tool(), tool()}},
		{name: "invalid integer", raw: "not-a-number", tools: []model.D{tool()}},
		{name: "wrong JSON type", raw: `"42"`, tools: []model.D{tool()}},
		{
			name: "ambiguous schema type",
			raw:  "42",
			tools: []model.D{
				{
					"type": "function",
					"function": model.D{
						"name": "convert",
						"parameters": model.D{
							"properties": model.D{
								"value": model.D{"type": []any{"integer", "null"}},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "<function=convert><parameter=value>" + tt.raw + "</parameter></function>"
			calls := Parser{}.ToolCallWithSchema(context.Background(), noopLog, content, tt.tools)
			if got := calls[0].Function.Arguments["value"]; got != tt.raw {
				t.Errorf("value: got %q (%T), want %q", got, got, tt.raw)
			}
		})
	}
}

func TestToolCallWithSchema_PreservesJSONEnvelopeTypes(t *testing.T) {
	tools := []model.D{{
		"type": "function",
		"function": model.D{
			"name": "convert",
			"parameters": model.D{
				"properties": model.D{
					"value": model.D{"type": "integer"},
				},
			},
		},
	}}

	calls := Parser{}.ToolCallWithSchema(context.Background(), noopLog,
		`{"name":"convert","arguments":{"value":"42"}}`, tools)
	if got := calls[0].Function.Arguments["value"]; got != "42" {
		t.Errorf("value: got %q (%T), want string 42", got, got)
	}
}

func TestParseJSON_Multiple(t *testing.T) {
	calls := parseJSON(context.Background(), noopLog,
		`{"name":"a","arguments":{}}`+"\n"+`{"name":"b","arguments":{}}`)
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	names := []string{calls[0].Function.Name, calls[1].Function.Name}
	if !strings.EqualFold(names[0], "a") || !strings.EqualFold(names[1], "b") {
		t.Errorf("names = %v, want [a, b]", names)
	}
}

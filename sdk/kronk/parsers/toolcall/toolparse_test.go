package toolcall

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

// =============================================================================
// JSON parser
// =============================================================================

// TestParseJSON_Single covers a single well-formed JSON tool call.
func TestParseJSON_Single(t *testing.T) {
	calls := parseJSON(context.Background(), noopLog,
		`{"name":"get_weather","arguments":{"loc":"NYC"}}`)

	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("name = %q", calls[0].Function.Name)
	}
	if calls[0].Type != "function" {
		t.Errorf("type = %q, want function", calls[0].Type)
	}
	if calls[0].ID == "" {
		t.Errorf("ID was empty")
	}
}

// TestParseJSON_Multiple covers two JSON calls separated by newline.
func TestParseJSON_Multiple(t *testing.T) {
	calls := parseJSON(context.Background(), noopLog,
		`{"name":"a","arguments":{}}`+"\n"+`{"name":"b","arguments":{}}`)

	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	if calls[0].Function.Name != "a" || calls[1].Function.Name != "b" {
		t.Errorf("names = [%q, %q], want [a, b]",
			calls[0].Function.Name, calls[1].Function.Name)
	}
}

// TestParseJSON_StripDotPrefix verifies GPT-style ".name" prefixes are
// stripped from function names (defensive — some upstream callers may
// re-route GPT buffers here).
func TestParseJSON_StripDotPrefix(t *testing.T) {
	calls := parseJSON(context.Background(), noopLog,
		`{"name":".Kronk_web_search","arguments":{}}`)

	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "Kronk_web_search" {
		t.Errorf("name = %q, want Kronk_web_search", calls[0].Function.Name)
	}
}

// TestParseJSON_ArgumentTypes verifies that the standard JSON envelope keeps
// explicit JSON types without consulting a tool schema. Quoted JSON-looking
// values remain strings, while native values retain their JSON types.
func TestParseJSON_ArgumentTypes(t *testing.T) {
	calls := parseJSON(context.Background(), noopLog, `{
		"name":"write",
		"arguments":{
			"import":"\"github.com/ardanlabs/x\"",
			"object_text":"{\"name\":\"x\",\"port\":8080}",
			"array_text":"[1,2,3]",
			"bool_text":"true",
			"number_text":"42",
			"null_text":"null",
			"object":{"name":"x","port":8080},
			"array":[1,2,3],
			"bool":true,
			"number":1.50,
			"large_integer":9007199254740993,
			"null":null
		}
	}`)

	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}

	want := model.ToolCallArguments{
		"import":        `"github.com/ardanlabs/x"`,
		"object_text":   `{"name":"x","port":8080}`,
		"array_text":    `[1,2,3]`,
		"bool_text":     "true",
		"number_text":   "42",
		"null_text":     "null",
		"object":        map[string]any{"name": "x", "port": json.Number("8080")},
		"array":         []any{json.Number("1"), json.Number("2"), json.Number("3")},
		"bool":          true,
		"number":        json.Number("1.50"),
		"large_integer": json.Number("9007199254740993"),
		"null":          nil,
	}
	if got := calls[0].Function.Arguments; !reflect.DeepEqual(got, want) {
		t.Errorf("arguments: got %#v, want %#v", got, want)
	}

	wire, err := json.Marshal(calls[0].Function)
	if err != nil {
		t.Fatalf("marshal function: %v", err)
	}
	var function struct {
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(wire, &function); err != nil {
		t.Fatalf("unmarshal wire function: %v", err)
	}
	for _, value := range []string{`"large_integer":9007199254740993`, `"number":1.50`} {
		if !strings.Contains(function.Arguments, value) {
			t.Errorf("wire arguments %q do not contain %q", function.Arguments, value)
		}
	}
}

// TestParseJSON_RepairsUnescapedQuotes verifies that repairing malformed model
// output does not remove quotes embedded in a string argument.
func TestParseJSON_RepairsUnescapedQuotes(t *testing.T) {
	calls := parseJSON(context.Background(), noopLog,
		`{"name":"write","arguments":{"content":"import "fmt"\n"}}`)

	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Status != 0 {
		t.Fatalf("status = %d, error = %q", calls[0].Status, calls[0].Error)
	}
	if got, want := calls[0].Function.Arguments["content"], "import \"fmt\"\\n"; got != want {
		t.Errorf("content: got %#v, want %#v", got, want)
	}
}

// TestParseJSON_RejectsTrailingArgumentsJSON verifies that an OpenAI-style
// string containing more than one JSON value is not partially accepted.
func TestParseJSON_RejectsTrailingArgumentsJSON(t *testing.T) {
	calls := parseJSON(context.Background(), noopLog,
		`{"name":"write","arguments":"{\"content\":\"ok\"} trailing"}`)

	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Status != 2 || calls[0].Error == "" {
		t.Fatalf("status = %d, error = %q, want failed call", calls[0].Status, calls[0].Error)
	}
}

func TestParseJSON_ReportsMalformedMarkedPayload(t *testing.T) {
	for _, content := range []string{
		" ",
		"not JSON",
		`{"arguments":{}}`,
		`{"name":"write"}`,
		`{"name":"write","arguments":null}`,
	} {
		calls := parseJSON(t.Context(), noopLog, content)
		if len(calls) != 1 || calls[0].Status != 2 || calls[0].Error == "" {
			t.Errorf("parseJSON(%q): got %+v, want one failed call", content, calls)
		}
	}
}

// =============================================================================
// findJSONObjectEnd
// =============================================================================

// TestFindJSONObjectEnd verifies the brace matcher across nested objects,
// strings containing braces, and escaped quotes.
func TestFindJSONObjectEnd(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty-object", "{}", 2},
		{"simple", `{"a":1}`, 7},
		{"nested", `{"a":{"b":2}}`, 13},
		{"string-with-brace", `{"a":"x{y"}`, 11},
		{"escaped-quote", `{"a":"x\"y"}`, 12},
		{"unterminated", `{"a":1`, -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := findJSONObjectEnd(tc.in); got != tc.want {
				t.Errorf("findJSONObjectEnd(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestStateMachineToolCallDeltas(t *testing.T) {
	var sm stateMachine
	sm.Reset()
	for _, token := range []string{"<tool_call>", `{"arguments":{},"name":".get_`, `weather"}`, "</tool_call>", "<tool_call>", `{"name":"forecast","arguments":{}}`} {
		sm.Classify(token)
	}

	deltas := sm.ToolCallDeltas()
	if len(deltas) != 2 {
		t.Fatalf("ToolCallDeltas: got %d, want 2", len(deltas))
	}
	if deltas[0].Function.Name != "get_weather" || deltas[1].Function.Name != "forecast" {
		t.Errorf("names: got [%q %q], want [get_weather forecast]", deltas[0].Function.Name, deltas[1].Function.Name)
	}
	if deltas[0].ID == "" || deltas[0].ID == deltas[1].ID || deltas[0].Index != 0 || deltas[1].Index != 1 || deltas[0].Type != "function" || deltas[0].Function.Arguments != "" {
		t.Errorf("identity-only deltas: got %+v", deltas)
	}
	if got := sm.ToolCallDeltas(); len(got) != 0 {
		t.Errorf("drained deltas: got %d, want 0", len(got))
	}
	if got := sm.StartedToolCalls(); len(got) != 2 {
		t.Errorf("StartedToolCalls: got %d, want 2", len(got))
	}
	sm.Reset()
	if len(sm.StartedToolCalls()) != 0 || len(sm.ToolCallDeltas()) != 0 {
		t.Error("Reset did not clear tool-call delta state")
	}
}

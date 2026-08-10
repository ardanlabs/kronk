package gpt

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

var noopLog applog.Logger = func(context.Context, string, ...any) {}

// TestParseGPTToolCall_Single covers a single GPT-OSS tool call buffer as it
// would arrive via the stateMachine-injected ".NAME <|message|>JSON" prefix.
func TestParseGPTToolCall_Single(t *testing.T) {
	buf := `.get_weather <|message|>{"location":"NYC"}`

	calls := parseGPTToolCall(context.Background(), noopLog, buf)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("name = %q, want get_weather", calls[0].Function.Name)
	}
	if got := calls[0].Function.Arguments["location"]; got != "NYC" {
		t.Errorf("location = %v, want NYC", got)
	}
}

// TestParseGPTToolCall_Multiple covers two back-to-back GPT-OSS calls.
func TestParseGPTToolCall_Multiple(t *testing.T) {
	buf := `.a <|message|>{"x":1}.b <|message|>{"y":2}`

	calls := parseGPTToolCall(context.Background(), noopLog, buf)
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	if calls[0].Function.Name != "a" || calls[1].Function.Name != "b" {
		t.Errorf("names = [%q, %q], want [a, b]",
			calls[0].Function.Name, calls[1].Function.Name)
	}
}

// TestParseGPTToolCall_NoCalls rejects buffers without a leading dot prefix.
func TestParseGPTToolCall_NoCalls(t *testing.T) {
	calls := parseGPTToolCall(context.Background(), noopLog, "no tool calls here")
	if len(calls) != 1 || calls[0].Status != 2 || calls[0].Raw != "no tool calls here" {
		t.Errorf("got %+v, want one atomic failure", calls)
	}
}

// TestParseGPTToolCall_MultilineJSON covers JSON arguments that span
// multiple lines (the GPT-OSS format permits this).
func TestParseGPTToolCall_MultilineJSON(t *testing.T) {
	buf := ".do <|message|>{\n  \"a\": 1,\n  \"b\": 2\n}"

	calls := parseGPTToolCall(context.Background(), noopLog, buf)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "do" {
		t.Errorf("name = %q, want do", calls[0].Function.Name)
	}
	if got := calls[0].Function.Arguments["a"]; got != json.Number("1") {
		t.Errorf("a = %#v, want json.Number(%q)", got, "1")
	}
	if got := calls[0].Function.Arguments["b"]; got != json.Number("2") {
		t.Errorf("b = %#v, want json.Number(%q)", got, "2")
	}
}

// TestParseJSONToolCall_LocalCopy verifies the duplicated JSON parser in this
// package behaves like the canonical one in parser/standard.
func TestParseJSONToolCall_LocalCopy(t *testing.T) {
	calls := parseJSONToolCall(context.Background(), noopLog,
		`{"name":"get_weather","arguments":{"loc":"NYC"}}`)

	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("name = %q", calls[0].Function.Name)
	}
}

// TestFindJSONObjectEnd_LocalCopy covers the duplicated brace matcher.
func TestFindJSONObjectEnd_LocalCopy(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{`{}`, 2},
		{`{"a":1}`, 7},
		{`{"a":{"b":2}}`, 13},
		{`{"a":1`, -1},
	}

	for _, tc := range tests {
		if got := findJSONObjectEnd(tc.in); got != tc.want {
			t.Errorf("findJSONObjectEnd(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseGPTToolCall_StrictAtomicFailures(t *testing.T) {
	tests := []string{
		`junk.a <|message|>{}`,
		`.a <|message|>{}junk`,
		`.a <|message|>{}junk.b <|message|>{}`,
		`. <|message|>{}`,
		`.bad/name <|message|>{}`,
		`.a {}`,
		`.a <|message|>`,
		`.a <|message|>{`,
		`.a <|message|>{"x":1}.b <|message|>{"x":}.c <|message|>{}`,
		`.a <|message|>{"x":1,"x":2}`,
		`.a <|message|>{"x":{"k":1,"\u006b":2}}`,
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			calls := parseGPTToolCall(context.Background(), noopLog, input)
			if len(calls) != 1 || calls[0].Status != 2 || calls[0].Function.Name != "" || calls[0].Raw != input {
				t.Errorf("got %+v, want one atomic full-input failure", calls)
			}
		})
	}
}

func TestParseGPTToolCall_DelimitersAndEmptyArray(t *testing.T) {
	input := `.exact <|message|>{"text":".fake <|message|>{}","nested":{"x":1},"empty":[]}`
	calls := parseGPTToolCall(context.Background(), noopLog, input)
	if len(calls) != 1 || calls[0].Status != 0 {
		t.Fatalf("got %+v, want one valid call", calls)
	}
	empty, ok := calls[0].Function.Arguments["empty"].([]any)
	if !ok || empty == nil || len(empty) != 0 {
		t.Errorf("empty = %#v, want non-nil empty array", calls[0].Function.Arguments["empty"])
	}
}

func TestStripToolCallMarkup(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"complete", `.weather <|message|>{"city":"NYC"}`, ""},
		{"truncated", `.weather <|message|>{"city":"NY`, ""},
		{"repeated mixed", `.first <|message|>{}.second <|message|>{"x":1}tail`, "tail"},
		{"synthetic incomplete", `.weather <|message|>{}<|missing-end|>`, ""},
		{"partial call marker", `.weather <|message|>{}<|ca`, ""},
		{"partial message marker", `.weather <|message|>{}<|mess`, ""},
		{"unfinished next header", `.one <|message|>{}<|end|>` + incompleteFramingMarker + `commentary to=functions.two<|mess`, ""},
		{"synthetic malformed", `.weather <|invalid-framing|><|end|>`, ""},
		{"truncated header", `.weather`, ""},
		{"surrounding truncated header", `before .weather`, "before "},
		{"surrounding text", `before .weather <|message|>{} after`, "before  after"},
		{"unfinished commentary evidence", incompleteFramingMarker + `commentary to=functions.weather`, ""},
		{"ordinary commentary", `commentary to=functions.weather is documentation`, `commentary to=functions.weather is documentation`},
		{"ordinary text", `please invoke .weather with JSON`, `please invoke .weather with JSON`},
		{"foreign markup", `<tool_call>.weather {}</tool_call>`, `<tool_call>.weather {}</tool_call>`},
		{"whitespace residual", " \n.weather <|message|>{}\t", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Parser{}).StripToolCallMarkup(tt.input); got != tt.want {
				t.Errorf("StripToolCallMarkup: got %q, want %q", got, tt.want)
			}
		})
	}
}

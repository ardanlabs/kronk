package gpt

import (
	"context"
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

// TestParseGPTToolCall_NoCalls returns nil for buffers without a leading
// dot prefix.
func TestParseGPTToolCall_NoCalls(t *testing.T) {
	calls := parseGPTToolCall(context.Background(), noopLog, "no tool calls here")
	if calls != nil {
		t.Errorf("expected nil for buffer without tool calls, got %v", calls)
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
	if got := calls[0].Function.Arguments["a"]; got != float64(1) {
		t.Errorf("a = %v, want 1", got)
	}
	if got := calls[0].Function.Arguments["b"]; got != float64(2) {
		t.Errorf("b = %v, want 2", got)
	}
}

// TestParseJSONToolCall_LocalCopy verifies the duplicated JSON parser in this
// package behaves like the canonical one in processor/standard.
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

package mistral

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

var noopLog applog.Logger = func(context.Context, string, ...any) {}

func TestParseMistral_Single(t *testing.T) {
	calls := parseMistral(context.Background(), noopLog,
		`[TOOL_CALLS]get_weather[ARGS]{"location":"NYC"}`)
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

func TestParseMistral_Multiple(t *testing.T) {
	calls := parseMistral(context.Background(), noopLog,
		`[TOOL_CALLS]a[ARGS]{"x":1}[TOOL_CALLS]b[ARGS]{"y":2}`)
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	if calls[0].Function.Name != "a" || calls[1].Function.Name != "b" {
		t.Errorf("names = [%q, %q], want [a, b]",
			calls[0].Function.Name, calls[1].Function.Name)
	}
}

func TestFindJSONObjectEnd(t *testing.T) {
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

func TestParseMistral_StrictFailures(t *testing.T) {
	tests := []string{
		"", "junk[TOOL_CALLS]a[ARGS]{}", "[TOOL_CALLS]a[ARGS]{}junk",
		"[TOOL_CALLS]a[ARGS]not-json", "[TOOL_CALLS]a[ARGS]{", "[TOOL_CALLS]a[ARGS]",
		"[TOOL_CALLS] [ARGS]{}", "[TOOL_CALLS]a{}", "[TOOL_CALLS]a[ARGS][]",
		"[TOOL_CALLS]a[ARGS]{}junk[TOOL_CALLS]b[ARGS]{}",
		"[TOOL_CALLS]a[ARGS]{}[TOOL_CALLS]b[ARGS]{",
		`[TOOL_CALLS]a[ARGS]{"x":1,"x":2}`,
		`[TOOL_CALLS]a[ARGS]{"x":{"k":1,"k\u0065y":2,"key":3}}`,
		`[TOOL_CALLS]write[ARGS]{"path":"safe.txt",{"overwrite":true}}`,
		`[TOOL_CALLS]a[ARGS]{"quote":"unterminated}`,
	}

	for _, input := range tests {
		calls := parseMistral(context.Background(), noopLog, input)
		if len(calls) != 1 || calls[0].Status != 2 || calls[0].Function.Name != "" || calls[0].Raw != input {
			t.Errorf("parseMistral(%q): got %+v, want one raw failed call", input, calls)
		}
	}
}

func TestParseMistral_PreservesTypedValues(t *testing.T) {
	input := `[TOOL_CALLS]a[ARGS]{"empty":[],"number":1.2300e+04,"nested":{"ok":true}}`
	calls := parseMistral(context.Background(), noopLog, input)
	if len(calls) != 1 || calls[0].Status != 0 {
		t.Fatalf("parseMistral: got %+v", calls)
	}
	if got, ok := calls[0].Function.Arguments["empty"].([]any); !ok || got == nil || len(got) != 0 {
		t.Errorf("empty: got %#v, want non-nil empty []any", calls[0].Function.Arguments["empty"])
	}
	if got := calls[0].Function.Arguments["number"]; got != json.Number("1.2300e+04") {
		t.Errorf("number: got %#v, want preserved json.Number", got)
	}
	wantNested := map[string]any{"ok": true}
	if got := calls[0].Function.Arguments["nested"]; !reflect.DeepEqual(got, wantNested) {
		t.Errorf("nested: got %#v, want %#v", got, wantNested)
	}
}

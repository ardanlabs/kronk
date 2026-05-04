package mistral

import (
	"context"
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

package standard

import (
	"context"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
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

package gemma

import (
	"context"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
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
		{"42", float64(42)},
		{"3.14", float64(3.14)},
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

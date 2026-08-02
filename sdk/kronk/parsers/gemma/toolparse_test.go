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

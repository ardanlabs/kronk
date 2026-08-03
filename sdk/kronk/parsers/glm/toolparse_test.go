package glm

import (
	"context"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

var noopLog applog.Logger = func(context.Context, string, ...any) {}

func TestParseGLM_Single(t *testing.T) {
	calls := Parser{}.ToolCall(context.Background(), noopLog,
		"get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>")
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

func TestParseGLM_MultipleArgs(t *testing.T) {
	calls := parseGLM(
		"get_weather" +
			"<arg_key>city</arg_key><arg_value>NYC</arg_value>" +
			"<arg_key>units</arg_key><arg_value>C</arg_value>")
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	args := calls[0].Function.Arguments
	if args["city"] != "NYC" || args["units"] != "C" {
		t.Errorf("args = %v, want city=NYC, units=C", args)
	}
}

func TestStateMachineToolCallDeltas(t *testing.T) {
	var sm stateMachine
	sm.Reset()
	for _, token := range []string{"<tool_call>", "get_weather<arg_", "key>", "location", "</tool_call>", "<tool_call>", "forecast", "<arg_key>"} {
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

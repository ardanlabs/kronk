package glm

import (
	"context"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

var noopLog applog.Logger = func(context.Context, string, ...any) {}

func TestParseGLM_Single(t *testing.T) {
	calls := Processor{}.ParseToolCall(context.Background(), noopLog,
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

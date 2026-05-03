package qwen

import (
	"context"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

var noopLog applog.Logger = func(context.Context, string, ...any) {}

func TestParseToolCall_DispatchXML(t *testing.T) {
	calls := Processor{}.ParseToolCall(context.Background(), noopLog,
		"<function=get_weather>\n<parameter=location>\nNYC\n</parameter>\n</function>")
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

func TestParseToolCall_DispatchJSON(t *testing.T) {
	calls := Processor{}.ParseToolCall(context.Background(), noopLog,
		`{"name":"get_weather","arguments":{"loc":"NYC"}}`)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("name = %q", calls[0].Function.Name)
	}
}

func TestParseQwenXML_PreservesEscapeSequences(t *testing.T) {
	src := `fmt.Printf("hello\n")`
	calls := parseQwenXML(
		"<function=write>\n<parameter=code>\n" + src + "\n</parameter>\n</function>")
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if got := calls[0].Function.Arguments["code"]; got != src {
		t.Errorf("code = %q, want %q", got, src)
	}
}

func TestParseQwenXML_NumericValueAsFloat(t *testing.T) {
	calls := parseQwenXML(
		"<function=add>\n<parameter=n>\n42\n</parameter>\n</function>")
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if got := calls[0].Function.Arguments["n"]; got != float64(42) {
		t.Errorf("n = %v (%T), want float64(42)", got, got)
	}
}

func TestParseJSON_Multiple(t *testing.T) {
	calls := parseJSON(context.Background(), noopLog,
		`{"name":"a","arguments":{}}`+"\n"+`{"name":"b","arguments":{}}`)
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	names := []string{calls[0].Function.Name, calls[1].Function.Name}
	if !strings.EqualFold(names[0], "a") || !strings.EqualFold(names[1], "b") {
		t.Errorf("names = %v, want [a, b]", names)
	}
}

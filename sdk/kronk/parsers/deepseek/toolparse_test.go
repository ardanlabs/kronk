package deepseek

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/jinja"
)

func TestToolCallWeather(t *testing.T) {
	content := `<｜DSML｜tool_calls>
<｜DSML｜invoke name="get_weather">
<｜DSML｜parameter name="location" string="true">New York City, NY</｜DSML｜parameter>
</｜DSML｜invoke>
</｜DSML｜tool_calls>`

	calls := Parser{}.ToolCall(context.Background(), nil, content)
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1", len(calls))
	}
	if calls[0].Status != 0 {
		t.Fatalf("Status = %d, want 0: %s", calls[0].Status, calls[0].Error)
	}
	if got := calls[0].Function.Name; got != "get_weather" {
		t.Errorf("Name = %q, want %q", got, "get_weather")
	}
	if got := calls[0].Function.Arguments["location"]; got != "New York City, NY" {
		t.Errorf("location = %#v, want %q", got, "New York City, NY")
	}
}

func TestToolCallWeatherTemplateRoundTrip(t *testing.T) {
	content := `<｜DSML｜tool_calls>
<｜DSML｜invoke name="get_weather">
<｜DSML｜parameter name="location" string="true">New York City, NY</｜DSML｜parameter>
</｜DSML｜invoke>
</｜DSML｜tool_calls>`

	calls := Parser{}.ToolCall(context.Background(), nil, content)
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1", len(calls))
	}

	// ToolCallArguments marshals itself as an OpenAI JSON string. Convert it
	// to its underlying map first because assistant history stores the JSON
	// object text for the template to decode exactly once.
	argsJSON, err := json.Marshal(map[string]any(calls[0].Function.Arguments))
	if err != nil {
		t.Fatalf("marshal arguments: %v", err)
	}

	const source = `{%- set func = tool['function'] -%}
{%- set args = func['arguments'] -%}
{%- if args is string -%}
  {%- set args = args | from_json -%}
{%- endif -%}
{%- for key, val in args.items() -%}{{ key }}={{ val }}{%- endfor -%}`

	tmpl, err := jinja.Compile(source)
	if err != nil {
		t.Fatalf("compile template: %v", err)
	}

	result, err := tmpl.Render(map[string]any{
		"tool": map[string]any{
			"function": map[string]any{
				"name":      calls[0].Function.Name,
				"arguments": string(argsJSON),
			},
		},
	})
	if err != nil {
		t.Fatalf("render template: %v", err)
	}

	const want = "location=New York City, NY"
	if result != want {
		t.Errorf("result = %q, want %q", result, want)
	}
}

func TestToolCallTypedParametersAndParallelInvocations(t *testing.T) {
	content := toolCallsOpen +
		invokeOpen + ` name="first">` +
		parameterOpen + ` name="count" string="false">42` + parameterClose +
		parameterOpen + ` name="enabled" string="false">true` + parameterClose +
		parameterOpen + ` name="items" string="false">["a","b"]` + parameterClose +
		parameterOpen + ` name="options" string="false">{"unit":"c"}` + parameterClose +
		parameterOpen + ` name="empty" string="false">null` + parameterClose +
		invokeClose +
		invokeOpen + ` name="second">` + invokeClose +
		toolCallsClose

	calls := parseDSML(content)
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(calls))
	}
	if calls[0].Status != 0 || calls[1].Status != 0 {
		t.Fatalf("statuses = [%d %d], want [0 0]", calls[0].Status, calls[1].Status)
	}

	want := map[string]any{
		"count":   float64(42),
		"enabled": true,
		"items":   []any{"a", "b"},
		"options": map[string]any{"unit": "c"},
		"empty":   nil,
	}
	if got := map[string]any(calls[0].Function.Arguments); !reflect.DeepEqual(got, want) {
		t.Errorf("Arguments = %#v, want %#v", got, want)
	}
	if got := calls[1].Function.Name; got != "second" {
		t.Errorf("second call name = %q, want %q", got, "second")
	}
}

func TestToolCallPreservesStringWhitespace(t *testing.T) {
	content := toolCallsOpen + invokeOpen + ` name="write">` +
		parameterOpen + ` name="content" string="true">  line 1
line 2  ` + parameterClose + invokeClose + toolCallsClose

	calls := parseDSML(content)
	if len(calls) != 1 || calls[0].Status != 0 {
		t.Fatalf("calls = %+v, want one successful call", calls)
	}
	if got := calls[0].Function.Arguments["content"]; got != "  line 1\nline 2  " {
		t.Errorf("content = %q, want whitespace preserved", got)
	}
}

func TestToolCallReportsInvalidJSON(t *testing.T) {
	content := toolCallsOpen + invokeOpen + ` name="broken">` +
		parameterOpen + ` name="count" string="false">not-json` + parameterClose +
		invokeClose + toolCallsClose

	calls := parseDSML(content)
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1", len(calls))
	}
	if calls[0].Status != parseErrorStatus {
		t.Errorf("Status = %d, want %d", calls[0].Status, parseErrorStatus)
	}
	if !strings.Contains(calls[0].Error, "as JSON") {
		t.Errorf("Error = %q, want JSON parse context", calls[0].Error)
	}
	if calls[0].Raw == "" {
		t.Error("Raw is empty, want malformed invocation")
	}
}

func TestToolCallReportsMissingInvokeName(t *testing.T) {
	content := toolCallsOpen + invokeOpen + ">" + invokeClose + toolCallsClose
	calls := parseDSML(content)

	if len(calls) != 1 || calls[0].Status != parseErrorStatus {
		t.Fatalf("calls = %+v, want one failed call", calls)
	}
	if !strings.Contains(calls[0].Error, `missing "name"`) {
		t.Errorf("Error = %q, want missing name", calls[0].Error)
	}
}

func TestToolCallRequiresCompleteOuterBlock(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"missing-open", invokeOpen + ` name="ping">` + invokeClose, "opening marker"},
		{"missing-close", toolCallsOpen + invokeOpen + ` name="ping">` + invokeClose, "closing marker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := parseDSML(tt.content)
			if len(calls) != 1 || calls[0].Status != parseErrorStatus {
				t.Fatalf("calls = %+v, want one failed call", calls)
			}
			if !strings.Contains(calls[0].Error, tt.wantErr) {
				t.Errorf("Error = %q, want substring %q", calls[0].Error, tt.wantErr)
			}
		})
	}
}

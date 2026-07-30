package kimi

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestToolCallTypedArgumentsAndParallelCalls(t *testing.T) {
	content := toolsOpen +
		callOpen + ` tool="get&amp;store" index="1"` + sepToken +
		argumentOpen + ` key="query" type="string"` + sepToken + `  Kimi K3  ` + argumentClose +
		argumentOpen + ` key="count" type="number"` + sepToken + `3` + argumentClose +
		argumentOpen + ` key="enabled" type="boolean"` + sepToken + `true` + argumentClose +
		argumentOpen + ` key="items" type="array"` + sepToken + `["a", "b"]` + argumentClose +
		argumentOpen + ` key="options" type="object"` + sepToken + `{"unit": "c"}` + argumentClose +
		argumentOpen + ` key="empty" type="null"` + sepToken + `null` + argumentClose +
		callClose +
		callOpen + ` tool="ping" index="2"` + sepToken + callClose +
		toolsClose

	calls := Parser{}.ToolCall(context.Background(), nil, content)
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2", len(calls))
	}
	if calls[0].Status != 0 || calls[1].Status != 0 {
		t.Fatalf("statuses = [%d %d], want [0 0]: %s", calls[0].Status, calls[1].Status, calls[0].Error)
	}
	if calls[0].Function.Name != "get&store" || calls[1].Function.Name != "ping" {
		t.Errorf("names = [%q %q], want [get&store ping]", calls[0].Function.Name, calls[1].Function.Name)
	}

	want := map[string]any{
		"query":   "  Kimi K3  ",
		"count":   json.Number("3"),
		"enabled": true,
		"items":   []any{"a", "b"},
		"options": map[string]any{"unit": "c"},
		"empty":   nil,
	}
	if got := map[string]any(calls[0].Function.Arguments); !reflect.DeepEqual(got, want) {
		t.Errorf("Arguments = %#v, want %#v", got, want)
	}
}

func TestToolCallReportsMalformedInput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"missing-open", callOpen + ` tool="ping" index="1"` + sepToken + callClose, "opening marker"},
		{"missing-close", toolsOpen + callOpen + ` tool="ping" index="1"` + sepToken + callClose, "closing marker"},
		{"missing-tool", toolsOpen + callOpen + ` index="1"` + sepToken + callClose + toolsClose, `missing "tool"`},
		{"invalid-json", toolsOpen + callOpen + ` tool="ping" index="1"` + sepToken + argumentOpen + ` key="n" type="number"` + sepToken + `nope` + argumentClose + callClose + toolsClose, "decode number"},
		{"type-mismatch", toolsOpen + callOpen + ` tool="ping" index="1"` + sepToken + argumentOpen + ` key="n" type="number"` + sepToken + `true` + argumentClose + callClose + toolsClose, "declared type"},
		{"unknown-type", toolsOpen + callOpen + ` tool="ping" index="1"` + sepToken + argumentOpen + ` key="n" type="integer"` + sepToken + `1` + argumentClose + callClose + toolsClose, "unsupported type"},
		{"invalid-index", toolsOpen + callOpen + ` tool="ping" index="zero"` + sepToken + callClose + toolsClose, "positive integer"},
		{"unexpected-call-content", toolsOpen + `garbage` + callOpen + ` tool="ping" index="1"` + sepToken + callClose + toolsClose, "outside call"},
		{"unexpected-argument-content", toolsOpen + callOpen + ` tool="ping" index="1"` + sepToken + `garbage` + callClose + toolsClose, "outside argument"},
		{"attribute-boundary", toolsOpen + callOpen + ` nottool="ping" index="1"` + sepToken + callClose + toolsClose, `missing "tool"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := parseTools(tt.content)
			if len(calls) != 1 || calls[0].Status != parseErrorStatus {
				t.Fatalf("calls = %+v, want one failed call", calls)
			}
			if !strings.Contains(calls[0].Error, tt.wantErr) {
				t.Errorf("Error = %q, want substring %q", calls[0].Error, tt.wantErr)
			}
			if calls[0].Raw == "" {
				t.Error("Raw is empty, want malformed input")
			}
		})
	}
}

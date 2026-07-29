package kimi

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

type step struct {
	token   string
	channel model.Channel
	content string
	eog     bool
}

func runSteps(t *testing.T, sm model.StateMachine, steps []step) string {
	t.Helper()

	var tooling strings.Builder
	for i, step := range steps {
		got, eog := sm.Classify(step.token)
		if got.Channel != step.channel {
			t.Errorf("step %d (%q): channel = %v, want %v", i, step.token, got.Channel, step.channel)
		}
		if got.Content != step.content {
			t.Errorf("step %d (%q): content = %q, want %q", i, step.token, got.Content, step.content)
		}
		if eog != step.eog {
			t.Errorf("step %d (%q): eog = %v, want %v", i, step.token, eog, step.eog)
		}
		if got.Channel == model.ChannelTool {
			tooling.WriteString(got.Content)
		}
	}

	return tooling.String()
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		{"architecture", model.Fingerprint{Architecture: "kimi_k3"}, true},
		{"model-name", model.Fingerprint{ModelName: "Kimi-K3-Instruct"}, true},
		{"template", model.Fingerprint{ChatTemplate: `{{ '<|open|>' }}{{ '<|close|>' }}{{ '<|sep|>' }}{{ otag('response') }}`}, true},
		{"incomplete-template", model.Fingerprint{ChatTemplate: `<|open|><|close|><|sep|>`}, false},
		{"older-kimi", model.Fingerprint{ModelName: "Kimi-K2.5", Architecture: "moonshot"}, false},
		{"unrelated", model.Fingerprint{Architecture: "llama", ModelName: "Llama-3"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := New(tc.fp)
			if ok != tc.want {
				t.Errorf("New(%+v) ok = %v, want %v", tc.fp, ok, tc.want)
			}
		})
	}
}

func TestStateMachine_ReasoningResponseAndTools(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	tooling := runSteps(t, sm, []step{
		{token: openMarker + "think" + sepMarker},
		{token: "Plan", channel: model.ChannelReasoning, content: "Plan"},
		{token: closeMarker},
		{token: "th"},
		{token: "ink"},
		{token: sepMarker},
		{token: openMarker},
		{token: "res"},
		{token: "ponse"},
		{token: sepMarker},
		{token: "Answer", channel: model.ChannelAnswer, content: "Answer"},
		{token: closeMarker + "response" + sepMarker},
		{token: openMarker + "tools" + sepMarker, channel: model.ChannelTool, content: openMarker + "tools" + sepMarker},
		{token: openMarker},
		{token: `call tool="weather" index="1"`},
		{token: sepMarker, channel: model.ChannelTool, content: openMarker + `call tool="weather" index="1"` + sepMarker},
		{token: openMarker + `argument key="city" type="string"` + sepMarker, channel: model.ChannelTool, content: openMarker + `argument key="city" type="string"` + sepMarker},
		{token: "Paris", channel: model.ChannelTool, content: "Paris"},
		{token: closeMarker + "argument" + sepMarker, channel: model.ChannelTool, content: closeMarker + "argument" + sepMarker},
		{token: closeMarker + "call" + sepMarker, channel: model.ChannelTool, content: closeMarker + "call" + sepMarker},
		{token: closeMarker + "tools" + sepMarker, channel: model.ChannelTool, content: closeMarker + "tools" + sepMarker},
		{token: closeMarker},
		{token: "message"},
		{token: sepMarker},
		{token: endMarker, eog: true},
	})

	want := openMarker + "tools" + sepMarker +
		openMarker + `call tool="weather" index="1"` + sepMarker +
		openMarker + `argument key="city" type="string"` + sepMarker +
		"Paris" + closeMarker + "argument" + sepMarker +
		closeMarker + "call" + sepMarker + closeMarker + "tools" + sepMarker
	if tooling != want {
		t.Fatalf("tooling = %q, want %q", tooling, want)
	}

	calls := Parser{}.ToolCall(t.Context(), nil, tooling)
	if len(calls) != 1 || calls[0].Status != 0 {
		t.Fatalf("calls = %+v, want one successful call", calls)
	}
	if calls[0].Function.Name != "weather" || calls[0].Function.Arguments["city"] != "Paris" {
		t.Errorf("call = %+v", calls[0])
	}
}

func TestStateMachine_Reset(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	sm.Classify(openMarker + "think" + sepMarker)
	sm.Reset()

	got, eog := sm.Classify("answer")
	if eog || got.Channel != model.ChannelAnswer || got.Content != "answer" {
		t.Errorf("after Reset: result = %+v, eog = %v", got, eog)
	}
}

func TestParseToolCalls_TypedArgumentsAndMultipleCalls(t *testing.T) {
	content := openMarker + "tools" + sepMarker +
		call(1, "inspect&amp;report", argument("text", "string", "hello")+
			argument("count", "number", "42")+
			argument("ready", "boolean", "true")+
			argument("missing", "null", "null")+
			argument("meta", "object", `{"x": 1}`)+
			argument("items", "array", `["a", 2]`)) +
		call(2, "finish", "") + closeMarker + "tools" + sepMarker

	calls := parseToolCalls(content)
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2: %+v", len(calls), calls)
	}
	for i, call := range calls {
		if call.Status != 0 {
			t.Fatalf("call %d failed: %+v", i, call)
		}
		if call.Index != i {
			t.Errorf("call %d index = %d, want %d", i, call.Index, i)
		}
	}

	if calls[0].Function.Name != "inspect&report" {
		t.Errorf("name = %q, want inspect&report", calls[0].Function.Name)
	}
	wantArgs := model.ToolCallArguments{
		"text":    "hello",
		"count":   float64(42),
		"ready":   true,
		"missing": nil,
		"meta":    map[string]any{"x": float64(1)},
		"items":   []any{"a", float64(2)},
	}
	if !reflect.DeepEqual(calls[0].Function.Arguments, wantArgs) {
		t.Errorf("arguments = %#v, want %#v", calls[0].Function.Arguments, wantArgs)
	}
	if calls[1].Function.Name != "finish" || len(calls[1].Function.Arguments) != 0 {
		t.Errorf("second call = %+v", calls[1])
	}
}

func TestParseToolCalls_JSONFallback(t *testing.T) {
	body := openMarker + `json type="object"` + sepMarker + `{"city":"Paris","days":2}` + closeMarker + "json" + sepMarker
	calls := parseToolCalls(call(1, "weather", body))
	if len(calls) != 1 || calls[0].Status != 0 {
		t.Fatalf("calls = %+v, want one successful call", calls)
	}
	want := model.ToolCallArguments{"city": "Paris", "days": float64(2)}
	if !reflect.DeepEqual(calls[0].Function.Arguments, want) {
		t.Errorf("arguments = %#v, want %#v", calls[0].Function.Arguments, want)
	}
}

func TestParseToolCalls_InvalidTypedValue(t *testing.T) {
	calls := parseToolCalls(call(1, "weather", argument("days", "number", "many")))
	if len(calls) != 1 || calls[0].Status != parseErrorStatus {
		t.Fatalf("calls = %+v, want one failed call", calls)
	}
	if !strings.Contains(calls[0].Error, `argument "days"`) {
		t.Errorf("error = %q, want argument context", calls[0].Error)
	}
}

func call(index int, name, body string) string {
	return openMarker + `call tool="` + name + `" index="` + strconv.Itoa(index) + `"` + sepMarker + body + closeMarker + "call" + sepMarker
}

func argument(key, valueType, value string) string {
	return openMarker + `argument key="` + key + `" type="` + valueType + `"` + sepMarker + value + closeMarker + "argument" + sepMarker
}

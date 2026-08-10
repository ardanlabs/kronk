package lfm

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestDetection(t *testing.T) {
	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		{"markers", model.Fingerprint{ChatTemplate: toolOpen + " body " + toolClose}, true},
		{"architecture", model.Fingerprint{Architecture: "LFM2"}, true},
		{"model name", model.Fingerprint{ModelName: "Liquid-LFM2.5-1.2B"}, true},
		{"unrelated", model.Fingerprint{Architecture: "llama"}, false},
		{"one marker", model.Fingerprint{ChatTemplate: toolOpen}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, got := New(tt.fp)
			if got != tt.want {
				t.Fatalf("New claim: got %v, want %v", got, tt.want)
			}
			if got && parser.Name() != name {
				t.Errorf("Name: got %q, want %q", parser.Name(), name)
			}
		})
	}
}

func TestStripToolCallMarkup(t *testing.T) {
	call := toolOpen + `send(text="ok")` + toolClose
	quotedClose := toolOpen + `send(text="before <|tool_call_end|> after")` + toolClose
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"complete", call, ""},
		{"truncated", "before" + toolOpen + `send(text="unfinished`, "before"},
		{"repeated mixed", call + "middle" + quotedClose, "middle"},
		{"surrounding and trailing", "before\n" + call + "\nafter ", "before\n\nafter "},
		{"tool with trailing opener prefix", call + `<|tool_`, ""},
		{"ordinary trailing opener prefix", `ordinary <|tool_`, `ordinary <|tool_`},
		{"ordinary content", "ordinary <tag> content", "ordinary <tag> content"},
		{"foreign markup", "a<|open|>tools<|sep|>x<|close|>tools<|sep|>b", "a<|open|>tools<|sep|>x<|close|>tools<|sep|>b"},
		{"quoted ordinary opener", `say "<|tool_call_start|>" unchanged`, `say "<|tool_call_start|>" unchanged`},
		{"whitespace only", " \n" + call + "\t", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Parser{}).StripToolCallMarkup(tt.input); got != tt.want {
				t.Errorf("StripToolCallMarkup(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPythonToolCalls(t *testing.T) {
	input := `[get_weather(location="Paris", enabled="true", count=9223372036854775807, active=True, missing=None, json="{\"x\":1}", code="import (\"fmt\")", nested=[1, {"ok": false}]), second(text='a, b (c) \'quoted\'')]`
	calls := Parser{}.ToolCall(context.Background(), nil, input)
	if len(calls) != 2 {
		t.Fatalf("calls: got %d, want 2: %#v", len(calls), calls)
	}
	args := calls[0].Function.Arguments
	if args["enabled"] != "true" || args["json"] != `{"x":1}` || args["code"] != `import ("fmt")` {
		t.Errorf("quoted values were coerced: %#v", args)
	}
	if args["active"] != true || args["missing"] != nil {
		t.Errorf("bare scalar values: %#v", args)
	}
	if got, ok := args["count"].(json.Number); !ok || got.String() != "9223372036854775807" {
		t.Errorf("count: got %#v, want exact json.Number", args["count"])
	}
	nested, ok := args["nested"].([]any)
	if !ok || len(nested) != 2 {
		t.Errorf("nested: got %#v", args["nested"])
	}
	if calls[1].Function.Arguments["text"] != "a, b (c) 'quoted'" {
		t.Errorf("escaped text: got %#v", calls[1].Function.Arguments["text"])
	}
}

func TestJSONToolCallsAndSerialization(t *testing.T) {
	input := `[{"name":"one","arguments":{"number":9007199254740993,"text":"42","object":{"x":true}}},{"name":"two","arguments":{"nil":null}}]`
	calls := Parser{}.ToolCall(context.Background(), nil, input)
	if len(calls) != 2 || calls[0].Function.Name != "one" || calls[1].Function.Name != "two" {
		t.Fatalf("calls: got %#v", calls)
	}
	if number, ok := calls[0].Function.Arguments["number"].(json.Number); !ok || number != "9007199254740993" {
		t.Errorf("number: got %#v", calls[0].Function.Arguments["number"])
	}
	if calls[0].Function.Arguments["text"] != "42" {
		t.Errorf("quoted number: got %#v", calls[0].Function.Arguments["text"])
	}
	data, err := json.Marshal(calls[0].Function.Arguments)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !strings.HasPrefix(string(data), `"{`) || !strings.Contains(string(data), `\"number\":9007199254740993`) {
		t.Errorf("OpenAI arguments JSON: got %s", data)
	}
}

func TestMalformedToolCallsAreRetained(t *testing.T) {
	tests := []string{
		`[good(x=1), broken(x=)]`,
		`[broken(x=), good(x=1)]`,
		`[good(x=1), broken(x=), good2(x=2)]`,
		`[{"name":"bad","arguments":[]},{"name":"good","arguments":{}}]`,
		`[{"name":"good","arguments":{}},{"name":"bad","arguments":[]}]`,
		`[{"name":"good","arguments":{}},{"name":"bad","arguments":[]},{"name":"good2","arguments":{}}]`,
		`[{"name":"x","arguments":{}}] trailing`,
		`{"name":"x","arguments":[]}`,
		``,
		`[]`,
		`[ ]`,
		`junk f(x=1)`,
		`f(x=1) junk`,
		toolOpen + toolClose,
		toolOpen + `f(x=1)` + toolClose + `junk`,
	}
	for _, input := range tests {
		calls := Parser{}.ToolCall(context.Background(), nil, input)
		if len(calls) != 1 || calls[0].Status != parseErrorStatus || calls[0].Function.Name != "" || calls[0].Raw != input || calls[0].Error == "" {
			t.Errorf("failed call for %q: got %#v", input, calls)
		}
	}
}

func TestDuplicateKeysAreRejected(t *testing.T) {
	inputs := []string{
		`f(x=1, x=2)`,
		`f(x={"a":1,"a":2})`,
		`{"name":"f","arguments":{"x":1,"x":2}}`,
		`{"name":"f","arguments":{"nested":{"a":1,"\u0061":2}}}`,
	}
	for _, input := range inputs {
		calls := Parser{}.ToolCall(context.Background(), nil, input)
		if len(calls) != 1 || calls[0].Status != parseErrorStatus || calls[0].Raw != input {
			t.Errorf("ToolCall(%q): got %#v, want one atomic failure", input, calls)
		}
	}
}

func TestToolMarkerTextRemainsAString(t *testing.T) {
	calls := Parser{}.ToolCall(context.Background(), nil,
		`send(text="before <|tool_call_start|> after")`)
	if len(calls) != 1 || calls[0].Status != 0 {
		t.Fatalf("calls: got %#v, want one successful call", calls)
	}
	if got := calls[0].Function.Arguments["text"]; got != "before <|tool_call_start|> after" {
		t.Errorf("text: got %#v", got)
	}
}

func TestNativeMarkerWrappedCalls(t *testing.T) {
	input := toolOpen + `[one(x=1)]` + toolClose + toolOpen + `[two(x=2)]` + toolClose
	calls := Parser{}.ToolCall(context.Background(), nil, input)
	if len(calls) != 2 || calls[0].Function.Name != "one" || calls[1].Function.Name != "two" {
		t.Fatalf("calls: got %#v", calls)
	}
}

func TestClosingMarkerInsideArguments(t *testing.T) {
	inputs := []string{
		toolOpen + `send(text="before <|tool_call_end|> after")` + toolClose,
		toolOpen + `{"name":"send","arguments":{"text":"before <|tool_call_end|> after"}}` + toolClose,
		toolOpen + `send(text="escaped \"<|tool_call_end|>\" value")` + toolClose,
		toolOpen + `{"name":"send","arguments":{"text":"escaped \"<|tool_call_end|>\" value"}}` + toolClose,
	}
	for _, input := range inputs {
		calls := Parser{}.ToolCall(context.Background(), nil, input)
		if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "send" {
			t.Errorf("ToolCall(%q): got %#v, want one send call", input, calls)
		}
	}
}

func TestStateMachineAtEveryMarkerSplit(t *testing.T) {
	payload := `send(text="before <|tool_call_end|> after")`
	stream := toolOpen + payload + toolClose
	for split := range len(stream) + 1 {
		t.Run(fmt.Sprintf("split-%d", split), func(t *testing.T) {
			sm := Parser{}.NewStateMachine()
			var tooling strings.Builder
			for _, fragment := range []string{stream[:split], stream[split:]} {
				result, _ := sm.Classify(fragment)
				if result.Channel == model.ChannelTool {
					tooling.WriteString(result.Content)
				}
			}
			flusher := sm.(model.StateMachineFlusher)
			for result := flusher.Flush(); result != (model.Result{}); result = flusher.Flush() {
				if result.Channel == model.ChannelTool {
					tooling.WriteString(result.Content)
				}
			}
			calls := Parser{}.ToolCall(context.Background(), nil, tooling.String())
			if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Arguments["text"] != "before <|tool_call_end|> after" {
				t.Errorf("split %d: content %q, calls %#v", split, tooling.String(), calls)
			}
			if got := flusher.Flush(); got != (model.Result{}) {
				t.Errorf("second drained Flush: got %#v", got)
			}
		})
	}
}

func TestStateMachine(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	steps := []struct {
		input   string
		channel model.Channel
		content string
	}{
		{"<think>", model.ChannelNone, ""},
		{"reason", model.ChannelReasoning, "reason"},
		{"</think>", model.ChannelNone, ""},
		{"answer", model.ChannelAnswer, "answer"},
		{"<|tool_", model.ChannelNone, ""},
		{"call_start|>", model.ChannelNone, ""},
		{`[weather(city="Paris")]`, model.ChannelNone, ""},
		{toolClose, model.ChannelTool, toolOpen + `[weather(city="Paris")]` + toolClose},
		{"after", model.ChannelAnswer, "after"},
	}
	for _, step := range steps {
		got, eog := sm.Classify(step.input)
		if eog || got.Channel != step.channel || got.Content != step.content {
			t.Errorf("Classify(%q): got %#v, eog %v, want channel %v content %q", step.input, got, eog, step.channel, step.content)
		}
	}
	streamer := sm.(model.ToolCallDeltaStreamer)
	if got := streamer.ToolCallDeltas(); len(got) != 1 || got[0].Function.Name != "weather" {
		t.Errorf("deltas: got %#v", got)
	}
}

func TestFlushAndReset(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	sm.Classify(toolOpen)
	sm.Classify(`[unfinished(x=1)`)
	flusher := sm.(model.StateMachineFlusher)
	if got := flusher.Flush(); got.Channel != model.ChannelTool || got.Content != toolOpen+`[unfinished(x=1)` {
		t.Errorf("Flush: got %#v", got)
	}
	sm.Reset()
	if got := flusher.Flush(); got != (model.Result{}) {
		t.Errorf("Flush after Reset: got %#v", got)
	}
	if got := sm.(model.ToolCallDeltaStreamer).StartedToolCalls(); len(got) != 0 {
		t.Errorf("StartedToolCalls after Reset: got %#v", got)
	}
}

func TestFlushTrailingToolMarkerPrefix(t *testing.T) {
	call := toolOpen + `weather(city="Paris")` + toolClose
	sm := Parser{}.NewStateMachine()
	first, _ := sm.Classify(call + `<|tool_`)
	if first.Channel != model.ChannelTool || first.Content != call {
		t.Fatalf("Classify: got %#v, want completed tool call", first)
	}
	trailing := sm.(model.StateMachineFlusher).Flush()
	if trailing.Channel != model.ChannelTool || trailing.Content != `<|tool_` {
		t.Fatalf("Flush: got %#v, want trailing tool prefix", trailing)
	}
	if stripped := (Parser{}).StripToolCallMarkup(first.Content + trailing.Content); stripped != "" {
		t.Errorf("StripToolCallMarkup: got %q, want empty", stripped)
	}

	sm.Reset()
	answer, _ := sm.Classify(`ordinary <|tool_`)
	if answer.Channel != model.ChannelAnswer || answer.Content != "ordinary " {
		t.Fatalf("ordinary Classify: got %#v", answer)
	}
	trailing = sm.(model.StateMachineFlusher).Flush()
	if trailing.Channel != model.ChannelAnswer || trailing.Content != `<|tool_` {
		t.Errorf("ordinary Flush: got %#v, want answer prefix", trailing)
	}

	sm.Reset()
	first, _ = sm.Classify(call + "\n<|tool_")
	separator := sm.(model.StateMachineFlusher).Flush()
	trailing = sm.(model.StateMachineFlusher).Flush()
	if first.Channel != model.ChannelTool || separator.Channel != model.ChannelAnswer || separator.Content != "\n" ||
		trailing.Channel != model.ChannelTool || trailing.Content != `<|tool_` {
		t.Fatalf("whitespace-separated prefix: got first %#v separator %#v trailing %#v", first, separator, trailing)
	}
	if stripped := (Parser{}).StripToolCallMarkup(first.Content + trailing.Content); stripped != "" {
		t.Errorf("whitespace-separated StripToolCallMarkup: got %q, want empty", stripped)
	}
}

func TestDeltaDetectionIsStructural(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"incomplete JSON object", `{"name":"bad","arguments":{"text":"helper()"}`, nil},
		{"incomplete JSON array", `[{"name":"bad","arguments":{"text":"helper()"}`, nil},
		{"Python nested-looking string", `[send(text="helper(x)")]`, []string{"send"}},
		{"JSON invalid name", `{"name":1,"arguments":{}}`, nil},
		{"JSON invalid arguments", `{"name":"bad","arguments":[]}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolCallNames(tt.input)
			if !slices.Equal(got, tt.want) {
				t.Errorf("toolCallNames: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMalformedNestedObjectAndOuterList(t *testing.T) {
	for _, input := range []string{`f(x={`, `f(x={   `, `[good(x=1)`} {
		calls := Parser{}.ToolCall(context.Background(), nil, input)
		last := calls[len(calls)-1]
		if last.Status != parseErrorStatus || last.Raw == "" || last.Error == "" {
			t.Errorf("ToolCall(%q): got %#v, want failure evidence", input, calls)
		}
	}
}

func TestPythonJSONNumbers(t *testing.T) {
	calls := Parser{}.ToolCall(context.Background(), nil, `f(x=1e9999)`)
	if got := calls[0].Function.Arguments["x"]; got != json.Number("1e9999") {
		t.Errorf("huge exponent: got %#v, want exact json.Number", got)
	}
	if _, err := json.Marshal(calls[0].Function.Arguments); err != nil {
		t.Fatalf("marshal accepted number: %v", err)
	}
	for _, number := range []string{"NaN", "Inf", "Infinity", "0x1p2", "1_000", "+1", "01", "00.1", "-01"} {
		calls := Parser{}.ToolCall(context.Background(), nil, "f(x="+number+")")
		if calls[len(calls)-1].Status != parseErrorStatus {
			t.Errorf("number %q: got %#v, want failure", number, calls)
		}
	}
}

func TestAttachedReasoningAndCoalescedTools(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	first, _ := sm.Classify("answer<think>reason</thi")
	second, _ := sm.Classify("nk>tail")
	third, _ := sm.Classify("")
	if first.Channel != model.ChannelAnswer || first.Content != "answer" ||
		second.Channel != model.ChannelReasoning || second.Content != "reason" ||
		third.Channel != model.ChannelAnswer || third.Content != "tail" {
		t.Fatalf("reasoning results: got %#v, %#v, %#v", first, second, third)
	}

	sm.Reset()
	fragment := toolOpen + `[one(x=1)]` + toolClose + toolOpen + `[two(x=2)]` + toolClose
	one, _ := sm.Classify(fragment)
	two := sm.(model.StateMachineFlusher).Flush()
	if one.Channel != model.ChannelTool || two != (model.Result{}) {
		t.Fatalf("tool results: got %#v, %#v", one, two)
	}
	calls := Parser{}.ToolCall(context.Background(), nil, one.Content)
	if len(calls) != 2 || calls[0].Function.Name != "one" || calls[1].Function.Name != "two" {
		t.Errorf("coalesced calls: got %#v", calls)
	}
}

func TestStateMachinePreservesToolBlockBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		stream string
	}{
		{name: "malformed blocks cannot combine", stream: toolOpen + `f(x=` + toolClose + toolOpen + `1)` + toolClose},
		{name: "missing final closer", stream: toolOpen + `f()` + toolClose + toolOpen + `g()`},
		{name: "empty sibling", stream: toolOpen + `f()` + toolClose + toolOpen + toolClose + toolOpen + `g()` + toolClose},
		{name: "bare opener", stream: toolOpen},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := Parser{}.NewStateMachine()
			var tooling strings.Builder
			if result, _ := sm.Classify(tt.stream); result.Channel == model.ChannelTool {
				tooling.WriteString(result.Content)
			}
			for result := sm.(model.StateMachineFlusher).Flush(); result != (model.Result{}); result = sm.(model.StateMachineFlusher).Flush() {
				if result.Channel == model.ChannelTool {
					tooling.WriteString(result.Content)
				}
			}
			calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
			if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
				t.Fatalf("tooling %q parsed as %+v, want one non-executable failure", tooling.String(), calls)
			}
		})
	}
}

func TestStateMachinePreservesValidWrappedCalls(t *testing.T) {
	stream := toolOpen + `f(x=1)` + toolClose + toolOpen + `g(y=2)` + toolClose
	sm := Parser{}.NewStateMachine()
	var tooling strings.Builder
	if result, _ := sm.Classify(stream); result.Channel == model.ChannelTool {
		tooling.WriteString(result.Content)
	}
	for result := sm.(model.StateMachineFlusher).Flush(); result != (model.Result{}); result = sm.(model.StateMachineFlusher).Flush() {
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}
	calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
	if len(calls) != 2 || calls[0].Status != 0 || calls[1].Status != 0 || calls[0].Function.Name != "f" || calls[1].Function.Name != "g" {
		t.Fatalf("tooling %q parsed as %+v, want valid f and g", tooling.String(), calls)
	}
}

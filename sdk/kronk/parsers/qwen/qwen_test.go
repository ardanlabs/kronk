package qwen

import (
	"context"
	"reflect"
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

func runSteps(t *testing.T, name string, c model.StateMachine, steps []step) {
	t.Helper()
	for i, s := range steps {
		got, eog := c.Classify(s.token)
		if got.Channel != s.channel {
			t.Errorf("%s step %d (%q): channel = %v, want %v",
				name, i, s.token, got.Channel, s.channel)
		}
		if got.Content != s.content {
			t.Errorf("%s step %d (%q): content = %q, want %q",
				name, i, s.token, got.Content, s.content)
		}
		if eog != s.eog {
			t.Errorf("%s step %d (%q): eog = %v, want %v",
				name, i, s.token, eog, s.eog)
		}
	}
}

// TestNew_ClaimsQwen verifies parser selection across the layered
// architecture-prefix / chat-template-marker / model-name signals.
func TestNew_ClaimsQwen(t *testing.T) {
	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		// Architecture prefix (primary signal).
		{"arch-qwen2", model.Fingerprint{Architecture: "qwen2"}, true},
		{"arch-qwen3", model.Fingerprint{Architecture: "qwen3"}, true},
		{"arch-qwen35moe", model.Fingerprint{Architecture: "qwen35moe"}, true},
		{"arch-qwen2-moe", model.Fingerprint{Architecture: "qwen2_moe"}, true},
		{"arch-mixed-case", model.Fingerprint{Architecture: "Qwen3"}, true},

		// Chat template marker (secondary signal).
		{"template-function", model.Fingerprint{ChatTemplate: "example: <function=do_thing>"}, true},
		{"template-parameter", model.Fingerprint{ChatTemplate: "<parameter=k>v</parameter>"}, true},

		// Model name fallback.
		{"name-Qwen", model.Fingerprint{ModelName: "Qwen3-Coder-30B-A3B"}, true},
		{"name-lowercase", model.Fingerprint{ModelName: "qwen2.5-7b"}, true},

		// Negatives.
		{"name-llama", model.Fingerprint{ModelName: "Llama-3-8B"}, false},
		{"empty", model.Fingerprint{}, false},
		{"arch-gemma", model.Fingerprint{Architecture: "gemma3"}, false},
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

// =============================================================================
// Parser — JSON envelope path
// =============================================================================

func TestParser_ReasoningThenAnswer(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "reasoning-then-answer", c, []step{
		{token: "<think>", channel: model.ChannelNone},
		{token: "Plan", channel: model.ChannelReasoning, content: "Plan"},
		{token: "</think>", channel: model.ChannelNone},
		{token: "Hi", channel: model.ChannelAnswer, content: "Hi"},
	})
}

func TestParser_JSONToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "json-tool-call", c, []step{
		{token: "<tool_call>", channel: model.ChannelTool},
		{token: `{"name":"a","arguments":{}}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: encodeQwenWrapperFrame(`{"name":"a","arguments":{}}`+"\n", true)},
	})
	_, eog := c.Classify("done")
	if !eog {
		t.Errorf("expected EOG after tool call closed")
	}
}

func TestParser_WrappedJSONPreservesNativeMarkerTextInArguments(t *testing.T) {
	c := Parser{}.NewStateMachine()
	var tooling strings.Builder
	for _, token := range []string{"<tool_call>", `{"name":"write","arguments":{"text":"before `, "</tool_call>", "<tool_call>", ` after"}}`, "</tool_call>"} {
		result, _ := c.Classify(token)
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}

	calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
	if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Arguments["text"] != "before </tool_call><tool_call> after" {
		t.Fatalf("ToolCall: got %+v, want original marker text in argument", calls)
	}
	if got := (Parser{}).StripToolCallMarkup(tooling.String()); got != "" {
		t.Errorf("StripToolCallMarkup: got %q, want empty", got)
	}
}

func TestParser_JSONToolCallActivityDelta(t *testing.T) {
	c := Parser{}.NewStateMachine()
	streamer := c.(model.ToolCallDeltaStreamer)

	var deltas []model.ResponseToolCallDelta
	for _, token := range []string{
		"<tool_call>",
		`{"name":"get_`,
		`weather","arguments":`,
		`{"location":"London"}}`,
		"</tool_call>",
	} {
		if _, eog := c.Classify(token); eog {
			t.Fatalf("Classify(%q): got EOG before tool call completed", token)
		}
		deltas = append(deltas, streamer.ToolCallDeltas()...)
	}

	if len(deltas) != 1 {
		t.Fatalf("ToolCallDeltas: got %d deltas, want 1", len(deltas))
	}
	if deltas[0].ID == "" {
		t.Error("delta ID: got empty, want generated call ID")
	}
	if got, want := deltas[0].Index, 0; got != want {
		t.Errorf("delta Index: got %d, want %d", got, want)
	}
	if got, want := deltas[0].Type, "function"; got != want {
		t.Errorf("delta Type: got %q, want %q", got, want)
	}
	if got, want := deltas[0].Function.Name, "get_weather"; got != want {
		t.Errorf("delta Function.Name: got %q, want %q", got, want)
	}
	if got := deltas[0].Function.Arguments; got != "" {
		t.Errorf("delta Function.Arguments: got %q, want empty", got)
	}

	started := streamer.StartedToolCalls()
	if len(started) != 1 {
		t.Fatalf("StartedToolCalls: got %d calls, want 1", len(started))
	}
	if got, want := started[0].ID, deltas[0].ID; got != want {
		t.Errorf("started call ID: got %q, want %q", got, want)
	}
}

func TestParser_WrappedDirectToolCallActivityDelta(t *testing.T) {
	c := Parser{}.NewStateMachine()
	streamer := c.(model.ToolCallDeltaStreamer)

	var deltas []model.ResponseToolCallDelta
	for _, token := range []string{
		"<tool_call>",
		"\n", "<function", "=", "write", ">", "\n",
		"<parameter=filePath>\n/tmp/main.go\n</parameter>\n",
		"<parameter=content>\npackage main\n</parameter>\n",
		"</function>\n</tool_call>",
	} {
		if _, eog := c.Classify(token); eog {
			t.Fatalf("Classify(%q): got EOG before tool call completed", token)
		}
		deltas = append(deltas, streamer.ToolCallDeltas()...)
	}

	if len(deltas) != 1 {
		t.Fatalf("ToolCallDeltas: got %d deltas, want 1", len(deltas))
	}
	if got, want := deltas[0].Function.Name, "write"; got != want {
		t.Errorf("delta Function.Name: got %q, want %q", got, want)
	}
	if got := deltas[0].Function.Arguments; got != "" {
		t.Errorf("delta Function.Arguments: got %q, want empty", got)
	}
}

func TestParser_ToolCallActivityDeltaIndexes(t *testing.T) {
	c := Parser{}.NewStateMachine()
	streamer := c.(model.ToolCallDeltaStreamer)

	var indexes []int
	for _, token := range []string{
		"<function=first>", "</function>",
		"\n", "<function=second>", "</function>",
	} {
		if _, eog := c.Classify(token); eog {
			t.Fatalf("Classify(%q): got EOG before tool calls completed", token)
		}
		for _, delta := range streamer.ToolCallDeltas() {
			if delta.ID != "" {
				indexes = append(indexes, delta.Index)
			}
		}
	}

	if got, want := indexes, []int{0, 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("activity delta indexes: got %v, want %v", got, want)
	}
}

func TestParser_TruncatedJSONToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()

	var tooling strings.Builder
	for _, token := range []string{"<tool_call>", `{"name":"get_weather","arguments":{"location":"Paris"}}`} {
		result, eog := c.Classify(token)
		if eog {
			t.Fatalf("Classify(%q): got EOG before tool call completed", token)
		}
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}
	flusher := c.(model.StateMachineFlusher)
	tooling.WriteString(flusher.Flush().Content)

	calls := Parser{}.ToolCall(context.Background(), noopLog, tooling.String())
	if len(calls) != 1 {
		t.Fatalf("ToolCall: got %d calls, want 1", len(calls))
	}
	if got, want := calls[0].Function.Name, "get_weather"; got != want {
		t.Errorf("Function.Name: got %q, want %q", got, want)
	}
}

func TestParser_StripToolCallMarkup(t *testing.T) {
	tests := []struct {
		name string
		buf  string
		want string
	}{
		{name: "complete JSON", buf: `{"name":"get_weather","arguments":{"location":"Paris"}}`},
		{name: "truncated JSON", buf: `{"name":"get_weather","arguments":{"location":"Paris"`},
		{name: "wrapped JSON", buf: `<tool_call>{"name":"get_weather","arguments":{}}</tool_call>`},
		{name: "alternate wrapped JSON", buf: `<|tool_call>{"name":"get_weather","arguments":{}}<tool_call|>`},
		{name: "wrapped JSON closer in string", buf: `<tool_call>{"name":"write","arguments":{"text":"before </tool_call> after"}}</tool_call>`},
		{name: "truncated wrapped JSON", buf: `<tool_call>{"name":"get_weather"`},
		{name: "complete direct XML", buf: `<function=get_weather><parameter=location>Paris</parameter></function>`},
		{name: "truncated direct XML", buf: `<function=get_weather><parameter=location>Paris`},
		{name: "direct XML closer in value", buf: `<function=write><parameter=text>before </function> after</parameter></function>`},
		{name: "truncated direct XML structural close", buf: `<function=write><parameter=text>before </function>after`},
		{name: "trailing marker prefix", buf: `{"name":"first","arguments":{}}<tool_`},
		{name: "mixed calls", buf: `{"name":"first","arguments":{}}` + "\n" + `<function=second></function>`},
		{name: "ordinary JSON after direct call", buf: `<function=get_weather></function>` + "\n" + `{"ordinary":true}`, want: "\n" + `{"ordinary":true}`},
		{name: "ordinary JSON", buf: `{"ordinary":true}`, want: `{"ordinary":true}`},
		{name: "surrounding content", buf: `before {"name":"get_weather","arguments":{}} after`, want: "before  after"},
		{name: "trailing content", buf: `<function=get_weather></function> explanation`, want: " explanation"},
		{name: "ordinary content", buf: "explanation", want: "explanation"},
		{name: "ordinary marker prefix", buf: "explanation <", want: "explanation <"},
		{name: "foreign markup", buf: "[TOOL_CALLS]", want: "[TOOL_CALLS]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Parser{}).StripToolCallMarkup(tt.buf); got != tt.want {
				t.Errorf("StripToolCallMarkup: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParser_StateMachineWrapperEvidenceStripsInvalidBodies(t *testing.T) {
	tests := []struct {
		name   string
		tokens []string
		flush  bool
	}{
		{name: "complete ordinary JSON", tokens: []string{"<tool_call>", `{"ordinary":true}`, "</tool_call>"}},
		{name: "truncated named JSON", tokens: []string{"<tool_call>", `{"name":"get_weather"`}, flush: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Parser{}.NewStateMachine()
			var tooling strings.Builder
			for _, token := range tt.tokens {
				result, eog := c.Classify(token)
				if eog {
					t.Fatalf("Classify(%q): got unexpected EOG", token)
				}
				if result.Channel == model.ChannelTool {
					tooling.WriteString(result.Content)
				}
			}
			if tt.flush {
				tooling.WriteString(c.(model.StateMachineFlusher).Flush().Content)
			}

			if got := (Parser{}).StripToolCallMarkup(tooling.String()); got != "" {
				t.Errorf("StripToolCallMarkup(%q): got %q, want empty", tooling.String(), got)
			}
		})
	}
}

func TestParser_BackToBackJSONToolCalls(t *testing.T) {
	c := Parser{}.NewStateMachine()
	var tooling strings.Builder

	for _, token := range []string{
		"<tool_call>", `{"name":"first","arguments":{}}`, "</tool_call>",
		"\n",
		"<tool_call>", `{"name":"second","arguments":{}}`, "</tool_call>",
	} {
		result, eog := c.Classify(token)
		if eog {
			t.Fatalf("Classify(%q): got EOG before all tool calls completed", token)
		}
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}

	calls := Parser{}.ToolCall(context.Background(), noopLog, tooling.String())
	if len(calls) != 2 {
		t.Fatalf("ToolCall: got %d calls, want 2", len(calls))
	}
	for i, want := range []string{"first", "second"} {
		if got := calls[i].Function.Name; got != want {
			t.Errorf("call %d Function.Name: got %q, want %q", i, got, want)
		}
	}
}

// =============================================================================
// Parser — Direct <function= XML format
// =============================================================================

func TestParser_DirectFunctionTagSingleToken(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "direct-function-single", c, []step{
		{token: "<function=foo>", channel: model.ChannelTool},
		{token: "<parameter=k>v</parameter>", channel: model.ChannelNone},
		{token: "</function>", channel: model.ChannelTool,
			content: "<function=foo><parameter=k>v</parameter></function>\n"},
	})
}

func TestParser_DirectFunctionTagSplit(t *testing.T) {
	c := Parser{}.NewStateMachine()
	var tooling strings.Builder
	for _, tok := range []string{"<", "function", "=", "do_thing>\n", "<parameter=k>\nv\n</parameter>\n</function>"} {
		result, eog := c.Classify(tok)
		if eog {
			t.Fatalf("Classify(%q): got EOG before tool call completed", tok)
		}
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}

	calls := Parser{}.ToolCall(context.Background(), noopLog, tooling.String())
	if len(calls) != 1 {
		t.Fatalf("ToolCall: got %d calls, want 1", len(calls))
	}
	if got, want := calls[0].Function.Name, "do_thing"; got != want {
		t.Errorf("Function.Name: got %q, want %q", got, want)
	}
	if got, want := calls[0].Function.Arguments["k"], "v"; got != want {
		t.Errorf("argument k: got %q, want %q", got, want)
	}

	result, eog := c.Classify("trailing")
	if eog {
		t.Error("unexpected continuation after direct XML must be preserved for final validation")
	}
	if result.Channel != model.ChannelTool || result.Content != "trailing" {
		t.Errorf("unexpected continuation: got %+v, want tool content %q", result, "trailing")
	}
}

func TestParser_WrappedDirectToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	var tooling strings.Builder

	for _, tok := range []string{
		"<tool_call>", "\n", "<", "function", "=b", "ash>", "\n",
		"<parameter=command>\ngo build\n</parameter>", "\n",
		"</function>\n</tool_call>",
	} {
		result, eog := c.Classify(tok)
		if eog {
			t.Fatalf("Classify(%q): got EOG before outer tool wrapper closed", tok)
		}
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}

	calls := Parser{}.ToolCall(context.Background(), noopLog, tooling.String())
	if len(calls) != 1 {
		t.Fatalf("ToolCall: got %d calls, want 1", len(calls))
	}
	if got, want := calls[0].Function.Name, "bash"; got != want {
		t.Errorf("Function.Name: got %q, want %q", got, want)
	}
	if got, want := calls[0].Function.Arguments["command"], "go build"; got != want {
		t.Errorf("argument command: got %q, want %q", got, want)
	}
}

func TestParser_BackToBackDirectToolCalls(t *testing.T) {
	c := Parser{}.NewStateMachine()
	var tooling strings.Builder

	for _, tok := range []string{
		"<function=first>", "</function>", "\n<", "function", "=second>", "</function>",
	} {
		result, eog := c.Classify(tok)
		if eog {
			t.Fatalf("Classify(%q): got EOG before all tool calls completed", tok)
		}
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}

	calls := Parser{}.ToolCall(context.Background(), noopLog, tooling.String())
	if len(calls) != 2 {
		t.Fatalf("ToolCall: got %d calls, want 2", len(calls))
	}
	for i, want := range []string{"first", "second"} {
		if got := calls[i].Function.Name; got != want {
			t.Errorf("call %d Function.Name: got %q, want %q", i, got, want)
		}
	}
}

func TestParser_PreservesMalformedTrailingClosers(t *testing.T) {
	tests := []struct {
		name string
		tail []string
	}{
		{name: "whole closer", tail: []string{"</parameter>", "</function>"}},
		{name: "split after slash", tail: []string{"</", "parameter>", "</function>"}},
		{name: "split after opener", tail: []string{"<", "/parameter>", "</function>"}},
		{name: "prefixed closer", tail: []string{"unexpected</parameter>", "</function>"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Parser{}.NewStateMachine()
			var tooling strings.Builder
			tokens := []string{
				"<function=write_file><parameter=path>t.txt</parameter><parameter=content>",
				"</parameter>",
				"</function>",
				"<function=bash><parameter=command>id</parameter>",
				"</function>",
			}
			tokens = append(tokens, tt.tail...)

			for _, token := range tokens {
				result, eog := c.Classify(token)
				if eog {
					t.Fatalf("Classify(%q): got EOG before malformed output was preserved", token)
				}
				if result.Channel == model.ChannelTool {
					tooling.WriteString(result.Content)
				}
			}

			calls := Parser{}.ToolCall(context.Background(), noopLog, tooling.String())
			if len(calls) != 1 {
				t.Fatalf("ToolCall: got %d calls, want 1 failed call", len(calls))
			}
			if calls[0].Status == 0 {
				t.Fatalf("Status: got 0, want parse failure: %+v", calls[0])
			}
			if calls[0].Function.Name != "" {
				t.Errorf("Function.Name: got %q, want no executable function", calls[0].Function.Name)
			}
		})
	}
}

func TestParser_PendingTagFalseAlarm(t *testing.T) {
	c := Parser{}.NewStateMachine()
	got, _ := c.Classify("<f")
	if got.Content != "" {
		t.Errorf("after <f buffer, expected no content, got %q", got.Content)
	}
	got, _ = c.Classify("oobar")
	if got.Content != "<foobar" || got.Channel != model.ChannelAnswer {
		t.Errorf("expected flushed %q on answer channel, got %+v", "<foobar", got)
	}
}

func TestParser_FlushPendingTag(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<think>")
	c.Classify("<f")

	flusher := c.(model.StateMachineFlusher)
	got := flusher.Flush()
	if got.Channel != model.ChannelReasoning || got.Content != "<f" {
		t.Errorf("Flush: got {%v %q}, want {%v %q}", got.Channel, got.Content, model.ChannelReasoning, "<f")
	}
	if got := flusher.Flush(); got != (model.Result{}) {
		t.Errorf("second Flush: got %+v, want zero result", got)
	}
}

// =============================================================================
// Parser — foreign markers pass through
// =============================================================================

func TestParser_ForeignMarkersAreContent(t *testing.T) {
	c := Parser{}.NewStateMachine()
	for _, m := range []string{"[TOOL_CALLS]", "<|channel>", "call:foo"} {
		c.Reset()
		got, eog := c.Classify(m)
		if eog {
			t.Errorf("qwen should not EOG on foreign marker %q", m)
		}
		if got.Channel != model.ChannelAnswer || got.Content != m {
			t.Errorf("qwen should pass-through %q, got %+v", m, got)
		}
	}
}

// =============================================================================
// Parser — Reset
// =============================================================================

func TestParser_Reset(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<think>")
	c.Reset()
	got, _ := c.Classify("hi")
	if got.Channel != model.ChannelAnswer || got.Content != "hi" {
		t.Errorf("after Reset got %+v", got)
	}
}

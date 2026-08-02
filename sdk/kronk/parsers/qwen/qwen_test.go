package qwen

import (
	"context"
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
			content: `{"name":"a","arguments":{}}` + "\n"},
	})
	_, eog := c.Classify("done")
	if !eog {
		t.Errorf("expected EOG after tool call closed")
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

	_, eog := c.Classify("trailing")
	if !eog {
		t.Errorf("expected EOG after </function>")
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

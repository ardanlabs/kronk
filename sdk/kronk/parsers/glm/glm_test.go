package glm

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

// =============================================================================
// Parser selection
// =============================================================================

func TestNew_ClaimsGLM(t *testing.T) {
	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		// Architecture prefix (primary signal).
		{"arch-glm", model.Fingerprint{Architecture: "glm"}, true},
		{"arch-glm4", model.Fingerprint{Architecture: "glm4"}, true},
		{"arch-glm5", model.Fingerprint{Architecture: "glm_moe_dsa"}, true},
		{"arch-chatglm", model.Fingerprint{Architecture: "chatglm"}, true},
		{"arch-mixed-case", model.Fingerprint{Architecture: "GLM4"}, true},

		// Chat template marker (secondary signal).
		{"template-arg-key", model.Fingerprint{ChatTemplate: "<tool_call>name<arg_key>k</arg_key>"}, true},
		{"template-arg-value", model.Fingerprint{ChatTemplate: "<arg_value>v</arg_value>"}, true},
		{"template-glm5", model.Fingerprint{ChatTemplate: "<tool_call>{function-name}<arg_key>{arg-key}</arg_key><arg_value>{arg-value}</arg_value></tool_call>"}, true},

		// Model name fallback.
		{"name-GLM", model.Fingerprint{ModelName: "GLM-4.6"}, true},
		{"name-GLM5", model.Fingerprint{ModelName: "GLM-5.2"}, true},
		{"name-lowercase", model.Fingerprint{ModelName: "glm-4-9b-chat"}, true},

		// Negatives.
		{"name-llama", model.Fingerprint{ModelName: "Llama-3"}, false},
		{"empty", model.Fingerprint{}, false},
		{"arch-qwen", model.Fingerprint{Architecture: "qwen3"}, false},
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
// Parser
// =============================================================================

func TestParser_PureAnswer(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "pure-answer", c, []step{
		{token: "Hello", channel: model.ChannelAnswer, content: "Hello"},
	})
}

func TestParser_Reasoning(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "reasoning", c, []step{
		{token: "<think>", channel: model.ChannelNone},
		{token: "Plan", channel: model.ChannelReasoning, content: "Plan"},
		{token: "</think>", channel: model.ChannelNone},
		{token: "Answer", channel: model.ChannelAnswer, content: "Answer"},
	})
}

func TestParser_ToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "tool-call", c, []step{
		{token: "<tool_call>", channel: model.ChannelNone},
		{token: "get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>",
			channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: "get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>\n"},
	})
	result, eog := c.Classify("done")
	if eog {
		t.Error("unexpected continuation after a tool call must be preserved for final validation")
	}
	want := "\n</tool_call>done"
	if result.Channel != model.ChannelTool || result.Content != want {
		t.Errorf("unexpected continuation: got %+v, want tool content %q", result, want)
	}
}

func TestParser_GLM5ToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	result, eog := c.Classify("<tool_call>get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value><arg_key>days</arg_key><arg_value>3</arg_value></tool_call>")
	if eog {
		t.Fatal("GLM-5 tool call unexpectedly ended generation")
	}

	calls := Parser{}.ToolCall(t.Context(), nil, result.Content)
	if len(calls) != 1 || calls[0].Status != 0 {
		t.Fatalf("ToolCall: got %+v, want one successful GLM-5 call", calls)
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("Function.Name: got %q, want %q", calls[0].Function.Name, "get_weather")
	}
	if args := calls[0].Function.Arguments; args["location"] != "NYC" || args["days"] != "3" {
		t.Errorf("Function.Arguments: got %v, want location=NYC and days=3", args)
	}
}

func TestParser_PreservesMalformedTrailingContent(t *testing.T) {
	tests := []struct {
		name string
		tail []string
	}{
		{name: "whole delimiter", tail: []string{"</arg_value>"}},
		{name: "split delimiter", tail: []string{"</arg_", "value>"}},
		{name: "prefixed delimiter", tail: []string{"unexpected</arg_value>"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Parser{}.NewStateMachine()
			var tooling strings.Builder
			tokens := []string{
				"<tool_call>",
				"write_file<arg_key>content</arg_key><arg_value></arg_value>",
				"</tool_call>",
				"<tool_call>",
				"bash<arg_key>command</arg_key><arg_value>id</arg_value>",
				"</tool_call>",
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

			calls := Parser{}.ToolCall(context.Background(), nil, tooling.String())
			if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
				t.Fatalf("ToolCall: got %+v, want one non-executable failed call", calls)
			}
		})
	}
}

func TestParser_FlushIncompleteToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<tool_call>")
	c.Classify("get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>")

	flusher := c.(model.StateMachineFlusher)
	got := flusher.Flush()
	want := "get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>\n</tool_call>"
	if got.Channel != model.ChannelTool || got.Content != want {
		t.Errorf("Flush: got {%v %q}, want {%v %q}", got.Channel, got.Content, model.ChannelTool, want)
	}
	if got := flusher.Flush(); got != (model.Result{}) {
		t.Errorf("second Flush: got %+v, want zero result", got)
	}
}

func TestParser_EnvelopeTokenBoundaries(t *testing.T) {
	body := "get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>"
	for _, markers := range [][2]string{{"<tool_call>", "</tool_call>"}, {"<|tool_call>", "<tool_call|>"}} {
		for openerSplit := 1; openerSplit < len(markers[0]); openerSplit++ {
			for closerSplit := 1; closerSplit < len(markers[1]); closerSplit++ {
				sm := Parser{}.NewStateMachine()
				var aggregate strings.Builder
				for _, token := range []string{markers[0][:openerSplit], markers[0][openerSplit:], body + markers[1][:closerSplit], markers[1][closerSplit:]} {
					result, _ := sm.Classify(token)
					if result.Channel == model.ChannelTool {
						aggregate.WriteString(result.Content)
					}
				}
				calls := Parser{}.ToolCall(t.Context(), nil, aggregate.String())
				if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "get_weather" {
					t.Fatalf("markers %q/%q splits %d/%d: got %+v", markers[0], markers[1], openerSplit, closerSplit, calls)
				}
			}
		}
	}
}

func TestParser_CoalescedConsecutiveEnvelopes(t *testing.T) {
	first := "<tool_call>first<arg_key>value</arg_key><arg_value>x</arg_value></tool_call>"
	second := "<tool_call>second<arg_key>value</arg_key><arg_value>y</arg_value></tool_call>"
	sm := Parser{}.NewStateMachine()
	result, _ := sm.Classify(first + second)
	calls := Parser{}.ToolCall(t.Context(), nil, result.Content)
	if len(calls) != 2 || calls[0].Function.Name != "first" || calls[1].Function.Name != "second" {
		t.Fatalf("ToolCall: got %+v, want successful calls [first second]", calls)
	}
}

func TestParser_EmptyTrailingEnvelopeFailsAtomically(t *testing.T) {
	first := "<tool_call>first<arg_key>value</arg_key><arg_value>x</arg_value></tool_call>"
	for _, markers := range [][2]string{{"<tool_call>", "</tool_call>"}, {"<|tool_call>", "<tool_call|>"}} {
		for _, body := range []string{"", " \t\r\n"} {
			sm := Parser{}.NewStateMachine()
			result, _ := sm.Classify(first + markers[0] + body + markers[1])
			calls := Parser{}.ToolCall(t.Context(), nil, result.Content)
			if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
				t.Fatalf("completed %q/%q body %q: got %+v, want one non-executable failure", markers[0], markers[1], body, calls)
			}
		}

		sm := Parser{}.NewStateMachine()
		complete, _ := sm.Classify(first)
		sm.Classify(markers[0])
		tail := sm.(model.StateMachineFlusher).Flush()
		calls := Parser{}.ToolCall(t.Context(), nil, complete.Content+tail.Content)
		if tail.Channel != model.ChannelTool || len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
			t.Fatalf("incomplete %q: tail %+v, calls %+v; want one non-executable failure", markers[0], tail, calls)
		}
	}
}

func TestParser_ForeignMarkersAreContent(t *testing.T) {
	c := Parser{}.NewStateMachine()
	for _, m := range []string{"[TOOL_CALLS]", "<|channel>", "<function=x>"} {
		c.Reset()
		got, eog := c.Classify(m)
		if eog {
			t.Errorf("glm should not EOG on foreign marker %q", m)
		}
		if got.Channel != model.ChannelAnswer || got.Content != m {
			t.Errorf("glm should pass-through %q, got %+v", m, got)
		}
	}
}

func TestParser_Reset(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<think>")
	c.Reset()
	got, _ := c.Classify("hi")
	if got.Channel != model.ChannelAnswer || got.Content != "hi" {
		t.Errorf("after Reset got %+v", got)
	}
}

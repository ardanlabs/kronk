package glm

import (
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
		{"arch-chatglm", model.Fingerprint{Architecture: "chatglm"}, true},
		{"arch-mixed-case", model.Fingerprint{Architecture: "GLM4"}, true},

		// Chat template marker (secondary signal).
		{"template-arg-key", model.Fingerprint{ChatTemplate: "<tool_call>name<arg_key>k</arg_key>"}, true},
		{"template-arg-value", model.Fingerprint{ChatTemplate: "<arg_value>v</arg_value>"}, true},

		// Model name fallback.
		{"name-GLM", model.Fingerprint{ModelName: "GLM-4.6"}, true},
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
	_, eog := c.Classify("done")
	if !eog {
		t.Errorf("expected EOG after tool call closed")
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

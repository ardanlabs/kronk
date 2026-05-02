package mistral

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
		got, eog := c.Process(s.token)
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
// Processor selection
// =============================================================================

func TestNew_ClaimsMistralAndDevstral(t *testing.T) {
	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		// Architecture prefix (primary signal).
		{"arch-mistral", model.Fingerprint{Architecture: "mistral"}, true},
		{"arch-mistral3", model.Fingerprint{Architecture: "mistral3"}, true},
		{"arch-mixed-case", model.Fingerprint{Architecture: "Mistral"}, true},

		// Chat template marker (secondary signal).
		{"template-tool-calls", model.Fingerprint{ChatTemplate: "before [TOOL_CALLS]name[ARGS]{}"}, true},
		{"template-args", model.Fingerprint{ChatTemplate: "[ARGS]{...}"}, true},

		// Model name fallback.
		{"name-Mistral", model.Fingerprint{ModelName: "Mistral-7B-Instruct"}, true},
		{"name-Devstral", model.Fingerprint{ModelName: "Devstral-Small"}, true},

		// Negatives.
		{"name-llama", model.Fingerprint{ModelName: "Llama-3-8B"}, false},
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
// Processor
// =============================================================================

func TestProcessor_PureAnswer(t *testing.T) {
	c := Processor{}.NewStateMachine()
	runSteps(t, "pure-answer", c, []step{
		{token: "Hello", channel: model.ChannelAnswer, content: "Hello"},
		{token: ", ", channel: model.ChannelAnswer, content: ", "},
		{token: "world", channel: model.ChannelAnswer, content: "world"},
	})
}

func TestProcessor_Reasoning(t *testing.T) {
	c := Processor{}.NewStateMachine()
	runSteps(t, "reasoning", c, []step{
		{token: "<think>", channel: model.ChannelNone},
		{token: "Plan", channel: model.ChannelReasoning, content: "Plan"},
		{token: "</think>", channel: model.ChannelNone},
		{token: "Answer", channel: model.ChannelAnswer, content: "Answer"},
	})
}

// TestProcessor_StreamingToolCall verifies that [TOOL_CALLS] enters
// streaming tool mode (every subsequent token is tool-channel content).
func TestProcessor_StreamingToolCall(t *testing.T) {
	c := Processor{}.NewStateMachine()
	runSteps(t, "streaming-tool-call", c, []step{
		{token: "[TOOL_CALLS]", channel: model.ChannelTool, content: "[TOOL_CALLS]"},
		{token: "get_weather", channel: model.ChannelTool, content: "get_weather"},
		{token: "[ARGS]", channel: model.ChannelTool, content: "[ARGS]"},
		{token: `{"loc":"NYC"}`, channel: model.ChannelTool, content: `{"loc":"NYC"}`},
	})
}

// TestProcessor_RepeatedMarkerInsideToolMode verifies that a second
// [TOOL_CALLS] inside tool mode is silent (state already correct).
func TestProcessor_RepeatedMarkerInsideToolMode(t *testing.T) {
	c := Processor{}.NewStateMachine()
	c.Process("[TOOL_CALLS]")
	c.Process("a")
	got, _ := c.Process("[TOOL_CALLS]")
	if got.Channel != model.ChannelNone || got.Content != "" {
		t.Errorf("repeated [TOOL_CALLS] should be silent, got %+v", got)
	}
}

func TestProcessor_ForeignMarkersAreContent(t *testing.T) {
	c := Processor{}.NewStateMachine()
	for _, m := range []string{"<tool_call>", "<|channel>", "call:foo", "<function=x>"} {
		c.Reset()
		got, eog := c.Process(m)
		if eog {
			t.Errorf("mistral should not EOG on foreign marker %q", m)
		}
		if got.Channel != model.ChannelAnswer || got.Content != m {
			t.Errorf("mistral should pass-through %q, got %+v", m, got)
		}
	}
}

func TestProcessor_Reset(t *testing.T) {
	c := Processor{}.NewStateMachine()
	c.Process("[TOOL_CALLS]")
	c.Reset()
	got, _ := c.Process("hello")
	if got.Channel != model.ChannelAnswer || got.Content != "hello" {
		t.Errorf("after Reset got %+v", got)
	}
}

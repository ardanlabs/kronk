package gemma

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

func TestNew_ClaimsGemma(t *testing.T) {
	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		// Architecture prefix (primary signal).
		{"arch-gemma2", model.Fingerprint{Architecture: "gemma2"}, true},
		{"arch-gemma3", model.Fingerprint{Architecture: "gemma3"}, true},
		{"arch-gemma4", model.Fingerprint{Architecture: "gemma4"}, true},
		{"arch-mixed-case", model.Fingerprint{Architecture: "Gemma3"}, true},

		// Chat template marker (secondary signal).
		{"template-start-of-turn", model.Fingerprint{ChatTemplate: "before <start_of_turn>user after"}, true},
		{"template-channel", model.Fingerprint{ChatTemplate: "<|channel>thought<channel|>"}, true},

		// Model name fallback.
		{"name-Gemma", model.Fingerprint{ModelName: "Gemma-3-27B"}, true},
		{"name-lowercase", model.Fingerprint{ModelName: "gemma-2-9b-it"}, true},

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
// Processor
// =============================================================================

func TestProcessor_PureAnswer(t *testing.T) {
	c := Processor{}.NewStateMachine()
	runSteps(t, "pure-answer", c, []step{
		{token: "Hello", channel: model.ChannelAnswer, content: "Hello"},
		{token: " world", channel: model.ChannelAnswer, content: " world"},
	})
}

// TestProcessor_GemmaChannelMarker covers <|channel> ... <channel|>
// reasoning. The token immediately after <|channel> is swallowed (it's the
// channel name like "thought").
func TestProcessor_GemmaChannelMarker(t *testing.T) {
	c := Processor{}.NewStateMachine()
	runSteps(t, "gemma-channel", c, []step{
		{token: "<|channel>", channel: model.ChannelNone},
		{token: "thought", channel: model.ChannelNone}, // swallowed
		{token: "thinking", channel: model.ChannelReasoning, content: "thinking"},
		{token: "<channel|>", channel: model.ChannelNone},
		{token: "answer", channel: model.ChannelAnswer, content: "answer"},
	})
}

func TestProcessor_StructuralMarkersSkipped(t *testing.T) {
	c := Processor{}.NewStateMachine()
	for _, m := range []string{"<tool_call|>", "<|tool_response>", "<tool_response|>"} {
		c.Reset()
		got, eog := c.Process(m)
		if eog {
			t.Errorf("structural marker %q should not be eog", m)
		}
		if got.Content != "" || got.Channel != model.ChannelNone {
			t.Errorf("structural marker %q should be silent, got %+v", m, got)
		}
	}
}

func TestProcessor_ToolCall(t *testing.T) {
	c := Processor{}.NewStateMachine()
	runSteps(t, "tool-call", c, []step{
		{token: "<tool_call>", channel: model.ChannelNone},
		{token: `call:get_weather{location:<|"|>NYC<|"|>}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: `call:get_weather{location:<|"|>NYC<|"|>}` + "\n"},
	})
	_, eog := c.Process("done")
	if !eog {
		t.Errorf("expected EOG after tool call closed")
	}
}

func TestProcessor_ForeignMarkersAreContent(t *testing.T) {
	c := Processor{}.NewStateMachine()
	for _, m := range []string{"[TOOL_CALLS]", "<function=x>", "<think>"} {
		c.Reset()
		got, eog := c.Process(m)
		if eog {
			t.Errorf("gemma should not EOG on foreign marker %q", m)
		}
		if got.Channel != model.ChannelAnswer || got.Content != m {
			t.Errorf("gemma should pass-through %q, got %+v", m, got)
		}
	}
}

func TestProcessor_Reset(t *testing.T) {
	c := Processor{}.NewStateMachine()
	c.Process("<|channel>")
	c.Process("thought")
	c.Reset()
	got, _ := c.Process("hi")
	if got.Channel != model.ChannelAnswer || got.Content != "hi" {
		t.Errorf("after Reset got %+v", got)
	}
}

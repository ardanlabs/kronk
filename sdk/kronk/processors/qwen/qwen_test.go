package qwen

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

// TestNew_ClaimsQwen verifies processor selection across the layered
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
// Processor — JSON envelope path
// =============================================================================

func TestProcessor_ReasoningThenAnswer(t *testing.T) {
	c := Processor{}.NewStateMachine()
	runSteps(t, "reasoning-then-answer", c, []step{
		{token: "<think>", channel: model.ChannelNone},
		{token: "Plan", channel: model.ChannelReasoning, content: "Plan"},
		{token: "</think>", channel: model.ChannelNone},
		{token: "Hi", channel: model.ChannelAnswer, content: "Hi"},
	})
}

func TestProcessor_JSONToolCall(t *testing.T) {
	c := Processor{}.NewStateMachine()
	runSteps(t, "json-tool-call", c, []step{
		{token: "<tool_call>", channel: model.ChannelNone},
		{token: `{"name":"a","arguments":{}}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: `{"name":"a","arguments":{}}` + "\n"},
	})
	_, eog := c.Process("done")
	if !eog {
		t.Errorf("expected EOG after tool call closed")
	}
}

// =============================================================================
// Processor — Direct <function= XML format
// =============================================================================

func TestProcessor_DirectFunctionTagSingleToken(t *testing.T) {
	c := Processor{}.NewStateMachine()
	runSteps(t, "direct-function-single", c, []step{
		{token: "<function=foo>", channel: model.ChannelNone},
		{token: "<parameter=k>v</parameter>", channel: model.ChannelNone},
		{token: "</function>", channel: model.ChannelTool,
			content: "<function=foo><parameter=k>v</parameter></function>\n"},
	})
}

func TestProcessor_DirectFunctionTagSplit(t *testing.T) {
	c := Processor{}.NewStateMachine()
	for _, tok := range []string{"<", "function", "=", "do_thing>\n", "<parameter=k>\nv\n</parameter>\n</function>"} {
		c.Process(tok)
	}
	_, eog := c.Process("trailing")
	if !eog {
		t.Errorf("expected EOG after </function>")
	}
}

func TestProcessor_PendingTagFalseAlarm(t *testing.T) {
	c := Processor{}.NewStateMachine()
	got, _ := c.Process("<f")
	if got.Content != "" {
		t.Errorf("after <f buffer, expected no content, got %q", got.Content)
	}
	got, _ = c.Process("oobar")
	if got.Content != "<foobar" || got.Channel != model.ChannelAnswer {
		t.Errorf("expected flushed %q on answer channel, got %+v", "<foobar", got)
	}
}

// =============================================================================
// Processor — foreign markers pass through
// =============================================================================

func TestProcessor_ForeignMarkersAreContent(t *testing.T) {
	c := Processor{}.NewStateMachine()
	for _, m := range []string{"[TOOL_CALLS]", "<|channel>", "call:foo"} {
		c.Reset()
		got, eog := c.Process(m)
		if eog {
			t.Errorf("qwen should not EOG on foreign marker %q", m)
		}
		if got.Channel != model.ChannelAnswer || got.Content != m {
			t.Errorf("qwen should pass-through %q, got %+v", m, got)
		}
	}
}

// =============================================================================
// Processor — Reset
// =============================================================================

func TestProcessor_Reset(t *testing.T) {
	c := Processor{}.NewStateMachine()
	c.Process("<think>")
	c.Reset()
	got, _ := c.Process("hi")
	if got.Channel != model.ChannelAnswer || got.Content != "hi" {
		t.Errorf("after Reset got %+v", got)
	}
}

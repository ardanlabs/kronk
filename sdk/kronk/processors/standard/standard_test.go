package standard

import (
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

// TestNew_AlwaysClaims verifies the standard processor is the catch-all and
// claims any fingerprint.
func TestNew_AlwaysClaims(t *testing.T) {
	tests := []model.Fingerprint{
		{},
		{ModelName: "anything-1B"},
		{ChatTemplate: "any template"},
	}

	for _, fp := range tests {
		if _, ok := New(fp); !ok {
			t.Errorf("standard processor must claim every fingerprint, refused %+v", fp)
		}
	}
}

// =============================================================================
// Processor
// =============================================================================

// TestProcessor_PureAnswer covers a vanilla generation with no markers.
func TestProcessor_PureAnswer(t *testing.T) {
	c := Processor{}.NewStateMachine()

	runSteps(t, "pure-answer", c, []step{
		{token: "Hello", channel: model.ChannelAnswer, content: "Hello"},
		{token: ", ", channel: model.ChannelAnswer, content: ", "},
		{token: "world", channel: model.ChannelAnswer, content: "world"},
	})
}

// TestProcessor_ReasoningThenAnswer verifies <think>…</think> wrapping.
func TestProcessor_ReasoningThenAnswer(t *testing.T) {
	c := Processor{}.NewStateMachine()

	runSteps(t, "reasoning-then-answer", c, []step{
		{token: "<think>", channel: model.ChannelNone},
		{token: "Let", channel: model.ChannelReasoning, content: "Let"},
		{token: " me", channel: model.ChannelReasoning, content: " me"},
		{token: "</think>", channel: model.ChannelNone},
		{token: "Hi", channel: model.ChannelAnswer, content: "Hi"},
	})
}

// TestProcessor_SingleToolCall covers <tool_call>JSON</tool_call>.
func TestProcessor_SingleToolCall(t *testing.T) {
	c := Processor{}.NewStateMachine()

	runSteps(t, "single-tool-call", c, []step{
		{token: "<tool_call>", channel: model.ChannelNone},
		{token: `{"name":"get_weather"`, channel: model.ChannelNone},
		{token: `,"arguments":{"loc":"NYC"}}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: `{"name":"get_weather","arguments":{"loc":"NYC"}}` + "\n"},
	})

	_, eog := c.Process("\n")
	if !eog {
		t.Errorf("expected EOG after tool call closed")
	}
}

// TestProcessor_MultipleToolCalls verifies that a second opener after the
// first close is accepted (no EOG) and accumulates a fresh buffer.
func TestProcessor_MultipleToolCalls(t *testing.T) {
	c := Processor{}.NewStateMachine()

	runSteps(t, "multi-tool-call", c, []step{
		{token: "<tool_call>", channel: model.ChannelNone},
		{token: `{"name":"a","arguments":{}}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: `{"name":"a","arguments":{}}` + "\n"},
		{token: "<|tool_call>", channel: model.ChannelNone},
		{token: `{"name":"b","arguments":{}}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: `{"name":"b","arguments":{}}` + "\n"},
	})

	_, eog := c.Process("done")
	if !eog {
		t.Errorf("expected EOG after final tool call")
	}
}

// TestProcessor_UnknownMarkersAreContent verifies that markers belonging to
// other processors (e.g. Mistral [TOOL_CALLS], Gemma <|channel>) are treated
// as plain content by the standard stateMachine — the more-specific processors
// own those markers.
func TestProcessor_UnknownMarkersAreContent(t *testing.T) {
	c := Processor{}.NewStateMachine()

	for _, marker := range []string{"[TOOL_CALLS]", "<|channel>", "<function=foo>"} {
		c.Reset()
		got, eog := c.Process(marker)
		if eog {
			t.Errorf("standard should not EOG on foreign marker %q", marker)
		}
		if got.Channel != model.ChannelAnswer || got.Content != marker {
			t.Errorf("standard should pass-through %q as answer content, got %+v",
				marker, got)
		}
	}
}

// TestProcessor_Reset returns the state machine to initial state.
func TestProcessor_Reset(t *testing.T) {
	c := Processor{}.NewStateMachine()

	c.Process("<think>")
	c.Process("partial")
	c.Reset()

	got, eog := c.Process("hello")
	if eog {
		t.Errorf("Reset should clear EOG state")
	}
	if got.Channel != model.ChannelAnswer || got.Content != "hello" {
		t.Errorf("after Reset, got %+v, want {Answer, %q}", got, "hello")
	}
}

// TestProcessor_PortParity drives a long mixed stream and asserts the
// per-channel accumulation matches expectations.
func TestProcessor_PortParity(t *testing.T) {
	c := Processor{}.NewStateMachine()

	tokens := []string{
		"<think>", "Plan", " carefully", "</think>",
		"OK", " here", " goes", ":",
		"<tool_call>", `{"name":"x","arguments":{}}`, "</tool_call>",
	}

	var reasoning, answer, tool strings.Builder
	for _, tok := range tokens {
		got, _ := c.Process(tok)
		switch got.Channel {
		case model.ChannelReasoning:
			reasoning.WriteString(got.Content)
		case model.ChannelAnswer:
			answer.WriteString(got.Content)
		case model.ChannelTool:
			tool.WriteString(got.Content)
		}
	}

	if got := reasoning.String(); got != "Plan carefully" {
		t.Errorf("reasoning = %q, want %q", got, "Plan carefully")
	}
	if got := answer.String(); got != "OK here goes:" {
		t.Errorf("answer = %q, want %q", got, "OK here goes:")
	}
	wantTool := `{"name":"x","arguments":{}}` + "\n"
	if got := tool.String(); got != wantTool {
		t.Errorf("tool = %q, want %q", got, wantTool)
	}
}

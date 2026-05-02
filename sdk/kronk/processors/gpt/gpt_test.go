package gpt

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

// TestProcessor_AnalysisChannel covers the reasoning channel produced by
// the Harmony "analysis" channel name.
func TestProcessor_AnalysisChannel(t *testing.T) {
	c := Processor{}.NewStateMachine()

	runSteps(t, "analysis", c, []step{
		{token: "<|start|>", channel: model.ChannelNone},
		{token: "assistant", channel: model.ChannelNone}, // unrecognized → silent
		{token: "<|channel|>", channel: model.ChannelNone},
		{token: "analysis", channel: model.ChannelNone}, // accumulated
		{token: "<|message|>", channel: model.ChannelNone},
		{token: "Let", channel: model.ChannelReasoning, content: "Let"},
		{token: " me", channel: model.ChannelReasoning, content: " me"},
		{token: " think", channel: model.ChannelReasoning, content: " think"},
		{token: "<|end|>", channel: model.ChannelNone},
	})
}

// TestProcessor_FinalChannel covers the answer channel produced by the
// Harmony "final" channel name.
func TestProcessor_FinalChannel(t *testing.T) {
	c := Processor{}.NewStateMachine()

	runSteps(t, "final", c, []step{
		{token: "<|start|>", channel: model.ChannelNone},
		{token: "assistant", channel: model.ChannelNone},
		{token: "<|channel|>", channel: model.ChannelNone},
		{token: "final", channel: model.ChannelNone},
		{token: "<|message|>", channel: model.ChannelNone},
		{token: "The", channel: model.ChannelAnswer, content: "The"},
		{token: " answer", channel: model.ChannelAnswer, content: " answer"},
		{token: "<|return|>", channel: model.ChannelNone, eog: true},
	})
}

// TestProcessor_AnalysisThenFinal covers the most common Harmony layout:
// reasoning followed by an answer in two separate channel blocks.
func TestProcessor_AnalysisThenFinal(t *testing.T) {
	c := Processor{}.NewStateMachine()

	tokens := []string{
		"<|start|>", "assistant",
		"<|channel|>", "analysis", "<|message|>",
		"Plan", " carefully",
		"<|end|>",
		"<|start|>", "assistant",
		"<|channel|>", "final", "<|message|>",
		"OK", " here", " is", " the", " answer",
		"<|return|>",
	}

	var reasoning, answer strings.Builder
	var sawEog bool
	for _, tok := range tokens {
		got, eog := c.Process(tok)
		switch got.Channel {
		case model.ChannelReasoning:
			reasoning.WriteString(got.Content)
		case model.ChannelAnswer:
			answer.WriteString(got.Content)
		}
		if eog {
			sawEog = true
		}
	}

	if got := reasoning.String(); got != "Plan carefully" {
		t.Errorf("reasoning = %q, want %q", got, "Plan carefully")
	}
	if got := answer.String(); got != "OK here is the answer" {
		t.Errorf("answer = %q, want %q", got, "OK here is the answer")
	}
	if !sawEog {
		t.Errorf("expected EOG to fire on <|return|>")
	}
}

// TestProcessor_CommentaryToolCall covers the "commentary to=functions.NAME"
// channel, the <|constrain|>json swallowing, and the synthetic
// ".NAME <|message|>" prefix the stateMachine injects so parseGPTToolCall can
// recover the function name.
func TestProcessor_CommentaryToolCall(t *testing.T) {
	c := Processor{}.NewStateMachine()

	runSteps(t, "tool-call", c, []step{
		{token: "<|start|>", channel: model.ChannelNone},
		{token: "assistant", channel: model.ChannelNone},
		{token: "<|channel|>", channel: model.ChannelNone},
		{token: "commentary", channel: model.ChannelNone},
		{token: " to=functions.get_weather", channel: model.ChannelNone},
		{token: "<|constrain|>", channel: model.ChannelNone},
		{token: "json", channel: model.ChannelNone}, // swallowed
		{token: "<|message|>", channel: model.ChannelTool,
			content: ".get_weather <|message|>"},
		{token: `{"location":"NYC"}`, channel: model.ChannelTool,
			content: `{"location":"NYC"}`},
		{token: "<|call|>", channel: model.ChannelNone, eog: true},
	})
}

// TestProcessor_RecoversFromMissingEnd covers the resilience path where the
// model emits <|start|> or <|channel|> without first closing the previous
// block with <|end|>.
func TestProcessor_RecoversFromMissingEnd(t *testing.T) {
	c := Processor{}.NewStateMachine()

	tokens := []string{
		"<|start|>", "assistant", "<|channel|>", "analysis", "<|message|>",
		"reasoning", " content",
		// Skipping <|end|>:
		"<|start|>", "assistant", "<|channel|>", "final", "<|message|>",
		"answer",
		"<|return|>",
	}

	var reasoning, answer strings.Builder
	for _, tok := range tokens {
		got, _ := c.Process(tok)
		switch got.Channel {
		case model.ChannelReasoning:
			reasoning.WriteString(got.Content)
		case model.ChannelAnswer:
			answer.WriteString(got.Content)
		}
	}

	if got := reasoning.String(); got != "reasoning content" {
		t.Errorf("reasoning = %q, want %q", got, "reasoning content")
	}
	if got := answer.String(); got != "answer" {
		t.Errorf("answer = %q, want %q", got, "answer")
	}
}

// TestProcessor_Reset returns the GPT stateMachine to initial state.
func TestProcessor_Reset(t *testing.T) {
	c := Processor{}.NewStateMachine()

	c.Process("<|start|>")
	c.Process("<|channel|>")
	c.Process("analysis")

	c.Reset()

	// A fresh stream should classify normally.
	got, eog := c.Process("<|start|>")
	if eog {
		t.Errorf("Reset should clear EOG state")
	}
	if got.Channel != model.ChannelNone {
		t.Errorf("after Reset, <|start|> channel = %v, want None", got.Channel)
	}
}

// TestNew_ClaimsHarmonyTemplate verifies the processor selection logic claims
// chat templates carrying Harmony markers.
func TestNew_ClaimsHarmonyTemplate(t *testing.T) {
	tests := []struct {
		name     string
		fp       model.Fingerprint
		wantHave bool
	}{
		{
			name:     "harmony-template",
			fp:       model.Fingerprint{ChatTemplate: "...<|channel|>...<|message|>..."},
			wantHave: true,
		},
		{
			name:     "non-harmony-template",
			fp:       model.Fingerprint{ChatTemplate: "{% for m in messages %}<|im_start|>{{ m.role }}<|im_end|>{% endfor %}"},
			wantHave: false,
		},
		{
			name:     "empty-fingerprint",
			fp:       model.Fingerprint{},
			wantHave: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := New(tc.fp)
			if ok != tc.wantHave {
				t.Errorf("New(%+v) ok = %v, want %v", tc.fp, ok, tc.wantHave)
			}
		})
	}
}

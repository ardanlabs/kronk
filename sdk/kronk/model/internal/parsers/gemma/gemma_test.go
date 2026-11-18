package gemma

import (
	"context"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

type step struct {
	token   string
	channel parsers.Channel
	content string
	eog     bool
}

func runSteps(t *testing.T, name string, c parsers.StateMachine, steps []step) {
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

func TestNew_ClaimsGemma(t *testing.T) {
	tests := []struct {
		name string
		fp   parsers.Fingerprint
		want bool
	}{
		// Architecture prefix (primary signal).
		{"arch-gemma2", parsers.Fingerprint{Architecture: "gemma2"}, true},
		{"arch-gemma3", parsers.Fingerprint{Architecture: "gemma3"}, true},
		{"arch-gemma4", parsers.Fingerprint{Architecture: "gemma4"}, true},
		{"arch-mixed-case", parsers.Fingerprint{Architecture: "Gemma3"}, true},

		// Chat template marker (secondary signal).
		{"template-start-of-turn", parsers.Fingerprint{ChatTemplate: "before <start_of_turn>user after"}, true},
		{"template-channel", parsers.Fingerprint{ChatTemplate: "<|channel>thought<channel|>"}, true},

		// Model name fallback.
		{"name-Gemma", parsers.Fingerprint{ModelName: "Gemma-3-27B"}, true},
		{"name-lowercase", parsers.Fingerprint{ModelName: "gemma-2-9b-it"}, true},

		// Negatives.
		{"name-llama", parsers.Fingerprint{ModelName: "Llama-3"}, false},
		{"empty", parsers.Fingerprint{}, false},
		{"arch-qwen", parsers.Fingerprint{Architecture: "qwen3"}, false},
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
		{token: "Hello", channel: parsers.ChannelAnswer, content: "Hello"},
		{token: " world", channel: parsers.ChannelAnswer, content: " world"},
	})
}

// TestParser_GemmaChannelMarker covers <|channel> ... <channel|>
// reasoning. The token immediately after <|channel> is swallowed (it's the
// channel name like "thought").
func TestParser_GemmaChannelMarker(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "gemma-channel", c, []step{
		{token: "<|channel>", channel: parsers.ChannelNone},
		{token: "thought", channel: parsers.ChannelNone}, // swallowed
		{token: "thinking", channel: parsers.ChannelReasoning, content: "thinking"},
		{token: "<channel|>", channel: parsers.ChannelNone},
		{token: "answer", channel: parsers.ChannelAnswer, content: "answer"},
	})
}

func TestParser_StructuralMarkersSkipped(t *testing.T) {
	c := Parser{}.NewStateMachine()
	for _, m := range []string{"<tool_call|>", "<|tool_response>", "<tool_response|>"} {
		c.Reset()
		got, eog := c.Classify(m)
		if eog {
			t.Errorf("structural marker %q should not be eog", m)
		}
		if got.Content != "" || got.Channel != parsers.ChannelNone {
			t.Errorf("structural marker %q should be silent, got %+v", m, got)
		}
	}
}

func TestParser_ToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "tool-call", c, []step{
		{token: "<tool_call>", channel: parsers.ChannelNone},
		{token: `call:get_weather{location:<|"|>NYC<|"|>}`, channel: parsers.ChannelNone},
		{token: "</tool_call>", channel: parsers.ChannelTool,
			content: `call:get_weather{location:<|"|>NYC<|"|>}` + "\n"},
	})
	result, eog := c.Classify("done")
	if eog {
		t.Error("unexpected continuation after a tool call must be preserved for final validation")
	}
	if result.Channel != parsers.ChannelTool || result.Content != "done" {
		t.Errorf("unexpected continuation: got %+v, want tool content %q", result, "done")
	}
}

func TestParser_PreservesMalformedTrailingContent(t *testing.T) {
	tests := []struct {
		name string
		tail []string
	}{
		{name: "whole delimiter", tail: []string{`<|"|>}`}},
		{name: "split delimiter", tail: []string{`<|`, `"|>}`}},
		{name: "prefixed delimiter", tail: []string{`unexpected<|"|>}`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Parser{}.NewStateMachine()
			var tooling strings.Builder
			tokens := []string{
				"<tool_call>",
				`call:write_file{content:<|"|><|"|>}`,
				"</tool_call>",
				"<tool_call>",
				`call:bash{command:<|"|>id<|"|>}`,
				"</tool_call>",
			}
			tokens = append(tokens, tt.tail...)

			for _, token := range tokens {
				result, eog := c.Classify(token)
				if eog {
					t.Fatalf("Classify(%q): got EOG before malformed output was preserved", token)
				}
				if result.Channel == parsers.ChannelTool {
					tooling.WriteString(result.Content)
				}
			}

			calls := Parser{}.ToolCall(context.Background(), nil, tooling.String())
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

func TestParser_FlushIncompleteToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<tool_call>")
	c.Classify(`call:get_weather{location:<|"|>NYC<|"|>}`)

	flusher := c.(parsers.StateMachineFlusher)
	got := flusher.Flush()
	want := `call:get_weather{location:<|"|>NYC<|"|>}` + "\n"
	if got.Channel != parsers.ChannelTool || got.Content != want {
		t.Errorf("Flush: got {%v %q}, want {%v %q}", got.Channel, got.Content, parsers.ChannelTool, want)
	}
	if got := flusher.Flush(); got != (parsers.Result{}) {
		t.Errorf("second Flush: got %+v, want zero result", got)
	}
}

func TestParser_ForeignMarkersAreContent(t *testing.T) {
	c := Parser{}.NewStateMachine()
	for _, m := range []string{"[TOOL_CALLS]", "<function=x>", "<think>"} {
		c.Reset()
		got, eog := c.Classify(m)
		if eog {
			t.Errorf("gemma should not EOG on foreign marker %q", m)
		}
		if got.Channel != parsers.ChannelAnswer || got.Content != m {
			t.Errorf("gemma should pass-through %q, got %+v", m, got)
		}
	}
}

func TestParser_Reset(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<|channel>")
	c.Classify("thought")
	c.Reset()
	got, _ := c.Classify("hi")
	if got.Channel != parsers.ChannelAnswer || got.Content != "hi" {
		t.Errorf("after Reset got %+v", got)
	}
}

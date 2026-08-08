package toolcall

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

// TestNew verifies the parser claims only templates that explicitly declare
// the marked JSON tool-call protocol.
func TestNew(t *testing.T) {
	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		{"marked JSON", model.Fingerprint{ChatTemplate: `return <tool_call>{"name":"x","arguments":{}}</tool_call>`}, true},
		{"RNJ template", model.Fingerprint{Architecture: "gemma3", ChatTemplate: `For each function call, return a json object with function name and arguments within <tool_call></tool_call> XML tags: {"name": <function-name>, "arguments": <args-json-object>}`}, true},
		{"markers without JSON envelope", model.Fingerprint{ChatTemplate: `<tool_call>native syntax</tool_call>`}, false},
		{"architecture only", model.Fingerprint{Architecture: "gemma3"}, false},
		{"empty", model.Fingerprint{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := New(tt.fp)
			if got != tt.want {
				t.Errorf("New: got %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Parser
// =============================================================================

// TestParser_PureAnswer covers a vanilla generation with no markers.
func TestParser_PureAnswer(t *testing.T) {
	c := Parser{}.NewStateMachine()

	runSteps(t, "pure-answer", c, []step{
		{token: "Hello", channel: model.ChannelAnswer, content: "Hello"},
		{token: ", ", channel: model.ChannelAnswer, content: ", "},
		{token: "world", channel: model.ChannelAnswer, content: "world"},
	})
}

// TestParser_ReasoningThenAnswer verifies <think>…</think> wrapping.
func TestParser_ReasoningThenAnswer(t *testing.T) {
	c := Parser{}.NewStateMachine()

	runSteps(t, "reasoning-then-answer", c, []step{
		{token: "<think>", channel: model.ChannelNone},
		{token: "Let", channel: model.ChannelReasoning, content: "Let"},
		{token: " me", channel: model.ChannelReasoning, content: " me"},
		{token: "</think>", channel: model.ChannelNone},
		{token: "Hi", channel: model.ChannelAnswer, content: "Hi"},
	})
}

// TestParser_SingleToolCall covers <tool_call>JSON</tool_call>.
func TestParser_SingleToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()

	runSteps(t, "single-tool-call", c, []step{
		{token: "<tool_call>", channel: model.ChannelNone},
		{token: `{"name":"get_weather"`, channel: model.ChannelNone},
		{token: `,"arguments":{"loc":"NYC"}}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: `{"name":"get_weather","arguments":{"loc":"NYC"}}` + "\n"},
	})

	if _, eog := c.Classify("\n"); eog {
		t.Errorf("unexpected EOG while waiting through whitespace after tool call")
	}
	got, _ := c.Classify("done")
	if got.Channel != model.ChannelTool || !strings.Contains(got.Content, "unexpected-content-after-tool") {
		t.Errorf("trailing content: got %+v, want invalid tool evidence", got)
	}
}

func TestParser_FlushIncompleteToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<tool_call>")
	c.Classify(`{"name":"get_weather","arguments":{"loc":"NYC"}}`)

	flusher := c.(model.StateMachineFlusher)
	got := flusher.Flush()
	if got.Channel != model.ChannelTool || !strings.Contains(got.Content, "missing-tool-call-close") {
		t.Errorf("Flush: got {%v %q}, want invalid missing-close evidence", got.Channel, got.Content)
	}
	if got := flusher.Flush(); got != (model.Result{}) {
		t.Errorf("second Flush: got %+v, want zero result", got)
	}
}

func TestParser_EmptyMarkedCallReachesToolParser(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<tool_call>")
	got, _ := c.Classify("</tool_call>")
	if got.Channel != model.ChannelTool || got.Content == "" {
		t.Errorf("empty marked call: got %+v, want non-empty tool result", got)
	}
}

// TestParser_MultipleToolCalls verifies that a second opener after the
// first close is accepted (no EOG) and accumulates a fresh buffer.
func TestParser_MultipleToolCalls(t *testing.T) {
	c := Parser{}.NewStateMachine()

	runSteps(t, "multi-tool-call", c, []step{
		{token: "<tool_call>", channel: model.ChannelNone},
		{token: `{"name":"a","arguments":{}}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: `{"name":"a","arguments":{}}` + "\n"},
		{token: "\n", channel: model.ChannelNone},
		{token: "<|tool_call>", channel: model.ChannelNone},
		{token: `{"name":"b","arguments":{}}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: `{"name":"b","arguments":{}}` + "\n"},
	})

	got, _ := c.Classify("done")
	if got.Channel != model.ChannelTool || !strings.Contains(got.Content, "unexpected-content-after-tool") {
		t.Errorf("trailing content: got %+v, want invalid tool evidence", got)
	}
}

func TestParser_CoalescedTransitionsAndMalformedSiblings(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantAnswer string
		wantTool   string
	}{
		{
			name:       "answer then tool",
			input:      `prefix<tool_call>{"name":"ok","arguments":{}}</tool_call>`,
			wantAnswer: "prefix",
			wantTool:   `{"name":"ok","arguments":{}}` + "\n",
		},
		{
			name:     "empty sibling",
			input:    `<tool_call>{"name":"ok","arguments":{}}</tool_call><tool_call></tool_call>`,
			wantTool: `{"name":"ok","arguments":{}}` + "\n<empty-tool-call>\n",
		},
		{
			name:     "trailing junk",
			input:    `<tool_call>{"name":"ok","arguments":{}}</tool_call>junk`,
			wantTool: `{"name":"ok","arguments":{}}` + "\n<unexpected-content-after-tool>junk",
		},
		{
			name:     "cross-wrapper splice",
			input:    `<tool_call>{"name":"bash","arguments":{</tool_call><tool_call>"command":"id"}}</tool_call>`,
			wantTool: `{"name":"bash","arguments":{<invalid-tool-call-boundary>` + "\n" + `"command":"id"}}<invalid-tool-call-boundary>` + "\n",
		},
		{
			name:     "valid then cross-wrapper splice",
			input:    `<tool_call>{"name":"ok","arguments":{}}{"name":"bash","arguments":{</tool_call><tool_call>"command":"id"}}</tool_call>`,
			wantTool: `{"name":"ok","arguments":{}}{"name":"bash","arguments":{<invalid-tool-call-boundary>` + "\n" + `"command":"id"}}<invalid-tool-call-boundary>` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Parser{}.NewStateMachine()
			first, _ := c.Classify(tt.input)
			results := []model.Result{first}
			flusher := c.(model.StateMachineFlusher)
			for result := flusher.Flush(); result != (model.Result{}); result = flusher.Flush() {
				results = append(results, result)
			}
			var answer, tool strings.Builder
			for _, result := range results {
				switch result.Channel {
				case model.ChannelAnswer:
					answer.WriteString(result.Content)
				case model.ChannelTool:
					tool.WriteString(result.Content)
				}
			}
			if answer.String() != tt.wantAnswer || tool.String() != tt.wantTool {
				t.Errorf("got answer %q tool %q, want answer %q tool %q", answer.String(), tool.String(), tt.wantAnswer, tt.wantTool)
			}
		})
	}
}

func TestParser_PreservesFramingAtEverySplit(t *testing.T) {
	inputs := []string{
		`<tool_call>{"name":"ok","arguments":{"text":"</tool_call>"}}</tool_call>`,
		`<|tool_call>{"name":"ok","arguments":{}}<tool_call|>`,
		`<tool_call>{"name":"ok","arguments":{}}<tool_call|>`,
		`<|tool_call>{"name":"ok","arguments":{}}</tool_call>`,
		`<tool_call>{"name":"ok","arguments":{}}</tool_call><tool_call>`,
		`<tool_call>{"name":"ok","arguments":{}}</tool_call><tool`,
		`<tool_call>{"name":"bash","arguments":{</tool_call><tool_call>"command":"id"}}</tool_call>`,
		`<tool_call>{"name":"ok","arguments":{}}{"name":"bash","arguments":{</tool_call><tool_call>"command":"id"}}</tool_call>`,
	}
	for _, input := range inputs {
		var baseline string
		for split := 0; split <= len(input); split++ {
			c := Parser{}.NewStateMachine()
			var tool strings.Builder
			for _, fragment := range []string{input[:split], input[split:]} {
				result, _ := c.Classify(fragment)
				if result.Channel == model.ChannelTool {
					tool.WriteString(result.Content)
				}
			}
			for result := c.(model.StateMachineFlusher).Flush(); result != (model.Result{}); result = c.(model.StateMachineFlusher).Flush() {
				if result.Channel == model.ChannelTool {
					tool.WriteString(result.Content)
				}
			}
			if split == 0 {
				baseline = tool.String()
			} else if tool.String() != baseline {
				t.Fatalf("input %q split %d: tool = %q, want %q", input, split, tool.String(), baseline)
			}
		}
	}
}

// TestParser_UnknownMarkersAreContent verifies that markers belonging to
// other parsers (e.g. Mistral [TOOL_CALLS], Gemma <|channel>) are treated
// as plain content by the toolcall stateMachine — the more-specific parsers
// own those markers.
func TestParser_UnknownMarkersAreContent(t *testing.T) {
	c := Parser{}.NewStateMachine()

	for _, marker := range []string{"[TOOL_CALLS]", "<|channel>", "<function=foo>"} {
		c.Reset()
		got, eog := c.Classify(marker)
		if eog {
			t.Errorf("toolcall should not EOG on foreign marker %q", marker)
		}
		if got.Channel != model.ChannelAnswer || got.Content != marker {
			t.Errorf("toolcall should pass-through %q as answer content, got %+v",
				marker, got)
		}
	}
}

// TestParser_Reset returns the state machine to initial state.
func TestParser_Reset(t *testing.T) {
	c := Parser{}.NewStateMachine()

	c.Classify("<think>")
	c.Classify("partial")
	c.Reset()

	got, eog := c.Classify("hello")
	if eog {
		t.Errorf("Reset should clear EOG state")
	}
	if got.Channel != model.ChannelAnswer || got.Content != "hello" {
		t.Errorf("after Reset, got %+v, want {Answer, %q}", got, "hello")
	}
}

// TestParser_PortParity drives a long mixed stream and asserts the
// per-channel accumulation matches expectations.
func TestParser_PortParity(t *testing.T) {
	c := Parser{}.NewStateMachine()

	tokens := []string{
		"<think>", "Plan", " carefully", "</think>",
		"OK", " here", " goes", ":",
		"<tool_call>", `{"name":"x","arguments":{}}`, "</tool_call>",
	}

	var reasoning, answer, tool strings.Builder
	for _, tok := range tokens {
		got, _ := c.Classify(tok)
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

package gemma

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
// Parser
// =============================================================================

func TestParser_PureAnswer(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "pure-answer", c, []step{
		{token: "Hello", channel: model.ChannelAnswer, content: "Hello"},
		{token: " world", channel: model.ChannelAnswer, content: " world"},
	})
}

// TestParser_GemmaChannelMarker covers <|channel> ... <channel|>
// reasoning. The token immediately after <|channel> is swallowed (it's the
// channel name like "thought").
func TestParser_GemmaChannelMarker(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "gemma-channel", c, []step{
		{token: "<|channel>", channel: model.ChannelNone},
		{token: "thought", channel: model.ChannelNone}, // swallowed
		{token: "thinking", channel: model.ChannelReasoning, content: "thinking"},
		{token: "<channel|>", channel: model.ChannelNone},
		{token: "answer", channel: model.ChannelAnswer, content: "answer"},
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
		if got.Content != "" || got.Channel != model.ChannelNone {
			t.Errorf("structural marker %q should be silent, got %+v", m, got)
		}
	}
}

func TestParser_ToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "tool-call", c, []step{
		{token: "<tool_call>", channel: model.ChannelTool},
		{token: `call:get_weather{location:<|"|>NYC<|"|>}`, channel: model.ChannelNone},
		{token: "</tool_call>", channel: model.ChannelTool,
			content: encodeGemmaWrapperFrame(`call:get_weather{location:<|"|>NYC<|"|>}`+"\n", true)},
	})
	result, eog := c.Classify("done")
	if eog {
		t.Error("unexpected continuation after a tool call must be preserved for final validation")
	}
	if result.Channel != model.ChannelTool || result.Content != "done" {
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
				if result.Channel == model.ChannelTool {
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

	flusher := c.(model.StateMachineFlusher)
	got := flusher.Flush()
	want := encodeGemmaWrapperFrame(`call:get_weather{location:<|"|>NYC<|"|>}`+"\n", false)
	if got.Channel != model.ChannelTool || got.Content != want {
		t.Errorf("Flush: got {%v %q}, want {%v %q}", got.Channel, got.Content, model.ChannelTool, want)
	}
	if got := flusher.Flush(); got != (model.Result{}) {
		t.Errorf("second Flush: got %+v, want zero result", got)
	}
}

func TestParser_WrappedCallPreservesNativeMarkerTextInArguments(t *testing.T) {
	c := Parser{}.NewStateMachine()
	var tooling strings.Builder
	for _, token := range []string{"<tool_call>", `call:write{text:<|"|>before `, "</tool_call>", "<tool_call>", ` after<|"|>}`, "</tool_call>"} {
		result, _ := c.Classify(token)
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}

	calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
	if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Arguments["text"] != "before </tool_call><tool_call> after" {
		t.Fatalf("ToolCall: got %+v, want original marker text in argument", calls)
	}
	if got := (Parser{}).StripToolCallMarkup(tooling.String()); got != "" {
		t.Errorf("StripToolCallMarkup: got %q, want empty", got)
	}
}

func TestParser_StateMachineWrapperEvidenceStripsInvalidBodies(t *testing.T) {
	for _, complete := range []bool{true, false} {
		t.Run(map[bool]string{true: "complete", false: "missing close"}[complete], func(t *testing.T) {
			c := Parser{}.NewStateMachine()
			var tooling strings.Builder
			for _, token := range []string{"<tool_call>", "ordinary"} {
				result, eog := c.Classify(token)
				if eog {
					t.Fatalf("Classify(%q): got unexpected EOG", token)
				}
				if result.Channel == model.ChannelTool {
					tooling.WriteString(result.Content)
				}
			}
			if complete {
				result, eog := c.Classify("</tool_call>")
				if eog {
					t.Fatal("Classify(close): got unexpected EOG")
				}
				tooling.WriteString(result.Content)
			} else {
				tooling.WriteString(c.(model.StateMachineFlusher).Flush().Content)
			}

			if got := (Parser{}).StripToolCallMarkup(tooling.String()); got != "" {
				t.Errorf("StripToolCallMarkup(%q): got %q, want empty", tooling.String(), got)
			}
		})
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
		if got.Channel != model.ChannelAnswer || got.Content != m {
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
	if got.Channel != model.ChannelAnswer || got.Content != "hi" {
		t.Errorf("after Reset got %+v", got)
	}
}

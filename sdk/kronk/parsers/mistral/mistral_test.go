package mistral

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
// Parser
// =============================================================================

func TestParser_PureAnswer(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "pure-answer", c, []step{
		{token: "Hello", channel: model.ChannelAnswer, content: "Hello"},
		{token: ", ", channel: model.ChannelAnswer, content: ", "},
		{token: "world", channel: model.ChannelAnswer, content: "world"},
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

// TestParser_StreamingToolCall verifies native buffering and split delimiter
// identity detection.
func TestParser_StreamingToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	streamer := c.(model.ToolCallDeltaStreamer)
	flusher := c.(model.StateMachineFlusher)
	runSteps(t, "streaming-tool-call", c, []step{
		{token: "[TOOL_CALLS]", channel: model.ChannelNone},
		{token: "get_", channel: model.ChannelNone},
		{token: "weather", channel: model.ChannelNone},
		{token: "[AR", channel: model.ChannelNone},
		{token: "GS]", channel: model.ChannelNone},
		{token: `{"loc":"NYC"}`, channel: model.ChannelNone},
	})
	deltas := streamer.ToolCallDeltas()
	if len(deltas) != 1 || deltas[0].Index != 0 || deltas[0].ID == "" || deltas[0].Function.Name != "get_weather" {
		t.Fatalf("ToolCallDeltas: got %+v", deltas)
	}
	got := flusher.Flush()
	want := `[TOOL_CALLS]get_weather[ARGS]{"loc":"NYC"}`
	if got.Channel != model.ChannelTool || got.Content != want {
		t.Errorf("Flush: got %+v, want %q", got, want)
	}
}

// TestParser_RepeatedMarkerInsideToolMode verifies that a second
// [TOOL_CALLS] inside tool mode is silent (state already correct).
func TestParser_RepeatedMarkerInsideToolMode(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("[TOOL_CALLS]")
	c.Classify("a")
	got, _ := c.Classify("[TOOL_CALLS]")
	if got.Channel != model.ChannelNone || got.Content != "" {
		t.Errorf("repeated [TOOL_CALLS] should be silent, got %+v", got)
	}
}

func TestParser_MultipleToolCallsAndReset(t *testing.T) {
	c := Parser{}.NewStateMachine()
	streamer := c.(model.ToolCallDeltaStreamer)
	flusher := c.(model.StateMachineFlusher)
	input := []string{"[TOOL_CALLS]", "first[ARGS]{}", "[TOOL_", "CALLS]second[AR", "GS]", `{"x":1}`}
	for _, token := range input {
		got, _ := c.Classify(token)
		if got != (model.Result{}) {
			t.Errorf("Classify(%q): got incremental payload %+v", token, got)
		}
	}
	deltas := streamer.ToolCallDeltas()
	if len(deltas) != 2 || deltas[0].Function.Name != "first" || deltas[1].Function.Name != "second" || deltas[0].Index != 0 || deltas[1].Index != 1 || deltas[0].ID == deltas[1].ID {
		t.Fatalf("ToolCallDeltas: got %+v", deltas)
	}
	want := strings.Join(input, "")
	if got := flusher.Flush(); got.Content != want || got.Channel != model.ChannelTool {
		t.Errorf("Flush: got %+v, want %q", got, want)
	}
	c.Reset()
	if len(streamer.StartedToolCalls()) != 0 || flusher.Flush() != (model.Result{}) {
		t.Errorf("Reset did not clear tool state")
	}
}

func TestParser_ToolMarkerInsideArgumentsIsNotActivity(t *testing.T) {
	c := Parser{}.NewStateMachine()
	streamer := c.(model.ToolCallDeltaStreamer)

	for _, token := range []string{
		"[TOOL_CALLS]",
		`first[ARGS]{"text":"[TOOL_CALLS]fake[ARGS]{}"}`,
		`[TOOL_CALLS]second[ARGS]{}`,
	} {
		c.Classify(token)
	}

	deltas := streamer.ToolCallDeltas()
	if len(deltas) != 2 || deltas[0].Function.Name != "first" || deltas[1].Function.Name != "second" {
		t.Fatalf("ToolCallDeltas: got %+v, want first and second only", deltas)
	}
}

func TestParser_ForeignMarkersAreContent(t *testing.T) {
	c := Parser{}.NewStateMachine()
	for _, m := range []string{"<tool_call>", "<|channel>", "call:foo", "<function=x>"} {
		c.Reset()
		got, eog := c.Classify(m)
		if eog {
			t.Errorf("mistral should not EOG on foreign marker %q", m)
		}
		if got.Channel != model.ChannelAnswer || got.Content != m {
			t.Errorf("mistral should pass-through %q, got %+v", m, got)
		}
	}
}

func TestParser_Reset(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("[TOOL_CALLS]")
	c.Reset()
	got, _ := c.Classify("hello")
	if got.Channel != model.ChannelAnswer || got.Content != "hello" {
		t.Errorf("after Reset got %+v", got)
	}
}

package gpt

import (
	"strconv"
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

// TestParser_AnalysisChannel covers the reasoning channel produced by
// the Harmony "analysis" channel name.
func TestParser_AnalysisChannel(t *testing.T) {
	c := Parser{}.NewStateMachine()

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

// TestParser_FinalChannel covers the answer channel produced by the
// Harmony "final" channel name.
func TestParser_FinalChannel(t *testing.T) {
	c := Parser{}.NewStateMachine()

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

// TestParser_AnalysisThenFinal covers the most common Harmony layout:
// reasoning followed by an answer in two separate channel blocks.
func TestParser_AnalysisThenFinal(t *testing.T) {
	c := Parser{}.NewStateMachine()

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
		got, eog := c.Classify(tok)
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

// TestParser_CommentaryToolCall covers the "commentary to=functions.NAME"
// channel, early identity delta, and buffered native payload.
func TestParser_CommentaryToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	streamer := c.(model.ToolCallDeltaStreamer)
	flusher := c.(model.StateMachineFlusher)

	runSteps(t, "tool-call", c, []step{
		{token: "<|start|>", channel: model.ChannelNone},
		{token: "assistant", channel: model.ChannelNone},
		{token: "<|channel|>", channel: model.ChannelNone},
		{token: "commentary", channel: model.ChannelNone},
		{token: " to=functions.get_weather", channel: model.ChannelNone},
		{token: "<|constrain|>", channel: model.ChannelNone},
		{token: "json", channel: model.ChannelNone}, // swallowed
		{token: "<|message|>", channel: model.ChannelNone},
		{token: `{"location":"NYC"}`, channel: model.ChannelNone},
		{token: "<|call|>", channel: model.ChannelNone, eog: true},
	})

	if deltas := streamer.ToolCallDeltas(); len(deltas) != 0 {
		t.Fatalf("ToolCallDeltas: got %+v, want identities deferred until final validation", deltas)
	}
	if got := streamer.ToolCallDeltas(); got != nil {
		t.Errorf("drained ToolCallDeltas: got %+v, want nil", got)
	}
	if got := flusher.Flush(); got.Channel != model.ChannelTool || got.Content != `.get_weather <|message|>{"location":"NYC"}` {
		t.Errorf("Flush after parser EOG: got %+v, want buffered tool call", got)
	}
}

func TestParser_AnalysisToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	stream := `<|start|>assistant<|channel|>analysis to=functions.get_weather<|constrain|>json<|message|>{"location":"London"}<|call|>`

	result, eog := c.Classify(stream)
	if !eog {
		t.Fatal("Classify: got eog false, want true")
	}
	if result != (model.Result{}) {
		t.Errorf("Classify: got %+v, want empty result before flush", result)
	}

	tooling := c.(model.StateMachineFlusher).Flush()
	if tooling.Channel != model.ChannelTool {
		t.Fatalf("Flush: got channel %v, want %v", tooling.Channel, model.ChannelTool)
	}
	calls := Parser{}.ToolCall(t.Context(), nil, tooling.Content)
	if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "get_weather" {
		t.Fatalf("ToolCall: got %+v, want one valid get_weather call", calls)
	}
	if got := calls[0].Function.Arguments["location"]; got != "London" {
		t.Errorf("location: got %v, want London", got)
	}
}

func TestParser_RoleRecipientToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	stream := `<|start|>assistant to=functions.get_weather<|channel|>commentary<|constrain|>json<|message|>{"location":"London"}<|call|>`

	_, eog := c.Classify(stream)
	if !eog {
		t.Fatal("Classify: got eog false, want true")
	}
	tooling := c.(model.StateMachineFlusher).Flush()
	calls := Parser{}.ToolCall(t.Context(), nil, tooling.Content)
	if tooling.Channel != model.ChannelTool || len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "get_weather" {
		t.Fatalf("ToolCall: got tooling %+v, calls %+v; want one valid get_weather call", tooling, calls)
	}
}

func TestParser_VocabEOGToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	stream := `<|start|>assistant to=functions.get_weather<|channel|>commentary<|constrain|>json<|message|>{"location":"London"}`

	if _, eog := c.Classify(stream); eog {
		t.Fatal("Classify: got eog true before the call marker")
	}
	c.(model.VocabEOGConsumer).ConsumeVocabEOG("<|call|>")
	tooling := c.(model.StateMachineFlusher).Flush()
	calls := Parser{}.ToolCall(t.Context(), nil, tooling.Content)
	if tooling.Channel != model.ChannelTool || len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "get_weather" {
		t.Fatalf("ToolCall: got tooling %+v, calls %+v; want one valid get_weather call", tooling, calls)
	}
}

func TestParser_ToolCallFlushMultipleAndReset(t *testing.T) {
	c := Parser{}.NewStateMachine()
	streamer := c.(model.ToolCallDeltaStreamer)
	flusher := c.(model.StateMachineFlusher)

	for _, token := range []string{"<|channel|>", "commentary to=functions.", "first", "<|message|>", `{}`, "<|end|>", "<|channel|>", "commentary to=functions.second", "<|message|>", `{"x":1}`} {
		c.Classify(token)
	}
	if deltas := streamer.ToolCallDeltas(); len(deltas) != 0 {
		t.Fatalf("ToolCallDeltas: got %+v, want identities deferred until final validation", deltas)
	}
	got := flusher.Flush()
	if got.Channel != model.ChannelTool || got.Content != `.second <|message|>{"x":1}<|missing-end|>` {
		t.Errorf("Flush: got %+v", got)
	}
	c.Reset()
	if len(streamer.StartedToolCalls()) != 0 || flusher.Flush() != (model.Result{}) {
		t.Errorf("Reset did not clear tool state")
	}
}

func TestParser_FlushIncompleteCommentaryFraming(t *testing.T) {
	for _, content := range []string{"commentary", "commentary to=functions.weather"} {
		t.Run(content, func(t *testing.T) {
			sm := Parser{}.NewStateMachine()
			if got, _ := sm.Classify("<|channel|>" + content); got != (model.Result{}) {
				t.Fatalf("Classify: got %+v, want buffered", got)
			}
			got := sm.(model.StateMachineFlusher).Flush()
			want := incompleteFramingMarker + content
			if got.Channel != model.ChannelTool || got.Content != want {
				t.Fatalf("Flush: got %+v, want tool content %q", got, want)
			}
			if stripped := (Parser{}).StripToolCallMarkup(got.Content); stripped != "" {
				t.Errorf("StripToolCallMarkup: got %q, want empty", stripped)
			}
		})
	}
}

// TestParser_RecoversFromMissingEnd covers the resilience path where the
// model emits <|start|> or <|channel|> without first closing the previous
// block with <|end|>.
func TestParser_RecoversFromMissingEnd(t *testing.T) {
	c := Parser{}.NewStateMachine()

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
		got, _ := c.Classify(tok)
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

// TestParser_Reset returns the GPT stateMachine to initial state.
func TestParser_Reset(t *testing.T) {
	c := Parser{}.NewStateMachine()

	c.Classify("<|start|>")
	c.Classify("<|channel|>")
	c.Classify("analysis")

	c.Reset()

	// A fresh stream should classify normally.
	got, eog := c.Classify("<|start|>")
	if eog {
		t.Errorf("Reset should clear EOG state")
	}
	if got.Channel != model.ChannelNone {
		t.Errorf("after Reset, <|start|> channel = %v, want None", got.Channel)
	}
}

func TestParser_HarmonyEverySplitPoint(t *testing.T) {
	stream := `<|start|>assistant<|channel|>commentary to=functions.echo<|constrain|>json<|message|>{"text":"<|call|> and \\"quoted\\""}<|call|>`
	for split := 0; split <= len(stream); split++ {
		t.Run(strconv.Itoa(split), func(t *testing.T) {
			c := Parser{}.NewStateMachine()
			flusher := c.(model.StateMachineFlusher)
			var results []model.Result
			for _, chunk := range []string{stream[:split], stream[split:]} {
				if result, _ := c.Classify(chunk); result.Channel != model.ChannelNone {
					results = append(results, result)
				}
			}
			for result := flusher.Flush(); result.Channel != model.ChannelNone; result = flusher.Flush() {
				results = append(results, result)
			}
			if len(results) != 1 || results[0].Channel != model.ChannelTool || results[0].Content != `.echo <|message|>{"text":"<|call|> and \\"quoted\\""}` {
				t.Errorf("split %d: got %+v", split, results)
			}
		})
	}
}

func TestParser_EndDoesNotAuthorizeToolCalls(t *testing.T) {
	c := Parser{}.NewStateMachine()
	flusher := c.(model.StateMachineFlusher)
	stream := `<|channel|>commentary to=functions.one<|message|>{}<|end|><|channel|>commentary to=functions.two<|message|>{"x":2}<|end|>`
	var contents []string
	if result, _ := c.Classify(stream); result.Channel == model.ChannelTool {
		contents = append(contents, result.Content)
	}
	for result := flusher.Flush(); result.Channel != model.ChannelNone; result = flusher.Flush() {
		contents = append(contents, result.Content)
	}
	got := strings.Join(contents, "")
	calls := Parser{}.ToolCall(t.Context(), nil, got)
	if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
		t.Errorf("got content %q, calls %+v; want one non-executable failure", got, calls)
	}
}

func TestParser_MalformedToolEvidenceSurvivesLaterFinalChannel(t *testing.T) {
	c := Parser{}.NewStateMachine()
	stream := `<|channel|>commentary to=functions.echo<|constrain|>json<|message|>{}<|end|><|channel|>final<|message|>ok<|return|>`
	result, eog := c.Classify(stream)
	var tooling, answer strings.Builder
	for result != (model.Result{}) {
		switch result.Channel {
		case model.ChannelTool:
			tooling.WriteString(result.Content)
		case model.ChannelAnswer:
			answer.WriteString(result.Content)
		}
		result = c.(model.StateMachineFlusher).Flush()
	}
	if !eog || answer.String() != "ok" {
		t.Fatalf("got eog=%v answer=%q, want final answer retained", eog, answer.String())
	}
	calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
	if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
		t.Fatalf("tooling %q parsed as %+v, want one non-executable failure", tooling.String(), calls)
	}
}

func TestParser_OnlyCallAuthorizesTool(t *testing.T) {
	for _, marker := range []string{"<|call|>", "<|return|>", "<|end|>"} {
		t.Run(marker, func(t *testing.T) {
			c := Parser{}.NewStateMachine()
			result, eog := c.Classify(`<|channel|>commentary to=functions.echo<|constrain|>json<|message|>{}` + marker)
			var tooling strings.Builder
			if result.Channel == model.ChannelTool {
				tooling.WriteString(result.Content)
			}
			for result = c.(model.StateMachineFlusher).Flush(); result != (model.Result{}); result = c.(model.StateMachineFlusher).Flush() {
				if result.Channel == model.ChannelTool {
					tooling.WriteString(result.Content)
				}
			}
			calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
			if marker == "<|call|>" {
				if !eog || len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "echo" {
					t.Fatalf("got eog=%v calls=%+v, want executable echo", eog, calls)
				}
				return
			}
			if marker == "<|return|>" && !eog {
				t.Error("return marker did not signal EOG")
			}
			if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
				t.Fatalf("got %+v, want one non-executable failure", calls)
			}
		})
	}
}

func TestParser_RejectsMalformedConstraintAndPostEOGContent(t *testing.T) {
	for _, stream := range []string{
		`<|channel|>commentary to=functions.echo<|constrain|>not-json<|message|>{}<|call|>`,
		`<|channel|>commentary to=functions.echo<|call|>`,
		`<|channel|>commentary to=functions.bad/name<|constrain|>json<|message|>{}<|call|>`,
		`<|channel|>commentary to=functions.<|constrain|>json<|message|>{}<|call|>`,
		"<|channel|>commentary to=functions.echo\u00a0<|constrain|>json<|message|>{}<|call|>",
		`<|channel|>commentary to=functions.echo<|message|>{}<|call|><|channel|>commentary to=functions.other<|message|>{}<|call|>`,
	} {
		c := Parser{}.NewStateMachine()
		var tooling strings.Builder
		if result, _ := c.Classify(stream); result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
		for result := c.(model.StateMachineFlusher).Flush(); result != (model.Result{}); result = c.(model.StateMachineFlusher).Flush() {
			if result.Channel == model.ChannelTool {
				tooling.WriteString(result.Content)
			}
		}
		calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
		if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
			t.Fatalf("stream %q: content %q calls %+v, want one non-executable failure", stream, tooling.String(), calls)
		}
	}
}

// TestNew_ClaimsHarmonyTemplate verifies the parser selection logic claims
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

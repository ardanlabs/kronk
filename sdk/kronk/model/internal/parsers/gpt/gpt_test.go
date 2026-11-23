package gpt

import (
	"strconv"
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

func TestParser_TokenProvenance(t *testing.T) {
	for _, tc := range []struct {
		name, channel, payload string
		want                   parsers.Channel
	}{
		{"answer", "final", `Literal <|channel|>analysis<|message|>not reasoning<|return|> still answer.`, parsers.ChannelAnswer},
		{"reasoning", "analysis", `Literal <|end|><|start|>assistant<|channel|>final<|message|>still reasoning.`, parsers.ChannelReasoning},
		{"tool", "commentary to=functions.echo", `{"text":"<|channel|>final<|message|><|end|><|call|>","n":9007199254740993}`, parsers.ChannelTool},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for split := range len(tc.payload) + 2 {
				sm := Parser{}.NewStateMachine()
				consumer := sm.(parsers.TokenStateMachine)
				got := make(map[parsers.Channel]string)
				feed := func(content string, special bool) {
					t.Helper()
					r, eog := consumer.ClassifyToken(content, special)
					got[r.Channel] += r.Content
					if eog {
						t.Fatalf("split %d: premature EOG for %q, special=%t", split, content, special)
					}
				}
				feed("<|channel|>", true)
				feed(tc.channel, false)
				feed("<|message|>", true)
				if split <= len(tc.payload) {
					feed(tc.payload[:split], false)
					feed(tc.payload[split:], false)
				} else {
					for i := range len(tc.payload) {
						feed(tc.payload[i:i+1], false)
					}
				}
				end := "<|return|>"
				if tc.want == parsers.ChannelTool {
					end = "<|call|>"
				}
				sm.(parsers.VocabEOGConsumer).ConsumeVocabEOG(end)
				flusher := sm.(parsers.StateMachineFlusher)
				for r := flusher.Flush(); r != (parsers.Result{}); r = flusher.Flush() {
					got[r.Channel] += r.Content
				}
				want := tc.payload
				if tc.want == parsers.ChannelTool {
					want = ".echo <|message|>" + tc.payload
					calls := (Parser{}).ToolCall(t.Context(), nil, got[tc.want])
					if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "echo" || calls[0].Function.Arguments["text"] != "<|channel|>final<|message|><|end|><|call|>" {
						t.Fatalf("split %d: got calls %+v, want echo with literal markers", split, calls)
					}
				}
				if got[tc.want] != want {
					t.Fatalf("split %d: got %q, want %q", split, got[tc.want], want)
				}
				delete(got, tc.want)
				for channel, text := range got {
					if text != "" {
						t.Fatalf("split %d: unexpected %s content %q", split, channel, text)
					}
				}
				sm.Reset()
				feed("<|channel|>", true)
				feed("final", false)
				feed("<|message|>", true)
				r, eog := consumer.ClassifyToken("next request", false)
				if r != (parsers.Result{Channel: parsers.ChannelAnswer, Content: "next request"}) || eog {
					t.Fatalf("slot reuse: got %+v, EOG=%t", r, eog)
				}
			}
		})
	}
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

// TestParser_AnalysisChannel covers the reasoning channel produced by
// the Harmony "analysis" channel name.
func TestParser_AnalysisChannel(t *testing.T) {
	c := Parser{}.NewStateMachine()

	runSteps(t, "analysis", c, []step{
		{token: "<|start|>", channel: parsers.ChannelNone},
		{token: "assistant", channel: parsers.ChannelNone}, // unrecognized → silent
		{token: "<|channel|>", channel: parsers.ChannelNone},
		{token: "analysis", channel: parsers.ChannelNone}, // accumulated
		{token: "<|message|>", channel: parsers.ChannelNone},
		{token: "Let", channel: parsers.ChannelReasoning, content: "Let"},
		{token: " me", channel: parsers.ChannelReasoning, content: " me"},
		{token: " think", channel: parsers.ChannelReasoning, content: " think"},
		{token: "<|end|>", channel: parsers.ChannelNone},
	})
}

// TestParser_FinalChannel covers the answer channel produced by the
// Harmony "final" channel name.
func TestParser_FinalChannel(t *testing.T) {
	c := Parser{}.NewStateMachine()

	runSteps(t, "final", c, []step{
		{token: "<|start|>", channel: parsers.ChannelNone},
		{token: "assistant", channel: parsers.ChannelNone},
		{token: "<|channel|>", channel: parsers.ChannelNone},
		{token: "final", channel: parsers.ChannelNone},
		{token: "<|message|>", channel: parsers.ChannelNone},
		{token: "The", channel: parsers.ChannelAnswer, content: "The"},
		{token: " answer", channel: parsers.ChannelAnswer, content: " answer"},
		{token: "<|return|>", channel: parsers.ChannelNone, eog: true},
	})
}

func TestParser_ConstrainedFinalEverySplitPoint(t *testing.T) {
	for _, tt := range []struct {
		name, header, answer string
	}{
		{"plain", "final", "answer body"},
		{"inline code", "final code", "answer body"},
		{"inline json", "final json", "answer body"},
		{"explicit json", "final <|constrain|>json", "answer body"},
		{"explicit code", "final <|constrain|>code", "answer body"},
		{"constraint alphabet", "final X_9-test", "answer body"},
		{"long constraint", "final <|constrain|>" + strings.Repeat("x", 65), "answer body"},
		{"ASCII whitespace", "final\tcode", "answer body"},
		{"not a final prefix", "finalize", ""},
		{"invalid alphabet", "final code/type", ""},
		{"multiple words", "final code extra", ""},
		{"non-ASCII separator", "final\u00a0code", ""},
		{"empty constraint", "final <|constrain|>", ""},
		{"invalid explicit constraint", "final <|constrain|>code/type", ""},
		{"duplicate constraint", "final code<|constrain|>json", ""},
		{"constraint with recipient", "final code to=functions.echo", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stream := `<|channel|>analysis<|message|>Plan<|end|><|start|>assistant<|channel|>` + tt.header + `<|message|>answer body<|return|>`
			// Two chunks at every byte boundary, then all one-byte chunks.
			for split := range len(stream) + 2 {
				var chunks []string
				if split <= len(stream) {
					chunks = []string{stream[:split], stream[split:]}
				} else {
					for i := range len(stream) {
						chunks = append(chunks, stream[i:i+1])
					}
				}
				c := Parser{}.NewStateMachine()
				got := make(map[parsers.Channel]string)
				var ended bool
				for _, chunk := range chunks {
					result, eog := c.Classify(chunk)
					got[result.Channel] += result.Content
					ended = ended || eog
				}
				flusher := c.(parsers.StateMachineFlusher)
				for result := flusher.Flush(); result.Channel != parsers.ChannelNone; result = flusher.Flush() {
					got[result.Channel] += result.Content
				}
				if got[parsers.ChannelReasoning] != "Plan" || got[parsers.ChannelAnswer] != tt.answer || got[parsers.ChannelTool] != "" {
					t.Fatalf("split %d: got reasoning/answer/tool %q/%q/%q, want %q/%q/empty", split, got[parsers.ChannelReasoning], got[parsers.ChannelAnswer], got[parsers.ChannelTool], "Plan", tt.answer)
				}
				if tt.answer != "" && !ended {
					t.Fatalf("split %d: final answer did not end generation", split)
				}
			}
		})
	}
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
		case parsers.ChannelReasoning:
			reasoning.WriteString(got.Content)
		case parsers.ChannelAnswer:
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
	streamer := c.(parsers.ToolCallDeltaStreamer)
	flusher := c.(parsers.StateMachineFlusher)

	runSteps(t, "tool-call", c, []step{
		{token: "<|start|>", channel: parsers.ChannelNone},
		{token: "assistant", channel: parsers.ChannelNone},
		{token: "<|channel|>", channel: parsers.ChannelNone},
		{token: "commentary", channel: parsers.ChannelNone},
		{token: " to=functions.get_weather", channel: parsers.ChannelNone},
		{token: "<|constrain|>", channel: parsers.ChannelNone},
		{token: "json", channel: parsers.ChannelNone}, // swallowed
		{token: "<|message|>", channel: parsers.ChannelNone},
		{token: `{"location":"NYC"}`, channel: parsers.ChannelNone},
		{token: "<|call|>", channel: parsers.ChannelNone, eog: true},
	})

	if deltas := streamer.ToolCallDeltas(); len(deltas) != 0 {
		t.Fatalf("ToolCallDeltas: got %+v, want identities deferred until final validation", deltas)
	}
	if got := streamer.ToolCallDeltas(); got != nil {
		t.Errorf("drained ToolCallDeltas: got %+v, want nil", got)
	}
	if got := flusher.Flush(); got.Channel != parsers.ChannelTool || got.Content != `.get_weather <|message|>{"location":"NYC"}` {
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
	if result != (parsers.Result{}) {
		t.Errorf("Classify: got %+v, want empty result before flush", result)
	}

	tooling := c.(parsers.StateMachineFlusher).Flush()
	if tooling.Channel != parsers.ChannelTool {
		t.Fatalf("Flush: got channel %v, want %v", tooling.Channel, parsers.ChannelTool)
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
	tooling := c.(parsers.StateMachineFlusher).Flush()
	calls := Parser{}.ToolCall(t.Context(), nil, tooling.Content)
	if tooling.Channel != parsers.ChannelTool || len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "get_weather" {
		t.Fatalf("ToolCall: got tooling %+v, calls %+v; want one valid get_weather call", tooling, calls)
	}
}

func TestParser_VocabEOGToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	stream := `<|start|>assistant to=functions.get_weather<|channel|>commentary<|constrain|>json<|message|>{"location":"London"}`

	if _, eog := c.Classify(stream); eog {
		t.Fatal("Classify: got eog true before the call marker")
	}
	c.(parsers.VocabEOGConsumer).ConsumeVocabEOG("<|call|>")
	tooling := c.(parsers.StateMachineFlusher).Flush()
	calls := Parser{}.ToolCall(t.Context(), nil, tooling.Content)
	if tooling.Channel != parsers.ChannelTool || len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "get_weather" {
		t.Fatalf("ToolCall: got tooling %+v, calls %+v; want one valid get_weather call", tooling, calls)
	}
}

func TestParser_ToolCallFlushMultipleAndReset(t *testing.T) {
	c := Parser{}.NewStateMachine()
	streamer := c.(parsers.ToolCallDeltaStreamer)
	flusher := c.(parsers.StateMachineFlusher)

	for _, token := range []string{"<|channel|>", "commentary to=functions.", "first", "<|message|>", `{}`, "<|end|>", "<|channel|>", "commentary to=functions.second", "<|message|>", `{"x":1}`} {
		c.Classify(token)
	}
	if deltas := streamer.ToolCallDeltas(); len(deltas) != 0 {
		t.Fatalf("ToolCallDeltas: got %+v, want identities deferred until final validation", deltas)
	}
	got := flusher.Flush()
	if got.Channel != parsers.ChannelTool || got.Content != `.second <|message|>{"x":1}<|missing-end|>` {
		t.Errorf("Flush: got %+v", got)
	}
	c.Reset()
	if len(streamer.StartedToolCalls()) != 0 || flusher.Flush() != (parsers.Result{}) {
		t.Errorf("Reset did not clear tool state")
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
		case parsers.ChannelReasoning:
			reasoning.WriteString(got.Content)
		case parsers.ChannelAnswer:
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
	if got.Channel != parsers.ChannelNone {
		t.Errorf("after Reset, <|start|> channel = %v, want None", got.Channel)
	}
}

func TestParser_HarmonyEverySplitPoint(t *testing.T) {
	stream := `<|start|>assistant<|channel|>commentary to=functions.echo<|constrain|>json<|message|>{"text":"<|call|> and \\"quoted\\""}<|call|>`
	for split := 0; split <= len(stream); split++ {
		t.Run(strconv.Itoa(split), func(t *testing.T) {
			c := Parser{}.NewStateMachine()
			flusher := c.(parsers.StateMachineFlusher)
			var results []parsers.Result
			for _, chunk := range []string{stream[:split], stream[split:]} {
				if result, _ := c.Classify(chunk); result.Channel != parsers.ChannelNone {
					results = append(results, result)
				}
			}
			for result := flusher.Flush(); result.Channel != parsers.ChannelNone; result = flusher.Flush() {
				results = append(results, result)
			}
			if len(results) != 1 || results[0].Channel != parsers.ChannelTool || results[0].Content != `.echo <|message|>{"text":"<|call|> and \\"quoted\\""}` {
				t.Errorf("split %d: got %+v", split, results)
			}
		})
	}
}

func TestParser_EndDoesNotAuthorizeToolCalls(t *testing.T) {
	c := Parser{}.NewStateMachine()
	flusher := c.(parsers.StateMachineFlusher)
	stream := `<|channel|>commentary to=functions.one<|message|>{}<|end|><|channel|>commentary to=functions.two<|message|>{"x":2}<|end|>`
	var contents []string
	if result, _ := c.Classify(stream); result.Channel == parsers.ChannelTool {
		contents = append(contents, result.Content)
	}
	for result := flusher.Flush(); result.Channel != parsers.ChannelNone; result = flusher.Flush() {
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
	for result != (parsers.Result{}) {
		switch result.Channel {
		case parsers.ChannelTool:
			tooling.WriteString(result.Content)
		case parsers.ChannelAnswer:
			answer.WriteString(result.Content)
		}
		result = c.(parsers.StateMachineFlusher).Flush()
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
			if result.Channel == parsers.ChannelTool {
				tooling.WriteString(result.Content)
			}
			for result = c.(parsers.StateMachineFlusher).Flush(); result != (parsers.Result{}); result = c.(parsers.StateMachineFlusher).Flush() {
				if result.Channel == parsers.ChannelTool {
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
		if result, _ := c.Classify(stream); result.Channel == parsers.ChannelTool {
			tooling.WriteString(result.Content)
		}
		for result := c.(parsers.StateMachineFlusher).Flush(); result != (parsers.Result{}); result = c.(parsers.StateMachineFlusher).Flush() {
			if result.Channel == parsers.ChannelTool {
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
		fp       parsers.Fingerprint
		wantHave bool
	}{
		{
			name:     "harmony-template",
			fp:       parsers.Fingerprint{ChatTemplate: "...<|channel|>...<|message|>..."},
			wantHave: true,
		},
		{
			name:     "non-harmony-template",
			fp:       parsers.Fingerprint{ChatTemplate: "{% for m in messages %}<|im_start|>{{ m.role }}<|im_end|>{% endfor %}"},
			wantHave: false,
		},
		{
			name:     "empty-fingerprint",
			fp:       parsers.Fingerprint{},
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

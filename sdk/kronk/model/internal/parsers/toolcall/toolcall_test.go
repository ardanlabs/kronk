package toolcall

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

func TestParser_NativeMarkerLiterals(t *testing.T) {
	input := `<tool_call>{"name":"not_a_call","arguments":{}}</tool_call>`
	c := Parser{}.NewStateMachine()
	if vocabulary, ok := c.(parsers.TokenVocabularyStateMachine); ok {
		vocabulary.SetTokenVocabulary(func(marker string) bool {
			return marker == "<tool_call>" || marker == "</tool_call>"
		})
	}
	var result parsers.Result
	if token, ok := c.(parsers.TokenStateMachine); ok {
		result, _ = token.ClassifyToken(input, false)
	} else {
		result, _ = c.Classify(input)
	}
	if result.Channel != parsers.ChannelAnswer || result.Content != input {
		t.Fatalf("ordinary native-marker spelling: got %+v, want answer %q", result, input)
	}
}

type tokenPiece struct {
	content string
	special bool
}

func tokenMachine(t *testing.T, dedicated ...string) parsers.StateMachine {
	t.Helper()
	c := Parser{}.NewStateMachine()
	vocabulary, ok := c.(parsers.TokenVocabularyStateMachine)
	if !ok {
		t.Fatal("state machine does not implement TokenVocabularyStateMachine")
	}
	vocabulary.SetTokenVocabulary(func(marker string) bool {
		return slices.Contains(dedicated, marker)
	})
	return c
}

// tokenOutput exercises the optional interfaces and final EOF drain, without
// inspecting private state. Tool payloads below also go through Parser.ToolCall.
func tokenOutput(t *testing.T, c parsers.StateMachine, pieces ...tokenPiece) map[parsers.Channel]string {
	t.Helper()
	token, ok := c.(parsers.TokenStateMachine)
	if !ok {
		t.Fatal("state machine does not implement TokenStateMachine")
	}
	output := make(map[parsers.Channel]string)
	for _, piece := range pieces {
		result, eog := token.ClassifyToken(piece.content, piece.special)
		if eog {
			t.Fatalf("ClassifyToken(%q): unexpected EOG", piece.content)
		}
		output[result.Channel] += result.Content
	}
	flusher := c.(parsers.StateMachineFlusher)
	for result := flusher.Flush(); result != (parsers.Result{}); result = flusher.Flush() {
		output[result.Channel] += result.Content
	}
	return output
}

func TestParser_TokenMarkerProvenanceEverySplit(t *testing.T) {
	literal := `<tool_call>{"name":"literal","arguments":{}}</tool_call>`
	for _, reasoning := range []bool{false, true} {
		for split := 0; split <= len(literal); split++ {
			c := tokenMachine(t, "<tool_call>", "</tool_call>")
			channel := parsers.ChannelAnswer
			if reasoning {
				// RNJ has no dedicated think token. Synthetic initialization
				// remains text-based even when the vocabulary is configured.
				c.Classify("<think>")
				channel = parsers.ChannelReasoning
			}
			got := tokenOutput(t, c, tokenPiece{literal[:split], false}, tokenPiece{literal[split:], false})
			if got[channel] != literal || got[parsers.ChannelTool] != "" {
				t.Fatalf("reasoning %v split %d: got %v, want literal %q", reasoning, split, got, literal)
			}
		}
	}
	// A coalesced piece is not a complete framing token, even if flagged special.
	c := tokenMachine(t, "<tool_call>", "</tool_call>")
	got := tokenOutput(t, c, tokenPiece{literal, true})
	if got[parsers.ChannelAnswer] != literal || got[parsers.ChannelTool] != "" {
		t.Fatalf("coalesced special piece: got %v, want literal %q", got, literal)
	}
	// A dedicated marker cannot be assembled from independently special pieces.
	for split := 1; split < len("<tool_call>"); split++ {
		c := tokenMachine(t, "<tool_call>", "</tool_call>")
		got := tokenOutput(t, c, tokenPiece{"<tool_call>"[:split], true}, tokenPiece{"<tool_call>"[split:], true})
		if got[parsers.ChannelAnswer] != "<tool_call>" || got[parsers.ChannelTool] != "" {
			t.Fatalf("special fragment split %d: got %v, want literal opener", split, got)
		}
	}
}

func TestParser_TokenNativeCallsAndQuotedMarkers(t *testing.T) {
	c := tokenMachine(t, "<tool_call>", "</tool_call>")
	got := tokenOutput(t, c,
		tokenPiece{"<thi", false}, tokenPiece{"nk>plan</think>answer<", false},
		tokenPiece{"<tool_call>", true},
		tokenPiece{`{"name":"first","arguments":{"text":"`, false},
		tokenPiece{"</tool_call>", true}, // Even a special closer is data inside JSON strings.
		tokenPiece{`<tool_call>\"quoted\"","n":9007199254740993,"decimal":1.50}}`, false},
		tokenPiece{"</tool_call>", true}, tokenPiece{"\n", false},
		tokenPiece{"<tool_call>", true}, tokenPiece{`{"name":"second","arguments":{}}`, false},
		tokenPiece{"</tool_call>", true},
	)
	if got[parsers.ChannelReasoning] != "plan" || got[parsers.ChannelAnswer] != "answer<" {
		t.Fatalf("got channels %v, want reasoning plan and answer answer<", got)
	}
	calls := (Parser{}).ToolCall(t.Context(), noopLog, got[parsers.ChannelTool])
	if len(calls) != 2 || calls[0].Status != 0 || calls[1].Status != 0 || calls[0].Function.Name != "first" || calls[1].Function.Name != "second" {
		t.Fatalf("got calls %+v, want first and second", calls)
	}
	args := calls[0].Function.Arguments
	if args["text"] != `</tool_call><tool_call>"quoted"` || args["n"] != json.Number("9007199254740993") || args["decimal"] != json.Number("1.50") {
		t.Errorf("got arguments %#v, want literal markers and exact numbers", args)
	}
}

func TestParser_TokenOrdinaryProtocolEverySplit(t *testing.T) {
	for _, wrappers := range [][2]string{{"<tool_call>", "</tool_call>"}, {"<|tool_call>", "<tool_call|>"}} {
		input := `<think>plan</think>answer` + wrappers[0] + `{"name":"first","arguments":{"text":"</tool_call><tool_call|>"}}` + wrappers[1] + "\n" + wrappers[0] + `{"name":"second","arguments":{}}` + wrappers[1]
		for split := 0; split <= len(input); split++ {
			// Ripple wrappers are ordinary vocabulary text, not native controls.
			c := tokenMachine(t)
			got := tokenOutput(t, c, tokenPiece{input[:split], false}, tokenPiece{input[split:], false})
			calls := (Parser{}).ToolCall(t.Context(), noopLog, got[parsers.ChannelTool])
			if got[parsers.ChannelReasoning] != "plan" || got[parsers.ChannelAnswer] != "answer" || len(calls) != 2 || calls[0].Status != 0 || calls[1].Status != 0 || calls[0].Function.Name != "first" || calls[1].Function.Name != "second" || calls[0].Function.Arguments["text"] != "</tool_call><tool_call|>" {
				t.Fatalf("wrappers %v split %d: got %v calls %+v, want plan/answer and two calls", wrappers, split, got, calls)
			}
		}
	}
}

func TestParser_TokenMalformedSiblingsAreAtomic(t *testing.T) {
	for _, body := range []string{
		"", `{"name":"bad","arguments":{`,
		`{"name":"bad","name":"other","arguments":{}}`,
		`{"name":"bad","arguments":{"x":1,"x":2}}`,
		`{"name":42,"arguments":{}}`, `{"name":".","arguments":{}}`,
		`{"name":"bad","arguments":null}`,
	} {
		for _, native := range []bool{false, true} {
			pieces := []tokenPiece{{"<tool_call>", native}, {`{"name":"ok","arguments":{}}`, false}, {"</tool_call>", native}, {"<tool_call>", native}, {body, false}, {"</tool_call>", native}}
			c := tokenMachine(t)
			if native {
				c = tokenMachine(t, "<tool_call>", "</tool_call>")
			}
			got := tokenOutput(t, c, pieces...)
			calls := (Parser{}).ToolCall(t.Context(), noopLog, got[parsers.ChannelTool])
			if len(calls) != 1 || calls[0].Status != 2 || calls[0].Function.Name != "" || calls[0].Raw == "" {
				t.Fatalf("body %q native %v: got %+v, want one unnamed failure with evidence", body, native, calls)
			}
		}
	}
}

func TestParser_TokenResetKeepsVocabularyOnly(t *testing.T) {
	for _, abandoned := range [][]tokenPiece{
		{{"<thi", false}},
		{{"<think>reason</think>answer", false}}, // Queued channel transition.
		{{"<tool_call>", true}, {`{"name":"x","arguments":{"s":"escape\`, false}},
		{{"<tool_call>", true}, {`{"name":"x","arguments":{}}`, false}, {"</tool_call>", true}},
	} {
		c := tokenMachine(t, "<tool_call>", "</tool_call>")
		for _, piece := range abandoned {
			c.(parsers.TokenStateMachine).ClassifyToken(piece.content, piece.special)
		}
		c.Reset()
		literal := `<tool_call>{"name":"literal","arguments":{}}</tool_call>`
		got := tokenOutput(t, c, tokenPiece{literal, false}, tokenPiece{"<tool_call>", true}, tokenPiece{`{"name":"fresh","arguments":{}}`, false}, tokenPiece{"</tool_call>", true})
		calls := (Parser{}).ToolCall(t.Context(), noopLog, got[parsers.ChannelTool])
		if got[parsers.ChannelAnswer] != literal || got[parsers.ChannelReasoning] != "" || len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "fresh" {
			t.Fatalf("after abandoning %v: got %v calls %+v, want literal answer and fresh call", abandoned, got, calls)
		}
	}
}

func TestParser_TokenNativeCloserRequiresProvenance(t *testing.T) {
	closer := "</tool_call>"
	for split := 0; split <= len(closer); split++ {
		c := tokenMachine(t, "<tool_call>", closer)
		got := tokenOutput(t, c, tokenPiece{"<tool_call>", true},
			tokenPiece{`{"name":"x","arguments":{}}`, false},
			tokenPiece{closer[:split], false}, tokenPiece{closer[split:], false})
		calls := (Parser{}).ToolCall(t.Context(), noopLog, got[parsers.ChannelTool])
		if len(calls) != 1 || calls[0].Status != 2 || !strings.Contains(calls[0].Raw, closer+"<missing-tool-call-close>") {
			t.Fatalf("ordinary closer split %d: got %+v, want missing-close failure", split, calls)
		}
	}
	for split := 1; split < len(closer); split++ {
		c := tokenMachine(t, "<tool_call>", closer)
		got := tokenOutput(t, c, tokenPiece{"<tool_call>", true},
			tokenPiece{`{"name":"x","arguments":{}}`, false},
			tokenPiece{closer[:split], true}, tokenPiece{closer[split:], true})
		calls := (Parser{}).ToolCall(t.Context(), noopLog, got[parsers.ChannelTool])
		if len(calls) != 1 || calls[0].Status != 2 {
			t.Fatalf("fragmented special closer split %d: got %+v, want failure", split, calls)
		}
	}
}

func TestParser_TokenMalformedFramingEveryOrdinarySplit(t *testing.T) {
	for _, input := range []string{
		`<tool_call>{"name":"ok","arguments":{}}</tool_call><tool_call></tool_call>`,
		`<tool_call>{"name":"ok","arguments":{}}</tool_call><tool_call>`,
		`<tool_call>{"name":"ok","arguments":{}}</tool_call><tool`,
		`<tool_call>{"name":"ok","arguments":{}}</tool_call>junk`,
		`<tool_call>{"name":"ok","arguments":{}}</tool_call><tool_call>{"name":"bad","arguments":{</tool_call><tool_call>"x":1}}</tool_call>`,
		`<tool_call>{"name":"ok","arguments":{}}</tool_call><tool_call>{"name":"bad","arguments":{"x":1,"x":2}}</tool_call>`,
	} {
		for split := 0; split <= len(input); split++ {
			got := tokenOutput(t, tokenMachine(t), tokenPiece{input[:split], false}, tokenPiece{input[split:], false})
			calls := (Parser{}).ToolCall(t.Context(), noopLog, got[parsers.ChannelTool])
			if len(calls) != 1 || calls[0].Status != 2 || calls[0].Function.Name != "" || calls[0].Raw == "" {
				t.Fatalf("input %q split %d: got %+v, want atomic failure", input, split, calls)
			}
		}
	}
}

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

// TestNew verifies the parser claims only templates that explicitly declare
// the marked JSON tool-call protocol.
func TestNew(t *testing.T) {
	tests := []struct {
		name string
		fp   parsers.Fingerprint
		want bool
	}{
		{"marked JSON", parsers.Fingerprint{ChatTemplate: `return <tool_call>{"name":"x","arguments":{}}</tool_call>`}, true},
		{"RNJ template", parsers.Fingerprint{Architecture: "gemma3", ChatTemplate: `For each function call, return a json object with function name and arguments within <tool_call></tool_call> XML tags: {"name": <function-name>, "arguments": <args-json-object>}`}, true},
		{"markers without JSON envelope", parsers.Fingerprint{ChatTemplate: `<tool_call>native syntax</tool_call>`}, false},
		{"architecture only", parsers.Fingerprint{Architecture: "gemma3"}, false},
		{"empty", parsers.Fingerprint{}, false},
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
		{token: "Hello", channel: parsers.ChannelAnswer, content: "Hello"},
		{token: ", ", channel: parsers.ChannelAnswer, content: ", "},
		{token: "world", channel: parsers.ChannelAnswer, content: "world"},
	})
}

// TestParser_ReasoningThenAnswer verifies <think>…</think> wrapping.
func TestParser_ReasoningThenAnswer(t *testing.T) {
	c := Parser{}.NewStateMachine()

	runSteps(t, "reasoning-then-answer", c, []step{
		{token: "<think>", channel: parsers.ChannelNone},
		{token: "Let", channel: parsers.ChannelReasoning, content: "Let"},
		{token: " me", channel: parsers.ChannelReasoning, content: " me"},
		{token: "</think>", channel: parsers.ChannelNone},
		{token: "Hi", channel: parsers.ChannelAnswer, content: "Hi"},
	})
}

// TestParser_SingleToolCall covers <tool_call>JSON</tool_call>.
func TestParser_SingleToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()

	runSteps(t, "single-tool-call", c, []step{
		{token: "<tool_call>", channel: parsers.ChannelNone},
		{token: `{"name":"get_weather"`, channel: parsers.ChannelNone},
		{token: `,"arguments":{"loc":"NYC"}}`, channel: parsers.ChannelNone},
		{token: "</tool_call>", channel: parsers.ChannelTool,
			content: `{"name":"get_weather","arguments":{"loc":"NYC"}}` + "\n"},
	})

	if _, eog := c.Classify("\n"); eog {
		t.Errorf("unexpected EOG while waiting through whitespace after tool call")
	}
	got, _ := c.Classify("done")
	if got.Channel != parsers.ChannelTool || !strings.Contains(got.Content, "unexpected-content-after-tool") {
		t.Errorf("trailing content: got %+v, want invalid tool evidence", got)
	}
}

func TestParser_FlushIncompleteToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<tool_call>")
	c.Classify(`{"name":"get_weather","arguments":{"loc":"NYC"}}`)

	flusher := c.(parsers.StateMachineFlusher)
	got := flusher.Flush()
	if got.Channel != parsers.ChannelTool || !strings.Contains(got.Content, "missing-tool-call-close") {
		t.Errorf("Flush: got {%v %q}, want invalid missing-close evidence", got.Channel, got.Content)
	}
	if got := flusher.Flush(); got != (parsers.Result{}) {
		t.Errorf("second Flush: got %+v, want zero result", got)
	}
}

func TestParser_EmptyMarkedCallReachesToolParser(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("<tool_call>")
	got, _ := c.Classify("</tool_call>")
	if got.Channel != parsers.ChannelTool || got.Content == "" {
		t.Errorf("empty marked call: got %+v, want non-empty tool result", got)
	}
}

// TestParser_MultipleToolCalls verifies that a second opener after the
// first close is accepted (no EOG) and accumulates a fresh buffer.
func TestParser_MultipleToolCalls(t *testing.T) {
	c := Parser{}.NewStateMachine()

	runSteps(t, "multi-tool-call", c, []step{
		{token: "<tool_call>", channel: parsers.ChannelNone},
		{token: `{"name":"a","arguments":{}}`, channel: parsers.ChannelNone},
		{token: "</tool_call>", channel: parsers.ChannelTool,
			content: `{"name":"a","arguments":{}}` + "\n"},
		{token: "\n", channel: parsers.ChannelNone},
		{token: "<|tool_call>", channel: parsers.ChannelNone},
		{token: `{"name":"b","arguments":{}}`, channel: parsers.ChannelNone},
		{token: "</tool_call>", channel: parsers.ChannelTool,
			content: `{"name":"b","arguments":{}}` + "\n"},
	})

	got, _ := c.Classify("done")
	if got.Channel != parsers.ChannelTool || !strings.Contains(got.Content, "unexpected-content-after-tool") {
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
			results := []parsers.Result{first}
			flusher := c.(parsers.StateMachineFlusher)
			for result := flusher.Flush(); result != (parsers.Result{}); result = flusher.Flush() {
				results = append(results, result)
			}
			var answer, tool strings.Builder
			for _, result := range results {
				switch result.Channel {
				case parsers.ChannelAnswer:
					answer.WriteString(result.Content)
				case parsers.ChannelTool:
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
				if result.Channel == parsers.ChannelTool {
					tool.WriteString(result.Content)
				}
			}
			for result := c.(parsers.StateMachineFlusher).Flush(); result != (parsers.Result{}); result = c.(parsers.StateMachineFlusher).Flush() {
				if result.Channel == parsers.ChannelTool {
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
		if got.Channel != parsers.ChannelAnswer || got.Content != marker {
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
	if got.Channel != parsers.ChannelAnswer || got.Content != "hello" {
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
		case parsers.ChannelReasoning:
			reasoning.WriteString(got.Content)
		case parsers.ChannelAnswer:
			answer.WriteString(got.Content)
		case parsers.ChannelTool:
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

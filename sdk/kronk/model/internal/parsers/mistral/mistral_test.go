package mistral

import (
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

func TestNew_ClaimsMistralAndDevstral(t *testing.T) {
	tests := []struct {
		name string
		fp   parsers.Fingerprint
		want bool
	}{
		// Architecture prefix (primary signal).
		{"arch-mistral", parsers.Fingerprint{Architecture: "mistral"}, true},
		{"arch-mistral3", parsers.Fingerprint{Architecture: "mistral3"}, true},
		{"arch-mixed-case", parsers.Fingerprint{Architecture: "Mistral"}, true},

		// Chat template marker (secondary signal).
		{"template-tool-calls", parsers.Fingerprint{ChatTemplate: "before [TOOL_CALLS]name[ARGS]{}"}, true},
		{"template-args", parsers.Fingerprint{ChatTemplate: "[ARGS]{...}"}, true},

		// Model name fallback.
		{"name-Mistral", parsers.Fingerprint{ModelName: "Mistral-7B-Instruct"}, true},
		{"name-Devstral", parsers.Fingerprint{ModelName: "Devstral-Small"}, true},

		// Negatives.
		{"name-llama", parsers.Fingerprint{ModelName: "Llama-3-8B"}, false},
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

func TestParser_AdjustParams(t *testing.T) {
	tests := []struct {
		name   string
		parser Parser
		input  string
		want   string
	}{
		{name: "template default remains unset", parser: Parser{strictReasoningEffort: true}},
		{name: "supported none", parser: Parser{strictReasoningEffort: true}, input: parsers.ReasoningEffortNone, want: parsers.ReasoningEffortNone},
		{name: "supported high", parser: Parser{strictReasoningEffort: true}, input: parsers.ReasoningEffortHigh, want: parsers.ReasoningEffortHigh},
		{name: "unsupported medium becomes high", parser: Parser{strictReasoningEffort: true}, input: parsers.ReasoningEffortMedium, want: parsers.ReasoningEffortHigh},
		{name: "unrestricted template", input: parsers.ReasoningEffortMedium, want: parsers.ReasoningEffortMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := tt.parser.AdjustParams(parsers.Params{ReasoningEffort: tt.input})
			if params.ReasoningEffort != tt.want {
				t.Errorf("ReasoningEffort: got %q, want %q", params.ReasoningEffort, tt.want)
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
		{token: ", ", channel: parsers.ChannelAnswer, content: ", "},
		{token: "world", channel: parsers.ChannelAnswer, content: "world"},
	})
}

func TestParser_Reasoning(t *testing.T) {
	c := Parser{}.NewStateMachine()
	runSteps(t, "reasoning", c, []step{
		{token: "<think>", channel: parsers.ChannelNone},
		{token: "Plan", channel: parsers.ChannelReasoning, content: "Plan"},
		{token: "</think>", channel: parsers.ChannelNone},
		{token: "Answer", channel: parsers.ChannelAnswer, content: "Answer"},
	})
}

// TestParser_StreamingToolCall verifies native buffering and split delimiter
// detection.
func TestParser_StreamingToolCall(t *testing.T) {
	c := Parser{}.NewStateMachine()
	flusher := c.(parsers.StateMachineFlusher)
	runSteps(t, "streaming-tool-call", c, []step{
		{token: "[TOOL_CALLS]", channel: parsers.ChannelNone},
		{token: "get_", channel: parsers.ChannelNone},
		{token: "weather", channel: parsers.ChannelNone},
		{token: "[AR", channel: parsers.ChannelNone},
		{token: "GS]", channel: parsers.ChannelNone},
		{token: `{"loc":"NYC"}`, channel: parsers.ChannelNone},
	})
	got := flusher.Flush()
	want := `[TOOL_CALLS]get_weather[ARGS]{"loc":"NYC"}`
	if got.Channel != parsers.ChannelTool || got.Content != want {
		t.Errorf("Flush: got %+v, want %q", got, want)
	}
	if calls := parseMistral(t.Context(), noopLog, got.Content); len(calls) != 1 || calls[0].Function.Name != "get_weather" {
		t.Fatalf("parseMistral: got %+v", calls)
	}
}

// TestParser_RepeatedMarkerInsideToolMode verifies that a second
// [TOOL_CALLS] inside tool mode is silent (state already correct).
func TestParser_RepeatedMarkerInsideToolMode(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("[TOOL_CALLS]")
	c.Classify("a")
	got, _ := c.Classify("[TOOL_CALLS]")
	if got.Channel != parsers.ChannelNone || got.Content != "" {
		t.Errorf("repeated [TOOL_CALLS] should be silent, got %+v", got)
	}
}

func TestParser_MultipleToolCallsAndReset(t *testing.T) {
	c := Parser{}.NewStateMachine()
	flusher := c.(parsers.StateMachineFlusher)
	input := []string{"[TOOL_CALLS]", "first[ARGS]{}", "[TOOL_", "CALLS]second[AR", "GS]", `{"x":1}`}
	for _, token := range input {
		got, _ := c.Classify(token)
		if got != (parsers.Result{}) {
			t.Errorf("Classify(%q): got incremental payload %+v", token, got)
		}
	}
	want := strings.Join(input, "")
	if got := flusher.Flush(); got.Content != want || got.Channel != parsers.ChannelTool {
		t.Errorf("Flush: got %+v, want %q", got, want)
	}
	calls := parseMistral(t.Context(), noopLog, want)
	if len(calls) != 2 || calls[0].Function.Name != "first" || calls[1].Function.Name != "second" {
		t.Fatalf("parseMistral: got %+v", calls)
	}
	c.Reset()
	if flusher.Flush() != (parsers.Result{}) {
		t.Errorf("Reset did not clear tool state")
	}
}

func TestParser_ToolMarkerInsideArgumentsIsNotActivity(t *testing.T) {
	c := Parser{}.NewStateMachine()

	for _, token := range []string{
		"[TOOL_CALLS]",
		`first[ARGS]{"text":"[TOOL_CALLS]fake[ARGS]{}"}`,
		`[TOOL_CALLS]second[ARGS]{}`,
	} {
		c.Classify(token)
	}
	got := c.(parsers.StateMachineFlusher).Flush()
	want := `[TOOL_CALLS]first[ARGS]{"text":"[TOOL_CALLS]fake[ARGS]{}"}[TOOL_CALLS]second[ARGS]{}`
	if got.Channel != parsers.ChannelTool || got.Content != want {
		t.Fatalf("Flush: got %+v, want verbatim tool buffer", got)
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
		if got.Channel != parsers.ChannelAnswer || got.Content != m {
			t.Errorf("mistral should pass-through %q, got %+v", m, got)
		}
	}
}

func TestParser_Reset(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.Classify("[TOOL_CALLS]")
	c.Reset()
	got, _ := c.Classify("hello")
	if got.Channel != parsers.ChannelAnswer || got.Content != "hello" {
		t.Errorf("after Reset got %+v", got)
	}
}

func TestParser_EveryToolMarkerSplit(t *testing.T) {
	input := `[TOOL_CALLS]echo[ARGS]{"text":"[TOOL_CALLS] is data"}`
	for split := range len(toolCallsMarker) + 1 {
		c := Parser{}.NewStateMachine()
		first, _ := c.Classify(input[:split])
		second, _ := c.Classify(input[split:])
		if first != (parsers.Result{}) || second != (parsers.Result{}) {
			t.Errorf("split %d: got incremental output %+v, %+v", split, first, second)
		}
		got := c.(parsers.StateMachineFlusher).Flush()
		if got.Channel != parsers.ChannelTool || got.Content != input {
			t.Errorf("split %d: Flush got %+v", split, got)
		}
		if again := c.(parsers.StateMachineFlusher).Flush(); again != (parsers.Result{}) {
			t.Errorf("split %d: repeated Flush got %+v", split, again)
		}
	}
}

func TestParser_FalsePrefixIsLossless(t *testing.T) {
	for _, chunks := range [][]string{{"[TOOL_", "BOX]hello"}, {"[", "x"}} {
		c := Parser{}.NewStateMachine()
		var output strings.Builder
		for _, chunk := range chunks {
			got, _ := c.Classify(chunk)
			output.WriteString(got.Content)
		}
		if want := strings.Join(chunks, ""); output.String() != want {
			t.Errorf("chunks %q: got %q, want %q", chunks, output.String(), want)
		}
	}
}

func TestParser_IncompleteStagesFlushAsTool(t *testing.T) {
	for _, input := range []string{"[", "[TOOL_CALLS]", "[TOOL_CALLS]name", "[TOOL_CALLS]name[ARGS]", "[TOOL_CALLS]name[ARGS]{"} {
		c := Parser{}.NewStateMachine()
		c.Classify(input)
		got := c.(parsers.StateMachineFlusher).Flush()
		if got.Channel != parsers.ChannelTool || got.Content != input {
			t.Errorf("input %q: Flush got %+v", input, got)
		}
	}
}

func TestParser_ContentBeforeTool(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantPrefix parsers.Result
	}{
		{
			name:       "answer",
			input:      "Here is the result[TOOL_CALLS]write[ARGS]{}",
			wantPrefix: parsers.Result{Channel: parsers.ChannelAnswer, Content: "Here is the result"},
		},
		{
			name:       "reasoning",
			input:      "<think>Need a file</think>[TOOL_CALLS]write[ARGS]{}",
			wantPrefix: parsers.Result{Channel: parsers.ChannelReasoning, Content: "Need a file"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for split := range len(tc.input) + 1 {
				c := Parser{}.NewStateMachine()
				var results []parsers.Result
				for _, chunk := range []string{tc.input[:split], tc.input[split:]} {
					if got, _ := c.Classify(chunk); got != (parsers.Result{}) {
						results = appendResult(results, got)
					}
				}
				flusher := c.(parsers.StateMachineFlusher)
				for got := flusher.Flush(); got != (parsers.Result{}); got = flusher.Flush() {
					results = appendResult(results, got)
				}

				wantTool := parsers.Result{Channel: parsers.ChannelTool, Content: "[TOOL_CALLS]write[ARGS]{}"}
				if len(results) != 2 || results[0] != tc.wantPrefix || results[1] != wantTool {
					t.Errorf("split %d: got %+v, want [%+v %+v]", split, results, tc.wantPrefix, wantTool)
				}
			}
		})
	}
}

func appendResult(results []parsers.Result, result parsers.Result) []parsers.Result {
	if len(results) > 0 && results[len(results)-1].Channel == result.Channel {
		results[len(results)-1].Content += result.Content
		return results
	}
	return append(results, result)
}

func TestParser_PartialToolMarkerAfterContentAtEOS(t *testing.T) {
	c := Parser{}.NewStateMachine()
	got, _ := c.Classify("answer[TOOL_")
	if want := (parsers.Result{Channel: parsers.ChannelAnswer, Content: "answer"}); got != want {
		t.Fatalf("Classify: got %+v, want %+v", got, want)
	}
	if got := c.(parsers.StateMachineFlusher).Flush(); got.Channel != parsers.ChannelTool || got.Content != "[TOOL_" {
		t.Fatalf("Flush: got %+v, want partial marker as tool evidence", got)
	}
}

func TestParser_WhitespaceOnlyEOSIsAnswer(t *testing.T) {
	c := Parser{}.NewStateMachine()
	got, _ := c.Classify(" \t\r\n")
	if want := (parsers.Result{Channel: parsers.ChannelAnswer, Content: " \t\r\n"}); got != want {
		t.Fatalf("Classify: got %+v, want %+v", got, want)
	}
	if got := c.(parsers.StateMachineFlusher).Flush(); got != (parsers.Result{}) {
		t.Fatalf("Flush: got %+v, want empty repeated flush", got)
	}
}

func TestParser_TokenProvenance(t *testing.T) {
	c := Parser{}.NewStateMachine()
	tokens, ok := c.(parsers.TokenStateMachine)
	if !ok {
		t.Fatal("missing TokenStateMachine")
	}
	vocabulary, ok := c.(parsers.TokenVocabularyStateMachine)
	if !ok {
		t.Fatal("missing TokenVocabularyStateMachine")
	}
	vocabulary.SetTokenVocabulary(func(string) bool { return true })
	input := `[TOOL_CALLS]echo[ARGS]{}`
	for split := range len(input) + 1 {
		c.Reset()
		var results []parsers.Result
		for _, chunk := range []string{input[:split], input[split:]} {
			got, eog := tokens.ClassifyToken(chunk, false)
			if eog {
				t.Fatal("unexpected EOG")
			}
			results = appendResult(results, got)
		}
		for got := c.(parsers.StateMachineFlusher).Flush(); got != (parsers.Result{}); got = c.(parsers.StateMachineFlusher).Flush() {
			results = appendResult(results, got)
		}
		var answer string
		for _, got := range results {
			if got.Content != "" && got.Channel != parsers.ChannelAnswer {
				t.Errorf("split %d: got %+v, want answer", split, got)
			}
			answer += got.Content
		}
		if answer != input {
			t.Errorf("split %d: got %q, want %q", split, answer, input)
		}
	}
}

func TestParser_TokenVocabularyPolicies(t *testing.T) {
	for _, dedicated := range []bool{false, true} {
		for _, marker := range streamMarkers {
			for split := range len(marker) + 1 {
				c := Parser{}.NewStateMachine()
				c.(parsers.TokenVocabularyStateMachine).SetTokenVocabulary(func(string) bool { return dedicated })
				tokens := c.(parsers.TokenStateMachine)
				var results []parsers.Result
				for _, chunk := range []string{marker[:split], marker[split:]} {
					got, _ := tokens.ClassifyToken(chunk, false)
					if got != (parsers.Result{}) {
						results = appendResult(results, got)
					}
				}
				got, _ := tokens.ClassifyToken("body", false)
				if got != (parsers.Result{}) {
					results = appendResult(results, got)
				}
				for got := c.(parsers.StateMachineFlusher).Flush(); got != (parsers.Result{}); got = c.(parsers.StateMachineFlusher).Flush() {
					results = appendResult(results, got)
				}
				want := parsers.Result{Channel: parsers.ChannelAnswer, Content: marker + "body"}
				if !dedicated {
					want.Content = "body"
					switch marker {
					case toolCallsMarker:
						want = parsers.Result{Channel: parsers.ChannelTool, Content: marker + "body"}
					case "<think>", "[THINK]":
						want.Channel = parsers.ChannelReasoning
					}
				}
				if len(results) != 1 || results[0] != want {
					t.Errorf("dedicated %v marker %q split %d: got %+v, want %+v", dedicated, marker, split, results, want)
				}
			}
		}
	}
}

func TestParser_NativeReasoningCompatibilityAndReuse(t *testing.T) {
	c := Parser{}.NewStateMachine()
	c.(parsers.TokenVocabularyStateMachine).SetTokenVocabulary(func(marker string) bool {
		return marker == "[THINK]" || marker == "[/THINK]" || marker == toolCallsMarker || marker == argsMarker
	})
	tokens := c.(parsers.TokenStateMachine)
	for _, pair := range [][2]string{{"[THINK]", "[/THINK]"}, {"<think>", "</think>"}} {
		c.Reset()
		tokens.ClassifyToken(pair[0], true)
		if got, _ := tokens.ClassifyToken("plan", false); got != (parsers.Result{Channel: parsers.ChannelReasoning, Content: "plan"}) {
			t.Fatalf("got %+v, want reasoning", got)
		}
		tokens.ClassifyToken(pair[1], true)
		if got, _ := tokens.ClassifyToken("answer", false); got != (parsers.Result{Channel: parsers.ChannelAnswer, Content: "answer"}) {
			t.Fatalf("got %+v, want answer", got)
		}
	}
	// Discard an unfinished native call and queued/pending text on slot reuse.
	tokens.ClassifyToken(toolCallsMarker, true)
	tokens.ClassifyToken("echo", false)
	tokens.ClassifyToken(argsMarker, true)
	c.Reset()
	if got := c.(parsers.StateMachineFlusher).Flush(); got != (parsers.Result{}) {
		t.Fatalf("after Reset: got %+v, want empty", got)
	}
	if got, _ := tokens.ClassifyToken(toolCallsMarker, false); got != (parsers.Result{Channel: parsers.ChannelAnswer, Content: toolCallsMarker}) {
		t.Errorf("after Reset: got %+v, want literal marker", got)
	}
	// Text-only callers and synthetic initialization remain compatible.
	c.Reset()
	c.Classify("[THINK]")
	if got, _ := tokens.ClassifyToken("plan", false); got.Channel != parsers.ChannelReasoning || got.Content != "plan" {
		t.Errorf("synthetic initialization: got %+v, want reasoning", got)
	}
}

func TestParser_OrdinaryVocabularyCallsEverySplit(t *testing.T) {
	input := `[TOOL_CALLS]echo[ARGS]{"text":"[ARGS] [TOOL_CALLS]"}[TOOL_CALLS]second[ARGS]{}`
	for split := range len(input) + 1 {
		c := Parser{}.NewStateMachine()
		c.(parsers.TokenVocabularyStateMachine).SetTokenVocabulary(func(string) bool { return false })
		tokens := c.(parsers.TokenStateMachine)
		tokens.ClassifyToken(input[:split], false)
		tokens.ClassifyToken(input[split:], false)
		got := c.(parsers.StateMachineFlusher).Flush()
		calls := (Parser{}).ToolCall(t.Context(), nil, got.Content)
		if got.Content != input || len(calls) != 2 || calls[0].Status != 0 || calls[0].Function.Arguments["text"] != "[ARGS] [TOOL_CALLS]" {
			t.Errorf("split %d: got buffer %+v calls %+v, want two calls with literal marker data", split, got, calls)
		}
	}
}

func TestParser_SpecialPiecesMustBeCompleteMarkers(t *testing.T) {
	for _, marker := range streamMarkers {
		for split := 1; split < len(marker); split++ {
			c := Parser{}.NewStateMachine()
			c.(parsers.TokenVocabularyStateMachine).SetTokenVocabulary(func(string) bool { return true })
			tokens := c.(parsers.TokenStateMachine)
			var answer string
			for _, piece := range []string{marker[:split], marker[split:]} {
				got, _ := tokens.ClassifyToken(piece, true)
				if got.Channel != parsers.ChannelAnswer {
					t.Errorf("marker %q split %d: got %+v, want answer", marker, split, got)
				}
				answer += got.Content
			}
			if answer != marker {
				t.Errorf("got %q, want %q", answer, marker)
			}
		}
	}
}

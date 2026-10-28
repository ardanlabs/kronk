package qwen

import (
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestParser_SeparatedMarkerTokens(t *testing.T) {
	c := Parser{}.NewStateMachine()
	var reasoning strings.Builder
	for _, token := range []string{"<think>", "Plan", "\n", "</think>"} {
		result, _ := c.Classify(token)
		if result.Channel == model.ChannelReasoning {
			reasoning.WriteString(result.Content)
		}
	}
	if got := reasoning.String(); got != "Plan" {
		t.Errorf("reasoning: got %q, want %q", got, "Plan")
	}

	c.Reset()
	var answer strings.Builder
	for _, token := range []string{"Inspect.", "\n", "\n", "<tool_call>"} {
		result, _ := c.Classify(token)
		if result.Channel == model.ChannelAnswer {
			answer.WriteString(result.Content)
		}
	}
	if got := answer.String(); got != "Inspect." {
		t.Errorf("answer: got %q, want %q", got, "Inspect.")
	}
}

// TestParser_TemplateSeparators exercises the public streaming contract, including
// Flush: only complete template separators may consume whitespace or markers.
func TestParser_TemplateSeparators(t *testing.T) {
	const call = `<function=run><parameter=command>pwd</parameter></function>`
	tests := []struct {
		name      string
		input     string
		answer    string
		reasoning string
		tool      bool
	}{
		{name: "reasoning no separator", input: "<think>Plan</think>Hi", reasoning: "Plan", answer: "Hi"},
		{name: "reasoning separator", input: "<think>Plan\n</think>Hi", reasoning: "Plan", answer: "Hi"},
		{name: "reasoning extra newline", input: "<think>Plan\n\n</think>Hi", reasoning: "Plan\n", answer: "Hi"},
		{name: "reasoning meaningful spaces", input: "<think>  Plan \t\n</think>Hi", reasoning: "  Plan \t", answer: "Hi"},
		{name: "tool no separator", input: "Inspect.<tool_call>" + call + "</tool_call>", answer: "Inspect.", tool: true},
		{name: "tool one newline", input: "Inspect.\n<tool_call>" + call + "</tool_call>", answer: "Inspect.", tool: true},
		{name: "tool two newlines", input: "Inspect.\n\n<tool_call>" + call + "</tool_call>", answer: "Inspect.", tool: true},
		{name: "tool extra newline", input: "Inspect.\n\n\n<tool_call>" + call + "</tool_call>", answer: "Inspect.\n", tool: true},
		{name: "tool meaningful spaces", input: "  Inspect. \t\n\n<tool_call>" + call + "</tool_call>", answer: "  Inspect. \t", tool: true},
		{name: "spaces are not a separator", input: "Inspect.\n \t<tool_call>" + call + "</tool_call>", answer: "Inspect.\n \t", tool: true},
		{name: "only LF is a separator", input: "Inspect.\r\n<tool_call>" + call + "</tool_call>", answer: "Inspect.\r", tool: true},
		{name: "direct tool separator", input: "Inspect.\n\n" + call, answer: "Inspect.", tool: true},
		{name: "alternate wrapper", input: "Inspect.\n<|tool_call>" + call + "<tool_call|>", answer: "Inspect.", tool: true},
		{name: "JSON wrapper", input: "Inspect.\n\n<tool_call>" + `{"name":"run","arguments":{"command":"pwd"}}` + "</tool_call>", answer: "Inspect.", tool: true},
		{name: "ordinary whitespace", input: "  line\n\nnext \t\n\n", answer: "  line\n\nnext \t\n\n"},
		{name: "unicode whitespace and text", input: "　計画\n\n続く　\n", answer: "　計画\n\n続く　\n"},
		{name: "partial think closer", input: "<think>Plan\n</thi", reasoning: "Plan\n</thi"},
		{name: "partial tool opener", input: "Inspect.\n\n<tool_ca", answer: "Inspect.\n\n<tool_ca"},
		{name: "partial direct opener", input: "Inspect.\n<function", answer: "Inspect.\n<function"},
		{name: "false marker", input: "#include <functional>\n", answer: "#include <functional>\n"},
		{name: "reasoning before tool is not content separator", input: "<think>Plan\n<tool_call>" + call + "</tool_call>", reasoning: "Plan\n", tool: true},
	}
	for _, marker := range []string{"<think>", "</think>", "<tool_call>", "<|tool_call>", "<function="} {
		for end := 1; end < len(marker); end++ {
			prefix := "Text\n\n" + marker[:end]
			tests = append(tests, struct {
				name      string
				input     string
				answer    string
				reasoning string
				tool      bool
			}{name: "final prefix " + marker[:end], input: prefix, answer: prefix})
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Every two-piece split and byte-at-a-time delivery must agree with
			// a single decoded piece, including splits inside delimiters.
			chunks := make([]string, len(tt.input))
			for i := range len(tt.input) {
				chunks[i] = tt.input[i : i+1]
			}
			variants := [][]string{{tt.input}, chunks}
			for i := 1; i < len(tt.input); i++ {
				variants = append(variants, []string{tt.input[:i], tt.input[i:]})
			}
			for i, tokens := range variants {
				c := Parser{}.NewStateMachine()
				var answer, reasoning, tooling strings.Builder
				collect := func(result model.Result) {
					switch result.Channel {
					case model.ChannelAnswer:
						answer.WriteString(result.Content)
					case model.ChannelReasoning:
						reasoning.WriteString(result.Content)
					case model.ChannelTool:
						tooling.WriteString(result.Content)
					}
				}
				for _, token := range tokens {
					result, eog := c.Classify(token)
					if eog {
						t.Fatalf("variant %d: got premature EOG", i)
					}
					collect(result)
				}
				flusher := c.(model.StateMachineFlusher)
				for result := flusher.Flush(); result != (model.Result{}); result = flusher.Flush() {
					collect(result)
				}
				if answer.String() != tt.answer || reasoning.String() != tt.reasoning {
					t.Fatalf("variant %d: got answer %q, reasoning %q; want answer %q, reasoning %q", i, answer.String(), reasoning.String(), tt.answer, tt.reasoning)
				}
				if tt.tool {
					calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
					if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "run" || calls[0].Function.Arguments["command"] != "pwd" {
						t.Fatalf("variant %d: got calls %+v, want run(command=pwd)", i, calls)
					}
				} else if tooling.Len() != 0 {
					t.Fatalf("variant %d: got tool payload %q, want none", i, tooling.String())
				}
			}
		})
	}
}

func TestParser_SeparatorReset(t *testing.T) {
	for _, input := range []string{"Text\n\n<tool_ca", "<think>Plan</think>Answer\n"} {
		c := Parser{}.NewStateMachine()
		c.Classify(input)
		c.Reset()
		result, eog := c.Classify("fresh")
		if eog || result.Channel != model.ChannelAnswer || result.Content != "fresh" {
			t.Fatalf("after Reset: got (%+v, %v), want fresh answer", result, eog)
		}
		if result := c.(model.StateMachineFlusher).Flush(); result != (model.Result{}) {
			t.Errorf("Flush after Reset: got %+v, want zero result", result)
		}
	}
}

func TestParser_FailedSplitPrefixAfterTool(t *testing.T) {
	c := Parser{}.NewStateMachine()
	var tooling strings.Builder
	for _, token := range []string{"<tool_call>", `{"name":"run","arguments":{"command":"pwd"}}`, "</tool_call>", "<", "foo>"} {
		result, eog := c.Classify(token)
		if eog {
			t.Fatal("got premature EOG, want malformed continuation preserved")
		}
		if result.Channel == model.ChannelTool {
			tooling.WriteString(result.Content)
		}
	}
	flusher := c.(model.StateMachineFlusher)
	for result := flusher.Flush(); result != (model.Result{}); result = flusher.Flush() {
		tooling.WriteString(result.Content)
	}
	calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
	if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" || !strings.HasSuffix(calls[0].Raw, "<foo>") {
		t.Fatalf("ToolCall: got %+v, want one non-executable failed call preserving <foo>", calls)
	}
}

// Kronk recognizes the direct-XML discriminator before a name or closing >,
// unlike upstream's declared-name PEG alternatives. Preserve that contract:
// an incomplete call is a tool parse failure, not silently executable or text.
func TestParser_TruncatedDirectOpener(t *testing.T) {
	for _, input := range []string{"<function=", "<function=run", "<function=run>"} {
		c := Parser{}.NewStateMachine()
		c.Classify(input)
		result := c.(model.StateMachineFlusher).Flush()
		if result.Channel != model.ChannelTool {
			t.Fatalf("Flush(%q): got channel %v, want tool", input, result.Channel)
		}
		calls := Parser{}.ToolCall(t.Context(), nil, result.Content)
		if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
			t.Errorf("ToolCall(%q): got %+v, want one non-executable failed call", input, calls)
		}
	}
}

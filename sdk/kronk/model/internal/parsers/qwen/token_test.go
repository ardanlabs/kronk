package qwen

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

func TestParser_TokenFraming(t *testing.T) {
	type piece struct {
		text    string
		special bool
	}
	for _, xmlSpecial := range []bool{false, true} {
		t.Run(fmt.Sprintf("xml-special=%t", xmlSpecial), func(t *testing.T) {
			literal := "Show <think>literal</think> and <tool_call>not a call</tool_call>."
			if xmlSpecial {
				literal += " <function=echo>not XML tooling</function>"
			}
			for _, tt := range []struct {
				name      string
				prime     bool
				pieces    []piece
				answer    string
				reasoning string
				arguments []string
			}{
				{
					name:   "literal delimiters",
					pieces: []piece{{literal, false}},
					answer: literal,
				},
				{
					name:      "real reasoning with literal closer",
					pieces:    []piece{{"<think>", true}, {"Explain </think> literally\n", false}, {"</think>", true}, {"Answer.", false}},
					answer:    "Answer.",
					reasoning: "Explain </think> literally",
				},
				{
					name:      "template primed reasoning",
					prime:     true,
					pieces:    []piece{{"Keep </think> as text.", false}, {"</think>", true}, {"Done.", false}},
					answer:    "Done.",
					reasoning: "Keep </think> as text.",
				},
				{
					name: "JSON arguments with literal framing and parallel calls",
					pieces: []piece{
						{"<tool_call>", true}, {`{"name":"echo","arguments":{"text":"`, false},
						{"<tool_call>", false}, {" </tool_call> </think> café", false}, {`"}}`, false}, {"</tool_call>", true},
						{"\n\n", false}, {"<tool_call>", true}, {`{"name":"echo","arguments":{"text":"second"}}`, false}, {"</tool_call>", true},
					},
					arguments: []string{"<tool_call> </tool_call> </think> café", "second"},
				},
				{
					name: "direct XML",
					pieces: []piece{
						{"<function=", xmlSpecial}, {"echo><parameter=text>hello</parameter>", false}, {"</function>", xmlSpecial},
					},
					arguments: []string{"hello"},
				},
				{
					name: "wrapped XML",
					pieces: []piece{
						{"<tool_call>", true}, {"\n", false}, {"<function=", xmlSpecial},
						{"echo><parameter=text>wrapped</parameter>", false}, {"</function>", xmlSpecial},
						{"\n", false}, {"</tool_call>", true},
					},
					arguments: []string{"wrapped"},
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					state := Parser{}.NewStateMachine()
					state.(parsers.TokenVocabularyStateMachine).SetTokenVocabulary(func(marker string) bool {
						return xmlSpecial || (marker != "<function=" && marker != "</function>")
					})
					maxLen := 0
					for _, p := range tt.pieces {
						maxLen = max(maxLen, len(p.text))
					}
					// Split ordinary text at every byte boundary, but never split
					// a native special token. Reuse the slot with its vocabulary.
					for split := range maxLen + 1 {
						state.Reset()
						if tt.prime {
							state.Classify("<think>")
						}
						var answer, reasoning, tooling strings.Builder
						collect := func(result parsers.Result) {
							switch result.Channel {
							case parsers.ChannelAnswer:
								answer.WriteString(result.Content)
							case parsers.ChannelReasoning:
								reasoning.WriteString(result.Content)
							case parsers.ChannelTool:
								tooling.WriteString(result.Content)
							}
						}
						for _, p := range tt.pieces {
							parts := []string{p.text}
							if !p.special {
								at := min(split, len(p.text))
								parts = []string{p.text[:at], p.text[at:]}
							}
							for _, part := range parts {
								result, eog := state.(parsers.TokenStateMachine).ClassifyToken(part, p.special)
								if eog {
									t.Fatalf("split %d: got premature EOG", split)
								}
								collect(result)
							}
						}
						flusher := state.(parsers.StateMachineFlusher)
						for result := flusher.Flush(); result != (parsers.Result{}); result = flusher.Flush() {
							collect(result)
						}
						if answer.String() != tt.answer || reasoning.String() != tt.reasoning {
							t.Fatalf("split %d: got answer %q, reasoning %q; want %q, %q", split, answer.String(), reasoning.String(), tt.answer, tt.reasoning)
						}
						if len(tt.arguments) == 0 {
							if tooling.Len() != 0 {
								t.Fatalf("split %d: got unexpected tooling %q", split, tooling.String())
							}
							continue
						}
						calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
						if len(calls) != len(tt.arguments) {
							t.Fatalf("split %d: got calls %+v, want %d", split, calls, len(tt.arguments))
						}
						for i, call := range calls {
							if call.Status != 0 || call.Function.Name != "echo" || call.Function.Arguments["text"] != tt.arguments[i] {
								t.Fatalf("split %d: got call %+v, want echo(%q)", split, call, tt.arguments[i])
							}
						}
					}
				})
			}
		})
	}
}

func TestParser_TextOnlyVocabulary(t *testing.T) {
	state := Parser{}.NewStateMachine()
	state.(parsers.TokenVocabularyStateMachine).SetTokenVocabulary(func(string) bool { return false })
	const input = "<think>Plan</think>Answer<function=echo><parameter=text>hello</parameter></function>"
	var answer, reasoning, tooling strings.Builder
	collect := func(result parsers.Result) {
		switch result.Channel {
		case parsers.ChannelAnswer:
			answer.WriteString(result.Content)
		case parsers.ChannelReasoning:
			reasoning.WriteString(result.Content)
		case parsers.ChannelTool:
			tooling.WriteString(result.Content)
		}
	}
	for i := range len(input) {
		result, _ := state.(parsers.TokenStateMachine).ClassifyToken(input[i:i+1], false)
		collect(result)
	}
	flusher := state.(parsers.StateMachineFlusher)
	for result := flusher.Flush(); result != (parsers.Result{}); result = flusher.Flush() {
		collect(result)
	}
	if answer.String() != "Answer" || reasoning.String() != "Plan" {
		t.Fatalf("got answer %q, reasoning %q; want Answer, Plan", answer.String(), reasoning.String())
	}
	calls := Parser{}.ToolCall(t.Context(), nil, tooling.String())
	if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "echo" || calls[0].Function.Arguments["text"] != "hello" {
		t.Fatalf("got calls %+v, want echo(hello)", calls)
	}
}

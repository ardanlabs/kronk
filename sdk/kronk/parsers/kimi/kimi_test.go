package kimi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestNewClaimsKimiK3(t *testing.T) {
	constructedTemplate := `{{- '<|open|>' + tag -}}{{- '<|sep|>' -}}
{{- '<|close|>' + tag + '<|sep|>' -}}
{{- otag('response') -}}{{- otag('tools') -}}{{- otag('call', attrs) -}}`

	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		{"constructed-template", model.Fingerprint{ChatTemplate: constructedTemplate}, true},
		{"upstream-macro-template", model.Fingerprint{ChatTemplate: strings.ReplaceAll(constructedTemplate, "otag(", "open_tag(")}, true},
		{"literal-template", model.Fingerprint{ChatTemplate: thinkOpen + responseOpen + toolsOpen}, true},
		{"architecture", model.Fingerprint{Architecture: "kimi-k3"}, true},
		{"architecture-case", model.Fingerprint{Architecture: "KimiK3"}, true},
		{"k31-architecture", model.Fingerprint{Architecture: "kimi-k31"}, true},
		{"k31-model-name", model.Fingerprint{ModelName: "Moonshot-Kimi-K3.1"}, true},
		{"model-name", model.Fingerprint{ModelName: "Moonshot-Kimi-K3-Instruct"}, true},
		{"k2-architecture", model.Fingerprint{Architecture: "kimi-k2"}, false},
		{"k2-model-name", model.Fingerprint{ModelName: "Kimi-K2-Instruct"}, false},
		{"k25-model-name", model.Fingerprint{ModelName: "Kimi-K2.5"}, false},
		{"incomplete-template", model.Fingerprint{ChatTemplate: thinkOpen + responseOpen}, false},
		{"unrelated", model.Fingerprint{Architecture: "llama", ModelName: "Llama-3"}, false},
		{"empty", model.Fingerprint{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := New(tt.fp)
			if ok != tt.want {
				t.Errorf("New(%+v) ok = %v, want %v", tt.fp, ok, tt.want)
			}
		})
	}
}

func TestStateMachineReasoningResponseAndTools(t *testing.T) {
	sm := Parser{}.NewStateMachine()

	assertResult(t, sm, thinkOpen, model.ChannelNone, "", false)
	assertResult(t, sm, "plan", model.ChannelReasoning, "plan", false)
	assertResult(t, sm, thinkClose, model.ChannelNone, "", false)
	assertResult(t, sm, responseOpen, model.ChannelNone, "", false)
	assertResult(t, sm, "answer", model.ChannelAnswer, "answer", false)
	assertResult(t, sm, responseClose, model.ChannelNone, "", false)
	assertResult(t, sm, toolsOpen, model.ChannelNone, "", false)
	assertResult(t, sm, callOpen, model.ChannelNone, "", false)
}

func TestStateMachineEveryThinkOpenerSplit(t *testing.T) {
	for splitAt := 1; splitAt < len(thinkOpen); splitAt++ {
		t.Run(fmt.Sprintf("split-%d", splitAt), func(t *testing.T) {
			sm := Parser{}.NewStateMachine()
			assertResult(t, sm, thinkOpen[:splitAt], model.ChannelNone, "", false)
			assertResult(t, sm, thinkOpen[splitAt:], model.ChannelNone, "", false)
			assertResult(t, sm, "plan", model.ChannelReasoning, "plan", false)
		})
	}
}

func TestStateMachineSplitToolsBlock(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	fragments := []string{"\n<|op", "en|>tools", sepToken, callOpen, ` tool="ping" index="1"`, sepToken, callClose, toolsClose}

	var tool strings.Builder
	for _, fragment := range fragments {
		result, eog := sm.Classify(fragment)
		if eog {
			t.Fatalf("Classify(%q) returned unexpected EOG", fragment)
		}
		if result.Channel == model.ChannelTool {
			tool.WriteString(result.Content)
		}
	}

	if got := tool.String(); !strings.HasPrefix(got, toolsOpen) || !strings.HasSuffix(got, toolsClose) {
		t.Errorf("tool content = %q, want complete Kimi tools block", got)
	}
}

func TestStateMachineMultipleToolBlocks(t *testing.T) {
	first := toolsOpen + callOpen + ` tool="first" index="1"` + sepToken + callClose + toolsClose
	second := toolsOpen + callOpen + ` tool="second" index="1"` + sepToken + callClose + toolsClose

	for _, tt := range []struct {
		name   string
		tokens []string
	}{
		{"one-token", []string{first + second}},
		{"separate-tokens", []string{first, second}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sm := Parser{}.NewStateMachine()
			streamer := sm.(model.ToolCallDeltaStreamer)
			var content strings.Builder
			for _, token := range tt.tokens {
				result, _ := sm.Classify(token)
				content.WriteString(result.Content)
			}

			calls := Parser{}.ToolCall(t.Context(), nil, content.String())
			if len(calls) != 2 || calls[0].Function.Name != "first" || calls[1].Function.Name != "second" {
				t.Fatalf("ToolCall: got %+v, want complete calls [first second]", calls)
			}
			deltas := streamer.ToolCallDeltas()
			if len(deltas) != 2 || deltas[0].Index != 0 || deltas[1].Index != 1 ||
				deltas[0].ID == "" || deltas[1].ID == "" || deltas[0].ID == deltas[1].ID {
				t.Errorf("ToolCallDeltas: got %+v, want distinct calls at indexes 0 and 1", deltas)
			}
		})
	}
}

func TestStateMachinePreservesMalformedPostToolContent(t *testing.T) {
	first := toolsOpen + callOpen + ` tool="write_file" index="1"` + sepToken + callClose + toolsClose
	second := toolsOpen + callOpen + ` tool="bash" index="1"` + sepToken + callClose + toolsClose

	for _, tt := range []struct {
		name string
		tail []string
	}{
		{name: "whole", tail: []string{argumentClose + callClose + toolsClose}},
		{name: "split", tail: []string{"<|cl", "ose|>argument" + callClose + toolsClose}},
		{name: "prefixed", tail: []string{"unexpected" + argumentClose + callClose + toolsClose}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sm := Parser{}.NewStateMachine()
			var aggregate strings.Builder
			for _, token := range append([]string{first, second}, tt.tail...) {
				result, _ := sm.Classify(token)
				if result.Channel == model.ChannelTool {
					aggregate.WriteString(result.Content)
				}
			}
			calls := Parser{}.ToolCall(t.Context(), nil, aggregate.String())
			if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
				t.Fatalf("ToolCall: got %+v, want one non-executable failed call", calls)
			}
		})
	}
}

func TestStateMachineEndOfMessageAfterTools(t *testing.T) {
	block := toolsOpen + callOpen + ` tool="ping" index="1"` + sepToken + callClose + toolsClose
	for _, marker := range []string{"<|end_of_msg|>", " \n<|end_of_msg|>"} {
		sm := Parser{}.NewStateMachine()
		result, _ := sm.Classify(block)
		if result.Channel != model.ChannelTool || result.Content != block {
			t.Fatalf("tool result = %+v, want complete block", result)
		}
		result, eog := sm.Classify(marker)
		if result != (model.Result{}) || !eog {
			t.Fatalf("end marker %q: got (%+v, %v), want empty EOG", marker, result, eog)
		}
	}

	sm := Parser{}.NewStateMachine()
	result, eog := sm.Classify(block + "<|end_of_msg|>")
	if result.Channel != model.ChannelTool || result.Content != block || eog {
		t.Fatalf("coalesced end marker: got (%+v, %v), want tool block without immediate EOG", result, eog)
	}
	if tail := sm.(model.StateMachineFlusher).Flush(); tail != (model.Result{}) {
		t.Fatalf("coalesced end marker Flush = %+v, want empty", tail)
	}
}

func TestStateMachineToolActivityAndCompleteRelease(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	streamer := sm.(model.ToolCallDeltaStreamer)
	first := callOpen + ` tool="weather" index="1"` + sepToken + callClose
	second := callOpen + ` tool="clock" index="2"` + sepToken + callClose
	block := toolsOpen + first + second + toolsClose

	for _, token := range []string{toolsOpen, "<|op", `en|>call tool="wea`, `ther" index="1"<|sep|>` + callClose, second} {
		assertResult(t, sm, token, model.ChannelNone, "", false)
	}
	deltas := streamer.ToolCallDeltas()
	if len(deltas) != 2 {
		t.Fatalf("ToolCallDeltas: got %d, want 2", len(deltas))
	}
	for i, wantName := range []string{"weather", "clock"} {
		if deltas[i].ID == "" || deltas[i].Index != i || deltas[i].Type != "function" ||
			deltas[i].Function.Name != wantName || deltas[i].Function.Arguments != "" {
			t.Errorf("delta %d: got %+v, want name %q at index %d", i, deltas[i], wantName, i)
		}
	}
	if got := streamer.ToolCallDeltas(); len(got) != 0 {
		t.Errorf("drained ToolCallDeltas: got %d, want 0", len(got))
	}
	assertResult(t, sm, toolsClose+"ignored", model.ChannelTool, block, false)
	remainder := sm.(model.StateMachineFlusher).Flush()
	if remainder.Channel != model.ChannelTool || remainder.Content != "ignored" {
		t.Fatalf("Flush malformed remainder: got %+v, want tool content %q", remainder, "ignored")
	}
	calls := Parser{}.ToolCall(t.Context(), nil, block+remainder.Content)
	if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
		t.Fatalf("ToolCall malformed aggregate: got %+v, want one non-executable failed call", calls)
	}
	started := streamer.StartedToolCalls()
	if len(started) != 2 || started[0].ID != deltas[0].ID || started[1].ID != deltas[1].ID {
		t.Errorf("StartedToolCalls: got %+v, want delta identities", started)
	}
}

func TestStateMachineFlushAndResetToolBlock(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	partial := toolsOpen + callOpen + ` tool="ping" index="1"` + sepToken
	assertResult(t, sm, partial, model.ChannelNone, "", false)
	got := sm.(model.StateMachineFlusher).Flush()
	if got.Channel != model.ChannelTool || got.Content != partial {
		t.Errorf("Flush: got {%v %q}, want {%v %q}", got.Channel, got.Content, model.ChannelTool, partial)
	}

	sm.Reset()
	streamer := sm.(model.ToolCallDeltaStreamer)
	if len(streamer.ToolCallDeltas()) != 0 || len(streamer.StartedToolCalls()) != 0 {
		t.Error("Reset did not clear tool-call delta state")
	}
	assertResult(t, sm, "answer", model.ChannelAnswer, "answer", false)
}

func TestStateMachinePendingFalseAlarmAndReset(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	assertResult(t, sm, "<", model.ChannelNone, "", false)
	assertResult(t, sm, "not-kimi", model.ChannelAnswer, "<not-kimi", false)

	sm.Classify("<|op")
	sm.Reset()
	assertResult(t, sm, "answer", model.ChannelAnswer, "answer", false)
}

func TestStateMachineFlushPendingTag(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	assertResult(t, sm, thinkOpen, model.ChannelNone, "", false)
	assertResult(t, sm, "<|op", model.ChannelNone, "", false)

	flusher := sm.(model.StateMachineFlusher)
	got := flusher.Flush()
	if got.Channel != model.ChannelReasoning || got.Content != "<|op" {
		t.Errorf("Flush: got {%v %q}, want {%v %q}", got.Channel, got.Content, model.ChannelReasoning, "<|op")
	}
	if got := flusher.Flush(); got != (model.Result{}) {
		t.Errorf("second Flush: got %+v, want zero result", got)
	}
}

func TestToolMessageChunkBoundaries(t *testing.T) {
	parser := Parser{}
	prompt := `<|open|>message role="assistant"<|sep|><|open|>response<|sep|><|close|>response<|sep|><|open|>tools<|sep|><|open|>call tool="ping" index="1"<|sep|><|open|>argument key="text" type="string"<|sep|>hello<|close|>argument<|sep|><|close|>call<|sep|><|close|>tools<|sep|><|close|>message<|sep|><|end_of_msg|>`
	for size := 1; size <= len(prompt); size++ {
		t.Run(fmt.Sprintf("chunk-%d", size), func(t *testing.T) {
			calls := classifyToolChunks(t, parser, prompt, size)
			if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "ping" || calls[0].Function.Arguments["text"] != "hello" {
				t.Fatalf("ToolCall: got %+v, want executable ping(text=hello)", calls)
			}
		})
	}
}

func TestStateMachineToolTrailers(t *testing.T) {
	block := toolsOpen + callOpen + ` tool="ping" index="1"` + sepToken + callClose + toolsClose
	for _, tt := range []struct {
		name    string
		tail    string
		wantErr bool
	}{
		{name: "message-close", tail: messageClose},
		{name: "message-close-eom", tail: messageClose + endOfMessage},
		{name: "eom", tail: endOfMessage},
		{name: "whitespace", tail: " \n" + messageClose + "\t" + endOfMessage},
		{name: "garbage", tail: "unexpected", wantErr: true},
		{name: "argument-close", tail: argumentClose, wantErr: true},
		{name: "garbage-after-close", tail: messageClose + "unexpected", wantErr: true},
		{name: "duplicate-close", tail: messageClose + messageClose, wantErr: true},
		{name: "partial-close", tail: messageClose[:len(messageClose)-1], wantErr: true},
		{name: "partial-eom", tail: messageClose + "<|end_of_", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Vary chunk sizes, including fully coalesced tools and trailer.
			input := block + tt.tail
			for split := 1; split <= len(input); split++ {
				calls := classifyToolChunks(t, Parser{}, input, split)
				if len(calls) != 1 || (calls[0].Status != 0) != tt.wantErr {
					t.Fatalf("chunk size %d: got %+v, want error %v", split, calls, tt.wantErr)
				}
			}
		})
	}

	// Content in the same piece as EOM is still validated. Do not call the
	// state machine after it has already returned EOG on an earlier piece.
	input := block + messageClose + endOfMessage + "unexpected"
	calls := classifyToolChunks(t, Parser{}, input, len(input))
	if len(calls) != 1 || calls[0].Status == 0 {
		t.Fatalf("coalesced garbage after EOM: got %+v, want failed call", calls)
	}
}

func classifyToolChunks(t *testing.T, parser model.Parser, input string, size int) []model.ResponseToolCall {
	t.Helper()
	sm := parser.NewStateMachine()
	var tool strings.Builder
	for offset := 0; offset < len(input); offset += size {
		result, eog := sm.Classify(input[offset:min(offset+size, len(input))])
		if result.Channel == model.ChannelTool {
			tool.WriteString(result.Content)
		}
		if eog {
			break
		}
	}
	flusher := sm.(model.StateMachineFlusher)
	for result := flusher.Flush(); result != (model.Result{}); result = flusher.Flush() {
		if result.Channel == model.ChannelTool {
			tool.WriteString(result.Content)
		}
	}
	return parser.ToolCall(t.Context(), nil, tool.String())
}

func assertResult(t *testing.T, sm model.StateMachine, token string, channel model.Channel, content string, eog bool) {
	t.Helper()
	result, gotEOG := sm.Classify(token)
	if result.Channel != channel || result.Content != content || gotEOG != eog {
		t.Errorf("Classify(%q) = {%v %q}, %v; want {%v %q}, %v",
			token, result.Channel, result.Content, gotEOG, channel, content, eog)
	}
}

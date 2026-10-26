package deepseek

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestNewClaimsDeepSeek(t *testing.T) {
	constructedTemplate := `{%- set dsml_token = '｜DSML｜' -%}
{{- '<' + dsml_token + 'tool_calls>' -}}
{{- '<' + dsml_token + 'invoke name="' + func['name'] + '">' -}}
{{- '<' + dsml_token + 'parameter name="' + key + '">' -}}`

	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		{"constructed-template", model.Fingerprint{ChatTemplate: constructedTemplate}, true},
		{"literal-template", model.Fingerprint{ChatTemplate: toolCallsOpen + invokeOpen + parameterOpen}, true},
		{"v4.1-literal-template", model.Fingerprint{ChatTemplate: `<｜DSML｜ calls><｜DSML｜ invoke name="ping"><｜DSML｜ parameter name="value" string="true">`}, true},
		{"v4.1-constructed-template", model.Fingerprint{ChatTemplate: `{% set dsml_token = '｜DSML｜' %}{{ '<' + dsml_token + ' calls>' }}{{ '<' + dsml_token + ' invoke name="' }}{{ '<' + dsml_token + ' parameter name="' }}`}, true},
		{"architecture", model.Fingerprint{Architecture: "deepseek4"}, true},
		{"v4.1-architecture", model.Fingerprint{Architecture: "deepseek41"}, true},
		{"architecture-case", model.Fingerprint{Architecture: "DeepSeek2"}, true},
		{"model-name", model.Fingerprint{ModelName: "DeepSeek-V4-Flash"}, true},
		{"different-token", model.Fingerprint{ChatTemplate: strings.ReplaceAll(constructedTemplate, dsmlToken, "CUSTOM")}, false},
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

func TestStateMachineReasoningThenAnswer(t *testing.T) {
	sm := Parser{}.NewStateMachine()

	assertResult(t, sm, "<think>", model.ChannelNone, "", false)
	assertResult(t, sm, "plan", model.ChannelReasoning, "plan", false)
	assertResult(t, sm, "</think>", model.ChannelNone, "", false)
	assertResult(t, sm, "answer", model.ChannelAnswer, "answer", false)
}

func TestStateMachineSplitDSMLBlock(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	fragments := []string{
		"\n\n<", dsmlToken, "tool_", "calls>",
		"\n<", dsmlToken, `invoke name="get_weather">`,
		"\n<", dsmlToken, `parameter name="location" string="true">New York`,
		" City</", dsmlToken, "parameter>",
		"\n</", dsmlToken, "invoke>",
		"\n</", dsmlToken, "tool_calls>",
	}

	var tool strings.Builder
	for _, fragment := range fragments {
		result, eog := sm.Classify(fragment)
		if eog {
			t.Fatalf("Classify(%q) returned unexpected EOG", fragment)
		}
		if result.Channel == model.ChannelTool {
			tool.WriteString(result.Content)
		} else if result.Channel != model.ChannelNone && result.Channel != model.ChannelAnswer {
			t.Fatalf("Classify(%q) channel = %v, want none, answer, or tool", fragment, result.Channel)
		}
	}

	if got := tool.String(); !strings.HasPrefix(got, toolCallsOpen) || !strings.HasSuffix(got, toolCallsClose) {
		t.Errorf("tool content = %q, want complete DSML block", got)
	}

}

func TestStateMachineSingleChunkDSMLBlock(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	block := toolCallsOpen + invokeOpen + ` name="ping">` + invokeClose + toolCallsClose

	assertResult(t, sm, block, model.ChannelTool, block, false)
}

func TestStateMachineV41ToolCalls(t *testing.T) {
	const block = `<｜DSML｜ calls>
<｜DSML｜ invoke name="get_time">
<｜DSML｜ parameter name="city" string="true">Tokyo</｜DSML｜ parameter>
</｜DSML｜ invoke>
<｜DSML｜ invoke name="get_weather">
<｜DSML｜ parameter name="city" string="true">Paris</｜DSML｜ parameter>
</｜DSML｜ invoke>
</｜DSML｜ calls>`

	// Include both unsplit output and every possible byte boundary, including
	// boundaries inside the DSML token, tag names, attributes and closing tags.
	for split := 0; split <= len(block); split++ {
		sm := Parser{}.NewStateMachine()
		assertResult(t, sm, "<think>", model.ChannelNone, "", false)
		assertResult(t, sm, "plan", model.ChannelReasoning, "plan", false)
		assertResult(t, sm, "</think>", model.ChannelNone, "", false)

		var tools strings.Builder
		for _, fragment := range []string{block[:split], block[split:]} {
			if fragment == "" {
				continue
			}
			result, eog := sm.Classify(fragment)
			if eog || (result.Channel != model.ChannelNone && result.Channel != model.ChannelTool) {
				t.Fatalf("split %d: Classify got %+v, EOG %v, want only tool content", split, result, eog)
			}
			tools.WriteString(result.Content)
		}
		for result := sm.(model.StateMachineFlusher).Flush(); result != (model.Result{}); result = sm.(model.StateMachineFlusher).Flush() {
			if result.Channel != model.ChannelTool {
				t.Fatalf("split %d: Flush got %+v, want tool content", split, result)
			}
			tools.WriteString(result.Content)
		}
		if tools.String() != block {
			t.Fatalf("split %d: tool output got %q, want %q", split, tools.String(), block)
		}

		calls := Parser{}.ToolCall(t.Context(), nil, tools.String())
		if len(calls) != 2 || calls[0].Status != 0 || calls[1].Status != 0 ||
			calls[0].Function.Name != "get_time" || calls[0].Function.Arguments["city"] != "Tokyo" ||
			calls[1].Function.Name != "get_weather" || calls[1].Function.Arguments["city"] != "Paris" {
			t.Fatalf("split %d: ToolCall got %+v, want time in Tokyo and weather in Paris", split, calls)
		}
		deltas := sm.(model.ToolCallDeltaStreamer).ToolCallDeltas()
		if len(deltas) != 2 || deltas[0].Function.Name != "get_time" || deltas[1].Function.Name != "get_weather" ||
			deltas[0].Index != 0 || deltas[1].Index != 1 || deltas[0].ID == "" || deltas[0].ID == deltas[1].ID {
			t.Fatalf("split %d: deltas got %+v, want two distinct ordered calls", split, deltas)
		}

		sm.Reset()
		legacy := `<｜DSML｜tool_calls><｜DSML｜invoke name="ping"></｜DSML｜invoke></｜DSML｜tool_calls>`
		assertResult(t, sm, legacy, model.ChannelTool, legacy, false)
	}
}

func TestStateMachineV41ToolActivityBeforeCompletion(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	streamer := sm.(model.ToolCallDeltaStreamer)
	for _, fragment := range []string{"<", "｜DSML｜", " calls>", "<｜DSML｜ inv", `oke name="get_`} {
		assertResult(t, sm, fragment, model.ChannelNone, "", false)
	}
	if got := streamer.ToolCallDeltas(); len(got) != 0 {
		t.Fatalf("incomplete name: got %+v, want no activity", got)
	}
	assertResult(t, sm, `time">`, model.ChannelNone, "", false)
	deltas := streamer.ToolCallDeltas()
	if len(deltas) != 1 || deltas[0].Function.Name != "get_time" || deltas[0].ID == "" {
		t.Fatalf("complete opener: got %+v, want get_time activity before arguments arrive", deltas)
	}
	partial := sm.(model.StateMachineFlusher).Flush()
	calls := Parser{}.ToolCall(t.Context(), nil, partial.Content)
	if partial.Channel != model.ChannelTool || len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
		t.Fatalf("truncated output: got %+v and %+v, want non-executable tool content", partial, calls)
	}
}

func TestStateMachinePreservesAnswerBeforeCompleteToolBlock(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	block := toolCallsOpen + invokeOpen + ` name="ping">` + invokeClose + toolCallsClose

	assertResult(t, sm, "answer"+block, model.ChannelAnswer, "answer", false)
	got := sm.(model.StateMachineFlusher).Flush()
	if got.Channel != model.ChannelTool || got.Content != block {
		t.Errorf("Flush: got {%v %q}, want {%v %q}", got.Channel, got.Content, model.ChannelTool, block)
	}
}

func TestStateMachineMultipleToolBlocks(t *testing.T) {
	first := toolCallsOpen + invokeOpen + ` name="first">` + invokeClose + toolCallsClose
	second := toolCallsOpen + invokeOpen + ` name="second">` + invokeClose + toolCallsClose

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

func TestStateMachineEveryToolOpenerSplit(t *testing.T) {
	for splitAt := 1; splitAt < len(toolCallsOpen); splitAt++ {
		t.Run(fmt.Sprintf("split-%d", splitAt), func(t *testing.T) {
			sm := Parser{}.NewStateMachine()
			assertResult(t, sm, toolCallsOpen[:splitAt], model.ChannelNone, "", false)
			assertResult(t, sm, toolCallsOpen[splitAt:], model.ChannelNone, "", false)
		})
	}
}

func TestStateMachinePreservesMalformedPostToolContent(t *testing.T) {
	first := toolCallsOpen + invokeOpen + ` name="write_file">` + invokeClose + toolCallsClose
	second := toolCallsOpen + invokeOpen + ` name="bash">` + invokeClose + toolCallsClose

	for _, tt := range []struct {
		name string
		tail []string
	}{
		{name: "whole", tail: []string{parameterClose + invokeClose + toolCallsClose}},
		{name: "split", tail: []string{"</", "｜DSML｜parameter>" + invokeClose + toolCallsClose}},
		{name: "prefixed", tail: []string{"unexpected" + parameterClose + invokeClose + toolCallsClose}},
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

func TestStateMachineFlushesPostToolOpenerPrefixesAsToolContent(t *testing.T) {
	block := toolCallsOpen + invokeOpen + ` name="first">` + invokeClose + toolCallsClose
	for splitAt := 1; splitAt < len(toolCallsOpen); splitAt++ {
		sm := Parser{}.NewStateMachine()
		complete, _ := sm.Classify(block)
		sm.Classify(toolCallsOpen[:splitAt])
		tail := sm.(model.StateMachineFlusher).Flush()
		if tail.Channel != model.ChannelTool || tail.Content != toolCallsOpen[:splitAt] {
			t.Fatalf("split %d: Flush got %+v, want tool suffix", splitAt, tail)
		}
		calls := Parser{}.ToolCall(t.Context(), nil, complete.Content+tail.Content)
		if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" {
			t.Fatalf("split %d: ToolCall got %+v, want one non-executable failure", splitAt, calls)
		}
	}
}

func TestStateMachineBuffersIncompleteToolBlock(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	assertResult(t, sm, toolCallsOpen, model.ChannelNone, "", false)
	assertResult(t, sm, invokeOpen, model.ChannelNone, "", false)
}

func TestStateMachineToolActivityAndCompleteRelease(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	streamer := sm.(model.ToolCallDeltaStreamer)
	first := invokeOpen + ` name="get_weather">` + invokeClose
	second := invokeOpen + ` name="get_time">` + invokeClose
	block := toolCallsOpen + first + second + toolCallsClose

	for _, token := range []string{toolCallsOpen, "<｜DSML｜inv", `oke name="get_`, `weather">` + invokeClose, second} {
		assertResult(t, sm, token, model.ChannelNone, "", false)
	}
	deltas := streamer.ToolCallDeltas()
	if len(deltas) != 2 {
		t.Fatalf("ToolCallDeltas: got %d, want 2", len(deltas))
	}
	for i, wantName := range []string{"get_weather", "get_time"} {
		if deltas[i].ID == "" || deltas[i].Index != i || deltas[i].Type != "function" ||
			deltas[i].Function.Name != wantName || deltas[i].Function.Arguments != "" {
			t.Errorf("delta %d: got %+v, want name %q at index %d", i, deltas[i], wantName, i)
		}
	}
	if got := streamer.ToolCallDeltas(); len(got) != 0 {
		t.Errorf("drained ToolCallDeltas: got %d, want 0", len(got))
	}
	assertResult(t, sm, toolCallsClose+"ignored", model.ChannelTool, block, false)
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
	partial := toolCallsOpen + invokeOpen + ` name="ping">`
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

func TestStateMachinePendingFalseAlarm(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	assertResult(t, sm, "<", model.ChannelNone, "", false)
	assertResult(t, sm, "not-dsml", model.ChannelAnswer, "<not-dsml", false)
}

func TestStateMachineFlushPendingOpener(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	assertResult(t, sm, "<think>", model.ChannelNone, "", false)
	assertResult(t, sm, "<", model.ChannelNone, "", false)

	flusher := sm.(model.StateMachineFlusher)
	got := flusher.Flush()
	if got.Channel != model.ChannelReasoning || got.Content != "<" {
		t.Errorf("Flush: got {%v %q}, want {%v %q}", got.Channel, got.Content, model.ChannelReasoning, "<")
	}
	if got := flusher.Flush(); got != (model.Result{}) {
		t.Errorf("second Flush: got %+v, want zero result", got)
	}
}

func TestStateMachineReset(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	sm.Classify("<")
	sm.Reset()

	assertResult(t, sm, "answer", model.ChannelAnswer, "answer", false)
}

func assertResult(t *testing.T, sm model.StateMachine, token string, channel model.Channel, content string, eog bool) {
	t.Helper()

	result, gotEOG := sm.Classify(token)
	if result.Channel != channel {
		t.Errorf("Classify(%q) channel = %v, want %v", token, result.Channel, channel)
	}
	if result.Content != content {
		t.Errorf("Classify(%q) content = %q, want %q", token, result.Content, content)
	}
	if gotEOG != eog {
		t.Errorf("Classify(%q) eog = %v, want %v", token, gotEOG, eog)
	}
}

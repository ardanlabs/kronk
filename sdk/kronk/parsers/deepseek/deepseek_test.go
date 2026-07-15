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
		{"architecture", model.Fingerprint{Architecture: "deepseek4"}, true},
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

func TestStateMachineEveryToolOpenerSplit(t *testing.T) {
	for splitAt := 1; splitAt < len(toolCallsOpen); splitAt++ {
		t.Run(fmt.Sprintf("split-%d", splitAt), func(t *testing.T) {
			sm := Parser{}.NewStateMachine()
			assertResult(t, sm, toolCallsOpen[:splitAt], model.ChannelNone, "", false)
			assertResult(t, sm, toolCallsOpen[splitAt:], model.ChannelTool, toolCallsOpen, false)
		})
	}
}

func TestStateMachineStreamsIncompleteToolBlock(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	assertResult(t, sm, toolCallsOpen, model.ChannelTool, toolCallsOpen, false)
	assertResult(t, sm, invokeOpen, model.ChannelTool, invokeOpen, false)
}

func TestStateMachinePendingFalseAlarm(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	assertResult(t, sm, "<", model.ChannelNone, "", false)
	assertResult(t, sm, "not-dsml", model.ChannelAnswer, "<not-dsml", false)
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

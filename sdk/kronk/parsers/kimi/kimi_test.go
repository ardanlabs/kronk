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
		{"literal-template", model.Fingerprint{ChatTemplate: thinkOpen + responseOpen + toolsOpen}, true},
		{"architecture", model.Fingerprint{Architecture: "kimi-k3"}, true},
		{"architecture-case", model.Fingerprint{Architecture: "KimiK3"}, true},
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
	assertResult(t, sm, toolsOpen, model.ChannelTool, toolsOpen, false)
	assertResult(t, sm, callOpen, model.ChannelTool, callOpen, false)
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

func TestReasoningNormalization(t *testing.T) {
	p := Parser{}
	input := "before" + thinkOpen + "private" + thinkClose + "after"
	if got := p.StripReasoningContent(input); got != "beforeafter" {
		t.Errorf("StripReasoningContent() = %q, want %q", got, "beforeafter")
	}

	rendered := thinkOpen + " \n" + thinkClose + "history" + thinkOpen
	if got := p.StripEmptyReasoning(rendered); got != "history"+thinkOpen {
		t.Errorf("StripEmptyReasoning() = %q, want %q", got, "history"+thinkOpen)
	}
}

func assertResult(t *testing.T, sm model.StateMachine, token string, channel model.Channel, content string, eog bool) {
	t.Helper()
	result, gotEOG := sm.Classify(token)
	if result.Channel != channel || result.Content != content || gotEOG != eog {
		t.Errorf("Classify(%q) = {%v %q}, %v; want {%v %q}, %v",
			token, result.Channel, result.Content, gotEOG, channel, content, eog)
	}
}

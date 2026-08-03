package fallback

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestNewAlwaysClaimsAsFallback(t *testing.T) {
	for _, fp := range []model.Fingerprint{{}, {ModelName: "unknown"}, {ChatTemplate: "anything"}} {
		parser, ok := New(fp)
		if !ok {
			t.Errorf("New(%+v): got false, want true", fp)
			continue
		}
		if got := parser.Name(); got != "fallback" {
			t.Errorf("Name: got %q, want fallback", got)
		}
	}
}

func TestParserPassesUnmarkedToolLikeContentAsAnswer(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	content := `<tool_call>{"name":"x","arguments":{}}</tool_call>`
	got, eog := sm.Classify(content)
	if eog {
		t.Fatal("Classify: got EOG, want false")
	}
	if got.Channel != model.ChannelAnswer || got.Content != content {
		t.Errorf("Classify: got %+v, want answer content", got)
	}
}

func TestParserReasoningThenAnswer(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	sm.Classify("<think>")
	got, _ := sm.Classify("reason")
	if got.Channel != model.ChannelReasoning || got.Content != "reason" {
		t.Errorf("reasoning: got %+v", got)
	}
	sm.Classify("</think>")
	got, _ = sm.Classify("answer")
	if got.Channel != model.ChannelAnswer || got.Content != "answer" {
		t.Errorf("answer: got %+v", got)
	}
}

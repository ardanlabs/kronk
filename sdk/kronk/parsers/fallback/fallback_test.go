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

func TestParserReasoningMarkersAtEverySplit(t *testing.T) {
	const stream = "<think>reason</think>answer"
	for split := range len(stream) + 1 {
		sm := Parser{}.NewStateMachine()
		var reasoning, answer string
		for _, fragment := range []string{stream[:split], stream[split:]} {
			result, _ := sm.Classify(fragment)
			switch result.Channel {
			case model.ChannelReasoning:
				reasoning += result.Content
			case model.ChannelAnswer:
				answer += result.Content
			}
		}
		flusher := sm.(model.StateMachineFlusher)
		for result := flusher.Flush(); result != (model.Result{}); result = flusher.Flush() {
			switch result.Channel {
			case model.ChannelReasoning:
				reasoning += result.Content
			case model.ChannelAnswer:
				answer += result.Content
			}
		}
		if reasoning != "reason" || answer != "answer" {
			t.Errorf("split %d: got reasoning %q answer %q", split, reasoning, answer)
		}
	}
}

func TestParserFlushesIncompleteReasoningMarker(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	if result, _ := sm.Classify("answer<thi"); result.Content != "answer" {
		t.Fatalf("Classify: got %+v, want answer content", result)
	}
	result := sm.(model.StateMachineFlusher).Flush()
	if result.Channel != model.ChannelAnswer || result.Content != "<thi" {
		t.Errorf("Flush: got %+v, want incomplete marker as answer", result)
	}
}

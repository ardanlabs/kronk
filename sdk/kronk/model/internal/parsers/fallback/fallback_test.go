package fallback

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

func TestNewAlwaysClaimsAsFallback(t *testing.T) {
	for _, fp := range []parsers.Fingerprint{{}, {ModelName: "unknown"}, {ChatTemplate: "anything"}} {
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
	if got.Channel != parsers.ChannelAnswer || got.Content != content {
		t.Errorf("Classify: got %+v, want answer content", got)
	}
}

func TestParserReasoningThenAnswer(t *testing.T) {
	sm := Parser{}.NewStateMachine()
	sm.Classify("<think>")
	got, _ := sm.Classify("reason")
	if got.Channel != parsers.ChannelReasoning || got.Content != "reason" {
		t.Errorf("reasoning: got %+v", got)
	}
	sm.Classify("</think>")
	got, _ = sm.Classify("answer")
	if got.Channel != parsers.ChannelAnswer || got.Content != "answer" {
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
			case parsers.ChannelReasoning:
				reasoning += result.Content
			case parsers.ChannelAnswer:
				answer += result.Content
			}
		}
		flusher := sm.(parsers.StateMachineFlusher)
		for result := flusher.Flush(); result != (parsers.Result{}); result = flusher.Flush() {
			switch result.Channel {
			case parsers.ChannelReasoning:
				reasoning += result.Content
			case parsers.ChannelAnswer:
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
	result := sm.(parsers.StateMachineFlusher).Flush()
	if result.Channel != parsers.ChannelAnswer || result.Content != "<thi" {
		t.Errorf("Flush: got %+v, want incomplete marker as answer", result)
	}
}

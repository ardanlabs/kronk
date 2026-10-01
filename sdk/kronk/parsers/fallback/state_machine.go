package fallback

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// stateMachine classifies plain answers and the common reasoning wrapper.
type stateMachine struct {
	status  model.Channel
	pending string
	queue   []model.Result
}

// Reset returns the state machine to answer mode.
func (sm *stateMachine) Reset() {
	*sm = stateMachine{status: model.ChannelAnswer}
}

// Classify classifies one decoded token.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	sm.pending += content
	for sm.pending != "" {
		at, marker := nextReasoningMarker(sm.pending)
		if at >= 0 {
			sm.enqueue(sm.status, sm.pending[:at])
			sm.pending = sm.pending[at+len(marker):]
			if marker == thinkOpen {
				sm.status = model.ChannelReasoning
			} else {
				sm.status = model.ChannelAnswer
			}
			continue
		}

		keep := partialReasoningMarkerSuffix(sm.pending)
		sm.enqueue(sm.status, sm.pending[:len(sm.pending)-keep])
		sm.pending = sm.pending[len(sm.pending)-keep:]
		break
	}

	return sm.dequeue(), false
}

func nextReasoningMarker(content string) (int, string) {
	best := -1
	var marker string
	for _, candidate := range [...]string{thinkOpen, thinkClose} {
		if at := strings.Index(content, candidate); at >= 0 && (best == -1 || at < best) {
			best = at
			marker = candidate
		}
	}
	return best, marker
}

func partialReasoningMarkerSuffix(content string) int {
	best := 0
	for _, marker := range [...]string{thinkOpen, thinkClose} {
		for size := 1; size < len(marker) && size <= len(content); size++ {
			if strings.HasSuffix(content, marker[:size]) {
				best = max(best, size)
			}
		}
	}
	return best
}

func (sm *stateMachine) enqueue(channel model.Channel, content string) {
	if content == "" {
		return
	}
	if len(sm.queue) > 0 && sm.queue[len(sm.queue)-1].Channel == channel {
		sm.queue[len(sm.queue)-1].Content += content
		return
	}
	sm.queue = append(sm.queue, model.Result{Channel: channel, Content: content})
}

func (sm *stateMachine) dequeue() model.Result {
	if len(sm.queue) == 0 {
		return model.Result{}
	}
	result := sm.queue[0]
	sm.queue = sm.queue[1:]
	return result
}

// Flush drains queued content and incomplete reasoning markers.
func (sm *stateMachine) Flush() model.Result {
	if len(sm.queue) > 0 {
		return sm.dequeue()
	}
	if sm.pending == "" {
		return model.Result{}
	}
	result := model.Result{Channel: sm.status, Content: sm.pending}
	sm.pending = ""
	return result
}

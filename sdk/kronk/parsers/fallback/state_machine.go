package fallback

import "github.com/ardanlabs/kronk/sdk/kronk/model"

// stateMachine classifies plain answers and the common reasoning wrapper.
type stateMachine struct {
	status model.Channel
}

// Reset returns the state machine to answer mode.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
}

// Classify classifies one decoded token.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	switch content {
	case "<think>":
		sm.status = model.ChannelReasoning
		return model.Result{}, false

	case "</think>":
		sm.status = model.ChannelAnswer
		return model.Result{}, false

	default:
		return model.Result{Channel: sm.status, Content: content}, false
	}
}

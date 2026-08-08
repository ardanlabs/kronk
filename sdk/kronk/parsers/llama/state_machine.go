package llama

import (
	"encoding/json"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const (
	pythonTag  = "<|python_tag|>"
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// stateMachine recognizes Llama markers without depending on tokenizer chunk
// boundaries. Tool candidates are retained until EOS so only complete output
// can be classified as executable.
type stateMachine struct {
	status        model.Channel
	pending       strings.Builder
	toolNames     map[string]struct{}
	answerStarted bool
	marked        bool
	queue         []model.Result
}

// Reset returns the state machine to its initial state.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.pending.Reset()
	sm.toolNames = nil
	sm.answerStarted = false
	sm.marked = false
	sm.queue = nil
}

// SetTools supplies the request's declared tools.
func (sm *stateMachine) SetTools(tools []model.D) {
	sm.toolNames = make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if name := declaredToolName(tool); name != "" {
			sm.toolNames[name] = struct{}{}
		}
	}
}

// Classify classifies one decoded piece.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	sm.process(content)
	if len(sm.queue) == 0 {
		return model.Result{}, false
	}

	result := sm.queue[0]
	sm.queue = sm.queue[1:]
	return result, false
}

func (sm *stateMachine) process(content string) {
	if sm.marked {
		sm.pending.WriteString(content)
		return
	}
	if sm.status == model.ChannelReasoning {
		sm.processReasoning(content)
		return
	}
	if sm.answerStarted {
		sm.enqueue(model.ChannelAnswer, content)
		return
	}

	sm.pending.WriteString(content)
	candidate := sm.pending.String()
	trimmed := strings.TrimLeft(candidate, " \t\r\n")
	leading := candidate[:len(candidate)-len(trimmed)]

	if strings.HasPrefix(pythonTag, trimmed) && len(trimmed) < len(pythonTag) {
		return
	}
	if strings.HasPrefix(trimmed, pythonTag) {
		sm.marked = true
		sm.status = model.ChannelTool
		return
	}
	if strings.HasPrefix(thinkOpen, trimmed) && len(trimmed) < len(thinkOpen) {
		return
	}
	if strings.HasPrefix(trimmed, thinkOpen) {
		sm.pending.Reset()
		sm.status = model.ChannelReasoning
		remainder := trimmed[len(thinkOpen):]
		sm.enqueue(model.ChannelAnswer, leading)
		sm.processReasoning(remainder)
		return
	}
	if trimmed == "" {
		return
	}
	if len(sm.toolNames) > 0 && strings.HasPrefix(trimmed, "{") {
		if name, ok := envelopeName(trimmed); ok {
			if _, declared := sm.toolNames[name]; declared {
				return
			}
			sm.pending.Reset()
			sm.answerStarted = true
			sm.enqueue(model.ChannelAnswer, candidate)
			return
		}
		if !firstJSONValueComplete(trimmed) {
			return
		}
	}

	sm.pending.Reset()
	sm.answerStarted = true
	sm.enqueue(model.ChannelAnswer, candidate)
}

func firstJSONValueComplete(content string) bool {
	decoder := json.NewDecoder(strings.NewReader(content))
	var value json.RawMessage
	return decoder.Decode(&value) == nil
}

func (sm *stateMachine) processReasoning(content string) {
	sm.pending.WriteString(content)
	candidate := sm.pending.String()
	if before, after, ok := strings.Cut(candidate, thinkClose); ok {
		sm.pending.Reset()
		sm.status = model.ChannelAnswer
		sm.enqueue(model.ChannelReasoning, before)
		sm.process(after)
		return
	}

	keep := markerPrefixSuffix(candidate, thinkClose)
	emit := candidate[:len(candidate)-keep]
	sm.pending.Reset()
	sm.pending.WriteString(candidate[len(candidate)-keep:])
	sm.enqueue(model.ChannelReasoning, emit)
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

func markerPrefixSuffix(content, marker string) int {
	for size := min(len(content), len(marker)-1); size > 0; size-- {
		if strings.HasSuffix(content, marker[:size]) {
			return size
		}
	}
	return 0
}

// Flush drains buffered output at EOS.
func (sm *stateMachine) Flush() model.Result {
	if len(sm.queue) > 0 {
		result := sm.queue[0]
		sm.queue = sm.queue[1:]
		return result
	}
	if sm.pending.Len() == 0 {
		return model.Result{}
	}

	content := sm.pending.String()
	sm.pending.Reset()
	if sm.marked {
		if name, ok := envelopeName(content); ok {
			if _, declared := sm.toolNames[name]; !declared {
				return model.Result{Channel: model.ChannelAnswer, Content: content}
			}
		}
		return model.Result{Channel: model.ChannelTool, Content: content}
	}
	if sm.status == model.ChannelReasoning {
		return model.Result{Channel: model.ChannelReasoning, Content: content}
	}
	trimmed := trimASCIIWhitespace(content)
	if name, ok := envelopeName(trimmed); ok {
		if _, declared := sm.toolNames[name]; declared {
			return model.Result{Channel: model.ChannelTool, Content: trimmed}
		}
	}
	return model.Result{Channel: model.ChannelAnswer, Content: content}
}

func envelopeName(content string) (string, bool) {
	var function model.ResponseToolCallFunction
	if unmarshalFunction(content, &function) != nil {
		return "", false
	}
	return function.Name, true
}

func declaredToolName(tool model.D) string {
	if tool["type"] != "function" {
		return ""
	}
	function, ok := tool["function"].(model.D)
	if !ok {
		return ""
	}
	name, _ := function["name"].(string)
	return name
}

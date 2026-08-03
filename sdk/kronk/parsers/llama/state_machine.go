package llama

import (
	"encoding/json"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const pythonTag = "<|python_tag|>"

// stateMachine recognizes marked Llama calls and conservatively buffers bare
// JSON only when the request declares tools.
type stateMachine struct {
	status        model.Channel
	candidate     strings.Builder
	toolNames     map[string]struct{}
	answerStarted bool
}

// Reset returns the state machine to its initial state.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.candidate.Reset()
	sm.toolNames = nil
	sm.answerStarted = false
}

// SetTools supplies the request's declared tools for conservative bare-JSON
// classification.
func (sm *stateMachine) SetTools(tools []model.D) {
	sm.toolNames = make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		name := declaredToolName(tool)
		if name != "" {
			sm.toolNames[name] = struct{}{}
		}
	}
}

// Classify classifies one decoded token.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if sm.status == model.ChannelTool {
		return model.Result{Channel: model.ChannelTool, Content: content}, false
	}

	if sm.candidate.Len() > 0 && strings.TrimSpace(sm.candidate.String()) == "" {
		switch {
		case strings.HasPrefix(content, pythonTag) && len(sm.toolNames) > 0:
			sm.candidate.Reset()
			sm.status = model.ChannelTool
			return model.Result{Channel: model.ChannelTool, Content: pythonTagPayload(content)}, false

		case content == "<think>":
			sm.candidate.Reset()
			sm.status = model.ChannelReasoning
			return model.Result{}, false
		}
	}

	if strings.HasPrefix(content, pythonTag) && len(sm.toolNames) > 0 {
		sm.status = model.ChannelTool
		return model.Result{Channel: model.ChannelTool, Content: pythonTagPayload(content)}, false
	}

	if sm.candidate.Len() > 0 {
		sm.candidate.WriteString(content)
		return sm.classifyCandidate(false)
	}

	switch content {
	case "<think>":
		sm.status = model.ChannelReasoning
		return model.Result{}, false

	case "</think>":
		sm.status = model.ChannelAnswer
		return model.Result{}, false

	case pythonTag:
		if len(sm.toolNames) > 0 {
			sm.status = model.ChannelTool
			return model.Result{}, false
		}
	}

	if sm.status == model.ChannelAnswer && !sm.answerStarted && len(sm.toolNames) > 0 {
		trimmed := strings.TrimLeft(content, " \t\r\n")
		if trimmed == "" || strings.HasPrefix(trimmed, "{") {
			sm.candidate.WriteString(content)
			return sm.classifyCandidate(false)
		}
	}

	if sm.status == model.ChannelAnswer && strings.TrimSpace(content) != "" {
		sm.answerStarted = true
	}
	return model.Result{Channel: sm.status, Content: content}, false
}

func pythonTagPayload(content string) string {
	payload := strings.TrimPrefix(content, pythonTag)
	if payload == "" {
		return pythonTag
	}
	return payload
}

// Flush drains a possible bare-JSON call or returns it as answer content.
func (sm *stateMachine) Flush() model.Result {
	if sm.candidate.Len() == 0 {
		return model.Result{}
	}

	result, _ := sm.classifyCandidate(true)
	return result
}

func (sm *stateMachine) classifyCandidate(flush bool) (model.Result, bool) {
	content := sm.candidate.String()
	trimmed := strings.TrimSpace(content)
	if trimmed == "" && !flush {
		return model.Result{}, false
	}
	if !strings.HasPrefix(trimmed, "{") {
		sm.candidate.Reset()
		return model.Result{Channel: model.ChannelAnswer, Content: content}, false
	}

	end := findJSONObjectEnd(trimmed)
	if end == -1 && !flush {
		return model.Result{}, false
	}
	if end == -1 || strings.TrimSpace(trimmed[end:]) != "" {
		return sm.releaseCandidate(content)
	}

	name, ok := envelopeName(trimmed[:end])
	if !ok {
		return sm.releaseCandidate(content)
	}
	if _, ok := sm.toolNames[name]; !ok {
		return sm.releaseCandidate(content)
	}
	if !flush {
		return model.Result{}, false
	}

	sm.candidate.Reset()
	sm.status = model.ChannelTool
	return model.Result{Channel: model.ChannelTool, Content: trimmed[:end]}, false
}

func (sm *stateMachine) releaseCandidate(content string) (model.Result, bool) {
	sm.candidate.Reset()
	if strings.TrimSpace(content) != "" {
		sm.answerStarted = true
	}
	return model.Result{Channel: model.ChannelAnswer, Content: content}, false
}

func envelopeName(content string) (string, bool) {
	var envelope struct {
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		return "", false
	}
	if envelope.Name == "" || len(envelope.Parameters) == 0 || envelope.Parameters[0] != '{' {
		return "", false
	}

	return envelope.Name, true
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

func findJSONObjectEnd(content string) int {
	depth := 0
	inString := false
	escaped := false
	for i := range len(content) {
		char := content[i]
		if escaped {
			escaped = false
			continue
		}
		if inString && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch char {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}

	return -1
}

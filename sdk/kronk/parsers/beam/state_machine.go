package beam

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const (
	startMarker   = "<|start|>"
	messageMarker = "<|message|>"
	parameterOpen = "<rfl:parameter name=\""
	parameterEnd  = "\n</rfl:parameter>"
)

var frameMarkers = []string{
	startMarker, "<|end|>", "<|call|>", "<|return|>",
	"<|endoftext|>", "<|im_end|>", "<|fim_pad|>", "<|repo_name|>", "<|file_sep|>",
}

// stateMachine retains complete native tool frames until final validation.
// Ordinary reasoning and answer text can be emitted as soon as it arrives.
type stateMachine struct {
	input    string
	header   string
	tooling  string
	channel  model.Channel
	inHeader bool
	output   []model.Result
}

// Reset restores the assistant prefix left open by Beam's generation prompt.
func (sm *stateMachine) Reset() {
	*sm = stateMachine{header: "assistant", inHeader: true}
}

// Classify consumes a decoded piece, independent of tokenizer boundaries.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	sm.input += content
	eog := sm.consume(false)
	return sm.pop(), eog
}

// ConsumeVocabEOG retains native end markers for tool-frame validation.
func (sm *stateMachine) ConsumeVocabEOG(content string) {
	sm.input += content
	sm.consume(false)
}

func (sm *stateMachine) consume(flush bool) bool {
	for sm.input != "" {
		if sm.inHeader {
			// A full assistant prefix is also accepted, for replayed output.
			if strings.HasPrefix(sm.input, startMarker) {
				sm.header = ""
				sm.input = sm.input[len(startMarker):]
			}
			idx := strings.Index(sm.input, messageMarker)
			if idx < 0 {
				// As in llama.cpp, unchanneled output is answer text. Do not
				// reinterpret an incomplete recipient header as an answer.
				if !strings.Contains(sm.header+sm.input, " to=") {
					if at, marker := nextFrame(sm.input); flush || at >= 0 && marker != startMarker {
						sm.channel = model.ChannelAnswer
						sm.inHeader = false
						continue
					}
				}
				return false
			}
			sm.header += sm.input[:idx]
			sm.input = sm.input[idx+len(messageMarker):]
			sm.inHeader = false
			_, channel, err := parseHeader(sm.header)
			sm.channel = channel
			if err != nil && strings.Contains(sm.header, " to=") {
				sm.channel = model.ChannelTool
			}
			if sm.channel == model.ChannelTool {
				sm.tooling = startMarker + sm.header + messageMarker
			}
			continue
		}

		idx, marker := nextFrame(sm.input)
		if sm.channel == model.ChannelTool {
			idx, marker = toolBoundary(sm.input)
			if idx < 0 {
				if flush {
					sm.tooling += sm.input
					sm.input = ""
				}
				return false
			}
		} else if idx < 0 {
			keep := 0
			if !flush {
				for _, candidate := range frameMarkers {
					for n := 1; n < len(candidate) && n <= len(sm.input); n++ {
						if strings.HasSuffix(sm.input, candidate[:n]) {
							keep = max(keep, n)
						}
					}
				}
			}
			sm.emit(sm.input[:len(sm.input)-keep])
			sm.input = sm.input[len(sm.input)-keep:]
			return false
		}

		sm.emit(sm.input[:idx])
		sm.input = sm.input[idx:]
		if sm.channel == model.ChannelTool {
			if marker != startMarker {
				sm.tooling += marker
			}
			sm.output = append(sm.output, model.Result{Channel: model.ChannelTool, Content: sm.tooling})
			sm.tooling = ""
		}
		sm.input = sm.input[len(marker):]
		sm.channel = model.ChannelNone
		sm.header = ""
		sm.inHeader = true
		if marker != startMarker && marker != "<|end|>" {
			if strings.TrimSpace(sm.input) != "" {
				sm.output = append(sm.output, model.Result{Channel: model.ChannelTool, Content: sm.input})
			}
			sm.input = ""
			return true
		}
	}
	return false
}

func (sm *stateMachine) emit(text string) {
	if sm.channel == model.ChannelTool {
		sm.tooling += text
	} else if text != "" && sm.channel != model.ChannelNone {
		sm.output = append(sm.output, model.Result{Channel: sm.channel, Content: text})
	}
}

func (sm *stateMachine) pop() model.Result {
	if len(sm.output) == 0 {
		return model.Result{}
	}
	r := sm.output[0]
	sm.output = sm.output[1:]
	return r
}

// Flush drains queued channels and preserves truncated tool output as a failure.
func (sm *stateMachine) Flush() model.Result {
	if len(sm.output) > 0 {
		return sm.pop()
	}
	sm.consume(true)
	if sm.tooling != "" {
		sm.output = append(sm.output, model.Result{Channel: model.ChannelTool, Content: sm.tooling})
		sm.tooling = ""
	}
	if sm.inHeader && strings.Contains(sm.header+sm.input, " to=") {
		sm.output = append(sm.output, model.Result{Channel: model.ChannelTool, Content: startMarker + sm.header + sm.input})
		sm.header, sm.input = "", ""
	}
	return sm.pop()
}

func nextFrame(s string) (int, string) {
	idx, marker := -1, ""
	for _, candidate := range frameMarkers {
		if at := strings.Index(s, candidate); at >= 0 && (idx < 0 || at < idx) {
			idx, marker = at, candidate
		}
	}
	return idx, marker
}

// toolBoundary skips raw parameter values: inline Harmony text is data, not
// framing. Missing parameter closers are accepted only at newline boundaries,
// matching the native Harmony XML parser.
func toolBoundary(body string) (int, string) {
	remaining := strings.TrimLeft(body, " \t\r\n")
	for strings.HasPrefix(remaining, parameterOpen) {
		_, _, tail, err := splitParameter(remaining)
		if err != nil {
			return -1, ""
		}
		remaining = strings.TrimLeft(tail, " \t\r\n")
	}
	idx, marker := nextFrame(remaining)
	if idx < 0 {
		return -1, ""
	}
	return len(body) - len(remaining) + idx, marker
}

package kimi

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const (
	openMarker  = "<|open|>"
	closeMarker = "<|close|>"
	sepMarker   = "<|sep|>"
	endMarker   = "<|end_of_msg|>"
)

// stateMachine classifies Kimi K3 reasoning, response, and tools sections.
// The tokenizer represents the protocol delimiters as special tokens while
// tag names and attributes may be split across any number of decoded tokens.
type stateMachine struct {
	status model.Channel

	header       strings.Builder
	headerOpen   bool
	headerClose  bool
	inTools      bool
	pendingTools bool
}

// Reset returns the state machine to its initial state.
func (sm *stateMachine) Reset() {
	sm.status = model.ChannelAnswer
	sm.header.Reset()
	sm.headerOpen = false
	sm.headerClose = false
	sm.inTools = false
	sm.pendingTools = false
}

// Classify classifies one decoded token's content.
func (sm *stateMachine) Classify(content string) (model.Result, bool) {
	if content == endMarker {
		return model.Result{}, true
	}

	if sm.headerOpen || sm.headerClose {
		if content == sepMarker {
			return sm.finishHeader()
		}

		sm.header.WriteString(content)
		return model.Result{}, false
	}

	if kind, header, ok := completeHeader(content); ok {
		sm.header.WriteString(header)
		sm.headerOpen = kind == openMarker
		sm.headerClose = kind == closeMarker
		return sm.finishHeader()
	}

	switch content {
	case openMarker:
		sm.headerOpen = true
		sm.header.Reset()
		return model.Result{}, false

	case closeMarker:
		sm.headerClose = true
		sm.header.Reset()
		return model.Result{}, false
	}

	if sm.inTools {
		return model.Result{Channel: model.ChannelTool, Content: content}, false
	}

	if sm.pendingTools {
		return model.Result{}, false
	}

	return model.Result{Channel: sm.status, Content: content}, false
}

func (sm *stateMachine) finishHeader() (model.Result, bool) {
	header := sm.header.String()
	tag := header
	if idx := strings.IndexAny(tag, " \t\r\n"); idx != -1 {
		tag = tag[:idx]
	}

	isOpen := sm.headerOpen
	control := closeMarker + header + sepMarker
	if isOpen {
		control = openMarker + header + sepMarker
	}

	sm.header.Reset()
	sm.headerOpen = false
	sm.headerClose = false

	if sm.inTools {
		if !isOpen && tag == "tools" {
			sm.inTools = false
			sm.pendingTools = true
		}
		return model.Result{Channel: model.ChannelTool, Content: control}, false
	}

	if isOpen {
		switch tag {
		case "think":
			sm.status = model.ChannelReasoning
		case "response":
			sm.status = model.ChannelAnswer
		case "tools":
			sm.status = model.ChannelTool
			sm.inTools = true
			sm.pendingTools = false
			return model.Result{Channel: model.ChannelTool, Content: control}, false
		}
	} else if tag == "think" || tag == "response" {
		sm.status = model.ChannelNone
	}

	return model.Result{}, false
}

func completeHeader(content string) (kind string, header string, ok bool) {
	for _, marker := range []string{openMarker, closeMarker} {
		if !strings.HasPrefix(content, marker) || !strings.HasSuffix(content, sepMarker) {
			continue
		}

		return marker, content[len(marker) : len(content)-len(sepMarker)], true
	}

	return "", "", false
}

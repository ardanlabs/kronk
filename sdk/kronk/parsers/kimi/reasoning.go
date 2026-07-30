package kimi

import (
	"regexp"
	"strings"
)

var (
	closedThink = regexp.MustCompile(`(?s)<\|open\|>think<\|sep\|>.*?<\|close\|>think<\|sep\|>`)
	emptyThink  = regexp.MustCompile(`(?s)<\|open\|>think<\|sep\|>\s*<\|close\|>think<\|sep\|>`)
)

// StripReasoningContent removes complete Kimi thinking spans embedded in an
// assistant message's content. Text outside the spans is preserved.
func (Parser) StripReasoningContent(content string) string {
	if !strings.Contains(content, thinkOpen) {
		return content
	}
	return closedThink.ReplaceAllString(content, "")
}

// StripEmptyReasoning removes complete empty thinking spans. The Kimi
// generation marker is an unclosed think opener, so it cannot match here.
func (Parser) StripEmptyReasoning(rendered string) string {
	return emptyThink.ReplaceAllString(rendered, "")
}

package deepseek

import "github.com/ardanlabs/kronk/sdk/kronk/parsers/standard"

// StripReasoningContent removes <think>...</think> spans embedded in an
// assistant message's content. Text outside the spans is preserved.
func (Parser) StripReasoningContent(content string) string {
	return standard.StripThinkContent(content)
}

// StripEmptyReasoning removes empty <think>...</think> spans from a rendered
// prompt, leaving a trailing generation marker intact.
func (Parser) StripEmptyReasoning(rendered string) string {
	return standard.StripEmptyThink(rendered)
}

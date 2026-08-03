package llama

import "github.com/ardanlabs/kronk/sdk/kronk/parsers/fallback"

// StripReasoningContent removes <think>...</think> spans embedded in
// assistant content.
func (Parser) StripReasoningContent(content string) string {
	return fallback.StripThinkContent(content)
}

// StripEmptyReasoning removes empty <think>...</think> spans from a rendered
// prompt, leaving the trailing generation marker intact.
func (Parser) StripEmptyReasoning(rendered string) string {
	return fallback.StripEmptyThink(rendered)
}

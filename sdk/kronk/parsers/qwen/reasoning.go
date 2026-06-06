package qwen

import "github.com/ardanlabs/kronk/sdk/kronk/parsers/standard"

// Qwen emits reasoning between <think>...</think> tags. With thinking disabled
// the template emits an empty span (<think>\n\n</think>) on the generation
// prompt, and — position-dependently — on assistant turns that follow the last
// user message, which shifts the tokenized prefix across turns and forces IMC
// rebuilds. The <think> convention is shared, so the stripping logic lives in
// the standard package and Qwen delegates to it.

// StripReasoningContent removes <think>...</think> spans embedded in an
// assistant message's content. Text outside the spans is preserved.
func (Parser) StripReasoningContent(content string) string {
	return standard.StripThinkContent(content)
}

// StripEmptyReasoning removes empty <think>...</think> spans from a rendered
// prompt, leaving a trailing span (the generation-prompt marker) intact.
func (Parser) StripEmptyReasoning(rendered string) string {
	return standard.StripEmptyThink(rendered)
}

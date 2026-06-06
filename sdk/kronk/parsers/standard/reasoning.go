package standard

import (
	"regexp"
	"strings"
)

// The <think>...</think> convention is the most common reasoning wrap and is
// shared by several lineages (standard, qwen, glm). The regexes and helpers
// below own that convention so the family parsers can reuse them rather than
// each carrying a private copy.

var (
	// closedThink matches a complete <think>...</think> span, empty or not.
	closedThink = regexp.MustCompile(`(?s)<think>.*?</think>`)

	// emptyThink matches a <think>...</think> span whose body is only whitespace.
	emptyThink = regexp.MustCompile(`(?s)<think>\s*</think>`)
)

// StripThinkContent removes <think>...</think> spans embedded in an assistant
// message's content. Text outside the spans is preserved. Family parsers that
// share the <think> convention (qwen, glm) delegate here.
func StripThinkContent(content string) string {
	if !strings.Contains(content, "<think>") {
		return content
	}
	return closedThink.ReplaceAllString(content, "")
}

// StripEmptyThink removes empty <think>...</think> spans from a rendered
// prompt, leaving a trailing span (the generation-prompt marker) intact.
// Family parsers that share the <think> convention delegate here.
func StripEmptyThink(rendered string) string {
	return StripExceptTrailing(rendered, emptyThink)
}

// StripExceptTrailing removes every span matched by re, except one that sits at
// the very end of s (only whitespace follows it). The trailing span is the
// generation-prompt marker that primes the model to answer; removing it would
// change what the model generates. Exported for reuse by sibling parsers that
// need the same "strip all but trailing" semantics with a custom pattern.
func StripExceptTrailing(s string, re *regexp.Regexp) string {
	locs := re.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}

	var b strings.Builder
	prev := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if strings.TrimSpace(s[end:]) == "" {
			continue
		}
		b.WriteString(s[prev:start])
		prev = end
	}
	b.WriteString(s[prev:])

	return b.String()
}

// StripReasoningContent implements model.ReasoningNormalizer for the standard
// lineage. It removes <think>...</think> spans embedded in assistant content.
func (Parser) StripReasoningContent(content string) string {
	return StripThinkContent(content)
}

// StripEmptyReasoning implements model.ReasoningNormalizer for the standard
// lineage. It removes empty <think>...</think> spans from the rendered prompt,
// leaving the trailing generation marker intact.
func (Parser) StripEmptyReasoning(rendered string) string {
	return StripEmptyThink(rendered)
}

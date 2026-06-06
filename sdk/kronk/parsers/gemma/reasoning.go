package gemma

import (
	"regexp"
	"strings"
)

// Gemma emits reasoning on a dedicated "thought" channel:
//
//	<|channel>thought
//	...reasoning text...
//	<channel|>
//
// On the generation prompt with thinking disabled the template emits an empty
// thought marker (<|channel>thought\n<channel|>) to prime the model to answer
// without reasoning.

var (
	// closedThought matches a complete thought-channel span, empty or not.
	closedThought = regexp.MustCompile(`(?s)<\|channel>thought\b.*?<channel\|>`)

	// emptyThought matches a thought-channel span whose body is only whitespace.
	emptyThought = regexp.MustCompile(`(?s)<\|channel>thought\s*<channel\|>`)
)

// StripReasoningContent removes thought-channel spans embedded in an assistant
// message's content. Text outside the spans is preserved.
func (Parser) StripReasoningContent(content string) string {
	if !strings.Contains(content, "<|channel>thought") {
		return content
	}
	return closedThought.ReplaceAllString(content, "")
}

// StripEmptyReasoning removes empty thought-channel spans from a rendered
// prompt, leaving a trailing span (the generation-prompt marker) intact.
func (Parser) StripEmptyReasoning(rendered string) string {
	return stripEmptyExceptTrailing(rendered, emptyThought)
}

// stripEmptyExceptTrailing removes every empty reasoning span matched by re,
// except one that sits at the very end of s (only whitespace follows it). The
// trailing span is the generation-prompt marker that primes the model to
// answer; removing it would change what the model generates.
func stripEmptyExceptTrailing(s string, re *regexp.Regexp) string {
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

package glm

import (
	"regexp"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/parsers/standard"
)

// GLM's chat template manufactures thinking markers by message position rather
// than reading a stable field, so the same assistant turn renders differently
// depending on where the last user message sits:
//
//   - A turn in the current block (after the last user message, e.g. the
//     tool-calling chain) renders an empty span:  <|assistant|><think></think>…
//   - Once a newer user message arrives, that same turn becomes history and
//     renders a bare closing marker instead:       <|assistant|></think>…
//
// Either form shifts the tokenized prefix when a turn crosses the user-message
// boundary, forcing IMC rebuilds. To make every historical assistant turn
// render content-only and byte-stable, both forms are removed here while the
// trailing generation marker (the open <think> primed for the next answer, or a
// trailing </think> when thinking is disabled) is preserved.
//
// GLM shares the <think> convention, so content-level stripping delegates to
// the standard package; the bare </think> marker is GLM-specific.

// bareClose matches the bare </think> marker GLM emits directly after the
// <|assistant|> tag on prior turns. Group 1 (the assistant tag and any
// following whitespace) is retained; the </think> is dropped.
var bareClose = regexp.MustCompile(`(?s)(<\|assistant\|>\s*)</think>`)

// StripReasoningContent removes <think>...</think> spans embedded in an
// assistant message's content. Text outside the spans is preserved.
func (Parser) StripReasoningContent(content string) string {
	return standard.StripThinkContent(content)
}

// StripEmptyReasoning removes the empty <think></think> spans (current-block
// turns) and the bare </think> markers (prior turns) GLM emits, leaving the
// trailing generation marker intact.
func (Parser) StripEmptyReasoning(rendered string) string {
	rendered = standard.StripEmptyThink(rendered)
	return stripBareClose(rendered)
}

// stripBareClose removes each bare </think> marker following an <|assistant|>
// tag, retaining the assistant tag, except a marker at the very end of the
// string (only whitespace follows) which is the generation-prompt marker.
func stripBareClose(s string) string {
	locs := bareClose.FindAllStringSubmatchIndex(s, -1)
	if len(locs) == 0 {
		return s
	}

	var b strings.Builder
	prev := 0
	for _, loc := range locs {
		matchStart, matchEnd := loc[0], loc[1]
		if strings.TrimSpace(s[matchEnd:]) == "" {
			continue
		}
		b.WriteString(s[prev:matchStart])
		// Retain group 1 (the <|assistant|> tag and following whitespace).
		b.WriteString(s[loc[2]:loc[3]])
		prev = matchEnd
	}
	b.WriteString(s[prev:])

	return b.String()
}

// Package jsonrepair verifies and repairs malformed JSON produced by LLM
// tool calls. The most common issue is unescaped double quotes inside string
// values — e.g., when a model outputs source code containing import "fmt" or
// markdown with "quoted" text. Standard json.Unmarshal fails on these, but the
// key-value structure is otherwise intact.
//
// The package also handles Gemma4-style <|"|> quote tokens, bare (unquoted)
// JSON keys, and backtick-comma separators.
//
// Usage:
//
//	repaired, err := jsonrepair.Repair(raw)
package jsonrepair

import (
	"encoding/json"
	"errors"
	"strings"
)

// ErrIrrecoverable is returned when the JSON cannot be repaired.
var ErrIrrecoverable = errors.New("jsonrepair: irrecoverable JSON")

// gemmaToken is the special quote delimiter used by Gemma4 models.
const gemmaToken = "<|\"|>"

// Repair verifies JSON and returns a valid version. If the input already
// parses, it is returned unchanged. If the input is malformed but
// repairable, the repaired JSON is returned. If the input cannot be
// repaired, the original string and ErrIrrecoverable are returned.
//
// The repair pipeline:
//  1. Quick-check: try json.Unmarshal; if valid, return immediately.
//  2. Normalize Gemma4 <|"|> tokens to standard quotes (escaping inner ").
//  3. Normalize backtick delimiters in structural positions to standard quotes.
//  4. Quote bare JSON keys.
//  5. Re-check: if valid after normalization, return.
//  6. Key-aware repair: walk the JSON structure, find true closing quotes
//     by scanning right-to-left for " followed by } / ] / ,"key": patterns.
//  7. Verify the result with json.Unmarshal.
//  8. If verification fails, try trimming a trailing extra } (Gemma double-brace).
func Repair(s string) (string, error) {
	if len(s) == 0 {
		return s, ErrIrrecoverable
	}

	// Step 1: if it already parses, no repair needed.
	if json.Valid([]byte(s)) {
		return s, nil
	}

	// Step 2-4: normalize quoting.
	normalized, changed := normalize(s)

	// Step 5: check again after normalization (skip if nothing changed).
	if changed && json.Valid([]byte(normalized)) {
		return normalized, nil
	}

	// Step 6: key-aware repair of unescaped quotes in values.
	repaired := repairQuotes(normalized)

	// Step 7: verify. If the result has a trailing extra } from Gemma's
	// double-brace wrapping (call:write{{...}}), try trimming it.
	if json.Valid([]byte(repaired)) {
		return repaired, nil
	}

	repaired = trimTrailingBrace(repaired)
	if json.Valid([]byte(repaired)) {
		return repaired, nil
	}

	return s, ErrIrrecoverable
}

// Unmarshal repairs the JSON string and unmarshals the result into v.
// It combines Repair and json.Unmarshal into a single call.
func Unmarshal(s string, v any) error {
	repaired, err := Repair(s)
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(repaired), v)
}

// =============================================================================
// Normalization
// =============================================================================

// normalize applies all pre-repair transformations: Gemma <|"|> replacement,
// backtick delimiter normalization, and bare key quoting. The second return
// value reports whether any transformation changed the string.
func normalize(s string) (string, bool) {
	orig := s
	s = normalizeGemmaQuotes(s)
	s = normalizeBacktickDelimiters(s)
	s = quoteBareKeys(s)
	return s, s != orig
}

// normalizeGemmaQuotes converts <|"|> delimited values to standard JSON
// quoted strings. The model uses <|"|> when the value contains literal "
// characters (e.g., source code with import "fmt"). This function finds
// each <|"|>...<|"|> pair, escapes any " inside the value to \", then
// replaces the <|"|> delimiters with standard ".
func normalizeGemmaQuotes(s string) string {
	if !strings.Contains(s, gemmaToken) {
		return s
	}

	tokenLen := len(gemmaToken)

	var b strings.Builder
	b.Grow(len(s))
	i := 0

	for i < len(s) {
		openIdx := strings.Index(s[i:], gemmaToken)
		if openIdx == -1 {
			b.WriteString(s[i:])
			break
		}

		// Write everything before the opening <|"|>.
		// Fix missing key close-quote: the model sometimes writes
		// "content:<|"|> instead of "content":<|"|> because the closing "
		// gets swallowed by the adjacent <|"|> token boundary.
		prefix := s[i : i+openIdx]
		prefix = fixMissingKeyQuote(prefix)
		b.WriteString(prefix)
		i += openIdx + tokenLen

		// Find the closing <|"|>.
		closeIdx := strings.Index(s[i:], gemmaToken)
		if closeIdx == -1 {
			// No closing token — this <|"|> acts as the end of the
			// preceding string value. Write " to close the value, then
			// handle the remainder.
			//
			// The model sometimes wraps the remaining keys in an extra
			// object: ,{"filePath":"path"}} instead of ,"filePath":"path"}.
			// Unwrap the nested object so all keys end up at the top level.
			b.WriteByte('"')
			rest := s[i:]
			if unwrapped := unwrapNestedObject(rest); unwrapped != rest {
				b.WriteString(unwrapped)
			} else {
				b.WriteString(rest)
			}
			break
		}

		// Extract the value between the <|"|> pair and convert to a valid
		// JSON string: escape inner quotes, convert raw control characters
		// to JSON escapes, and double-escape source code sequences like \n
		// so they survive JSON parsing as literal backslash-n.
		inner := s[i : i+closeIdx]
		b.WriteByte('"')
		for j := 0; j < len(inner); j++ {
			switch {
			case inner[j] == '"':
				b.WriteString(`\"`)
			case inner[j] == '\\':
				if j+1 < len(inner) {
					next := inner[j+1]
					j++

					switch next {
					case 'n', 'r', 't', 'b', 'f':
						// Source code escape (e.g., \n in fmt.Print("\n")).
						// Double-escape so JSON preserves the literal chars.
						b.WriteString(`\\`)
						b.WriteByte(next)
					case '"', '\\', '/', 'u':
						// Valid JSON escape — preserve as-is.
						b.WriteByte('\\')
						b.WriteByte(next)
					default:
						// Not a valid JSON escape (e.g., \0 from \033
						// ANSI codes). Double-escape the backslash.
						b.WriteString(`\\`)
						b.WriteByte(next)
					}
				} else {
					b.WriteByte('\\')
				}
			case inner[j] == '\n':
				b.WriteString(`\n`)
			case inner[j] == '\r':
				b.WriteString(`\r`)
			case inner[j] == '\t':
				b.WriteString(`\t`)
			default:
				b.WriteByte(inner[j])
			}
		}
		b.WriteByte('"')
		i += closeIdx + tokenLen

		// After the closing <|"|>, the model may write the next key with
		// a missing opening quote: ,filePath":  instead of ,"filePath":.
		// The <|"|> token boundary swallows the opening " of the key.
		// Splice the fix into s so the remainder is parsed correctly.
		if i < len(s) {
			tail := s[i:]
			if fixed := fixMissingKeyOpenQuote(tail); fixed != tail {
				s = s[:i] + fixed
			}
		}
	}

	return b.String()
}

// fixMissingKeyOpenQuote fixes the pattern ,key": that appears after a
// closing <|"|>. The model sometimes writes ,filePath": instead of
// ,"filePath": because the opening " of the key name gets swallowed by
// the adjacent <|"|> token boundary. This function inserts the missing "
// before the key identifier.
func fixMissingKeyOpenQuote(suffix string) string {
	// Must start with comma (optionally whitespace) then identifier then ":
	j := 0
	if j >= len(suffix) || suffix[j] != ',' {
		return suffix
	}
	j++

	// Skip whitespace after comma.
	for j < len(suffix) && isWhitespace(suffix[j]) {
		j++
	}

	// Must NOT start with " (already quoted).
	if j >= len(suffix) || suffix[j] == '"' {
		return suffix
	}

	// Scan identifier.
	keyStart := j
	for j < len(suffix) && isIdentChar(suffix[j]) {
		j++
	}
	keyLen := j - keyStart
	if keyLen == 0 || keyLen > 40 {
		return suffix
	}

	// Must be followed by ":
	if j+1 < len(suffix) && suffix[j] == '"' && suffix[j+1] == ':' {
		// Pattern matched: ,identifier": → ,"identifier":
		return suffix[:keyStart] + `"` + suffix[keyStart:]
	}

	return suffix
}

// fixMissingKeyQuote fixes the pattern "key: that appears before <|"|>.
// The model sometimes writes "content:<|"|> instead of "content":<|"|>
// because the closing " of the key name gets swallowed by the adjacent
// <|"|> token boundary. This function inserts the missing " before :.
func fixMissingKeyQuote(prefix string) string {
	n := len(prefix)
	if n < 3 || prefix[n-1] != ':' {
		return prefix
	}

	// Scan backwards past the identifier before :.
	k := n - 2
	for k >= 0 && isIdentChar(prefix[k]) {
		k--
	}

	keyLen := (n - 2) - k
	if keyLen == 0 || k < 0 || prefix[k] != '"' {
		return prefix
	}

	// Pattern matched: "identifier: → "identifier":
	return prefix[:n-1] + `":`
}

// normalizeBacktickDelimiters replaces backticks that appear in JSON structural
// positions with standard double quotes. Models sometimes use backticks as
// string delimiters — e.g., opening with <|"|> but closing with `, or using
// backtick pairs around values. A backtick is considered structural when:
//   - It's followed (skipping whitespace) by , + key pattern, or } ] or end-of-string.
//   - It's preceded (skipping whitespace) by : (value opener).
func normalizeBacktickDelimiters(s string) string {
	if !strings.Contains(s, "`") {
		return s
	}

	var buf strings.Builder
	buf.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] != '`' {
			buf.WriteByte(s[i])
			continue
		}

		structural := false

		// Look ahead: backtick closing a value.
		j := i + 1
		for j < len(s) && isWhitespace(s[j]) {
			j++
		}

		if j >= len(s) || s[j] == '}' || s[j] == ']' {
			structural = true
		} else if s[j] == ',' {
			// Comma followed by a key pattern: this backtick closes a value.
			if isFollowedByKey(s, j+1) {
				structural = true
			}

			// Comma followed by another backtick: paired delimiter pattern
			// (e.g., "hello`,`filePath" where backticks replace quotes).
			if !structural {
				k := j + 1
				for k < len(s) && isWhitespace(s[k]) {
					k++
				}
				if k < len(s) && s[k] == '`' {
					structural = true
				}
			}
		}

		// Look behind: backtick opening a value after colon, or opening a
		// key after comma / { / [.
		if !structural {
			k := i - 1
			for k >= 0 && isWhitespace(s[k]) {
				k--
			}
			if k >= 0 && (s[k] == ':' || s[k] == ',' || s[k] == '{' || s[k] == '[') {
				structural = true
			}
		}

		if structural {
			buf.WriteByte('"')
		} else {
			buf.WriteByte('`')
		}
	}

	return buf.String()
}

// quoteBareKeys adds double quotes around unquoted JSON keys.
// Models often emit keys without quotes: {content:"text",priority:"high"}
// which is not valid JSON. This function converts them to proper JSON:
// {"content":"text","priority":"high"}.
func quoteBareKeys(s string) string {
	var buf strings.Builder
	changed := false

	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			if changed {
				buf.WriteByte(c)
			}
			escaped = false
			continue
		}

		if c == '\\' && inString {
			if changed {
				buf.WriteByte(c)
			}
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			if changed {
				buf.WriteByte(c)
			}
			continue
		}

		if inString {
			if changed {
				buf.WriteByte(c)
			}
			continue
		}

		// Outside a string: check if this is the start of a bare key.
		// A bare key follows { , [ or is at the start, and is a word
		// followed by a colon.
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' {
			// Look ahead to find the colon.
			j := i + 1
			for j < len(s) && (s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9' || s[j] == '_') {
				j++
			}

			if j < len(s) && s[j] == ':' {
				// This is a bare key — start the builder on first mutation.
				if !changed {
					buf.Grow(len(s) + 32)
					buf.WriteString(s[:i])
					changed = true
				}
				buf.WriteByte('"')
				buf.WriteString(s[i:j])
				buf.WriteByte('"')
				i = j - 1
				continue
			}
		}

		if changed {
			buf.WriteByte(c)
		}
	}

	if !changed {
		return s
	}

	return buf.String()
}

// =============================================================================
// Key-aware quote repair
// =============================================================================

// repairQuotes walks the JSON with key-value structure awareness. After a
// colon introduces a string value, the real closing " is found by
// findKeyAwareClosingQuote. All other " between open and close are escaped.
func repairQuotes(s string) string {
	var buf strings.Builder
	buf.Grow(len(s) + 64)

	i := 0
	for i < len(s) {
		c := s[i]

		switch c {
		case '{', '}', '[', ']', ',':
			buf.WriteByte(c)
			i++

		case ':':
			buf.WriteByte(c)
			i++

			// Skip whitespace after colon.
			for i < len(s) && isWhitespace(s[i]) {
				buf.WriteByte(s[i])
				i++
			}

			if i >= len(s) {
				break
			}

			// Value starts here. If it's a string, repair inner quotes.
			if s[i] == '"' {
				i++ // skip opening quote

				// Find the closing quote using key-aware analysis.
				closePos := findKeyAwareClosingQuote(s, i)

				if closePos == -1 {
					// No valid closing quote found. Write what we have.
					buf.WriteByte('"')
					buf.WriteString(s[i:])
					i = len(s)
					break
				}

				// Write the opening quote, escape all inner " chars,
				// then write the closing quote.
				inner := s[i:closePos]

				// Determine whether the model did any JSON escaping at
				// all. If the inner content has unescaped " characters,
				// the model output raw source code — so \n, \t, etc.
				// are source-code literals that need double-escaping.
				// If there are no unescaped quotes, the model did
				// proper JSON escaping (only raw control chars like
				// TAB slipped through), so \n, \t are valid JSON
				// escapes that must be preserved as-is.
				hasUnescapedQuotes := containsUnescapedQuote(inner)

				buf.WriteByte('"')
				for j := 0; j < len(inner); j++ {
					switch {
					case inner[j] == '\\' && j+1 < len(inner):
						next := inner[j+1]
						j++

						switch next {
						case '"', '\\', '/', 'u':
							// Valid JSON escape — always preserve.
							buf.WriteByte('\\')
							buf.WriteByte(next)
						case 'n', 'r', 't', 'b', 'f':
							if hasUnescapedQuotes {
								// Model didn't JSON-escape at all —
								// these are source-code literals.
								buf.WriteString(`\\`)
								buf.WriteByte(next)
							} else {
								// Model used proper JSON escaping —
								// these are valid JSON escapes.
								buf.WriteByte('\\')
								buf.WriteByte(next)
							}
						default:
							// Invalid JSON escape (e.g., \0 from \033 ANSI
							// codes). Double-escape so the literal backslash
							// survives JSON parsing.
							buf.WriteString(`\\`)
							buf.WriteByte(next)
						}
					case inner[j] == '"':
						buf.WriteString(`\"`)
					case inner[j] == '\n':
						buf.WriteString(`\n`)
					case inner[j] == '\r':
						buf.WriteString(`\r`)
					case inner[j] == '\t':
						buf.WriteString(`\t`)
					default:
						buf.WriteByte(inner[j])
					}
				}
				buf.WriteByte('"')
				i = closePos + 1
			}

			// Non-string values (numbers, booleans, null, objects, arrays)
			// are written as-is by the outer loop.

		default:
			buf.WriteByte(c)
			i++
		}
	}

	return buf.String()
}

// findKeyAwareClosingQuote finds the real closing quote of a JSON string value
// starting at position start in s (start is the index right after the opening
// quote). It scans right-to-left for a " followed by either end-of-object
// (} / ]), or the start of a new key-value pair (,"key": or ,key:). Keys are
// always short identifiers — never code fragments — so this distinguishes
// structural quotes from content quotes even when content contains patterns
// like "1", "2" that fool simpler heuristics.
//
// Returns the index of the closing quote in s, or -1 if not found.
func findKeyAwareClosingQuote(s string, start int) int {
	// Scan right-to-left so the longest (most inclusive) value wins.
	best := -1

	for k := len(s) - 1; k >= start; k-- {
		if s[k] != '"' {
			continue
		}

		// Skip escaped quotes.
		if k > 0 && s[k-1] == '\\' {
			continue
		}

		// Check what follows this quote.
		j := k + 1

		// Skip whitespace.
		for j < len(s) && isWhitespace(s[j]) {
			j++
		}

		if j >= len(s) {
			// Quote at end of string — structural.
			if best == -1 || k < best {
				best = k
			}
			continue
		}

		switch s[j] {
		case '}', ']':
			// Only treat as structural if this is the outermost closing
			// brace/bracket — i.e., nothing but whitespace and possibly
			// extra } from Gemma's double-brace wrapping follows it.
			// Content like [9]string{"3"} has } inside the value, but
			// more content follows, so it's not the JSON object end.
			after := j + 1
			for after < len(s) && (isWhitespace(s[after]) || s[after] == '}') {
				after++
			}
			if after >= len(s) {
				if best == -1 || k < best {
					best = k
				}
			}
			continue

		case ',':
			// Comma — check if followed by a key (short identifier + colon).
			if isFollowedByKey(s, j+1) {
				if best == -1 || k < best {
					best = k
				}
			}
			continue
		}
	}

	return best
}

// isFollowedByKey checks whether position j in s starts with an identifier
// key pattern: optional whitespace, then either "identifier": or identifier:
// where identifier is a short (≤40 char) word made of [a-zA-Z0-9_].
func isFollowedByKey(s string, j int) bool {
	// Skip whitespace.
	for j < len(s) && isWhitespace(s[j]) {
		j++
	}

	if j >= len(s) {
		return false
	}

	// Quoted key: "identifier":
	if s[j] == '"' {
		j++
		keyStart := j
		for j < len(s) && isIdentChar(s[j]) {
			j++
		}
		keyLen := j - keyStart
		if keyLen == 0 || keyLen > 40 {
			return false
		}
		if j < len(s) && s[j] == '"' {
			j++
			// Skip whitespace after closing quote.
			for j < len(s) && isWhitespace(s[j]) {
				j++
			}
			return j < len(s) && s[j] == ':'
		}
		return false
	}

	// Bare key: identifier:
	keyStart := j
	for j < len(s) && isIdentChar(s[j]) {
		j++
	}
	keyLen := j - keyStart
	if keyLen == 0 || keyLen > 40 {
		return false
	}

	// Skip whitespace before colon.
	for j < len(s) && isWhitespace(s[j]) {
		j++
	}

	return j < len(s) && s[j] == ':'
}

// =============================================================================
// Structural fixups
// =============================================================================

// unwrapNestedObject handles the pattern ,{"key":"value"}} that appears after
// an unpaired <|"|> token. The model wraps the remaining keys in an extra
// object instead of listing them as flat siblings. This converts:
//
//	,{"filePath":"path"}}  →  ,"filePath":"path"}
//
// Returns the original string unchanged if the pattern is not detected.
func unwrapNestedObject(s string) string {
	// Must start with ,{ (optional whitespace).
	j := 0
	for j < len(s) && isWhitespace(s[j]) {
		j++
	}
	if j >= len(s) || s[j] != ',' {
		return s
	}
	j++
	for j < len(s) && isWhitespace(s[j]) {
		j++
	}
	if j >= len(s) || s[j] != '{' {
		return s
	}

	// Must end with }} (optional whitespace).
	trimmed := strings.TrimRight(s, " \t\n\r")
	if len(trimmed) < 2 || trimmed[len(trimmed)-1] != '}' || trimmed[len(trimmed)-2] != '}' {
		return s
	}

	// Remove the nested { and one trailing }.
	return s[:j] + s[j+1:len(trimmed)-1]
}

// trimTrailingBrace removes one trailing } when the JSON has unbalanced braces.
// This fixes Gemma's double-brace wrapping: call:write{{...}} leaks the outer
// closing } into the extracted JSON, producing {"key":"val"}}.
func trimTrailingBrace(s string) string {
	trimmed := strings.TrimRight(s, " \t\n\r")
	if len(trimmed) < 2 || trimmed[len(trimmed)-1] != '}' || trimmed[len(trimmed)-2] != '}' {
		return s
	}

	return trimmed[:len(trimmed)-1]
}

// =============================================================================
// Character classification helpers
// =============================================================================

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// containsUnescapedQuote returns true if s contains at least one " that is not
// preceded by a backslash. This indicates the model produced raw source code
// without JSON escaping its string values.
func containsUnescapedQuote(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '"' && (i == 0 || s[i-1] != '\\') {
			return true
		}
	}
	return false
}

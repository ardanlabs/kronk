package model

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/jsonrepair"
)

// parseGemmaToolCall parses Gemma4-style tool calls.
// Format: call:get_weather{location:<|"|>New York City, NY<|"|>}
// Multiple calls may appear separated by newlines or back-to-back.
func (p *processor) parseGemmaToolCall(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

	remaining := content
	for {
		callIdx := strings.Index(remaining, "call:")
		if callIdx == -1 {
			break
		}

		remaining = remaining[callIdx+5:]

		// Find the opening brace for the arguments.
		braceIdx := strings.Index(remaining, "{")
		if braceIdx == -1 {
			break
		}

		name := strings.TrimSpace(remaining[:braceIdx])
		remaining = remaining[braceIdx:]

		// Find the matching closing brace. When the model uses mixed
		// quoting (e.g., opens with <|"|> but closes with `) the brace
		// matcher can fail. In that case, take everything remaining as
		// the raw args so the tool call still reaches the client.
		braceEnd := findGemmaBraceEnd(remaining)

		var argsRaw string
		if braceEnd == -1 {
			argsRaw = remaining[1:]
			remaining = ""
		} else {
			argsRaw = remaining[1:braceEnd] // content between { and }
			remaining = remaining[braceEnd+1:]
		}

		// Gemma4 outputs double braces: call:func{{"key":"val"}}.
		// After stripping the outer pair, argsRaw still has {…}.
		//
		// The model may mix <|"|> tokens with standard JSON quotes
		// (e.g., opening a value with <|"|> but closing with ").
		// jsonrepair.Repair handles all normalization and repair.
		var args map[string]any
		trimmed := strings.TrimSpace(argsRaw)

		// Wrap in braces if the content doesn't already start with {.
		// This handles cases like: "content:<|"|>text<|"|>,"filePath":"x"
		// which becomes {"content":"text","filePath":"x"} after wrapping
		// and <|"|> replacement.
		jsonCandidate := trimmed
		if len(jsonCandidate) > 0 && jsonCandidate[0] != '{' {
			jsonCandidate = "{" + jsonCandidate + "}"
		}

		if err := jsonrepair.Unmarshal(jsonCandidate, &args); err != nil {
			p.model.log(context.Background(), "jsonrepair", "status", "unmarshal-failed",
				"format", "gemma", "error", err, "json", jsonCandidate)

			// Fall back to Gemma-specific key:value parsing.
			inner := trimmed
			if len(inner) > 0 && inner[0] == '{' {
				inner = inner[1:]
				if idx := strings.LastIndex(inner, "}"); idx >= 0 {
					inner = inner[:idx]
				}
			}
			args = parseGemmaArgs(inner)
		}

		toolCalls = append(toolCalls, ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})
	}

	return toolCalls
}

// =============================================================================

// findGemmaBraceEnd finds the closing brace that matches the opening brace at
// position 0, accounting for nested braces. Returns the index of the closing
// brace, or -1 if not found. Braces inside quoted strings are ignored so that
// code snippets like `board[move-1] != 0 {` don't break the match.
//
// Two quoting modes are supported:
//   - Gemma4 <|"|> tokens: paired as open/close delimiters; everything
//     between them (including standard " and braces) is skipped.
//   - Standard JSON " quotes: used only when no <|"|> tokens are present
//     in the input, since <|"|> contains a literal " that would confuse
//     JSON-style string scanning.
func findGemmaBraceEnd(s string) int {
	if len(s) == 0 || s[0] != '{' {
		return -1
	}

	// When <|"|> tokens are present, the model uses Gemma-style quoting.
	// Standard " characters inside <|"|>-delimited values (e.g., Go import
	// paths like "fmt") must NOT be treated as JSON string boundaries.
	useJSONQuotes := !strings.Contains(s, "<|\"|>")

	depth := 0
	i := 0
	for i < len(s) {
		// Pair <|"|> tokens — skip everything between open and close.
		if strings.HasPrefix(s[i:], "<|\"|>") {
			i += len("<|\"|>")
			for i < len(s) {
				if strings.HasPrefix(s[i:], "<|\"|>") {
					i += len("<|\"|>")
					break
				}
				i++
			}
			continue
		}

		// Skip standard JSON quoted strings only when the model is using
		// pure JSON format (no <|"|> tokens anywhere in the tool call).
		if useJSONQuotes && s[i] == '"' {
			i++
			for i < len(s) {
				if s[i] == '\\' {
					i += 2 // skip escaped character
					continue
				}
				if s[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}

		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}

	return -1
}

// findClosingGemmaQuote finds the position of the closing <|"|> token that
// ends a value. For nested structures (arrays/objects containing their own
// <|"|> tokens), the correct closing token is the one followed by a
// structural character (comma, closing brace/bracket, double quote for
// JSON transition) or end of string, not an inner one.
func findClosingGemmaQuote(s string) int {
	const token = "<|\"|>"
	searchFrom := 0

	for {
		idx := strings.Index(s[searchFrom:], token)
		if idx == -1 {
			return -1
		}

		pos := searchFrom + idx
		afterQuote := pos + len(token)

		if afterQuote >= len(s) {
			return pos
		}

		// Closing <|"|> if followed by a structural character.
		// The model may transition from Gemma format to standard JSON
		// mid-output (e.g., <|"|>","filePath":"post4.md"), so accept
		// double-quote as a valid transition character.
		switch s[afterQuote] {
		case ',', '}', ']', '"':
			return pos
		}

		searchFrom = afterQuote
	}
}

// findGemmaStructEnd finds the end of a JSON array or object in Gemma4 format,
// accounting for nesting and <|"|> tokens. Returns the index after the closing
// bracket/brace, or -1 if not found.
func findGemmaStructEnd(s string) int {
	if len(s) == 0 {
		return -1
	}

	open := s[0]
	var close byte
	switch open {
	case '[':
		close = ']'
	case '{':
		close = '}'
	default:
		return -1
	}

	depth := 0
	i := 0
	for i < len(s) {
		// Skip <|"|> tokens.
		if strings.HasPrefix(s[i:], "<|\"|>") {
			i += len("<|\"|>")
			continue
		}

		switch s[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}

	return -1
}

// findClosingStandardQuote finds the closing " that ends a JSON-like value.
// When model output contains unescaped quotes inside values (e.g., markdown
// with "silent" failures), a naive strings.Index finds the wrong quote.
// The correct closing quote is the one followed by a structural character
// (comma, closing brace) or end of string — not one embedded in content.
func findClosingStandardQuote(s string) int {
	searchFrom := 0

	for {
		idx := strings.Index(s[searchFrom:], "\"")
		if idx == -1 {
			return -1
		}

		pos := searchFrom + idx

		// Skip escaped quotes.
		if pos > 0 && s[pos-1] == '\\' {
			searchFrom = pos + 1
			continue
		}

		afterQuote := pos + 1

		// Closing quote if followed by end of string, comma, or closing brace.
		if afterQuote >= len(s) {
			return pos
		}

		next := s[afterQuote]
		if next == ',' || next == '}' || next == ' ' || next == '\n' || next == '\r' || next == '\t' {
			return pos
		}

		searchFrom = afterQuote
	}
}

// parseGemmaArgs parses the key-value pairs inside a Gemma4 tool call argument
// block. Values are delimited by <|"|> tokens (acting as quotes).
// Format: key1:<|"|>value1<|"|>, key2:<|"|>value2<|"|>
func parseGemmaArgs(raw string) map[string]any {
	args := make(map[string]any)

	remaining := raw
	for len(remaining) > 0 {
		// Find the colon that separates key from value.
		colonIdx := strings.Index(remaining, ":")
		if colonIdx == -1 {
			break
		}

		key := strings.TrimLeft(remaining[:colonIdx], ", \t\n")
		key = strings.Trim(key, "\"")
		remaining = remaining[colonIdx+1:]

		// Check if the value is wrapped in <|"|> tokens.
		if strings.HasPrefix(remaining, "<|\"|>") {
			remaining = remaining[len("<|\"|>"):]

			endQuote := findClosingGemmaQuote(remaining)
			if endQuote == -1 {
				// No closing quote, take the rest.
				args[key] = strings.TrimSpace(remaining)
				break
			}

			value := remaining[:endQuote]

			// If the value looks like a JSON array or object with inner
			// <|"|> tokens, replace them with quotes and try to unmarshal
			// into a proper Go type so arrays/objects aren't flattened to
			// strings.
			trimVal := strings.TrimSpace(value)
			if len(trimVal) > 0 && (trimVal[0] == '[' || trimVal[0] == '{') {
				jsonVal := strings.ReplaceAll(trimVal, "<|\"|>", "\"")
				var parsed any
				if err := json.Unmarshal([]byte(jsonVal), &parsed); err == nil {
					args[key] = parsed
					remaining = remaining[endQuote+len("<|\"|>"):]
					continue
				}
			}

			args[key] = value
			remaining = remaining[endQuote+len("<|\"|>"):]
			continue
		}

		// Check if the value is wrapped in standard JSON double quotes.
		if strings.HasPrefix(remaining, "\"") {
			remaining = remaining[1:]

			endQuote := findClosingStandardQuote(remaining)
			if endQuote == -1 {
				args[key] = strings.TrimSpace(remaining)
				break
			}

			args[key] = remaining[:endQuote]
			remaining = remaining[endQuote+1:]
			continue
		}

		// Value is a JSON array or object — find the matching bracket/brace
		// accounting for nesting and <|"|> tokens so we don't match a
		// structural character inside the value.
		if len(remaining) > 0 && (remaining[0] == '[' || remaining[0] == '{') {
			endIdx := findGemmaStructEnd(remaining)
			if endIdx == -1 {
				args[key] = strings.TrimSpace(remaining)
				break
			}

			raw := remaining[:endIdx]
			jsonVal := strings.ReplaceAll(raw, "<|\"|>", "\"")

			var parsed any
			if err := json.Unmarshal([]byte(jsonVal), &parsed); err == nil {
				args[key] = parsed
			} else {
				args[key] = raw
			}

			remaining = remaining[endIdx:]
			continue
		}

		// Value without quote delimiters - take until next comma or end.
		endIdx := strings.IndexAny(remaining, ",}")
		var rawVal string
		if endIdx == -1 {
			rawVal = strings.TrimSpace(remaining)
		} else {
			rawVal = strings.TrimSpace(remaining[:endIdx])
		}

		// Preserve boolean and numeric types instead of storing as strings.
		args[key] = parseGemmaBareValue(rawVal)

		if endIdx == -1 {
			break
		}
		remaining = remaining[endIdx:]
	}

	return args
}

// parseGemmaBareValue converts a bare (unquoted) value string to the
// appropriate Go type. Booleans and null are converted to their native types;
// numeric strings are converted to float64 (matching json.Unmarshal behavior).
// Everything else is returned as a string.
func parseGemmaBareValue(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}

	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n
	}

	return s
}

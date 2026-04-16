package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	statusNone       = 0
	statusReasoning  = 1
	statusCompletion = 2
	statusTooling    = 3
)

type response struct {
	status  int
	content string
}

type processor struct {
	model           *Model
	status          int
	collecting      bool
	awaitingChannel bool

	// For accumulating tool call content across tokens (batch engine use).
	toolCallBuf  strings.Builder
	inToolCall   bool
	toolCallDone bool // Set after </tool_call> or <tool_call|>; next non-tool-call token triggers EOG.

	// For GPT models: accumulate channel name tokens and handle <|constrain|>.
	channelBuf        strings.Builder
	awaitingConstrain bool
	toolFuncName      string // Function name extracted from "to=NAME" in channel

	// For detecting split tags like "<function=" across multiple tokens.
	// Some models (Qwen3-Coder variants) emit <function=...> directly without
	// the <tool_call> wrapper, and the tag may be tokenized as "<", "function", "=".
	pendingTagBuf strings.Builder
	inPendingTag  bool
}

func newProcessor(m *Model) *processor {
	return &processor{
		model:  m,
		status: statusCompletion,
	}
}

func newToolCallID() string {
	return "call_" + uuid.NewString()
}

// =============================================================================

func parseGPTToolCall(content string) []ResponseToolCall {
	// Format: .FUNC_NAME <|message|>JSON_ARGS
	// The JSON may span multiple lines, so we can't split by newlines.
	// Instead, find each ".NAME <|message|>" prefix and extract the JSON that follows.

	var jsonCalls []string
	remaining := content

	for {
		// Find the start of a tool call (leading dot).
		dotIdx := strings.Index(remaining, ".")
		if dotIdx == -1 {
			break
		}

		remaining = remaining[dotIdx:]

		// Find <|message|> marker.
		msgIdx := strings.Index(remaining, "<|message|>")
		if msgIdx == -1 {
			break
		}

		// Extract function name (between dot and space before <|message|>).
		prefix := remaining[:msgIdx]
		parts := strings.SplitN(prefix, " ", 2)
		name := strings.TrimPrefix(parts[0], ".")

		// Move past <|message|> to get the JSON.
		jsonStart := msgIdx + 11
		remaining = remaining[jsonStart:]

		// Find the end of the JSON object by matching braces.
		jsonEnd := findJSONObjectEnd(remaining)
		if jsonEnd == -1 {
			// No valid JSON found, take the rest.
			jsonEnd = len(remaining)
		}

		args := remaining[:jsonEnd]
		remaining = remaining[jsonEnd:]

		// Build JSON: {"name":"get_weather","arguments":{"location":"NYC"}}
		jsonCall := `{"name":"` + name + `","arguments":` + args + `}`
		jsonCalls = append(jsonCalls, jsonCall)
	}

	return parseToolCall(strings.Join(jsonCalls, "\n"))
}

// findJSONObjectEnd finds the end of a JSON object starting at the beginning of s.
// Returns the index after the closing brace, or -1 if not found.
func findJSONObjectEnd(s string) int {
	if len(s) == 0 || s[0] != '{' {
		// Try to find the start of JSON object.
		idx := strings.Index(s, "{")
		if idx == -1 {
			return -1
		}
		s = s[idx:]
	}

	depth := 0
	inString := false
	escape := false

	for i, c := range s {
		if escape {
			escape = false
			continue
		}

		if c == '\\' && inString {
			escape = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}

	return -1
}

func parseToolCall(content string) []ResponseToolCall {

	// {"name":"get_weather", "arguments":{"location":"NYC"}
	if strings.HasPrefix(content, "{\"name\"") {
		return parseJSONFormat(content)
	}

	// <function=get_weather>\n<parameter=location>\nNYC\n</parameter>\n</function>
	// <function=invoke_cli_command>\n<parameter=call>\ngo version\n</parameter>\n</function>
	if strings.HasPrefix(content, "<function=") {
		return parseFunctionFormat(content)
	}

	// get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>
	// GLM-style format with <arg_key>/<arg_value> tags
	if strings.Contains(content, "<arg_key>") {
		return parseArgKeyValueFormat(content)
	}

	// [TOOL_CALLS]get_weather[ARGS]{"location": "NYC"}
	// Mistral/Devstral format
	if strings.Contains(content, "[TOOL_CALLS]") {
		return parseMistralToolCallFormat(content)
	}

	// call:get_weather{location:<|"|>NYC<|"|>}
	// Gemma4 format with call: prefix and <|"|> escaped quotes
	if strings.Contains(content, "call:") {
		return parseGemmaToolCallFormat(content)
	}

	return nil
}

func parseFunctionFormat(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

	// Handle escaped newlines (literal \n) by converting to actual newlines
	content = strings.ReplaceAll(content, "\\n", "\n")

	for {
		funcStart := strings.Index(content, "<function=")
		if funcStart == -1 {
			break
		}

		funcEnd := strings.Index(content[funcStart:], ">")
		if funcEnd == -1 {
			break
		}

		name := content[funcStart+10 : funcStart+funcEnd]

		bodyStart := funcStart + funcEnd + 1
		closeFunc := strings.Index(content[bodyStart:], "</function>")
		if closeFunc == -1 {
			break
		}
		closeFunc += bodyStart

		funcBody := content[bodyStart:closeFunc]
		args := make(map[string]any)

		remaining := funcBody
		for {
			paramStart := strings.Index(remaining, "<parameter=")
			if paramStart == -1 {
				break
			}

			paramNameEnd := strings.Index(remaining[paramStart:], ">")
			if paramNameEnd == -1 {
				break
			}

			paramName := remaining[paramStart+11 : paramStart+paramNameEnd]

			paramClose := strings.Index(remaining, "</parameter>")
			if paramClose == -1 {
				break
			}

			paramValue := strings.TrimSpace(remaining[paramStart+paramNameEnd+1 : paramClose])

			// Try to parse the value as JSON so that arrays and objects
			// are stored as proper Go types ([]any, map[string]any) instead
			// of raw strings. Fall back to the plain string for scalars.
			switch {
			case len(paramValue) == 0:
				args[paramName] = paramValue

			case paramValue[0] == '{', paramValue[0] == '[', paramValue[0] == '"':
				args[paramName] = paramValue

				var parsed any
				if err := json.Unmarshal([]byte(paramValue), &parsed); err == nil {
					args[paramName] = parsed
				}

			default:
				args[paramName] = paramValue
			}

			remaining = remaining[paramClose+12:]
		}

		toolCalls = append(toolCalls, ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
			Function: ResponseToolCallFunction{
				Name:      name,
				Arguments: args,
			},
		})

		content = content[closeFunc+11:]
	}

	return toolCalls
}

func parseJSONFormat(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

	remaining := content
	for len(remaining) > 0 {
		// Skip leading whitespace and newlines.
		remaining = strings.TrimLeft(remaining, " \t\n\r")
		if len(remaining) == 0 {
			break
		}

		// Find the start of a JSON object.
		if remaining[0] != '{' {
			// Skip non-JSON content until we find '{' or run out.
			idx := strings.Index(remaining, "{")
			if idx == -1 {
				break
			}
			remaining = remaining[idx:]
		}

		// Find the end of this JSON object.
		jsonEnd := findJSONObjectEnd(remaining)
		if jsonEnd == -1 {
			// Malformed JSON - try to parse what's left.
			jsonEnd = len(remaining)
		}

		call := remaining[:jsonEnd]
		remaining = remaining[jsonEnd:]

		toolCall := ResponseToolCall{
			ID:   newToolCallID(),
			Type: "function",
		}

		if err := json.Unmarshal([]byte(call), &toolCall.Function); err != nil {
			if repaired, ok := repairJSON(call); ok {
				if err2 := json.Unmarshal([]byte(repaired), &toolCall.Function); err2 != nil {
					toolCall.Status = 2
					toolCall.Error = err2.Error()
					toolCall.Raw = call
				}
			} else {
				toolCall.Status = 2
				toolCall.Error = err.Error()
				toolCall.Raw = call
			}
		}

		// GPT models prefix function names with a dot (e.g. ".Kronk_web_search").
		// Strip it so clients can match the name to their registered tools.
		toolCall.Function.Name = strings.TrimPrefix(toolCall.Function.Name, ".")

		toolCalls = append(toolCalls, toolCall)
	}

	return toolCalls
}

// parseArgKeyValueFormat parses GLM-style tool calls with <arg_key>/<arg_value> tags.
// Format: get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>
func parseArgKeyValueFormat(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

	for call := range strings.SplitSeq(content, "\n") {
		if call == "" {
			continue
		}

		// Find the function name (everything before the first <arg_key>)
		argKeyIdx := strings.Index(call, "<arg_key>")
		if argKeyIdx == -1 {
			continue
		}

		name := strings.TrimSpace(call[:argKeyIdx])
		args := make(map[string]any)

		// Parse all <arg_key>...</arg_key><arg_value>...</arg_value> pairs
		remaining := call[argKeyIdx:]
		for {
			keyStart := strings.Index(remaining, "<arg_key>")
			if keyStart == -1 {
				break
			}

			keyEnd := strings.Index(remaining, "</arg_key>")
			if keyEnd == -1 {
				break
			}

			key := remaining[keyStart+9 : keyEnd]

			valStart := strings.Index(remaining, "<arg_value>")
			if valStart == -1 {
				break
			}

			valEnd := strings.Index(remaining, "</arg_value>")
			if valEnd == -1 {
				break
			}

			value := remaining[valStart+11 : valEnd]
			args[key] = value

			remaining = remaining[valEnd+12:]
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

func parseMistralToolCallFormat(content string) []ResponseToolCall {
	var toolCalls []ResponseToolCall

	remaining := content
	for {
		callStart := strings.Index(remaining, "[TOOL_CALLS]")
		if callStart == -1 {
			break
		}

		argsStart := strings.Index(remaining[callStart:], "[ARGS]")
		if argsStart == -1 {
			break
		}

		name := remaining[callStart+12 : callStart+argsStart]

		argsContent := remaining[callStart+argsStart+6:]

		endIdx := findJSONObjectEnd(argsContent)
		var argsJSON string
		switch endIdx == -1 {
		case true:
			argsJSON = argsContent
			remaining = ""
		case false:
			argsJSON = argsContent[:endIdx]
			remaining = argsContent[endIdx:]
		}

		var args map[string]any
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			if repaired, ok := repairJSON(argsJSON); ok {
				if err2 := json.Unmarshal([]byte(repaired), &args); err2 != nil {
					args = make(map[string]any)
				}
			} else {
				args = make(map[string]any)
			}
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

// parseGemmaToolCallFormat parses Gemma4-style tool calls.
// Format: call:get_weather{location:<|"|>New York City, NY<|"|>}
// Multiple calls may appear separated by newlines or back-to-back.
func parseGemmaToolCallFormat(content string) []ResponseToolCall {
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

		// Find the matching closing brace.
		braceEnd := findGemmaBraceEnd(remaining)
		if braceEnd == -1 {
			break
		}

		argsRaw := remaining[1:braceEnd] // content between { and }
		remaining = remaining[braceEnd+1:]

		// Gemma4 outputs double braces: call:func{{"key":"val"}}.
		// After stripping the outer pair, argsRaw still has {…}.
		//
		// The model may mix <|"|> tokens with standard JSON quotes
		// (e.g., opening a value with <|"|> but closing with ").
		// Strategy: try normalizing <|"|> → " first, then JSON parse,
		// then repairJSON, then fall back to parseGemmaArgs.
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

		// Normalize Gemma4 quoting variations to standard JSON quotes.
		// The model uses <|"|> as quote delimiters when the value contains
		// standard " characters (e.g., code with import "fmt"). Escape
		// any " inside <|"|> pairs first, then replace the delimiters.
		normalized := normalizeGemmaQuotes(jsonCandidate)
		normalized = strings.ReplaceAll(normalized, "`,`", "\",\"")
		normalized = quoteBareJSONKeys(normalized)
		parsed := false

		if json.Unmarshal([]byte(normalized), &args) == nil {
			parsed = true
		} else if repaired, ok := repairJSON(normalized); ok {
			if json.Unmarshal([]byte(repaired), &args) == nil {
				parsed = true
			}
		}

		if !parsed {
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

// normalizeGemmaQuotes converts <|"|> delimited values to standard JSON
// quoted strings. The model uses <|"|> when the value contains literal "
// characters (e.g., source code with import "fmt"). This function finds
// each <|"|>...<|"|> pair, escapes any " inside the value to \", then
// replaces the <|"|> delimiters with standard ".
func normalizeGemmaQuotes(s string) string {
	const token = "<|\"|>"
	tokenLen := len(token)

	if !strings.Contains(s, token) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	i := 0

	for i < len(s) {
		openIdx := strings.Index(s[i:], token)
		if openIdx == -1 {
			b.WriteString(s[i:])
			break
		}

		// Write everything before the opening <|"|>.
		b.WriteString(s[i : i+openIdx])
		i += openIdx + tokenLen

		// Find the closing <|"|>.
		closeIdx := strings.Index(s[i:], token)
		if closeIdx == -1 {
			// No closing token — write " and the rest as-is.
			b.WriteByte('"')
			b.WriteString(s[i:])
			break
		}

		// Extract the value between the <|"|> pair, escape inner quotes,
		// and wrap with standard ".
		inner := s[i : i+closeIdx]
		b.WriteByte('"')
		for j := 0; j < len(inner); j++ {
			switch {
			case inner[j] == '"':
				b.WriteString(`\"`)
			case inner[j] == '\\':
				// Preserve existing escape sequences.
				b.WriteByte('\\')
				if j+1 < len(inner) {
					j++
					b.WriteByte(inner[j])
				}
			default:
				b.WriteByte(inner[j])
			}
		}
		b.WriteByte('"')
		i += closeIdx + tokenLen
	}

	return b.String()
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

// quoteBareJSONKeys adds double quotes around unquoted JSON keys.
// Gemma4 often emits keys without quotes: {content:"text",priority:"high"}
// which is not valid JSON. This function converts them to proper JSON:
// {"content":"text","priority":"high"}.
func quoteBareJSONKeys(s string) string {
	var buf strings.Builder
	buf.Grow(len(s) + 32)

	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			buf.WriteByte(c)
			escaped = false
			continue
		}

		if c == '\\' && inString {
			buf.WriteByte(c)
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			buf.WriteByte(c)
			continue
		}

		if inString {
			buf.WriteByte(c)
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
				// This is a bare key — quote it.
				buf.WriteByte('"')
				buf.WriteString(s[i:j])
				buf.WriteByte('"')
				i = j - 1
				continue
			}
		}

		buf.WriteByte(c)
	}

	return buf.String()
}

// repairJSON attempts to fix malformed JSON from model-generated tool calls.
// The most common issue is unescaped double quotes inside string values — e.g.,
// when a model outputs markdown content containing "quoted" text. Standard
// json.Unmarshal fails on these, but the structure is otherwise valid.
//
// The repair walks the JSON character by character, tracking whether we're
// inside a string value. A quote that appears inside a value and is NOT
// followed by a structural character (comma, colon, closing brace/bracket,
// or whitespace before one) is escaped with a backslash.
//
// Returns the repaired JSON and true if any changes were made, or the
// original string and false if no repair was needed or possible.
func repairJSON(s string) (string, bool) {
	if len(s) == 0 {
		return s, false
	}

	// Quick check: if it already parses, no repair needed.
	var test any
	if json.Unmarshal([]byte(s), &test) == nil {
		return s, false
	}

	// Normalize quoting before repairing.
	s = strings.ReplaceAll(s, "<|\"|>", "\"")
	s = quoteBareJSONKeys(s)

	var buf strings.Builder
	buf.Grow(len(s) + 64)

	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			buf.WriteByte(c)
			escaped = false
			continue
		}

		if c == '\\' && inString {
			buf.WriteByte(c)
			escaped = true
			continue
		}

		if c == '"' {
			if !inString {
				// Opening quote — enter string mode.
				buf.WriteByte(c)
				inString = true
				continue
			}

			// We're inside a string and hit a quote. Decide if this is
			// the closing quote or an unescaped internal quote.
			// The closing quote is followed by a JSON structural character
			// (possibly after whitespace): , : } ]
			if isClosingQuote(s, i) {
				buf.WriteByte(c)
				inString = false
				continue
			}

			// Internal quote — escape it.
			buf.WriteByte('\\')
			buf.WriteByte('"')
			continue
		}

		buf.WriteByte(c)
	}

	repaired := buf.String()

	// Verify the repair actually produces valid JSON.
	if json.Unmarshal([]byte(repaired), &test) != nil {
		return s, false
	}

	return repaired, true
}

// isClosingQuote checks whether the quote at position i in s is a closing
// JSON string quote. A closing quote is followed (after optional whitespace)
// by a JSON structural character: , : } ] or end of string.
func isClosingQuote(s string, i int) bool {
	j := i + 1

	// Skip whitespace.
	for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
		j++
	}

	if j >= len(s) {
		return true
	}

	switch s[j] {
	case ',', ':', '}', ']':
		return true
	}

	return false
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
		if endIdx == -1 {
			args[key] = strings.TrimSpace(remaining)
			break
		}

		args[key] = strings.TrimSpace(remaining[:endIdx])
		remaining = remaining[endIdx:]
	}

	return args
}

// =============================================================================
// Step methods for batch engine (no llama calls - pure state machine)
// =============================================================================

// stepStandard processes a single token for standard models without calling llama.
// This is used by the batch engine where decode/sample happens externally.
// Returns (response, endOfGeneration).
func (p *processor) stepStandard(content string) (response, bool) {
	// Handle pending tag accumulation for detecting split tags like "<function=".
	if p.inPendingTag {
		p.pendingTagBuf.WriteString(content)
		accumulated := p.pendingTagBuf.String()

		// Check if we've accumulated enough to detect <function=.
		if strings.HasPrefix(accumulated, "<function=") {
			// Found the pattern. Enter tool call mode and start accumulating.
			p.inPendingTag = false
			p.pendingTagBuf.Reset()
			p.status = statusTooling
			p.inToolCall = true
			p.toolCallBuf.Reset()
			p.toolCallBuf.WriteString(accumulated)
			return response{}, false
		}

		// Check if it's definitely not going to be <function=.
		if !strings.HasPrefix("<function=", accumulated) {
			// Flush accumulated content as normal output.
			p.inPendingTag = false
			p.pendingTagBuf.Reset()
			return response{status: p.status, content: accumulated}, false
		}

		// Still a prefix match, continue accumulating.
		return response{}, false
	}

	// Handle tool call accumulation mode.
	if p.inToolCall {
		switch content {
		case "<tool_call>", "<|tool_call>":
			// Nested or repeated tag, skip.
			return response{}, false

		case "</tool_call>", "<tool_call|>":
			// End of one tool call block. Check if we have accumulated content.
			toolContent := strings.Trim(p.toolCallBuf.String(), "\n")
			if toolContent != "" {
				toolContent = fmt.Sprintf("%s\n", toolContent)
			}

			p.toolCallBuf.Reset()
			p.inToolCall = false
			p.toolCallDone = true

			// Stay in tool call mode in case there are more tool calls.
			// The next token will be checked by the toolCallDone guard:
			// another <|tool_call> continues, anything else triggers EOG.
			return response{status: statusTooling, content: toolContent}, false

		case "[TOOL_CALLS]":
			// Another tool call starting - flush buffer and start new accumulation.
			p.toolCallBuf.Reset()
			p.toolCallBuf.WriteString("[TOOL_CALLS]")
			return response{}, false

		default:
			// Check if we're accumulating Mistral format (no closing tag).
			buf := p.toolCallBuf.String()
			if strings.HasPrefix(buf, "[TOOL_CALLS]") {
				// Mistral format: accumulate and stream to finalTooling.
				p.toolCallBuf.WriteString(content)
				return response{status: statusTooling, content: content}, false
			}

			// Standard format: accumulate in buffer only.
			p.toolCallBuf.WriteString(content)

			// Check if we've completed a function call (models that skip </tool_call>).
			accumulated := p.toolCallBuf.String()
			if strings.HasSuffix(strings.TrimSpace(accumulated), "</function>") {
				toolContent := strings.Trim(accumulated, "\n")
				if toolContent != "" {
					toolContent = fmt.Sprintf("%s\n", toolContent)
				}

				p.toolCallBuf.Reset()
				p.inToolCall = false

				return response{status: statusTooling, content: toolContent}, false
			}

			return response{}, false
		}
	}

	// After a tool call closes, only allow another tool call opener.
	// Anything else (reasoning, text, etc.) means the model is done.
	if p.toolCallDone {
		switch content {
		case "<tool_call>", "<|tool_call>":
			p.toolCallDone = false
			p.inToolCall = true
			p.toolCallBuf.Reset()
			return response{}, false
		default:
			p.toolCallDone = false
			return response{}, true // EOG — stop generation after tool call(s).
		}
	}

	// Handle Gemma4 channel: swallow the channel name token (e.g. "thought")
	// that follows <|channel>, then stream content as reasoning until <channel|>.
	if p.awaitingChannel {
		p.awaitingChannel = false
		p.status = statusReasoning
		return response{}, false
	}

	// Normal token processing.
	switch content {
	case "<think>":
		p.status = statusReasoning
		return response{}, false

	case "</think>", "<channel|>":
		p.status = statusCompletion
		return response{}, false

	case "<|channel>":
		p.awaitingChannel = true
		return response{}, false

	case "<tool_call>", "<|tool_call>":
		p.status = statusTooling
		p.inToolCall = true
		p.toolCallBuf.Reset()
		return response{}, false

	case "<tool_call|>", "<|tool_response>", "<tool_response|>":
		// Gemma4 structural markers outside of tool call accumulation; skip.
		return response{}, false

	case "[TOOL_CALLS]":
		// Mistral/Devstral format: [TOOL_CALLS]name[ARGS]{...}
		// Stream the marker to finalTooling for parsing at EOG.
		p.status = statusTooling
		p.inToolCall = true
		p.toolCallBuf.Reset()
		p.toolCallBuf.WriteString("[TOOL_CALLS]")
		return response{status: statusTooling, content: "[TOOL_CALLS]"}, false

	default:
		// Check for start of <function= pattern (may be split across tokens).
		if content == "<" || strings.HasPrefix(content, "<f") || strings.HasPrefix(content, "<function") {
			if strings.HasPrefix(content, "<function=") {
				// Complete tag in one token, enter tool call mode directly.
				p.status = statusTooling
				p.inToolCall = true
				p.toolCallBuf.Reset()
				p.toolCallBuf.WriteString(content)
				return response{}, false
			}

			// Could be start of <function=, start accumulating.
			if strings.HasPrefix("<function=", content) {
				p.inPendingTag = true
				p.pendingTagBuf.Reset()
				p.pendingTagBuf.WriteString(content)
				return response{}, false
			}
		}

		return response{status: p.status, content: content}, false
	}
}

// stepGPT processes a single token for GPT models without calling llama.
// This is used by the batch engine where decode/sample happens externally.
// Returns (response, endOfGeneration).
func (p *processor) stepGPT(content string) (response, bool) {
	if p.collecting {
		if content == "<|return|>" || content == "<|call|>" {
			p.collecting = false
			p.status = statusNone
			return response{}, true // End of generation
		}

		if content == "<|end|>" {
			p.collecting = false
			p.status = statusNone
			return response{}, false
		}

		// Handle non-deterministic models that emit <|start|> or <|channel|>
		// without first closing the current block with <|end|>.
		if content == "<|start|>" {
			p.collecting = false
			p.status = statusNone
			p.awaitingChannel = false
			p.awaitingConstrain = false
			p.channelBuf.Reset()
			return response{}, false
		}

		if content == "<|channel|>" {
			p.collecting = false
			p.awaitingChannel = true
			p.channelBuf.Reset()
			return response{}, false
		}

		return response{status: p.status, content: content}, false
	}

	// Skip tokens between <|constrain|> and <|message|> (e.g., "json").
	if p.awaitingConstrain {
		if content == "<|message|>" {
			p.awaitingConstrain = false
			p.collecting = true

			// Emit the function name prefix for tool calls so parseGPTToolCall can parse it.
			// Format: ".FUNC_NAME <|message|>" which parseGPTToolCall expects.
			if p.status == statusTooling && p.toolFuncName != "" {
				prefix := fmt.Sprintf(".%s <|message|>", p.toolFuncName)
				p.toolFuncName = ""
				return response{status: p.status, content: prefix}, false
			}
		}
		return response{}, false
	}

	// Accumulate channel name tokens until <|message|> or <|constrain|>.
	if p.awaitingChannel {
		if content == "<|message|>" || content == "<|constrain|>" {
			p.awaitingChannel = false
			channelName := strings.TrimSpace(p.channelBuf.String())
			p.channelBuf.Reset()

			// Determine status from channel name prefix.
			switch {
			case strings.HasPrefix(channelName, "analysis"):
				p.status = statusReasoning

			case strings.HasPrefix(channelName, "final"):
				p.status = statusCompletion

			case strings.HasPrefix(channelName, "commentary"):
				p.status = statusTooling

				// Extract function name from "commentary to=functions.FUNC_NAME".
				if _, after, ok := strings.Cut(channelName, " to="); ok {
					funcName := strings.TrimSpace(after)
					p.toolFuncName = strings.TrimPrefix(funcName, "functions.")
				}
			}

			switch content == "<|constrain|>" {
			case true:
				p.awaitingConstrain = true
			case false:
				p.collecting = true
			}

			return response{}, false
		}

		p.channelBuf.WriteString(content)

		return response{}, false
	}

	switch content {
	case "<|start|>":
		p.status = statusNone
		p.collecting = false
		p.awaitingChannel = false
		p.awaitingConstrain = false
		p.channelBuf.Reset()
		return response{}, false

	case "<|channel|>":
		p.awaitingChannel = true
		p.channelBuf.Reset()
		return response{}, false

	case "<|message|>":
		p.collecting = true
		return response{}, false

	case "functions":
		p.collecting = true
		p.status = statusTooling
		return response{}, false

	default:
		return response{}, false
	}
}

// resetState resets the processor state for reuse in a new slot.
func (p *processor) resetState() {
	p.status = statusCompletion
	p.collecting = false
	p.awaitingChannel = false
	p.toolCallBuf.Reset()
	p.inToolCall = false
	p.toolCallDone = false
	p.channelBuf.Reset()
	p.awaitingConstrain = false
	p.toolFuncName = ""
}

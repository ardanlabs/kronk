// Package qwen implements the Parser for Qwen and Qwen-Coder models.
//
// Qwen models emit reasoning between <think>...</think> tags and tool calls
// in one of two formats:
//   - JSON envelope: <tool_call>{"name":"x","arguments":{…}}</tool_call>
//   - Direct XML:    <function=x>\n<parameter=k>\nv\n</parameter>\n</function>
//
// Some Qwen-Coder variants tokenize the direct-XML opener as separate tokens
// ("<", "function", "="), so the stateMachine carries a small lookahead buffer
// to detect the split <function=... pattern.
package qwen

import (
	"context"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Parser.Name.
const name = "qwen"

const qwenWrapperFramePrefix = "<qwen-tool-call:"

// Parser implements model.Parser for Qwen.
type Parser struct{}

// New returns a Parser value if the fingerprint indicates a Qwen model,
// otherwise returns false. Detection is layered: GGUF
// "general.architecture" prefix (e.g. "qwen2", "qwen3", "qwen35moe") is
// the strongest signal, the chat template's distinctive Qwen tool-call
// markers (<function=, <parameter=) is the next, and the model name
// substring is a last-resort legacy fallback.
func New(fp model.Fingerprint) (model.Parser, bool) {
	// 1. GGUF architecture prefix.
	if strings.HasPrefix(strings.ToLower(fp.Architecture), "qwen") {
		return Parser{}, true
	}

	// 2. Chat template markers distinctive to Qwen tool calls.
	if containsQwenMarkers(fp.ChatTemplate) {
		return Parser{}, true
	}

	// 3. Model name fallback.
	if strings.Contains(strings.ToLower(fp.ModelName), "qwen") {
		return Parser{}, true
	}

	return Parser{}, false
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelAnswer}
}

// ToolCall parses Qwen's accumulated tool-call buffer. The buffer
// content varies by emission format (JSON envelope vs direct XML), so the
// parser inspects the leading bytes to choose between them.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	buf = unwrapToolCallEvidence(buf)
	trimmed := strings.TrimLeft(buf, " \t\n\r")

	// Direct <function=…> XML format.
	if strings.HasPrefix(trimmed, "<function=") {
		return parseQwenXML(buf)
	}

	// JSON envelope is the default Qwen tool-call format.
	return parseJSON(ctx, log, buf)
}

// ToolCallWithSchema parses Qwen tool calls and uses the declared tool schema
// to recover argument types from the otherwise untyped direct-XML format.
// Qwen's JSON envelope already carries argument types and is left unchanged.
func (Parser) ToolCallWithSchema(ctx context.Context, log applog.Logger, buf string, tools []model.D) []model.ResponseToolCall {
	buf = unwrapToolCallEvidence(buf)
	trimmed := strings.TrimLeft(buf, " \t\n\r")
	if !strings.HasPrefix(trimmed, "<function=") {
		return parseJSON(ctx, log, buf)
	}

	toolCalls := parseQwenXML(buf)
	normalizeXMLArguments(toolCalls, tools)

	return toolCalls
}

// StripToolCallMarkup removes Qwen JSON-envelope and direct-XML tool calls
// from a discarded round while preserving text outside recognized calls.
func (Parser) StripToolCallMarkup(buf string) string {
	var content strings.Builder
	var removed bool

	for len(buf) > 0 {
		switch {
		case strings.HasPrefix(buf, qwenWrapperFramePrefix):
			removed = true
			_, _, rest, ok := decodeQwenWrapperFrame(buf)
			if !ok {
				buf = ""
				continue
			}
			buf = rest

		case strings.HasPrefix(buf, "<tool_call>"), strings.HasPrefix(buf, "<|tool_call>"):
			removed = true
			end := qwenToolCallBlockEnd(buf)
			if end == -1 {
				buf = ""
				continue
			}
			buf = buf[end:]

		case strings.HasPrefix(buf, "<function="):
			removed = true
			end := qwenFunctionEnd(buf)
			if end == -1 {
				buf = ""
				continue
			}
			buf = buf[end:]

		case strings.HasPrefix(buf, "</tool_call>"):
			removed = true
			buf = buf[len("</tool_call>"):]

		case strings.HasPrefix(buf, "<tool_call|>"):
			removed = true
			buf = buf[len("<tool_call|>"):]

		case buf[0] == '{':
			end := findJSONObjectEnd(buf)
			if end == -1 {
				if qwenJSONToolCall(buf) {
					removed = true
					buf = ""
					continue
				}
				content.WriteString(buf)
				buf = ""
				continue
			}
			if qwenJSONToolCall(buf[:end]) {
				removed = true
				buf = buf[end:]
				continue
			}
			content.WriteString(buf[:end])
			buf = buf[end:]

		default:
			content.WriteByte(buf[0])
			buf = buf[1:]
		}
	}

	stripped := content.String()
	if removed {
		stripped = stripQwenMarkerPrefix(stripped)
	}
	if strings.TrimSpace(stripped) != "" {
		return stripped
	}

	return ""
}

func qwenJSONToolCall(content string) bool {
	if _, ok := toolCallName(content); !ok {
		return false
	}
	_, ok := jsonFieldValueStart(content, "arguments")
	return ok
}

func stripQwenMarkerPrefix(content string) string {
	for _, marker := range []string{"<tool_call>", "<|tool_call>", "</tool_call>", "<tool_call|>", "<function="} {
		for size := min(len(content), len(marker)-1); size > 0; size-- {
			if strings.HasSuffix(content, marker[:size]) {
				return content[:len(content)-size]
			}
		}
	}
	return content
}

func qwenToolCallBlockEnd(buf string) int {
	var opener string
	var closer string
	switch {
	case strings.HasPrefix(buf, "<tool_call>"):
		opener, closer = "<tool_call>", "</tool_call>"
	case strings.HasPrefix(buf, "<|tool_call>"):
		opener, closer = "<|tool_call>", "<tool_call|>"
	default:
		return -1
	}
	body := buf[len(opener):]
	end := qwenWrapperClose(body, closer)
	if end == -1 {
		return -1
	}
	return len(opener) + end + len(closer)
}

func qwenFunctionEnd(buf string) int {
	openerEnd := strings.IndexByte(buf, '>')
	if openerEnd == -1 {
		return -1
	}

	inParameter := false
	for offset := openerEnd + 1; offset < len(buf); {
		next := strings.IndexByte(buf[offset:], '<')
		if next == -1 {
			return -1
		}
		offset += next

		switch {
		case strings.HasPrefix(buf[offset:], "<parameter="):
			if inParameter {
				return -1
			}
			inParameter = true
			offset += len("<parameter=")
		case strings.HasPrefix(buf[offset:], "</parameter>"):
			if !inParameter {
				return -1
			}
			inParameter = false
			offset += len("</parameter>")
		case strings.HasPrefix(buf[offset:], "</function>"):
			if inParameter {
				return -1
			}
			return offset + len("</function>")
		case strings.HasPrefix(buf[offset:], "<function="):
			return -1
		default:
			offset++
		}
	}

	return -1
}

func unwrapToolCallEvidence(buf string) string {
	var content strings.Builder
	remaining := strings.TrimLeft(buf, " \t\n\r")
	for remaining != "" {
		if strings.HasPrefix(remaining, qwenWrapperFramePrefix) {
			body, _, rest, ok := decodeQwenWrapperFrame(remaining)
			if !ok {
				content.WriteString(remaining)
				return content.String()
			}
			content.WriteString(body)
			remaining = strings.TrimLeft(rest, " \t\n\r")
			if remaining != "" {
				content.WriteByte('\n')
			}
			continue
		}

		var opener string
		var closer string
		switch {
		case strings.HasPrefix(remaining, "<tool_call>"):
			opener, closer = "<tool_call>", "</tool_call>"
		case strings.HasPrefix(remaining, "<|tool_call>"):
			opener, closer = "<|tool_call>", "<tool_call|>"
		default:
			content.WriteString(remaining)
			return content.String()
		}

		remaining = remaining[len(opener):]
		end := qwenWrapperClose(remaining, closer)
		if end == -1 {
			content.WriteString(remaining)
			return content.String()
		}
		content.WriteString(remaining[:end])
		remaining = strings.TrimLeft(remaining[end+len(closer):], " \t\n\r")
		if remaining != "" {
			content.WriteByte('\n')
		}
	}

	return content.String()
}

func encodeQwenWrapperFrame(content string, complete bool) string {
	status := byte('M')
	if complete {
		status = 'C'
	}
	return qwenWrapperFramePrefix + string(status) + ":" + strconv.Itoa(len(content)) + ":" + content
}

func decodeQwenWrapperFrame(content string) (string, bool, string, bool) {
	if !strings.HasPrefix(content, qwenWrapperFramePrefix) {
		return "", false, content, false
	}
	rest := content[len(qwenWrapperFramePrefix):]
	if len(rest) < 3 || (rest[0] != 'C' && rest[0] != 'M') || rest[1] != ':' {
		return "", false, content, false
	}
	sizeEnd := strings.IndexByte(rest[2:], ':')
	if sizeEnd == -1 {
		return "", false, content, false
	}
	sizeEnd += 2
	size, err := strconv.Atoi(rest[2:sizeEnd])
	bodyStart := sizeEnd + 1
	if err != nil || size < 0 || len(rest)-bodyStart < size {
		return "", false, content, false
	}
	return rest[bodyStart : bodyStart+size], rest[0] == 'C', rest[bodyStart+size:], true
}

func qwenWrapperClose(content string, closer string) int {
	limit := len(content)
	for _, opener := range []string{"<tool_call>", "<|tool_call>"} {
		if at := strings.Index(content, opener); at != -1 && at < limit {
			limit = at
		}
	}
	return strings.LastIndex(content[:limit], closer)
}

// containsQwenMarkers reports whether a chat template carries distinctive
// Qwen tool-call tokens. The <function= and <parameter= openers are
// specific to Qwen's direct-XML tool-call format and unlikely to appear
// in any other lineage's template.
func containsQwenMarkers(template string) bool {
	for _, marker := range []string{
		"<function=",
		"<parameter=",
	} {
		if strings.Contains(template, marker) {
			return true
		}
	}
	return false
}

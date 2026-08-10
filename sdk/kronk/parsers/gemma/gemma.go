// Package gemma implements the Parser for Google's Gemma model lineage.
//
// Detection is layered: the GGUF "general.architecture" prefix (e.g.
// "gemma2", "gemma3", "gemma4") is the strongest signal, the chat
// template's distinctive Gemma markers (e.g. <start_of_turn>) is the
// next, and the model name substring is a last-resort legacy fallback.
package gemma

import (
	"context"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Parser.Name.
const name = "gemma"

const gemmaWrapperFramePrefix = "<gemma-tool-call:"

// Parser implements model.Parser for the Gemma lineage.
type Parser struct{}

// New returns a Parser value if the fingerprint indicates a Gemma model,
// otherwise returns false.
func New(fp model.Fingerprint) (model.Parser, bool) {
	// 1. GGUF architecture prefix: set by the model author and the most
	//    reliable signal of model lineage.
	if strings.HasPrefix(strings.ToLower(fp.Architecture), "gemma") {
		return Parser{}, true
	}

	// 2. Chat template markers: identify the prompt format the model was
	//    fine-tuned to emit, which determines reasoning and tool-call
	//    parsing.
	if containsGemmaMarkers(fp.ChatTemplate) {
		return Parser{}, true
	}

	// 3. Model name fallback: legacy GGUFs without rich metadata.
	if strings.Contains(strings.ToLower(fp.ModelName), "gemma") {
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

// ToolCall parses Gemma's accumulated tool-call buffer.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseGemma(ctx, log, unwrapGemmaToolCallEvidence(buf))
}

// ToolCallWithSchema parses Gemma tool calls and reconciles the native value
// types with the matching function's declared parameter schema.
func (Parser) ToolCallWithSchema(ctx context.Context, log applog.Logger, buf string, tools []model.D) []model.ResponseToolCall {
	toolCalls := parseGemma(ctx, log, unwrapGemmaToolCallEvidence(buf))
	normalizeGemmaArguments(toolCalls, tools)

	return toolCalls
}

// StripToolCallMarkup removes Gemma tool-call bodies from a tool buffer.
func (Parser) StripToolCallMarkup(buf string) string {
	trimmed := strings.TrimLeft(buf, " \t\n\r")
	if strings.HasPrefix(trimmed, gemmaWrapperFramePrefix) {
		_, _, rest, ok := decodeGemmaWrapperFrame(trimmed)
		if !ok {
			return ""
		}
		result := (Parser{}).StripToolCallMarkup(rest)
		return emptyGemmaToolWhitespace(stripGemmaMarkerPrefix(result))
	}
	for _, wrapper := range []struct {
		opener string
		closer string
	}{
		{opener: "<tool_call>", closer: "</tool_call>"},
		{opener: "<|tool_call>", closer: "<tool_call|>"},
	} {
		if !strings.HasPrefix(trimmed, wrapper.opener) {
			continue
		}

		body := trimmed[len(wrapper.opener):]
		end := gemmaWrapperClose(body, wrapper.closer)
		if end == -1 {
			return ""
		}
		result := (Parser{}).StripToolCallMarkup(body[end+len(wrapper.closer):])
		return emptyGemmaToolWhitespace(stripGemmaMarkerPrefix(result))
	}

	var clean strings.Builder
	removed := false
	for buf != "" {
		start := strings.Index(buf, "call:")
		if start == -1 {
			clean.WriteString(buf)
			break
		}

		brace := strings.IndexByte(buf[start+len("call:"):], '{')
		if brace == -1 {
			if strings.TrimSpace(buf[:start]) != "" || strings.TrimSpace(buf[start+len("call:"):]) == "" {
				clean.WriteString(buf)
			} else {
				clean.WriteString(buf[:start])
			}
			break
		}
		brace += start + len("call:")
		if strings.TrimSpace(buf[start+len("call:"):brace]) == "" {
			clean.WriteString(buf[:start+len("call:")])
			buf = buf[start+len("call:"):]
			continue
		}

		clean.WriteString(buf[:start])
		removed = true
		end := findGemmaBraceEnd(buf[brace:])
		if end == -1 {
			buf = ""
			break
		}
		buf = buf[brace+end+1:]
	}

	result := clean.String()
	if removed {
		replacer := strings.NewReplacer("<tool_call>", "", "<|tool_call>", "", "</tool_call>", "", "<tool_call|>", "")
		result = replacer.Replace(result)
		result = stripGemmaMarkerPrefix(result)
	}
	if strings.TrimSpace(result) == "" {
		return ""
	}
	return result
}

func stripGemmaMarkerPrefix(content string) string {
	for _, marker := range []string{"<tool_call>", "<|tool_call>", "</tool_call>", "<tool_call|>"} {
		for size := min(len(content), len(marker)-1); size > 0; size-- {
			if strings.HasSuffix(content, marker[:size]) {
				return content[:len(content)-size]
			}
		}
	}
	return content
}

func unwrapGemmaToolCallEvidence(buf string) string {
	var content strings.Builder
	remaining := strings.TrimLeft(buf, " \t\n\r")
	for remaining != "" {
		if strings.HasPrefix(remaining, gemmaWrapperFramePrefix) {
			body, _, rest, ok := decodeGemmaWrapperFrame(remaining)
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

		if !strings.HasPrefix(remaining, "<tool_call>") {
			content.WriteString(remaining)
			return content.String()
		}

		remaining = remaining[len("<tool_call>"):]
		end := gemmaWrapperClose(remaining, "</tool_call>")
		if end == -1 {
			content.WriteString(remaining)
			return content.String()
		}
		content.WriteString(remaining[:end])
		remaining = strings.TrimLeft(remaining[end+len("</tool_call>"):], " \t\n\r")
		if remaining != "" {
			content.WriteByte('\n')
		}
	}

	return content.String()
}

func encodeGemmaWrapperFrame(content string, complete bool) string {
	status := byte('M')
	if complete {
		status = 'C'
	}
	return gemmaWrapperFramePrefix + string(status) + ":" + strconv.Itoa(len(content)) + ":" + content
}

func decodeGemmaWrapperFrame(content string) (string, bool, string, bool) {
	if !strings.HasPrefix(content, gemmaWrapperFramePrefix) {
		return "", false, content, false
	}
	rest := content[len(gemmaWrapperFramePrefix):]
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

func gemmaWrapperClose(content string, closer string) int {
	limit := len(content)
	for _, opener := range []string{"<tool_call>", "<|tool_call>"} {
		if at := strings.Index(content, opener); at != -1 && at < limit {
			limit = at
		}
	}
	return strings.LastIndex(content[:limit], closer)
}

func emptyGemmaToolWhitespace(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return content
}

// containsGemmaMarkers reports whether a chat template carries distinctive
// Gemma tokens. Any one is sufficient because no other supported lineage
// uses these exact tokens.
func containsGemmaMarkers(template string) bool {
	for _, marker := range []string{
		"<start_of_turn>",
		"<end_of_turn>",
		"<|channel>",
	} {
		if strings.Contains(template, marker) {
			return true
		}
	}
	return false
}

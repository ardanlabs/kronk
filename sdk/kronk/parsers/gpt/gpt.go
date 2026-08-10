// Package gpt implements the Parser for GPT-OSS models, which use the
// OpenAI Harmony chat-template markers (<|channel|>, <|message|>, <|return|>,
// <|call|>, <|end|>, <|start|>, <|constrain|>).
//
// Reasoning, completion, and tool-call routing all hinge on the same
// <|channel|> marker (e.g. "analysis" → reasoning, "final" → completion,
// a functions recipient on the role or an analysis/commentary channel → tool
// call). Because the marker is shared,
// the stateMachine here cannot be assembled from independent reasoning and
// tool-call plugins; it must be a single state machine, which is why
// GPT-OSS is its own parser rather than a parser-only variant of standard.
package gpt

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// name is the canonical name returned by Parser.Name.
const name = "gpt-oss"

// Parser implements model.Parser for GPT-OSS.
type Parser struct{}

// New returns a Parser value if the fingerprint indicates GPT-OSS, otherwise
// returns false. Detection uses the chat template's Harmony markers.
func New(fp model.Fingerprint) (model.Parser, bool) {
	// GPT-OSS chat templates contain the unique Harmony markers.
	if containsHarmonyMarkers(fp.ChatTemplate) {
		return Parser{}, true
	}

	return Parser{}, false
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot streaming state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelNone}
}

// ToolCall extracts JSON tool calls from the GPT-OSS Harmony format and
// parses each one. The format is "[.NAME <|message|>]JSON" repeated; this
// function recovers the pairs and turns each into a JSON object that the
// shared JSON parser can decode.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	return parseGPTToolCall(ctx, log, buf)
}

// StripToolCallMarkup removes GPT-OSS Harmony tool-call frames from a tool
// buffer.
func (Parser) StripToolCallMarkup(buf string) string {
	var clean strings.Builder
	for buf != "" {
		start, payload, ok := gptToolFrameStart(buf)
		if evidence := strings.Index(buf, incompleteFramingMarker); evidence >= 0 && (!ok || evidence < start) {
			clean.WriteString(buf[:evidence])
			break
		}
		if !ok {
			if evidence := gptMalformedToolEvidenceStart(buf); evidence >= 0 {
				clean.WriteString(buf[:evidence])
				buf = ""
				break
			}
			clean.WriteString(buf)
			break
		}
		clean.WriteString(buf[:start])

		end, err := strictJSONObjectEnd(payload)
		if err != nil {
			if next, _, ok := gptToolFrameStart(payload); ok {
				buf = payload[next:]
				continue
			}
			buf = ""
			break
		}
		buf = payload[end:]
		if strings.Contains(buf, incompleteFramingMarker) {
			buf = ""
			continue
		}
		buf = consumeGPTToolMarkers(buf)
		if gptToolMarkerPrefix(buf) {
			buf = ""
		}
	}

	result := clean.String()
	if trimASCIIWhitespace(result) == "" {
		return ""
	}
	return result
}

func consumeGPTToolMarkers(content string) string {
	markers := append(append([]string{}, harmonyMarkers...), "<|missing-end|>", "<|post-eog|>", "<|invalid-framing|>", incompleteFramingMarker)
	for {
		matched := false
		for _, marker := range markers {
			if strings.HasPrefix(content, marker) {
				content = content[len(marker):]
				matched = true
				break
			}
		}
		if !matched {
			return content
		}
	}
}

func gptToolMarkerPrefix(content string) bool {
	trimmed := strings.TrimSpace(content)
	for _, marker := range harmonyMarkers {
		if trimmed != "" && len(trimmed) < len(marker) && strings.HasPrefix(marker, trimmed) {
			return true
		}
	}
	return false
}

func gptToolFrameStart(buf string) (int, string, bool) {
	for start := 0; start < len(buf); start++ {
		if buf[start] != '.' {
			continue
		}
		nameEnd := start + 1
		for nameEnd < len(buf) && !isASCIIWhitespace(rune(buf[nameEnd])) {
			nameEnd++
		}
		if !safeFunctionName(buf[start+1 : nameEnd]) {
			continue
		}
		marker := nameEnd
		for marker < len(buf) && isASCIIWhitespace(rune(buf[marker])) {
			marker++
		}
		if !strings.HasPrefix(buf[marker:], messageMarker) {
			continue
		}
		payload := marker + len(messageMarker)
		for payload < len(buf) && isASCIIWhitespace(rune(buf[payload])) {
			payload++
		}
		return start, buf[payload:], true
	}
	return 0, "", false
}

func gptMalformedToolEvidenceStart(buf string) int {
	for start := 0; start < len(buf); start++ {
		if buf[start] != '.' {
			continue
		}
		nameEnd := start + 1
		for nameEnd < len(buf) && !isASCIIWhitespace(rune(buf[nameEnd])) {
			nameEnd++
		}
		if !safeFunctionName(buf[start+1 : nameEnd]) {
			continue
		}
		rest := strings.TrimLeft(buf[nameEnd:], " \t\r\n")
		if strings.HasPrefix(rest, "<|invalid-framing|>") || rest == "" {
			return start
		}
	}
	return -1
}

// containsHarmonyMarkers reports whether a chat template carries the
// distinctive GPT-OSS Harmony tokens. Any one is sufficient because no
// other parser uses these exact tokens.
func containsHarmonyMarkers(template string) bool {
	for _, marker := range []string{
		"<|channel|>",
		"<|message|>",
		"<|return|>",
		"<|call|>",
	} {
		if strings.Contains(template, marker) {
			return true
		}
	}
	return false
}

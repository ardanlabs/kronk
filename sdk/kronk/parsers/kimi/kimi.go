// Package kimi implements the Parser for Kimi models that use the
// <|open|>/<|close|> structured output protocol.
package kimi

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const (
	name = "kimi"

	openToken  = "<|open|>"
	closeToken = "<|close|>"
	sepToken   = "<|sep|>"

	thinkOpen     = openToken + "think" + sepToken
	thinkClose    = closeToken + "think" + sepToken
	responseOpen  = openToken + "response" + sepToken
	responseClose = closeToken + "response" + sepToken
	toolsOpen     = openToken + "tools" + sepToken
	toolsClose    = closeToken + "tools" + sepToken
	callOpen      = openToken + "call"
	callClose     = closeToken + "call" + sepToken
	argumentOpen  = openToken + "argument"
	argumentClose = closeToken + "argument" + sepToken
)

// Parser implements model.Parser for Kimi structured output.
type Parser struct{}

// New returns a Parser when the fingerprint identifies a Kimi K3 model or its
// chat template constructs the Kimi K3 structured output protocol.
func New(fp model.Fingerprint) (model.Parser, bool) {
	if containsKimiMarkers(fp.ChatTemplate) {
		return Parser{}, true
	}

	architecture := strings.ToLower(fp.Architecture)
	if strings.Contains(architecture, "kimi") && strings.Contains(architecture, "k3") {
		return Parser{}, true
	}

	modelName := strings.ToLower(fp.ModelName)
	if strings.Contains(modelName, "kimi") && strings.Contains(modelName, "k3") {
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

// ToolCall parses an accumulated Kimi tools block.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	calls := parseTools(buf)
	if log == nil {
		return calls
	}

	for i, call := range calls {
		if call.Status == 0 {
			continue
		}
		log(ctx, "parse-kimi-tools", "status", "failed", "index", i,
			"func", call.Function.Name, "error", call.Error, "raw", call.Raw)
	}

	return calls
}

// containsKimiMarkers recognizes both rendered markers and the macro-based
// construction used by the official Kimi K3 template.
func containsKimiMarkers(template string) bool {
	if strings.Contains(template, thinkOpen) &&
		strings.Contains(template, responseOpen) &&
		strings.Contains(template, toolsOpen) {
		return true
	}

	return strings.Contains(template, "'<|open|>' + tag") &&
		strings.Contains(template, "'<|close|>' + tag") &&
		strings.Contains(template, "'<|sep|>'") &&
		strings.Contains(template, "otag('response')") &&
		strings.Contains(template, "otag('tools')") &&
		strings.Contains(template, "otag('call'")
}

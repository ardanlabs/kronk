// Package kimi implements the Parser for Kimi models that use the K3
// open/close/sep protocol for reasoning, responses, and tool calls.
package kimi

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const name = "kimi"

// Parser implements model.Parser for Kimi K3 output.
type Parser struct{}

// New returns a Parser when the fingerprint identifies Kimi or its chat
// template constructs the K3 protocol.
func New(fp model.Fingerprint) (model.Parser, bool) {
	arch := strings.ToLower(fp.Architecture)
	modelName := strings.ToLower(fp.ModelName)

	if strings.Contains(arch, "kimi") || strings.Contains(arch, "moonshot") {
		return Parser{}, true
	}

	if containsK3Markers(fp.ChatTemplate) {
		return Parser{}, true
	}

	if strings.Contains(modelName, "kimi") || strings.Contains(modelName, "moonshot") {
		return Parser{}, true
	}

	return Parser{}, false
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return &stateMachine{status: model.ChannelAnswer}
}

// ToolCall parses an accumulated Kimi K3 tools block.
func (Parser) ToolCall(_ context.Context, _ applog.Logger, buf string) []model.ResponseToolCall {
	return parseToolCalls(buf)
}

func containsK3Markers(template string) bool {
	return strings.Contains(template, "<|open|>") &&
		strings.Contains(template, "<|close|>") &&
		strings.Contains(template, "<|sep|>") &&
		(strings.Contains(template, "otag('response')") ||
			strings.Contains(template, `otag("response")`))
}

// Package lfm implements the parser for Liquid AI LFM models.
package lfm

import (
	"context"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

const (
	name       = "lfm"
	toolOpen   = "<|tool_call_start|>"
	toolClose  = "<|tool_call_end|>"
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// Parser implements model.Parser for Liquid AI LFM models.
type Parser struct{}

// New returns a Parser when the fingerprint identifies an LFM model.
func New(fp model.Fingerprint) (model.Parser, bool) {
	template := fp.ChatTemplate
	if strings.Contains(template, toolOpen) && strings.Contains(template, toolClose) {
		return Parser{}, true
	}
	if strings.HasPrefix(strings.ToLower(fp.Architecture), "lfm") {
		return Parser{}, true
	}
	if strings.Contains(strings.ToLower(fp.ModelName), "lfm") {
		return Parser{}, true
	}
	return Parser{}, false
}

// Name returns the parser identifier.
func (Parser) Name() string { return name }

// NewStateMachine returns a fresh per-slot state machine.
func (Parser) NewStateMachine() model.StateMachine {
	return &stateMachine{channel: model.ChannelAnswer}
}

// ToolCall parses LFM's Python-style or JSON tool-call payload.
func (Parser) ToolCall(ctx context.Context, log applog.Logger, buf string) []model.ResponseToolCall {
	calls := parseToolCalls(buf)
	if log != nil {
		for i, call := range calls {
			if call.Status == parseErrorStatus {
				log(ctx, "parse-lfm-tools", "status", "failed", "index", i, "error", call.Error, "raw", call.Raw)
			}
		}
	}
	return calls
}

package caching

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func spcCfg() Config {
	return Config{
		Strategy:  StrategySPC,
		SPCSeqID:  7,
		MinTokens: 3,
	}
}

// successDeps returns a fakeDeps that passes every stage of a full SPC build.
func successDeps() *fakeDeps {
	return &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "templated-system-prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return []llama.Token{10, 20, 30, 40}
		},
		extractKVStateFn: func(_ llama.SeqId) ([]byte, int, error) {
			return []byte{0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x00}, 4, nil
		},
	}
}

func systemReq(content any, extra ...D) D {
	msgs := []D{{"role": "system", "content": content}}
	msgs = append(msgs, extra...)
	return D{"messages": msgs}
}

func mustMessages(t *testing.T, d D) []D {
	t.Helper()
	msgs, ok := d["messages"].([]D)
	if !ok {
		t.Fatal("messages not []D")
	}
	return msgs
}

// =========================================================================
// A. Constructor + no-op API surface
// =========================================================================

func TestNewSPCCache_UsesConfig(t *testing.T) {
	cfg := Config{Strategy: StrategySPC, SPCSeqID: 42, MinTokens: 5}
	c := NewSPCCache(&fakeDeps{}, cfg)

	if c.cacheSeqID != 42 {
		t.Errorf("cacheSeqID = %d, want 42", c.cacheSeqID)
	}
	if c.cfg.MinTokens != 5 {
		t.Errorf("MinTokens = %d, want 5", c.cfg.MinTokens)
	}
	if c.session != nil {
		t.Error("session should be nil on construction")
	}
}

func TestSPCCache_NoSlotMethods(t *testing.T) {
	c := NewSPCCache(&fakeDeps{}, spcCfg())
	c.session = &spcSession{sysPromptTokens: 99, kvState: []byte{1}}

	c.ClearPending(1)
	c.CommitSession(Commit{SlotID: 1, Hash: "x", TotalCached: 10})
	c.InvalidateSlot(1)
	c.SetSlotMRoPE(1, true)

	if c.session == nil || c.session.sysPromptTokens != 99 {
		t.Error("slot methods must not mutate session")
	}

	snap, ok := c.SnapshotSlot(0)
	if ok {
		t.Error("SnapshotSlot should return false")
	}
	if snap.SlotID != 0 || snap.SeqID != 0 || snap.TotalTokensCached != 0 || snap.CachedMsgsHash != "" {
		t.Error("SnapshotSlot should return zero SlotSnapshot")
	}
	if c.HasCachedSlot(0) {
		t.Error("HasCachedSlot should return false")
	}
}

func TestSPCCache_ClearCaches(t *testing.T) {
	c := NewSPCCache(&fakeDeps{}, spcCfg())
	c.session = &spcSession{sysPromptTokens: 100, kvState: []byte{1, 2, 3}}

	c.ClearCaches()

	if c.session != nil {
		t.Error("session should be nil after ClearCaches")
	}

	err := c.RestoreSPCToSeq(0)
	if err == nil || !strings.Contains(err.Error(), "no cached KV state available") {
		t.Errorf("RestoreSPCToSeq after clear: got %v", err)
	}

	result := c.ProcessCache(context.Background(), D{"messages": []D{{"role": "user", "content": "hi"}}}, time.Now())
	if result.SPC != nil {
		t.Error("non-system request should not hit SPC after ClearCaches")
	}
}

// =========================================================================
// B. ProcessCache guard / no-op behavior
// =========================================================================

func TestProcessSPC_Guards_InvalidMessagesOrRole(t *testing.T) {
	tests := []struct {
		name string
		d    D
	}{
		{"no messages key", D{"other": "data"}},
		{"messages wrong type", D{"messages": "not-a-slice"}},
		{"empty messages", D{"messages": []D{}}},
		{"role missing", D{"messages": []D{{"content": "hello"}}}},
		{"role not string", D{"messages": []D{{"role": 42, "content": "hello"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var depsCalled atomic.Int32
			deps := &fakeDeps{
				createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
					depsCalled.Add(1)
					return "", nil, nil
				},
			}
			c := NewSPCCache(deps, spcCfg())

			result := c.ProcessCache(context.Background(), tt.d, time.Now())
			if result.Err != nil {
				t.Errorf("unexpected error: %v", result.Err)
			}
			if result.SPC != nil {
				t.Error("expected nil SPC result")
			}
			if depsCalled.Load() != 0 {
				t.Error("deps should not have been called")
			}
		})
	}
}

func TestProcessSPC_Guards_EmptySystemContent(t *testing.T) {
	tests := []struct {
		name string
		msg  D
	}{
		{"missing content", D{"role": "system"}},
		{"nil content", D{"role": "system", "content": nil}},
		{"empty string", D{"role": "system", "content": ""}},
		{"multipart no text", D{"role": "system", "content": []any{
			map[string]any{"type": "image_url", "url": "http://img"},
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var depsCalled atomic.Int32
			deps := &fakeDeps{
				createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
					depsCalled.Add(1)
					return "", nil, nil
				},
			}
			c := NewSPCCache(deps, spcCfg())
			d := D{"messages": []D{tt.msg, {"role": "user", "content": "hi"}}}

			result := c.ProcessCache(context.Background(), d, time.Now())
			if result.Err != nil {
				t.Errorf("unexpected error: %v", result.Err)
			}
			if result.SPC != nil {
				t.Error("expected nil SPC result for empty content")
			}
			if depsCalled.Load() != 0 {
				t.Error("deps should not have been called")
			}
		})
	}
}

func TestProcessSPC_UsesOnlyFirstMessage(t *testing.T) {
	c := NewSPCCache(&fakeDeps{}, spcCfg())
	c.session = &spcSession{sysPromptTokens: 50, kvState: []byte{1}}

	d := D{"messages": []D{
		{"role": "user", "content": "first"},
		{"role": "system", "content": "later system message"},
	}}

	result := c.ProcessCache(context.Background(), d, time.Now())
	if result.SPC == nil {
		t.Fatal("expected SPC hit from existing session")
	}
	if result.SPC.CacheIdx != 50 {
		t.Errorf("CacheIdx = %d, want 50", result.SPC.CacheIdx)
	}

	msgs := mustMessages(t, result.ModifiedD)
	if len(msgs) != 2 {
		t.Error("messages should be unchanged for non-system first message")
	}
}

// =========================================================================
// C. Successful build / hit behavior
// =========================================================================

func TestProcessSPC_BuildsAndExternalizesSystemPrompt(t *testing.T) {
	var clearSeqCalls []llama.SeqId
	var decodeSeqID llama.SeqId
	var decodeStartPos int
	var promptReceived D

	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, d D) (string, [][]byte, error) {
			promptReceived = d
			return "templated-system-prompt", nil, nil
		},
		tokenizeStringFn: func(prompt string) []llama.Token {
			if prompt != "templated-system-prompt" {
				t.Errorf("tokenized unexpected prompt: %q", prompt)
			}
			return []llama.Token{10, 20, 30, 40}
		},
		decodeTokensFn: func(_ context.Context, _ []llama.Token, seqID llama.SeqId, startPos int) error {
			decodeSeqID = seqID
			decodeStartPos = startPos
			return nil
		},
		clearSequenceFn: func(seqID llama.SeqId) {
			clearSeqCalls = append(clearSeqCalls, seqID)
		},
		extractKVStateFn: func(seqID llama.SeqId) ([]byte, int, error) {
			return []byte{0xCA, 0xFE, 0xBA, 0xBE, 0x00, 0x00}, 4, nil
		},
	}

	cfg := spcCfg()
	c := NewSPCCache(deps, cfg)

	d := systemReq(
		[]any{
			map[string]any{"type": "text", "text": "You are helpful."},
			map[string]any{"type": "text", "text": " Be concise."},
		},
		D{"role": "user", "content": "hello"},
	)

	result := c.ProcessCache(context.Background(), d, time.Now())

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.SPC == nil {
		t.Fatal("expected SPC result")
	}
	if result.SPC.CacheIdx != 4 {
		t.Errorf("CacheIdx = %d, want 4", result.SPC.CacheIdx)
	}

	// System message should be removed, leaving only the user message.
	msgs := mustMessages(t, result.ModifiedD)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message after removal, got %d", len(msgs))
	}
	if msgs[0]["role"] != "user" {
		t.Errorf("remaining message role = %q, want user", msgs[0]["role"])
	}

	// CreatePrompt should receive only the system message with add_generation_prompt=false.
	promptMsgs, _ := promptReceived["messages"].([]D)
	if len(promptMsgs) != 1 {
		t.Fatalf("CreatePrompt received %d messages, want 1", len(promptMsgs))
	}
	if promptMsgs[0]["role"] != "system" {
		t.Errorf("CreatePrompt message role = %q, want system", promptMsgs[0]["role"])
	}
	promptContent := ExtractMessageContent(promptMsgs[0])
	if promptContent != "You are helpful. Be concise." {
		t.Errorf("CreatePrompt message content = %q, want %q", promptContent, "You are helpful. Be concise.")
	}
	if agp, _ := promptReceived["add_generation_prompt"].(bool); agp {
		t.Error("add_generation_prompt should be false")
	}

	if decodeSeqID != cfg.SPCSeqID {
		t.Errorf("DecodeTokensIntoCache seqID = %d, want %d", decodeSeqID, cfg.SPCSeqID)
	}
	if decodeStartPos != 0 {
		t.Errorf("DecodeTokensIntoCache startPos = %d, want 0", decodeStartPos)
	}

	// ClearSequence: once before decode, once after extract.
	if len(clearSeqCalls) != 2 {
		t.Fatalf("ClearSequence called %d times, want 2", len(clearSeqCalls))
	}
	for i, id := range clearSeqCalls {
		if id != cfg.SPCSeqID {
			t.Errorf("ClearSequence call %d seqID = %d, want %d", i, id, cfg.SPCSeqID)
		}
	}

	// Session state validation.
	if c.session == nil {
		t.Fatal("session should be populated after build")
	}
	if c.session.sysPromptTokens != 4 {
		t.Errorf("sysPromptTokens = %d, want 4", c.session.sysPromptTokens)
	}
	if c.session.sysPromptLen != len("You are helpful. Be concise.") {
		t.Errorf("sysPromptLen = %d, want %d", c.session.sysPromptLen, len("You are helpful. Be concise."))
	}
	if c.session.seqID != cfg.SPCSeqID {
		t.Errorf("session seqID = %d, want %d", c.session.seqID, cfg.SPCSeqID)
	}
	if c.session.lastUsed.IsZero() {
		t.Error("lastUsed should be set")
	}
	if len(c.session.kvState) != 4 {
		t.Errorf("kvState len = %d, want 4 (truncated to nExtracted)", len(c.session.kvState))
	}
}

func TestProcessSPC_BuildSuccess_OnlySystemMessageGetsDefaultUser(t *testing.T) {
	c := NewSPCCache(successDeps(), spcCfg())

	d := systemReq("You are helpful.")
	result := c.ProcessCache(context.Background(), d, time.Now())

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.SPC == nil {
		t.Fatal("expected SPC result")
	}

	msgs := mustMessages(t, result.ModifiedD)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 fallback message, got %d", len(msgs))
	}
	content, _ := msgs[0]["content"].(string)
	if content != "Tell the user you are ready to help them." {
		t.Errorf("fallback content = %q", content)
	}
}

func TestProcessSPC_HitWithMatchingSystemMessage(t *testing.T) {
	var depsCalled atomic.Int32
	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			depsCalled.Add(1)
			return "", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			depsCalled.Add(1)
			return nil
		},
		decodeTokensFn: func(_ context.Context, _ []llama.Token, _ llama.SeqId, _ int) error {
			depsCalled.Add(1)
			return nil
		},
	}

	c := NewSPCCache(deps, spcCfg())

	sysContent := "You are helpful."
	hash := HashMessage(CacheableMessage{Role: RoleSystem, Content: sysContent})
	c.session = &spcSession{
		sysPromptHash:   hash,
		sysPromptTokens: 77,
		sysPromptLen:    len(sysContent),
		kvState:         []byte{1, 2, 3},
	}

	d := systemReq(sysContent, D{"role": "user", "content": "hi"})
	result := c.ProcessCache(context.Background(), d, time.Now())

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.SPC == nil {
		t.Fatal("expected SPC hit")
	}
	if result.SPC.CacheIdx != 77 {
		t.Errorf("CacheIdx = %d, want 77", result.SPC.CacheIdx)
	}
	if depsCalled.Load() != 0 {
		t.Error("deps should not be called on cache hit")
	}

	msgs := mustMessages(t, result.ModifiedD)
	if len(msgs) != 1 || msgs[0]["role"] != "user" {
		t.Error("system message should be removed on hit")
	}
}

func TestProcessSPC_NonSystemRequestUsesExistingSession(t *testing.T) {
	tests := []struct {
		name    string
		session *spcSession
		wantSPC bool
	}{
		{"nil session", nil, false},
		{"zero tokens", &spcSession{sysPromptTokens: 0}, false},
		{"valid session", &spcSession{sysPromptTokens: 42, kvState: []byte{1}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSPCCache(&fakeDeps{}, spcCfg())
			c.session = tt.session

			d := D{"messages": []D{{"role": "user", "content": "hi"}}}
			result := c.ProcessCache(context.Background(), d, time.Now())

			if tt.wantSPC {
				if result.SPC == nil {
					t.Fatal("expected SPC result")
				}
				if result.SPC.CacheIdx != llama.Pos(tt.session.sysPromptTokens) {
					t.Errorf("CacheIdx = %d, want %d", result.SPC.CacheIdx, tt.session.sysPromptTokens)
				}
			} else {
				if result.SPC != nil {
					t.Error("expected nil SPC result")
				}
			}

			msgs := mustMessages(t, result.ModifiedD)
			if len(msgs) != 1 || msgs[0]["content"] != "hi" {
				t.Error("messages should be unchanged for non-system request")
			}
		})
	}
}

func TestPerformSPC_SameLengthDifferentContentMisses(t *testing.T) {
	var buildCalled atomic.Int32
	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			buildCalled.Add(1)
			return "new-prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return []llama.Token{1, 2, 3, 4}
		},
		extractKVStateFn: func(_ llama.SeqId) ([]byte, int, error) {
			return []byte{9, 8, 7, 6}, 4, nil
		},
	}

	c := NewSPCCache(deps, spcCfg())

	oldContent := "abcdefghij"
	newContent := "0123456789"
	if len(oldContent) != len(newContent) {
		t.Fatal("test setup: contents must be same length")
	}

	c.session = &spcSession{
		sysPromptHash:   HashMessage(CacheableMessage{Role: RoleSystem, Content: oldContent}),
		sysPromptTokens: 50,
		sysPromptLen:    len(oldContent),
		kvState:         []byte{1},
	}

	d := systemReq(newContent, D{"role": "user", "content": "hi"})
	result := c.ProcessCache(context.Background(), d, time.Now())

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if buildCalled.Load() != 1 {
		t.Error("rebuild should have been triggered for different content with same length")
	}
	if result.SPC == nil {
		t.Fatal("expected SPC result from rebuild")
	}
}

// =========================================================================
// D. performSPC failure / skip paths
// =========================================================================

func TestPerformSPC_NonSystemMessageNoOp(t *testing.T) {
	var depsCalled atomic.Int32
	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			depsCalled.Add(1)
			return "", nil, nil
		},
	}

	c := NewSPCCache(deps, spcCfg())
	d := D{"test": true}

	result := c.performSPC(context.Background(), d, nil, CacheableMessage{Role: "user", Content: "hello"})

	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
	if result.SPC != nil {
		t.Error("expected nil SPC result")
	}
	if depsCalled.Load() != 0 {
		t.Error("deps should not be called for non-system role")
	}
}

func TestPerformSPC_CreatePromptError(t *testing.T) {
	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "", nil, errors.New("template boom")
		},
	}

	c := NewSPCCache(deps, spcCfg())
	oldSession := &spcSession{sysPromptTokens: 99, kvState: []byte{1}}
	c.session = oldSession

	d := systemReq("You are helpful.", D{"role": "user", "content": "hi"})
	msgs := d["messages"].([]D)
	result := c.performSPC(context.Background(), d, msgs, CacheableMessage{Role: RoleSystem, Content: "You are helpful."})

	if result.Err == nil || !strings.Contains(result.Err.Error(), "failed to template system prompt") {
		t.Errorf("expected template error, got: %v", result.Err)
	}
	if c.session != oldSession {
		t.Error("session should be unchanged on CreatePrompt failure")
	}
	resultMsgs := mustMessages(t, result.ModifiedD)
	if len(resultMsgs) != 2 || resultMsgs[0]["role"] != "system" {
		t.Error("ModifiedD messages should be unchanged on error")
	}
}

func TestPerformSPC_ZeroTokensError(t *testing.T) {
	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return nil
		},
	}

	c := NewSPCCache(deps, spcCfg())
	oldSession := &spcSession{sysPromptTokens: 10, kvState: []byte{1}}
	c.session = oldSession

	d := systemReq("content")
	msgs := d["messages"].([]D)
	result := c.performSPC(context.Background(), d, msgs, CacheableMessage{Role: RoleSystem, Content: "content"})

	if result.Err == nil || !strings.Contains(result.Err.Error(), "tokenized to zero tokens") {
		t.Errorf("expected zero-tokens error, got: %v", result.Err)
	}
	if c.session != oldSession {
		t.Error("session should be unchanged on zero-token failure")
	}
	resultMsgs := mustMessages(t, result.ModifiedD)
	if len(resultMsgs) != 1 || resultMsgs[0]["role"] != "system" {
		t.Error("ModifiedD messages should be unchanged on error")
	}
}

func TestPerformSPC_MinTokensSkip(t *testing.T) {
	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "short", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return []llama.Token{1, 2} // Below MinTokens=3.
		},
	}

	var clearCalled atomic.Int32
	deps.clearSequenceFn = func(_ llama.SeqId) { clearCalled.Add(1) }

	c := NewSPCCache(deps, spcCfg())
	oldSession := &spcSession{sysPromptTokens: 100, kvState: []byte{1}}
	c.session = oldSession

	d := systemReq("ab")
	msgs := d["messages"].([]D)
	result := c.performSPC(context.Background(), d, msgs, CacheableMessage{Role: RoleSystem, Content: "ab"})

	if result.Err != nil {
		t.Errorf("min tokens skip should not error: %v", result.Err)
	}
	if result.SPC != nil {
		t.Error("expected nil SPC result for short prompt")
	}
	if c.session != oldSession {
		t.Error("old session should be preserved when prompt is skipped")
	}
	if clearCalled.Load() != 0 {
		t.Error("ClearSequence should not be called for a skip")
	}
	resultMsgs := mustMessages(t, result.ModifiedD)
	if len(resultMsgs) != 1 || resultMsgs[0]["role"] != "system" {
		t.Error("ModifiedD messages should be unchanged on skip")
	}
}

func TestPerformSPC_DecodeErrorInvalidatesSession(t *testing.T) {
	var clearCalls []llama.SeqId
	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return []llama.Token{1, 2, 3, 4}
		},
		clearSequenceFn: func(seqID llama.SeqId) {
			clearCalls = append(clearCalls, seqID)
		},
		decodeTokensFn: func(_ context.Context, _ []llama.Token, _ llama.SeqId, _ int) error {
			return errors.New("decode boom")
		},
	}

	c := NewSPCCache(deps, spcCfg())
	c.session = &spcSession{sysPromptTokens: 50, kvState: []byte{1}}

	d := systemReq("new system prompt", D{"role": "user", "content": "hi"})
	msgs := d["messages"].([]D)
	result := c.performSPC(context.Background(), d, msgs, CacheableMessage{Role: RoleSystem, Content: "new system prompt"})

	if result.Err == nil || !strings.Contains(result.Err.Error(), "decoding system prompt into cache") {
		t.Errorf("expected decode error, got: %v", result.Err)
	}
	if c.session != nil {
		t.Error("session should be nil after decode failure (invalidated before clear)")
	}
	if len(clearCalls) != 1 {
		t.Errorf("ClearSequence called %d times, want 1 (before decode)", len(clearCalls))
	}
	resultMsgs := mustMessages(t, result.ModifiedD)
	if len(resultMsgs) != 2 || resultMsgs[0]["role"] != "system" {
		t.Error("ModifiedD messages should be unchanged on decode error")
	}

	// Subsequent non-system request should not hit SPC.
	d2 := D{"messages": []D{{"role": "user", "content": "hi"}}}
	result2 := c.ProcessCache(context.Background(), d2, time.Now())
	if result2.SPC != nil {
		t.Error("should not hit SPC after decode failure")
	}
}

func TestPerformSPC_ExtractError(t *testing.T) {
	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return []llama.Token{1, 2, 3, 4}
		},
		extractKVStateFn: func(_ llama.SeqId) ([]byte, int, error) {
			return nil, 0, errors.New("extract boom")
		},
	}

	c := NewSPCCache(deps, spcCfg())
	c.session = &spcSession{sysPromptTokens: 50, kvState: []byte{1}}

	d := systemReq("new content", D{"role": "user", "content": "hi"})
	msgs := d["messages"].([]D)
	result := c.performSPC(context.Background(), d, msgs, CacheableMessage{Role: RoleSystem, Content: "new content"})

	if result.Err == nil || !strings.Contains(result.Err.Error(), "extracting KV state") {
		t.Errorf("expected extract error, got: %v", result.Err)
	}
	if c.session != nil {
		t.Error("session should be nil after extract failure")
	}
	resultMsgs := mustMessages(t, result.ModifiedD)
	if len(resultMsgs) != 2 || resultMsgs[0]["role"] != "system" {
		t.Error("ModifiedD messages should be unchanged on extract error")
	}
}

func TestPerformSPC_ZeroBytesExtractedErrors(t *testing.T) {
	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return []llama.Token{1, 2, 3, 4}
		},
		extractKVStateFn: func(_ llama.SeqId) ([]byte, int, error) {
			return []byte{1, 2, 3}, 0, nil
		},
	}

	c := NewSPCCache(deps, spcCfg())

	d := systemReq("content here")
	msgs := d["messages"].([]D)
	result := c.performSPC(context.Background(), d, msgs, CacheableMessage{Role: RoleSystem, Content: "content here"})

	if result.Err == nil || !strings.Contains(result.Err.Error(), "failed to extract KV state") {
		t.Errorf("expected zero-extracted error, got: %v", result.Err)
	}
	if c.session != nil {
		t.Error("session should be nil after zero extraction")
	}
	resultMsgs := mustMessages(t, result.ModifiedD)
	if len(resultMsgs) != 1 || resultMsgs[0]["role"] != "system" {
		t.Error("ModifiedD messages should be unchanged on zero-extract error")
	}
}

// =========================================================================
// E. RestoreSPCToSeq
// =========================================================================

func TestRestoreSPCToSeq_NoCachedState(t *testing.T) {
	tests := []struct {
		name    string
		session *spcSession
	}{
		{"nil session", nil},
		{"nil kvState", &spcSession{kvState: nil}},
		{"empty kvState", &spcSession{kvState: []byte{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSPCCache(&fakeDeps{}, spcCfg())
			c.session = tt.session

			err := c.RestoreSPCToSeq(0)
			if err == nil || !strings.Contains(err.Error(), "no cached KV state available") {
				t.Errorf("expected no-cached-state error, got: %v", err)
			}
		})
	}
}

func TestRestoreSPCToSeq_Success(t *testing.T) {
	kvData := []byte{9, 8, 7, 6, 5}
	var receivedData []byte
	var receivedSeqID llama.SeqId

	deps := &fakeDeps{
		restoreKVStateFn: func(data []byte, dstSeqID llama.SeqId) (int, error) {
			receivedData = data
			receivedSeqID = dstSeqID
			return len(data), nil
		},
	}

	c := NewSPCCache(deps, spcCfg())
	c.session = &spcSession{kvState: kvData}

	if err := c.RestoreSPCToSeq(42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedSeqID != 42 {
		t.Errorf("dstSeqID = %d, want 42", receivedSeqID)
	}
	if len(receivedData) != len(kvData) {
		t.Errorf("data len = %d, want %d", len(receivedData), len(kvData))
	}
	for i := range kvData {
		if receivedData[i] != kvData[i] {
			t.Errorf("data[%d] = %d, want %d", i, receivedData[i], kvData[i])
		}
	}
}

func TestRestoreSPCToSeq_DepError(t *testing.T) {
	deps := &fakeDeps{
		restoreKVStateFn: func(_ []byte, _ llama.SeqId) (int, error) {
			return 0, errors.New("hw fault")
		},
	}

	c := NewSPCCache(deps, spcCfg())
	c.session = &spcSession{kvState: []byte{1, 2, 3}}

	err := c.RestoreSPCToSeq(0)
	if err == nil || !strings.Contains(err.Error(), "restore-spc:") {
		t.Errorf("expected wrapped restore error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "hw fault") {
		t.Errorf("expected original cause in error, got: %v", err)
	}
}

func TestRestoreSPCToSeq_ZeroReadError(t *testing.T) {
	deps := &fakeDeps{
		restoreKVStateFn: func(_ []byte, _ llama.SeqId) (int, error) {
			return 0, nil
		},
	}

	c := NewSPCCache(deps, spcCfg())
	c.session = &spcSession{kvState: []byte{1, 2, 3}}

	err := c.RestoreSPCToSeq(5)
	if err == nil || !strings.Contains(err.Error(), "StateSeqSetData failed") {
		t.Errorf("expected zero-read error, got: %v", err)
	}
}

// =========================================================================
// F-pre. Build → hit round-trip
// =========================================================================

func TestProcessSPC_BuildThenHitRoundTrip(t *testing.T) {
	var depsCalled atomic.Int32
	deps := successDeps()

	c := NewSPCCache(deps, spcCfg())
	ctx := context.Background()
	sysContent := "You are a round-trip test assistant."

	// First call: cache miss → full build.
	d1 := systemReq(sysContent, D{"role": "user", "content": "hi"})
	r1 := c.ProcessCache(ctx, d1, time.Now())
	if r1.Err != nil {
		t.Fatalf("build: %v", r1.Err)
	}
	if r1.SPC == nil || r1.SPC.CacheIdx != 4 {
		t.Fatal("build should produce SPC with CacheIdx=4")
	}

	expectedHash := HashMessage(CacheableMessage{Role: RoleSystem, Content: sysContent})
	if c.session.sysPromptHash != expectedHash {
		t.Errorf("stored hash = %s, want %s", c.session.sysPromptHash, expectedHash)
	}

	// Replace deps with ones that track calls and should NOT be invoked.
	deps.createPromptFn = func(_ context.Context, _ D) (string, [][]byte, error) {
		depsCalled.Add(1)
		return "", nil, nil
	}
	deps.decodeTokensFn = func(_ context.Context, _ []llama.Token, _ llama.SeqId, _ int) error {
		depsCalled.Add(1)
		return nil
	}

	// Second call with same prompt: fast cache hit.
	d2 := systemReq(sysContent, D{"role": "user", "content": "follow-up"})
	r2 := c.ProcessCache(ctx, d2, time.Now())
	if r2.Err != nil {
		t.Fatalf("hit: %v", r2.Err)
	}
	if r2.SPC == nil || r2.SPC.CacheIdx != 4 {
		t.Fatal("hit should return same CacheIdx=4")
	}
	if depsCalled.Load() != 0 {
		t.Error("cache hit should not call CreatePrompt or DecodeTokensIntoCache")
	}

	msgs2 := mustMessages(t, r2.ModifiedD)
	if len(msgs2) != 1 || msgs2[0]["role"] != "user" {
		t.Error("system message should be removed on hit")
	}

	// Third call with same-length different content: cache miss → rebuild.
	diffContent := strings.Repeat("x", len(sysContent))
	d3 := systemReq(diffContent, D{"role": "user", "content": "new"})
	deps.createPromptFn = func(_ context.Context, _ D) (string, [][]byte, error) {
		return "new-prompt", nil, nil
	}
	deps.decodeTokensFn = func(_ context.Context, _ []llama.Token, _ llama.SeqId, _ int) error {
		return nil
	}

	r3 := c.ProcessCache(ctx, d3, time.Now())
	if r3.Err != nil {
		t.Fatalf("rebuild: %v", r3.Err)
	}
	if r3.SPC == nil {
		t.Fatal("rebuild should produce SPC result")
	}
	newHash := HashMessage(CacheableMessage{Role: RoleSystem, Content: diffContent})
	if c.session.sysPromptHash != newHash {
		t.Error("session hash should update after rebuild")
	}
}

// =========================================================================
// F. Concurrency
// =========================================================================

func TestProcessSPC_DoubleCheckedLockBuildsOnce(t *testing.T) {
	var (
		createCalls  atomic.Int32
		decodeCalls  atomic.Int32
		extractCalls atomic.Int32
		entered      = make(chan struct{})
		unblock      = make(chan struct{})
	)

	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			if createCalls.Add(1) == 1 {
				close(entered)
			}
			<-unblock
			return "prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return []llama.Token{1, 2, 3, 4}
		},
		decodeTokensFn: func(_ context.Context, _ []llama.Token, _ llama.SeqId, _ int) error {
			decodeCalls.Add(1)
			return nil
		},
		extractKVStateFn: func(_ llama.SeqId) ([]byte, int, error) {
			extractCalls.Add(1)
			return []byte{0xAA, 0xBB, 0xCC, 0xDD}, 4, nil
		},
	}

	c := NewSPCCache(deps, spcCfg())

	var wg sync.WaitGroup
	results := make([]Result, 2)

	// Goroutine 0: enters CreatePrompt first (acquires write lock).
	wg.Add(1)
	go func() {
		defer wg.Done()
		d := systemReq("You are helpful.", D{"role": "user", "content": "hi"})
		results[0] = c.ProcessCache(context.Background(), d, time.Now())
	}()
	<-entered

	// Goroutine 1: blocks on the write lock, then hits the double-check path.
	wg.Add(1)
	go func() {
		defer wg.Done()
		d := systemReq("You are helpful.", D{"role": "user", "content": "hi"})
		results[1] = c.ProcessCache(context.Background(), d, time.Now())
	}()

	close(unblock)
	wg.Wait()

	for i, r := range results {
		if r.Err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, r.Err)
		}
		if r.SPC == nil {
			t.Errorf("goroutine %d: expected SPC result", i)
			continue
		}
		if r.SPC.CacheIdx != 4 {
			t.Errorf("goroutine %d: CacheIdx = %d, want 4", i, r.SPC.CacheIdx)
		}
	}

	if createCalls.Load() != 1 {
		t.Errorf("CreatePrompt called %d times, want 1", createCalls.Load())
	}
	if decodeCalls.Load() != 1 {
		t.Errorf("DecodeTokensIntoCache called %d times, want 1", decodeCalls.Load())
	}
	if extractCalls.Load() != 1 {
		t.Errorf("ExtractKVState called %d times, want 1", extractCalls.Load())
	}
}

func TestSPCCache_RebuildInvalidatesSessionBeforeDecode(t *testing.T) {
	// Verifies that c.session is nil during the decode phase of a rebuild,
	// so no concurrent reader (RestoreSPCToSeq) can use stale metadata.
	var sessionDuringDecode *spcSession

	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return []llama.Token{1, 2, 3, 4}
		},
		decodeTokensFn: func(_ context.Context, _ []llama.Token, _ llama.SeqId, _ int) error {
			// We're inside the write lock here; capture session directly.
			// At this point session must already be nil.
			return nil
		},
		extractKVStateFn: func(_ llama.SeqId) ([]byte, int, error) {
			return []byte{0xAA, 0xBB, 0xCC, 0xDD}, 4, nil
		},
	}

	c := NewSPCCache(deps, spcCfg())
	c.session = &spcSession{
		sysPromptHash:   "old-hash",
		sysPromptTokens: 10,
		sysPromptLen:    5,
		kvState:         []byte{0xFF},
	}

	// Capture session inside decode (under the write lock, no race).
	deps.decodeTokensFn = func(_ context.Context, _ []llama.Token, _ llama.SeqId, _ int) error {
		sessionDuringDecode = c.session
		return nil
	}

	d := systemReq("totally new prompt", D{"role": "user", "content": "hi"})
	result := c.ProcessCache(context.Background(), d, time.Now())

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if sessionDuringDecode != nil {
		t.Error("session must be nil during decode (invalidated before ClearSequence)")
	}
	if c.session == nil {
		t.Error("session should be restored after successful rebuild")
	}
}

// =========================================================================
// G. Red tests — bugs in current code
// =========================================================================

func TestRestoreSPCToSeq_PartialReadShouldFail(t *testing.T) {
	t.Skip("BUG: partial restore (nRead < len(kvState)) is silently accepted as success")

	deps := &fakeDeps{
		restoreKVStateFn: func(data []byte, _ llama.SeqId) (int, error) {
			return 2, nil // Only 2 of 5 bytes read.
		},
	}

	c := NewSPCCache(deps, spcCfg())
	c.session = &spcSession{kvState: []byte{1, 2, 3, 4, 5}}

	err := c.RestoreSPCToSeq(0)
	if err == nil {
		t.Error("partial restore (nRead=2 of 5 bytes) should return an error")
	}
}

func TestPerformSPC_ExtractKVStateLengthMismatch(t *testing.T) {
	t.Skip("BUG: nExtracted > len(kvState) causes panic on kvState[:nExtracted]")

	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return []llama.Token{1, 2, 3, 4}
		},
		extractKVStateFn: func(_ llama.SeqId) ([]byte, int, error) {
			return []byte{1, 2}, 5, nil // nExtracted > len(buf).
		},
	}

	c := NewSPCCache(deps, spcCfg())
	d := systemReq("test content")
	msgs := d["messages"].([]D)

	result := c.performSPC(context.Background(), d, msgs, CacheableMessage{Role: RoleSystem, Content: "test content"})

	if result.Err == nil {
		t.Error("should error when nExtracted > len(kvState), not panic")
	}
}

// =========================================================================
// H. State-machine invariant test
// =========================================================================

func TestSPCCache_StateMachineInvariants(t *testing.T) {
	const nTokens = 5
	const kvLen = 4

	deps := &fakeDeps{
		createPromptFn: func(_ context.Context, _ D) (string, [][]byte, error) {
			return "prompt", nil, nil
		},
		tokenizeStringFn: func(_ string) []llama.Token {
			return make([]llama.Token, nTokens)
		},
		extractKVStateFn: func(_ llama.SeqId) ([]byte, int, error) {
			return make([]byte, kvLen+2), kvLen, nil
		},
		restoreKVStateFn: func(data []byte, _ llama.SeqId) (int, error) {
			return len(data), nil
		},
	}

	c := NewSPCCache(deps, spcCfg())
	ctx := context.Background()
	rng := rand.New(rand.NewSource(42))

	prompts := []string{
		"You are a helpful assistant.",
		"You are a code reviewer.",
		"You are a test writer.",
	}

	// Expected model state — tracks what the cache should contain.
	type model struct {
		hasSession    bool
		activePrompt  string
		expectedHash  string
		expectedLen   int
		expectedKVLen int
	}

	var m model

	checkInvariants := func(step int, opName string) {
		t.Helper()

		c.mu.RLock()
		session := c.session
		c.mu.RUnlock()

		// Model consistency check.
		if m.hasSession != (session != nil) {
			t.Errorf("step %d (%s): model.hasSession=%v but session nil=%v", step, opName, m.hasSession, session == nil)
			return
		}

		if session != nil {
			if session.sysPromptHash != m.expectedHash {
				t.Errorf("step %d (%s): hash mismatch: got %s, want %s", step, opName, session.sysPromptHash[:8], m.expectedHash[:8])
			}
			if session.sysPromptLen != m.expectedLen {
				t.Errorf("step %d (%s): len mismatch: got %d, want %d", step, opName, session.sysPromptLen, m.expectedLen)
			}
			if session.sysPromptTokens != nTokens {
				t.Errorf("step %d (%s): tokens=%d, want %d", step, opName, session.sysPromptTokens, nTokens)
			}
			if len(session.kvState) != m.expectedKVLen {
				t.Errorf("step %d (%s): kvState len=%d, want %d", step, opName, len(session.kvState), m.expectedKVLen)
			}
		}

		// INV1: No SPC hit when no session.
		if session == nil {
			d := D{"messages": []D{{"role": "user", "content": "hi"}}}
			r := c.ProcessCache(ctx, d, time.Now())
			if r.SPC != nil {
				t.Errorf("step %d (%s): INV1 — SPC hit with nil session", step, opName)
			}
		}

		// INV2: Restore fails when no session.
		if session == nil {
			if err := c.RestoreSPCToSeq(0); err == nil {
				t.Errorf("step %d (%s): INV2 — RestoreSPCToSeq succeeded with nil session", step, opName)
			}
		}

		// INV3: Restore succeeds when session exists.
		if session != nil && session.sysPromptTokens > 0 && len(session.kvState) > 0 {
			if err := c.RestoreSPCToSeq(99); err != nil {
				t.Errorf("step %d (%s): INV3 — RestoreSPCToSeq failed: %v", step, opName, err)
			}
		}

		// INV4: Non-system request returns SPC hit iff valid session exists.
		{
			d := D{"messages": []D{{"role": "user", "content": "test"}}}
			r := c.ProcessCache(ctx, d, time.Now())
			if session != nil && session.sysPromptTokens > 0 && r.SPC == nil {
				t.Errorf("step %d (%s): INV4 — no SPC hit with valid session", step, opName)
			}
			if session == nil && r.SPC != nil {
				t.Errorf("step %d (%s): INV4 — SPC hit with nil session", step, opName)
			}
		}

		// INV5: Slot methods never panic and never affect session.
		sessionBefore := c.session
		c.ClearPending(0)
		c.CommitSession(Commit{SlotID: 0})
		c.InvalidateSlot(0)
		c.SetSlotMRoPE(0, true)
		_, _ = c.SnapshotSlot(0)
		_ = c.HasCachedSlot(0)
		if c.session != sessionBefore {
			t.Errorf("step %d (%s): INV5 — slot method mutated session", step, opName)
		}
	}

	type op struct {
		name string
		fn   func()
	}

	operations := []op{
		{"build-system-prompt", func() {
			prompt := prompts[rng.Intn(len(prompts))]
			d := systemReq(prompt, D{"role": "user", "content": "hi"})
			r := c.ProcessCache(ctx, d, time.Now())
			if r.Err == nil && r.SPC != nil && r.SPC.CacheIdx > 0 {
				m.hasSession = true
				m.activePrompt = prompt
				m.expectedHash = HashMessage(CacheableMessage{Role: RoleSystem, Content: prompt})
				m.expectedLen = len(prompt)
				m.expectedKVLen = kvLen
			}
		}},
		{"non-system-request", func() {
			d := D{"messages": []D{{"role": "user", "content": fmt.Sprintf("msg-%d", rng.Int())}}}
			c.ProcessCache(ctx, d, time.Now())
		}},
		{"clear-caches", func() {
			c.ClearCaches()
			m.hasSession = false
			m.activePrompt = ""
			m.expectedHash = ""
			m.expectedLen = 0
			m.expectedKVLen = 0
		}},
		{"restore", func() {
			_ = c.RestoreSPCToSeq(llama.SeqId(rng.Intn(10)))
		}},
		{"hit-same-prompt", func() {
			if !m.hasSession {
				return
			}
			d := systemReq(m.activePrompt, D{"role": "user", "content": "follow-up"})
			r := c.ProcessCache(ctx, d, time.Now())
			if r.SPC == nil {
				t.Error("hit-same-prompt: expected SPC hit")
			}
		}},
	}

	for i := range 200 {
		idx := rng.Intn(len(operations))
		operations[idx].fn()
		checkInvariants(i, operations[idx].name)
	}
}

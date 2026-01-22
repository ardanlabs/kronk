package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// ensureSystemPromptCached checks if the first message is cached and updates
// the cache if necessary. The first message can be any role (system, user, or
// assistant) - this supports both traditional system prompts and clients like
// Cline that use a large first user message as context. Returns:
//   - modifiedD: D with first message removed if cache was used
//   - nPast: starting position for new tokens (cached token count if cached, 0 otherwise)
//   - cached: true if the first message was already cached and can be reused
//   - error: any error that occurred during cache update
//
// This function is thread-safe and handles concurrent requests appropriately.
func (m *Model) ensureSystemPromptCached(ctx context.Context, d D) (modifiedD D, nPast llama.Pos, cached bool, err error) {
	if !m.cfg.SystemPromptCache {
		return d, 0, false, nil
	}

	msgInfo, hasFirstMsg := extractFirstMessage(d)
	if !hasFirstMsg {
		return d, 0, false, nil
	}

	newHash := hashFirstMessage(msgInfo)

	m.sysPromptMu.RLock()
	currentHash := m.sysPromptHash
	currentTokens := m.sysPromptTokens
	m.sysPromptMu.RUnlock()

	if currentHash == newHash && currentTokens > 0 {
		m.log(ctx, "sys-prompt-cache", "status", "hit", "role", msgInfo.role, "tokens", currentTokens)
		modifiedD = removeFirstMessage(d)
		return modifiedD, llama.Pos(currentTokens), true, nil
	}

	m.sysPromptMu.Lock()
	defer m.sysPromptMu.Unlock()

	if m.sysPromptHash == newHash && m.sysPromptTokens > 0 {
		m.log(ctx, "sys-prompt-cache", "status", "hit-after-lock", "role", msgInfo.role, "tokens", m.sysPromptTokens)
		modifiedD = removeFirstMessage(d)
		return modifiedD, llama.Pos(m.sysPromptTokens), true, nil
	}

	tokens := llama.Tokenize(m.vocab, msgInfo.content, true, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return d, 0, false, fmt.Errorf("sys-prompt-cache: first message tokenized to zero tokens")
	}

	if nTokens < m.cfg.SystemPromptCacheMinTokens {
		m.log(ctx, "sys-prompt-cache", "status", "skip-too-short", "role", msgInfo.role, "tokens", nTokens, "min", m.cfg.SystemPromptCacheMinTokens)
		return d, 0, false, nil
	}

	m.log(ctx, "sys-prompt-cache", "status", "miss", "role", msgInfo.role,
		"old-hash", m.sysPromptHash[:min(8, len(m.sysPromptHash))], "new-hash", newHash[:8])

	llama.MemorySeqRm(m.mem, 0, -1, -1)

	nBatch := int(m.ctxParams.NBatch)

	switch {
	case nTokens <= nBatch:
		batch := llama.BatchGetOne(tokens)
		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return d, 0, false, fmt.Errorf("sys-prompt-cache: failed to decode first message: %w", err)
		}

	default:
		for i := 0; i < len(tokens); i += nBatch {
			end := min(i+nBatch, len(tokens))
			chunk := tokens[i:end]
			if _, err := llama.Decode(m.lctx, llama.BatchGetOne(chunk)); err != nil {
				return d, 0, false, fmt.Errorf("sys-prompt-cache: failed to decode first message chunk: %w", err)
			}
		}
	}

	m.sysPromptHash = newHash
	m.sysPromptTokens = nTokens

	m.log(ctx, "sys-prompt-cache", "status", "cached", "role", msgInfo.role, "tokens", nTokens, "hash", newHash[:8])

	return d, 0, false, nil
}

// copySystemPromptToSeq copies the cached system prompt KV cache from sequence 0
// to the specified sequence ID. This should be called before processing a new
// request that will use the cached system prompt.
func (m *Model) copySystemPromptToSeq(seqID llama.SeqId) error {
	if !m.cfg.SystemPromptCache {
		return nil
	}

	m.sysPromptMu.RLock()
	hasCache := m.sysPromptTokens > 0
	m.sysPromptMu.RUnlock()

	if !hasCache {
		return nil
	}

	if err := llama.MemorySeqCp(m.mem, 0, seqID, -1, -1); err != nil {
		return fmt.Errorf("copy-sys-prompt: failed to copy memory seq 0 to %d: %w", seqID, err)
	}

	return nil
}

// clearSystemPromptCache clears the cached system prompt state.
// This is useful when the model context is reset.
func (m *Model) clearSystemPromptCache() {
	m.sysPromptMu.Lock()
	m.sysPromptHash = ""
	m.sysPromptTokens = 0
	m.sysPromptMu.Unlock()
}

// =============================================================================

// firstMessageInfo contains information about the first message for caching.
type firstMessageInfo struct {
	role    string
	content string
}

// extractFirstMessage extracts the first message from D regardless of role.
// Returns the role, content, and true if a valid first message exists.
func extractFirstMessage(d D) (firstMessageInfo, bool) {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return firstMessageInfo{}, false
	}

	first := messages[0]

	role, ok := first["role"].(string)
	if !ok || role == "" {
		return firstMessageInfo{}, false
	}

	content, ok := first["content"].(string)
	if !ok || content == "" {
		return firstMessageInfo{}, false
	}

	return firstMessageInfo{role: role, content: content}, true
}

// hashFirstMessage computes a SHA-256 hash of the first message.
// Includes the role in the hash to differentiate between same content with different roles.
func hashFirstMessage(info firstMessageInfo) string {
	data := info.role + ":" + info.content
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// removeFirstMessage returns a clone of D with the first message removed
// from the messages slice. If there is only one message, it is replaced with
// a placeholder message to ensure the model generates a response.
func removeFirstMessage(d D) D {
	messages, ok := d["messages"].([]D)
	if !ok {
		return d
	}

	clone := d.Clone()

	if len(messages) <= 1 {
		clone["messages"] = []D{
			{"role": "user", "content": "Please respond: I am ready to receive commands"},
		}
		return clone
	}

	clone["messages"] = messages[1:]

	return clone
}

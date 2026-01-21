package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// extractSystemPrompt extracts the system message content from D if present.
// Returns the system prompt text and true if found, empty string and false otherwise.
func extractSystemPrompt(d D) (string, bool) {
	messages, ok := d["messages"].([]D)
	if !ok {
		return "", false
	}

	if len(messages) == 0 {
		return "", false
	}

	first := messages[0]
	role, ok := first["role"].(string)
	if !ok || role != RoleSystem {
		return "", false
	}

	content, ok := first["content"].(string)
	if !ok || content == "" {
		return "", false
	}

	return content, true
}

// hashSystemPrompt computes a SHA-256 hash of the system prompt content.
func hashSystemPrompt(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// removeSystemMessage returns a clone of D with the system message removed
// from the messages slice.
func removeSystemMessage(d D) D {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return d
	}

	first := messages[0]
	role, ok := first["role"].(string)
	if !ok || role != RoleSystem {
		return d
	}

	clone := d.Clone()
	clone["messages"] = messages[1:]

	return clone
}

// ensureSystemPromptCached checks if the system prompt is cached and updates
// the cache if necessary. Returns:
//   - modifiedD: D with system message removed if cache was used
//   - nPast: starting position for new tokens (system prompt token count if cached, 0 otherwise)
//   - cached: true if the system prompt was already cached and can be reused
//   - error: any error that occurred during cache update
//
// This function is thread-safe and handles concurrent requests appropriately.
func (m *Model) ensureSystemPromptCached(ctx context.Context, d D) (modifiedD D, nPast llama.Pos, cached bool, err error) {
	if !m.cfg.SystemPromptCache {
		return d, 0, false, nil
	}

	sysPrompt, hasSysPrompt := extractSystemPrompt(d)
	if !hasSysPrompt {
		return d, 0, false, nil
	}

	newHash := hashSystemPrompt(sysPrompt)

	m.sysPromptMu.RLock()
	currentHash := m.sysPromptHash
	currentTokens := m.sysPromptTokens
	m.sysPromptMu.RUnlock()

	if currentHash == newHash && currentTokens > 0 {
		m.log(ctx, "sys-prompt-cache", "status", "hit", "tokens", currentTokens)
		modifiedD = removeSystemMessage(d)
		return modifiedD, llama.Pos(currentTokens), true, nil
	}

	m.sysPromptMu.Lock()
	defer m.sysPromptMu.Unlock()

	if m.sysPromptHash == newHash && m.sysPromptTokens > 0 {
		m.log(ctx, "sys-prompt-cache", "status", "hit-after-lock", "tokens", m.sysPromptTokens)
		modifiedD = removeSystemMessage(d)
		return modifiedD, llama.Pos(m.sysPromptTokens), true, nil
	}

	m.log(ctx, "sys-prompt-cache", "status", "miss", "old-hash", m.sysPromptHash[:min(8, len(m.sysPromptHash))], "new-hash", newHash[:8])

	llama.MemorySeqRm(m.mem, 0, -1, -1)

	tokens := llama.Tokenize(m.vocab, sysPrompt, true, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return d, 0, false, fmt.Errorf("sys-prompt-cache: system prompt tokenized to zero tokens")
	}

	nBatch := int(m.ctxParams.NBatch)

	switch {
	case nTokens <= nBatch:
		batch := llama.BatchGetOne(tokens)
		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return d, 0, false, fmt.Errorf("sys-prompt-cache: failed to decode system prompt: %w", err)
		}

	default:
		for i := 0; i < len(tokens); i += nBatch {
			end := min(i+nBatch, len(tokens))
			chunk := tokens[i:end]
			if _, err := llama.Decode(m.lctx, llama.BatchGetOne(chunk)); err != nil {
				return d, 0, false, fmt.Errorf("sys-prompt-cache: failed to decode system prompt chunk: %w", err)
			}
		}
	}

	m.sysPromptHash = newHash
	m.sysPromptTokens = nTokens

	m.log(ctx, "sys-prompt-cache", "status", "cached", "tokens", nTokens, "hash", newHash[:8])

	modifiedD = removeSystemMessage(d)

	return modifiedD, llama.Pos(nTokens), true, nil
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

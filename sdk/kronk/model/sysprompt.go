package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// cacheResult contains the results of cache processing.
type cacheResult struct {
	modifiedD D         // D with first message removed if cache was used
	prompt    string    // Templated prompt (set when caching occurs)
	media     [][]byte  // Media from templating (set when caching occurs)
	nPast     llama.Pos // Starting position for new tokens
	cached    bool      // True if cache is being used
	err       error     // Any error that occurred
}

// ensureFirstMessageCached checks if the first message (or system prompt) is
// cached and updates the cache if necessary. The behavior depends on which
// cache mode is enabled:
//
//   - SystemPromptCache: Caches messages with role="system". Only uses the cache
//     when there are multiple messages (system + user). Single-message requests
//     are processed normally and populate the cache for subsequent requests.
//   - FirstMessageCache: Caches messages with role="user". The first request
//     (single message) is templated, cached, and the prompt is returned. Subsequent
//     requests with the same first message use the cache and skip prefill.
//
// Returns a cacheResult containing:
//   - modifiedD: D with first message removed if cache was used
//   - prompt: The templated prompt (only set when this function handles templating)
//   - media: Media bytes from templating (only set when this function handles templating)
//   - nPast: starting position for new tokens (cached token count if cached, 0 otherwise)
//   - cached: true if the first message was already cached and can be reused
//   - err: any error that occurred during cache update
//
// This function is thread-safe and handles concurrent requests appropriately.
func (m *Model) ensureFirstMessageCached(ctx context.Context, d D) cacheResult {
	if !m.cfg.SystemPromptCache && !m.cfg.FirstMessageCache {
		return cacheResult{modifiedD: d}
	}

	msgInfo, hasFirstMsg := extractFirstMessage(d)
	if !hasFirstMsg {
		return cacheResult{modifiedD: d}
	}

	// -------------------------------------------------------------------------
	// SystemPromptCache mode

	if m.cfg.SystemPromptCache {
		return m.handleSystemPromptCache(ctx, d, msgInfo)
	}

	// -------------------------------------------------------------------------
	// FirstMessageCache mode

	return m.handleFirstMessageCache(ctx, d, msgInfo)
}

// handleSystemPromptCache handles caching for system prompt mode.
// This mode templates the system message with add_generation_prompt=false so
// the cached tokens form a valid prefix for subsequent multi-message requests.
func (m *Model) handleSystemPromptCache(ctx context.Context, d D, msgInfo firstMessageInfo) cacheResult {
	// Only cache system role messages.
	// If first message is not system but we have a cached system prompt, use it.
	// This handles clients that don't resend the system prompt on subsequent requests.
	if msgInfo.role != RoleSystem {
		m.sysPromptMu.RLock()
		cachedTokens := m.sysPromptTokens
		m.sysPromptMu.RUnlock()

		if cachedTokens > 0 {
			m.log(ctx, "cache", "status", "hit-no-system-prompt", "first-role", msgInfo.role, "tokens", cachedTokens)
			return cacheResult{modifiedD: d, nPast: llama.Pos(cachedTokens), cached: true}
		}

		m.log(ctx, "cache", "status", "no-system-prompt-detected", "first-role", msgInfo.role)
		return cacheResult{modifiedD: d}
	}

	return m.cacheFirstMessage(ctx, d, msgInfo, false)
}

// handleFirstMessageCache handles caching for first user message mode.
// This mode templates the first message with add_generation_prompt=false so
// the cached tokens form a valid prefix for subsequent multi-message requests.
func (m *Model) handleFirstMessageCache(ctx context.Context, d D, msgInfo firstMessageInfo) cacheResult {
	// Only cache user role messages.
	if msgInfo.role != RoleUser {
		m.log(ctx, "cache", "status", "no-user-prompt-detected", "first-role", msgInfo.role)
		return cacheResult{modifiedD: d}
	}

	return m.cacheFirstMessage(ctx, d, msgInfo, true)
}

// cacheFirstMessage is the common caching logic used by both SystemPromptCache
// and FirstMessageCache modes. It handles:
//   - Checking for cache hits when cache is populated
//   - Templating and caching the first message when cache is empty
//   - Returning the suffix prompt for immediate use after caching
//
// The requireSingleMessage parameter controls caching behavior:
//   - false (SystemPromptCache): Cache on any request where cache is empty
//   - true (FirstMessageCache): Only cache when there's exactly 1 message
func (m *Model) cacheFirstMessage(ctx context.Context, d D, msgInfo firstMessageInfo, requireSingleMessage bool) cacheResult {
	messages, _ := d["messages"].([]D)
	newHash := hashFirstMessage(msgInfo)

	// -------------------------------------------------------------------------
	// Check for cache hit (fast path with read lock).

	m.sysPromptMu.RLock()
	currentHash := m.sysPromptHash
	currentTokens := m.sysPromptTokens
	m.sysPromptMu.RUnlock()

	if currentHash == newHash && currentTokens > 0 {
		m.log(ctx, "cache", "status", "hit", "role", msgInfo.role, "tokens", currentTokens, "messages", len(messages))
		return cacheResult{modifiedD: removeFirstMessage(d), nPast: llama.Pos(currentTokens), cached: true}
	}

	// -------------------------------------------------------------------------
	// Cache miss - template and cache the first message.

	// For FirstMessageCache mode, only cache when there's exactly 1 message.
	// This matches the Cline pattern where the first request has just the context.
	if requireSingleMessage && len(messages) > 1 {
		m.log(ctx, "cache", "status", "miss-multi", "role", msgInfo.role, "messages", len(messages))
		return cacheResult{modifiedD: d}
	}

	m.sysPromptMu.Lock()
	defer m.sysPromptMu.Unlock()

	// Double-check in case another goroutine cached while we waited.
	if m.sysPromptHash == newHash && m.sysPromptTokens > 0 {
		m.log(ctx, "cache", "status", "hit-after-lock", "role", msgInfo.role, "tokens", m.sysPromptTokens)
		return cacheResult{modifiedD: d, nPast: llama.Pos(m.sysPromptTokens), cached: true}
	}

	// Template just the first message WITHOUT add_generation_prompt.
	// This creates a prompt that is a valid prefix for subsequent requests.
	firstMsgD := D{
		"messages":              []D{messages[0]},
		"add_generation_prompt": false,
	}

	// Copy tools if present (affects template output).
	if tools, ok := d["tools"]; ok {
		firstMsgD["tools"] = tools
	}

	firstMsgPrompt, _, err := m.createPrompt(ctx, firstMsgD)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: failed to template first message: %w", err)}
	}

	tokens := llama.Tokenize(m.vocab, firstMsgPrompt, true, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: first message tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "cache", "status", "skip-too-short", "role", msgInfo.role, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	m.log(ctx, "cache", "status", "miss", "role", msgInfo.role,
		"old-hash", m.sysPromptHash[:min(8, len(m.sysPromptHash))], "new-hash", newHash[:8])

	if err := m.decodeTokensToSeq0(ctx, tokens); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	m.sysPromptHash = newHash
	m.sysPromptTokens = nTokens

	m.log(ctx, "cache", "status", "cached", "role", msgInfo.role, "tokens", nTokens, "hash", newHash[:8])

	// Get the full prompt to extract just the suffix (generation prompt).
	// The cached prefix is already in seq 0, we only need to add the suffix.
	fullPrompt, fullMedia, err := m.createPrompt(ctx, d)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: failed to template full message: %w", err)}
	}

	// Extract only the suffix (the part after the cached prefix).
	// This is typically just the assistant role marker for generation.
	// If extraction fails, use a sensible default that works with most templates.
	suffix := "<|im_start|>assistant\n"
	if len(fullPrompt) > len(firstMsgPrompt) {
		suffix = fullPrompt[len(firstMsgPrompt):]
	}

	m.log(ctx, "cache", "suffix-len", len(suffix))

	return cacheResult{
		modifiedD: d,
		prompt:    suffix,
		media:     fullMedia,
		nPast:     llama.Pos(nTokens),
		cached:    true,
	}
}

// decodeTokensToSeq0 decodes tokens into sequence 0 for caching.
func (m *Model) decodeTokensToSeq0(ctx context.Context, tokens []llama.Token) error {
	llama.MemorySeqRm(m.mem, 0, -1, -1)

	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	m.log(ctx, "cache", "status", "decoding-started", "tokens", nTokens)

	switch {
	case nTokens <= nBatch:
		batch := llama.BatchGetOne(tokens)
		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return fmt.Errorf("cache: failed to decode tokens: %w", err)
		}

	default:
		for i := 0; i < len(tokens); i += nBatch {
			end := min(i+nBatch, len(tokens))
			chunk := tokens[i:end]
			if _, err := llama.Decode(m.lctx, llama.BatchGetOne(chunk)); err != nil {
				return fmt.Errorf("cache: failed to decode token chunk: %w", err)
			}
		}
	}

	m.log(ctx, "cache", "status", "decoding-ended", "tokens", nTokens)

	return nil
}

// copySystemPromptToSeq copies the cached system prompt KV cache from sequence 0
// to the specified sequence ID. This should be called before processing a new
// request that will use the cached system prompt.
func (m *Model) copySystemPromptToSeq(seqID llama.SeqId) error {
	if !m.cfg.SystemPromptCache && !m.cfg.FirstMessageCache {
		return nil
	}

	m.sysPromptMu.RLock()
	hasCache := m.sysPromptTokens > 0
	m.sysPromptMu.RUnlock()

	if !hasCache {
		return nil
	}

	if err := llama.MemorySeqCp(m.mem, 0, seqID, -1, -1); err != nil {
		return fmt.Errorf("copy-cache: failed to copy memory seq 0 to %d: %w", seqID, err)
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

// removeFirstMessage returns a clone of D with the first message removed.
func removeFirstMessage(d D) D {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) <= 1 {
		return d
	}

	clone := d.Clone()
	clone["messages"] = messages[1:]

	return clone
}

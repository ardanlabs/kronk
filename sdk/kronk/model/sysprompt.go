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
	modifiedD D         // D with cached messages removed if cache was used
	prompt    string    // Templated prompt (set when caching occurs)
	media     [][]byte  // Media from templating (set when caching occurs)
	nPast     llama.Pos // Starting position for new tokens (cumulative from both caches)
	cached    bool      // True if any cache is being used
	err       error     // Any error that occurred
}

// ensureMessagesCached checks if system prompt and/or first user message are
// cached and updates the caches if necessary. The behavior depends on which
// cache modes are enabled:
//
//   - SystemPromptCache: Caches the first message with role="system".
//   - FirstMessageCache: Caches the first message with role="user".
//
// Both can be enabled simultaneously, using separate sequences for each cache.
//
// Returns a cacheResult containing:
//   - modifiedD: D with cached messages removed
//   - prompt: The templated prompt (only set when this function handles templating)
//   - media: Media bytes from templating (only set when this function handles templating)
//   - nPast: cumulative starting position from all cache hits
//   - cached: true if any message was cached and can be reused
//   - err: any error that occurred during cache update
//
// This function is thread-safe and handles concurrent requests appropriately.
func (m *Model) ensureFirstMessageCached(ctx context.Context, d D) cacheResult {
	if !m.cfg.SystemPromptCache && !m.cfg.FirstMessageCache {
		return cacheResult{modifiedD: d}
	}

	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	var totalNPast llama.Pos
	var anyCached bool
	var indicesToRemove []int

	// -------------------------------------------------------------------------
	// SystemPromptCache: cache first system message

	if m.cfg.SystemPromptCache {
		sysMsg, found := findCacheableMessage(messages, RoleSystem)

		switch found {
		case true:
			result := m.handleSystemPromptCache(ctx, d, sysMsg)
			if result.err != nil {
				return result
			}

			if result.cached {
				totalNPast += result.nPast
				anyCached = true
				indicesToRemove = append(indicesToRemove, sysMsg.index)
			}

		case false:
			// No system message but cache exists - use it.
			m.cacheMu.RLock()
			cachedTokens := m.sysPromptTokens
			m.cacheMu.RUnlock()

			if cachedTokens > 0 {
				m.log(ctx, "cache", "status", "hit-no-system-prompt", "tokens", cachedTokens)
				totalNPast += llama.Pos(cachedTokens)
				anyCached = true
			}
		}
	}

	// -------------------------------------------------------------------------
	// FirstMessageCache: cache first user message

	if m.cfg.FirstMessageCache {
		userMsg, found := findCacheableMessage(messages, RoleUser)
		if found {
			result := m.handleFirstMessageCache(ctx, d, userMsg)
			if result.err != nil {
				return result
			}

			if result.cached {
				totalNPast += result.nPast
				anyCached = true
				indicesToRemove = append(indicesToRemove, userMsg.index)

				// If FMC returned a prompt (first-time cache), use it directly.
				if result.prompt != "" {
					return cacheResult{
						modifiedD: d,
						prompt:    result.prompt,
						media:     result.media,
						nPast:     totalNPast,
						cached:    true,
					}
				}
			}
		}
	}

	// -------------------------------------------------------------------------
	// Remove cached messages from D

	if len(indicesToRemove) > 0 {
		d = removeMessagesAtIndices(d, indicesToRemove)
	}

	return cacheResult{
		modifiedD: d,
		nPast:     totalNPast,
		cached:    anyCached,
	}
}

// handleSystemPromptCache handles caching for system prompt mode.
// Uses sequence 0 for the cache.
func (m *Model) handleSystemPromptCache(ctx context.Context, d D, msgInfo cacheableMessage) cacheResult {
	return m.cacheMessage(ctx, d, msgInfo, 0, &m.sysPromptHash, &m.sysPromptTokens)
}

// handleFirstMessageCache handles caching for first user message mode.
// Uses sequence 0 (if only FMC) or sequence 1 (if both SPC and FMC enabled).
func (m *Model) handleFirstMessageCache(ctx context.Context, d D, msgInfo cacheableMessage) cacheResult {
	return m.cacheMessage(ctx, d, msgInfo, m.firstMsgSeqID, &m.firstMsgHash, &m.firstMsgTokens)
}

// cacheMessage is the common caching logic used by both SystemPromptCache
// and FirstMessageCache modes. It handles:
//   - Checking for cache hits when cache is populated
//   - Templating and caching the message when cache is empty
//   - Returning the suffix prompt for immediate use after caching
func (m *Model) cacheMessage(ctx context.Context, d D, msgInfo cacheableMessage, seqID llama.SeqId, hashPtr *string, tokensPtr *int) cacheResult {
	messages, _ := d["messages"].([]D)
	newHash := hashMessage(msgInfo)

	// -------------------------------------------------------------------------
	// Check for cache hit (fast path with read lock).

	m.cacheMu.RLock()
	currentHash := *hashPtr
	currentTokens := *tokensPtr
	m.cacheMu.RUnlock()

	if currentHash == newHash && currentTokens > 0 {
		m.log(ctx, "cache", "status", "hit", "role", msgInfo.role, "seq", seqID, "tokens", currentTokens, "messages", len(messages))
		return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
	}

	// -------------------------------------------------------------------------
	// Cache miss - template and cache the message.

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine cached while we waited.
	if *hashPtr == newHash && *tokensPtr > 0 {
		m.log(ctx, "cache", "status", "hit-after-lock", "role", msgInfo.role, "seq", seqID, "tokens", *tokensPtr)
		return cacheResult{nPast: llama.Pos(*tokensPtr), cached: true}
	}

	// Template just the messages up to and including the cached message WITHOUT add_generation_prompt.
	// This creates a prompt that is a valid prefix for subsequent requests.
	prefixMessages := messages[:msgInfo.index+1]
	prefixD := D{
		"messages":              prefixMessages,
		"add_generation_prompt": false,
	}

	// Copy tools if present (affects template output).
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	prefixPrompt, _, err := m.createPrompt(ctx, prefixD)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: failed to template message: %w", err)}
	}

	tokens := llama.Tokenize(m.vocab, prefixPrompt, true, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: message tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "cache", "status", "skip-too-short", "role", msgInfo.role, "seq", seqID, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	oldHash := *hashPtr
	if len(oldHash) > 8 {
		oldHash = oldHash[:8]
	}
	m.log(ctx, "cache", "status", "miss", "role", msgInfo.role, "seq", seqID,
		"old-hash", oldHash, "new-hash", newHash[:8])

	if err := m.decodeTokensToSeq(ctx, tokens, seqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	*hashPtr = newHash
	*tokensPtr = nTokens

	m.log(ctx, "cache", "status", "cached", "role", msgInfo.role, "seq", seqID, "tokens", nTokens, "hash", newHash[:8])

	// Get the full prompt to extract just the suffix (generation prompt).
	// The cached prefix is already in the sequence, we only need to add the suffix.
	fullPrompt, fullMedia, err := m.createPrompt(ctx, d)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: failed to template full message: %w", err)}
	}

	// Extract only the suffix (the part after the cached prefix).
	// This is typically just the assistant role marker for generation.
	// If extraction fails, use a sensible default that works with most templates.
	suffix := "<|im_start|>assistant\n"
	if len(fullPrompt) > len(prefixPrompt) {
		suffix = fullPrompt[len(prefixPrompt):]
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

// decodeTokensToSeq decodes tokens into the specified sequence for caching.
func (m *Model) decodeTokensToSeq(ctx context.Context, tokens []llama.Token, seqID llama.SeqId) error {
	llama.MemorySeqRm(m.mem, seqID, -1, -1)

	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	m.log(ctx, "cache", "status", "decoding-started", "seq", seqID, "tokens", nTokens)

	// Lock to prevent concurrent decode with batch engine.
	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

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

	m.log(ctx, "cache", "status", "decoding-ended", "seq", seqID, "tokens", nTokens)
	return nil
}

// copyCachesToSeq copies cached KV states from cache sequences to the target
// sequence. Handles both SPC (seq 0) and FMC (seq 0 or 1) based on config.
func (m *Model) copyCachesToSeq(seqID llama.SeqId) error {
	if !m.cfg.SystemPromptCache && !m.cfg.FirstMessageCache {
		return nil
	}

	m.cacheMu.RLock()
	sysTokens := m.sysPromptTokens
	fmcTokens := m.firstMsgTokens
	m.cacheMu.RUnlock()

	// Copy system prompt cache (seq 0) if enabled and populated.
	if m.cfg.SystemPromptCache && sysTokens > 0 {
		if err := llama.MemorySeqCp(m.mem, 0, seqID, -1, -1); err != nil {
			return fmt.Errorf("copy-cache: failed to copy SPC seq 0 to %d: %w", seqID, err)
		}
	}

	// Copy first message cache if enabled and populated.
	if m.cfg.FirstMessageCache && fmcTokens > 0 {
		if err := llama.MemorySeqCp(m.mem, m.firstMsgSeqID, seqID, -1, -1); err != nil {
			return fmt.Errorf("copy-cache: failed to copy FMC seq %d to %d: %w", m.firstMsgSeqID, seqID, err)
		}
	}

	return nil
}

// copySystemPromptToSeq copies the cached system prompt KV cache from sequence 0
// to the specified sequence ID. This is kept for backward compatibility.
// For new code, use copyCachesToSeq which handles both caches.
func (m *Model) copySystemPromptToSeq(seqID llama.SeqId) error {
	return m.copyCachesToSeq(seqID)
}

// clearCaches clears all cached prompt states.
// This is useful when the model context is reset.
func (m *Model) clearCaches() {
	m.cacheMu.Lock()
	m.sysPromptHash = ""
	m.sysPromptTokens = 0
	m.firstMsgHash = ""
	m.firstMsgTokens = 0
	m.cacheMu.Unlock()
}

// clearSystemPromptCache clears the cached system prompt state.
// Kept for backward compatibility.
func (m *Model) clearSystemPromptCache() {
	m.clearCaches()
}

// =============================================================================

// cacheableMessage contains information about a message that can be cached.
type cacheableMessage struct {
	index   int
	role    string
	content string
}

// findCacheableMessage finds the first message with the specified role.
// Returns the message info and true if found.
func findCacheableMessage(messages []D, targetRole string) (cacheableMessage, bool) {
	for i, msg := range messages {
		role, ok := msg["role"].(string)
		if !ok || role != targetRole {
			continue
		}

		// Handle content as string or array (OpenAI multi-part format).
		var content string
		switch c := msg["content"].(type) {
		case string:
			content = c

		case []any:
			// Extract text from array of content parts.
			for _, part := range c {
				if partMap, ok := part.(map[string]any); ok {
					if partMap["type"] == "text" {
						if text, ok := partMap["text"].(string); ok {
							content += text
						}
					}
				}
			}

		case []D:
			// Extract text from array of D content parts.
			for _, part := range c {
				if part["type"] == "text" {
					if text, ok := part["text"].(string); ok {
						content += text
					}
				}
			}
		}

		if content == "" {
			continue
		}

		return cacheableMessage{index: i, role: role, content: content}, true
	}

	return cacheableMessage{}, false
}

// hashMessage computes a SHA-256 hash of a message.
// Includes the role in the hash to differentiate between same content with different roles.
func hashMessage(info cacheableMessage) string {
	data := info.role + ":" + info.content
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// removeMessagesAtIndices returns a clone of D with messages at the specified indices removed.
// Indices should be in ascending order for correct removal.
func removeMessagesAtIndices(d D, indices []int) D {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 || len(indices) == 0 {
		return d
	}

	// Build a set of indices to remove for O(1) lookup.
	removeSet := make(map[int]bool, len(indices))
	for _, idx := range indices {
		removeSet[idx] = true
	}

	// Build new messages slice excluding removed indices.
	newMessages := make([]D, 0, len(messages)-len(indices))
	for i, msg := range messages {
		if !removeSet[i] {
			newMessages = append(newMessages, msg)
		}
	}

	if len(newMessages) == 0 {
		return d
	}

	clone := d.Clone()
	clone["messages"] = newMessages

	return clone
}

package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

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
// SPC and FMC are mutually exclusive; FMC includes the system prompt in its cache.
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
	// FirstMessageCache (IMC): Incremental multi-turn caching for agentic workflows

	if m.cfg.FirstMessageCache {
		result := m.handleIncrementalMessageCache(ctx, d, messages)
		if result.err != nil {
			return result
		}

		if result.cached {
			totalNPast += result.nPast
			anyCached = true

			// IMC caches messages[0:imcMsgCount]. Remove all cached messages.
			m.cacheMu.RLock()
			cachedCount := m.imcMsgCount
			m.cacheMu.RUnlock()

			for i := 0; i < cachedCount; i++ {
				indicesToRemove = append(indicesToRemove, i)
			}

			// If IMC returned a prompt (first-time cache or extension), use it directly.
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
	return m.cacheMessage(ctx, d, msgInfo, 0, &m.sysPromptHash, &m.sysPromptTokens, &m.sysPromptLen)
}

// handleIncrementalMessageCache implements incremental multi-turn caching (IMC)
// for agentic workflows. It caches all messages except the last one (which
// triggers generation) and extends the cache incrementally on subsequent requests.
//
// Algorithm:
//  1. If cache is empty (imcMsgCount=0), cache messages[0:len-1]
//  2. If cache exists, check if messages[0:imcMsgCount] match cached hash
//  3. On prefix match: extend cache with messages[imcMsgCount:len-1]
//  4. On prefix mismatch: rebuild cache from scratch
func (m *Model) handleIncrementalMessageCache(ctx context.Context, d D, messages []D) cacheResult {
	nMessages := len(messages)
	if nMessages < 2 {
		// Need at least 2 messages: one to cache, one to generate from.
		return cacheResult{modifiedD: d}
	}

	// Target: cache all messages except the last one (which triggers generation).
	targetCacheEnd := nMessages - 1

	// -------------------------------------------------------------------------
	// Read current cache state (fast path with read lock).

	m.cacheMu.RLock()
	currentHash := m.imcHash
	currentTokens := m.imcTokens
	currentMsgCount := m.imcMsgCount
	m.cacheMu.RUnlock()

	// -------------------------------------------------------------------------
	// Check for cache hit on existing prefix.

	if currentMsgCount > 0 && currentTokens > 0 {
		// Verify the cached prefix still matches.
		if currentMsgCount <= nMessages {
			prefixHash := hashMessages(messages[:currentMsgCount])
			if prefixHash == currentHash {
				// Cache hit on prefix. Check if we need to extend.
				if currentMsgCount < targetCacheEnd {
					// Extend cache with new messages.
					return m.extendIMCCache(ctx, d, messages, currentMsgCount, targetCacheEnd, currentTokens)
				}

				// Prefix matches and covers all cacheable messages.
				m.log(ctx, "imc", "status", "hit", "cached-msgs", currentMsgCount, "tokens", currentTokens)
				return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
			}
		}

		// Prefix mismatch - need to rebuild cache.
		m.log(ctx, "imc", "status", "prefix-mismatch", "cached-msgs", currentMsgCount, "request-msgs", nMessages)
	}

	// -------------------------------------------------------------------------
	// Cache miss - build cache from scratch.

	return m.buildIMCCache(ctx, d, messages, targetCacheEnd)
}

// cacheMessage is the common caching logic used by both SystemPromptCache
// and FirstMessageCache modes. It handles:
//   - Checking for cache hits when cache is populated
//   - Templating and caching the message when cache is empty
//   - Returning the suffix prompt for immediate use after caching
func (m *Model) cacheMessage(ctx context.Context, d D, msgInfo cacheableMessage, seqID llama.SeqId, hashPtr *string, tokensPtr *int, lenPtr *int) cacheResult {
	messages, _ := d["messages"].([]D)
	contentLen := len(msgInfo.content)

	// -------------------------------------------------------------------------
	// Check for cache hit (fast path with read lock).
	// Use length check first to avoid expensive SHA-256 hash on cache miss.

	m.cacheMu.RLock()
	currentHash := *hashPtr
	currentTokens := *tokensPtr
	currentLen := *lenPtr
	m.cacheMu.RUnlock()

	if currentLen == contentLen && currentTokens > 0 {
		newHash := hashMessage(msgInfo)
		if currentHash == newHash {
			m.log(ctx, "cache", "status", "hit", "role", msgInfo.role, "seq", seqID, "tokens", currentTokens, "messages", len(messages))
			return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
		}
	}

	// -------------------------------------------------------------------------
	// Cache miss - template and cache the message.

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine cached while we waited.
	// Must compute hash here since we skipped it above on length mismatch.
	newHash := hashMessage(msgInfo)
	if *lenPtr == contentLen && *hashPtr == newHash && *tokensPtr > 0 {
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
	*lenPtr = contentLen

	m.log(ctx, "cache", "status", "cached", "role", msgInfo.role, "seq", seqID, "tokens", nTokens, "hash", newHash[:8])

	// -------------------------------------------------------------------------
	// System Prompt Caching

	// SPC doesn't need suffix - remaining messages are templated later in chat.go.
	// Only FMC needs the suffix for immediate use.
	if msgInfo.role == RoleSystem {
		return cacheResult{
			modifiedD: d,
			nPast:     llama.Pos(nTokens),
			cached:    true,
		}
	}

	// -------------------------------------------------------------------------
	// First Message Caching

	// Extract the suffix needed for generation (FMC only).
	var suffix string
	var fullMedia [][]byte

	// Check if the cached message is the last message in the array.
	// - SPC: System message always has user message(s) after, so this is false.
	// - FMC: On first request with [system, user], this is true (cheap path).
	//        Later requests hit cache earlier and don't reach this code.
	switch msgInfo.index+1 == len(messages) {
	case true:
		// Cached message is the last message. Template empty messages to get
		// only the generation prompt suffix (avoids re-templating all messages).
		genD := D{"messages": []D{}, "add_generation_prompt": true}
		suffix, _, err = m.createPrompt(ctx, genD)
		if err != nil {
			suffix = "<|im_start|>assistant\n"
		}

	case false:
		// Additional messages exist after the cached message.
		// Must template the full D to extract the complete suffix.
		var fullPrompt string
		fullPrompt, fullMedia, err = m.createPrompt(ctx, d)
		if err != nil {
			return cacheResult{modifiedD: d, err: fmt.Errorf("cache: failed to template full message: %w", err)}
		}

		suffix = "<|im_start|>assistant\n"
		if len(fullPrompt) > len(prefixPrompt) {
			suffix = fullPrompt[len(prefixPrompt):]
		}
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

	// Defensive check: log if NBatch seems invalid.
	if nBatch <= 0 {
		m.log(ctx, "decode-tokens-to-seq", "ERROR", "invalid-nbatch",
			"nbatch", nBatch,
			"ctxParams.NBatch", m.ctxParams.NBatch,
			"cfg.NBatch", m.cfg.NBatch)

		nBatch = m.cfg.NBatch // Fall back to config value
	}

	m.log(ctx, "cache", "status", "decoding-started", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

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

// copyCachesToSeq copies cached KV state from the cache sequence (seq 0) to
// the target sequence. SPC and IMC are mutually exclusive; both use seq 0.
func (m *Model) copyCachesToSeq(seqID llama.SeqId) error {
	if !m.cfg.SystemPromptCache && !m.cfg.FirstMessageCache {
		return nil
	}

	m.cacheMu.RLock()
	sysTokens := m.sysPromptTokens
	imcTokens := m.imcTokens
	m.cacheMu.RUnlock()

	// Copy system prompt cache (seq 0) if enabled and populated.
	if m.cfg.SystemPromptCache && sysTokens > 0 {
		if err := llama.MemorySeqCp(m.mem, 0, seqID, -1, -1); err != nil {
			return fmt.Errorf("copy-cache: failed to copy SPC seq 0 to %d: %w", seqID, err)
		}
	}

	// Copy IMC cache (seq 0) if enabled and populated.
	if m.cfg.FirstMessageCache && imcTokens > 0 {
		if err := llama.MemorySeqCp(m.mem, 0, seqID, -1, -1); err != nil {
			return fmt.Errorf("copy-cache: failed to copy IMC seq 0 to %d: %w", seqID, err)
		}
	}

	return nil
}

// copySystemPromptToSeq copies the cached KV cache from sequence 0 to the
// specified sequence ID. This is an alias for copyCachesToSeq.
func (m *Model) copySystemPromptToSeq(seqID llama.SeqId) error {
	return m.copyCachesToSeq(seqID)
}

// clearCaches clears all cached prompt states.
// This is useful when the model context is reset.
func (m *Model) clearCaches() {
	m.cacheMu.Lock()
	m.sysPromptHash = ""
	m.sysPromptTokens = 0
	m.sysPromptLen = 0
	m.imcHash = ""
	m.imcTokens = 0
	m.imcMsgCount = 0
	m.imcPromptLen = 0
	m.cacheMu.Unlock()
}

// clearSystemPromptCache clears the cached system prompt state.
// Kept for backward compatibility.
func (m *Model) clearSystemPromptCache() {
	m.clearCaches()
}

// =============================================================================
// IMC (Incremental Message Cache) Functions
// =============================================================================

// buildIMCCache builds the cache from scratch for messages[0:targetEnd].
func (m *Model) buildIMCCache(ctx context.Context, d D, messages []D, targetEnd int) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine built the cache while we waited.
	// Also verify we have enough messages to check against cached count.
	if m.imcMsgCount > 0 && m.imcTokens > 0 && m.imcMsgCount <= len(messages) {
		prefixHash := hashMessages(messages[:m.imcMsgCount])
		if prefixHash == m.imcHash {
			m.log(ctx, "imc", "status", "hit-after-lock", "cached-msgs", m.imcMsgCount, "tokens", m.imcTokens)
			return cacheResult{nPast: llama.Pos(m.imcTokens), cached: true}
		}
	}

	// Clear existing cache sequence.
	llama.MemorySeqRm(m.mem, m.imcSeqID, -1, -1)

	// Template messages[0:targetEnd] WITHOUT add_generation_prompt.
	prefixMessages := messages[:targetEnd]
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
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template messages: %w", err)}
	}

	tokens := llama.Tokenize(m.vocab, prefixPrompt, true, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: messages tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "imc", "status", "skip-too-short", "msgs", targetEnd, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	// Decode tokens into cache sequence.
	if err := m.decodeTokensToSeq(ctx, tokens, m.imcSeqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	// Update cache state.
	newHash := hashMessages(prefixMessages)
	m.imcHash = newHash
	m.imcTokens = nTokens
	m.imcMsgCount = targetEnd
	m.imcPromptLen = len(prefixPrompt)

	m.log(ctx, "imc", "status", "built", "msgs", targetEnd, "tokens", nTokens, "prompt-len", len(prefixPrompt), "hash", newHash[:8])

	// Generate suffix for immediate use.
	suffix, media, err := m.generateIMCSuffix(ctx, d, messages, prefixPrompt, targetEnd)
	if err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	return cacheResult{
		modifiedD: d,
		prompt:    suffix,
		media:     media,
		nPast:     llama.Pos(nTokens),
		cached:    true,
	}
}

// extendIMCCache extends the existing cache with messages[currentEnd:targetEnd].
func (m *Model) extendIMCCache(ctx context.Context, d D, messages []D, currentEnd, targetEnd, currentTokens int) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check cache state hasn't changed.
	if m.imcMsgCount != currentEnd || m.imcTokens != currentTokens {
		// State changed, fall back to full rebuild.
		m.cacheMu.Unlock()
		result := m.buildIMCCache(ctx, d, messages, targetEnd)
		m.cacheMu.Lock()
		return result
	}

	// Get the stored prefix length from the cache.
	oldPrefixLen := m.imcPromptLen

	// Template the new prefix (messages to cache) - needed to find extension boundary.
	prefixD := D{
		"messages":              messages[:targetEnd],
		"add_generation_prompt": false,
	}
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	prefixPrompt, _, err := m.createPrompt(ctx, prefixD)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template prefix: %w", err)}
	}

	// Check for extension content.
	if len(prefixPrompt) <= oldPrefixLen {
		m.log(ctx, "imc", "status", "extend-no-content", "old-len", oldPrefixLen, "new-len", len(prefixPrompt))
		return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
	}

	// Extract extension from the new prefix.
	extensionPrompt := prefixPrompt[oldPrefixLen:]
	extensionTokens := llama.Tokenize(m.vocab, extensionPrompt, false, true)
	nExtTokens := len(extensionTokens)

	if nExtTokens == 0 {
		m.log(ctx, "imc", "status", "extend-zero-tokens")
		return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
	}

	// Decode extension tokens into cache sequence.
	if err := m.decodeExtensionTokens(ctx, extensionTokens, m.imcSeqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	// Template full D once for the suffix (last message + generation prompt).
	fullPrompt, media, err := m.createPrompt(ctx, d)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template full message: %w", err)}
	}

	// Verify prefix is actually a prefix of full prompt. If not, template
	// nondeterminism has occurred and we must rebuild the cache.
	if !strings.HasPrefix(fullPrompt, prefixPrompt) {
		m.log(ctx, "imc", "status", "prefix-mismatch-rebuild",
			"prefix-len", len(prefixPrompt), "full-len", len(fullPrompt))

		// Rebuild cache from scratch to ensure consistency.
		m.cacheMu.Unlock()
		result := m.buildIMCCache(ctx, d, messages, targetEnd)
		m.cacheMu.Lock()
		return result
	}

	// Update cache state.
	newTokens := currentTokens + nExtTokens
	newHash := hashMessages(messages[:targetEnd])
	m.imcHash = newHash
	m.imcTokens = newTokens
	m.imcMsgCount = targetEnd
	m.imcPromptLen = len(prefixPrompt)

	// Suffix is everything after the new prefix.
	suffix := fullPrompt[len(prefixPrompt):]

	m.log(ctx, "imc", "status", "extended", "old-msgs", currentEnd, "new-msgs", targetEnd,
		"old-tokens", currentTokens, "new-tokens", newTokens, "ext-tokens", nExtTokens, "suffix-len", len(suffix))

	return cacheResult{
		modifiedD: d,
		prompt:    suffix,
		media:     media,
		nPast:     llama.Pos(newTokens),
		cached:    true,
	}
}

// decodeExtensionTokens decodes additional tokens into an existing cache sequence.
// Unlike decodeTokensToSeq, this does NOT clear the sequence first.
func (m *Model) decodeExtensionTokens(ctx context.Context, tokens []llama.Token, seqID llama.SeqId) error {
	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch
	}

	m.log(ctx, "imc", "status", "extending", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	switch {
	case nTokens <= nBatch:
		batch := llama.BatchGetOne(tokens)
		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return fmt.Errorf("imc: failed to decode extension tokens: %w", err)
		}

	default:
		for i := 0; i < len(tokens); i += nBatch {
			end := min(i+nBatch, len(tokens))
			chunk := tokens[i:end]
			if _, err := llama.Decode(m.lctx, llama.BatchGetOne(chunk)); err != nil {
				return fmt.Errorf("imc: failed to decode extension chunk: %w", err)
			}
		}
	}

	return nil
}

// generateIMCSuffix generates the suffix prompt for un-cached messages.
// The suffix includes messages[cachedEnd:] plus the generation prompt.
func (m *Model) generateIMCSuffix(ctx context.Context, d D, messages []D, prefixPrompt string, cachedEnd int) (string, [][]byte, error) {
	// Template the full D to get the complete prompt, then extract suffix.
	fullPrompt, media, err := m.createPrompt(ctx, d)
	if err != nil {
		return "", nil, fmt.Errorf("imc: failed to template full message: %w", err)
	}

	// Verify prefix is actually a prefix of full prompt.
	if !strings.HasPrefix(fullPrompt, prefixPrompt) {
		return "", nil, fmt.Errorf("imc: prefix mismatch, prefix-len=%d full-len=%d", len(prefixPrompt), len(fullPrompt))
	}

	suffix := fullPrompt[len(prefixPrompt):]

	m.log(ctx, "imc", "suffix-len", len(suffix), "prefix-len", len(prefixPrompt), "full-len", len(fullPrompt))

	return suffix, media, nil
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
	data := fmt.Sprintf("%s:%s", info.role, info.content)
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// hashMessages computes a SHA-256 hash of a slice of messages.
// Used by IMC to validate that the cached prefix matches the current request.
func hashMessages(messages []D) string {
	h := sha256.New()
	for i, msg := range messages {
		role, _ := msg["role"].(string)
		content := extractMessageContent(msg)
		h.Write([]byte(fmt.Sprintf("%d:%s:%s|", i, role, content)))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// extractMessageContent extracts the text content from a message.
// Handles both string content and array content (OpenAI multi-part format).
func extractMessageContent(msg D) string {
	switch c := msg["content"].(type) {
	case string:
		return c

	case []any:
		var content string
		for _, part := range c {
			if partMap, ok := part.(map[string]any); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						content += text
					}
				}
			}
		}
		return content

	case []D:
		var content string
		for _, part := range c {
			if part["type"] == "text" {
				if text, ok := part["text"].(string); ok {
					content += text
				}
			}
		}
		return content
	}

	return ""
}

// removeMessagesAtIndices returns D with messages at the specified indices removed.
// Indices should be in ascending order for correct removal. Mutates d in place.
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

	d["messages"] = newMessages

	return d
}

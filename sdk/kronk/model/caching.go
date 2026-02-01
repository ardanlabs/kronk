package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

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

	// IMC-specific fields (set when IncrementalCache is used)
	imcID    string      // IMC session ID
	imcSeqID llama.SeqId // IMC session's cache sequence
}

// processCache checks if the system prompt or incremental messages are
// being cached and updates to the caches are necessary. The behavior depends
// on which cache modes are enabled:
//
//   - SystemPromptCache (SPC): Caches the first message with role="system".
//   - IncrementalCache (IMC): Caches all messages except the last one.
//
// SPC and IMC are mutually exclusive
// IMC includes the system prompt in its cache.
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
func (m *Model) processCache(ctx context.Context, d D) cacheResult {
	if !m.cfg.SystemPromptCache && !m.cfg.IncrementalCache {
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
	// IncrementalCache (IMC): Incremental multi-turn caching for agentic
	// workflows. Requires imc_id to identify the session.

	if m.cfg.IncrementalCache {
		// IMC requires an imc_id to identify the session.
		imcID, _ := d["imc_id"].(string)
		if imcID == "" {
			// No imc_id provided - bypass IMC for this request.
			m.log(ctx, "cache", "status", "bypass-IMC", "reason", "IMC turned on but no imc_id was provided")
			return cacheResult{modifiedD: d}
		}

		result, cachedMsgCount := m.handleIncrementalMessageCache(ctx, d, messages, imcID)
		if result.err != nil {
			return result
		}

		if result.cached {
			totalNPast += result.nPast
			anyCached = true

			// IMC caches messages[0:cachedMsgCount]. Remove all cached messages.
			for i := 0; i < cachedMsgCount; i++ {
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
					imcID:    result.imcID,
					imcSeqID: result.imcSeqID,
				}
			}

			// IMC hit without prompt - propagate session info.
			return cacheResult{
				modifiedD: removeMessagesAtIndices(d, indicesToRemove),
				nPast:     totalNPast,
				cached:    true,
				imcID:     result.imcID,
				imcSeqID:  result.imcSeqID,
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
// Each unique imcID gets its own session with a dedicated cache sequence.
// Returns the cache result and the number of cached messages (for removal).
//
// Algorithm:
//  1. Look up or create session for imcID
//  2. If session cache is empty, cache messages[0:len-1]
//  3. If session cache exists, check if messages[0:msgCount] match cached hash
//  4. On prefix match: extend cache with messages[msgCount:len-1]
//  5. On prefix mismatch: rebuild cache from scratch
func (m *Model) handleIncrementalMessageCache(ctx context.Context, d D, messages []D, imcID string) (cacheResult, int) {
	nMessages := len(messages)
	if nMessages < 2 {
		// Need at least 2 messages: one to cache, one to generate from.
		return cacheResult{modifiedD: d}, 0
	}

	// Target: cache all messages except the last one (which triggers generation).
	targetCacheEnd := nMessages - 1

	// -------------------------------------------------------------------------
	// Look up or create session for this imcID.

	session, isNew := m.getOrCreateIMCSession(ctx, imcID)
	if session == nil {
		// All session slots are in use - bypass IMC gracefully.
		m.log(ctx, "imc", "status", "bypass-slots-full", "imc_id", imcID, "max", m.imcMaxSeqs)
		return cacheResult{modifiedD: d}, 0
	}

	// -------------------------------------------------------------------------
	// Read current session state (fast path with read lock).

	m.cacheMu.RLock()
	currentHash := session.hash
	currentTokens := session.tokens
	currentMsgCount := session.msgCount
	seqID := session.seqID
	m.cacheMu.RUnlock()

	// -------------------------------------------------------------------------
	// Check for cache hit on existing prefix.

	if !isNew && currentMsgCount > 0 && currentTokens > 0 {
		// Verify the cached prefix still matches.
		if currentMsgCount <= nMessages {
			prefixHash := hashMessages(messages[:currentMsgCount])
			if prefixHash == currentHash {
				// Cache hit on prefix. Check if we need to extend.
				if currentMsgCount < targetCacheEnd {
					// Extend cache with new messages.
					return m.extendIMCCache(ctx, d, messages, imcID, session, currentMsgCount, targetCacheEnd, currentTokens), currentMsgCount
				}

				// Prefix matches and covers all cacheable messages.
				m.log(ctx, "imc", "status", "hit", "imc_id", imcID, "seq", seqID, "cached-msgs", currentMsgCount, "tokens", currentTokens)
				return cacheResult{
					nPast:    llama.Pos(currentTokens),
					cached:   true,
					imcID:    imcID,
					imcSeqID: seqID,
				}, currentMsgCount
			}
		}

		// Prefix mismatch - need to rebuild cache.
		m.log(ctx, "imc", "status", "prefix-mismatch", "imc_id", imcID, "cached-msgs", currentMsgCount, "request-msgs", nMessages)
	}

	// -------------------------------------------------------------------------
	// Cache miss - build cache from scratch.

	result := m.buildIMCCache(ctx, d, messages, imcID, session, targetCacheEnd)
	return result, targetCacheEnd
}

// cacheMessage is the common caching logic used by SystemPromptCache mode.
// It handles:
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

	// SPC doesn't need suffix - remaining messages are templated later in chat.go.
	return cacheResult{
		modifiedD: d,
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

// copyCachesToSeq copies cached KV state from a cache sequence to the target
// sequence. For SPC, uses seq 0. For IMC, the srcSeqID must be passed explicitly.
func (m *Model) copyCachesToSeq(dstSeqID llama.SeqId, srcSeqID llama.SeqId) error {
	if !m.cfg.SystemPromptCache && !m.cfg.IncrementalCache {
		return nil
	}

	if err := llama.MemorySeqCp(m.mem, srcSeqID, dstSeqID, -1, -1); err != nil {
		return fmt.Errorf("copy-cache: failed to copy seq %d to %d: %w", srcSeqID, dstSeqID, err)
	}

	return nil
}

// copySystemPromptToSeq copies the SPC cache (seq 0) to the target sequence.
// For batch engine slot restoration after completion.
func (m *Model) copySystemPromptToSeq(seqID llama.SeqId) error {
	if !m.cfg.SystemPromptCache {
		return nil
	}

	m.cacheMu.RLock()
	sysTokens := m.sysPromptTokens
	m.cacheMu.RUnlock()

	if sysTokens > 0 {
		return m.copyCachesToSeq(seqID, 0)
	}

	return nil
}

// clearCaches clears all cached prompt states.
// This is useful when the model context is reset.
func (m *Model) clearCaches() {
	m.cacheMu.Lock()
	m.sysPromptHash = ""
	m.sysPromptTokens = 0
	m.sysPromptLen = 0

	// Clear all IMC sessions.
	for id := range m.imcSessions {
		delete(m.imcSessions, id)
	}
	m.imcNextSeq = 0
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

// getOrCreateIMCSession looks up an existing session by imcID or creates a new one.
// Returns nil if all session slots are in use (graceful bypass).
// Returns the session and true if it was newly created.
func (m *Model) getOrCreateIMCSession(ctx context.Context, imcID string) (*imcSession, bool) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Check if session already exists.
	if session, exists := m.imcSessions[imcID]; exists {
		session.lastUsed = time.Now()
		return session, false
	}

	// Check if we have room for a new session.
	if int(m.imcNextSeq) >= m.imcMaxSeqs {
		// All slots are in use. Could implement LRU eviction here in the future.
		return nil, false
	}

	// Create new session with next available sequence.
	session := &imcSession{
		seqID:    m.imcNextSeq,
		lastUsed: time.Now(),
	}
	m.imcSessions[imcID] = session
	m.imcNextSeq++

	m.log(ctx, "imc", "status", "session-created", "imc_id", imcID, "seq", session.seqID, "total-sessions", len(m.imcSessions))

	return session, true
}

// =============================================================================

// buildIMCCache builds the cache from scratch for messages[0:targetEnd].
func (m *Model) buildIMCCache(ctx context.Context, d D, messages []D, imcID string, session *imcSession, targetEnd int) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine built the cache while we waited.
	if session.msgCount > 0 && session.tokens > 0 && session.msgCount <= len(messages) {
		prefixHash := hashMessages(messages[:session.msgCount])
		if prefixHash == session.hash {
			m.log(ctx, "imc", "status", "hit-after-lock", "imc_id", imcID, "cached-msgs", session.msgCount, "tokens", session.tokens)
			return cacheResult{nPast: llama.Pos(session.tokens), cached: true}
		}
	}

	// Clear existing cache sequence.
	llama.MemorySeqRm(m.mem, session.seqID, -1, -1)

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
		m.log(ctx, "imc", "status", "skip-too-short", "imc_id", imcID, "msgs", targetEnd, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	// Decode tokens into cache sequence.
	if err := m.decodeTokensToSeq(ctx, tokens, session.seqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	// Update session state.
	newHash := hashMessages(prefixMessages)
	session.hash = newHash
	session.tokens = nTokens
	session.msgCount = targetEnd
	session.promptLen = len(prefixPrompt)
	session.lastUsed = time.Now()

	m.log(ctx, "imc", "status", "built", "imc_id", imcID, "seq", session.seqID, "msgs", targetEnd, "tokens", nTokens, "prompt-len", len(prefixPrompt), "hash", newHash[:8])

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
		imcID:     imcID,
		imcSeqID:  session.seqID,
	}
}

// extendIMCCache extends the existing cache with messages[currentEnd:targetEnd].
func (m *Model) extendIMCCache(ctx context.Context, d D, messages []D, imcID string, session *imcSession, currentEnd, targetEnd, currentTokens int) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check session state hasn't changed.
	if session.msgCount != currentEnd || session.tokens != currentTokens {
		// State changed, fall back to full rebuild.
		m.cacheMu.Unlock()
		result := m.buildIMCCache(ctx, d, messages, imcID, session, targetEnd)
		m.cacheMu.Lock()
		return result
	}

	// Get the stored prefix length from the session.
	oldPrefixLen := session.promptLen

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
		m.log(ctx, "imc", "status", "extend-no-content", "imc_id", imcID, "old-len", oldPrefixLen, "new-len", len(prefixPrompt))
		return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
	}

	// Extract extension from the new prefix.
	extensionPrompt := prefixPrompt[oldPrefixLen:]
	extensionTokens := llama.Tokenize(m.vocab, extensionPrompt, false, true)
	nExtTokens := len(extensionTokens)

	if nExtTokens == 0 {
		m.log(ctx, "imc", "status", "extend-zero-tokens", "imc_id", imcID)
		return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
	}

	// Decode extension tokens into cache sequence.
	if err := m.decodeExtensionTokens(ctx, extensionTokens, session.seqID); err != nil {
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
		m.log(ctx, "imc", "status", "prefix-mismatch-rebuild", "imc_id", imcID,
			"prefix-len", len(prefixPrompt), "full-len", len(fullPrompt))

		// Rebuild cache from scratch to ensure consistency.
		m.cacheMu.Unlock()
		result := m.buildIMCCache(ctx, d, messages, imcID, session, targetEnd)
		m.cacheMu.Lock()
		return result
	}

	// Update session state.
	newTokens := currentTokens + nExtTokens
	newHash := hashMessages(messages[:targetEnd])
	session.hash = newHash
	session.tokens = newTokens
	session.msgCount = targetEnd
	session.promptLen = len(prefixPrompt)
	session.lastUsed = time.Now()

	// Suffix is everything after the new prefix.
	suffix := fullPrompt[len(prefixPrompt):]

	m.log(ctx, "imc", "status", "extended", "imc_id", imcID, "seq", session.seqID, "old-msgs", currentEnd, "new-msgs", targetEnd,
		"old-tokens", currentTokens, "new-tokens", newTokens, "ext-tokens", nExtTokens, "suffix-len", len(suffix))

	return cacheResult{
		modifiedD: d,
		prompt:    suffix,
		media:     media,
		nPast:     llama.Pos(newTokens),
		cached:    true,
		imcID:     imcID,
		imcSeqID:  session.seqID,
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

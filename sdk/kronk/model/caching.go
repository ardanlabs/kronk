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
				m.log(ctx, "cache", "status", "hit (no system prompt)", "tokens", cachedTokens)
				totalNPast += llama.Pos(cachedTokens)
				anyCached = true
			}
		}
	}

	// -------------------------------------------------------------------------
	// IncrementalCache (IMC): Incremental multi-turn caching for agentic
	// workflows. Requires imc_id to identify the session.

	if m.cfg.IncrementalCache {
		imcID, _ := d["imc_id"].(string)
		if imcID == "" {
			imcID = "default"
			m.log(ctx, "cache", "status", "using default imc id", "reason", "IMC turned on but no imc_id was provided")
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
					imcID:     result.imcID,
					imcSeqID:  result.imcSeqID,
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
//  2. Template the full prompt to get canonical bytes
//  3. Compare cached prompt hash with prefix of new prompt
//  4. On match: extend cache with new bytes
//  5. On mismatch: fall back to cached tokens only (no extension)
func (m *Model) handleIncrementalMessageCache(ctx context.Context, d D, messages []D, imcID string) (cacheResult, int) {
	nMessages := len(messages)
	if nMessages < 2 {
		// Need at least 2 messages: one to cache, one to generate from.
		return cacheResult{modifiedD: d}, 0
	}

	// -------------------------------------------------------------------------
	// Look up or create session for this imcID.

	session, isNew := m.getOrCreateIMCSession(ctx, imcID)
	if session == nil {
		// All session slots are in use - bypass IMC gracefully.
		m.log(ctx, "imc", "status", "bypass (slots full)", "imc_id", imcID, "max", m.imcMaxSeqs)
		return cacheResult{modifiedD: d}, 0
	}

	// -------------------------------------------------------------------------
	// Template the full prompt - this is our source of truth.

	fullPrompt, media, err := m.createPrompt(ctx, d)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template prompt: %w", err)}, 0
	}

	// -------------------------------------------------------------------------
	// Read current session state.

	m.cacheMu.RLock()
	cachedPromptHash := session.promptHash
	cachedPromptLen := session.promptLen
	cachedTokens := session.tokens
	cachedMsgCount := session.msgCount
	seqID := session.seqID
	m.cacheMu.RUnlock()

	// -------------------------------------------------------------------------
	// New session - build cache from scratch.

	if isNew || cachedTokens == 0 {
		return m.buildIMCCache(ctx, d, fullPrompt, media, imcID, session), nMessages - 1
	}

	// -------------------------------------------------------------------------
	// Existing session - validate cached prefix against new prompt.

	// Check if message count dropped - this indicates a new conversation.
	if nMessages <= cachedMsgCount {
		m.log(ctx, "imc", "status", "rebuild (new conversation)", "imc_id", imcID,
			"cached-msgs", cachedMsgCount, "new-msgs", nMessages)
		return m.buildIMCCache(ctx, d, fullPrompt, media, imcID, session), nMessages - 1
	}

	// Check if the new prompt is long enough to contain the cached prefix.
	if len(fullPrompt) < cachedPromptLen {
		// New prompt is shorter - this is a new conversation, rebuild.
		m.log(ctx, "imc", "status", "rebuild (shorter prompt)", "imc_id", imcID,
			"cached-len", cachedPromptLen, "new-len", len(fullPrompt))
		return m.buildIMCCache(ctx, d, fullPrompt, media, imcID, session), nMessages - 1
	}

	// Hash the prefix portion of the new prompt and compare.
	prefixToCheck := fullPrompt[:cachedPromptLen]
	prefixHash := hashPrompt(prefixToCheck)

	switch prefixHash == cachedPromptHash {
	case true:
		// Cached prefix is still valid - extend with new content.
		return m.extendIMCCache(ctx, d, fullPrompt, media, imcID, session), nMessages - 1

	default:
		// Template changed earlier content - fall back to cached tokens only.
		// This happens with GPT/GLM models where tool call injection changes
		// how earlier messages are rendered.
		m.log(ctx, "imc", "status", "prefix mismatch (no extend)", "imc_id", imcID,
			"seq", seqID, "cached-tokens", cachedTokens,
			"cached-hash", cachedPromptHash[:8], "new-hash", prefixHash[:8])

		// Return cached position but don't extend - caller will re-template.
		return cacheResult{
			nPast:    llama.Pos(cachedTokens),
			cached:   true,
			imcID:    imcID,
			imcSeqID: seqID,
		}, 0 // Return 0 cached messages so caller re-templates everything
	}
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
			m.log(ctx, "cache", "status", "cache hit", "role", msgInfo.role, "seq", seqID, "tokens", currentTokens, "messages", len(messages))
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
		m.log(ctx, "cache", "status", "hit (after lock)", "role", msgInfo.role, "seq", seqID, "tokens", *tokensPtr)
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

	tokens := llama.Tokenize(m.vocab, prefixPrompt, m.addBOSToken, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: message tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "cache", "status", "skip (too short)", "role", msgInfo.role, "seq", seqID, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
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
// Uses explicit sequence ID assignment to ensure tokens go into the correct
// cache sequence (important for multi-user IMC).
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

	m.log(ctx, "cache", "status", "decoding started", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	// Lock to prevent concurrent decode with batch engine.
	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	// Create batch with explicit sequence ID. llama.BatchGetOne uses seq 0 by
	// default, which breaks multi-user IMC where each session has its own seq.
	nCtx := llama.NCtx(m.lctx)
	batch := llama.BatchInit(int32(nCtx), 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{seqID}

	for i := 0; i < nTokens; i += nBatch {
		batchClear(&batch)

		end := min(i+nBatch, nTokens)
		for j := i; j < end; j++ {
			// Only compute logits for the last token of the last chunk.
			logits := j == nTokens-1
			batchAdd(&batch, tokens[j], llama.Pos(j), seqIDs, logits)
		}

		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return fmt.Errorf("cache: failed to decode tokens at pos %d: %w", i, err)
		}
	}

	m.log(ctx, "cache", "status", "decoding ended", "seq", seqID, "tokens", nTokens)
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

	m.log(ctx, "imc", "status", "session created", "imc_id", imcID, "seq", session.seqID, "total-sessions", len(m.imcSessions))

	return session, true
}

// =============================================================================

// buildIMCCache builds the cache from scratch using the full templated prompt.
// It caches all content except the last message (suffix for generation).
func (m *Model) buildIMCCache(ctx context.Context, d D, fullPrompt string, media [][]byte, imcID string, session *imcSession) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Clear existing cache sequence.
	llama.MemorySeqRm(m.mem, session.seqID, -1, -1)

	// Extract prefix (everything except the generation prompt suffix).
	// We need to template without add_generation_prompt to get the prefix.
	messages, _ := d["messages"].([]D)
	msgCount := len(messages) - 1

	prefixD := D{
		"messages":              messages[:msgCount],
		"add_generation_prompt": false,
	}
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	prefixPrompt, _, err := m.createPrompt(ctx, prefixD)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template prefix: %w", err)}
	}

	// Check if the prefix is actually a prefix of the full prompt.
	// Some templates (e.g., gpt-oss.jinja) produce different output when
	// messages are templated separately vs together. When this happens,
	// we still cache the prefix and process the full prompt without caching.
	prefixMatches := strings.HasPrefix(fullPrompt, prefixPrompt)
	if !prefixMatches {
		m.log(ctx, "imc", "status", "build (prefix mismatch)", "imc_id", imcID,
			"prefix-len", len(prefixPrompt), "full-len", len(fullPrompt))
	}

	tokens := llama.Tokenize(m.vocab, prefixPrompt, m.addBOSToken, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: prefix tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "imc", "status", "skip (too short)", "imc_id", imcID, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	// Decode tokens into cache sequence.
	if err := m.decodeTokensToSeq(ctx, tokens, session.seqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	// Update session state with prompt hash for future validation.
	promptHash := hashPrompt(prefixPrompt)
	session.promptHash = promptHash
	session.promptLen = len(prefixPrompt)
	session.tokens = nTokens
	session.msgCount = len(messages)
	session.lastUsed = time.Now()

	// When prefix matches, return the suffix for generation with cached nPast.
	// When prefix doesn't match (template incompatibility), we still cached the
	// prefix for future requests, but must process the full prompt without caching.
	if prefixMatches {
		suffix := fullPrompt[len(prefixPrompt):]

		m.log(ctx, "imc", "status", "built", "imc_id", imcID, "seq", session.seqID,
			"tokens", nTokens, "prompt-len", len(prefixPrompt),
			"suffix-len", len(suffix), "hash", promptHash[:8])

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

	// Prefix mismatch: cached for future, but process full prompt now.
	// Return the full prompt we already templated to avoid re-templating.
	m.log(ctx, "imc", "status", "built (no cache benefit)", "imc_id", imcID, "seq", session.seqID,
		"tokens", nTokens, "prompt-len", len(prefixPrompt), "hash", promptHash[:8])

	return cacheResult{
		modifiedD: d,
		prompt:    fullPrompt,
		media:     media,
	}
}

// extendIMCCache extends the existing cache with new content from the full prompt.
// The caller has already validated that the cached prefix matches.
func (m *Model) extendIMCCache(ctx context.Context, d D, fullPrompt string, media [][]byte, imcID string, session *imcSession) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	cachedPromptLen := session.promptLen
	cachedTokens := session.tokens

	// Extract the new prefix (all messages except last, without generation prompt).
	messages, _ := d["messages"].([]D)
	prefixD := D{
		"messages":              messages[:len(messages)-1],
		"add_generation_prompt": false,
	}
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	newPrefixPrompt, _, err := m.createPrompt(ctx, prefixD)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template new prefix: %w", err)}
	}

	// Verify the new prefix starts with what we cached.
	if !strings.HasPrefix(newPrefixPrompt, fullPrompt[:cachedPromptLen]) {
		// This shouldn't happen since caller validated, but be defensive.
		m.log(ctx, "imc", "status", "extend (prefix changed)", "imc_id", imcID)
		return cacheResult{
			nPast:    llama.Pos(cachedTokens),
			cached:   true,
			imcID:    imcID,
			imcSeqID: session.seqID,
		}
	}

	// Check if there's new content to cache.
	switch {
	case len(newPrefixPrompt) <= cachedPromptLen:
		// No new content - just return cached state.
		m.log(ctx, "imc", "status", "extend (no new content)", "imc_id", imcID,
			"cached-len", cachedPromptLen, "new-len", len(newPrefixPrompt))

		suffix := fullPrompt[cachedPromptLen:]
		return cacheResult{
			modifiedD: d,
			prompt:    suffix,
			media:     media,
			nPast:     llama.Pos(cachedTokens),
			cached:    true,
			imcID:     imcID,
			imcSeqID:  session.seqID,
		}

	default:
		// New content to add to cache.
		extensionPrompt := newPrefixPrompt[cachedPromptLen:]
		extensionTokens := llama.Tokenize(m.vocab, extensionPrompt, false, true)
		nExtTokens := len(extensionTokens)

		if nExtTokens == 0 {
			m.log(ctx, "imc", "status", "extend (zero tokens)", "imc_id", imcID)
			suffix := fullPrompt[cachedPromptLen:]
			return cacheResult{
				modifiedD: d,
				prompt:    suffix,
				media:     media,
				nPast:     llama.Pos(cachedTokens),
				cached:    true,
				imcID:     imcID,
				imcSeqID:  session.seqID,
			}
		}

		// Decode extension tokens into cache sequence.
		if err := m.decodeExtensionTokens(ctx, extensionTokens, session.seqID, cachedTokens); err != nil {
			return cacheResult{modifiedD: d, err: err}
		}

		// Update session state.
		newTokens := cachedTokens + nExtTokens
		newPromptHash := hashPrompt(newPrefixPrompt)
		session.promptHash = newPromptHash
		session.promptLen = len(newPrefixPrompt)
		session.tokens = newTokens
		session.msgCount = len(messages)
		session.lastUsed = time.Now()

		// Suffix is everything after the new prefix.
		suffix := fullPrompt[len(newPrefixPrompt):]

		m.log(ctx, "imc", "status", "extended", "imc_id", imcID, "seq", session.seqID,
			"old-tokens", cachedTokens, "new-tokens", newTokens,
			"ext-tokens", nExtTokens, "suffix-len", len(suffix), "hash", newPromptHash[:8])

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
}

// decodeExtensionTokens decodes additional tokens into an existing cache sequence.
// Unlike decodeTokensToSeq, this does NOT clear the sequence first.
// startPos is the position offset for the new tokens (i.e., existing token count).
func (m *Model) decodeExtensionTokens(ctx context.Context, tokens []llama.Token, seqID llama.SeqId, startPos int) error {
	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch
	}

	m.log(ctx, "imc", "status", "extending", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	// Create batch with explicit sequence ID.
	nCtx := llama.NCtx(m.lctx)
	batch := llama.BatchInit(int32(nCtx), 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{seqID}

	for i := 0; i < nTokens; i += nBatch {
		batchClear(&batch)

		end := min(i+nBatch, nTokens)
		for j := i; j < end; j++ {
			// Position is offset by existing tokens in cache.
			pos := llama.Pos(startPos + j)
			// Only compute logits for the last token of the last chunk.
			logits := j == nTokens-1
			batchAdd(&batch, tokens[j], pos, seqIDs, logits)
		}

		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return fmt.Errorf("imc: failed to decode extension tokens at pos %d: %w", i, err)
		}
	}

	return nil
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

// hashPrompt computes a SHA-256 hash of a prompt string.
// Used by IMC to validate that the cached prefix matches the current prompt.
func hashPrompt(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(h[:])
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

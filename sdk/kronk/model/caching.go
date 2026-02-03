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
	modifiedD D           // D with cached messages removed if cache was used
	prompt    string      // Templated prompt (set when caching occurs)
	media     [][]byte    // Media from templating (set when caching occurs)
	nPast     llama.Pos   // Starting position for new tokens (cumulative from both caches)
	cached    bool        // True if any cache is being used
	err       error       // Any error that occurred
	imcID     string      // IMC session ID
	imcSeqID  llama.SeqId // IMC session's cache sequence
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

	if m.cfg.SystemPromptCache {
		return m.processSystemCache(ctx, d)
	}

	return m.processIncrementalCache(ctx, d)
}

// processSystemCache orchestrates the system prompt caching flow. It examines
// the first message and either caches it (if it's a system message) or reuses
// an existing cache (if the client omitted the system message on a follow-up
// request). The system message is always removed from d after processing.
func (m *Model) processSystemCache(ctx context.Context, d D) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	role, ok := messages[0]["role"].(string)
	if !ok {
		return cacheResult{modifiedD: d}
	}

	content := extractMessageContent(messages[0])
	if content == "" {
		return cacheResult{modifiedD: d}
	}

	sysMsg := cacheableMessage{
		index:   0,
		role:    role,
		content: content,
	}

	var totalNPast llama.Pos
	var anyCached bool

	switch role {
	case RoleSystem:
		result := m.performSystemPromptCache(ctx, d, sysMsg)
		if result.err != nil {
			return result
		}

		// Mark we cached the system prompt and where those system prompt tokens
		// end in the cache.
		if result.cached {
			totalNPast = result.nPast
			anyCached = true
		}

		// Remove the system prompt from the messages.
		d = removeMessagesAtIndices(d, []int{0})

	default:
		// No system message in the input, but cache exists. This means the
		// client did not send the system message again on their next request.

		m.cacheMu.RLock()
		cachedTokens := m.sysPromptTokens
		m.cacheMu.RUnlock()

		if cachedTokens > 0 {
			m.log(ctx, "cache", "status", "cache hit (system prompt excluded on this request)", "tokens", cachedTokens)
			totalNPast = llama.Pos(cachedTokens)
			anyCached = true
		}
	}

	return cacheResult{
		modifiedD: d,
		nPast:     totalNPast,
		cached:    anyCached,
	}
}

// performSystemPromptCache performs the actual caching of a system prompt message.
// It checks for cache hits, and on a miss, templates the system message, tokenizes
// it, and decodes the tokens into sequence 0 for reuse on subsequent requests.
func (m *Model) performSystemPromptCache(ctx context.Context, d D, msgInfo cacheableMessage) cacheResult {
	// Default sequence id for caching.
	const seqID llama.SeqId = 0

	if msgInfo.role != RoleSystem {
		m.log(ctx, "cache", "status", "no system prompt message provided", "role", msgInfo.role)
		return cacheResult{modifiedD: d}
	}

	contentLen := len(msgInfo.content)

	// -------------------------------------------------------------------------
	// Check for cache hit (fast path with read lock).
	// We check the length of the system prompt string first to see if this is a
	// new system prompt before checking with the more accurate hash compare.
	// It's possible a new system prompt is of the same length as the old one.

	m.cacheMu.RLock()
	currentHash := m.sysPromptHash
	currentTokens := m.sysPromptTokens
	currentLen := m.sysPromptLen
	m.cacheMu.RUnlock()

	newHash := hashMessage(msgInfo)

	if currentLen == contentLen && currentHash == newHash && currentTokens > 0 {
		m.log(ctx, "cache", "status", "system prompt cache hit", "role", msgInfo.role, "seq", seqID, "tokens", currentTokens)
		return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
	}

	// -------------------------------------------------------------------------
	// Cache miss - template and cache the message.

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine cached while we waited.
	// Must compute hash here since we skipped it above on length mismatch.
	if m.sysPromptLen == contentLen && m.sysPromptHash == newHash && m.sysPromptTokens > 0 {
		m.log(ctx, "cache", "status", "system prompt cache hit (after lock)", "role", msgInfo.role, "seq", seqID, "tokens", m.sysPromptTokens)
		return cacheResult{nPast: llama.Pos(m.sysPromptTokens), cached: true}
	}

	// Template just the system message since that is what's going into the cache.
	systemMsg := D{
		"messages":              []D{d["messages"].([]D)[0]},
		"add_generation_prompt": false,
	}

	prefixPrompt, _, err := m.createPrompt(ctx, systemMsg)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: failed to template system prompt: %w", err)}
	}

	tokens := llama.Tokenize(m.vocab, prefixPrompt, m.addBOSToken, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("cache: system prompt tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "cache", "status", "system prompt cache skip (too short)", "role", msgInfo.role, "seq", seqID, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	if err := m.decodeTokensToSeq(ctx, tokens, seqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	m.sysPromptHash = newHash
	m.sysPromptTokens = nTokens
	m.sysPromptLen = contentLen

	m.log(ctx, "cache", "status", "system prompt cached", "role", msgInfo.role, "seq", seqID, "tokens", nTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD: d,
		nPast:     llama.Pos(nTokens),
		cached:    true,
	}
}

func (m *Model) processIncrementalCache(ctx context.Context, d D) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	imcID, _ := d["imc_id"].(string)
	if imcID == "" {
		imcID = "default"
		m.log(ctx, "cache", "status", "using default imc id", "reason", "IMC turned on but no imc_id was provided")
	}

	result, cachedMsgCount := m.performIncrementalMessageCache(ctx, d, messages, imcID)
	if result.err != nil {
		return result
	}

	var totalNPast llama.Pos
	var anyCached bool
	var indicesToRemove []int

	if result.cached {
		totalNPast += result.nPast
		anyCached = true

		// IMC caches messages[0:cachedMsgCount]. Remove all cached messages.
		for i := range cachedMsgCount {
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

	return cacheResult{
		modifiedD: d,
		nPast:     totalNPast,
		cached:    anyCached,
	}
}

// performIncrementalMessageCache implements incremental multi-turn caching (IMC)
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
func (m *Model) performIncrementalMessageCache(ctx context.Context, d D, messages []D, imcID string) (cacheResult, int) {
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
		m.log(ctx, "imc", "status", "bypass (slots full)", "imc_id", imcID, "max", m.imcMaxSeqs)
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
				m.log(ctx, "imc", "status", "cache hit", "imc_id", imcID, "seq", seqID, "cached-msgs", currentMsgCount, "tokens", currentTokens)
				return cacheResult{
					nPast:    llama.Pos(currentTokens),
					cached:   true,
					imcID:    imcID,
					imcSeqID: seqID,
				}, currentMsgCount
			}
		}

		// Prefix mismatch - need to rebuild cache.
		m.log(ctx, "imc", "status", "prefix mismatch", "imc_id", imcID, "cached-msgs", currentMsgCount, "request-msgs", nMessages)
	}

	// -------------------------------------------------------------------------
	// Cache miss - build cache from scratch.

	result := m.buildIMCCache(ctx, d, messages, imcID, session, targetCacheEnd)
	return result, targetCacheEnd
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

	tokens := llama.Tokenize(m.vocab, prefixPrompt, m.addBOSToken, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: messages tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "imc", "status", "skip (too short)", "imc_id", imcID, "msgs", targetEnd, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
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
	suffix, media, err := m.generateIMCSuffix(ctx, d, prefixPrompt)
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
		m.log(ctx, "imc", "status", "extend (no content)", "imc_id", imcID, "old-len", oldPrefixLen, "new-len", len(prefixPrompt))
		return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
	}

	// Extract extension from the new prefix.
	extensionPrompt := prefixPrompt[oldPrefixLen:]
	extensionTokens := llama.Tokenize(m.vocab, extensionPrompt, false, true)
	nExtTokens := len(extensionTokens)

	if nExtTokens == 0 {
		m.log(ctx, "imc", "status", "extend (zero tokens)", "imc_id", imcID)
		return cacheResult{nPast: llama.Pos(currentTokens), cached: true}
	}

	// Decode extension tokens into cache sequence, starting after existing tokens.
	if err := m.decodeExtensionTokens(extensionTokens, session.seqID, currentTokens); err != nil {
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
		m.log(ctx, "imc", "status", "prefix (mismatch rebuild)", "imc_id", imcID,
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

	m.log(ctx, "imc", "status", "cache extended", "imc_id", imcID, "seq", session.seqID,
		"cached-msgs", fmt.Sprintf("%d->%d", currentEnd, targetEnd),
		"cached-tokens", fmt.Sprintf("%d->%d (+%d)", currentTokens, newTokens, nExtTokens),
		"suffix-len", len(suffix))

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
// startPos is the position offset for the new tokens (i.e., existing token count).
func (m *Model) decodeExtensionTokens(tokens []llama.Token, seqID llama.SeqId, startPos int) error {
	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch
	}

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

// generateIMCSuffix generates the suffix prompt for un-cached messages.
// The suffix includes messages[cachedEnd:] plus the generation prompt.
func (m *Model) generateIMCSuffix(ctx context.Context, d D, prefixPrompt string) (string, [][]byte, error) {
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
// If no messages remain after removal, adds a default user message prompting the
// agent to greet the user. Mutates d in place.
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

	// If no messages remain, add a prompt for the agent to greet the user.
	if len(newMessages) == 0 {
		newMessages = []D{
			{"role": RoleUser, "content": "Tell the user you are ready to help them."},
		}
	}

	d["messages"] = newMessages

	return d
}

package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// cacheResult contains the results of cache processing.
type cacheResult struct {
	modifiedD      D           // D with cached messages removed if cache was used
	cacheIdx       llama.Pos   // Token position where cached content ends; new tokens start here
	cachedMsgCount int         // Number of messages cached (for IMC removal)
	cacheHit       bool        // True if we reused existing cache (no new decode needed)
	cacheUpdated   bool        // True if we modified the cache (decoded new tokens)
	err            error       // Any error that occurred
	cacheID        string      // Cache session ID (used by both SPC and IMC)
	cacheSeqID     llama.SeqId // Cache session's sequence ID
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
// This function is thread-safe and handles concurrent requests appropriately.
func (m *Model) processCache(ctx context.Context, d D) cacheResult {
	if !m.cfg.SystemPromptCache && !m.cfg.IncrementalCache {
		return cacheResult{modifiedD: d}
	}

	if m.cfg.SystemPromptCache {
		return m.processSPC(ctx, d)
	}

	return m.processIMC(ctx, d)
}

// processSPC orchestrates the system prompt caching flow. It examines
// the first message and either caches it (if it's a system message) or reuses
// an existing cache (if the client omitted the system message on a follow-up
// request). The system message is always removed from d after processing.
//
// Each unique cache_id gets its own session with a dedicated cache sequence,
// allowing multiple users to have their own system prompt caches.
func (m *Model) processSPC(ctx context.Context, d D) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	// Get cache_id for user-based caching (same key used by both SPC and IMC).
	cacheID, _ := d["cache_id"].(string)
	if cacheID == "" {
		cacheID = "default"
		m.log(ctx, "spc", "status", "using default cache id", "reason", "SPC turned on but no cache_id was provided")
	}

	// Look up or create session for this cacheID.
	session, _ := m.getOrCreateSPCSession(ctx, cacheID)
	if session == nil {
		// All session slots are in use - bypass SPC gracefully.
		m.log(ctx, "spc", "status", "bypass (slots full)", "cache_id", cacheID, "max-possible-ids", m.spcMaxSeqs)
		return cacheResult{modifiedD: d}
	}

	role, ok := messages[0]["role"].(string)
	if !ok {
		return cacheResult{modifiedD: d}
	}

	// We need to use this function because we don't know what
	// format that first message is in. Input messages can be
	// in different formats.
	content := extractMessageContent(messages[0])
	if content == "" {
		return cacheResult{modifiedD: d}
	}

	sysMsg := cacheableMessage{
		index:   0,
		role:    role,
		content: content,
	}

	var totalCacheIdx llama.Pos
	var cacheHit bool
	var cacheUpdated bool

	switch role {
	case RoleSystem:
		result := m.performSPC(ctx, d, messages, sysMsg, cacheID, session)
		if result.err != nil {
			return result
		}

		// Mark we cached the system prompt and where those system prompt tokens
		// end in the cache.
		if result.cacheIdx > 0 {
			totalCacheIdx = result.cacheIdx
			cacheHit = result.cacheHit
			cacheUpdated = result.cacheUpdated
		}

		// Remove the system prompt from the messages.
		d = removeMessagesAtIndices(d, []int{0})

	default:
		// No system message in the input, but cache exists. This means the
		// client did not send the system message again on their next request.

		m.cacheMu.RLock()
		cachedTokens := session.sysPromptTokens
		seqID := session.seqID
		m.cacheMu.RUnlock()

		if cachedTokens > 0 {
			m.log(ctx, "spc", "status", "cache hit (system prompt excluded on this request)", "cache_id", cacheID, "seq", seqID, "tokens", cachedTokens)
			totalCacheIdx = llama.Pos(cachedTokens)
			cacheHit = true
		}
	}

	return cacheResult{
		modifiedD:    d,
		cacheIdx:     totalCacheIdx,
		cacheHit:     cacheHit,
		cacheUpdated: cacheUpdated,
		cacheID:      cacheID,
		cacheSeqID:   session.seqID,
	}
}

// performSPC performs the actual caching of a system prompt message.
// It checks for cache hits, and on a miss, templates the system message, tokenizes
// it, and decodes the tokens into the session's cache sequence for reuse on
// subsequent requests.
func (m *Model) performSPC(ctx context.Context, d D, messages []D, msgInfo cacheableMessage, cacheID string, session *spcSession) cacheResult {
	if msgInfo.role != RoleSystem {
		m.log(ctx, "spc", "status", "no system prompt message provided", "role", msgInfo.role)
		return cacheResult{modifiedD: d}
	}

	contentLen := len(msgInfo.content)

	// -------------------------------------------------------------------------
	// Check for cache hit (fast path with read lock).
	// We check the length of the system prompt string first to see if this is a
	// new system prompt before checking with the more accurate hash compare.
	// It's possible a new system prompt is of the same length as the old one.

	m.cacheMu.RLock()
	currentHash := session.sysPromptHash
	currentTokens := session.sysPromptTokens
	currentLen := session.sysPromptLen
	seqID := session.seqID
	m.cacheMu.RUnlock()

	newHash := hashMessage(msgInfo)

	if currentLen == contentLen && currentHash == newHash && currentTokens > 0 {
		m.log(ctx, "spc", "status", "cache hit", "cache_id", cacheID, "seq", seqID, "tokens", currentTokens)
		return cacheResult{
			cacheIdx:   llama.Pos(currentTokens),
			cacheHit:   true,
			cacheID:    cacheID,
			cacheSeqID: seqID,
		}
	}

	// -------------------------------------------------------------------------
	// Cache miss - template and cache the message.

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine cached while we waited.
	// Must compute hash here since we skipped it above on length mismatch.
	if session.sysPromptLen == contentLen && session.sysPromptHash == newHash && session.sysPromptTokens > 0 {
		m.log(ctx, "spc", "status", "cache hit (after lock)", "cache_id", cacheID, "seq", seqID, "tokens", session.sysPromptTokens)
		return cacheResult{
			cacheIdx:   llama.Pos(session.sysPromptTokens),
			cacheHit:   true,
			cacheID:    cacheID,
			cacheSeqID: seqID,
		}
	}

	// We just need to cache the system message, so create a D with
	// this message.
	msgsToCache := D{
		"messages":              []D{messages[0]},
		"add_generation_prompt": false,
	}

	systemMsgPrompt, _, err := m.createPrompt(ctx, msgsToCache)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("spc: failed to template system prompt: %w", err)}
	}

	tokens := llama.Tokenize(m.vocab, systemMsgPrompt, m.addBOSToken, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("spc: system prompt tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "spc", "status", "cache skip (too short)", "cache_id", cacheID, "seq", seqID, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	if err := m.addTokensToCache(ctx, tokens, seqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	session.sysPromptHash = newHash
	session.sysPromptTokens = nTokens
	session.sysPromptLen = contentLen
	session.lastUsed = time.Now()

	m.log(ctx, "spc", "status", "cached", "cache_id", cacheID, "seq", seqID, "tokens", nTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD:    d,
		cacheIdx:     llama.Pos(nTokens),
		cacheUpdated: true,
		cacheID:      cacheID,
		cacheSeqID:   seqID,
	}
}

// processIMC implements incremental multi-turn caching (IMC) for agentic
// workflows. It caches all messages except the last one (which triggers
// generation) and extends the cache incrementally on subsequent requests.
//
// Each unique cache_id gets its own session with a dedicated cache sequence.
//
// Algorithm:
//  1. Look up or create session for cache_id
//  2. If session cache is empty, cache messages[0:len-1]
//  3. If session cache exists, check if cached messages match current request
//  4. On match: extend cache with new messages
//  5. On mismatch: rebuild cache from scratch
func (m *Model) processIMC(ctx context.Context, d D) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	// They may forget to provide an id, so we will use the "default" id. If
	// multiple people are on the system not providing an id, this won't end
	// well. Hahaha
	cacheID, _ := d["cache_id"].(string)
	if cacheID == "" {
		cacheID = "default"
		m.log(ctx, "imc", "status", "using default cache id", "reason", "IMC turned on but no cache_id was provided")
	}

	// We need at least 2 messages to start: one to cache, one to generate from.
	totalMsgs := len(messages)
	if totalMsgs < 2 {
		return cacheResult{modifiedD: d}
	}

	// -------------------------------------------------------------------------
	// Look up or create session for this cacheID.

	session, isNew := m.getOrCreateIMCSession(ctx, cacheID)
	if session == nil {
		// All session slots are in use - bypass IMC gracefully.
		m.log(ctx, "imc", "status", "bypass (slots full)", "cache_id", cacheID, "max-possible-ids", m.imcMaxSeqs)
		return cacheResult{modifiedD: d}
	}

	// -------------------------------------------------------------------------
	// Read current session state (fast path with read lock).

	m.cacheMu.RLock()
	currentCachedMsgsHash := session.cachedMsgsHash
	currentTotalTokensCached := session.totalTokensCached
	currentLastMsgIdxCached := session.lastMsgIdxCached
	seqID := session.seqID
	m.cacheMu.RUnlock()

	// -------------------------------------------------------------------------
	// Check for cache hit on existing prefix.

	// We will cache all messages but the last one.
	lastMsgIdxToCache := totalMsgs - 1

	// If this is not a new session and we have data in the cache.
	if !isNew && currentTotalTokensCached > 0 {

		// Check if the current number of messages in the input has increased.
		// This means we are still processing the same thread.
		if totalMsgs > currentLastMsgIdxCached {
			newMsgsHash := hashMessages(messages[:currentLastMsgIdxCached])

			// Check if the current messages we have cached are the same with
			// the new input request. If nothing has changed with those existing
			// messages we can extend the cache with new messages
			if newMsgsHash == currentCachedMsgsHash {

				// If there are more messages in this request, then we need
				// to extend the cache. I would expect this to be the case
				// everytime.
				if currentLastMsgIdxCached < lastMsgIdxToCache {
					return m.extendIMCCache(ctx, d, messages, cacheID, session, currentLastMsgIdxCached, lastMsgIdxToCache, currentTotalTokensCached)
				}

				// If by some weird circumstance the client sends back the
				// exact same messages as before, we have all that already
				// in the cache. Just return it.

				m.log(ctx, "imc", "status", "cache hit", "cache_id", cacheID, "seq", seqID, "current-msg-idx-cached", currentLastMsgIdxCached, "current-total-tokens-cached", currentTotalTokensCached)

				return cacheResult{
					modifiedD:      removeFirstNMessages(d, currentLastMsgIdxCached),
					cacheIdx:       llama.Pos(currentTotalTokensCached),
					cachedMsgCount: currentLastMsgIdxCached,
					cacheHit:       true,
					cacheID:        cacheID,
					cacheSeqID:     seqID,
				}
			}
		}

		// Prefix mismatch - need to rebuild cache.
		m.log(ctx, "imc", "status", "prefix mismatch", "cache_id", cacheID, "current-last-msg-idx-cached", currentLastMsgIdxCached, "total-msgs", totalMsgs)
	}

	// -------------------------------------------------------------------------
	// Cache miss - build cache from scratch.

	return m.buildIMCCacheFromScratch(ctx, d, messages, cacheID, session, lastMsgIdxToCache)
}

// getOrCreateIMCSession looks up an existing session by cacheID or creates a new one.
// Returns nil if all session slots are in use (graceful bypass).
// Returns the session and true if it was newly created.
func (m *Model) getOrCreateIMCSession(ctx context.Context, cacheID string) (*imcSession, bool) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Check if session already exists.
	if session, exists := m.imcSessions[cacheID]; exists {
		session.lastUsed = time.Now()
		return session, false
	}

	// Check if we have room for a new session.
	if int(m.imcNextSeq) >= m.imcMaxSeqs {
		// All slots are in use. Could implement LRU eviction here in the future.
		return nil, false
	}

	// Create new session with next available sequence.
	session := imcSession{
		seqID:    m.imcNextSeq,
		lastUsed: time.Now(),
	}

	m.imcSessions[cacheID] = &session
	m.imcNextSeq++

	m.log(ctx, "imc", "status", "session created", "cache_id", cacheID, "seq", session.seqID, "total-sessions", len(m.imcSessions))

	return &session, true
}

// getOrCreateSPCSession looks up an existing session by cacheID or creates a new one.
// Returns nil if all session slots are in use (graceful bypass).
// Returns the session and true if it was newly created.
func (m *Model) getOrCreateSPCSession(ctx context.Context, cacheID string) (*spcSession, bool) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Check if session already exists.
	if session, exists := m.spcSessions[cacheID]; exists {
		session.lastUsed = time.Now()
		return session, false
	}

	// Check if we have room for a new session.
	if int(m.spcNextSeq) >= m.spcMaxSeqs {
		// All slots are in use. Could implement LRU eviction here in the future.
		return nil, false
	}

	// Create new session with next available sequence.
	session := spcSession{
		seqID:    m.spcNextSeq,
		lastUsed: time.Now(),
	}

	m.spcSessions[cacheID] = &session
	m.spcNextSeq++

	m.log(ctx, "spc", "status", "session created", "cache_id", cacheID, "seq", session.seqID, "total-sessions", len(m.spcSessions))

	return &session, true
}

// extendIMCCache extends the existing cache with new messages from
// messages[currentLastMsgIdxCached:lastMsgIdxToCache].
func (m *Model) extendIMCCache(ctx context.Context, d D, messages []D, cacheID string, session *imcSession, currentLastMsgIdxCached, lastMsgIdxToCache, currentTotalTokensCached int) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check session state hasn't changed. If it has, fall back to
	// doing a full rebuild. This could happen if two people are using the
	// cache id.
	if session.lastMsgIdxCached != currentLastMsgIdxCached || session.totalTokensCached != currentTotalTokensCached {
		m.cacheMu.Unlock()
		result := m.buildIMCCacheFromScratch(ctx, d, messages, cacheID, session, lastMsgIdxToCache)
		m.cacheMu.Lock()

		return result
	}

	msgs := messages[:lastMsgIdxToCache]

	// We need to template all the messages we want to cache. We don't want
	// the assistant tag produced.
	msgsToCache := D{
		"messages":              msgs,
		"add_generation_prompt": false,
	}

	// Copy tools if present (affects template output).
	if tools, ok := d["tools"]; ok {
		msgsToCache["tools"] = tools
	}

	// Create the prompt and tokenize it. We'll extract only the new tokens
	// beyond what's already cached.
	promptToCache, _, err := m.createPrompt(ctx, msgsToCache)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template prefix: %w", err)}
	}

	allTokens := llama.Tokenize(m.vocab, promptToCache, m.addBOSToken, true)
	totalTokens := len(allTokens)

	// If we don't have more tokens than what's cached, nothing to extend.
	if totalTokens <= currentTotalTokensCached {
		m.log(ctx, "imc", "status", "extend (no new tokens)", "cache_id", cacheID, "cached", currentTotalTokensCached, "total", totalTokens)

		return cacheResult{
			modifiedD:      removeFirstNMessages(d, currentLastMsgIdxCached),
			cacheIdx:       llama.Pos(currentTotalTokensCached),
			cachedMsgCount: currentLastMsgIdxCached,
			cacheHit:       true,
			cacheID:        cacheID,
			cacheSeqID:     session.seqID,
		}
	}

	// Extract only the new tokens beyond what's already cached.
	extensionTokens := allTokens[currentTotalTokensCached:]
	numOfExtTokens := len(extensionTokens)

	m.log(ctx, "imc", "status", "extending cache", "cache_id", cacheID, "new-tokens", numOfExtTokens)

	// Add the new tokens into the cache.
	if err := m.extendTokensInCache(ctx, extensionTokens, session.seqID, currentTotalTokensCached); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	// Update session state.
	newHash := hashMessages(msgs)
	session.cachedMsgsHash = newHash
	session.totalTokensCached = totalTokens
	session.lastMsgIdxCached = lastMsgIdxToCache
	session.lastUsed = time.Now()

	m.log(ctx, "imc", "status", "cache extended", "cache_id", cacheID, "seq", session.seqID,
		"idx", fmt.Sprintf("cur[%d] -> new[%d]", currentLastMsgIdxCached, lastMsgIdxToCache),
		"tokens", fmt.Sprintf("cur[%d] -> new[%d] (+%d)", currentTotalTokensCached, totalTokens, numOfExtTokens))

	return cacheResult{
		modifiedD:      removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:       llama.Pos(totalTokens),
		cachedMsgCount: lastMsgIdxToCache,
		cacheUpdated:   true,
		cacheID:        cacheID,
		cacheSeqID:     session.seqID,
	}
}

// buildIMCCacheFromScratch builds the cache from scratch for messages[0:lastMsgIdxToCache].
func (m *Model) buildIMCCacheFromScratch(ctx context.Context, d D, messages []D, cacheID string, session *imcSession, lastMsgIdxToCache int) cacheResult {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine built the cache while we waited.
	if session.lastMsgIdxCached > 0 && session.totalTokensCached > 0 && session.lastMsgIdxCached <= len(messages) {
		prefixHash := hashMessages(messages[:session.lastMsgIdxCached])
		if prefixHash == session.cachedMsgsHash {
			m.log(ctx, "imc", "status", "cache hit (after-lock)", "cache_id", cacheID, "last-msg-idx-cached", session.lastMsgIdxCached, "total-tokens-cached", session.totalTokensCached)
			return cacheResult{cacheIdx: llama.Pos(session.totalTokensCached), cacheHit: true}
		}
	}

	// Clear existing cache sequence.
	llama.MemorySeqRm(m.mem, session.seqID, -1, -1)

	// We need to cache all the message from the beginning to the target
	// message which should represent the second to last message. We don't
	// want the assistant tag produced.
	msgsToCache := messages[:lastMsgIdxToCache]
	prefixD := D{
		"messages":              msgsToCache,
		"add_generation_prompt": false,
	}

	// Copy tools if present (affects template output).
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	dataToCache, _, err := m.createPrompt(ctx, prefixD)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template messages: %w", err)}
	}

	tokens := llama.Tokenize(m.vocab, dataToCache, m.addBOSToken, true)
	nTokens := len(tokens)

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: messages tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "imc", "status", "skip (too short)", "cache_id", cacheID, "last-msg-index-to-cache", lastMsgIdxToCache, "tokens", nTokens, "cache-min-tokens", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	// Decode tokens into cache sequence.
	if err := m.addTokensToCache(ctx, tokens, session.seqID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	// Update session state.
	newHash := hashMessages(msgsToCache)
	session.cachedMsgsHash = newHash
	session.totalTokensCached = nTokens
	session.lastMsgIdxCached = lastMsgIdxToCache
	session.lastUsed = time.Now()

	m.log(ctx, "imc", "status", "cache built from scratch", "cache_id", cacheID, "seq", session.seqID, "msgs", lastMsgIdxToCache, "tokens", nTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD:      removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:       llama.Pos(nTokens),
		cachedMsgCount: lastMsgIdxToCache,
		cacheUpdated:   true,
		cacheID:        cacheID,
		cacheSeqID:     session.seqID,
	}
}

// extendTokensInCache decodes additional tokens into an existing cache sequence.
// Unlike addTokensToCache, this does NOT clear the sequence first.
// startPos is the position offset for the new tokens (i.e., existing token count).
func (m *Model) extendTokensInCache(ctx context.Context, tokens []llama.Token, seqID llama.SeqId, startPos int) error {
	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch
	}

	m.log(ctx, "cache", "status", "extending tokens in cache", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

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

	m.log(ctx, "cache", "status", "finished (extending tokens in cache)", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	return nil
}

// addTokensToCache decodes tokens into the specified sequence for caching.
// Uses explicit sequence ID assignment to ensure tokens go into the correct
// cache sequence (important for multi-user IMC).
func (m *Model) addTokensToCache(ctx context.Context, tokens []llama.Token, seqID llama.SeqId) error {
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

	m.log(ctx, "cache", "status", "adding tokens to cache", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

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

	return nil
}

// copyCachesToSeq copies cached KV state from a cache sequence to the target
// sequence. Used by both SPC and IMC with their session-specific sequences.
func (m *Model) copyCachesToSeq(dstSeqID llama.SeqId, srcSeqID llama.SeqId) error {
	if !m.cfg.SystemPromptCache && !m.cfg.IncrementalCache {
		return nil
	}

	if err := llama.MemorySeqCp(m.mem, srcSeqID, dstSeqID, -1, -1); err != nil {
		return fmt.Errorf("copy-cache: failed to copy seq %d to %d: %w", srcSeqID, dstSeqID, err)
	}

	return nil
}

// clearCaches clears all cached prompt states.
// This is useful when the model context is reset.
func (m *Model) clearCaches() {
	m.cacheMu.Lock()

	// Clear all IMC sessions.
	for id := range m.imcSessions {
		delete(m.imcSessions, id)
	}

	m.imcNextSeq = 0

	// Clear all SPC sessions.
	for id := range m.spcSessions {
		delete(m.spcSessions, id)
	}

	m.spcNextSeq = 0
	m.cacheMu.Unlock()
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
func hashMessage(cm cacheableMessage) string {
	data := fmt.Sprintf("%s:%s", cm.role, cm.content)
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
		fmt.Fprintf(h, "%d:%s:%s|", i, role, content)
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

// removeFirstNMessages removes the first n messages from d.
// Convenience wrapper around removeMessagesAtIndices for IMC.
func removeFirstNMessages(d D, n int) D {
	indices := make([]int, n)
	for i := range n {
		indices[i] = i
	}
	return removeMessagesAtIndices(d, indices)
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

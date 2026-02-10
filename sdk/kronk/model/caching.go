package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
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

	// SPC shared-sequence fields — tokens to stage into seq 0 at startSlot.
	spcCacheID string        // SPC cache_id for atomic staging
	spcHash    string        // Expected hash of SPC content in seq 0
	spcTokens  []llama.Token // Tokens to stage into seq 0 if needed

	// IMC dedicated slot fields — tokens to decode into slot's sequence.
	imcNewCacheTokens []llama.Token // New tokens to extend the cache (decoded at startSlot)
	imcNewTotalCached int           // Total cached tokens after extension
	imcNewMsgIdx      int           // New lastMsgIdxCached after extension
	imcNewMsgsHash    string        // New cachedMsgsHash after extension
	imcClearSeq       bool          // True if sequence must be cleared before decoding (rebuild from scratch)
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
// Tokens are saved to RAM per cache_id. The actual KV decode happens directly
// into the slot's sequence via decodeSPCToSlot at startSlot time.
func (m *Model) processSPC(ctx context.Context, d D) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	cacheID, _ := d["cache_id"].(string)
	if cacheID == "" {
		cacheID = "default"
		m.log(ctx, "spc", "status", "using default cache id", "reason", "SPC turned on but no cache_id was provided")
	}

	entry := m.getOrCreateSPCEntry(cacheID)

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

	switch role {
	case RoleSystem:
		result := m.performSPC(ctx, d, messages, sysMsg, cacheID, entry)
		if result.err != nil {
			return result
		}

		d = removeMessagesAtIndices(d, []int{0})

		if len(result.spcTokens) > 0 {
			return cacheResult{
				modifiedD:  d,
				cacheIdx:   llama.Pos(len(result.spcTokens)),
				cacheHit:   result.cacheHit,
				spcCacheID: cacheID,
				spcHash:    result.spcHash,
				spcTokens:  result.spcTokens,
			}
		}

		return cacheResult{modifiedD: d}

	default:
		m.cacheMu.RLock()
		savedTokens := entry.tokens
		savedHash := entry.hash
		m.cacheMu.RUnlock()

		if len(savedTokens) > 0 {
			m.log(ctx, "spc", "status", "cache hit (system prompt excluded on this request)", "cache_id", cacheID, "tokens", len(savedTokens))
			return cacheResult{
				modifiedD:  d,
				cacheIdx:   llama.Pos(len(savedTokens)),
				cacheHit:   true,
				spcCacheID: cacheID,
				spcHash:    savedHash,
				spcTokens:  savedTokens,
			}
		}
	}

	return cacheResult{modifiedD: d}
}

// performSPC checks for a cache hit on the system prompt. On a miss, it
// templates and tokenizes the message, saving the tokens to RAM in the
// spcEntry. No KV decode happens here — that is deferred to decodeSPCToSlot
// which runs in the batch engine goroutine at startSlot time.
func (m *Model) performSPC(ctx context.Context, d D, messages []D, msgInfo cacheableMessage, cacheID string, entry *spcEntry) cacheResult {
	if msgInfo.role != RoleSystem {
		m.log(ctx, "spc", "status", "no system prompt message provided", "role", msgInfo.role)
		return cacheResult{modifiedD: d}
	}

	contentLen := len(msgInfo.content)

	m.cacheMu.RLock()
	currentHash := entry.hash
	currentTokens := entry.tokens
	currentLen := entry.len
	m.cacheMu.RUnlock()

	newHash := hashMessage(msgInfo)

	if currentLen == contentLen && currentHash == newHash && len(currentTokens) > 0 {
		m.log(ctx, "spc", "status", "cache hit", "cache_id", cacheID, "tokens", len(currentTokens))
		return cacheResult{
			cacheHit:  true,
			spcHash:   newHash,
			spcTokens: currentTokens,
		}
	}

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	if entry.len == contentLen && entry.hash == newHash && len(entry.tokens) > 0 {
		m.log(ctx, "spc", "status", "cache hit (after lock)", "cache_id", cacheID, "tokens", len(entry.tokens))
		return cacheResult{
			cacheHit:  true,
			spcHash:   newHash,
			spcTokens: entry.tokens,
		}
	}

	msgsToCache := D{
		"messages":              []D{messages[0]},
		"add_generation_prompt": false,
	}

	systemMsgPrompt, _, err := m.createPrompt(ctx, msgsToCache)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("spc: failed to template system prompt: %w", err)}
	}

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-spc",
		attribute.String("cache-type", "spc"),
	)

	tokens := llama.Tokenize(m.vocab, systemMsgPrompt, m.addBOSToken, true)
	nTokens := len(tokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", nTokens))
	tokenSpan.End()

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("spc: system prompt tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "spc", "status", "cache skip (too short)", "cache_id", cacheID, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	entry.hash = newHash
	entry.tokens = tokens
	entry.len = contentLen

	m.log(ctx, "spc", "status", "tokens saved", "cache_id", cacheID, "tokens", nTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD: d,
		spcHash:   newHash,
		spcTokens: tokens,
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

	// Create new session bound to a dedicated slot/sequence.
	slotID := int(m.imcNextSeq)
	session := imcSession{
		seqID:    llama.SeqId(slotID),
		slotID:   slotID,
		lastUsed: time.Now(),
	}

	m.imcSessions[cacheID] = &session
	m.imcNextSeq++

	m.log(ctx, "imc", "status", "session created", "cache_id", cacheID, "seq", session.seqID, "total-sessions", len(m.imcSessions))

	return &session, true
}

// getOrCreateSPCEntry looks up an existing entry by cacheID or creates a new one.
func (m *Model) getOrCreateSPCEntry(cacheID string) *spcEntry {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	if entry, exists := m.spcEntries[cacheID]; exists {
		return entry
	}

	entry := &spcEntry{}
	m.spcEntries[cacheID] = entry
	return entry
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

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-imc-extend",
		attribute.String("cache-type", "imc-extend"),
	)

	allTokens := llama.Tokenize(m.vocab, promptToCache, m.addBOSToken, true)
	totalTokens := len(allTokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", totalTokens))
	tokenSpan.End()

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

	m.log(ctx, "imc", "status", "extending cache (deferred)", "cache_id", cacheID, "new-tokens", numOfExtTokens)

	// Compute new session state to be applied after decode in startSlot.
	newHash := hashMessages(msgs)

	m.log(ctx, "imc", "status", "cache extend prepared", "cache_id", cacheID, "seq", session.seqID,
		"idx", fmt.Sprintf("cur[%d] -> new[%d]", currentLastMsgIdxCached, lastMsgIdxToCache),
		"tokens", fmt.Sprintf("cur[%d] -> new[%d] (+%d)", currentTotalTokensCached, totalTokens, numOfExtTokens))

	return cacheResult{
		modifiedD:         removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:          llama.Pos(currentTotalTokensCached),
		cachedMsgCount:    lastMsgIdxToCache,
		cacheHit:          true,
		cacheID:           cacheID,
		cacheSeqID:        session.seqID,
		imcNewCacheTokens: extensionTokens,
		imcNewTotalCached: totalTokens,
		imcNewMsgIdx:      lastMsgIdxToCache,
		imcNewMsgsHash:    newHash,
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

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-imc-scratch",
		attribute.String("cache-type", "imc-build"),
	)

	tokens := llama.Tokenize(m.vocab, dataToCache, m.addBOSToken, true)
	nTokens := len(tokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", nTokens))
	tokenSpan.End()

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: messages tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "imc", "status", "skip (too short)", "cache_id", cacheID, "last-msg-index-to-cache", lastMsgIdxToCache, "tokens", nTokens, "cache-min-tokens", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	// Reset session state immediately so startSlot doesn't read stale values.
	session.totalTokensCached = 0
	session.lastMsgIdxCached = 0
	session.cachedMsgsHash = ""

	// Return tokens for deferred decode in startSlot.
	newHash := hashMessages(msgsToCache)

	m.log(ctx, "imc", "status", "cache build prepared", "cache_id", cacheID, "seq", session.seqID, "msgs", lastMsgIdxToCache, "tokens", nTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD:         removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:          0,
		cachedMsgCount:    lastMsgIdxToCache,
		cacheID:           cacheID,
		cacheSeqID:        session.seqID,
		imcNewCacheTokens: tokens,
		imcNewTotalCached: nTokens,
		imcNewMsgIdx:      lastMsgIdxToCache,
		imcNewMsgsHash:    newHash,
		imcClearSeq:       true,
	}
}

// decodeTokensIntoCache decodes tokens into a cache sequence starting at startPos.
// Unlike addTokensToCache, this does NOT clear the sequence first — the caller
// is responsible for clearing if needed (e.g., rebuild from scratch).
func (m *Model) decodeTokensIntoCache(ctx context.Context, tokens []llama.Token, seqID llama.SeqId, startPos int) error {
	ctx, decodeSpan := otel.AddSpan(ctx, "cache-decode",
		attribute.Int("tokens", len(tokens)),
	)
	defer decodeSpan.End()

	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch
	}

	m.log(ctx, "cache", "status", "decoding tokens into cache", "seq", seqID, "tokens", nTokens, "start_pos", startPos, "nbatch", nBatch)

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	// Create batch with explicit sequence ID.
	// Allocate batch sized to nBatch (not nCtx) to avoid huge allocations for
	// large context windows that can cause C-side allocation failures.
	batchSize := int32(min(nBatch, nTokens))
	if batchSize <= 0 {
		batchSize = 1
	}
	batch := llama.BatchInit(batchSize, 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{seqID}

	for i := 0; i < nTokens; i += nBatch {
		batch.Clear()

		end := min(i+nBatch, nTokens)

		for j := i; j < end; j++ {
			// Position is offset by existing tokens in cache.
			pos := llama.Pos(startPos + j)

			// Only compute logits for the last token of the last chunk.
			logits := j == nTokens-1

			batch.Add(tokens[j], pos, seqIDs, logits)
		}

		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return fmt.Errorf("imc: failed to decode extension tokens at pos %d: %w", i, err)
		}
	}

	m.log(ctx, "cache", "status", "finished (decoding tokens into cache)", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	return nil
}

// decodeSPCToSlot decodes the saved system prompt tokens directly into the
// specified slot sequence. This avoids needing a dedicated cache sequence —
// tokens are stored in RAM and re-decoded into each slot as needed.
func (m *Model) decodeSPCToSlot(ctx context.Context, tokens []llama.Token, seqID llama.SeqId) (llama.Pos, error) {
	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch
	}

	m.log(ctx, "spc", "status", "decoding to slot", "seq", seqID, "tokens", nTokens)

	batchSize := int32(min(nBatch, nTokens))
	if batchSize <= 0 {
		batchSize = 1
	}
	batch := llama.BatchInit(batchSize, 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{seqID}

	for i := 0; i < nTokens; i += nBatch {
		batch.Clear()

		end := min(i+nBatch, nTokens)
		for j := i; j < end; j++ {
			logits := j == nTokens-1
			batch.Add(tokens[j], llama.Pos(j), seqIDs, logits)
		}

		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return 0, fmt.Errorf("spc: failed to decode tokens at pos %d: %w", i, err)
		}
	}

	return llama.Pos(nTokens), nil
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

	// Clear all SPC entries.
	for id := range m.spcEntries {
		delete(m.spcEntries, id)
	}
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

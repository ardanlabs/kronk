package model

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

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

// extendIMCCache extends the existing cache with new messages from
// messages[currentLastMsgIdxCached:lastMsgIdxToCache].
func (m *Model) extendIMCCache(ctx context.Context, d D, messages []D, cacheID string, session *imcSession, currentLastMsgIdxCached, lastMsgIdxToCache, currentTotalTokensCached int) cacheResult {
	m.cacheMu.Lock()

	// Double-check session state hasn't changed. If it has, fall back to
	// doing a full rebuild. This could happen if two people are using the
	// cache id.
	if session.lastMsgIdxCached != currentLastMsgIdxCached || session.totalTokensCached != currentTotalTokensCached {
		m.cacheMu.Unlock()
		return m.buildIMCCacheFromScratch(ctx, d, messages, cacheID, session, lastMsgIdxToCache)
	}
	defer m.cacheMu.Unlock()

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
			return cacheResult{cacheIdx: llama.Pos(session.totalTokensCached)}
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
			pos := llama.Pos(startPos + j)
			batch.Add(tokens[j], pos, seqIDs, false)
		}

		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return fmt.Errorf("imc: failed to decode extension tokens at pos %d: %w", i, err)
		}
	}

	m.log(ctx, "cache", "status", "finished (decoding tokens into cache)", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	return nil
}

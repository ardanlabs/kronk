package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// processIMC implements incremental multi-turn caching (IMC) for agentic
// workflows. It caches all messages except the last one (which triggers
// generation) and extends the cache incrementally on subsequent requests.
//
// All NSeqMax slots are available for one active cache_id. Each slot
// independently tracks its own conversation branch (hash, token count,
// message index). Sub-agents sharing a cache_id get routed to different
// slots via hash matching.
//
// Algorithm:
//  1. Validate cache_id matches the active cache_id (only one at a time)
//  2. Scan all slots for a prefix hash match against the incoming messages
//  3. On match: extend or reuse the matching slot's cache
//  4. No match: pick an empty slot or evict the LRU slot, rebuild from scratch
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
	// Validate cache_id. Only one cache_id can be active at a time.

	if err := m.validateIMCCacheID(ctx, cacheID); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	// We will cache all messages but the last one.
	lastMsgIdxToCache := totalMsgs - 1

	// -------------------------------------------------------------------------
	// Scan all slots for a prefix hash match.

	m.log(ctx, "imc", "status", "scanning slots", "cache_id", cacheID, "total-msgs", totalMsgs, "msgs-to-cache", lastMsgIdxToCache, "total-slots", len(m.imcSlots))

	m.cacheMu.RLock()

	var bestSlot *imcSession
	var bestCachedMsgsHash string
	var bestTotalTokensCached int
	var bestLastMsgIdxCached int
	var emptySlot *imcSession
	var lruSlot *imcSession

	for _, slot := range m.imcSlots {

		// Skip slots with a build/rebuild in-flight.
		if slot.pending {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] pending (build in-flight)", slot.slotID))
			continue
		}

		// Track first empty slot for fallback.
		if slot.totalTokensCached == 0 {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] empty", slot.slotID))

			if emptySlot == nil {
				emptySlot = slot
			}
			continue
		}

		// Track LRU slot for eviction fallback.
		if lruSlot == nil || slot.lastUsed.Before(lruSlot.lastUsed) {
			lruSlot = slot
		}

		// Skip slots with more cached messages than this request has total.
		if totalMsgs <= slot.lastMsgIdxCached {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] skip (cached-msgs[%d] >= total-msgs[%d])", slot.slotID, slot.lastMsgIdxCached, totalMsgs))
			continue
		}

		// Check if this slot's cached prefix matches the incoming messages.
		prefixHash := hashMessages(messages[:slot.lastMsgIdxCached])
		if prefixHash != slot.cachedMsgsHash {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] mismatch (cached-msgs[%d] tokens[%d] hash[%s..] != [%s..])",
				slot.slotID, slot.lastMsgIdxCached, slot.totalTokensCached, slot.cachedMsgsHash[:8], prefixHash[:8]))
			continue
		}

		m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] MATCH (cached-msgs[%d] tokens[%d] hash[%s..])",
			slot.slotID, slot.lastMsgIdxCached, slot.totalTokensCached, slot.cachedMsgsHash[:8]))

		// This slot matches. Pick the one with the most cached messages
		// (best prefix coverage).
		if bestSlot == nil || slot.lastMsgIdxCached > bestLastMsgIdxCached {
			bestSlot = slot
			bestCachedMsgsHash = slot.cachedMsgsHash
			bestTotalTokensCached = slot.totalTokensCached
			bestLastMsgIdxCached = slot.lastMsgIdxCached
		}
	}

	m.cacheMu.RUnlock()

	// -------------------------------------------------------------------------
	// Handle matched slot: extend or pure hit.

	if bestSlot != nil {
		m.log(ctx, "imc", "status", "slot matched", "cache_id", cacheID, "slot", bestSlot.slotID, "seq", bestSlot.seqID,
			"cached-msgs", bestLastMsgIdxCached, "cached-tokens", bestTotalTokensCached, "msgs-to-cache", lastMsgIdxToCache)

		// If there are more messages to cache, extend.
		if bestLastMsgIdxCached < lastMsgIdxToCache {
			return m.extendIMCCache(ctx, d, messages, cacheID, bestSlot, bestLastMsgIdxCached, lastMsgIdxToCache, bestTotalTokensCached)
		}

		// Exact same messages as before — pure cache hit.
		m.log(ctx, "imc", "status", "cache hit", "cache_id", cacheID, "slot", bestSlot.slotID, "seq", bestSlot.seqID,
			"current-msg-idx-cached", bestLastMsgIdxCached, "current-total-tokens-cached", bestTotalTokensCached,
			"hash", bestCachedMsgsHash[:8])

		return cacheResult{
			modifiedD:      removeFirstNMessages(d, bestLastMsgIdxCached),
			cacheIdx:       llama.Pos(bestTotalTokensCached),
			cachedMsgCount: bestLastMsgIdxCached,
			cacheID:        cacheID,
			cacheSeqID:     bestSlot.seqID,
		}
	}

	// -------------------------------------------------------------------------
	// No match — pick an empty slot or evict LRU.

	m.log(ctx, "imc", "status", "no slot matched", "cache_id", cacheID, "total-msgs", totalMsgs)

	var targetSlot *imcSession

	switch {
	case emptySlot != nil:
		targetSlot = emptySlot
		m.log(ctx, "imc", "status", "using empty slot", "cache_id", cacheID, "slot", targetSlot.slotID)

	case lruSlot != nil:
		targetSlot = lruSlot
		m.log(ctx, "imc", "status", "evicting LRU slot", "cache_id", cacheID, "slot", targetSlot.slotID,
			"evicted-msgs", targetSlot.lastMsgIdxCached, "evicted-tokens", targetSlot.totalTokensCached)

	default:
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: no slots available for cache_id %q", cacheID)}
	}

	return m.buildIMCCacheFromScratch(ctx, d, messages, cacheID, targetSlot, lastMsgIdxToCache)
}

// validateIMCCacheID ensures only one cache_id is active at a time. If a
// different cache_id arrives while the current one has cached data, it returns
// an error. If all slots are empty, it switches to the new cache_id.
func (m *Model) validateIMCCacheID(ctx context.Context, cacheID string) error {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Same cache_id or no active cache_id — allow.
	if m.imcCacheID == "" || m.imcCacheID == cacheID {
		m.imcCacheID = cacheID
		return nil
	}

	// Different cache_id — check if any slot has data.
	for _, slot := range m.imcSlots {
		if slot.totalTokensCached > 0 {
			m.log(ctx, "imc", "status", "cache_id rejected", "active", m.imcCacheID, "requested", cacheID)
			return fmt.Errorf("imc: all slots are bound to cache_id %q, cannot serve cache_id %q", m.imcCacheID, cacheID)
		}
	}

	// All slots empty — switch to new cache_id.
	m.log(ctx, "imc", "status", "switching cache_id", "from", m.imcCacheID, "to", cacheID)
	m.imcCacheID = cacheID

	return nil
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

	m.log(ctx, "imc", "status", "extending cache (deferred)", "cache_id", cacheID, "slot", session.slotID, "new-tokens", numOfExtTokens)

	// Compute new session state to be applied after decode in startSlot.
	newHash := hashMessages(msgs)

	m.log(ctx, "imc", "status", "cache extend prepared", "cache_id", cacheID, "slot", session.slotID, "seq", session.seqID,
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

	// Reset session state and mark pending so concurrent scanners skip this
	// slot while the deferred decode is in-flight. Cleared in startSlot.
	session.totalTokensCached = 0
	session.lastMsgIdxCached = 0
	session.cachedMsgsHash = ""
	session.pending = true

	m.log(ctx, "imc", "status", "slot marked pending", "cache_id", cacheID, "slot", session.slotID, "seq", session.seqID)

	// Return tokens for deferred decode in startSlot.
	newHash := hashMessages(msgsToCache)

	m.log(ctx, "imc", "status", "cache build prepared", "cache_id", cacheID, "slot", session.slotID, "seq", session.seqID, "msgs", lastMsgIdxToCache, "tokens", nTokens, "hash", newHash[:8])

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

package model

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// imcSlotSnapshot holds a point-in-time copy of an imcSession's metadata.
// Used by processIMC to release the read lock before hashing.
type imcSlotSnapshot struct {
	slotID            int
	seqID             llama.SeqId
	cachedMsgsHash    string
	cachedTokens      []llama.Token
	totalTokensCached int
	lastMsgIdxCached  int
	lastUsed          time.Time
	pending           bool
	empty             bool
}

// processIMC implements incremental multi-turn caching (IMC) for agentic
// workflows. It caches all messages except the last one (which triggers
// generation) and extends the cache incrementally on subsequent requests.
//
// All NSeqMax slots are available. Each slot independently tracks its own
// conversation branch (hash, token count, message index). Sub-agents get
// routed to different slots via hash matching.
//
// Algorithm:
//  1. Scan all slots for a prefix hash match against the incoming messages
//  2. On match: extend or reuse the matching slot's cache
//  3. No match: pick an empty slot or evict the LRU slot, rebuild from scratch
func (m *Model) processIMC(ctx context.Context, d D, requestStart time.Time) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	// We need at least 2 messages to start: one to cache, one to generate from.
	totalMsgs := len(messages)
	if totalMsgs < 2 {
		return cacheResult{modifiedD: d}
	}

	// We will cache all messages but the last one.
	lastMsgIdxToCache := totalMsgs - 1

	// -------------------------------------------------------------------------
	// Snapshot slot metadata under RLock, then release before hashing.

	m.log(ctx, "imc", "status", "scanning slots", "total-msgs", totalMsgs, "msgs-to-cache", lastMsgIdxToCache, "total-slots", len(m.imcSlots))

	m.cacheMu.RLock()

	snapshots := make([]imcSlotSnapshot, len(m.imcSlots))
	for i, slot := range m.imcSlots {
		snapshots[i] = imcSlotSnapshot{
			slotID:            slot.slotID,
			seqID:             slot.seqID,
			cachedMsgsHash:    slot.cachedMsgsHash,
			cachedTokens:      slot.cachedTokens,
			totalTokensCached: slot.totalTokensCached,
			lastMsgIdxCached:  slot.lastMsgIdxCached,
			lastUsed:          slot.lastUsed,
			pending:           slot.pending,
			empty:             slot.totalTokensCached == 0,
		}
	}

	m.cacheMu.RUnlock()

	// -------------------------------------------------------------------------
	// Scan snapshots and hash outside the lock.

	var bestSlot *imcSession
	var bestCachedMsgsHash string
	var bestTotalTokensCached int
	var bestLastMsgIdxCached int
	var emptySlots []*imcSession
	var lruSlot *imcSession

	for i, snap := range snapshots {

		// Skip slots with a build/rebuild in-flight.
		if snap.pending {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] pending (build in-flight)", snap.slotID))
			continue
		}

		// Track empty slots for fallback.
		if snap.empty {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] empty", snap.slotID))

			emptySlots = append(emptySlots, m.imcSlots[i])
			continue
		}

		// Track LRU slot for eviction fallback.
		if lruSlot == nil || snap.lastUsed.Before(snapshots[lruSlot.slotID].lastUsed) {
			lruSlot = m.imcSlots[i]
		}

		// Skip slots with more cached messages than this request has total.
		if totalMsgs <= snap.lastMsgIdxCached {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] skip (cached-msgs[%d] >= total-msgs[%d])", snap.slotID, snap.lastMsgIdxCached, totalMsgs))
			continue
		}

		// Check if this slot's cached prefix matches the incoming messages.
		prefixHash := hashMessages(messages[:snap.lastMsgIdxCached])
		if prefixHash != snap.cachedMsgsHash {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] mismatch (cached-msgs[%d] tokens[%d] hash[%s..] != [%s..])",
				snap.slotID, snap.lastMsgIdxCached, snap.totalTokensCached, snap.cachedMsgsHash[:8], prefixHash[:8]))
			continue
		}

		m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] MATCH (cached-msgs[%d] tokens[%d] hash[%s..])",
			snap.slotID, snap.lastMsgIdxCached, snap.totalTokensCached, snap.cachedMsgsHash[:8]))

		// This slot matches. Pick the one with the most cached messages
		// (best prefix coverage).
		if bestSlot == nil || snap.lastMsgIdxCached > bestLastMsgIdxCached {
			bestSlot = m.imcSlots[i]
			bestCachedMsgsHash = snap.cachedMsgsHash
			bestTotalTokensCached = snap.totalTokensCached
			bestLastMsgIdxCached = snap.lastMsgIdxCached
		}
	}

	// -------------------------------------------------------------------------
	// Handle matched slot: extend or pure hit.

	if bestSlot != nil {
		m.log(ctx, "imc", "status", "slot matched", "slot", bestSlot.slotID, "seq", bestSlot.seqID,
			"cached-msgs", bestLastMsgIdxCached, "cached-tokens", bestTotalTokensCached, "msgs-to-cache", lastMsgIdxToCache)

		// If there are more messages to cache, extend.
		if bestLastMsgIdxCached < lastMsgIdxToCache {
			return m.extendIMCCache(ctx, d, messages, bestSlot, bestLastMsgIdxCached, lastMsgIdxToCache, bestTotalTokensCached)
		}

		// Exact same messages as before — pure cache hit.
		m.log(ctx, "imc", "status", "cache hit", "slot", bestSlot.slotID, "seq", bestSlot.seqID,
			"current-msg-idx-cached", bestLastMsgIdxCached, "current-total-tokens-cached", bestTotalTokensCached,
			"hash", bestCachedMsgsHash[:8])

		return cacheResult{
			modifiedD:       removeFirstNMessages(d, bestLastMsgIdxCached),
			cacheIdx:        llama.Pos(bestTotalTokensCached),
			cachedMsgCount:  bestLastMsgIdxCached,
			cacheSeqID:      bestSlot.seqID,
			imcSlotID:       bestSlot.slotID,
			imcExpectedHash: bestCachedMsgsHash,
		}
	}

	// -------------------------------------------------------------------------
	// No hash match — try token-level partial prefix matching before falling
	// back to empty slot or LRU eviction. Non-deterministic templates (e.g.,
	// gpt-oss) may produce different token sequences for identical messages,
	// but often share a long common prefix that we can salvage.

	m.log(ctx, "imc", "status", "no slot matched, trying token prefix match", "total-msgs", totalMsgs)

	// Collect non-empty, non-pending slots as candidates for token comparison.
	// Only consider slots where the message count is compatible — the request
	// must have at least as many messages as the slot cached. When the request
	// has fewer messages (e.g., 2 vs 11), it's a new conversation and sharing
	// system prompt tokens from an unrelated conversation is not useful.
	var tokenMatchCandidates []int
	for i, snap := range snapshots {
		if !snap.pending && !snap.empty && len(snap.cachedTokens) > 0 && totalMsgs > snap.lastMsgIdxCached {
			tokenMatchCandidates = append(tokenMatchCandidates, i)
		}
	}

	// If we have candidates, tokenize the current messages and compare.
	if len(tokenMatchCandidates) > 0 {
		msgs := messages[:lastMsgIdxToCache]

		tokenMatchD := D{
			"messages":              msgs,
			"add_generation_prompt": false,
		}

		if tools, ok := d["tools"]; ok {
			tokenMatchD["tools"] = tools
		}

		tokenMatchPrompt, _, tmErr := m.createPrompt(ctx, tokenMatchD)
		if tmErr == nil {
			_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-imc-prefix-match",
				attribute.String("cache-type", "imc-prefix-match"),
			)

			incomingTokens := llama.Tokenize(m.vocab, tokenMatchPrompt, m.addBOSToken, true)

			tokenSpan.SetAttributes(attribute.Int("tokens", len(incomingTokens)))
			tokenSpan.End()

			var bestPartialSlotIdx int
			var bestPartialLen int

			for _, idx := range tokenMatchCandidates {
				snap := snapshots[idx]
				commonLen := tokenPrefixMatch(snap.cachedTokens, incomingTokens)

				pct := 0
				if snap.totalTokensCached > 0 {
					pct = commonLen * 100 / snap.totalTokensCached
				}

				m.log(ctx, "imc", "token-match", fmt.Sprintf("slot[%d] common-prefix %d/%d tokens (%d%% salvageable)",
					snap.slotID, commonLen, snap.totalTokensCached, pct))

				if commonLen > bestPartialLen {
					bestPartialLen = commonLen
					bestPartialSlotIdx = idx
				}
			}

			if bestPartialLen >= m.cfg.CacheMinTokens {
				partialSlot := m.imcSlots[bestPartialSlotIdx]
				discarded := snapshots[bestPartialSlotIdx].totalTokensCached - bestPartialLen
				saved := len(incomingTokens) - bestPartialLen

				m.log(ctx, "imc", "status", "token prefix match found",
					"slot", partialSlot.slotID,
					"common-prefix", bestPartialLen,
					"discarded-cached", discarded,
					"new-tokens-to-decode", saved,
					"total-incoming", len(incomingTokens))

				return m.rebuildIMCFromPartialPrefix(ctx, d, messages, partialSlot, lastMsgIdxToCache, incomingTokens, bestPartialLen)
			}

			m.log(ctx, "imc", "status", "no usable token prefix match",
				"best-prefix", bestPartialLen, "min-required", m.cfg.CacheMinTokens)
		}
	}

	// -------------------------------------------------------------------------
	// No hash match, no token prefix match — pick an empty slot or evict LRU.
	// Try each empty slot in order; if a concurrent request already marked one
	// pending, move to the next.

	for _, slot := range emptySlots {
		m.log(ctx, "imc", "status", "trying empty slot", "slot", slot.slotID)

		result := m.buildIMCCacheFromScratch(ctx, d, messages, slot, lastMsgIdxToCache)
		if !result.imcPending {
			return result
		}

		m.log(ctx, "imc", "status", "empty slot pending, trying next", "slot", slot.slotID)
	}

	if lruSlot != nil {
		m.log(ctx, "imc", "status", "evicting LRU slot", "slot", lruSlot.slotID,
			"evicted-msgs", lruSlot.lastMsgIdxCached, "evicted-tokens", lruSlot.totalTokensCached)

		return m.buildIMCCacheFromScratch(ctx, d, messages, lruSlot, lastMsgIdxToCache)
	}

	// All slots are pending. Wait for one to become available, then retry
	// the entire scan. Use the cacheCond condvar which is broadcast whenever
	// any slot's pending flag is cleared.
	m.log(ctx, "imc", "status", "all slots pending, waiting for slot")

	if err := m.waitForIMCSlot(ctx, requestStart); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	m.log(ctx, "imc", "status", "slot became available, retrying scan")

	return m.processIMC(ctx, d, requestStart)
}

// extendIMCCache extends the existing cache with new messages from
// messages[currentLastMsgIdxCached:lastMsgIdxToCache].
func (m *Model) extendIMCCache(ctx context.Context, d D, messages []D, session *imcSession, currentLastMsgIdxCached, lastMsgIdxToCache, currentTotalTokensCached int) cacheResult {

	// Reserve the slot under lock. Validate state hasn't changed and mark
	// pending so concurrent scanners skip this slot during the heavy work.
	m.cacheMu.Lock()

	if session.lastMsgIdxCached != currentLastMsgIdxCached || session.totalTokensCached != currentTotalTokensCached {
		m.cacheMu.Unlock()
		return m.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
	}

	if session.pending {
		m.cacheMu.Unlock()
		return m.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID
	currentHash := session.cachedMsgsHash

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "slot marked pending (extend)", "slot", slotID, "seq", seqID)

	// -------------------------------------------------------------------------
	// Heavy work: template + tokenize outside the lock.

	msgs := messages[:lastMsgIdxToCache]

	msgsToCache := D{
		"messages":              msgs,
		"add_generation_prompt": false,
	}

	// Copy tools if present (affects template output).
	if tools, ok := d["tools"]; ok {
		msgsToCache["tools"] = tools
	}

	promptToCache, _, err := m.createPrompt(ctx, msgsToCache)
	if err != nil {
		m.cacheMu.Lock()
		session.pending = false
		m.cacheMu.Unlock()
		m.notifyIMCSlotAvailable()

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
		m.log(ctx, "imc", "status", "extend (no new tokens)", "cached", currentTotalTokensCached, "total", totalTokens)

		m.cacheMu.Lock()
		session.pending = false
		m.cacheMu.Unlock()
		m.notifyIMCSlotAvailable()

		return cacheResult{
			modifiedD:       removeFirstNMessages(d, currentLastMsgIdxCached),
			cacheIdx:        llama.Pos(currentTotalTokensCached),
			cachedMsgCount:  currentLastMsgIdxCached,
			cacheSeqID:      seqID,
			imcSlotID:       slotID,
			imcExpectedHash: currentHash,
		}
	}

	// Extract only the new tokens beyond what's already cached.
	extensionTokens := allTokens[currentTotalTokensCached:]
	numOfExtTokens := len(extensionTokens)

	m.log(ctx, "imc", "status", "extending cache (deferred)", "slot", slotID, "new-tokens", numOfExtTokens)

	// Compute new session state to be applied after decode in startSlot.
	newHash := hashMessages(msgs)

	m.log(ctx, "imc", "status", "cache extend prepared", "slot", slotID, "seq", seqID,
		"idx", fmt.Sprintf("cur[%d] -> new[%d]", currentLastMsgIdxCached, lastMsgIdxToCache),
		"tokens", fmt.Sprintf("cur[%d] -> new[%d] (+%d)", currentTotalTokensCached, totalTokens, numOfExtTokens))

	return cacheResult{
		modifiedD:          removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:           llama.Pos(currentTotalTokensCached),
		cachedMsgCount:     lastMsgIdxToCache,
		cacheSeqID:         seqID,
		imcSlotID:          slotID,
		imcExpectedHash:    newHash,
		imcNewCacheTokens:  extensionTokens,
		imcNewTotalCached:  totalTokens,
		imcNewMsgIdx:       lastMsgIdxToCache,
		imcNewMsgsHash:     newHash,
		imcNewCachedTokens: allTokens,
	}
}

// buildIMCCacheFromScratch builds the cache from scratch for messages[0:lastMsgIdxToCache].
func (m *Model) buildIMCCacheFromScratch(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int) cacheResult {

	// Reserve the slot under lock. Check for after-lock cache hit, mark
	// pending, and reset session state before releasing the lock.
	m.cacheMu.Lock()

	// Double-check in case another goroutine built the cache while we waited.
	if session.lastMsgIdxCached > 0 && session.totalTokensCached > 0 && session.lastMsgIdxCached <= len(messages) {
		prefixHash := hashMessages(messages[:session.lastMsgIdxCached])
		if prefixHash == session.cachedMsgsHash {
			m.log(ctx, "imc", "status", "cache hit (after-lock)", "slot", session.slotID, "seq", session.seqID,
				"last-msg-idx-cached", session.lastMsgIdxCached, "total-tokens-cached", session.totalTokensCached)

			lastMsgIdx := session.lastMsgIdxCached
			totalTokens := session.totalTokensCached
			seqID := session.seqID
			sID := session.slotID
			hash := session.cachedMsgsHash

			m.cacheMu.Unlock()

			return cacheResult{
				modifiedD:       removeFirstNMessages(d, lastMsgIdx),
				cacheIdx:        llama.Pos(totalTokens),
				cachedMsgCount:  lastMsgIdx,
				cacheSeqID:      seqID,
				imcSlotID:       sID,
				imcExpectedHash: hash,
			}
		}
	}

	if session.pending {
		m.cacheMu.Unlock()

		return cacheResult{modifiedD: d, imcPending: true, err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	// Reset session state and mark pending so concurrent scanners skip this
	// slot while we do the heavy work outside the lock.
	session.totalTokensCached = 0
	session.lastMsgIdxCached = 0
	session.cachedMsgsHash = ""
	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "slot marked pending", "slot", slotID, "seq", seqID)

	// -------------------------------------------------------------------------
	// Heavy work: template + tokenize outside the lock.

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
		m.cacheMu.Lock()
		session.pending = false
		m.cacheMu.Unlock()
		m.notifyIMCSlotAvailable()

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
		m.cacheMu.Lock()
		session.pending = false
		m.cacheMu.Unlock()
		m.notifyIMCSlotAvailable()

		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: messages tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "imc", "status", "skip (too short)", "last-msg-index-to-cache", lastMsgIdxToCache, "tokens", nTokens, "cache-min-tokens", m.cfg.CacheMinTokens)

		m.cacheMu.Lock()
		session.pending = false
		m.cacheMu.Unlock()
		m.notifyIMCSlotAvailable()

		return cacheResult{modifiedD: d}
	}

	// Return tokens for deferred decode in startSlot.
	newHash := hashMessages(msgsToCache)

	m.log(ctx, "imc", "status", "cache build prepared", "slot", slotID, "seq", seqID, "msgs", lastMsgIdxToCache, "tokens", nTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD:          removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:           0,
		cachedMsgCount:     lastMsgIdxToCache,
		cacheSeqID:         seqID,
		imcSlotID:          slotID,
		imcExpectedHash:    newHash,
		imcNewCacheTokens:  tokens,
		imcNewTotalCached:  nTokens,
		imcNewMsgIdx:       lastMsgIdxToCache,
		imcNewMsgsHash:     newHash,
		imcClearSeq:        true,
		imcNewCachedTokens: tokens,
	}
}

// rebuildIMCFromPartialPrefix rebuilds a slot's cache using a token-level
// partial prefix match. When a non-deterministic template produces different
// tokens for the same messages, this function salvages the longest common
// prefix in the KV cache, trims the divergent suffix, and decodes only the
// new tokens from the divergence point forward.
func (m *Model) rebuildIMCFromPartialPrefix(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int, allTokens []llama.Token, commonPrefixLen int) cacheResult {

	// Reserve the slot under lock.
	m.cacheMu.Lock()

	if session.pending {
		m.cacheMu.Unlock()
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "slot marked pending (partial prefix)", "slot", slotID, "seq", seqID)

	// Extract only the tokens beyond the common prefix.
	extensionTokens := allTokens[commonPrefixLen:]
	totalTokens := len(allTokens)

	msgsToCache := messages[:lastMsgIdxToCache]
	newHash := hashMessages(msgsToCache)

	m.log(ctx, "imc", "status", "partial prefix rebuild prepared", "slot", slotID, "seq", seqID,
		"common-prefix", commonPrefixLen, "extension-tokens", len(extensionTokens),
		"total-tokens", totalTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD:          removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:           llama.Pos(commonPrefixLen),
		cachedMsgCount:     lastMsgIdxToCache,
		cacheSeqID:         seqID,
		imcSlotID:          slotID,
		imcExpectedHash:    newHash,
		imcNewCacheTokens:  extensionTokens,
		imcNewTotalCached:  totalTokens,
		imcNewMsgIdx:       lastMsgIdxToCache,
		imcNewMsgsHash:     newHash,
		imcTrimPos:         llama.Pos(commonPrefixLen),
		imcNewCachedTokens: allTokens,
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

// waitForIMCSlot blocks until at least one IMC slot is no longer pending,
// the context is canceled, or the wait timeout expires. It uses the cacheCond
// condvar which is broadcast whenever any slot's pending flag is cleared.
// The timeout is the remaining time from the shared CacheSlotTimeout budget
// that started at requestStart.
func (m *Model) waitForIMCSlot(ctx context.Context, requestStart time.Time) error {
	timeout := time.Duration(m.cfg.CacheSlotTimeout) * time.Second
	remaining := timeout - time.Since(requestStart)

	if remaining <= 0 {
		return fmt.Errorf("server busy processing other requests, try again shortly")
	}

	// Spawn a goroutine that waits on the cond var and signals a channel.
	// This lets us select on both the cond signal and context cancellation.
	ready := make(chan struct{}, 1)

	go func() {
		m.cacheMu.Lock()
		defer m.cacheMu.Unlock()

		for {
			for _, slot := range m.imcSlots {
				if !slot.pending {
					select {
					case ready <- struct{}{}:
					default:
					}
					return
				}
			}

			// Check context so we don't loop forever after cancellation.
			if ctx.Err() != nil {
				return
			}

			m.cacheCond.Wait()
		}
	}()

	timer := time.NewTimer(remaining)
	defer timer.Stop()

	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		m.cacheCond.Broadcast()
		return fmt.Errorf("imc: context canceled while waiting for slot: %w", ctx.Err())
	case <-timer.C:
		m.cacheCond.Broadcast()
		return fmt.Errorf("server busy processing other requests, try again shortly")
	}
}

// notifyIMCSlotAvailable broadcasts to any goroutines waiting for a slot to
// become available. Must be called after clearing a slot's pending flag.
func (m *Model) notifyIMCSlotAvailable() {
	if m.cacheCond != nil {
		m.cacheCond.Broadcast()
	}
}

// tokenPrefixMatch returns the number of tokens that match between two slices,
// starting from the beginning. Used to find the longest common prefix between
// a slot's cached tokens and a new request's tokens.
func tokenPrefixMatch(cached, incoming []llama.Token) int {
	n := min(len(cached), len(incoming))
	for i := range n {
		if cached[i] != incoming[i] {
			return i
		}
	}
	return n
}

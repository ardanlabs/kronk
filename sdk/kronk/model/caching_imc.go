package model

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// =============================================================================
// Incremental Message Cache (IMC) — Core Algorithm
//
// IMC has two matching strategies, automatically selected based on template
// behavior:
//
//   - Deterministic:     Hash-based prefix matching for models with consistent
//     templates (QWEN, Llama, MoE, Hybrid). Fastest path.
//   - Non-Deterministic: Token-level prefix fallback for models with variable
//     templates (GPT-OSS, GLM). Activated when hash matching fails.
//
// The matching strategy is independent of the model type (Dense, MoE, Hybrid).
// All model types use the same caching functions in this file. What changes
// per model type is how the batch engine manages state between requests:
//
//   - Dense/MoE: batch_finish.go trims generated tokens via partial range
//     delete (MemorySeqRm).
//   - Hybrid: batch_slot_start.go captures a snapshot after cache build;
//     batch_finish.go restores the snapshot instead of trimming.
//
// All functions in this file are shared: processIMC (slot scan + hash
// matching), extendIMCCache, buildIMCCacheFromScratch, and
// rebuildIMCFromPartialPrefix.
// =============================================================================

// imcSlotSnapshot holds a point-in-time copy of an imcSession's metadata.
// Used by processIMC to release the read lock before hashing.
type imcSlotSnapshot struct {
	slotID            int
	seqID             llama.SeqId
	cachedMsgsHash    string
	cachedTokens      []llama.Token
	totalTokensCached int
	cachedMsgCount    int
	lastUsed          time.Time
	pending           bool
	empty             bool
	hasMedia          bool
}

// processIMC implements incremental multi-turn caching (IMC) for agentic
// workflows. It caches all messages except the last one (which triggers
// generation) and extends the cache incrementally on subsequent requests.
//
// This function implements the shared slot selection algorithm used by both
// IMC strategies (Deterministic and Non-Deterministic) across all model
// types (Dense, MoE, Hybrid). All NSeqMax slots are available. Each slot
// independently tracks its own conversation branch (hash, token count,
// message index). Sub-agents get routed to different slots via hash matching.
//
// Algorithm:
//  1. Scan all slots for a prefix hash match (Deterministic path)
//  2. On match: extend or reuse the matching slot's cache
//  3. No hash match: try token prefix fallback (Non-Deterministic path)
//  4. No match at all: pick an empty slot or evict the LRU slot, rebuild
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
			cachedMsgCount:    slot.cachedMsgCount,
			lastUsed:          slot.lastUsed,
			pending:           slot.pending,
			empty:             slot.totalTokensCached == 0,
			hasMedia:          slot.hasMedia,
		}
	}

	m.cacheMu.RUnlock()

	// -------------------------------------------------------------------------
	// Step 1: Hash-based slot scan (Deterministic path, all model types).

	var bestSlot *imcSession
	var bestCachedMsgsHash string
	var bestTotalTokensCached int
	var bestCachedMsgCount int
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
		if totalMsgs <= snap.cachedMsgCount {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] skip (cached-msgs[%d] >= total-msgs[%d])", snap.slotID, snap.cachedMsgCount, totalMsgs))
			continue
		}

		// Check if this slot's cached prefix matches the incoming messages.
		prefixHash := hashMessages(messages[:snap.cachedMsgCount])
		if prefixHash != snap.cachedMsgsHash {
			m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] mismatch (cached-msgs[%d] tokens[%d] hash[%s..] != [%s..])",
				snap.slotID, snap.cachedMsgCount, snap.totalTokensCached, snap.cachedMsgsHash[:8], prefixHash[:8]))
			continue
		}

		m.log(ctx, "imc", "scan", fmt.Sprintf("slot[%d] MATCH (cached-msgs[%d] tokens[%d] hash[%s..])",
			snap.slotID, snap.cachedMsgCount, snap.totalTokensCached, snap.cachedMsgsHash[:8]))

		// This slot matches. Pick the one with the most cached messages
		// (best prefix coverage).
		if bestSlot == nil || snap.cachedMsgCount > bestCachedMsgCount {
			bestSlot = m.imcSlots[i]
			bestCachedMsgsHash = snap.cachedMsgsHash
			bestTotalTokensCached = snap.totalTokensCached
			bestCachedMsgCount = snap.cachedMsgCount
		}
	}

	// -------------------------------------------------------------------------
	// Step 2: Handle matched slot — extend or pure hit.

	if bestSlot != nil {
		m.log(ctx, "imc", "status", "slot matched", "slot", bestSlot.slotID, "seq", bestSlot.seqID,
			"cached-msgs", bestCachedMsgCount, "cached-tokens", bestTotalTokensCached, "msgs-to-cache", lastMsgIdxToCache)

		// If there are more messages to cache, extend.
		if bestCachedMsgCount < lastMsgIdxToCache {
			return m.extendIMCCache(ctx, d, messages, bestSlot, bestCachedMsgCount, lastMsgIdxToCache, bestTotalTokensCached)
		}

		// Exact same messages as before — pure cache hit.
		m.log(ctx, "imc", "status", "cache hit", "slot", bestSlot.slotID, "seq", bestSlot.seqID,
			"cached-msgs", bestCachedMsgCount, "current-total-tokens-cached", bestTotalTokensCached,
			"hash", bestCachedMsgsHash[:8])

		return cacheResult{
			modifiedD:       removeFirstNMessages(d, bestCachedMsgCount),
			cacheIdx:        llama.Pos(bestTotalTokensCached),
			cachedMsgCount:  bestCachedMsgCount,
			cacheSeqID:      bestSlot.seqID,
			imcSlotID:       bestSlot.slotID,
			imcExpectedHash: bestCachedMsgsHash,
		}
	}

	// -------------------------------------------------------------------------
	// Step 3: Token prefix fallback (Non-Deterministic mode).
	//
	// No hash match — try token-level partial prefix matching before falling
	// back to empty slot or LRU eviction. Non-deterministic templates (e.g.,
	// GPT-OSS, GLM) may produce different token sequences for identical
	// messages, but often share a long common prefix that we can salvage.

	m.log(ctx, "imc", "status", "no slot matched, trying token prefix match", "total-msgs", totalMsgs)

	// Collect non-empty, non-pending slots as candidates for token comparison.
	// Only consider slots where the message count is compatible — the request
	// must have at least as many messages as the slot cached. When the request
	// has fewer messages (e.g., 2 vs 11), it's a new conversation and sharing
	// system prompt tokens from an unrelated conversation is not useful.
	// Skip slots with media: token prefix matching can't compare image
	// embeddings at the token level — only hash matching works for media.
	//
	// Also skip entirely when the incoming request contains media messages.
	// createPrompt/applyRequestJinjaTemplate mutates []byte content to string
	// markers in-place, which would destroy media detection for downstream
	// functions (buildIMCCacheFromScratch) that share the same message maps.
	requestHasMedia := slices.ContainsFunc(messages[:lastMsgIdxToCache], messageHasMedia)

	var tokenMatchCandidates []int
	if !requestHasMedia {
		for i, snap := range snapshots {
			if !snap.pending && !snap.empty && !snap.hasMedia && len(snap.cachedTokens) > 0 && totalMsgs > snap.cachedMsgCount {
				tokenMatchCandidates = append(tokenMatchCandidates, i)
			}
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
	// Step 4: No match — pick an empty slot or evict LRU.
	//
	// No hash match, no token prefix match — try each empty slot in order.
	// If a concurrent request already marked one pending, move to the next.

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
			"evicted-msgs", lruSlot.cachedMsgCount, "evicted-tokens", lruSlot.totalTokensCached)

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
// messages[currentCachedMsgCount:lastMsgIdxToCache].
func (m *Model) extendIMCCache(ctx context.Context, d D, messages []D, session *imcSession, currentCachedMsgCount, lastMsgIdxToCache, currentTotalTokensCached int) cacheResult {

	// When the slot has media cached or any extension messages contain media,
	// determine the appropriate extension strategy.
	if m.projFile != "" {
		// Check if any of the NEW messages being added contain media.
		newMsgsHaveMedia := false
		for i := currentCachedMsgCount; i < lastMsgIdxToCache; i++ {
			if messageHasMedia(messages[i]) {
				newMsgsHaveMedia = true
				break
			}
		}

		// New messages contain media — full rebuild required to re-encode
		// through the mtmd pipeline.
		if newMsgsHaveMedia {
			m.log(ctx, "imc", "status", "extend requires media rebuild (new media)",
				"slot", session.slotID, "cached-msgs", currentCachedMsgCount,
				"target-msgs", lastMsgIdxToCache)

			return m.rebuildIMCWithMedia(ctx, d, messages, session, lastMsgIdxToCache)
		}

		// Slot has media but new messages are text-only — extend with text
		// tokens using mediaKVCounts to compute the correct token offset.
		m.cacheMu.RLock()
		slotHasMedia := session.hasMedia
		slotMediaKVCounts := session.mediaKVCounts
		m.cacheMu.RUnlock()

		if slotHasMedia {
			return m.extendIMCMediaSlotWithText(ctx, d, messages, session,
				currentCachedMsgCount, lastMsgIdxToCache, currentTotalTokensCached,
				slotMediaKVCounts)
		}
	}

	// Reserve the slot under lock. Validate state hasn't changed and mark
	// pending so concurrent scanners skip this slot during the heavy work.
	m.cacheMu.Lock()

	if session.cachedMsgCount != currentCachedMsgCount || session.totalTokensCached != currentTotalTokensCached {
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
		m.imcClearPending(slotID)

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

		m.imcClearPending(slotID)

		return cacheResult{
			modifiedD:       removeFirstNMessages(d, currentCachedMsgCount),
			cacheIdx:        llama.Pos(currentTotalTokensCached),
			cachedMsgCount:  currentCachedMsgCount,
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
		"idx", fmt.Sprintf("cur[%d] -> new[%d]", currentCachedMsgCount, lastMsgIdxToCache),
		"tokens", fmt.Sprintf("cur[%d] -> new[%d] (+%d)", currentTotalTokensCached, totalTokens, numOfExtTokens))

	return cacheResult{
		modifiedD:            removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:             llama.Pos(currentTotalTokensCached),
		cachedMsgCount:       lastMsgIdxToCache,
		cacheSeqID:           seqID,
		imcSlotID:            slotID,
		imcExpectedHash:      newHash,
		imcNewCacheTokens:    extensionTokens,
		imcNewTotalCached:    totalTokens,
		imcNewCachedMsgCount: lastMsgIdxToCache,
		imcNewMsgsHash:       newHash,
		imcNewCachedTokens:   allTokens,
	}
}

// extendIMCMediaSlotWithText extends a media-cached slot with text-only
// messages. The image/audio embeddings remain in the KV cache; only new text
// tokens are decoded. Uses mediaKVCounts to compute the offset between text
// token counts (from tokenization, which sees markers) and KV positions
// (which include image/audio embeddings instead of markers).
func (m *Model) extendIMCMediaSlotWithText(ctx context.Context, d D, messages []D, session *imcSession, currentCachedMsgCount, lastMsgIdxToCache, currentTotalTokensCached int, mediaKVCounts []int) cacheResult {

	// Reserve the slot under lock.
	m.cacheMu.Lock()

	if session.cachedMsgCount != currentCachedMsgCount || session.totalTokensCached != currentTotalTokensCached {
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

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "slot marked pending (media text extend)", "slot", slotID, "seq", seqID)

	// Compute the marker token count lazily (once per model lifetime).
	if m.mediaMarkerTokens == 0 {
		marker := fmt.Sprintf("%s\n", mtmd.DefaultMarker())
		markerTokens := llama.Tokenize(m.vocab, marker, false, false)
		m.mediaMarkerTokens = len(markerTokens)
		m.log(ctx, "imc", "status", "computed media marker tokens", "marker", marker, "tokens", m.mediaMarkerTokens)
	}

	// Compute the KV-to-token offset. Each media chunk occupies mediaKVCounts[i]
	// KV positions but only mediaMarkerTokens text tokens in the tokenized prompt.
	// The delta tells us how to map between token indices and KV positions.
	var totalMediaKV int
	for _, kv := range mediaKVCounts {
		totalMediaKV += kv
	}
	totalMarkerTokens := len(mediaKVCounts) * m.mediaMarkerTokens
	kvTokenDelta := totalMediaKV - totalMarkerTokens

	// cachedTextTokens is the number of text tokens (with markers) that
	// correspond to the cached KV positions.
	cachedTextTokens := currentTotalTokensCached - kvTokenDelta

	// Template and tokenize the full prefix (all messages to cache).
	msgs := messages[:lastMsgIdxToCache]

	// Hash before createPrompt which mutates []byte media content to string
	// markers in-place. Hashing after would exclude raw media bytes, causing
	// mismatches when processIMC hashes fresh []byte content on the next request.
	newHash := hashMessages(msgs)

	msgsToCache := D{
		"messages":              msgs,
		"add_generation_prompt": false,
	}
	if tools, ok := d["tools"]; ok {
		msgsToCache["tools"] = tools
	}

	promptToCache, _, err := m.createPrompt(ctx, msgsToCache)
	if err != nil {
		m.imcClearPending(slotID)
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template prefix (media text extend): %w", err)}
	}

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-imc-media-text-extend",
		attribute.String("cache-type", "imc-media-text-extend"),
	)

	allTokens := llama.Tokenize(m.vocab, promptToCache, m.addBOSToken, true)
	totalTextTokens := len(allTokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", totalTextTokens))
	tokenSpan.End()

	// Extract the extension tokens: everything after the cached text token count.
	if totalTextTokens <= cachedTextTokens {
		m.log(ctx, "imc", "status", "media text extend (no new tokens)",
			"slot", slotID, "cached_text_tokens", cachedTextTokens, "total_text_tokens", totalTextTokens)

		m.imcClearPending(slotID)

		return cacheResult{
			modifiedD:       removeFirstNMessages(d, currentCachedMsgCount),
			cacheIdx:        llama.Pos(currentTotalTokensCached),
			cachedMsgCount:  currentCachedMsgCount,
			cacheSeqID:      seqID,
			imcSlotID:       slotID,
			imcExpectedHash: session.cachedMsgsHash,
		}
	}

	extensionTokens := allTokens[cachedTextTokens:]

	// The new total KV positions = current cached + extension tokens.
	newTotalCached := currentTotalTokensCached + len(extensionTokens)

	m.log(ctx, "imc", "status", "media text extend prepared", "slot", slotID, "seq", seqID,
		"cached_kv", currentTotalTokensCached, "cached_text_tokens", cachedTextTokens,
		"kv_token_delta", kvTokenDelta, "extension_tokens", len(extensionTokens),
		"new_total_kv", newTotalCached)

	return cacheResult{
		modifiedD:            removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:             llama.Pos(currentTotalTokensCached),
		cachedMsgCount:       lastMsgIdxToCache,
		cacheSeqID:           seqID,
		imcSlotID:            slotID,
		imcExpectedHash:      newHash,
		imcNewCacheTokens:    extensionTokens,
		imcNewTotalCached:    newTotalCached,
		imcNewCachedMsgCount: lastMsgIdxToCache,
		imcNewMsgsHash:       newHash,
		imcMediaKVCounts:     mediaKVCounts,
	}
}

// buildIMCCacheFromScratch builds the cache from scratch for messages[0:lastMsgIdxToCache].
func (m *Model) buildIMCCacheFromScratch(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int) cacheResult {

	// Reserve the slot under lock. Check for after-lock cache hit, mark
	// pending, and reset session state before releasing the lock.
	m.cacheMu.Lock()

	// Double-check in case another goroutine built the cache while we waited.
	if session.cachedMsgCount > 0 && session.totalTokensCached > 0 && session.cachedMsgCount <= len(messages) {
		prefixHash := hashMessages(messages[:session.cachedMsgCount])
		if prefixHash == session.cachedMsgsHash {
			m.log(ctx, "imc", "status", "cache hit (after-lock)", "slot", session.slotID, "seq", session.seqID,
				"cached-msgs", session.cachedMsgCount, "total-tokens-cached", session.totalTokensCached)

			lastMsgIdx := session.cachedMsgCount
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
	session.cachedMsgCount = 0
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

	// If any cacheable message contains media, defer the build to startSlot
	// where the mtmd pipeline (projection model + embedding decode) is available.
	if m.projFile != "" {
		if slices.ContainsFunc(msgsToCache, messageHasMedia) {
			newHash := hashMessages(msgsToCache)

			m.log(ctx, "imc", "status", "media cache build prepared", "slot", slotID, "seq", seqID,
				"msgs", lastMsgIdxToCache, "hash", newHash[:8])

			return cacheResult{
				modifiedD:            removeFirstNMessages(d, lastMsgIdxToCache),
				cacheIdx:             0,
				cachedMsgCount:       lastMsgIdxToCache,
				cacheSeqID:           seqID,
				imcSlotID:            slotID,
				imcExpectedHash:      newHash,
				imcNewCachedMsgCount: lastMsgIdxToCache,
				imcNewMsgsHash:       newHash,
				imcClearSeq:          true,
				imcMediaBuild:        true,
				imcMediaCacheD:       prefixD,
			}
		}
	}

	dataToCache, _, err := m.createPrompt(ctx, prefixD)
	if err != nil {
		m.imcClearPending(slotID)

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
		m.imcClearPending(slotID)

		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: messages tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "imc", "status", "skip (too short)", "last-msg-index-to-cache", lastMsgIdxToCache, "tokens", nTokens, "cache-min-tokens", m.cfg.CacheMinTokens)

		m.imcClearPending(slotID)

		return cacheResult{modifiedD: d}
	}

	// Return tokens for deferred decode in startSlot.
	newHash := hashMessages(msgsToCache)

	m.log(ctx, "imc", "status", "cache build prepared", "slot", slotID, "seq", seqID, "msgs", lastMsgIdxToCache, "tokens", nTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD:            removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:             0,
		cachedMsgCount:       lastMsgIdxToCache,
		cacheSeqID:           seqID,
		imcSlotID:            slotID,
		imcExpectedHash:      newHash,
		imcNewCacheTokens:    tokens,
		imcNewTotalCached:    nTokens,
		imcNewCachedMsgCount: lastMsgIdxToCache,
		imcNewMsgsHash:       newHash,
		imcClearSeq:          true,
		imcNewCachedTokens:   tokens,
	}
}

// rebuildIMCWithMedia handles cache builds/extends that involve media content.
// When a slot has media cached or extension messages contain media, we can't do
// text-only extension because text tokenization can't account for image/audio
// token positions. This function prepares a full media rebuild by marking the
// slot pending and returning a cacheResult with imcMediaBuild=true for deferred
// media decode in startSlot.
func (m *Model) rebuildIMCWithMedia(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int) cacheResult {
	m.cacheMu.Lock()

	if session.pending {
		m.cacheMu.Unlock()
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	session.pending = true
	session.totalTokensCached = 0
	session.cachedMsgCount = 0
	session.cachedMsgsHash = ""
	session.hasMedia = false
	seqID := session.seqID
	slotID := session.slotID

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "slot marked pending (media rebuild)", "slot", slotID, "seq", seqID)

	msgsToCache := messages[:lastMsgIdxToCache]
	newHash := hashMessages(msgsToCache)

	prefixD := D{
		"messages":              msgsToCache,
		"add_generation_prompt": false,
	}
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	m.log(ctx, "imc", "status", "media rebuild prepared", "slot", slotID, "seq", seqID,
		"msgs", lastMsgIdxToCache, "hash", newHash[:8])

	return cacheResult{
		modifiedD:            removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:             0,
		cachedMsgCount:       lastMsgIdxToCache,
		cacheSeqID:           seqID,
		imcSlotID:            slotID,
		imcExpectedHash:      newHash,
		imcNewCachedMsgCount: lastMsgIdxToCache,
		imcNewMsgsHash:       newHash,
		imcClearSeq:          true,
		imcMediaBuild:        true,
		imcMediaCacheD:       prefixD,
	}
}

// rebuildIMCFromPartialPrefix rebuilds a slot's cache using a token-level
// partial prefix match. This is the Non-Deterministic mode path — when a
// model template produces different tokens for the same messages (e.g.,
// GPT-OSS, GLM), this function salvages the longest common prefix in the
// KV cache, trims the divergent suffix, and decodes only the new tokens
// from the divergence point forward.
//
// For Hybrid models, batch_slot_start.go converts the partial trim into a
// full clear + re-decode since partial MemorySeqRm corrupts recurrent state.
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
		modifiedD:            removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:             llama.Pos(commonPrefixLen),
		cachedMsgCount:       lastMsgIdxToCache,
		cacheSeqID:           seqID,
		imcSlotID:            slotID,
		imcExpectedHash:      newHash,
		imcNewCacheTokens:    extensionTokens,
		imcNewTotalCached:    totalTokens,
		imcNewCachedMsgCount: lastMsgIdxToCache,
		imcNewMsgsHash:       newHash,
		imcTrimPos:           llama.Pos(commonPrefixLen),
		imcNewCachedTokens:   allTokens,
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

// imcClearPending clears a slot's pending flag and notifies waiters.
// Safe to call even if the slot wasn't pending.
func (m *Model) imcClearPending(slotID int) {
	m.cacheMu.Lock()
	if slotID < len(m.imcSlots) {
		m.imcSlots[slotID].pending = false
	}
	m.cacheMu.Unlock()
	m.notifyIMCSlotAvailable()
}

// imcCommitSession updates a slot's session metadata after a successful
// cache build/extend/rebuild and clears the pending flag. When hasMedia is
// true, cachedTokens is cleared since token-level operations (prefix matching,
// speculative decoding) are not valid for media-cached slots. mediaKVCounts
// records the KV positions consumed per media chunk for text-only extend math.
func (m *Model) imcCommitSession(slotID int, hash string, totalCached int, cachedMsgCount int, cachedTokens []llama.Token, hasMedia bool, mediaKVCounts []int) {
	m.cacheMu.Lock()
	if slotID < len(m.imcSlots) {
		slot := m.imcSlots[slotID]
		slot.cachedMsgsHash = hash
		slot.totalTokensCached = totalCached
		slot.cachedMsgCount = cachedMsgCount
		slot.lastUsed = time.Now()
		slot.pending = false
		slot.hasMedia = hasMedia
		slot.mediaKVCounts = mediaKVCounts
		switch {
		case hasMedia:
			slot.cachedTokens = nil
		case len(cachedTokens) > 0:
			slot.cachedTokens = cachedTokens
		}
	}
	m.cacheMu.Unlock()
	m.notifyIMCSlotAvailable()
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

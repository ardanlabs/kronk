package model

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// =============================================================================
// Incremental Message Cache (IMC) — Core Algorithm
//
// IMC uses a cascading match algorithm:
//
//  1. Hash-based prefix matching (fastest path, zero tokenization).
//  2. System prompt preservation (keep sys prompt KV, rebuild conversation).
//  3. Token-level prefix fallback (salvage common prefix when hash fails).
//  4. Full rebuild from scratch (empty session or LRU eviction).
//
// The matching algorithm is the same for all model types (Dense, MoE, Hybrid).
// After each request, text-only sessions externalize their KV state to RAM
// (via StateSeqGetData) and release the slot. The next request restores the
// KV state into any available slot (via StateSeqSetData). Media sessions
// remain slot-dedicated because image/audio embeddings can't be externalized.
//
// All functions in this file are shared: processIMC (session scan + hash
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
	sysPromptHash     string
	sysPromptTokens   int
}

// processIMC implements incremental multi-turn caching (IMC) for agentic
// workflows. It caches all messages except the last one (which triggers
// generation) and extends the cache incrementally on subsequent requests.
//
// This function implements the session selection algorithm across all model
// types (Dense, MoE, Hybrid). All NSeqMax sessions are available. Each session
// independently tracks its own conversation branch (hash, token count,
// message index). Sub-agents get routed to different sessions via hash matching.
//
// Algorithm:
//  1. Scan all sessions for a prefix hash match
//  2. On match: extend or reuse the matching session's cache
//  3. System prompt match: preserve sys prompt KV, rebuild conversation body
//  4. No hash match: try token prefix fallback
//  5. No match at all: pick an empty session or evict the LRU session, rebuild
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

	// We will cache all messages but the last one. However, tool response
	// messages (role:"tool") cannot be rendered alone by many Jinja templates
	// (e.g., Gemma 4) — the template expects the preceding assistant message
	// with tool_calls to forward-scan and consume the tool responses. If the
	// last uncached message is a tool response, move the cache boundary back
	// to keep the assistant tool_call message in the suffix alongside its
	// tool response(s).
	lastMsgIdxToCache := totalMsgs - 1
	for lastMsgIdxToCache > 0 {
		role, _ := messages[lastMsgIdxToCache]["role"].(string)
		if role != "tool" {
			break
		}
		lastMsgIdxToCache--
	}

	// If we walked all the way back to 0, there's nothing to cache.
	if lastMsgIdxToCache < 1 {
		return cacheResult{modifiedD: d}
	}

	// Snapshot session metadata under RLock, then release before hashing.

	m.log(ctx, "imc", "status", "scanning sessions", "total-msgs", totalMsgs, "msgs-to-cache", lastMsgIdxToCache, "total-sessions", len(m.imcSessions))

	m.cacheMu.RLock()

	snapshots := make([]imcSlotSnapshot, len(m.imcSessions))
	for i, sess := range m.imcSessions {
		snapshots[i] = imcSlotSnapshot{
			slotID:            sess.slotID,
			seqID:             sess.seqID,
			cachedMsgsHash:    sess.cachedMsgsHash,
			cachedTokens:      sess.cachedTokens,
			totalTokensCached: sess.totalTokensCached,
			cachedMsgCount:    sess.cachedMsgCount,
			lastUsed:          sess.lastUsed,
			pending:           sess.pending,
			empty:             sess.totalTokensCached == 0,
			hasMedia:          sess.hasMedia,
			sysPromptHash:     sess.sysPromptHash,
			sysPromptTokens:   sess.sysPromptTokens,
		}
	}

	m.cacheMu.RUnlock()

	// -------------------------------------------------------------------------
	// Step 1: Hash-based session scan.

	// Pre-compute the incoming system prompt hash for two-tier matching.
	// If the first message is a system prompt, hash it once and use it to
	// check sys-prompt-only matches when the full prefix hash mismatches.
	var incomingSysHash string
	if len(messages) > 0 {
		if role, _ := messages[0]["role"].(string); role == "system" {
			incomingSysHash = hashMessages(messages[:1])
		}
	}

	var bestSession *imcSession
	var bestCachedMsgsHash string
	var bestTotalTokensCached int
	var bestCachedMsgCount int
	var emptySessions []*imcSession
	var lruSession *imcSession
	var mismatchSessions []int // Snapshot indices of non-matching sessions (eviction candidates).

	// Two-tier: track the best session where only the system prompt hash matches.
	var bestSysPromptSession *imcSession
	var bestSysPromptSnapIdx int

	for i, snap := range snapshots {

		// Skip sessions with a build/rebuild in-flight.
		if snap.pending {
			m.log(ctx, "imc", "scan", fmt.Sprintf("session[%d] pending (build in-flight)", snap.slotID))
			continue
		}

		// Track empty sessions for fallback.
		if snap.empty {
			m.log(ctx, "imc", "scan", fmt.Sprintf("session[%d] empty", snap.slotID))

			emptySessions = append(emptySessions, m.imcSessions[i])
			continue
		}

		// Track LRU session for eviction fallback.
		if lruSession == nil || snap.lastUsed.Before(snapshots[lruSession.slotID].lastUsed) {
			lruSession = m.imcSessions[i]
		}

		// Skip sessions with more cached messages than this request has total.
		if totalMsgs <= snap.cachedMsgCount {
			m.log(ctx, "imc", "scan", fmt.Sprintf("session[%d] skip (cached-msgs[%d] >= total-msgs[%d])", snap.slotID, snap.cachedMsgCount, totalMsgs))
			mismatchSessions = append(mismatchSessions, i)
			continue
		}

		// Check if this session's cached prefix matches the incoming messages.
		prefixHash := hashMessages(m.imcEnsureUserMessage(ctx, messages[:snap.cachedMsgCount]))
		if prefixHash != snap.cachedMsgsHash {
			m.log(ctx, "imc", "scan", fmt.Sprintf("session[%d] mismatch (cached-msgs[%d] tokens[%d] hash[%s..] != [%s..])",
				snap.slotID, snap.cachedMsgCount, snap.totalTokensCached, snap.cachedMsgsHash[:8], prefixHash[:8]))
			mismatchSessions = append(mismatchSessions, i)

			// Two-tier: full hash failed, check if system prompt matches.
			// Skip media sessions (token-level trim is unsafe for media KV).
			if incomingSysHash != "" && !snap.hasMedia &&
				snap.sysPromptHash == incomingSysHash && snap.sysPromptTokens > 0 {

				// Pick the LRU among sys-prompt matches (least disruptive to evict).
				if bestSysPromptSession == nil || snap.lastUsed.Before(snapshots[bestSysPromptSnapIdx].lastUsed) {
					bestSysPromptSession = m.imcSessions[i]
					bestSysPromptSnapIdx = i
				}

				m.log(ctx, "imc", "scan", fmt.Sprintf("session[%d] sys-prompt-match (sys-tokens[%d] hash[%s..])",
					snap.slotID, snap.sysPromptTokens, snap.sysPromptHash[:8]))
			}

			continue
		}

		m.log(ctx, "imc", "scan", fmt.Sprintf("session[%d] MATCH (cached-msgs[%d] tokens[%d] hash[%s..])",
			snap.slotID, snap.cachedMsgCount, snap.totalTokensCached, snap.cachedMsgsHash[:8]))

		// This session matches. Pick the one with the most cached messages
		// (best prefix coverage).
		if bestSession == nil || snap.cachedMsgCount > bestCachedMsgCount {
			bestSession = m.imcSessions[i]
			bestCachedMsgsHash = snap.cachedMsgsHash
			bestTotalTokensCached = snap.totalTokensCached
			bestCachedMsgCount = snap.cachedMsgCount
		}
	}

	// -------------------------------------------------------------------------
	// Step 1b: KV pressure eviction.
	//
	// With unified KV cache (KVUnified=1), all sequences share the same n_ctx
	// pool. Mismatched sessions holding stale conversation prefixes consume KV
	// cells that the active session may need. Before proceeding to Step 2,
	// estimate the projected total KV usage and evict mismatched sessions
	// (largest first) until the active request fits within n_ctx.

	if bestSession != nil && len(mismatchSessions) > 0 && m.cfg.ContextWindow() > 0 {
		nCtx := m.cfg.ContextWindow()

		// Sum KV usage across all non-empty, non-pending sessions that
		// don't have externalized kvState. Sessions with kvState had their
		// VRAM sequences cleared in finishSlot and don't consume KV cells.
		var projectedKV int
		for i, snap := range snapshots {
			if !snap.empty && !snap.pending {
				session := m.imcSessions[i]
				if len(session.kvState) == 0 {
					projectedKV += snap.totalTokensCached
				}
			}
		}

		if projectedKV > nCtx {
			// Sort mismatched sessions by token count descending (evict largest first).
			sort.Slice(mismatchSessions, func(a, b int) bool {
				return snapshots[mismatchSessions[a]].totalTokensCached > snapshots[mismatchSessions[b]].totalTokensCached
			})

			for _, idx := range mismatchSessions {
				if projectedKV <= nCtx {
					break
				}

				snap := snapshots[idx]
				session := m.imcSessions[idx]

				m.log(ctx, "imc", "status", "kv-pressure-evict",
					"slot", snap.slotID, "seq", snap.seqID,
					"evicted-tokens", snap.totalTokensCached,
					"projected-kv", projectedKV, "n_ctx", nCtx)

				// Clear VRAM KV only if not already externalized.
				if len(session.kvState) == 0 {
					m.decodeMu.Lock()
					llama.MemorySeqRm(m.mem, snap.seqID, -1, -1)
					m.decodeMu.Unlock()
					projectedKV -= snap.totalTokensCached
				}

				// Reset the session metadata (frees RAM snapshot too).
				m.cacheMu.Lock()
				imcResetSession(session)
				m.cacheMu.Unlock()
			}

			m.log(ctx, "imc", "status", "kv-pressure-evict-done",
				"projected-kv-after", projectedKV, "n_ctx", nCtx)
		}
	}

	// -------------------------------------------------------------------------
	// Step 2: Handle matched session — extend or pure hit.

	if bestSession != nil {
		m.log(ctx, "imc", "status", "session matched", "slot", bestSession.slotID, "seq", bestSession.seqID,
			"cached-msgs", bestCachedMsgCount, "cached-tokens", bestTotalTokensCached, "msgs-to-cache", lastMsgIdxToCache)

		// If there are more messages to cache, extend.
		if bestCachedMsgCount < lastMsgIdxToCache {
			return m.extendIMCCache(ctx, d, messages, bestSession, bestCachedMsgCount, lastMsgIdxToCache, bestTotalTokensCached)
		}

		// Exact same messages as before — pure cache hit.
		m.log(ctx, "imc", "status", "cache hit", "slot", bestSession.slotID, "seq", bestSession.seqID,
			"cached-msgs", bestCachedMsgCount, "current-total-tokens-cached", bestTotalTokensCached,
			"hash", bestCachedMsgsHash[:8])

		return cacheResult{
			modifiedD:       removeFirstNMessages(d, bestCachedMsgCount),
			cacheIdx:        llama.Pos(bestTotalTokensCached),
			cacheSeqID:      bestSession.seqID,
			imcSlotID:       bestSession.slotID,
			imcExpectedHash: bestCachedMsgsHash,
			imcSession:      bestSession,
		}
	}

	// -------------------------------------------------------------------------
	// Step 2b: System prompt preservation (two-tier hash match).
	//
	// No full hash match, but a session has the same system prompt cached. Keep
	// the system prompt KV in place, trim everything after it, and re-decode
	// only the conversation body. This avoids re-decoding the system prompt
	// (often thousands of tokens) when the client edits conversation history.

	if bestSysPromptSession != nil {
		snap := snapshots[bestSysPromptSnapIdx]

		m.log(ctx, "imc", "status", "sys-prompt-match (preserving system prompt)",
			"slot", snap.slotID, "seq", snap.seqID,
			"sys-tokens", snap.sysPromptTokens, "old-total-tokens", snap.totalTokensCached)

		return m.rebuildIMCPreservingSysPrompt(ctx, d, messages, bestSysPromptSession, lastMsgIdxToCache, snap.sysPromptTokens)
	}

	// -------------------------------------------------------------------------
	// Step 3: Token prefix fallback.
	//
	// No hash match — try token-level partial prefix matching before falling
	// back to empty session or LRU eviction. Templates, tool call formatting, or
	// client behavior may produce different token sequences for logically
	// identical messages, but often share a long common prefix we can salvage.

	m.log(ctx, "imc", "status", "no session matched, trying token prefix match", "total-msgs", totalMsgs)

	// Collect non-empty, non-pending sessions as candidates for token comparison.
	// Only consider sessions where the message count is compatible — the request
	// must have at least as many messages as the session cached. When the request
	// has fewer messages (e.g., 2 vs 11), it's a new conversation and sharing
	// system prompt tokens from an unrelated conversation is not useful.
	// Skip sessions with media: token prefix matching can't compare image
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
		msgs := m.imcEnsureUserMessage(ctx, messages[:lastMsgIdxToCache])

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

				m.log(ctx, "imc", "token-match", fmt.Sprintf("session[%d] common-prefix %d/%d tokens (%d%% salvageable)",
					snap.slotID, commonLen, snap.totalTokensCached, pct))

				if commonLen > bestPartialLen {
					bestPartialLen = commonLen
					bestPartialSlotIdx = idx
				}
			}

			if bestPartialLen >= m.cfg.CacheMinTokens() {
				partialSession := m.imcSessions[bestPartialSlotIdx]
				discarded := snapshots[bestPartialSlotIdx].totalTokensCached - bestPartialLen
				saved := len(incomingTokens) - bestPartialLen

				m.log(ctx, "imc", "status", "token prefix match found",
					"slot", partialSession.slotID,
					"common-prefix", bestPartialLen,
					"discarded-cached", discarded,
					"new-tokens-to-decode", saved,
					"total-incoming", len(incomingTokens))

				return m.rebuildIMCFromPartialPrefix(ctx, d, messages, partialSession, lastMsgIdxToCache, incomingTokens, bestPartialLen)
			}

			m.log(ctx, "imc", "status", "no usable token prefix match",
				"best-prefix", bestPartialLen, "min-required", m.cfg.CacheMinTokens())
		}
	}

	// -------------------------------------------------------------------------
	// Step 4: No match — pick an empty session or evict LRU.
	//
	// No hash match, no token prefix match — try each empty session in order.
	// If a concurrent request already marked one pending, move to the next.

	for _, session := range emptySessions {
		m.log(ctx, "imc", "status", "trying empty session", "slot", session.slotID)

		result := m.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
		if !result.imcPending {
			return result
		}

		m.log(ctx, "imc", "status", "empty session pending, trying next", "slot", session.slotID)
	}

	if lruSession != nil {
		m.log(ctx, "imc", "status", "evicting LRU session", "slot", lruSession.slotID,
			"evicted-msgs", lruSession.cachedMsgCount, "evicted-tokens", lruSession.totalTokensCached)

		return m.buildIMCCacheFromScratch(ctx, d, messages, lruSession, lastMsgIdxToCache)
	}

	// All sessions are pending. Wait for one to become available, then retry
	// the entire scan. Use the cacheCond condvar which is broadcast whenever
	// any session's pending flag is cleared.
	m.log(ctx, "imc", "status", "all sessions pending, waiting for session")

	if err := m.waitForIMCSlot(ctx, requestStart); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	m.log(ctx, "imc", "status", "session became available, retrying scan")

	return m.processIMC(ctx, d, requestStart)
}

// extendIMCCache extends the existing cache with new messages from
// messages[currentCachedMsgCount:lastMsgIdxToCache].
func (m *Model) extendIMCCache(ctx context.Context, d D, messages []D, session *imcSession, currentCachedMsgCount, lastMsgIdxToCache, currentTotalTokensCached int) cacheResult {

	// When the session has media cached or any extension messages contain media,
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

		// New messages contain media — determine extension strategy.
		if newMsgsHaveMedia {
			m.cacheMu.RLock()
			slotHasMedia := session.hasMedia
			m.cacheMu.RUnlock()

			if slotHasMedia {
				// Session already has media cached. Adding more media requires a
				// full rebuild through the mtmd pipeline to re-encode all media.
				m.log(ctx, "imc", "status", "extend requires media rebuild (new media, session has media)",
					"slot", session.slotID, "cached-msgs", currentCachedMsgCount,
					"target-msgs", lastMsgIdxToCache)

				return m.rebuildIMCWithMedia(ctx, d, messages, session, lastMsgIdxToCache)
			}

			// Session has text-only cache. Use partial media extend to preserve
			// the cached text prefix and only decode the new content (remaining
			// text + media + post-media text) through the mtmd pipeline.
			return m.extendIMCTextCacheWithMedia(ctx, d, messages, session, lastMsgIdxToCache, currentTotalTokensCached)
		}

		// Session has media but new messages are text-only — extend with text
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

	// Reserve the session under lock. Validate state hasn't changed and mark
	// pending so concurrent scanners skip this session during the heavy work.
	m.cacheMu.Lock()

	if session.cachedMsgCount != currentCachedMsgCount || session.totalTokensCached != currentTotalTokensCached {
		m.log(ctx, "imc", "status", "extend fallback (state changed)", "slot", session.slotID,
			"expected-msgs", currentCachedMsgCount, "actual-msgs", session.cachedMsgCount,
			"expected-tokens", currentTotalTokensCached, "actual-tokens", session.totalTokensCached)
		m.cacheMu.Unlock()
		return m.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
	}

	if session.pending {
		m.log(ctx, "imc", "status", "extend fallback (session pending)", "slot", session.slotID)
		m.cacheMu.Unlock()
		return m.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID
	currentHash := session.cachedMsgsHash

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "session marked pending (extend)", "slot", slotID, "seq", seqID)

	// -------------------------------------------------------------------------
	// Heavy work: template + tokenize outside the lock.

	msgs := m.imcEnsureUserMessage(ctx, messages[:lastMsgIdxToCache])

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
		m.log(ctx, "imc", "status", "extend (no new tokens)", "slot", slotID, "cached", currentTotalTokensCached, "total", totalTokens)

		// When the new prompt has fewer tokens than cached (e.g., the client
		// dropped reasoning blocks or truncated earlier messages), rebuild the
		// cache from scratch rather than trimming. Trimming preserves stale KV
		// state that causes the model to lose attention on recent instructions.
		// A full rebuild ensures the KV cache exactly matches the conversation
		// the client sent.
		if totalTokens < currentTotalTokensCached {
			newHash := hashMessages(msgs)
			sysHash, sysToks := m.imcSysPromptInfo(ctx, msgs, allTokens)

			trimFrom := 0
			if sysToks > 0 && session.sysPromptHash == sysHash && session.sysPromptTokens == sysToks {
				trimFrom = sysToks
			}

			m.log(ctx, "imc", "status", "extend->rebuild", "slot", slotID,
				"cached", currentTotalTokensCached, "new_total", totalTokens,
				"sys_prompt_kept", trimFrom > 0, "sys_prompt_tokens", sysToks)

			return imcRebuildResult(d, seqID, slotID, lastMsgIdxToCache, allTokens, newHash, sysHash, sysToks, trimFrom, session)
		}

		m.imcClearPending(slotID)

		return cacheResult{
			modifiedD:       removeFirstNMessages(d, currentCachedMsgCount),
			cacheIdx:        llama.Pos(currentTotalTokensCached),
			cacheSeqID:      seqID,
			imcSlotID:       slotID,
			imcExpectedHash: currentHash,
			imcSession:      session,
		}
	}

	// Extract only the new tokens beyond what's already cached.
	extensionTokens := allTokens[currentTotalTokensCached:]
	numOfExtTokens := len(extensionTokens)

	m.log(ctx, "imc", "status", "extending cache (deferred)", "slot", slotID, "new-tokens", numOfExtTokens)

	// Compute new session state to be applied after decode in startSlot.
	newHash := hashMessages(msgs)

	// Carry forward the system prompt info from the session. On the first
	// build it was computed; on extends it doesn't change.
	sysHash := session.sysPromptHash
	sysToks := session.sysPromptTokens

	m.log(ctx, "imc", "status", "cache extend prepared", "slot", slotID, "seq", seqID,
		"idx", fmt.Sprintf("cur[%d] -> new[%d]", currentCachedMsgCount, lastMsgIdxToCache),
		"tokens", fmt.Sprintf("cur[%d] -> new[%d] (+%d)", currentTotalTokensCached, totalTokens, numOfExtTokens))

	return cacheResult{
		modifiedD:            removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:             llama.Pos(currentTotalTokensCached),
		cacheSeqID:           seqID,
		imcSlotID:            slotID,
		imcExpectedHash:      newHash,
		imcSession:           session,
		imcNewCacheTokens:    extensionTokens,
		imcNewTotalCached:    totalTokens,
		imcNewCachedMsgCount: lastMsgIdxToCache,
		imcNewMsgsHash:       newHash,
		imcNewCachedTokens:   allTokens,
		imcSysPromptHash:     sysHash,
		imcSysPromptTokens:   sysToks,
	}
}

// extendIMCMediaSlotWithText extends a media-cached session with text-only
// messages. The image/audio embeddings remain in the KV cache; only new text
// tokens are decoded. Uses mediaKVCounts to compute the offset between text
// token counts (from tokenization, which sees markers) and KV positions
// (which include image/audio embeddings instead of markers).
func (m *Model) extendIMCMediaSlotWithText(ctx context.Context, d D, messages []D, session *imcSession, currentCachedMsgCount, lastMsgIdxToCache, currentTotalTokensCached int, mediaKVCounts []int) cacheResult {

	// Reserve the session under lock.
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

	m.log(ctx, "imc", "status", "session marked pending (media text extend)", "slot", slotID, "seq", seqID)

	// Compute the marker token count once per model lifetime.
	m.mediaMarkerOnce.Do(func() {
		marker := fmt.Sprintf("%s\n", mtmd.DefaultMarker())
		markerTokens := llama.Tokenize(m.vocab, marker, false, false)
		m.mediaMarkerTokens = len(markerTokens)
		m.log(ctx, "imc", "status", "computed media marker tokens", "marker", marker, "tokens", m.mediaMarkerTokens)
	})

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
	msgs := m.imcEnsureUserMessage(ctx, messages[:lastMsgIdxToCache])

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

		// When the new prompt has fewer text tokens than cached, the KV cache
		// contains stale entries. For media sessions, a trim-only approach is
		// unsafe because the KV-to-token mapping is complex (media embeddings
		// occupy different KV counts than their marker tokens). Fall back to
		// a full rebuild to ensure correctness.
		if totalTextTokens < cachedTextTokens {
			m.imcClearPending(slotID)

			m.log(ctx, "imc", "status", "media text extend shrink (rebuilding)",
				"slot", slotID, "cached_text_tokens", cachedTextTokens, "total_text_tokens", totalTextTokens)

			return m.buildIMCCacheFromScratch(ctx, d, messages, session, lastMsgIdxToCache)
		}

		m.imcClearPending(slotID)

		return cacheResult{
			modifiedD:       removeFirstNMessages(d, currentCachedMsgCount),
			cacheIdx:        llama.Pos(currentTotalTokensCached),
			cacheSeqID:      seqID,
			imcSlotID:       slotID,
			imcExpectedHash: session.cachedMsgsHash,
			imcSession:      session,
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
		cacheSeqID:           seqID,
		imcSlotID:            slotID,
		imcExpectedHash:      newHash,
		imcSession:           session,
		imcNewCacheTokens:    extensionTokens,
		imcNewTotalCached:    newTotalCached,
		imcNewCachedMsgCount: lastMsgIdxToCache,
		imcNewMsgsHash:       newHash,
		imcMediaKVCounts:     mediaKVCounts,
	}
}

// extendIMCTextCacheWithMedia extends a text-only cached session with new messages
// that contain media. Instead of clearing the KV cache and rebuilding everything
// through the mtmd pipeline, this preserves the existing text prefix and only
// decodes the new content (remaining text + media + post-media text).
//
// This avoids re-decoding potentially tens of thousands of text tokens that are
// already in the KV cache, saving significant prefill time when media is first
// introduced in a conversation that started as text-only.
func (m *Model) extendIMCTextCacheWithMedia(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache, currentTotalTokensCached int) cacheResult {
	m.cacheMu.Lock()

	if session.pending {
		m.cacheMu.Unlock()
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "session marked pending (media extend from text)",
		"slot", slotID, "seq", seqID, "skip_text_tokens", currentTotalTokensCached)

	msgsToCache := m.imcEnsureUserMessage(ctx, messages[:lastMsgIdxToCache])
	newHash := hashMessages(msgsToCache)

	prefixD := D{
		"messages":              msgsToCache,
		"add_generation_prompt": false,
	}
	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	m.log(ctx, "imc", "status", "media extend prepared", "slot", slotID, "seq", seqID,
		"msgs", lastMsgIdxToCache, "hash", newHash[:8], "skip_text_tokens", currentTotalTokensCached)

	return cacheResult{
		modifiedD:              removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:               0,
		cacheSeqID:             seqID,
		imcSlotID:              slotID,
		imcExpectedHash:        newHash,
		imcSession:             session,
		imcNewCachedMsgCount:   lastMsgIdxToCache,
		imcNewMsgsHash:         newHash,
		imcMediaBuild:          true,
		imcMediaCacheD:         prefixD,
		imcMediaSkipTextTokens: currentTotalTokensCached,
	}
}

// buildIMCCacheFromScratch builds the cache from scratch for messages[0:lastMsgIdxToCache].
func (m *Model) buildIMCCacheFromScratch(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int) cacheResult {

	// Reserve the session under lock. Check for after-lock cache hit, mark
	// pending, and reset session state before releasing the lock.
	m.cacheMu.Lock()

	// Double-check in case another goroutine built the cache while we waited.
	if session.cachedMsgCount > 0 && session.totalTokensCached > 0 && session.cachedMsgCount <= len(messages) {
		prefixHash := hashMessages(m.imcEnsureUserMessage(ctx, messages[:session.cachedMsgCount]))
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
				cacheSeqID:      seqID,
				imcSlotID:       sID,
				imcExpectedHash: hash,
				imcSession:      session,
			}
		}
	}

	if session.pending {
		m.log(ctx, "imc", "status", "build-from-scratch skipped (session pending)", "slot", session.slotID)
		m.cacheMu.Unlock()

		return cacheResult{modifiedD: d, imcPending: true, err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	// Reset session state and mark pending so concurrent scanners skip this
	// session while we do the heavy work outside the lock.
	imcResetSession(session)
	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "session marked pending", "slot", slotID, "seq", seqID)

	// -------------------------------------------------------------------------
	// Heavy work: template + tokenize outside the lock.

	msgsToCache := m.imcEnsureUserMessage(ctx, messages[:lastMsgIdxToCache])
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
				cacheSeqID:           seqID,
				imcSlotID:            slotID,
				imcExpectedHash:      newHash,
				imcSession:           session,
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

	if nTokens < m.cfg.CacheMinTokens() {
		m.log(ctx, "imc", "status", "skip (too short)", "last-msg-index-to-cache", lastMsgIdxToCache, "tokens", nTokens, "cache-min-tokens", m.cfg.CacheMinTokens())

		m.imcClearPending(slotID)

		return cacheResult{modifiedD: d}
	}

	// Return tokens for deferred decode in startSlot.
	newHash := hashMessages(msgsToCache)
	sysHash, sysToks := m.imcSysPromptInfo(ctx, msgsToCache, tokens)

	m.log(ctx, "imc", "status", "cache build prepared", "slot", slotID, "seq", seqID, "msgs", lastMsgIdxToCache, "tokens", nTokens, "hash", newHash[:8])

	return imcRebuildResult(d, seqID, slotID, lastMsgIdxToCache, tokens, newHash, sysHash, sysToks, 0, session)
}

// rebuildIMCWithMedia handles cache builds/extends that involve media content.
// When a session has media cached or extension messages contain media, we can't do
// text-only extension because text tokenization can't account for image/audio
// token positions. This function prepares a full media rebuild by marking the
// session pending and returning a cacheResult with imcMediaBuild=true for deferred
// media decode in startSlot.
func (m *Model) rebuildIMCWithMedia(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int) cacheResult {
	m.cacheMu.Lock()

	if session.pending {
		m.cacheMu.Unlock()
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	imcResetSession(session)
	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "session marked pending (media rebuild)", "slot", slotID, "seq", seqID)

	msgsToCache := m.imcEnsureUserMessage(ctx, messages[:lastMsgIdxToCache])
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
		cacheSeqID:           seqID,
		imcSlotID:            slotID,
		imcExpectedHash:      newHash,
		imcSession:           session,
		imcNewCachedMsgCount: lastMsgIdxToCache,
		imcNewMsgsHash:       newHash,
		imcClearSeq:          true,
		imcMediaBuild:        true,
		imcMediaCacheD:       prefixD,
	}
}

// rebuildIMCFromPartialPrefix rebuilds a session's cache using a token-level
// partial prefix match. When hash matching fails (due to template variation,
// tool call formatting differences, or client behavior), this function
// salvages the longest common prefix in the KV cache, trims the divergent
// suffix, and decodes only the new tokens from the divergence point forward.
//
// For Hybrid models, batch_slot_start.go converts the partial trim into a
// full clear + re-decode since partial MemorySeqRm corrupts recurrent state.
func (m *Model) rebuildIMCFromPartialPrefix(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache int, allTokens []llama.Token, commonPrefixLen int) cacheResult {

	// Reserve the session under lock.
	m.cacheMu.Lock()

	if session.pending {
		m.cacheMu.Unlock()
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "session marked pending (partial prefix)", "slot", slotID, "seq", seqID)

	msgsToCache := m.imcEnsureUserMessage(ctx, messages[:lastMsgIdxToCache])
	newHash := hashMessages(msgsToCache)
	sysHash, sysToks := m.imcSysPromptInfo(ctx, msgsToCache, allTokens)

	m.log(ctx, "imc", "status", "partial prefix rebuild prepared", "slot", slotID, "seq", seqID,
		"common-prefix", commonPrefixLen, "extension-tokens", len(allTokens)-commonPrefixLen,
		"total-tokens", len(allTokens), "hash", newHash[:8])

	return imcRebuildResult(d, seqID, slotID, lastMsgIdxToCache, allTokens, newHash, sysHash, sysToks, commonPrefixLen, session)
}

// rebuildIMCPreservingSysPrompt rebuilds a session's conversation cache while
// preserving the system prompt KV state. This is the two-tier hash path —
// when the full prefix hash mismatches but the system prompt hash still
// matches, the system prompt tokens in the KV cache are kept and only the
// conversation body (everything after the system prompt) is trimmed and
// re-decoded.
//
// This avoids re-decoding potentially thousands of system prompt tokens when
// the client edits or removes conversation messages but keeps the same system
// prompt.
func (m *Model) rebuildIMCPreservingSysPrompt(ctx context.Context, d D, messages []D, session *imcSession, lastMsgIdxToCache, cachedSysPromptTokens int) cacheResult {

	// Reserve the session under lock.
	m.cacheMu.Lock()

	if session.pending {
		m.cacheMu.Unlock()
		return cacheResult{modifiedD: d, imcPending: true, err: fmt.Errorf("imc: slot %d pending, retry request", session.slotID)}
	}

	session.pending = true
	seqID := session.seqID
	slotID := session.slotID

	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "session marked pending (sys-prompt-preserve)", "slot", slotID, "seq", seqID)

	// -------------------------------------------------------------------------
	// Heavy work: template + tokenize outside the lock.

	msgsToCache := m.imcEnsureUserMessage(ctx, messages[:lastMsgIdxToCache])
	prefixD := D{
		"messages":              msgsToCache,
		"add_generation_prompt": false,
	}

	if tools, ok := d["tools"]; ok {
		prefixD["tools"] = tools
	}

	dataToCache, _, err := m.createPrompt(ctx, prefixD)
	if err != nil {
		m.imcClearPending(slotID)
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: failed to template messages (sys-prompt-preserve): %w", err)}
	}

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-imc-sysprompt-preserve",
		attribute.String("cache-type", "imc-sysprompt-preserve"),
	)

	allTokens := llama.Tokenize(m.vocab, dataToCache, m.addBOSToken, true)
	totalTokens := len(allTokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", totalTokens))
	tokenSpan.End()

	if totalTokens == 0 {
		m.imcClearPending(slotID)
		return cacheResult{modifiedD: d, err: fmt.Errorf("imc: messages tokenized to zero tokens (sys-prompt-preserve)")}
	}

	newHash := hashMessages(msgsToCache)
	sysHash, sysToks := m.imcSysPromptInfo(ctx, msgsToCache, allTokens)

	// Verify the system prompt token boundary is consistent after
	// re-templating. If the template rendered the system prompt differently,
	// the cached KV state at those positions is wrong. Fall back to a full
	// rebuild from scratch.
	trimFrom := sysToks
	if sysToks != cachedSysPromptTokens {
		m.log(ctx, "imc", "status", "sys-prompt-preserve aborted (token boundary shifted)",
			"slot", slotID, "expected-sys-tokens", cachedSysPromptTokens, "actual-sys-tokens", sysToks)
		trimFrom = 0
	} else {
		m.log(ctx, "imc", "status", "sys-prompt-preserve prepared", "slot", slotID, "seq", seqID,
			"sys-tokens-kept", sysToks, "conv-tokens-to-decode", totalTokens-sysToks,
			"total-tokens", totalTokens, "hash", newHash[:8])
	}

	return imcRebuildResult(d, seqID, slotID, lastMsgIdxToCache, allTokens, newHash, sysHash, sysToks, trimFrom, session)
}

// imcRebuildResult constructs a cacheResult for any rebuild/trim scenario.
// trimFrom determines the behavior:
//   - trimFrom == 0: full rebuild (clear sequence, decode all tokens)
//   - trimFrom > 0:  partial rebuild (trim KV from trimFrom, decode suffix)
func imcRebuildResult(d D, seqID llama.SeqId, slotID, lastMsgIdxToCache int, allTokens []llama.Token, newHash, sysHash string, sysToks, trimFrom int, session *imcSession) cacheResult {
	return cacheResult{
		modifiedD:            removeFirstNMessages(d, lastMsgIdxToCache),
		cacheIdx:             llama.Pos(trimFrom),
		cacheSeqID:           seqID,
		imcSlotID:            slotID,
		imcExpectedHash:      newHash,
		imcSession:           session,
		imcNewCacheTokens:    allTokens[trimFrom:],
		imcNewTotalCached:    len(allTokens),
		imcNewCachedMsgCount: lastMsgIdxToCache,
		imcNewMsgsHash:       newHash,
		imcClearSeq:          trimFrom == 0,
		imcTrimPos:           llama.Pos(trimFrom),
		imcNewCachedTokens:   allTokens,
		imcSysPromptHash:     sysHash,
		imcSysPromptTokens:   sysToks,
	}
}

// decodeTokensIntoCache decodes tokens into a cache sequence starting at startPos.
// Unlike addTokensToCache, this does NOT clear the sequence first — the caller
// is responsible for clearing if needed (e.g., rebuild from scratch).
//
// isFinal must be true when this call decodes the FINAL tokens of the cache
// build (no further decode follows before the caller reads the KV state).
// When true, logits=true is set on the very last token to mirror the engine's
// regular prefill path. The media path passes isFinal=false because additional
// text/media chunks may follow in the same cache build.
//
// Performance:
//   - Reuses m.imcBatch (pre-allocated at model init) when available, avoiding
//     per-call BatchInit/BatchFree.
//   - Sets logits=true on the final token (only when isFinal=true) to mirror
//     the engine's regular prefill path; some llama.cpp backends pick a faster
//     kernel when logits is set.
//   - Does ONE llama.Synchronize at the end of the multi-chunk loop instead of
//     per chunk, allowing the GPU pipeline to overlap chunk submissions.
func (m *Model) decodeTokensIntoCache(ctx context.Context, tokens []llama.Token, seqID llama.SeqId, startPos int, isFinal bool) error {
	ctx, decodeSpan := otel.AddSpan(ctx, "cache-decode",
		attribute.Int("tokens", len(tokens)),
	)
	defer decodeSpan.End()

	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch()
	}

	m.log(ctx, "cache", "status", "decoding tokens into cache", "seq", seqID, "tokens", nTokens, "start_pos", startPos, "nbatch", nBatch)

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	// Use the reusable IMC batch when available; otherwise fall back to
	// per-call allocation (e.g., IncrementalCache disabled but somehow
	// reached this path).
	var batch llama.Batch
	if m.imcHasBatch {
		batch = m.imcBatch
	} else {
		batchSize := int32(min(nBatch, nTokens))
		if batchSize <= 0 {
			batchSize = 1
		}
		batch = llama.BatchInit(batchSize, 0, 1)
		defer llama.BatchFree(batch)
	}

	seqIDs := []llama.SeqId{seqID}

	for i := 0; i < nTokens; i += nBatch {
		batch.Clear()

		end := min(i+nBatch, nTokens)
		isFinalChunk := isFinal && end == nTokens

		for j := i; j < end; j++ {
			pos := llama.Pos(startPos + j)
			// Set logits=true on the final token of the final chunk (only when
			// the caller marked this as the final cache-build decode) to mirror
			// the regular prefill path (`addPrefillChunk` sets isLast on the
			// last prompt token). The computed logits are unused by IMC, but
			// matching the engine's batch shape avoids any backend-specific
			// fast-path divergence.
			isLast := isFinalChunk && j == end-1
			batch.Add(tokens[j], pos, seqIDs, isLast)
		}

		if _, err := llama.Decode(m.lctx, batch); err != nil {
			// Sync before returning so any in-flight GPU work is drained.
			llama.Synchronize(m.lctx)
			return fmt.Errorf("imc: failed to decode extension tokens at pos %d: %w", i, err)
		}
	}

	// Single sync at the end. Decode submissions can overlap on backends that
	// queue work asynchronously (e.g., Metal); we only need the KV state to be
	// fully written before the snapshot/extend caller reads it.
	llama.Synchronize(m.lctx)

	m.log(ctx, "cache", "status", "finished (decoding tokens into cache)", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	return nil
}

// waitForIMCSlot blocks until at least one IMC session is no longer pending,
// the context is canceled, or the wait timeout expires. It uses the cacheCond
// condvar which is broadcast whenever any session's pending flag is cleared.
// The timeout is the remaining time from the shared CacheSlotTimeout budget
// that started at requestStart.
func (m *Model) waitForIMCSlot(ctx context.Context, requestStart time.Time) error {
	timeout := time.Duration(m.cfg.CacheSlotTimeout()) * time.Second
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
			for _, sess := range m.imcSessions {
				if !sess.pending {
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
		return fmt.Errorf("imc: context canceled while waiting for session: %w", ctx.Err())
	case <-timer.C:
		m.cacheCond.Broadcast()
		return fmt.Errorf("server busy processing other requests, try again shortly")
	}
}

// notifyIMCSlotAvailable broadcasts to any goroutines waiting for a session to
// become available. Must be called after clearing a session's pending flag.
func (m *Model) notifyIMCSlotAvailable() {
	if m.cacheCond != nil {
		m.cacheCond.Broadcast()
	}
}

// imcResetSession clears all metadata on an IMC session, returning it to
// an empty state. The caller must hold m.cacheMu (write lock).
func imcResetSession(s *imcSession) {
	s.cachedMsgsHash = ""
	s.cachedTokens = nil
	s.totalTokensCached = 0
	s.cachedMsgCount = 0
	s.kvState = nil
	s.kvStateBytes = 0
	s.lastUsed = time.Time{}
	s.pending = false
	s.hasMedia = false
	s.useMRoPE = false
	s.mediaKVCounts = nil
	s.sysPromptHash = ""
	s.sysPromptTokens = 0
}

// imcClearPending clears a session's pending flag and notifies waiters.
// Safe to call even if the session wasn't pending.
func (m *Model) imcClearPending(slotID int) {
	m.cacheMu.Lock()
	if slotID < len(m.imcSessions) {
		m.imcSessions[slotID].pending = false
	}
	m.cacheMu.Unlock()
	m.notifyIMCSlotAvailable()
}

// imcCommitSession updates a session's metadata after a successful cache
// build/extend/rebuild and clears the pending flag. When hasMedia is true,
// cachedTokens is cleared since token-level operations (prefix matching,
// speculative decoding) are not valid for media-cached sessions. mediaKVCounts
// records the KV positions consumed per media chunk for text-only extend math.
//
// The session parameter is the matched session (job.imcSession), not a
// slot-indexed lookup. With externalized KV, any slot can serve any session.
func (m *Model) imcCommitSession(session *imcSession, hash string, totalCached int, cachedMsgCount int, cachedTokens []llama.Token, hasMedia bool, mediaKVCounts []int, sysHash string, sysToks int) {
	if session == nil {
		return
	}

	m.cacheMu.Lock()
	session.cachedMsgsHash = hash
	session.totalTokensCached = totalCached
	session.cachedMsgCount = cachedMsgCount
	session.lastUsed = time.Now()
	session.pending = false
	session.hasMedia = hasMedia
	session.mediaKVCounts = mediaKVCounts
	session.sysPromptHash = sysHash
	session.sysPromptTokens = sysToks
	if !hasMedia {
		session.useMRoPE = false
	}
	switch {
	case hasMedia:
		session.cachedTokens = nil
	case len(cachedTokens) > 0:
		session.cachedTokens = cachedTokens
	}
	m.cacheMu.Unlock()
	m.notifyIMCSlotAvailable()
}

// imcSysPromptInfo computes the system prompt hash and token count for the
// given message slice and its full tokenization. If the first message has
// role="system", it hashes just that message and determines its token boundary
// by tokenizing the system prompt alone through the template and comparing
// against the full token sequence prefix.
func (m *Model) imcSysPromptInfo(ctx context.Context, msgs []D, allTokens []llama.Token) (string, int) {
	if len(msgs) == 0 {
		return "", 0
	}

	role, _ := msgs[0]["role"].(string)
	if role != "system" {
		return "", 0
	}

	sysHash := hashMessages(msgs[:1])

	// Tokenize just the system prompt to find its token boundary.
	sysD := D{
		"messages":              m.imcEnsureUserMessage(ctx, msgs[:1]),
		"add_generation_prompt": false,
	}

	sysPrompt, _, err := m.createPrompt(ctx, sysD)
	if err != nil {
		return sysHash, 0
	}

	sysTokens := llama.Tokenize(m.vocab, sysPrompt, m.addBOSToken, true)

	// Find how many leading tokens match between the system-only render and
	// the full conversation. When imcEnsureUserMessage injects an empty user
	// turn (required by some GGUF templates), sysTokens contains extra tokens
	// after the real system prompt boundary. tokenPrefixMatch returns the
	// exact position where the two sequences diverge — which is the true
	// system prompt token boundary.
	matched := tokenPrefixMatch(sysTokens, allTokens)
	if matched == 0 {
		return sysHash, 0
	}

	return sysHash, matched
}

// tokenPrefixMatch returns the number of tokens that match between two slices,
// starting from the beginning. Used to find the longest common prefix between
// a session's cached tokens and a new request's tokens.
func tokenPrefixMatch(cached, incoming []llama.Token) int {
	n := min(len(cached), len(incoming))
	for i := range n {
		if cached[i] != incoming[i] {
			return i
		}
	}
	return n
}

// imcEnsureUserMessage returns a message slice guaranteed to contain at least
// one user message. If the first message has role "system" and no user message
// exists in msgs, an empty user message is injected at position 1. The
// injected message satisfies GGUF embedded templates that raise when no user
// query is present, while keeping the token sequence consistent across all
// IMC renders and hashes (the injection is always in the same position).
// The original slice is never mutated; a new slice is returned when injection
// is needed.
func (m *Model) imcEnsureUserMessage(ctx context.Context, msgs []D) []D {
	if len(msgs) == 0 {
		return msgs
	}

	role, _ := msgs[0]["role"].(string)
	if role != "system" {
		return msgs
	}

	// Check if any message already has role "user".
	for _, msg := range msgs {
		if r, _ := msg["role"].(string); r == "user" {
			return msgs
		}
	}

	m.log(ctx, "imc", "status", "injecting empty user message", "msgs", len(msgs))

	// Inject empty user message at position 1 (after system).
	result := make([]D, 0, len(msgs)+1)
	result = append(result, msgs[0])
	result = append(result, D{"role": "user", "content": ""})
	result = append(result, msgs[1:]...)
	return result
}

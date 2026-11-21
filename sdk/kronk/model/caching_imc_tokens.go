package model

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// processIMCTokenPlan selects a text session using cached tokens as the
// authority. Only complete cached sequences are reusable; divergence never
// trims an existing session and instead rebuilds an empty/LRU session.
func (m *Model) processIMCTokenPlan(ctx context.Context, d D, actual, stable, system []llama.Token, requestStart time.Time) cacheResult {
	result := cacheResult{modifiedD: d}
	if len(actual) == 0 || !tokensHavePrefix(actual, stable) {
		return result
	}

	// Keep a non-empty inference tail even when the template renders the same
	// tokens with and without add_generation_prompt. Its final decode produces
	// logits and captures pendingH for shared-KV Gemma MTP without changing the
	// rendered prompt.
	targetLen := min(len(stable), len(actual)-1)
	target := slices.Clone(actual[:targetLen])
	tail := slices.Clone(actual[targetLen:])
	if len(system) >= targetLen || !tokensHavePrefix(target, system) {
		m.log(ctx, "imc", "status", "system-cache-skipped", "reason", "render-not-prefix-compatible")
		system = nil
	}
	renderFingerprint, fingerprintOK := m.imcRenderFingerprint(d, dMessages(d))
	result.imcTokenPlan = true
	result.imcSamplerPromptTokens = slices.Clone(actual)
	result.imcTailTokens = tail

	if targetLen < m.cfg.CacheMinTokens() || len(m.imcSessions) == 0 {
		result.imcTailTokens = slices.Clone(actual)
		m.log(ctx, "imc", "status", "plan-ready", "cache_mode", "token-v2", "session_format", "token-v2",
			"match_kind", "rebuild", "reusable_tokens", 0, "extension_tokens", 0, "tail_tokens", len(actual),
			"actual_tokens", len(actual), "stable_tokens", targetLen, "reason", "below-cache-minimum")
		return result
	}

	m.cacheMu.Lock()
	hashes := m.imcCandidateHashes(target, nil)

	matches := func(session *imcSession) bool {
		return imcTextPrefixMatches(session, target, renderFingerprint, fingerprintOK)
	}

	var best *imcSession
	var bestLen int
	for _, session := range m.imcSessions {
		if session.reserved {
			continue
		}
		if session.matchesPrefixHash(hashes) && matches(session) {
			if len(session.cache.cachedTokens) > bestLen {
				best = session
				bestLen = len(session.cache.cachedTokens)
			}
		}
	}

	if best == nil {
		best = m.imcFindStoredSession(ctx, target, nil, matches)
	}

	// System is an immutable preload tier. Search it only when no Current
	// working cache can serve the request, then reserve a separate destination
	// session for the new Current state.
	var systemCache *imcSystemCache
	if best == nil {
		systemCache = m.imcFindSystemCache(system)
	}

	matchKind := "rebuild"
	matchReason := "no-complete-prefix"
	reusable := 0
	extension := target
	clearSeq := true
	var selected *imcSession
	if best != nil {
		selected = best
		reusable = len(selected.cache.cachedTokens)
		extension = slices.Clone(target[reusable:])
		clearSeq = false
		if len(extension) == 0 {
			matchKind = "exact"
			matchReason = "complete-prefix-equal"
			selected.reserved = true
		} else {
			matchKind = "append"
			matchReason = "complete-prefix-append"
			selected.reserved = true
		}
		selected.lastUsed = time.Now()
	} else {
		selected = m.imcRebuildSession()
		if selected == nil {
			if systemCache != nil {
				systemCache.activeRestores--
			}
			m.cacheMu.Unlock()
			result.err = fmt.Errorf("imc: server busy processing other requests, try again shortly")
			return result
		}
		selected.reserved = true
		m.cacheMu.Unlock()
		resetErr := resetSessionForRequest(ctx, selected, messageCount(d))
		m.cacheMu.Lock()
		resetSessionMetadata(selected)
		m.imcNewStoredIdentity(selected)
		selected.reserved = true
		if resetErr != nil {
			selected.reserved = false
			if systemCache != nil {
				systemCache.activeRestores--
			}
			m.cacheMu.Unlock()
			result.err = fmt.Errorf("imc: reset session store: %w", resetErr)
			return result
		}
		if systemCache != nil {
			reusable = len(systemCache.cachedTokens)
			extension = slices.Clone(target[reusable:])
			clearSeq = false
			matchKind = "append"
			matchReason = "system-cache-prefix"
		}
	}

	result.cacheIdx = llama.Pos(reusable)
	result.imcSession = selected
	result.imcSessionID = selected.id
	result.imcExpectedHash = selected.cache.cachedMsgsHash
	result.imcExpectedCachedMsgs = selected.cache.cachedMsgCount
	result.imcExpectedTokens = selected.cache.totalTokensCached
	result.imcExpectedPosition = selected.cache.logicalPosition()
	if fingerprintOK {
		result.imcExpectedRenderHash = renderFingerprint
	}
	result.imcNewCacheTokens = extension
	result.imcNewTotalCached = targetLen
	result.imcNewCachedMsgCount = messageCount(d)
	result.imcNewMsgsHash = documentMessagesHash(d)
	result.imcClearSeq = clearSeq
	result.imcNewCachedTokens = target
	result.imcMatchKind = matchKind
	result.imcReadOnlyReservation = matchKind == "exact"
	result.imcPureHitSkipSnapshot = matchKind == "exact"
	result.imcSystemCache = systemCache
	if systemCache == nil && len(system) >= m.cfg.CacheMinTokens() && reusable == 0 && len(system) < targetLen && tokensHavePrefix(target, system) {
		result.imcSystemBoundaryTokens = len(system)
	}
	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "plan-ready", "cache_mode", "token-v2", "session_format", "token-v2",
		"imc_cache_entry", selected.id, "match_kind", matchKind, "match_reason", matchReason, "reusable_tokens", reusable,
		"system_boundary_tokens", result.imcSystemBoundaryTokens,
		"full_input_snapshot_tokens", targetLen, "full_input_snapshot_messages", messageCount(d),
		"extension_tokens", len(extension), "tail_tokens", len(tail), "actual_tokens", len(actual),
		"stable_tokens", targetLen, "logical_units", targetLen, "text_tokens", targetLen, "kv_positions", targetLen,
		"request_age", fmtDur(time.Since(requestStart)))

	return result
}

func imcTextPrefixMatches(session *imcSession, target []llama.Token, fingerprint string, fingerprintOK bool) bool {
	c := session.cache
	return c.totalTokensCached > 0 && !c.hasMedia && len(c.cachedTokens) > 0 && session.kvState != nil && session.kvState.Len() > 0 &&
		tokensHavePrefix(target, c.cachedTokens) && (len(c.cachedTokens) != len(target) || exactRenderFingerprintMatches(c.cachedRenderInputHash, fingerprint, fingerprintOK))
}

func commonTokenPrefixLen(a, b []llama.Token) int {
	limit := min(len(a), len(b))
	for i := range limit {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func tokensHavePrefix(tokens, prefix []llama.Token) bool {
	return len(prefix) <= len(tokens) && slices.Equal(tokens[:len(prefix)], prefix)
}

func messageCount(d D) int {
	return len(dMessages(d))
}

func documentMessagesHash(d D) string {
	return hashMessages(dMessages(d))
}

func dMessages(d D) []D {
	messages, _ := d["messages"].([]D)
	return messages
}

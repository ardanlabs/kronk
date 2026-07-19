package model

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func (m *Model) processIMCMediaTokenPlan(ctx context.Context, d, stableD D, actualPrompt, stablePrompt string, actualMedia, stableMedia [][]byte, requestStart time.Time) cacheResult {
	result := cacheResult{modifiedD: d}
	actual, err := buildPromptPlan(m.vocab, actualPrompt, actualMedia)
	if err != nil {
		m.log(ctx, "imc-media-cache", "status", "plan-fallback", "cache_mode", "token-v2", "reason", "actual-plan-invalid")
		return result
	}
	stable, err := buildPromptPlan(m.vocab, stablePrompt, stableMedia)
	if err != nil || !actual.hasPrefix(stable) {
		m.log(ctx, "imc-media-cache", "status", "plan-fallback", "cache_mode", "token-v2", "reason", "render-not-prefix-compatible")
		return result
	}
	tail, ok := actual.textTail(stable)
	if !ok || len(tail) == 0 {
		m.log(ctx, "imc-media-cache", "status", "plan-fallback", "cache_mode", "token-v2", "reason", "non-text-or-empty-tail")
		return result
	}
	return m.processIMCMediaPlans(ctx, d, stableD, actual, stable, tail, requestStart)
}

func (m *Model) processIMCMediaPlans(ctx context.Context, d, stableD D, actual, stable promptPlan, defaultTail []llama.Token, requestStart time.Time) cacheResult {
	result := cacheResult{modifiedD: d}
	result.imcTokenPlan = true
	result.imcTailTokens = slices.Clone(defaultTail)
	result.imcPromptPlan = stable
	result.imcNewCachedMsgCount = messageCount(d)
	result.imcNewMsgsHash = documentMessagesHash(d)
	result.imcMediaCacheD = stableD

	m.cacheMu.Lock()
	var match, empty, lru *imcSession
	for _, session := range m.imcSessions {
		if session.pending {
			continue
		}
		if session.totalTokensCached == 0 {
			if empty == nil {
				empty = session
			}
			continue
		}
		if lru == nil || session.lastUsed.Before(lru.lastUsed) {
			lru = session
		}
		if !validMediaAnchorSession(session) || !stable.hasPrefix(session.promptPlan) {
			continue
		}
		_, stableTextOnly := stable.textTail(session.promptPlan)
		actualTail, actualTextOnly := actual.textTail(session.promptPlan)
		if !stableTextOnly || !actualTextOnly || len(actualTail) == 0 || stable.mediaCount != session.promptPlan.mediaCount {
			continue
		}
		if match == nil || len(session.promptPlan.units) > len(match.promptPlan.units) {
			match = session
		}
	}

	selected := match
	matchKind := "rebuild"
	matchReason := "no-exact-media-plan"
	if match != nil {
		extension, _ := stable.textTail(match.promptPlan)
		actualTail, _ := actual.textTail(match.promptPlan)
		switch {
		case len(extension) == 0 && stable.equal(match.promptPlan):
			matchKind = "exact"
			matchReason = "logical-plan-equal"
		case len(extension) > 0:
			matchKind = "anchor"
			matchReason = "media-prefix-text-replay"
			result.imcMediaAnchorAdvance = true
			result.imcNewCacheTokens = slices.Clone(extension)
			result.imcNewTotalCached = match.totalTokensCached + len(extension)
			result.imcNewLogicalPosition = match.logicalPosition() + len(extension)
			result.imcMediaKVCounts = slices.Clone(match.mediaKVCounts)
			result.imcTailTokens = slices.Clone(defaultTail)
		default:
			selected = nil
		}
		if selected != nil {
			result.cacheIdx = llama.Pos(match.logicalPosition())
			if !result.imcMediaAnchorAdvance {
				result.imcTailTokens = slices.Clone(actualTail)
			}
			match.pending = true
		}
	}
	if selected == nil {
		selected = empty
		if selected == nil {
			selected = lru
		}
		if selected == nil {
			m.cacheMu.Unlock()
			result.err = fmt.Errorf("imc: server busy processing other requests, try again shortly")
			return result
		}
		imcResetSession(selected)
		selected.pending = true
		result.imcMediaBuild = true
		result.imcClearSeq = true
	}
	if matchKind != "exact" && !result.imcMediaAnchorAdvance {
		selected.lastUsed = time.Now()
	}
	result.imcSession = selected
	result.imcSessionID = selected.id
	result.imcMatchKind = matchKind
	result.imcExpectedHash = selected.cachedMsgsHash
	result.imcExpectedCachedMsgs = selected.cachedMsgCount
	result.imcExpectedTokens = selected.totalTokensCached
	result.imcExpectedPosition = selected.logicalPosition()
	result.imcExpectedPromptPlan = selected.promptPlan
	if fingerprint, ok := m.imcRenderFingerprint(d, dMessages(d)); ok {
		result.imcExpectedRenderHash = fingerprint
	}
	result.imcReadOnlyReservation = matchKind == "exact"
	result.imcPureHitSkipSnapshot = result.imcReadOnlyReservation
	m.cacheMu.Unlock()

	m.log(ctx, "imc-media-cache", "status", "plan-ready", "cache_mode", "token-v2", "session_format", "token-v2",
		"media_count", stable.mediaCount, "logical_units", len(stable.units), "text_tokens", stable.textTokens,
		"match_kind", matchKind, "match_reason", matchReason, "reusable_logical_position", result.cacheIdx, "anchor_physical_kv", result.imcExpectedTokens,
		"anchor_logical_position", result.imcExpectedPosition, "replay_text_tokens", len(result.imcTailTokens), "extension_text", len(result.imcNewCacheTokens),
		"extension_media", 0, "position_mode", "linear-or-mrope", "request_age", fmtDur(time.Since(requestStart)))

	return result
}

func validMediaAnchorSession(session *imcSession) bool {
	if session == nil || !session.hasMedia || session.totalTokensCached <= 0 ||
		session.promptPlan.mediaCount == 0 || session.kvState == nil || session.kvState.Len() == 0 ||
		len(session.mediaKVCounts) != session.promptPlan.mediaCount {
		return false
	}

	mediaCells := 0
	for _, count := range session.mediaKVCounts {
		if count <= 0 {
			return false
		}
		mediaCells += count
	}
	if mediaCells > session.totalTokensCached {
		return false
	}

	switch {
	case session.useMRoPE:
		return session.nextLogicalPos > 0
	default:
		return session.logicalPosition() == session.totalTokensCached
	}
}

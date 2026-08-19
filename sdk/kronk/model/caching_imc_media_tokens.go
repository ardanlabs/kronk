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
	result := cacheResult{modifiedD: d,
		imcTokenPlan:         true,
		imcTailTokens:        slices.Clone(defaultTail),
		imcPromptPlan:        stable,
		imcNewCachedMsgCount: messageCount(d),
		imcNewMsgsHash:       documentMessagesHash(d),
		imcMediaCacheD:       stableD}
	renderFingerprint, fingerprintOK := m.imcRenderFingerprint(d, dMessages(d))

	m.cacheMu.Lock()
	var match, empty, lru *imcSession
	for _, session := range m.imcSessions {
		if session.reserved {
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
		mediaAnchor := validMediaAnchorSession(session) && stable.hasPrefix(session.promptPlan)
		textAnchor := validTextToMediaAnchorSession(session, stable)
		if !mediaAnchor && !textAnchor {
			continue
		}
		if mediaAnchor && stable.equal(session.promptPlan) && !exactRenderFingerprintMatches(session.cachedRenderInputHash, renderFingerprint, fingerprintOK) {
			continue
		}
		extensionHasMedia := slices.ContainsFunc(stable.units[len(session.promptPlan.units):], func(unit promptUnit) bool {
			return unit.isMedia
		})
		if extensionHasMedia && (session.useNonCausal || (session.hasMedia && len(session.mediaNativeChunks) == 0)) {
			continue
		}
		actualTail, actualTextOnly := actual.textTail(stable)
		if !actualTextOnly || len(actualTail) == 0 {
			continue
		}
		if match == nil || session.logicalPosition() > match.logicalPosition() {
			match = session
		}
	}

	selected := match
	matchKind := "rebuild"
	matchReason := "no-exact-media-plan"
	if match != nil {
		extension := stable.units[len(match.promptPlan.units):]
		extensionHasMedia := slices.ContainsFunc(extension, func(unit promptUnit) bool {
			return unit.isMedia
		})
		switch {
		case len(extension) == 0 && stable.equal(match.promptPlan):
			matchKind = "exact"
			matchReason = "logical-plan-equal"
		case extensionHasMedia:
			matchKind = "media-append"
			if match.hasMedia {
				matchReason = "media-prefix-append"
			} else {
				matchReason = "text-prefix-media-append"
			}
			result.imcMediaAnchorAdvance = true
			result.imcMediaAppend = true
			result.imcMediaKVCounts = slices.Clone(match.mediaKVCounts)
		case len(extension) > 0:
			matchKind = "anchor"
			matchReason = "media-prefix-text-replay"
			result.imcMediaAnchorAdvance = true
			result.imcNewCacheTokens = make([]llama.Token, 0, len(extension))
			for _, unit := range extension {
				result.imcNewCacheTokens = append(result.imcNewCacheTokens, unit.token)
			}
			result.imcNewTotalCached = match.totalTokensCached + len(extension)
			result.imcNewLogicalPosition = match.logicalPosition() + len(extension)
			result.imcMediaKVCounts = slices.Clone(match.mediaKVCounts)
		default:
			selected = nil
		}
		if selected != nil {
			result.cacheIdx = llama.Pos(match.logicalPosition())
			result.imcMediaSamplerTokens = slices.Clone(match.samplerPromptTokens)
			if !result.imcMediaAppend {
				result.imcMediaSamplerTokens = append(result.imcMediaSamplerTokens, result.imcNewCacheTokens...)
				result.imcSamplerPromptTokens = slices.Clone(result.imcMediaSamplerTokens)
				result.imcSamplerPromptTokens = append(result.imcSamplerPromptTokens, result.imcTailTokens...)
			}
			match.reserved = true
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
		selected.reserved = true
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
	if fingerprintOK {
		result.imcExpectedRenderHash = renderFingerprint
	}
	result.imcReadOnlyReservation = matchKind == "exact"
	result.imcPureHitSkipSnapshot = result.imcReadOnlyReservation
	m.cacheMu.Unlock()

	extensionText := len(result.imcNewCacheTokens)
	extensionMedia := 0
	if result.imcMediaAppend {
		extension := stable.units[len(result.imcExpectedPromptPlan.units):]
		for _, unit := range extension {
			if unit.isMedia {
				extensionMedia++
			} else {
				extensionText++
			}
		}
	}
	m.log(ctx, "imc-media-cache", "status", "plan-ready", "cache_mode", "token-v2", "session_format", "token-v2",
		"imc_cache_entry", selected.id, "media_count", stable.mediaCount, "logical_units", len(stable.units), "text_tokens", stable.textTokens,
		"match_kind", matchKind, "match_reason", matchReason, "reusable_logical_position", result.cacheIdx, "anchor_physical_kv", result.imcExpectedTokens,
		"anchor_logical_position", result.imcExpectedPosition, "replay_text_tokens", len(result.imcTailTokens), "extension_text", extensionText,
		"extension_media", extensionMedia, "position_mode", "linear-or-mrope", "request_age", fmtDur(time.Since(requestStart)))

	return result
}

func validMediaAnchorSession(session *imcSession) bool {
	if session == nil || !session.hasMedia || session.totalTokensCached <= 0 ||
		session.promptPlan.mediaCount == 0 || session.kvState == nil || session.kvState.Len() == 0 ||
		len(session.mediaKVCounts) < session.promptPlan.mediaCount || len(session.samplerPromptTokens) == 0 {
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

func validTextToMediaAnchorSession(session *imcSession, stable promptPlan) bool {
	if session == nil || session.hasMedia || session.totalTokensCached <= 0 ||
		len(session.cachedTokens) != session.totalTokensCached || session.logicalPosition() != session.totalTokensCached ||
		session.kvState == nil || session.kvState.Len() == 0 || len(session.cachedTokens) >= len(stable.units) {
		return false
	}

	for i, token := range session.cachedTokens {
		unit := stable.units[i]
		if unit.isMedia || unit.token != token {
			return false
		}
	}

	return slices.ContainsFunc(stable.units[len(session.cachedTokens):], func(unit promptUnit) bool {
		return unit.isMedia
	})
}

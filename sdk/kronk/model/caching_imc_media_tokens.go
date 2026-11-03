package model

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func (m *Model) processIMCMediaTokenPlan(ctx context.Context, d, stableD D, actualPrompt, stablePrompt string, actualMedia, stableMedia [][]byte, system []llama.Token, requestStart time.Time) cacheResult {
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
	return m.processIMCMediaPlans(ctx, d, stableD, actual, stable, tail, system, requestStart)
}

func (m *Model) processIMCMediaPlans(ctx context.Context, d, stableD D, actual, stable promptPlan, defaultTail, system []llama.Token, requestStart time.Time) cacheResult {
	result := cacheResult{modifiedD: d,
		imcTokenPlan:         true,
		imcTailTokens:        slices.Clone(defaultTail),
		imcPromptPlan:        stable,
		imcNewCachedMsgCount: messageCount(d),
		imcNewMsgsHash:       documentMessagesHash(d),
		imcMediaCacheD:       stableD}
	if len(system) >= len(stable.units) || !stable.hasTextPrefix(system) {
		m.log(ctx, "imc", "status", "system-cache-skipped", "reason", "render-not-prefix-compatible")
		system = nil
	}
	renderFingerprint, fingerprintOK := m.imcRenderFingerprint(d, dMessages(d))

	m.cacheMu.Lock()
	hashes := m.imcCandidateHashes(nil, stable.units)
	var match *imcSession
	var reserved, emptyCount, missingCheckpoint, incompatible, prefixMismatch, fingerprintMismatch, matches int
	for _, session := range m.imcSessions {
		cache := session.cache
		if session.reserved {
			reserved++
			continue
		}
		if cache.totalTokensCached == 0 {
			emptyCount++
			continue
		}
		if session.kvState == nil || session.kvState.Len() == 0 {
			missingCheckpoint++
			continue
		}
		validMediaAnchor := validMediaAnchorSession(session)
		hashMatch := session.matchesPrefixHash(hashes)
		mediaAnchor := hashMatch && validMediaAnchor && stable.hasPrefix(cache.promptPlan)
		textAnchor := hashMatch && validTextToMediaAnchorSession(session, stable)
		if !mediaAnchor && !textAnchor {
			if validMediaAnchor || (!cache.hasMedia && len(cache.cachedTokens) == cache.totalTokensCached && cache.logicalPosition() == cache.totalTokensCached) {
				prefixMismatch++
			} else {
				incompatible++
			}
			continue
		}
		if mediaAnchor && stable.equal(cache.promptPlan) && !exactRenderFingerprintMatches(cache.cachedRenderInputHash, renderFingerprint, fingerprintOK) {
			fingerprintMismatch++
			continue
		}
		extensionHasMedia := slices.ContainsFunc(stable.units[len(cache.promptPlan.units):], func(unit promptUnit) bool {
			return unit.isMedia
		})
		if extensionHasMedia && (cache.useNonCausal || (cache.hasMedia && len(cache.mediaNativeChunks) == 0)) {
			incompatible++
			continue
		}
		actualTail, actualTextOnly := actual.textTail(stable)
		if !actualTextOnly || len(actualTail) == 0 {
			incompatible++
			continue
		}
		matches++
		if match == nil || cache.logicalPosition() > match.cache.logicalPosition() {
			match = session
		}
	}

	var selected *imcSession
	matchKind := "rebuild"
	matchReason := "no-exact-media-plan"
	selection := "matched-prefix"
	previousTokens := 0
	if match != nil {
		selected = match
		cache := selected.cache
		extension := stable.units[len(cache.promptPlan.units):]
		extensionHasMedia := slices.ContainsFunc(extension, func(unit promptUnit) bool {
			return unit.isMedia
		})
		switch {
		case len(extension) == 0 && stable.equal(cache.promptPlan):
			matchKind = "exact"
			matchReason = "logical-plan-equal"
		case extensionHasMedia:
			matchKind = "media-append"
			if cache.hasMedia {
				matchReason = "media-prefix-append"
			} else {
				matchReason = "text-prefix-media-append"
			}
			result.imcMediaAnchorAdvance = true
			result.imcMediaAppend = true
			result.imcMediaKVCounts = slices.Clone(cache.mediaKVCounts)
		case len(extension) > 0:
			matchKind = "anchor"
			matchReason = "media-prefix-text-replay"
			result.imcMediaAnchorAdvance = true
			result.imcNewCacheTokens = make([]llama.Token, 0, len(extension))
			for _, unit := range extension {
				result.imcNewCacheTokens = append(result.imcNewCacheTokens, unit.token)
			}
			result.imcNewTotalCached = cache.totalTokensCached + len(extension)
			result.imcNewLogicalPosition = cache.logicalPosition() + len(extension)
			result.imcMediaKVCounts = slices.Clone(cache.mediaKVCounts)
		default:
			selected = nil
		}
		if selected != nil {
			result.cacheIdx = llama.Pos(cache.logicalPosition())
			result.imcMediaSamplerTokens = slices.Clone(cache.samplerPromptTokens)
			if !result.imcMediaAppend {
				result.imcMediaSamplerTokens = append(result.imcMediaSamplerTokens, result.imcNewCacheTokens...)
				result.imcSamplerPromptTokens = slices.Clone(result.imcMediaSamplerTokens)
				result.imcSamplerPromptTokens = append(result.imcSamplerPromptTokens, result.imcTailTokens...)
			}
			selected.reserved = true
		}
	}
	if selected == nil {
		selected, selection = m.imcRebuildSession()
		if selected == nil {
			m.cacheMu.Unlock()
			m.log(ctx, "imc-media-cache", "status", "plan-unavailable", "reason", "no-available-resident",
				"candidates_reserved", reserved, "candidates_empty", emptyCount,
				"candidates_missing_checkpoint", missingCheckpoint, "candidates_incompatible", incompatible,
				"candidates_prefix_mismatch", prefixMismatch, "candidates_fingerprint_mismatch", fingerprintMismatch, "candidates_matching", matches)
			result.err = fmt.Errorf("imc: server busy processing other requests, try again shortly")
			return result
		}
		previousTokens = selected.cache.totalTokensCached
		selected.reserved = true
		m.cacheMu.Unlock()
		resetErr := resetSessionStores(ctx, selected)
		m.cacheMu.Lock()
		resetSessionMetadata(selected)
		selected.reserved = true
		if resetErr != nil {
			selected.reserved = false
			tokenCacheID := selected.id
			m.cacheMu.Unlock()
			m.log(ctx, "imc-media-cache", "status", "plan-reset-failed", "token_cache_id", tokenCacheID,
				"imc_cache_entry", selected.id, "selection", selection, "previous_cached_tokens", previousTokens, "err", resetErr)
			result.err = fmt.Errorf("imc: reset session store: %w", resetErr)
			return result
		}
		result.imcMediaBuild = true
		result.imcClearSeq = true
		result.imcSystemCache = m.imcFindSystemCache(system)
		if result.imcSystemCache != nil {
			result.cacheIdx = llama.Pos(len(system))
			result.imcClearSeq = false
			matchKind = "append"
			matchReason = "system-cache-prefix"
		} else if len(system) > 0 && len(system) >= m.cfg.CacheMinTokens() {
			result.imcSystemBoundaryTokens = len(system)
			result.imcNewCachedTokens = slices.Clone(system)
		}
	}
	if matchKind != "exact" && !result.imcMediaAnchorAdvance {
		selected.lastUsed = time.Now()
	}
	result.imcSession = selected
	result.imcSessionID = selected.id
	result.imcMatchKind = matchKind
	result.imcExpectedHash = selected.cache.cachedMsgsHash
	result.imcExpectedCachedMsgs = selected.cache.cachedMsgCount
	result.imcExpectedTokens = selected.cache.totalTokensCached
	result.imcExpectedPosition = selected.cache.logicalPosition()
	result.imcExpectedPromptPlan = selected.cache.promptPlan
	if fingerprintOK {
		result.imcExpectedRenderHash = renderFingerprint
	}
	result.imcReadOnlyReservation = matchKind == "exact"
	result.imcPureHitSkipSnapshot = result.imcReadOnlyReservation
	tokenCacheID := selected.id
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
		"token_cache_id", tokenCacheID, "selection", selection, "previous_cached_tokens", previousTokens,
		"candidates_reserved", reserved, "candidates_empty", emptyCount,
		"candidates_missing_checkpoint", missingCheckpoint, "candidates_incompatible", incompatible,
		"candidates_prefix_mismatch", prefixMismatch, "candidates_fingerprint_mismatch", fingerprintMismatch, "candidates_matching", matches,
		"imc_cache_entry", selected.id, "media_count", stable.mediaCount, "logical_units", len(stable.units), "text_tokens", stable.textTokens,
		"match_kind", matchKind, "match_reason", matchReason, "reusable_logical_position", result.cacheIdx, "anchor_physical_kv", result.imcExpectedTokens,
		"anchor_logical_position", result.imcExpectedPosition, "replay_text_tokens", len(result.imcTailTokens), "extension_text", extensionText,
		"extension_media", extensionMedia, "position_mode", "linear-or-mrope", "request_age", fmtDur(time.Since(requestStart)))

	return result
}

func validMediaAnchorSession(session *imcSession) bool {
	if session == nil || session.cache == nil || session.kvState == nil || session.kvState.Len() == 0 {
		return false
	}
	cache := session.cache
	if !cache.hasMedia || cache.totalTokensCached <= 0 || cache.promptPlan.mediaCount == 0 ||
		len(cache.mediaKVCounts) < cache.promptPlan.mediaCount || len(cache.samplerPromptTokens) == 0 {
		return false
	}

	mediaCells := 0
	for _, count := range cache.mediaKVCounts {
		if count <= 0 {
			return false
		}
		mediaCells += count
	}
	if mediaCells > cache.totalTokensCached {
		return false
	}

	switch {
	case cache.useMRoPE:
		return cache.nextLogicalPos > 0
	default:
		return cache.logicalPosition() == cache.totalTokensCached
	}
}

func validTextToMediaAnchorSession(session *imcSession, stable promptPlan) bool {
	if session == nil || session.cache == nil || session.kvState == nil || session.kvState.Len() == 0 {
		return false
	}
	cache := session.cache
	if cache.hasMedia || cache.totalTokensCached <= 0 ||
		len(cache.cachedTokens) != cache.totalTokensCached || cache.logicalPosition() != cache.totalTokensCached ||
		len(cache.cachedTokens) >= len(stable.units) {
		return false
	}

	if !stable.hasTextPrefix(cache.cachedTokens) {
		return false
	}

	return slices.ContainsFunc(stable.units[len(cache.cachedTokens):], func(unit promptUnit) bool {
		return unit.isMedia
	})
}

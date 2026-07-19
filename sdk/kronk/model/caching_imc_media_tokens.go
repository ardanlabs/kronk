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
	actual, err := buildPromptPlan(m.vocab, actualPrompt, actualMedia, m.addBOSToken)
	if err != nil {
		m.log(ctx, "imc-media-cache", "status", "plan-fallback", "cache_mode", "token-v2", "reason", "actual-plan-invalid")
		return result
	}
	stable, err := buildPromptPlan(m.vocab, stablePrompt, stableMedia, m.addBOSToken)
	if err != nil || !actual.hasPrefix(stable) {
		m.log(ctx, "imc-media-cache", "status", "plan-fallback", "cache_mode", "token-v2", "reason", "render-not-prefix-compatible")
		return result
	}
	tail, ok := actual.textTail(stable)
	if !ok || len(tail) == 0 {
		m.log(ctx, "imc-media-cache", "status", "plan-fallback", "cache_mode", "token-v2", "reason", "non-text-or-empty-tail")
		return result
	}

	result.imcTokenPlan = true
	result.imcTailTokens = slices.Clone(tail)
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
		if !session.hasMedia || len(session.kvState.Bytes()) == 0 || !stable.hasPrefix(session.promptPlan) {
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
		switch {
		case len(extension) == 0 && stable.equal(match.promptPlan):
			matchKind = "exact"
			matchReason = "logical-plan-equal"
			result.cacheIdx = llama.Pos(match.logicalPosition())
			match.pending = true
		default:
			// A logical prompt plan does not prove that an appended text
			// suffix exactly matches mtmd's model-specific decoded chunk
			// stream. Preserve exact media reuse, but rebuild all changed
			// media conversations through the authoritative mtmd pipeline.
			selected = nil
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
	selected.lastUsed = time.Now()
	result.imcSession = selected
	result.imcSessionID = selected.id
	result.imcMatchKind = matchKind
	result.imcExpectedHash = selected.cachedMsgsHash
	result.imcExpectedCachedMsgs = selected.cachedMsgCount
	result.imcExpectedTokens = selected.totalTokensCached
	result.imcExpectedPosition = selected.logicalPosition()
	if fingerprint, ok := m.imcRenderFingerprint(d, dMessages(d)); ok {
		result.imcExpectedRenderHash = fingerprint
	}
	result.imcPureHitSkipSnapshot = matchKind == "exact"
	m.cacheMu.Unlock()

	m.log(ctx, "imc-media-cache", "status", "plan-ready", "cache_mode", "token-v2", "session_format", "token-v2",
		"media_count", stable.mediaCount, "logical_units", len(stable.units), "text_tokens", stable.textTokens,
		"match_kind", matchKind, "match_reason", matchReason, "reusable_kv", result.cacheIdx, "extension_text", len(result.imcNewCacheTokens),
		"extension_media", 0, "position_mode", "linear-or-mrope", "request_age", fmtDur(time.Since(requestStart)))

	return result
}

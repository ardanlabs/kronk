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
func (m *Model) processIMCTokenPlan(ctx context.Context, d D, actual, stable []llama.Token, requestStart time.Time) cacheResult {
	result := cacheResult{modifiedD: d}
	if len(actual) == 0 || len(stable) >= len(actual) || !tokensHavePrefix(actual, stable) {
		return result
	}

	// The generation-enabled render must contribute a non-empty inference
	// tail. Besides producing logits, its final decode captures pendingH for
	// shared-KV Gemma MTP.
	targetLen := len(stable)
	target := slices.Clone(actual[:targetLen])
	tail := slices.Clone(actual[targetLen:])
	result.imcTokenPlan = true
	result.imcActualTokens = slices.Clone(actual)
	result.imcTailTokens = tail

	if targetLen < m.cfg.CacheMinTokens() || len(m.imcSessions) == 0 {
		result.imcTailTokens = slices.Clone(actual)
		m.log(ctx, "imc", "status", "plan-ready", "cache_mode", "token-v2", "session_format", "token-v2",
			"match_kind", "rebuild", "reusable_tokens", 0, "extension_tokens", 0, "tail_tokens", len(actual),
			"actual_tokens", len(actual), "stable_tokens", targetLen, "reason", "below-cache-minimum")
		return result
	}

	m.cacheMu.Lock()
	var best *imcSession
	var empty *imcSession
	var lru *imcSession
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
		if session.hasMedia || len(session.cachedTokens) == 0 || len(session.kvState.Bytes()) == 0 || !tokensHavePrefix(target, session.cachedTokens) {
			continue
		}
		if best == nil || len(session.cachedTokens) > len(best.cachedTokens) {
			best = session
		}
	}

	matchKind := "rebuild"
	matchReason := "no-complete-prefix"
	reusable := 0
	extension := target
	clearSeq := true
	selected := best
	if best != nil {
		reusable = len(best.cachedTokens)
		extension = slices.Clone(target[reusable:])
		clearSeq = false
		if len(extension) == 0 {
			matchKind = "exact"
			matchReason = "complete-prefix-equal"
			best.pending = true
		} else {
			matchKind = "append"
			matchReason = "complete-prefix-append"
			best.pending = true
		}
		best.lastUsed = time.Now()
	} else {
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
	}

	result.cacheIdx = llama.Pos(reusable)
	result.imcSession = selected
	result.imcSessionID = selected.id
	result.imcExpectedHash = selected.cachedMsgsHash
	result.imcExpectedCachedMsgs = selected.cachedMsgCount
	result.imcExpectedTokens = selected.totalTokensCached
	result.imcExpectedPosition = selected.logicalPosition()
	if fingerprint, ok := m.imcRenderFingerprint(d, dMessages(d)); ok {
		result.imcExpectedRenderHash = fingerprint
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
	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "plan-ready", "cache_mode", "token-v2", "session_format", "token-v2",
		"imc_slot", selected.id, "match_kind", matchKind, "match_reason", matchReason, "reusable_tokens", reusable,
		"extension_tokens", len(extension), "tail_tokens", len(tail), "actual_tokens", len(actual),
		"stable_tokens", targetLen, "logical_units", targetLen, "text_tokens", targetLen, "kv_positions", targetLen,
		"request_age", fmtDur(time.Since(requestStart)))

	return result
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

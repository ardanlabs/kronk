package model

import (
	"context"
	"fmt"
	"slices"
	"strings"
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
	var best *imcSession
	var bestIsCheckpoint bool
	var bestLen int
	var candidateLCP int
	var empty *imcSession
	var lru *imcSession
	for _, session := range m.imcSessions {
		if session.reserved {
			continue
		}
		checkpointOccupied := session.turnCheckpoint != nil && session.turnCheckpoint.totalTokensCached > 0
		if session.totalTokensCached == 0 && !checkpointOccupied {
			if empty == nil {
				empty = session
			}
			continue
		}
		if lru == nil || session.lastUsed.Before(lru.lastUsed) {
			lru = session
		}
		if !session.hasMedia && len(session.cachedTokens) > 0 {
			lcp := commonTokenPrefixLen(session.cachedTokens, target)
			if lcp >= m.cfg.CacheMinTokens() && lcp < len(session.cachedTokens) && lcp < len(target) {
				candidateLCP = max(candidateLCP, lcp)
			}
		}
		rollingExact := len(session.cachedTokens) == len(target)
		rollingFingerprintOK := !rollingExact || exactRenderFingerprintMatches(session.cachedRenderInputHash, renderFingerprint, fingerprintOK)
		if !session.hasMedia && len(session.cachedTokens) > 0 && session.kvState != nil && len(session.kvState.Bytes()) > 0 &&
			tokensHavePrefix(target, session.cachedTokens) && rollingFingerprintOK {
			if len(session.cachedTokens) > bestLen || (len(session.cachedTokens) == bestLen && bestIsCheckpoint) {
				best = session
				bestIsCheckpoint = false
				bestLen = len(session.cachedTokens)
			}
		}

		checkpoint := session.turnCheckpoint
		checkpointExact := checkpoint != nil && len(checkpoint.cachedTokens) == len(target)
		checkpointFingerprintOK := !checkpointExact || exactRenderFingerprintMatches(checkpoint.cachedRenderInputHash, renderFingerprint, fingerprintOK)
		if checkpoint != nil && !checkpoint.hasMedia && len(checkpoint.cachedTokens) > 0 && checkpoint.kvState != nil && len(checkpoint.kvState.Bytes()) > 0 &&
			tokensHavePrefix(target, checkpoint.cachedTokens) && checkpointFingerprintOK {
			// Rolling wins ties so the common path does not churn snapshot
			// ownership when both complete states describe the same prefix.
			if len(checkpoint.cachedTokens) > bestLen {
				best = session
				bestIsCheckpoint = true
				bestLen = len(checkpoint.cachedTokens)
			}
		}
	}

	matchKind := "rebuild"
	matchReason := "no-complete-prefix"
	reusable := 0
	extension := target
	clearSeq := true
	selected := best
	if best != nil {
		if bestIsCheckpoint {
			best.swapTurnCheckpoint()
		}
		reusable = len(best.cachedTokens)
		extension = slices.Clone(target[reusable:])
		clearSeq = false
		if len(extension) == 0 {
			matchKind = "exact"
			matchReason = "complete-prefix-equal"
			best.reserved = true
		} else {
			matchKind = "append"
			matchReason = "complete-prefix-append"
			best.reserved = true
		}
		if bestIsCheckpoint {
			matchReason = "turn-checkpoint-prefix"
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
		selected.reserved = true
	}

	result.cacheIdx = llama.Pos(reusable)
	result.imcSession = selected
	result.imcSessionID = selected.id
	result.imcExpectedHash = selected.cachedMsgsHash
	result.imcExpectedCachedMsgs = selected.cachedMsgCount
	result.imcExpectedTokens = selected.totalTokensCached
	result.imcExpectedPosition = selected.logicalPosition()
	if fingerprintOK {
		result.imcExpectedRenderHash = renderFingerprint
	}
	result.imcNewCacheTokens = extension
	result.imcNewTotalCached = targetLen
	result.imcNewCachedMsgCount = messageCount(d)
	result.imcNewMsgsHash = documentMessagesHash(d)
	result.imcNewEndsAtUser = messagesEndAtRealUser(dMessages(d))
	result.imcClearSeq = clearSeq
	result.imcNewCachedTokens = target
	result.imcMatchKind = matchKind
	result.imcReadOnlyReservation = matchKind == "exact"
	result.imcPureHitSkipSnapshot = matchKind == "exact"
	result.imcPromoteCheckpoint = !clearSeq && len(extension) > 0 && selected.rollingEndsAtUser && !selected.hasMedia
	if candidateLCP > reusable {
		result.imcCheckpointTokens = candidateLCP
		result.imcPromoteCheckpoint = false
	}
	m.cacheMu.Unlock()

	m.log(ctx, "imc", "status", "plan-ready", "cache_mode", "token-v2", "session_format", "token-v2",
		"imc_cache_entry", selected.id, "match_kind", matchKind, "match_reason", matchReason, "reusable_tokens", reusable,
		"candidate_lcp_tokens", candidateLCP, "recomputed_to_checkpoint", max(0, result.imcCheckpointTokens-reusable),
		"reusable_snapshot_tokens", result.imcCheckpointTokens, "reusable_snapshot_messages", 0,
		"full_input_snapshot_tokens", targetLen, "full_input_snapshot_messages", messageCount(d),
		"extension_tokens", len(extension), "tail_tokens", len(tail), "actual_tokens", len(actual),
		"stable_tokens", targetLen, "logical_units", targetLen, "text_tokens", targetLen, "kv_positions", targetLen,
		"request_age", fmtDur(time.Since(requestStart)))

	return result
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

func messagesEndAtRealUser(messages []D) bool {
	if len(messages) == 0 {
		return false
	}

	last := messages[len(messages)-1]
	role, _ := last["role"].(string)
	if role != "user" {
		return false
	}
	if _, isToolResponse := last["tool_call_id"]; isToolResponse {
		return false
	}

	content, _ := last["content"].(string)
	content = strings.TrimSpace(content)
	return !(strings.HasPrefix(content, "<tool_response>") && strings.HasSuffix(content, "</tool_response>"))
}

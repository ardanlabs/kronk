package model

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// processSPC orchestrates the system prompt caching flow. It examines
// the first message and either caches it (if it's a system message) or reuses
// an existing cache (if the client omitted the system message on a follow-up
// request). The system message is always removed from d after processing.
//
// Multiple sessions are maintained in a map keyed by system prompt hash so
// that concurrent requests with different system prompts (e.g., title
// generation vs chat) each get their own cached KV state. When a request
// omits the system prompt, it is only reused if exactly one session exists
// (unambiguous). Otherwise the request proceeds without SPC.
//
// The system prompt is decoded once into a temporary sequence, the KV state
// is extracted into an external byte buffer, and the sequence is freed. At
// startSlot time, the KV state is restored into the slot's working sequence
// via restoreSPCToSeq (a StateSeqSetData operation).
func (m *Model) processSPC(ctx context.Context, d D) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	role, ok := messages[0]["role"].(string)
	if !ok {
		return cacheResult{modifiedD: d}
	}

	switch role {
	case RoleSystem:
		content := extractMessageContent(messages[0])
		if content == "" {
			return cacheResult{modifiedD: d}
		}

		sysMsg := cacheableMessage{
			index:   0,
			role:    role,
			content: content,
		}

		result := m.performSPC(ctx, d, messages, sysMsg)
		if result.err != nil {
			return result
		}

		if result.cacheIdx > 0 {
			d = removeMessagesAtIndices(d, []int{0})
			result.modifiedD = d
			return result
		}

		return cacheResult{modifiedD: d}

	default:
		// Client omitted the system prompt on a follow-up request.
		// Only reuse a cached session if exactly one exists (unambiguous).
		m.cacheMu.RLock()
		var session *spcSession
		if len(m.spcSessions) == 1 {
			for _, s := range m.spcSessions {
				session = s
			}
		}
		m.cacheMu.RUnlock()

		if session != nil && session.sysPromptTokens > 0 {
			m.log(ctx, "spc", "status", "cache hit (system prompt excluded on this request)", "tokens", session.sysPromptTokens)
			return cacheResult{
				modifiedD:  d,
				cacheIdx:   llama.Pos(session.sysPromptTokens),
				cacheSeqID: m.spcCacheSeqID,
				spcSession: session,
			}
		}
	}

	return cacheResult{modifiedD: d}
}

// performSPC checks for a cache hit on the system prompt. On a miss, it
// templates, tokenizes, and decodes the system prompt into the dedicated
// cache sequence. On a hit, it returns the cached position and sequence ID
// so that startSlot can copy the KV state into the slot's working sequence.
//
// When the session map is at capacity (spcMaxSessions), the least recently
// used session is evicted before the new session is inserted.
func (m *Model) performSPC(ctx context.Context, d D, messages []D, msgInfo cacheableMessage) cacheResult {
	if msgInfo.role != RoleSystem {
		m.log(ctx, "spc", "status", "no system prompt message provided", "role", msgInfo.role)
		return cacheResult{modifiedD: d}
	}

	contentLen := len(msgInfo.content)
	newHash := hashMessage(msgInfo)

	// Check for cache hit (fast path with read lock).
	m.cacheMu.RLock()
	session := m.spcSessions[newHash]
	m.cacheMu.RUnlock()

	if session != nil && session.sysPromptLen == contentLen && session.sysPromptTokens > 0 {
		m.cacheMu.Lock()
		session.lastUsed = time.Now()
		m.cacheMu.Unlock()

		m.log(ctx, "spc", "status", "cache hit", "tokens", session.sysPromptTokens, "sessions", len(m.spcSessions))
		return cacheResult{
			cacheIdx:   llama.Pos(session.sysPromptTokens),
			cacheSeqID: m.spcCacheSeqID,
			spcSession: session,
		}
	}

	// Cache miss — template and cache the message.
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine cached while we waited.
	session = m.spcSessions[newHash]
	if session != nil && session.sysPromptLen == contentLen && session.sysPromptTokens > 0 {
		session.lastUsed = time.Now()
		m.log(ctx, "spc", "status", "cache hit (after lock)", "tokens", session.sysPromptTokens, "sessions", len(m.spcSessions))
		return cacheResult{
			cacheIdx:   llama.Pos(session.sysPromptTokens),
			cacheSeqID: m.spcCacheSeqID,
			spcSession: session,
		}
	}

	msgsToCache := D{
		"messages":              []D{messages[0]},
		"add_generation_prompt": false,
	}

	systemMsgPrompt, _, err := m.createPrompt(ctx, msgsToCache)
	if err != nil {
		return cacheResult{modifiedD: d, err: fmt.Errorf("spc: failed to template system prompt: %w", err)}
	}

	_, tokenSpan := otel.AddSpan(ctx, "cache-tokenize-spc",
		attribute.String("cache-type", "spc"),
	)

	tokens := llama.Tokenize(m.vocab, systemMsgPrompt, m.addBOSToken, true)
	nTokens := len(tokens)

	tokenSpan.SetAttributes(attribute.Int("tokens", nTokens))
	tokenSpan.End()

	if nTokens == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("spc: system prompt tokenized to zero tokens")}
	}

	if nTokens < m.cfg.CacheMinTokens {
		m.log(ctx, "spc", "status", "cache skip (too short)", "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	// Clear any existing cache in the dedicated sequence.
	llama.MemorySeqRm(m.mem, m.spcCacheSeqID, -1, -1)

	// Decode tokens into the temporary cache sequence.
	if err := m.decodeTokensIntoCache(ctx, tokens, m.spcCacheSeqID, 0); err != nil {
		return cacheResult{modifiedD: d, err: err}
	}

	// Extract the KV state into an external buffer so the sequence can be freed.
	m.decodeMu.Lock()

	kvSize := llama.StateSeqGetSize(m.lctx, m.spcCacheSeqID)
	kvState := make([]byte, kvSize)
	nExtracted := llama.StateSeqGetData(m.lctx, kvState, m.spcCacheSeqID)

	// Free the sequence — KV entries are now in the external buffer.
	llama.MemorySeqRm(m.mem, m.spcCacheSeqID, -1, -1)

	m.decodeMu.Unlock()

	if nExtracted == 0 {
		return cacheResult{modifiedD: d, err: fmt.Errorf("spc: failed to extract KV state from seq %d", m.spcCacheSeqID)}
	}

	// Evict the least recently used session if at capacity.
	if len(m.spcSessions) >= m.spcMaxSessions {
		m.evictLRUSPCSession(ctx)
	}

	newSession := &spcSession{
		sysPromptHash:   newHash,
		sysPromptTokens: nTokens,
		sysPromptLen:    contentLen,
		lastUsed:        time.Now(),
		kvState:         kvState[:nExtracted],
	}

	m.spcSessions[newHash] = newSession

	m.log(ctx, "spc", "tokens", nTokens, "hash", newHash[:8], "kv_bytes", nExtracted, "sessions", len(m.spcSessions), "status", "tokens saved (externalized)")

	return cacheResult{
		cacheIdx:   llama.Pos(nTokens),
		cacheSeqID: m.spcCacheSeqID,
		spcSession: newSession,
	}
}

// evictLRUSPCSession removes the least recently used SPC session from the map.
// Must be called with cacheMu held.
func (m *Model) evictLRUSPCSession(ctx context.Context) {
	var oldestHash string
	var oldestTime time.Time

	for hash, session := range m.spcSessions {
		if oldestHash == "" || session.lastUsed.Before(oldestTime) {
			oldestHash = hash
			oldestTime = session.lastUsed
		}
	}

	if oldestHash != "" {
		evicted := m.spcSessions[oldestHash]
		delete(m.spcSessions, oldestHash)
		m.log(ctx, "spc", "status", "session evicted (LRU)", "hash", oldestHash[:8], "tokens", evicted.sysPromptTokens)
	}
}

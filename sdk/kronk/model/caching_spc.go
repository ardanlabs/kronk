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
		m.cacheMu.RLock()
		session := m.spcSession
		m.cacheMu.RUnlock()

		if session != nil && session.sysPromptTokens > 0 {
			m.log(ctx, "spc", "status", "cache hit (system prompt excluded on this request)", "tokens", session.sysPromptTokens)
			return cacheResult{
				modifiedD:  d,
				cacheIdx:   llama.Pos(session.sysPromptTokens),
				cacheSeqID: m.spcCacheSeqID,
			}
		}
	}

	return cacheResult{modifiedD: d}
}

// performSPC checks for a cache hit on the system prompt. On a miss, it
// templates, tokenizes, and decodes the system prompt into the dedicated
// cache sequence. On a hit, it returns the cached position and sequence ID
// so that startSlot can copy the KV state into the slot's working sequence.
func (m *Model) performSPC(ctx context.Context, d D, messages []D, msgInfo cacheableMessage) cacheResult {
	if msgInfo.role != RoleSystem {
		m.log(ctx, "spc", "status", "no system prompt message provided", "role", msgInfo.role)
		return cacheResult{modifiedD: d}
	}

	contentLen := len(msgInfo.content)

	// Check for cache hit (fast path with read lock).
	m.cacheMu.RLock()
	session := m.spcSession
	m.cacheMu.RUnlock()

	newHash := hashMessage(msgInfo)

	if session != nil && session.sysPromptLen == contentLen && session.sysPromptHash == newHash && session.sysPromptTokens > 0 {
		m.log(ctx, "spc", "status", "cache hit", "tokens", session.sysPromptTokens)
		return cacheResult{
			cacheIdx:   llama.Pos(session.sysPromptTokens),
			cacheSeqID: m.spcCacheSeqID,
		}
	}

	// Cache miss — template and cache the message.
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	// Double-check in case another goroutine cached while we waited.
	session = m.spcSession
	if session != nil && session.sysPromptLen == contentLen && session.sysPromptHash == newHash && session.sysPromptTokens > 0 {
		m.log(ctx, "spc", "status", "cache hit (after lock)", "tokens", session.sysPromptTokens)
		return cacheResult{
			cacheIdx:   llama.Pos(session.sysPromptTokens),
			cacheSeqID: m.spcCacheSeqID,
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

	// Invalidate session before clearing KV so a failed rebuild can't serve
	// stale metadata pointing at an empty/partial sequence.
	m.spcSession = nil

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

	m.spcSession = &spcSession{
		sysPromptHash:   newHash,
		sysPromptTokens: nTokens,
		sysPromptLen:    contentLen,
		seqID:           m.spcCacheSeqID,
		lastUsed:        time.Now(),
		kvState:         kvState[:nExtracted],
	}

	m.log(ctx, "spc", "tokens", nTokens, "hash", newHash[:8], "kv_bytes", nExtracted, "status", "tokens saved (externalized)")

	return cacheResult{
		modifiedD:  d,
		cacheIdx:   llama.Pos(nTokens),
		cacheSeqID: m.spcCacheSeqID,
	}
}

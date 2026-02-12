package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// processSPC orchestrates the system prompt caching flow. It examines
// the first message and either caches it (if it's a system message) or reuses
// an existing cache (if the client omitted the system message on a follow-up
// request). The system message is always removed from d after processing.
//
// Tokens are saved to RAM per cache_id. The actual KV decode happens directly
// into the slot's sequence via decodeSPCToSlot at startSlot time.
func (m *Model) processSPC(ctx context.Context, d D) cacheResult {
	messages, ok := d["messages"].([]D)
	if !ok || len(messages) == 0 {
		return cacheResult{modifiedD: d}
	}

	cacheID, _ := d["cache_id"].(string)
	if cacheID == "" {
		cacheID = "default"
		m.log(ctx, "spc", "status", "using default cache id", "reason", "SPC turned on but no cache_id was provided")
	}

	entry := m.getOrCreateSPCEntry(cacheID)

	role, ok := messages[0]["role"].(string)
	if !ok {
		return cacheResult{modifiedD: d}
	}

	content := extractMessageContent(messages[0])
	if content == "" {
		return cacheResult{modifiedD: d}
	}

	sysMsg := cacheableMessage{
		index:   0,
		role:    role,
		content: content,
	}

	switch role {
	case RoleSystem:
		result := m.performSPC(ctx, d, messages, sysMsg, cacheID, entry)
		if result.err != nil {
			return result
		}

		if len(result.spcTokens) > 0 {
			d = removeMessagesAtIndices(d, []int{0})
			return cacheResult{
				modifiedD:  d,
				cacheIdx:   llama.Pos(len(result.spcTokens)),
				spcCacheID: cacheID,
				spcHash:    result.spcHash,
				spcTokens:  result.spcTokens,
			}
		}

		return cacheResult{modifiedD: d}

	default:
		m.cacheMu.RLock()
		savedTokens := entry.tokens
		savedHash := entry.hash
		m.cacheMu.RUnlock()

		if len(savedTokens) > 0 {
			m.log(ctx, "spc", "status", "cache hit (system prompt excluded on this request)", "cache_id", cacheID, "tokens", len(savedTokens))
			return cacheResult{
				modifiedD:  d,
				cacheIdx:   llama.Pos(len(savedTokens)),
				spcCacheID: cacheID,
				spcHash:    savedHash,
				spcTokens:  savedTokens,
			}
		}
	}

	return cacheResult{modifiedD: d}
}

// performSPC checks for a cache hit on the system prompt. On a miss, it
// templates and tokenizes the message, saving the tokens to RAM in the
// spcEntry. No KV decode happens here — that is deferred to decodeSPCToSlot
// which runs in the batch engine goroutine at startSlot time.
func (m *Model) performSPC(ctx context.Context, d D, messages []D, msgInfo cacheableMessage, cacheID string, entry *spcEntry) cacheResult {
	if msgInfo.role != RoleSystem {
		m.log(ctx, "spc", "status", "no system prompt message provided", "role", msgInfo.role)
		return cacheResult{modifiedD: d}
	}

	contentLen := len(msgInfo.content)

	m.cacheMu.RLock()
	currentHash := entry.hash
	currentTokens := entry.tokens
	currentLen := entry.len
	m.cacheMu.RUnlock()

	newHash := hashMessage(msgInfo)

	if currentLen == contentLen && currentHash == newHash && len(currentTokens) > 0 {
		m.log(ctx, "spc", "status", "cache hit", "cache_id", cacheID, "tokens", len(currentTokens))
		return cacheResult{
			spcHash:   newHash,
			spcTokens: currentTokens,
		}
	}

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	if entry.len == contentLen && entry.hash == newHash && len(entry.tokens) > 0 {
		m.log(ctx, "spc", "status", "cache hit (after lock)", "cache_id", cacheID, "tokens", len(entry.tokens))
		return cacheResult{
			spcHash:   newHash,
			spcTokens: entry.tokens,
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
		m.log(ctx, "spc", "status", "cache skip (too short)", "cache_id", cacheID, "tokens", nTokens, "min", m.cfg.CacheMinTokens)
		return cacheResult{modifiedD: d}
	}

	entry.hash = newHash
	entry.tokens = tokens
	entry.len = contentLen

	m.log(ctx, "spc", "status", "tokens saved", "cache_id", cacheID, "tokens", nTokens, "hash", newHash[:8])

	return cacheResult{
		modifiedD: d,
		spcHash:   newHash,
		spcTokens: tokens,
	}
}

// getOrCreateSPCEntry looks up an existing entry by cacheID or creates a new one.
func (m *Model) getOrCreateSPCEntry(cacheID string) *spcEntry {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	if entry, exists := m.spcEntries[cacheID]; exists {
		return entry
	}

	entry := &spcEntry{}
	m.spcEntries[cacheID] = entry
	return entry
}

// decodeSPCToSlot decodes the saved system prompt tokens directly into the
// specified slot sequence. This avoids needing a dedicated cache sequence —
// tokens are stored in RAM and re-decoded into each slot as needed.
func (m *Model) decodeSPCToSlot(ctx context.Context, tokens []llama.Token, seqID llama.SeqId) (llama.Pos, error) {
	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch
	}

	m.log(ctx, "spc", "status", "decoding to slot", "seq", seqID, "tokens", nTokens)

	batchSize := int32(min(nBatch, nTokens))
	if batchSize <= 0 {
		batchSize = 1
	}
	batch := llama.BatchInit(batchSize, 0, 1)
	defer llama.BatchFree(batch)

	seqIDs := []llama.SeqId{seqID}

	for i := 0; i < nTokens; i += nBatch {
		batch.Clear()

		end := min(i+nBatch, nTokens)
		for j := i; j < end; j++ {
			batch.Add(tokens[j], llama.Pos(j), seqIDs, false)
		}

		if _, err := llama.Decode(m.lctx, batch); err != nil {
			return 0, fmt.Errorf("spc: failed to decode tokens at pos %d: %w", i, err)
		}
	}

	return llama.Pos(nTokens), nil
}

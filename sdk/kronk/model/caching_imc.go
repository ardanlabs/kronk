package model

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// IMCSessionState describes the current state of an allocated IMC cache entry.
type IMCSessionState string

const (
	// IMCSessionStateActive means a request currently holds the entry's
	// reservation.
	IMCSessionStateActive IMCSessionState = "active"

	// IMCSessionStateIdle means the entry contains a reusable cache snapshot.
	IMCSessionStateIdle IMCSessionState = "idle"

	// IMCSessionStateEmpty means the entry does not contain a cache snapshot.
	IMCSessionStateEmpty IMCSessionState = "empty"
)

// IMCSessionDetail is a scalar snapshot of one allocated IMC cache entry.
type IMCSessionDetail struct {
	ID            int
	State         IMCSessionState
	Context       int
	Allocated     int
	Messages      int
	ContextWindow int
	LastUsed      time.Time
	HasMedia      bool
}

// IMCSessions returns the current state of the model's allocated IMC cache
// entries. It does not retain history or expose the cached content.
func (m *Model) IMCSessions() []IMCSessionDetail {
	m.cacheMu.RLock()
	defer m.cacheMu.RUnlock()

	details := make([]IMCSessionDetail, 0, len(m.imcSessions))
	for _, session := range m.imcSessions {
		if session == nil {
			continue
		}

		state := IMCSessionStateEmpty
		switch {
		case session.reserved:
			state = IMCSessionStateActive
		case session.totalTokensCached > 0:
			state = IMCSessionStateIdle
		}

		details = append(details, IMCSessionDetail{
			ID:            session.id,
			State:         state,
			Context:       session.totalTokensCached,
			Allocated:     session.allocatedContext,
			Messages:      session.cachedMsgCount,
			ContextWindow: m.cfg.ContextWindow(),
			LastUsed:      session.lastUsed,
			HasMedia:      session.hasMedia,
		})
	}

	return details
}

// decodeTokensIntoCache decodes tokens into a cache sequence starting at startPos.
// Unlike addTokensToCache, this does NOT clear the sequence first — the caller
// is responsible for clearing if needed (e.g., rebuild from scratch).
func (m *Model) decodeTokensIntoCache(ctx context.Context, tokens []llama.Token, seqID llama.SeqId, startPos int) error {
	ctx, decodeSpan := otel.AddSpan(ctx, "cache-decode",
		attribute.Int("tokens", len(tokens)),
	)
	defer decodeSpan.End()

	nBatch := int(m.ctxParams.NBatch)
	nTokens := len(tokens)

	if nBatch <= 0 {
		nBatch = m.cfg.NBatch()
	}

	m.log(ctx, "cache", "status", "decoding tokens into cache", "seq", seqID, "tokens", nTokens, "start_pos", startPos, "nbatch", nBatch)

	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()

	// Create batch with explicit sequence ID.
	// Allocate batch sized to nBatch (not nCtx) to avoid huge allocations for
	// large context windows that can cause C-side allocation failures.
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
			pos := llama.Pos(startPos + j)
			batch.Add(tokens[j], pos, seqIDs, false)
		}

		ret, err := llama.Decode(m.lctx, batch)
		if err != nil || ret != 0 {
			return fmt.Errorf("imc: failed to decode extension tokens at pos %d: %w", startPos+i, decodeError(ret, err))
		}
		llama.Synchronize(m.lctx)
	}

	m.log(ctx, "cache", "status", "finished (decoding tokens into cache)", "seq", seqID, "tokens", nTokens, "nbatch", nBatch)

	return nil
}

// imcResetSession clears all metadata on an IMC session, returning it to
// an empty state. The session's pool index (id) is preserved; seqID is
// reset to imcSeqIDUnbound because a reset session is no longer bound
// to any execution slot's KV sequence. The caller must hold m.cacheMu
// (write lock).
func imcResetSession(s *imcSession) {
	s.seqID = imcSeqIDUnbound
	s.cachedMsgsHash = ""
	s.cachedTokens = nil
	s.totalTokensCached = 0
	// allocatedContext is intentionally preserved because Reset retains the
	// zeroed SessionStore backing allocation represented by this high-water mark.
	s.nextLogicalPos = 0
	s.cachedMsgCount = 0
	// Reset zeroes the retained backing byte array before setting Len to 0.
	// The next conversation can reuse the allocation without observing bytes
	// from the session that previously owned this cache entry.
	s.kvState.Reset()
	if s.draftKVState != nil {
		s.draftKVState.Reset()
	}
	clear(s.pendingH[:cap(s.pendingH)])
	s.pendingH = s.pendingH[:0]
	s.lastUsed = time.Time{}
	s.reserved = false
	s.hasMedia = false
	s.useMRoPE = false
	s.mediaKVCounts = nil
	s.promptPlan = promptPlan{}
	s.samplerPromptTokens = nil
	s.cachedRenderInputHash = ""
}

// imcReleaseReservation clears a session's reserved flag.
// Safe to call even if the session wasn't reserved. sessionID is the
// session-pool index (imcSession.id), not an execution slot id; the
// negative-index guard catches stray callers that pass a slot id by
// mistake on jobs that never reserved an IMC session.
func (m *Model) imcReleaseReservation(sessionID int) {
	m.cacheMu.Lock()
	if sessionID >= 0 && sessionID < len(m.imcSessions) {
		m.imcSessions[sessionID].reserved = false
	}
	m.cacheMu.Unlock()
}

// imcInvalidateReservedSession removes a corrupt snapshot while preserving
// this request's reservation. finishSlot unbinds the session and performs the
// single release, so no subsequent request can bind it during error cleanup.
func (m *Model) imcInvalidateReservedSession(session *imcSession) {
	if session == nil {
		return
	}

	m.cacheMu.Lock()
	imcResetSession(session)
	session.reserved = true
	m.cacheMu.Unlock()
}

// imcCommitSession updates a session's metadata after a successful cache
// build/extend/rebuild. The reserved flag is intentionally LEFT SET so that
// concurrent token-v2 planners continue to skip this session until the
// caller has also externalized the KV state (via StateSeqGetData) into
// session.kvState. Use imcPublishSession to clear reserved AFTER the snapshot
// has been committed; that pairing closes
// the publication race in which a reader would otherwise see fresh
// metadata (totalTokensCached etc.) but stale/empty kvState bytes and
// restore garbage into a new slot.
//
// When hasMedia is true, cachedTokens is cleared since token-level
// operations (prefix matching, speculative decoding) are not valid for
// media-cached sessions. mediaKVCounts records the KV positions consumed
// per media chunk for text-only extend math.
//
// The session parameter is the matched session (job.imcSession), not a
// slot-indexed lookup. With externalized KV, any slot can serve any session.
//
// renderInputHash is the imcRenderFingerprint of the inputs that produced
// the just-cached prefix; pass "" when no fingerprint was computed (the
// session simply will not qualify for the pure-hit snapshot-skip path).
func (m *Model) imcCommitSession(session *imcSession, hash string, totalCached int, cachedMsgCount int, cachedTokens []llama.Token, hasMedia bool, mediaKVCounts []int, renderInputHash string) {
	if session == nil {
		return
	}

	m.cacheMu.Lock()
	session.cachedMsgsHash = hash
	session.totalTokensCached = totalCached
	session.cachedMsgCount = cachedMsgCount
	session.lastUsed = time.Now()
	session.hasMedia = hasMedia
	session.mediaKVCounts = mediaKVCounts
	session.cachedRenderInputHash = renderInputHash
	session.samplerPromptTokens = nil
	if !hasMedia {
		session.useMRoPE = false
		session.nextLogicalPos = 0
	}
	switch {
	case hasMedia:
		session.cachedTokens = nil
		// Media is decoded only into the target context. An own-KV MTP
		// snapshot cannot prove that it covers the projected media cells, so
		// carrying it across a media commit would pair target and draft states
		// from different prefixes. Shared-target-KV MTP does not use these
		// fields and continues to resume from the target snapshot.
		if session.draftKVState != nil {
			session.draftKVState.Reset()
		}
		clear(session.pendingH[:cap(session.pendingH)])
		session.pendingH = session.pendingH[:0]
	case len(cachedTokens) > 0:
		session.cachedTokens = cachedTokens
	}
	m.cacheMu.Unlock()
}

// imcCommitMediaAdvance atomically swaps a fully staged target snapshot and
// its matching media-prefix metadata. The caller owns the returned old store
// and closes it after the swap. reserved remains set until the caller publishes
// or releases the reservation.
func (m *Model) imcCommitMediaAdvance(session *imcSession, staged SessionStore, hash string, totalCached, cachedMsgCount, nextLogicalPos int, plan promptPlan, samplerPromptTokens []llama.Token, renderInputHash string) SessionStore {
	if session == nil || staged == nil {
		return nil
	}

	m.cacheMu.Lock()
	oldStore := session.kvState
	session.kvState = staged
	session.cachedMsgsHash = hash
	session.totalTokensCached = totalCached
	session.nextLogicalPos = nextLogicalPos
	session.cachedMsgCount = cachedMsgCount
	session.promptPlan = plan
	session.samplerPromptTokens = samplerPromptTokens
	session.cachedRenderInputHash = renderInputHash
	session.lastUsed = time.Now()
	session.cachedTokens = nil
	session.allocatedContext = max(session.allocatedContext, totalCached)
	if session.draftKVState != nil {
		session.draftKVState.Reset()
	}
	clear(session.pendingH[:cap(session.pendingH)])
	session.pendingH = session.pendingH[:0]
	m.cacheMu.Unlock()

	return oldStore
}

// imcPublishSession completes the publication started by imcCommitSession.
// It clears the reserved flag, making the session visible to token-v2 planners.
// Call this only AFTER the externalized SessionStore snapshot has been
// committed for the same session, so the metadata and kvState bytes are
// guaranteed consistent to concurrent readers.
func (m *Model) imcPublishSession(session *imcSession) {
	if session == nil {
		return
	}

	m.cacheMu.Lock()
	session.allocatedContext = max(session.allocatedContext, session.totalTokensCached)
	session.reserved = false
	m.cacheMu.Unlock()
}

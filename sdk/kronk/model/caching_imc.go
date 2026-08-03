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

		context := session.totalTokensCached
		messages := session.cachedMsgCount
		hasMedia := session.hasMedia
		checkpointAllocated := 0
		if checkpoint := session.turnCheckpoint; checkpoint != nil {
			checkpointAllocated = checkpoint.allocatedContext
			if context == 0 && checkpoint.totalTokensCached > 0 {
				context = checkpoint.totalTokensCached
				messages = checkpoint.cachedMsgCount
				hasMedia = checkpoint.hasMedia
			}
		}

		state := IMCSessionStateEmpty
		switch {
		case session.reserved:
			state = IMCSessionStateActive
		case context > 0:
			state = IMCSessionStateIdle
		}

		details = append(details, IMCSessionDetail{
			ID:            session.id,
			State:         state,
			Context:       context,
			Allocated:     session.allocatedContext + checkpointAllocated,
			Messages:      messages,
			ContextWindow: m.cfg.ContextWindow(),
			LastUsed:      session.lastUsed,
			HasMedia:      hasMedia,
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

func (s *imcSession) takeRollingSnapshot() imcSnapshot {
	snapshot := imcSnapshot{
		cachedMsgsHash:        s.cachedMsgsHash,
		cachedTokens:          s.cachedTokens,
		totalTokensCached:     s.totalTokensCached,
		nextLogicalPos:        s.nextLogicalPos,
		cachedMsgCount:        s.cachedMsgCount,
		kvState:               s.kvState,
		draftKVState:          s.draftKVState,
		pendingH:              s.pendingH,
		allocatedContext:      s.allocatedContext,
		hasMedia:              s.hasMedia,
		useMRoPE:              s.useMRoPE,
		mediaKVCounts:         s.mediaKVCounts,
		promptPlan:            s.promptPlan,
		samplerPromptTokens:   s.samplerPromptTokens,
		cachedRenderInputHash: s.cachedRenderInputHash,
		endsAtUser:            s.rollingEndsAtUser,
	}

	s.installRollingSnapshot(imcSnapshot{})

	return snapshot
}

func (s *imcSession) installRollingSnapshot(snapshot imcSnapshot) {
	s.cachedMsgsHash = snapshot.cachedMsgsHash
	s.cachedTokens = snapshot.cachedTokens
	s.totalTokensCached = snapshot.totalTokensCached
	s.nextLogicalPos = snapshot.nextLogicalPos
	s.cachedMsgCount = snapshot.cachedMsgCount
	s.kvState = snapshot.kvState
	s.draftKVState = snapshot.draftKVState
	s.pendingH = snapshot.pendingH
	s.allocatedContext = snapshot.allocatedContext
	s.hasMedia = snapshot.hasMedia
	s.useMRoPE = snapshot.useMRoPE
	s.mediaKVCounts = snapshot.mediaKVCounts
	s.promptPlan = snapshot.promptPlan
	s.samplerPromptTokens = snapshot.samplerPromptTokens
	s.cachedRenderInputHash = snapshot.cachedRenderInputHash
	s.rollingEndsAtUser = snapshot.endsAtUser
}

func (s *imcSession) swapTurnCheckpoint() {
	if s == nil || s.turnCheckpoint == nil {
		return
	}

	rolling := s.takeRollingSnapshot()
	s.installRollingSnapshot(*s.turnCheckpoint)
	s.turnCheckpoint = &rolling
}

func closeIMCSnapshot(snapshot *imcSnapshot) {
	if snapshot == nil {
		return
	}
	if snapshot.kvState != nil {
		_ = snapshot.kvState.Close()
	}
	if snapshot.draftKVState != nil {
		_ = snapshot.draftKVState.Close()
	}
}

func imcResetRollingSession(s *imcSession) {
	if s == nil {
		return
	}

	s.cachedMsgsHash = ""
	s.cachedTokens = nil
	s.totalTokensCached = 0
	// allocatedContext is intentionally preserved because Reset retains the
	// zeroed SessionStore backing allocation represented by this high-water mark.
	s.nextLogicalPos = 0
	s.cachedMsgCount = 0
	if s.kvState != nil {
		s.kvState.Reset()
	}
	if s.draftKVState != nil {
		s.draftKVState.Reset()
	}
	clear(s.pendingH[:cap(s.pendingH)])
	s.pendingH = s.pendingH[:0]
	s.hasMedia = false
	s.useMRoPE = false
	s.mediaKVCounts = nil
	s.promptPlan = promptPlan{}
	s.samplerPromptTokens = nil
	s.cachedRenderInputHash = ""
	s.rollingEndsAtUser = false
}

// imcResetSession clears all metadata on an IMC session, returning it to
// an empty state. The session's pool index (id) is preserved; seqID is
// reset to imcSeqIDUnbound because a reset session is no longer bound
// to any execution slot's KV sequence. The caller must hold m.cacheMu
// (write lock).
func imcResetSession(s *imcSession) {
	s.seqID = imcSeqIDUnbound
	imcResetRollingSession(s)
	closeIMCSnapshot(s.turnCheckpoint)
	s.turnCheckpoint = nil
	s.lastUsed = time.Time{}
	s.reserved = false
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

// imcInvalidateReservedSession removes corrupt rolling state while preserving
// both this request's reservation and any independent turn checkpoint.
func (m *Model) imcInvalidateReservedSession(session *imcSession) {
	if session == nil {
		return
	}

	m.cacheMu.Lock()
	imcResetRollingSession(session)
	session.seqID = imcSeqIDUnbound
	session.reserved = true
	m.cacheMu.Unlock()
}

// imcPromoteTurnCheckpoint moves the selected rolling user-boundary snapshot
// into the retained checkpoint and installs fresh rolling stores. The slot has
// already restored and extended the model state, so moving host-side ownership
// avoids a snapshot-sized byte copy. If the new rolling snapshot later fails,
// invalidation clears only rolling state and leaves this checkpoint reusable.
func (m *Model) imcPromoteTurnCheckpoint(ctx context.Context, session *imcSession) error {
	if session == nil {
		return nil
	}

	targetStore, err := newSessionStore(m.cfg)
	if err != nil {
		return fmt.Errorf("create rolling session store: %w", err)
	}

	var draftStore SessionStore
	if session.draftKVState != nil {
		draftStore, err = newSessionStore(m.cfg)
		if err != nil {
			_ = targetStore.Close()
			return fmt.Errorf("create rolling draft session store: %w", err)
		}
	}

	m.cacheMu.Lock()
	if !session.rollingEndsAtUser || session.hasMedia || session.totalTokensCached == 0 || session.kvState == nil || session.kvState.Len() == 0 {
		m.cacheMu.Unlock()
		_ = targetStore.Close()
		if draftStore != nil {
			_ = draftStore.Close()
		}
		return nil
	}

	oldCheckpoint := session.turnCheckpoint
	checkpoint := session.takeRollingSnapshot()
	session.installRollingSnapshot(imcSnapshot{
		kvState:      targetStore,
		draftKVState: draftStore,
	})
	session.turnCheckpoint = &checkpoint
	m.cacheMu.Unlock()

	closeIMCSnapshot(oldCheckpoint)
	m.log(ctx, "imc", "status", "turn-checkpoint-promoted", "imc_cache_entry", session.id,
		"messages", checkpoint.cachedMsgCount, "tokens", checkpoint.totalTokensCached,
		"snapshot_bytes", fmtBytes(uint64(checkpoint.kvState.Len())))

	return nil
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
func (m *Model) imcCommitSession(session *imcSession, hash string, totalCached int, cachedMsgCount int, cachedTokens []llama.Token, hasMedia bool, mediaKVCounts []int, renderInputHash string, endsAtUser bool) {
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
	session.rollingEndsAtUser = endsAtUser
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
func (m *Model) imcCommitMediaAdvance(session *imcSession, staged SessionStore, hash string, totalCached, cachedMsgCount, nextLogicalPos int, plan promptPlan, samplerPromptTokens []llama.Token, renderInputHash string, endsAtUser bool) SessionStore {
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
	session.rollingEndsAtUser = endsAtUser
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

package model

import (
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// imcPreparation is the slot-owned continuation for a text IMC build or
// extension. position advances only after both target and MTP draft state have
// decoded successfully.
type imcPreparation struct {
	cacheIdx              llama.Pos
	nextToken             int
	position              int
	sessionUpdateDisabled bool
}

func (e *batchEngine) hasIMCPreparation() bool {
	for _, s := range e.slots {
		if s.active && s.imcPrep != nil {
			return true
		}
	}

	return false
}

func (e *batchEngine) nextIMCPreparationSlot() (*slot, int) {
	for offset := range e.slots {
		idx := (e.imcPrepNext + offset) % len(e.slots)
		s := e.slots[idx]
		if s.active && s.imcPrep != nil {
			return s, idx
		}
	}

	return nil, -1
}

func (e *batchEngine) imcPreparationSlotIDs() []int {
	var ids []int
	for _, s := range e.slots {
		if s.active && s.imcPrep != nil {
			ids = append(ids, s.id)
		}
	}

	return ids
}

// advanceIMCPreparation advances at most one preparing slot per batch-loop
// iteration. A selected slot retains ownership until its reusable state is
// fully prepared, then the cursor advances to the next slot.
func (e *batchEngine) advanceIMCPreparation(buf []byte) {
	selectorStart := e.imcPrepNext
	preparationSlots := e.imcPreparationSlotIDs()
	s, idx := e.nextIMCPreparationSlot()
	e.diagnosticIMCStart = selectorStart
	e.diagnosticIMCSelected = idx
	if s == nil {
		return
	}

	if err := s.job.ctx.Err(); err != nil {
		e.imcPrepNext = imcPreparationNext(idx, len(e.slots), true)
		e.diagnosticIMCSelected = -1
		e.finishSlot(s, err)
		return
	}

	e.imcPrepNext = idx
	e.advanceIMCPreparationSlot(s, selectorStart, idx, preparationSlots, buf)
	e.diagnosticIMCSelected = -1
}

func (e *batchEngine) advanceIMCPreparationSlot(s *slot, selectorStart, selectedIndex int, preparationSlots []int, buf []byte) {
	prep := s.imcPrep
	job := s.job
	chunkSize := e.imcPreparationChunkSize()
	checkpoint := job.imcCheckpointTokens
	end := imcPreparationChunkEnd(prep.nextToken, prep.position, len(job.imcNewCacheTokens),
		chunkSize, checkpoint, job.imcNewTotalCached)

	chunk := job.imcNewCacheTokens[prep.nextToken:end]
	e.publishDiagnostics(true)
	started := time.Now()
	var err error
	if e.model.draft != nil && e.model.draft.mtp() && !s.mtp.Disabled {
		err = e.decodeTokensIntoCacheMTP(job.ctx, s, chunk, prep.position)
	} else {
		err = e.model.decodeTokensIntoCache(job.ctx, chunk, s.seqID, prep.position)
	}
	if err != nil {
		e.imcPrepNext = imcPreparationNext(selectedIndex, len(e.slots), true)
		e.clearFailedIMCPreparation(s)
		e.finishSlot(s, fmt.Errorf("start-slot: imc decode: %w", err))
		return
	}

	elapsed := time.Since(started)
	metrics.AddPrefillTime(e.model.modelInfo.ID, "imc-decode", elapsed)
	prep.nextToken = end
	prep.position += len(chunk)
	prefillComplete := prep.nextToken >= len(job.imcNewCacheTokens)
	e.imcPrepNext = imcPreparationNext(selectedIndex, len(e.slots), prefillComplete)

	e.model.log(job.ctx, "start-slot", "status", "imc-preparation-chunk",
		"iteration", e.batchIteration, "slot", s.id, "seq", s.seqID,
		"preparation_slots", fmt.Sprintf("%v", preparationSlots),
		"selector_start", selectorStart, "selector_selected", selectedIndex, "selector_next", e.imcPrepNext,
		"chunk_tokens", len(chunk),
		"prepared_tokens", prep.nextToken, "total_tokens", len(job.imcNewCacheTokens),
		"next_position", prep.position, "remaining_tokens", len(job.imcNewCacheTokens)-prep.nextToken,
		"elapsed", fmtDur(elapsed))

	if checkpoint > int(prep.cacheIdx) && checkpoint < job.imcNewTotalCached && prep.position == checkpoint {
		checkpointPublished := e.snapshotProgressiveCheckpoint(job.ctx, s, job, checkpoint)
		if !checkpointPublished && job.reusedPromptTokens > 0 {
			preserved, preserveErr := e.model.imcPreserveCurrentSnapshot(job.ctx, job.imcSession, false)
			if preserveErr != nil || !preserved {
				prep.sessionUpdateDisabled = true
				e.model.log(job.ctx, "start-slot", "status", "imc-progressive-update-disabled",
					"session_id", job.imcSessionID, "reused_tokens", job.reusedPromptTokens,
					"checkpoint_tokens", checkpoint, "err", preserveErr)
			}
		}
	}

	if prep.nextToken < len(job.imcNewCacheTokens) {
		return
	}

	job.imcPhysicalCached = job.imcNewTotalCached
	if job.imcPromoteCheckpoint && !prep.sessionUpdateDisabled {
		if err := e.model.imcPromoteTurnCheckpoint(job.ctx, job.imcSession); err != nil {
			prep.sessionUpdateDisabled = true
			e.model.log(job.ctx, "start-slot", "status", "imc-reusable-preserve-failed",
				"session_id", job.imcSessionID, "err", err)
		}
	}

	sessionWasCommitted := false
	if !prep.sessionUpdateDisabled {
		hasMedia := len(job.imcMediaKVCounts) > 0
		e.model.imcCommitSession(job.imcSession, job.imcNewMsgsHash, job.imcNewTotalCached,
			job.imcNewCachedMsgCount, job.imcNewCachedTokens, hasMedia, job.imcMediaKVCounts,
			job.imcExpectedRenderHash, job.imcNewEndsAtUser)
		e.model.cacheMu.Lock()
		job.imcSession.promptPlan = job.imcPromptPlan
		e.model.cacheMu.Unlock()
		sessionWasCommitted = true
	}

	e.model.log(job.ctx, "start-slot", "status", "imc-cache-ready", "slot", s.id, "seq", s.seqID,
		"total_cached", job.imcNewTotalCached, "resumable", true, "session_committed", sessionWasCommitted)

	sessionUpdateDisabled := prep.sessionUpdateDisabled
	s.imcPrep = nil
	e.finishStartSlot(s, job, llama.Pos(job.imcNewTotalCached), sessionUpdateDisabled, sessionWasCommitted, buf)
}

func imcPreparationNext(selected, slots int, complete bool) int {
	if complete {
		return (selected + 1) % slots
	}
	return selected
}

func (e *batchEngine) clearFailedIMCPreparation(s *slot) {
	e.model.decodeMu.Lock()
	llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
	if e.model.draft != nil && e.model.draft.mtp() {
		llama.MemorySeqRm(e.model.draft.core().mem, s.seqID, -1, -1)
		s.draftNPast = 0
		s.mtp.PendingHidden = s.mtp.PendingHidden[:0]
	}
	e.model.decodeMu.Unlock()
}

func (e *batchEngine) imcPreparationChunkSize() int {
	return e.model.cfg.PrefillBatchSize()
}

func imcPreparationChunkEnd(nextToken, position, totalTokens, chunkSize, checkpoint, finalPosition int) int {
	end := min(nextToken+chunkSize, totalTokens)

	// Do not cross the progressive checkpoint boundary in a chunk. The
	// checkpoint must describe exactly that target/draft position.
	if checkpoint > position && checkpoint < finalPosition {
		end = min(end, nextToken+checkpoint-position)
	}

	return end
}

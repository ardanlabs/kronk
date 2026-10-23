package model

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// imcPreparation is the slot-owned continuation for an IMC build or
// extension. position advances only after both target and MTP draft state have
// decoded successfully.
type imcPreparation struct {
	cacheIdx              llama.Pos
	baseTokens            int
	nextToken             int
	position              int
	sessionUpdateDisabled bool
	media                 *imcMediaPreparation
}

// queueIMCMediaPreparation leaves all target prefill to subsequent preparation
// turns. Text-only media-anchor advances use the existing text continuation.
func (e *batchEngine) queueIMCMediaPreparation(s *slot, cacheIdx llama.Pos, sessionUpdateDisabled bool) error {
	job := s.job
	prep := imcPreparation{cacheIdx: cacheIdx, position: int(cacheIdx), sessionUpdateDisabled: sessionUpdateDisabled}
	s.imcPrep = &prep
	if !job.imcMediaBuild && !job.imcMediaAppend {
		job.imcMediaNativeChunks = job.imcSession.mediaNativeChunks
		return nil
	}

	var prefix []imcMediaChunk
	if job.imcSystemCache != nil {
		prefix = []imcMediaChunk{{kind: imcMediaChunkText, tokens: slices.Clone(job.imcSystemCache.cachedTokens)}}
		prep.baseTokens = int(cacheIdx)
	} else if !job.imcMediaBuild {
		prep.baseTokens = job.imcExpectedTokens
		prefix = job.imcSession.mediaNativeChunks
		if !job.imcSession.hasMedia {
			prefix = []imcMediaChunk{{kind: imcMediaChunkText, tokens: slices.Clone(job.imcSession.cachedTokens)}}
		}
	}
	if job.imcSystemCache == nil && (job.imcMediaBuild || (!job.imcSession.hasMedia && mtmd.DecodeUseNonCausal(s.mtmdCtx, 0))) {
		if err := e.resetIMCMediaPreparation(s); err != nil {
			return err
		}
		prefix = nil
		prep.cacheIdx = 0
		prep.baseTokens = 0
		prep.position = 0
	}
	media := s.mediaSetup.media
	s.mediaSetup.media = nil
	media.prefixCursor = imcMediaPrefixCursor{chunks: prefix}
	media.position = prep.position
	media.systemBoundaryTokens = job.imcSystemBoundaryTokens
	prep.media = media
	s.mediaPrefilling = true
	e.model.log(job.ctx, "start-slot", "status", "imc-media-preparation-queued",
		"iteration", e.batchIteration, "slot", s.id, "seq", s.seqID,
		"build", job.imcMediaBuild, "append", job.imcMediaAppend,
		"total_chunks", mtmd.InputChunksSize(media.inputChunks), "total_tokens", media.totalPhysicalTokens,
		"start_logical_position", prep.position,
		"prefill_batch_size", e.imcPreparationChunkSize(),
		"nbatch", e.model.cfg.EffectiveNBatch(), "nubatch", e.model.cfg.EffectiveNUBatch())
	return nil
}

func (e *batchEngine) resetIMCMediaPreparation(s *slot) error {
	e.model.decodeMu.Lock()
	defer e.model.decodeMu.Unlock()
	if _, err := llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1); err != nil {
		return fmt.Errorf("clear IMC media sequence: %w", err)
	}
	return e.resetMTPMediaPrefix(s)
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
	if prep.media != nil {
		e.advanceIMCMediaPreparation(s, selectorStart, selectedIndex, preparationSlots, buf)
		return
	}
	chunkSize := e.imcPreparationChunkSize()
	boundary := job.imcSystemBoundaryTokens
	end := imcPreparationChunkEnd(prep.nextToken, prep.position, len(job.imcNewCacheTokens),
		chunkSize, boundary, job.imcNewTotalCached)

	chunk := job.imcNewCacheTokens[prep.nextToken:end]
	e.publishDiagnostics(true)
	started := time.Now()
	var err error
	if job.imcMediaAnchorAdvance && job.imcSessionUseMRoPE {
		_, err = e.model.decodeTextMRoPEIntoCache(chunk, s.seqID, prep.position, func(batch *extendedBatch, nextPosition llama.Pos) error {
			return e.syncMTPDecodedRows(s, batch, 0, nextPosition)
		})
	} else if e.model.draft != nil && e.model.draft.mtp() && !s.mtp.Disabled {
		err = e.decodeTokensIntoCacheMTP(job.ctx, s, chunk, prep.position)
	} else {
		err = e.model.decodeTokensIntoCache(job.ctx, chunk, s.seqID, prep.position)
	}
	if err != nil {
		e.imcPrepNext = imcPreparationNext(selectedIndex, len(e.slots), true)
		err = errors.Join(err, e.clearFailedIMCPreparation(s))
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

	if boundary > int(prep.cacheIdx) && boundary < job.imcNewTotalCached && prep.position == boundary {
		e.snapshotSystemCache(job.ctx, s, job, boundary)
	}

	if prep.nextToken < len(job.imcNewCacheTokens) {
		return
	}

	job.imcPhysicalCached = job.imcNewTotalCached

	sessionWasCommitted := false
	if job.imcMediaAnchorAdvance {
		job.imcNewLogicalPosition = prep.position
	} else if !prep.sessionUpdateDisabled {
		hasMedia := len(job.imcMediaKVCounts) > 0
		e.model.imcCommitSession(job.ctx, job.imcSession, job.imcNewMsgsHash, job.imcNewTotalCached,
			job.imcNewCachedMsgCount, job.imcNewCachedTokens, hasMedia, job.imcMediaKVCounts,
			job.imcExpectedRenderHash)
		e.model.cacheMu.Lock()
		job.imcSession.promptPlan = job.imcPromptPlan
		e.model.cacheMu.Unlock()
		sessionWasCommitted = true
	}

	e.model.log(job.ctx, "start-slot", "status", "imc-cache-ready", "slot", s.id, "seq", s.seqID,
		"total_cached", job.imcNewTotalCached, "resumable", true, "session_committed", sessionWasCommitted)

	sessionUpdateDisabled := prep.sessionUpdateDisabled
	position := prep.position
	s.imcPrep = nil
	e.finishStartSlot(s, job, llama.Pos(position), sessionUpdateDisabled, sessionWasCommitted, buf)
}

func (e *batchEngine) advanceIMCMediaPreparation(s *slot, selectorStart, selectedIndex int, preparationSlots []int, buf []byte) {
	prep := s.imcPrep
	p := prep.media
	job := s.job
	chunkIdx := p.chunkIdx
	chunkType := "complete"
	if chunkIdx < mtmd.InputChunksSize(p.inputChunks) {
		switch mtmd.InputChunkGetType(mtmd.InputChunksGet(p.inputChunks, chunkIdx)) {
		case mtmd.InputChunkTypeText:
			chunkType = "text"
		case mtmd.InputChunkTypeImage:
			chunkType = "image"
		case mtmd.InputChunkTypeAudio:
			chunkType = "audio"
		default:
			chunkType = "unknown"
		}
	}
	baseTokens := prep.baseTokens
	e.model.log(job.ctx, "start-slot", "status", "imc-media-preparation-start",
		"iteration", e.batchIteration, "slot", s.id, "seq", s.seqID,
		"chunk", chunkIdx, "chunk_type", chunkType,
		"prepared_tokens", baseTokens+p.physicalKVCells, "total_tokens", p.totalPhysicalTokens,
		"next_logical_position", p.position)
	e.publishDiagnostics(true)
	started := time.Now()
	complete, err := e.model.advanceIMCMediaChunk(p, s.seqID, s.mtmdCtx, func(batch *extendedBatch, nextPosition llama.Pos) error {
		return e.syncMTPDecodedRows(s, batch, 0, nextPosition)
	})
	elapsed := time.Since(started)
	metrics.AddPrefillTime(e.model.modelInfo.ID, "imc-decode", elapsed)
	if errors.Is(err, errIMCMediaNativePrefix) {
		// Reuse the tokenized native stream but replay from an empty target
		// and draft sequence. The old externalized anchor stays authoritative.
		e.model.log(job.ctx, "start-slot", "status", "imc-media-native-prefix-rebuild",
			"iteration", e.batchIteration, "slot", s.id, "seq", s.seqID,
			"chunk", chunkIdx, "elapsed", fmtDur(elapsed), "err", err)
		if err = e.resetIMCMediaPreparation(s); err == nil {
			*p = imcMediaPreparation{ctx: p.ctx, span: p.span, bitmaps: p.bitmaps,
				inputChunks: p.inputChunks, totalPhysicalTokens: p.totalPhysicalTokens}
			prep.cacheIdx = 0
			prep.baseTokens = 0
			prep.position = 0
			s.nPast = 0
			return
		}
	}
	if err != nil {
		e.model.log(job.ctx, "start-slot", "status", "imc-media-preparation-failed",
			"iteration", e.batchIteration, "slot", s.id, "seq", s.seqID,
			"chunk", chunkIdx, "chunk_type", chunkType, "elapsed", fmtDur(elapsed), "err", err)
		e.imcPrepNext = imcPreparationNext(selectedIndex, len(e.slots), true)
		err = errors.Join(err, e.clearFailedIMCPreparation(s))
		e.finishSlot(s, fmt.Errorf("prepare IMC media: %w", err))
		return
	}
	prep.position = p.position
	s.nPast = llama.Pos(p.position)
	if boundary := p.systemBoundaryTokens; boundary > 0 && p.position == boundary {
		// Validate the native stream as well as the logical plan before
		// publishing text-only state from the multimodal pipeline.
		if p.physicalKVCells == boundary && len(p.mediaKVCounts) == 0 && tokensHavePrefix(p.samplerPromptTokens, job.imcNewCachedTokens[:boundary]) {
			e.snapshotSystemCache(job.ctx, s, job, boundary)
		}
		p.systemBoundaryTokens = 0
	}
	e.model.log(job.ctx, "start-slot", "status", "imc-media-preparation-chunk",
		"iteration", e.batchIteration, "slot", s.id, "seq", s.seqID,
		"preparation_slots", fmt.Sprintf("%v", preparationSlots),
		"selector_start", selectorStart, "selector_selected", selectedIndex,
		"selector_next", imcPreparationNext(selectedIndex, len(e.slots), complete),
		"chunk", chunkIdx, "chunk_type", chunkType, "next_chunk", p.chunkIdx,
		"physical_kv_cells", p.physicalKVCells,
		"prepared_tokens", baseTokens+p.physicalKVCells, "total_tokens", p.totalPhysicalTokens,
		"remaining_tokens", max(p.totalPhysicalTokens-baseTokens-p.physicalKVCells, 0),
		"next_logical_position", p.position, "complete", complete, "elapsed", fmtDur(elapsed))
	if !complete {
		return
	}

	job.imcSessionUseMRoPE = mtmd.DecodeUseMRope(s.mtmdCtx)
	job.imcMediaSamplerTokens = p.samplerPromptTokens
	job.samplerPromptTokens = append(slices.Clone(p.samplerPromptTokens), job.tailTokens...)
	job.imcMediaNativeChunks = p.nativeChunks
	job.imcNewLogicalPosition = p.position
	job.imcNewTotalCached = baseTokens + p.physicalKVCells
	if job.imcMediaAnchorAdvance && prep.cacheIdx > 0 {
		job.imcMediaKVCounts = append(job.imcMediaKVCounts, p.mediaKVCounts...)
	} else {
		job.imcMediaKVCounts = p.mediaKVCounts
	}
	job.imcPhysicalCached = job.imcNewTotalCached
	sessionWasCommitted := false
	if job.imcMediaBuild {
		e.model.imcCommitSession(job.ctx, job.imcSession, job.imcNewMsgsHash, job.imcNewTotalCached,
			job.imcNewCachedMsgCount, nil, true, job.imcMediaKVCounts, job.imcExpectedRenderHash)
		e.model.cacheMu.Lock()
		job.imcSession.promptPlan = job.imcPromptPlan
		job.imcSession.samplerPromptTokens = slices.Clone(p.samplerPromptTokens)
		job.imcSession.mediaNativeChunks = p.nativeChunks
		job.imcSession.nextLogicalPos = p.position
		job.imcSession.useMRoPE = job.imcSessionUseMRoPE
		job.imcSession.useNonCausal = mtmd.DecodeUseNonCausal(s.mtmdCtx, 0)
		e.model.cacheMu.Unlock()
		sessionWasCommitted = true
	}
	e.model.log(job.ctx, "start-slot", "status", "imc-cache-ready",
		"iteration", e.batchIteration, "slot", s.id, "seq", s.seqID,
		"total_cached", job.imcNewTotalCached, "next_logical_position", p.position,
		"media", true, "resumable", true, "session_committed", sessionWasCommitted)
	p.free()
	s.imcPrep = nil
	s.mediaPrefilling = false
	e.imcPrepNext = imcPreparationNext(selectedIndex, len(e.slots), true)
	e.finishStartSlot(s, job, llama.Pos(prep.position), prep.sessionUpdateDisabled, sessionWasCommitted, buf)
}

func imcPreparationNext(selected, slots int, complete bool) int {
	if complete {
		return (selected + 1) % slots
	}
	return selected
}

func (e *batchEngine) clearFailedIMCPreparation(s *slot) error {
	if err := e.model.waitForPendingDecode(); err != nil {
		return err
	}
	e.model.decodeMu.Lock()
	llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
	if e.model.draft != nil && e.model.draft.mtp() {
		if _, ownDraftKV := e.model.draft.(draftKVExternalizer); ownDraftKV {
			llama.MemorySeqRm(e.model.draft.core().mem, s.seqID, -1, -1)
		}
		s.draftNPast = 0
		s.mtp.PendingHidden = s.mtp.PendingHidden[:0]
	}
	e.model.decodeMu.Unlock()
	return nil
}

func (e *batchEngine) imcPreparationChunkSize() int {
	return e.model.cfg.PrefillBatchSize()
}

func imcPreparationChunkEnd(nextToken, position, totalTokens, chunkSize, boundary, finalPosition int) int {
	end := min(nextToken+chunkSize, totalTokens)

	// Do not cross the System cache boundary in a chunk. The preload must
	// describe exactly that target/draft position.
	if boundary > position && boundary < finalPosition {
		end = min(end, nextToken+boundary-position)
	}

	return end
}

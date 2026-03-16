package model

import (
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model/caching"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// startSlotIMCStaleCheck verifies the slot's cache hasn't been evicted between
// processIMC and now. Returns the current cacheIdx and false if the cache is
// stale (finishSlot already called).
func (e *batchEngine) startSlotIMCStaleCheck(s *slot, job *chatJob) (llama.Pos, bool) {
	var cacheIdx llama.Pos
	var currentHash string

	if snap, ok := e.model.cache.SnapshotSlot(s.id); ok {
		cacheIdx = llama.Pos(snap.TotalTokensCached)
		currentHash = snap.CachedMsgsHash
	}

	// Verify the slot's cache hasn't been evicted or rebuilt by another
	// goroutine between processIMC and now. This catches stale pure hits
	// only. Partial prefix rebuilds (imcTrimPos > 0) naturally have a
	// different hash because they're replacing the slot's content.
	//
	// Pure hits never set pending=true, so we must NOT clear pending here.
	// Another goroutine may own the reservation on this slot.
	if job.imc.expectedHash != "" && currentHash != job.imc.expectedHash && len(job.imc.newCacheTokens) == 0 && job.imc.trimPos == 0 && !job.imc.mediaBuild {
		e.model.log(job.ctx, "start-slot", "status", "imc-stale",
			"slot", s.id, "seq", s.seqID, "imc_slot", job.imc.slotID,
			"expected_hash", job.imc.expectedHash[:8], "current_hash", currentHash)

		s.skipKVCleanup = true
		e.finishSlot(s, fmt.Errorf("start-slot: imc cache stale (slot %d hash changed), retry request", s.id))
		return 0, false
	}

	return cacheIdx, true
}

// startSlotIMCMediaBuild handles media cache build/extend at slot start.
// Returns the new cacheIdx and true on success, or 0 and false on failure.
func (e *batchEngine) startSlotIMCMediaBuild(s *slot, job *chatJob) (llama.Pos, bool) {
	skipTokens := job.imc.mediaSkipTextTokens

	if skipTokens > 0 {
		// Partial media extend: keep existing text cache, only decode
		// new content (text suffix + media + post-media text).
		e.model.log(job.ctx, "start-slot", "status", "imc-media-extend", "slot", s.id, "seq", s.seqID,
			"skip_text_tokens", skipTokens)
	} else {
		// Full media rebuild: clear sequence and decode all cached
		// messages through the mtmd pipeline.
		e.model.log(job.ctx, "start-slot", "status", "imc-media-build", "slot", s.id, "seq", s.seqID)

		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
		e.model.decodeMu.Unlock()
	}

	imcDecodeStart := time.Now()

	totalCached, mediaKVCounts, err := e.model.decodeMediaIntoCache(job.ctx, job.imc.mediaCacheD, s.seqID, job.mtmdCtx, skipTokens)
	if err != nil {
		e.model.decodeMu.Lock()
		if skipTokens > 0 {
			// Partial extend failed: remove only the newly decoded
			// content, preserving the original text cache.
			llama.MemorySeqRm(e.model.mem, s.seqID, llama.Pos(skipTokens), -1)
		} else {
			llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
		}
		e.model.decodeMu.Unlock()

		e.model.cache.ClearPending(s.id)

		e.finishSlot(s, fmt.Errorf("start-slot: imc media build: %w", err))
		return 0, false
	}

	metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(imcDecodeStart))

	cacheIdx := llama.Pos(totalCached)

	commit := caching.Commit{
		SlotID:         s.id,
		Hash:           job.imc.newMsgsHash,
		TotalCached:    totalCached,
		CachedMsgCount: job.imc.newCachedMsgCount,
		HasMedia:       true,
		MediaKVCounts:  mediaKVCounts,
	}

	// Store whether this media build used M-RoPE so follow-up
	// text-only requests on this slot can use the correct position format.
	if job.mtmdCtx != 0 {
		commit.UseMRoPE = mtmd.DecodeUseMRope(job.mtmdCtx)
	}

	e.model.cache.CommitSession(commit)

	if skipTokens > 0 {
		e.model.log(job.ctx, "start-slot", "status", "imc-media-extended", "slot", s.id, "seq", s.seqID,
			"total_cached", totalCached, "skipped_text_tokens", skipTokens)
	} else {
		e.model.log(job.ctx, "start-slot", "status", "imc-media-built", "slot", s.id, "seq", s.seqID,
			"total_cached", totalCached)
	}

	return cacheIdx, true
}

// startSlotIMCTextBuild handles text cache extend/rebuild/trim at slot start.
// Returns the new cacheIdx and true on success, or 0 and false on failure.
func (e *batchEngine) startSlotIMCTextBuild(s *slot, job *chatJob, cacheIdx llama.Pos) (llama.Pos, bool) {
	// Detect stale extension: if another request extended this slot
	// between our scan and now, cacheIdx won't match the position
	// these tokens were sliced from. For extends (not rebuilds or
	// partial prefix trims), the expected start position is
	// imcNewTotalCached - len(imcNewCacheTokens).
	if !job.imc.clearSeq && job.imc.trimPos == 0 {
		expectedStart := llama.Pos(job.imc.newTotalCached - len(job.imc.newCacheTokens))
		if cacheIdx != expectedStart {
			e.model.log(job.ctx, "start-slot", "status", "imc-extend-stale", "slot", s.id, "seq", s.seqID,
				"cache_idx", cacheIdx, "expected_start", expectedStart,
				"new_total_cached", job.imc.newTotalCached)

			e.model.cache.ClearPending(s.id)

			s.skipKVCleanup = true
			e.finishSlot(s, fmt.Errorf("start-slot: imc extend stale (cache moved from %d to %d), retry request", expectedStart, cacheIdx))
			return 0, false
		}
	}

	switch {
	case job.imc.clearSeq:
		// Rebuilding from scratch (prefix mismatch). Clear the old
		// sequence first so we don't append on top of stale tokens.
		e.model.log(job.ctx, "start-slot", "status", "imc-clear-seq", "slot", s.id, "seq", s.seqID,
			"old_cached_tokens", cacheIdx)

		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
		e.model.decodeMu.Unlock()

		cacheIdx = 0

		e.model.log(job.ctx, "start-slot", "status", "imc-build", "slot", s.id, "seq", s.seqID,
			"tokens", len(job.imc.newCacheTokens))

	case job.imc.trimPos > 0:
		// Non-Deterministic mode: partial prefix rebuild. Trim the
		// divergent suffix from KV cache, keeping the common prefix,
		// then decode new tokens from the trim point forward.
		if job.imc.trimPos > cacheIdx {
			e.model.cache.ClearPending(s.id)

			s.skipKVCleanup = true
			e.finishSlot(s, fmt.Errorf("start-slot: imc trim stale (trim_pos %d > cache_idx %d), retry request", job.imc.trimPos, cacheIdx))
			return 0, false
		}

		switch e.model.modelInfo.Type {
		case ModelTypeHybrid:
			// Partial MemorySeqRm corrupts recurrent state. Use full
			// clear and re-decode the full cached token sequence from
			// position 0 (imcNewCachedTokens, not imcNewCacheTokens).
			e.model.log(job.ctx, "start-slot", "status", "imc-hybrid-rebuild", "slot", s.id, "seq", s.seqID,
				"cached_tokens", cacheIdx, "trim_pos", job.imc.trimPos,
				"redecode_tokens", len(job.imc.newCachedTokens))

			if !e.rebuildHybridCachedPrefix(s, job, "start-slot: imc hybrid rebuild") {
				return 0, false
			}

			cacheIdx = llama.Pos(job.imc.newTotalCached)

			// Update session state and skip the shared decode path below.
			e.model.cache.CommitSession(caching.Commit{
				SlotID:         s.id,
				Hash:           job.imc.newMsgsHash,
				TotalCached:    job.imc.newTotalCached,
				CachedMsgCount: job.imc.newCachedMsgCount,
				CachedTokens:   job.imc.newCachedTokens,
			})

			pct := int(job.imc.trimPos) * 100 / job.imc.newTotalCached
			e.model.log(job.ctx, "start-slot", "status", "imc-hybrid-rebuilt", "slot", s.id, "seq", s.seqID,
				"total_cached", job.imc.newTotalCached, "salvaged_pct", pct)

		case ModelTypeDense, ModelTypeMoE:
			e.model.log(job.ctx, "start-slot", "status", "imc-trim-prefix", "slot", s.id, "seq", s.seqID,
				"cached_tokens", cacheIdx, "trim_pos", job.imc.trimPos, "new_cache_tokens", len(job.imc.newCacheTokens))

			e.model.decodeMu.Lock()
			llama.MemorySeqRm(e.model.mem, s.seqID, job.imc.trimPos, -1)
			e.model.decodeMu.Unlock()

			cacheIdx = job.imc.trimPos
		}

	default:
		e.model.log(job.ctx, "start-slot", "status", "imc-extend", "slot", s.id, "seq", s.seqID,
			"cached_tokens", cacheIdx, "new_cache_tokens", len(job.imc.newCacheTokens))
	}

	// Hybrid trim already decoded the full token sequence and updated
	// metadata above, so skip the shared decode path.
	if !(e.model.modelInfo.Type == ModelTypeHybrid && job.imc.trimPos > 0) {
		imcDecodeStart := time.Now()

		if err := e.model.decodeTokensIntoCache(job.ctx, job.imc.newCacheTokens, s.seqID, int(cacheIdx)); err != nil {
			// Remove any partially decoded tokens so the KV sequence
			// stays consistent with the session metadata.
			e.model.decodeMu.Lock()
			switch {
			case job.imc.clearSeq:
				// Rebuild: sequence was cleared before decode, clear again
				// to remove any partial tokens.
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
			case job.imc.trimPos > 0:
				// Partial prefix: remove from trim point onward to
				// restore the pre-trim state.
				llama.MemorySeqRm(e.model.mem, s.seqID, job.imc.trimPos, -1)
			default:
				// Extend: remove from the old cache boundary onward to
				// restore the pre-extend state.
				llama.MemorySeqRm(e.model.mem, s.seqID, cacheIdx, -1)
			}
			e.model.decodeMu.Unlock()

			e.model.cache.InvalidateSlot(s.id)

			e.finishSlot(s, fmt.Errorf("start-slot: imc extend: %w", err))
			return 0, false
		}

		metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(imcDecodeStart))

		cacheIdx = llama.Pos(job.imc.newTotalCached)

		// Update session state now that tokens are decoded.
		// Preserve media state for text-only extensions of media slots.
		hasMedia := len(job.imc.mediaKVCounts) > 0
		e.model.cache.CommitSession(caching.Commit{
			SlotID:         s.id,
			Hash:           job.imc.newMsgsHash,
			TotalCached:    job.imc.newTotalCached,
			CachedMsgCount: job.imc.newCachedMsgCount,
			CachedTokens:   job.imc.newCachedTokens,
			HasMedia:       hasMedia,
			MediaKVCounts:  job.imc.mediaKVCounts,
		})

		switch {
		case job.imc.clearSeq:
			e.model.log(job.ctx, "start-slot", "status", "imc-built", "slot", s.id, "seq", s.seqID,
				"total_cached", job.imc.newTotalCached)
		case job.imc.trimPos > 0:
			pct := int(job.imc.trimPos) * 100 / job.imc.newTotalCached
			e.model.log(job.ctx, "start-slot", "status", "imc-partial-rebuilt", "slot", s.id, "seq", s.seqID,
				"total_cached", job.imc.newTotalCached, "salvaged_prefix", job.imc.trimPos, "salvaged_pct", pct)
		default:
			e.model.log(job.ctx, "start-slot", "status", "imc-extended", "slot", s.id, "seq", s.seqID,
				"total_cached", job.imc.newTotalCached)
		}
	}

	return cacheIdx, true
}

// startSlotIMCTrimOnly handles trim-only partial prefix rebuild at slot start.
// Returns the new cacheIdx and true on success, or 0 and false on failure.
func (e *batchEngine) startSlotIMCTrimOnly(s *slot, job *chatJob, cacheIdx llama.Pos) (llama.Pos, bool) {
	// Trim-only partial prefix rebuild: the common prefix equals all
	// incoming tokens so there are no new tokens to decode. Just trim
	// the divergent suffix from the KV cache and update metadata.
	if job.imc.trimPos > cacheIdx {
		e.model.cache.ClearPending(s.id)

		s.skipKVCleanup = true
		e.finishSlot(s, fmt.Errorf("start-slot: imc trim stale (trim_pos %d > cache_idx %d), retry request", job.imc.trimPos, cacheIdx))
		return 0, false
	}

	switch e.model.modelInfo.Type {
	case ModelTypeHybrid:
		// Partial MemorySeqRm corrupts recurrent state. Clear and
		// re-decode all cached tokens from position 0.
		e.model.log(job.ctx, "start-slot", "status", "imc-hybrid-trim-rebuild", "slot", s.id, "seq", s.seqID,
			"cached_tokens", cacheIdx, "trim_pos", job.imc.trimPos,
			"redecode_tokens", len(job.imc.newCachedTokens))

		if !e.rebuildHybridCachedPrefix(s, job, "start-slot: imc hybrid trim rebuild") {
			return 0, false
		}

	case ModelTypeDense, ModelTypeMoE:
		e.model.log(job.ctx, "start-slot", "status", "imc-trim-only", "slot", s.id, "seq", s.seqID,
			"cached_tokens", cacheIdx, "trim_pos", job.imc.trimPos)

		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, job.imc.trimPos, -1)
		e.model.decodeMu.Unlock()
	}

	cacheIdx = llama.Pos(job.imc.newTotalCached)

	e.model.cache.CommitSession(caching.Commit{
		SlotID:         s.id,
		Hash:           job.imc.newMsgsHash,
		TotalCached:    job.imc.newTotalCached,
		CachedMsgCount: job.imc.newCachedMsgCount,
		CachedTokens:   job.imc.newCachedTokens,
	})

	e.model.log(job.ctx, "start-slot", "status", "imc-trimmed", "slot", s.id, "seq", s.seqID,
		"total_cached", job.imc.newTotalCached)

	return cacheIdx, true
}

// clearIMCMetadataForNonCacheableRequest clears the slot's IMC session metadata
// when an uncacheable request (<2 messages) is assigned to the slot.
func (e *batchEngine) clearIMCMetadataForNonCacheableRequest(s *slot, job *chatJob) {
	if e.model.cfg.IncrementalCache && e.model.cache.HasCachedSlot(s.id) {
		e.model.cache.InvalidateSlot(s.id)

		e.model.log(job.ctx, "start-slot", "status", "imc-metadata-cleared", "slot", s.id, "seq", s.seqID)
	}
}

// captureIMCHybridSnapshot snapshots the full sequence state (KV + recurrent)
// after cache is populated and before suffix tokens are decoded.
func (e *batchEngine) captureIMCHybridSnapshot(s *slot, job *chatJob, cacheIdx llama.Pos) {
	if e.model.modelInfo.Type != ModelTypeHybrid || job.imc == nil || cacheIdx <= 0 {
		return
	}

	e.model.decodeMu.Lock()
	llama.Synchronize(e.model.lctx)
	kvSize := llama.StateSeqGetSize(e.model.lctx, s.seqID)
	switch {
	case cap(s.imcSavedState) >= int(kvSize):
		s.imcSavedState = s.imcSavedState[:kvSize]
	default:
		s.imcSavedState = make([]byte, kvSize)
	}
	nExtracted := llama.StateSeqGetData(e.model.lctx, s.imcSavedState, s.seqID)
	e.model.decodeMu.Unlock()

	switch {
	case nExtracted > 0:
		s.imcSavedState = s.imcSavedState[:nExtracted]
		e.model.log(job.ctx, "start-slot", "status", "imc-hybrid-snapshot",
			"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
			"snapshot_bytes", nExtracted, "kv_alloc", kvSize)
	default:
		s.imcSavedState = s.imcSavedState[:0]
		e.model.log(job.ctx, "start-slot", "status", "imc-hybrid-snapshot-failed",
			"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
			"kv_alloc", kvSize)
	}
}

// rebuildHybridCachedPrefix clears the slot's KV+recurrent state and re-decodes
// the full cached token sequence from position 0. Hybrid models require full
// clear + re-decode instead of partial MemorySeqRm because partial deletes
// corrupt recurrent state (DeltaNet/SSM). Returns false if decode fails
// (finishSlot already called).
func (e *batchEngine) rebuildHybridCachedPrefix(s *slot, job *chatJob, errPrefix string) bool {
	e.model.decodeMu.Lock()
	llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
	e.model.decodeMu.Unlock()

	if len(job.imc.newCachedTokens) > 0 {
		imcDecodeStart := time.Now()

		if err := e.model.decodeTokensIntoCache(job.ctx, job.imc.newCachedTokens, s.seqID, 0); err != nil {
			e.model.decodeMu.Lock()
			llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
			e.model.decodeMu.Unlock()

			e.model.cache.InvalidateSlot(s.id)

			e.finishSlot(s, fmt.Errorf("%s: %w", errPrefix, err))
			return false
		}

		metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(imcDecodeStart))
	}

	return true
}

// snapshotIMCSlotMetadata reads the final IMC slot metadata via SnapshotSlot and
// stores it in the job's imc struct. Called after cache build/extend/trim so
// that startSlotText and slotNeedsMRoPE see the post-build state.
func (e *batchEngine) snapshotIMCSlotMetadata(s *slot, job *chatJob) {
	if job.imc == nil {
		return
	}

	if snap, ok := e.model.cache.SnapshotSlot(s.id); ok {
		job.imc.prefixTokens = snap.CachedTokens
		job.imc.prefixHasMedia = snap.HasMedia
		job.imc.prefixUseMRoPE = snap.UseMRoPE
	}
}

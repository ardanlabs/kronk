package model

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// startSlot initializes a slot with a new request.
func (e *batchEngine) startSlot(s *slot, job *chatJob) {
	s.reset()
	s.active = true
	s.job = job
	// Note: startTime is set when prefillDone=true (first output token) for accurate TPS
	// seqID is already set correctly during slot creation in newBatchEngine

	// End the queue-wait span now that the job has been picked up.
	if job.queueWaitSpan != nil {
		job.queueWaitSpan.End()
	}

	// Start span for this chat request. Store the span context so child
	// spans (prefill, token-generation) are nested under process-request.
	var processCtx context.Context
	processCtx, s.span = otel.AddSpan(job.ctx, "process-request",
		attribute.String("id", job.id),
		attribute.Int("slot", s.id),
	)
	job.ctx = processCtx

	// Start prefill span and record start time for TTFT.
	_, s.prefillSpan = otel.AddSpan(processCtx, "prefill",
		attribute.Int("slot", s.id),
	)
	s.prefillStart = time.Now()

	// Create sampler for this request.
	s.sampler = e.model.toSampler(job.params)

	// Create grammar sampler if grammar is specified (kept separate from chain).
	if job.params.Grammar != "" {
		s.grammarSampler = NewGrammarSampler(e.model.vocab, job.params.Grammar)
	}

	// IMC dedicated slot mode: the slot's sequence IS the cache. No copy needed.
	// Re-read session state under lock to handle stale job data from queuing.
	var cacheIdx llama.Pos
	switch {
	case e.model.cfg.IncrementalCache && job.imcCacheHit:
		var currentHash string

		e.model.cacheMu.RLock()
		if s.id < len(e.model.imcSlots) {
			cacheIdx = llama.Pos(e.model.imcSlots[s.id].totalTokensCached)
			currentHash = e.model.imcSlots[s.id].cachedMsgsHash
		}
		e.model.cacheMu.RUnlock()

		// Verify the slot's cache hasn't been evicted or rebuilt by another
		// goroutine between processIMC and now. This catches stale pure hits
		// only. Partial prefix rebuilds (imcTrimPos > 0) naturally have a
		// different hash because they're replacing the slot's content.
		//
		// Pure hits never set pending=true, so we must NOT clear pending here.
		// Another goroutine may own the reservation on this slot.
		if job.imcExpectedHash != "" && currentHash != job.imcExpectedHash && len(job.imcNewCacheTokens) == 0 && job.imcTrimPos == 0 && !job.imcMediaBuild {
			e.model.log(job.ctx, "start-slot", "status", "imc-stale",
				"slot", s.id, "seq", s.seqID, "imc_slot", job.imcSlotID,
				"expected_hash", job.imcExpectedHash[:8], "current_hash", currentHash)

			e.finishSlot(s, fmt.Errorf("start-slot: imc cache stale (slot %d hash changed), retry request", s.id))
			return
		}

		// Decode new cache extension tokens into the slot's sequence if any.
		switch {
		case job.imcMediaBuild:
			// Media cache build: clear sequence and decode all cached messages
			// through the mtmd pipeline (text + image/audio embeddings).
			e.model.log(job.ctx, "start-slot", "status", "imc-media-build", "slot", s.id, "seq", s.seqID)

			e.model.decodeMu.Lock()
			llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
			e.model.decodeMu.Unlock()

			imcDecodeStart := time.Now()

			totalCached, mediaKVCounts, err := e.model.decodeMediaIntoCache(job.ctx, job.imcMediaCacheD, s.seqID, job.mtmdCtx)
			if err != nil {
				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				e.model.decodeMu.Unlock()

				e.model.imcClearPending(s.id)

				e.finishSlot(s, fmt.Errorf("start-slot: imc media build: %w", err))
				return
			}

			metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(imcDecodeStart))

			cacheIdx = llama.Pos(totalCached)

			e.model.imcCommitSession(s.id, job.imcNewMsgsHash, totalCached, job.imcNewCachedMsgCount, nil, true, mediaKVCounts)

			e.model.log(job.ctx, "start-slot", "status", "imc-media-built", "slot", s.id, "seq", s.seqID,
				"total_cached", totalCached)

		case len(job.imcNewCacheTokens) > 0:
			// Detect stale extension: if another request extended this slot
			// between our scan and now, cacheIdx won't match the position
			// these tokens were sliced from. For extends (not rebuilds or
			// partial prefix trims), the expected start position is
			// imcNewTotalCached - len(imcNewCacheTokens).
			if !job.imcClearSeq && job.imcTrimPos == 0 {
				expectedStart := llama.Pos(job.imcNewTotalCached - len(job.imcNewCacheTokens))
				if cacheIdx != expectedStart {
					e.model.log(job.ctx, "start-slot", "status", "imc-extend-stale", "slot", s.id, "seq", s.seqID,
						"cache_idx", cacheIdx, "expected_start", expectedStart,
						"new_total_cached", job.imcNewTotalCached)

					e.model.imcClearPending(s.id)

					e.finishSlot(s, fmt.Errorf("start-slot: imc extend stale (cache moved from %d to %d), retry request", expectedStart, cacheIdx))
					return
				}
			}

			switch {
			case job.imcClearSeq:
				// Rebuilding from scratch (prefix mismatch). Clear the old
				// sequence first so we don't append on top of stale tokens.
				e.model.log(job.ctx, "start-slot", "status", "imc-clear-seq", "slot", s.id, "seq", s.seqID,
					"old_cached_tokens", cacheIdx)

				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				e.model.decodeMu.Unlock()

				cacheIdx = 0

				e.model.log(job.ctx, "start-slot", "status", "imc-build", "slot", s.id, "seq", s.seqID,
					"tokens", len(job.imcNewCacheTokens))

			case job.imcTrimPos > 0:
				// Non-Deterministic mode: partial prefix rebuild. Trim the
				// divergent suffix from KV cache, keeping the common prefix,
				// then decode new tokens from the trim point forward.
				if job.imcTrimPos > cacheIdx {
					e.model.imcClearPending(s.id)

					e.finishSlot(s, fmt.Errorf("start-slot: imc trim stale (trim_pos %d > cache_idx %d), retry request", job.imcTrimPos, cacheIdx))
					return
				}

				switch e.model.modelInfo.Type {
				case ModelTypeHybrid:
					// Partial MemorySeqRm corrupts recurrent state. Use full
					// clear and re-decode the full cached token sequence from
					// position 0 (imcNewCachedTokens, not imcNewCacheTokens).
					e.model.log(job.ctx, "start-slot", "status", "imc-hybrid-rebuild", "slot", s.id, "seq", s.seqID,
						"cached_tokens", cacheIdx, "trim_pos", job.imcTrimPos,
						"redecode_tokens", len(job.imcNewCachedTokens))

					e.model.decodeMu.Lock()
					llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
					e.model.decodeMu.Unlock()

					// Decode the full cached token sequence from position 0.
					if len(job.imcNewCachedTokens) > 0 {
						imcDecodeStart := time.Now()

						if err := e.model.decodeTokensIntoCache(job.ctx, job.imcNewCachedTokens, s.seqID, 0); err != nil {
							e.model.decodeMu.Lock()
							llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
							e.model.decodeMu.Unlock()

							e.model.imcClearPending(s.id)

							e.finishSlot(s, fmt.Errorf("start-slot: imc hybrid rebuild: %w", err))
							return
						}

						metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(imcDecodeStart))
					}

					cacheIdx = llama.Pos(job.imcNewTotalCached)

					// Update session state and skip the shared decode path below.
					e.model.imcCommitSession(s.id, job.imcNewMsgsHash, job.imcNewTotalCached, job.imcNewCachedMsgCount, job.imcNewCachedTokens, false, nil)

					pct := int(job.imcTrimPos) * 100 / job.imcNewTotalCached
					e.model.log(job.ctx, "start-slot", "status", "imc-hybrid-rebuilt", "slot", s.id, "seq", s.seqID,
						"total_cached", job.imcNewTotalCached, "salvaged_pct", pct)

				case ModelTypeDense, ModelTypeMoE:
					e.model.log(job.ctx, "start-slot", "status", "imc-trim-prefix", "slot", s.id, "seq", s.seqID,
						"cached_tokens", cacheIdx, "trim_pos", job.imcTrimPos, "new_cache_tokens", len(job.imcNewCacheTokens))

					e.model.decodeMu.Lock()
					llama.MemorySeqRm(e.model.mem, s.seqID, job.imcTrimPos, -1)
					e.model.decodeMu.Unlock()

					cacheIdx = job.imcTrimPos
				}

			default:
				e.model.log(job.ctx, "start-slot", "status", "imc-extend", "slot", s.id, "seq", s.seqID,
					"cached_tokens", cacheIdx, "new_cache_tokens", len(job.imcNewCacheTokens))
			}

			// Hybrid trim already decoded the full token sequence and updated
			// metadata above, so skip the shared decode path.
			if !(e.model.modelInfo.Type == ModelTypeHybrid && job.imcTrimPos > 0) {
				imcDecodeStart := time.Now()

				if err := e.model.decodeTokensIntoCache(job.ctx, job.imcNewCacheTokens, s.seqID, int(cacheIdx)); err != nil {
					// Remove any partially decoded tokens so the KV sequence
					// stays consistent with the session metadata.
					e.model.decodeMu.Lock()
					switch {
					case job.imcClearSeq:
						// Rebuild: sequence was cleared before decode, clear again
						// to remove any partial tokens.
						llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
					case job.imcTrimPos > 0:
						// Partial prefix: remove from trim point onward to
						// restore the pre-trim state.
						llama.MemorySeqRm(e.model.mem, s.seqID, job.imcTrimPos, -1)
					default:
						// Extend: remove from the old cache boundary onward to
						// restore the pre-extend state.
						llama.MemorySeqRm(e.model.mem, s.seqID, cacheIdx, -1)
					}
					e.model.decodeMu.Unlock()

					e.model.imcClearPending(s.id)

					e.finishSlot(s, fmt.Errorf("start-slot: imc extend: %w", err))
					return
				}

				metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(imcDecodeStart))

				cacheIdx = llama.Pos(job.imcNewTotalCached)

				// Update session state now that tokens are decoded.
				// Preserve media state for text-only extensions of media slots.
				hasMedia := len(job.imcMediaKVCounts) > 0
				e.model.imcCommitSession(s.id, job.imcNewMsgsHash, job.imcNewTotalCached, job.imcNewCachedMsgCount, job.imcNewCachedTokens, hasMedia, job.imcMediaKVCounts)

				switch {
				case job.imcClearSeq:
					e.model.log(job.ctx, "start-slot", "status", "imc-built", "slot", s.id, "seq", s.seqID,
						"total_cached", job.imcNewTotalCached)
				case job.imcTrimPos > 0:
					pct := int(job.imcTrimPos) * 100 / job.imcNewTotalCached
					e.model.log(job.ctx, "start-slot", "status", "imc-partial-rebuilt", "slot", s.id, "seq", s.seqID,
						"total_cached", job.imcNewTotalCached, "salvaged_prefix", job.imcTrimPos, "salvaged_pct", pct)
				default:
					e.model.log(job.ctx, "start-slot", "status", "imc-extended", "slot", s.id, "seq", s.seqID,
						"total_cached", job.imcNewTotalCached)
				}
			}

		case job.imcTrimPos > 0:
			// Trim-only partial prefix rebuild: the common prefix equals all
			// incoming tokens so there are no new tokens to decode. Just trim
			// the divergent suffix from the KV cache and update metadata.
			if job.imcTrimPos > cacheIdx {
				e.model.imcClearPending(s.id)

				e.finishSlot(s, fmt.Errorf("start-slot: imc trim stale (trim_pos %d > cache_idx %d), retry request", job.imcTrimPos, cacheIdx))
				return
			}

			switch e.model.modelInfo.Type {
			case ModelTypeHybrid:
				// Partial MemorySeqRm corrupts recurrent state. Clear and
				// re-decode all cached tokens from position 0.
				e.model.log(job.ctx, "start-slot", "status", "imc-hybrid-trim-rebuild", "slot", s.id, "seq", s.seqID,
					"cached_tokens", cacheIdx, "trim_pos", job.imcTrimPos,
					"redecode_tokens", len(job.imcNewCachedTokens))

				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				e.model.decodeMu.Unlock()

				if len(job.imcNewCachedTokens) > 0 {
					imcDecodeStart := time.Now()

					if err := e.model.decodeTokensIntoCache(job.ctx, job.imcNewCachedTokens, s.seqID, 0); err != nil {
						e.model.decodeMu.Lock()
						llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
						e.model.decodeMu.Unlock()

						e.model.imcClearPending(s.id)

						e.finishSlot(s, fmt.Errorf("start-slot: imc hybrid trim rebuild: %w", err))
						return
					}

					metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(imcDecodeStart))
				}

			case ModelTypeDense, ModelTypeMoE:
				e.model.log(job.ctx, "start-slot", "status", "imc-trim-only", "slot", s.id, "seq", s.seqID,
					"cached_tokens", cacheIdx, "trim_pos", job.imcTrimPos)

				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, job.imcTrimPos, -1)
				e.model.decodeMu.Unlock()
			}

			cacheIdx = llama.Pos(job.imcNewTotalCached)

			e.model.imcCommitSession(s.id, job.imcNewMsgsHash, job.imcNewTotalCached, job.imcNewCachedMsgCount, job.imcNewCachedTokens, false, nil)

			e.model.log(job.ctx, "start-slot", "status", "imc-trimmed", "slot", s.id, "seq", s.seqID,
				"total_cached", job.imcNewTotalCached)

		case cacheIdx > 0:
			e.model.log(job.ctx, "start-slot", "status", "imc-reuse", "slot", s.id, "seq", s.seqID,
				"cached_tokens", cacheIdx)
		}

	default:
		// Non-IMC mode: clear the slot's sequence and copy from cache if available.
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)

		// If IMC is enabled but this request wasn't cacheable (e.g., <2 messages),
		// clear the slot's IMC session metadata so it stays consistent with the
		// now-empty KV sequence.
		if e.model.cfg.IncrementalCache && s.id < len(e.model.imcSlots) {
			e.model.cacheMu.Lock()
			slot := e.model.imcSlots[s.id]
			slot.cachedMsgsHash = ""
			slot.totalTokensCached = 0
			slot.cachedMsgCount = 0
			slot.pending = false
			e.model.cacheMu.Unlock()
			e.model.notifyIMCSlotAvailable()

			e.model.log(job.ctx, "start-slot", "status", "imc-metadata-cleared", "slot", s.id, "seq", s.seqID)
		}

		switch {
		case job.spcCacheHit:
			e.model.log(job.ctx, "start-slot", "status", "spc-restore", "dst_seq", s.seqID, "cached_tokens", job.spcCacheIdx)
			if err := e.model.restoreSPCToSeq(s.seqID); err != nil {
				e.finishSlot(s, fmt.Errorf("start-slot: %w", err))
				return
			}
			cacheIdx = job.spcCacheIdx

			e.model.log(job.ctx, "start-slot", "status", "spc-restored", "slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx)
		}
	}

	s.nPast = cacheIdx

	// IMC Hybrid: snapshot the full sequence state (KV + recurrent) after
	// cache is populated and before suffix tokens are decoded. This snapshot
	// is restored in finishSlot instead of using partial MemorySeqRm which
	// corrupts recurrent state (DeltaNet/SSM layers).
	if e.model.modelInfo.Type == ModelTypeHybrid && e.model.cfg.IncrementalCache && job.imcCacheHit && cacheIdx > 0 {
		e.model.decodeMu.Lock()
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
			e.model.log(job.ctx, "start-slot", "status", "imc-hybrid-snapshot-failed",
				"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
				"kv_alloc", kvSize)
		}
	}

	// Branch based on request type: media vs text-only.
	// Use len(job.media) to distinguish: after an IMC media cache build the
	// suffix is text-only (images are already in KV cache), so route to
	// startSlotText even though job.object may be ObjectChatMedia.
	switch {
	case job.object == ObjectChatMedia && len(job.media) > 0:
		if !e.startSlotMedia(s, job, cacheIdx) {
			return
		}

	default:
		if !e.startSlotText(s, job, cacheIdx) {
			return
		}
	}

	// Calculate current KV usage for diagnostics.
	var kvUsed llama.Pos
	if sysMax, err := llama.MemorySeqPosMax(e.model.mem, 0); err == nil && sysMax >= 0 {
		kvUsed += sysMax + 1
	}

	for _, slot := range e.slots {
		if slot.active && slot.id != s.id {
			if posMax, err := llama.MemorySeqPosMax(e.model.mem, slot.seqID); err == nil && posMax >= 0 {
				kvUsed += posMax + 1
			}
		}
	}

	e.model.log(job.ctx, "batch-engine", "status", "slot-started", "slot", s.id, "seq", s.seqID, "id", job.id,
		"total_prompt", s.nPrompt, "spc_cache_hit", job.spcCacheHit,
		"imc_cache_hit", job.imcCacheHit, "imc_slot", job.imcSlotID, "imc_seq", job.imcSeqID, "kv_used", kvUsed)
}

// startSlotText initializes a text-only slot. Returns true on success.
func (e *batchEngine) startSlotText(s *slot, job *chatJob, cacheIdx llama.Pos) bool {
	// Tokenize the prompt (cached messages already removed).
	// Only add BOS if no cached tokens AND model metadata says to add BOS.
	addBOS := cacheIdx == 0 && e.model.addBOSToken
	tokens := llama.Tokenize(e.model.vocab, job.prompt, addBOS, true)

	// suffixTokens is the number of new tokens to process (not cached).
	// totalPrompt is the full context size including cached tokens.
	suffixTokens := len(tokens)
	totalPrompt := suffixTokens + int(cacheIdx)
	s.nPrompt = totalPrompt

	// Log token counts for debugging batch overflow.
	e.model.log(job.ctx, "start-slot", "status", "tokenized",
		"slot", s.id,
		"suffix_tokens", suffixTokens,
		"cached_tokens", cacheIdx,
		"total_prompt", totalPrompt,
		"nbatch", e.model.cfg.NBatch,
		"batch_current", e.batch.NTokens)

	// Check context window.
	if s.nPrompt > e.model.cfg.ContextWindow {
		err := fmt.Errorf("start-slot: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow)
		e.finishSlot(s, err)
		return false
	}

	// Store full prompt tokens for draft model prefill if speculative decoding
	// is enabled. The draft model needs all tokens (cached + new suffix) to
	// build its KV cache after the target's prefill completes. Reuses the
	// pre-allocated promptBuf to avoid per-request allocations.
	// Skip when the slot has media cached — cachedTokens can't represent
	// image/audio embeddings, so the draft model can't reconstruct the prompt.
	slotHasMedia := false
	if job.imcCacheHit && s.id < len(e.model.imcSlots) {
		e.model.cacheMu.RLock()
		slotHasMedia = e.model.imcSlots[s.id].hasMedia
		e.model.cacheMu.RUnlock()
	}
	if e.model.draft != nil && !slotHasMedia {
		draft := e.model.draft
		var needed int
		var cachedLen int

		switch {
		case job.imcCacheHit && s.id < len(e.model.imcSlots):
			e.model.cacheMu.RLock()
			cached := e.model.imcSlots[s.id].cachedTokens
			e.model.cacheMu.RUnlock()

			cachedLen = len(cached)
			needed = cachedLen + len(tokens)

			if cap(draft.promptBuf) >= needed {
				draft.promptBuf = draft.promptBuf[:needed]
			} else {
				draft.promptBuf = make([]llama.Token, needed)
			}
			copy(draft.promptBuf, cached)
			copy(draft.promptBuf[cachedLen:], tokens)

		default:
			needed = len(tokens)

			if cap(draft.promptBuf) >= needed {
				draft.promptBuf = draft.promptBuf[:needed]
			} else {
				draft.promptBuf = make([]llama.Token, needed)
			}
			copy(draft.promptBuf, tokens)
		}

		s.draftPromptTokens = draft.promptBuf

		e.model.log(job.ctx, "speculative", "status", "draft-prompt-assembled",
			"slot", s.id, "imc_cached", cachedLen, "new_suffix", len(tokens),
			"total_draft_tokens", len(s.draftPromptTokens))

		s.draftPrefillNeeded = true
	}

	// Store tokens for chunked prefill.
	s.prefillTokens = tokens
	s.nPrefilled = 0

	// Add first chunk of prompt tokens to batch.
	if !e.addPrefillChunk(s) {
		e.finishSlot(s, e.slotCancelError(s))
		return false
	}

	return true
}

// startSlotMedia initializes a media (vision/audio) slot. Returns true on success.
func (e *batchEngine) startSlotMedia(s *slot, job *chatJob, cacheIdx llama.Pos) bool {
	// Convert raw media bytes into bitmap structures for the vision encoder.
	if len(job.media) > 0 {
		s.bitmaps = make([]mtmd.Bitmap, len(job.media))
		for i, med := range job.media {
			if len(med) == 0 {
				continue
			}
			s.bitmaps[i] = mtmd.BitmapInitFromBuf(job.mtmdCtx, &med[0], uint64(len(med)))
		}
	}

	// Create input chunks that interleave text tokens with image embeddings.
	s.inputChunks = mtmd.InputChunksInit()

	// Tokenize produces a sequence of chunks: text tokens and image patches.
	input := mtmd.NewInputText(job.prompt, true, true)
	if result := mtmd.Tokenize(job.mtmdCtx, s.inputChunks, input, s.bitmaps); result != 0 {
		err := fmt.Errorf("start-slot-media: tokenization failed with code %d", result)
		e.finishSlot(s, err)
		return false
	}

	// Set model-specific flags for positioning and attention.
	s.useMRoPE = mtmd.DecodeUseMRope(job.mtmdCtx)
	s.useNonCausal = mtmd.DecodeUseNonCausal(job.mtmdCtx)

	// Count total tokens across all chunks.
	numChunks := mtmd.InputChunksSize(s.inputChunks)
	var totalTokens uint64
	for i := range numChunks {
		chunk := mtmd.InputChunksGet(s.inputChunks, i)
		totalTokens += mtmd.InputChunkGetNTokens(chunk)
	}

	s.nPrompt = int(totalTokens) + int(cacheIdx)
	s.chunkIdx = 0

	e.model.log(job.ctx, "start-slot-media", "status", "tokenized",
		"slot", s.id,
		"num_chunks", numChunks,
		"total_tokens", totalTokens,
		"cached_tokens", cacheIdx,
		"use_mrope", s.useMRoPE,
		"use_noncausal", s.useNonCausal)

	// Check context window.
	if s.nPrompt > e.model.cfg.ContextWindow {
		err := fmt.Errorf("start-slot-media: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow)
		e.finishSlot(s, err)
		return false
	}

	// Process first chunk. Media prefill is handled chunk-by-chunk in processBatch.
	// Allocate buffer for potential first-token sampling (single-chunk edge case).
	buf := make([]byte, 32*1024)
	if !e.addPrefillMediaChunk(s, buf) {
		e.finishSlot(s, e.slotCancelError(s))
		return false
	}

	return true
}

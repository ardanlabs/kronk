package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// startSlot initializes a slot with a new request.
func (e *batchEngine) startSlot(s *slot, job *chatJob, buf []byte) {
	s.reset()
	s.active = true
	s.job = job

	// If the rendered prompt ends with "<think>\n", the template has already
	// opened a reasoning block. Prime the processor and slot to start in
	// reasoning mode so generated tokens are correctly classified until
	// </think>. Setting reasonFlag ensures grammar sampling is skipped
	// during the thinking phase.
	//
	// Skip reasoning mode when grammar is specified — grammar constrains
	// the output format, so free-form thinking is counterproductive and
	// would consume max_tokens before producing any constrained content.
	if strings.HasSuffix(job.prompt, "<think>\n") && job.params.Grammar == "" {
		// Drive the state machine into reasoning mode by feeding the same
		// marker the model would have emitted. Processors that recognize
		// <think> (standard, qwen, mistral, glm) flip to ChannelReasoning;
		// processors that don't (gemma, gpt) treat it as content — but
		// those processors do not produce a "<think>\n" suffix in the
		// prompt, so this branch never runs for them.
		s.stateMachine.Process("<think>")
		s.reasonFlag = 1
	}

	// End the queue-wait span now that the job has been picked up.
	if job.queueWaitSpan != nil {
		job.queueWaitSpan.End()
	}
	if !job.queuedAt.IsZero() {
		metrics.ObserveChatQueueWait(e.model.modelInfo.ID, time.Since(job.queuedAt))
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
	s.sampler = e.model.toSampler(job.ctx, job.params)

	// Create grammar sampler if grammar is specified (kept separate from chain).
	if job.params.Grammar != "" {
		s.grammarSampler = NewGrammarSampler(e.model.vocab, job.params.Grammar)
	}

	// IMC: restore externalized KV state from session.kvState, then decode
	// any extension tokens. The slot's sequence was cleared in the previous
	// finishSlot, so we always restore from RAM.
	//
	// Read cache state from the MATCHED session (job.imcSession), not from
	// the slot's own session (imcSessions[s.id]). With externalized KV, any
	// slot can serve any session — the slot index doesn't correspond to the
	// session index.
	var cacheIdx llama.Pos
	switch {
	case e.model.cfg.IncrementalCache() && job.imcCacheHit:
		// Snapshot session state under lock. With externalized KV, the
		// session's kvState slice header may be reset/regrown by another
		// goroutine's eviction, so we must copy the slice header atomically.
		var kvState []byte
		if job.imcSession != nil {
			e.model.cacheMu.RLock()
			cacheIdx = llama.Pos(job.imcSession.totalTokensCached)
			kvState = job.imcSession.kvState.Bytes()
			e.model.cacheMu.RUnlock()
		}

		// Restore externalized KV state from session.kvState into this
		// slot's sequence via StateSeqSetData. The slot's
		// sequence was cleared in finishSlot, so we must restore before
		// decoding extension tokens or processing the suffix.
		if len(kvState) > 0 && !job.imcClearSeq {
			e.model.log(job.ctx, "start-slot", "status", "imc-restore-start",
				"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
				"ram_bytes", fmtBytes(uint64(len(kvState))))

			restoreStart := time.Now()

			e.model.decodeMu.Lock()
			nRead := llama.StateSeqSetData(e.model.lctx, kvState, s.seqID)
			e.model.decodeMu.Unlock()

			if nRead == 0 {
				e.model.imcClearPending(s.id)
				e.finishSlot(s, fmt.Errorf("start-slot: imc restore failed for seq %d", s.seqID))
				return
			}

			e.model.log(job.ctx, "start-slot", "status", "imc-restore-done",
				"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
				"restored_bytes", fmtBytes(nRead), "elapsed", fmtDur(time.Since(restoreStart)))
		}

		// Decode new cache extension tokens into the slot's sequence if any.
		switch {
		case job.imcMediaBuild:
			skipTokens := job.imcMediaSkipTextTokens

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

			totalCached, mediaKVCounts, err := e.model.decodeMediaIntoCache(job.ctx, job.imcMediaCacheD, s.seqID, job.mtmdCtx, skipTokens)
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

				e.model.imcClearPending(s.id)

				e.finishSlot(s, fmt.Errorf("start-slot: imc media build: %w", err))
				return
			}

			metrics.AddPrefillTime(e.model.modelInfo.ID, "imc-decode", time.Since(imcDecodeStart))

			cacheIdx = llama.Pos(totalCached)

			e.model.imcCommitSession(job.imcSession, job.imcNewMsgsHash, totalCached, job.imcNewCachedMsgCount, nil, true, mediaKVCounts, job.imcSysPromptHash, job.imcSysPromptTokens)

			// Store whether this media build used M-RoPE so follow-up
			// text-only requests on this session can use the correct position format.
			if job.mtmdCtx != 0 && job.imcSession != nil {
				e.model.cacheMu.Lock()
				job.imcSession.useMRoPE = mtmd.DecodeUseMRope(job.mtmdCtx)
				e.model.cacheMu.Unlock()
			}

			if skipTokens > 0 {
				e.model.log(job.ctx, "start-slot", "status", "imc-media-extended", "slot", s.id, "seq", s.seqID,
					"total_cached", totalCached, "skipped_text_tokens", skipTokens)
			} else {
				e.model.log(job.ctx, "start-slot", "status", "imc-media-built", "slot", s.id, "seq", s.seqID,
					"total_cached", totalCached)
			}

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
				// Token prefix fallback or sys-prompt-preserve: trim the
				// divergent suffix and re-decode only the new tokens.
				//
				// Dense/MoE: partial MemorySeqRm is safe — delete from
				// trimPos onward and decode only the new conversation body.
				// This preserves the system prompt KV state.
				//
				// Hybrid: partial MemorySeqRm corrupts recurrent state
				// (DeltaNet/SSM), so clear the entire sequence and
				// re-decode everything from scratch.
				if e.model.modelInfo.Type == ModelTypeHybrid {
					e.model.log(job.ctx, "start-slot", "status", "imc-rebuild-full", "slot", s.id, "seq", s.seqID,
						"trim_pos", job.imcTrimPos, "total_tokens", len(job.imcNewCachedTokens))

					e.model.decodeMu.Lock()
					llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
					e.model.decodeMu.Unlock()

					job.imcNewCacheTokens = job.imcNewCachedTokens
					cacheIdx = 0
				} else {
					e.model.log(job.ctx, "start-slot", "status", "imc-trim-prefix", "slot", s.id, "seq", s.seqID,
						"trim_pos", job.imcTrimPos, "kept_tokens", job.imcTrimPos,
						"new_tokens", len(job.imcNewCacheTokens))

					e.model.decodeMu.Lock()
					llama.MemorySeqRm(e.model.mem, s.seqID, llama.Pos(job.imcTrimPos), -1)
					e.model.decodeMu.Unlock()

					cacheIdx = llama.Pos(job.imcTrimPos)
				}

			default:
				e.model.log(job.ctx, "start-slot", "status", "imc-extend", "slot", s.id, "seq", s.seqID,
					"cached_tokens", cacheIdx, "new_cache_tokens", len(job.imcNewCacheTokens))
			}

			// Decode extension tokens into the slot's sequence.
			imcDecodeStart := time.Now()

			if err := e.model.decodeTokensIntoCache(job.ctx, job.imcNewCacheTokens, s.seqID, int(cacheIdx)); err != nil {
				e.model.decodeMu.Lock()
				llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
				e.model.decodeMu.Unlock()

				if job.imcSession != nil {
					e.model.cacheMu.Lock()
					imcResetSession(job.imcSession)
					e.model.cacheMu.Unlock()
					e.model.notifyIMCSlotAvailable()
				}

				e.finishSlot(s, fmt.Errorf("start-slot: imc decode: %w", err))
				return
			}

			metrics.AddPrefillTime(e.model.modelInfo.ID, "imc-decode", time.Since(imcDecodeStart))

			cacheIdx = llama.Pos(job.imcNewTotalCached)

			hasMedia := len(job.imcMediaKVCounts) > 0
			e.model.imcCommitSession(job.imcSession, job.imcNewMsgsHash, job.imcNewTotalCached, job.imcNewCachedMsgCount, job.imcNewCachedTokens, hasMedia, job.imcMediaKVCounts, job.imcSysPromptHash, job.imcSysPromptTokens)

			e.model.log(job.ctx, "start-slot", "status", "imc-cache-ready", "slot", s.id, "seq", s.seqID,
				"total_cached", job.imcNewTotalCached)

		case cacheIdx > 0:
			e.model.log(job.ctx, "start-slot", "status", "imc-reuse", "slot", s.id, "seq", s.seqID,
				"cached_tokens", cacheIdx)
		}

	default:
		// Non-IMC mode: clear the slot's sequence.
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
	}

	s.nPast = cacheIdx

	// Snapshot the cached prefix KV state into session.kvState. This
	// externalized state is used to restore the cache into any available slot
	// on the next request. The snapshot is taken AFTER cache build/extend but
	// BEFORE suffix tokens are decoded, capturing exactly the cached
	// conversation prefix.
	//
	// StateSeqGetData captures raw KV bytes regardless of whether they were
	// produced by text tokens or media embeddings (image/audio). For Hybrid
	// models it also captures recurrent state (DeltaNet/SSM).
	if e.model.cfg.IncrementalCache() && job.imcCacheHit && cacheIdx > 0 && job.imcSession != nil {
		e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-start",
			"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx)

		snapshotStart := time.Now()

		// Reuse the session's kvBuffer in place. Prepare returns a slice
		// of length kvSize, reusing the existing backing array when its
		// capacity is sufficient (the common case after the first turn)
		// and allocating only when the conversation has grown beyond any
		// previous peak. Per-session serialization (the pending flag and
		// the imcSessions ownership model) guarantees no concurrent reader
		// holds a reference to this buffer while we fill it.
		//
		// capBefore lets us log whether this snapshot grew the backing
		// array (allocation) or reused it (zero allocation) — the central
		// invariant we want to observe in production.
		capBefore := job.imcSession.kvState.Cap()

		e.model.decodeMu.Lock()
		llama.Synchronize(e.model.lctx)
		kvSize := llama.StateSeqGetSize(e.model.lctx, s.seqID)
		kvBuf := job.imcSession.kvState.Prepare(int(kvSize))
		nExtracted := llama.StateSeqGetData(e.model.lctx, kvBuf, s.seqID)
		e.model.decodeMu.Unlock()

		capAfter := job.imcSession.kvState.Cap()
		bufAction := "reuse"
		if capAfter > capBefore {
			bufAction = "grow"
		}

		// Commit (or zero) the buffer length under cacheMu so concurrent
		// readers (LRU snapshot scans, future requests matching this
		// session) see a consistent length.
		e.model.cacheMu.Lock()
		job.imcSession.kvState.Commit(int(nExtracted))
		e.model.cacheMu.Unlock()

		if nExtracted > 0 {
			e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-done",
				"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
				"snapshot_bytes", fmtBytes(nExtracted), "kv_alloc", fmtBytes(kvSize),
				"buf_action", bufAction,
				"buf_cap_before", fmtBytes(uint64(capBefore)),
				"buf_cap_after", fmtBytes(uint64(capAfter)),
				"elapsed", fmtDur(time.Since(snapshotStart)))
		} else {
			e.model.log(job.ctx, "start-slot", "status", "imc-snapshot-failed",
				"slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx,
				"kv_alloc", fmtBytes(kvSize),
				"buf_action", bufAction,
				"buf_cap_before", fmtBytes(uint64(capBefore)),
				"buf_cap_after", fmtBytes(uint64(capAfter)),
				"elapsed", fmtDur(time.Since(snapshotStart)))
		}
	}

	// Branch based on request type: media vs text-only.
	// Use len(job.media) to distinguish: after an IMC media cache build the
	// suffix is text-only (images are already in KV cache), so route to
	// startSlotText even though job.object may be ObjectChatMedia.
	//
	// Special case: if the IMC media cache was built using M-RoPE positions,
	// the suffix text must also use M-RoPE 4D positions to maintain consistent
	// positional encoding. Route through startSlotTextMRoPE which decodes via
	// the M-RoPE text helper instead of the shared batch.
	switch {
	case job.object == ObjectChatMedia && len(job.media) > 0:
		if !e.startSlotMedia(s, job, cacheIdx, buf) {
			return
		}

	case e.slotNeedsMRoPE(s, job):
		if !e.startSlotTextMRoPE(s, job, cacheIdx, buf) {
			return
		}

	default:
		if !e.startSlotText(s, job, cacheIdx) {
			return
		}
	}

	// Calculate current KV usage for diagnostics.
	var kvUsed llama.Pos
	for _, slot := range e.slots {
		if slot.active {
			if posMax, err := llama.MemorySeqPosMax(e.model.mem, slot.seqID); err == nil && posMax >= 0 {
				kvUsed += posMax + 1
			}
		}
	}

	e.model.log(job.ctx, "batch-engine", "status", "slot-started", "slot", s.id, "seq", s.seqID, "id", job.id,
		"total_prompt", s.nPrompt, "imc_cache_hit", job.imcCacheHit, "imc_slot", job.imcSlotID, "kv_used", kvUsed)
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
		"nbatch", e.model.cfg.NBatch(),
		"batch_current", e.batch.NTokens)

	// Check context window.
	if s.nPrompt > e.model.cfg.ContextWindow() {
		err := fmt.Errorf("start-slot: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow())
		e.finishSlot(s, err)
		return false
	}

	// Store full prompt tokens for draft model prefill if speculative decoding
	// is enabled. The draft model needs all tokens (cached + new suffix) to
	// build its KV cache after the target's prefill completes. Reuses the
	// pre-allocated promptBuf to avoid per-request allocations.
	// Skip when the slot has media cached — cachedTokens can't represent
	// image/audio embeddings, so the draft model can't reconstruct the prompt.
	draftSlotHasMedia := false
	if job.imcCacheHit && job.imcSession != nil {
		e.model.cacheMu.RLock()
		draftSlotHasMedia = job.imcSession.hasMedia
		e.model.cacheMu.RUnlock()
	}
	if e.model.draft != nil && !draftSlotHasMedia {
		draft := e.model.draft
		var needed int
		var cachedLen int

		switch {
		case job.imcCacheHit && job.imcSession != nil:
			e.model.cacheMu.RLock()
			cached := job.imcSession.cachedTokens
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

	// Add first chunk of prompt tokens to batch. Use NBatch as the limit
	// since this is the initial fill for a newly assigned slot.
	if !e.addPrefillChunk(s, e.model.cfg.NBatch()) {
		e.finishSlot(s, e.slotCancelError(s))
		return false
	}

	return true
}

// slotNeedsMRoPE returns true if the slot has cached media that was built with
// M-RoPE 4D positions, meaning the suffix text must also use M-RoPE decoding.
func (e *batchEngine) slotNeedsMRoPE(s *slot, job *chatJob) bool {
	if !job.imcCacheHit {
		return false
	}

	// For the initial media build, check the mtmdCtx directly.
	if job.imcMediaBuild && job.mtmdCtx != 0 {
		return mtmd.DecodeUseMRope(job.mtmdCtx)
	}

	// For follow-up requests, check the stored flag on the matched session.
	if job.imcSession != nil {
		e.model.cacheMu.RLock()
		needsMRoPE := job.imcSession.useMRoPE
		e.model.cacheMu.RUnlock()
		return needsMRoPE
	}

	return false
}

// startSlotTextMRoPE initializes a text-only slot that must use M-RoPE 4D
// positioning. This is used when the IMC media cache was built with M-RoPE
// positions (e.g., Qwen vision models) and the suffix text must use the same
// positional encoding scheme. Decodes the suffix via decodeTextMRoPE instead
// of the shared batch, then samples the first token. Returns true on success.
func (e *batchEngine) startSlotTextMRoPE(s *slot, job *chatJob, cacheIdx llama.Pos, buf []byte) bool {
	addBOS := cacheIdx == 0 && e.model.addBOSToken
	tokens := llama.Tokenize(e.model.vocab, job.prompt, addBOS, true)

	suffixTokens := len(tokens)
	totalPrompt := suffixTokens + int(cacheIdx)
	s.nPrompt = totalPrompt

	e.model.log(job.ctx, "start-slot", "status", "tokenized-mrope-suffix",
		"slot", s.id,
		"suffix_tokens", suffixTokens,
		"cached_tokens", cacheIdx,
		"total_prompt", totalPrompt)

	if s.nPrompt > e.model.cfg.ContextWindow() {
		err := fmt.Errorf("start-slot: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow())
		e.finishSlot(s, err)
		return false
	}

	s.useMRoPE = true

	nBatch := e.model.cfg.NBatch()
	for start := 0; start < len(tokens); start += nBatch {
		end := min(start+nBatch, len(tokens))
		if err := e.decodeTextMRoPE(s, tokens[start:end]); err != nil {
			e.finishSlot(s, fmt.Errorf("decode cached-media suffix (M-RoPE) failed: %w", err))
			return false
		}
	}

	return e.sampleFirstToken(s, buf)
}

// startSlotMedia initializes a media (vision/audio) slot. Returns true on success.
func (e *batchEngine) startSlotMedia(s *slot, job *chatJob, cacheIdx llama.Pos, buf []byte) bool {
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
	s.useNonCausal = mtmd.DecodeUseNonCausal(job.mtmdCtx, 0)

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
	if s.nPrompt > e.model.cfg.ContextWindow() {
		err := fmt.Errorf("start-slot-media: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow())
		e.finishSlot(s, err)
		return false
	}

	// Process first chunk. Media prefill is handled chunk-by-chunk in processBatch.
	if !e.addPrefillMediaChunk(s, buf) {
		e.finishSlot(s, e.slotCancelError(s))
		return false
	}

	return true
}

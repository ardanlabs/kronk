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
	case e.model.cfg.IncrementalCache && job.imcID != "":
		e.model.cacheMu.RLock()
		if s.id < len(e.model.imcSlots) {
			cacheIdx = llama.Pos(e.model.imcSlots[s.id].totalTokensCached)
		}
		e.model.cacheMu.RUnlock()

		// Decode new cache extension tokens into the slot's sequence if any.
		switch {
		case len(job.imcNewCacheTokens) > 0:
			switch job.imcClearSeq {
			case true:
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

			case false:
				e.model.log(job.ctx, "start-slot", "status", "imc-extend", "slot", s.id, "seq", s.seqID,
					"cached_tokens", cacheIdx, "new_cache_tokens", len(job.imcNewCacheTokens))
			}

			imcDecodeStart := time.Now()

			if err := e.model.decodeTokensIntoCache(job.ctx, job.imcNewCacheTokens, s.seqID, int(cacheIdx)); err != nil {
				e.model.cacheMu.Lock()
				if s.id < len(e.model.imcSlots) {
					e.model.imcSlots[s.id].pending = false
					e.model.log(job.ctx, "start-slot", "status", "imc-pending-cleared (error)", "slot", s.id, "seq", s.seqID)
				}
				e.model.cacheMu.Unlock()

				e.finishSlot(s, fmt.Errorf("start-slot: imc extend: %w", err))
				return
			}

			metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(imcDecodeStart))

			cacheIdx = llama.Pos(job.imcNewTotalCached)

			// Update session state now that tokens are decoded.
			e.model.cacheMu.Lock()
			if s.id < len(e.model.imcSlots) {
				imcSlot := e.model.imcSlots[s.id]
				imcSlot.cachedMsgsHash = job.imcNewMsgsHash
				imcSlot.totalTokensCached = job.imcNewTotalCached
				imcSlot.lastMsgIdxCached = job.imcNewMsgIdx
				imcSlot.lastUsed = time.Now()
				imcSlot.pending = false

				e.model.log(job.ctx, "start-slot", "status", "imc-pending-cleared", "slot", s.id, "seq", s.seqID)
			}
			e.model.cacheMu.Unlock()

			switch job.imcClearSeq {
			case true:
				e.model.log(job.ctx, "start-slot", "status", "imc-built", "slot", s.id, "seq", s.seqID,
					"total_cached", job.imcNewTotalCached)
			case false:
				e.model.log(job.ctx, "start-slot", "status", "imc-extended", "slot", s.id, "seq", s.seqID,
					"total_cached", job.imcNewTotalCached)
			}

		case cacheIdx > 0:
			e.model.log(job.ctx, "start-slot", "status", "imc-reuse", "slot", s.id, "seq", s.seqID,
				"cached_tokens", cacheIdx)
		}

	default:
		// Non-IMC mode: clear the slot's sequence and copy from cache if available.
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)

		switch {
		case job.spcCacheHit && len(job.spcTokens) > 0:
			cachePos, err := e.model.decodeSPCToSlot(job.ctx, job.spcTokens, s.seqID)
			if err != nil {
				e.finishSlot(s, fmt.Errorf("start-slot: %w", err))
				return
			}
			cacheIdx = cachePos

			e.model.log(job.ctx, "start-slot", "status", "spc-decoded", "slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx)
		}
	}

	s.nPast = cacheIdx

	// Branch based on request type: media vs text-only.
	switch job.object {
	case ObjectChatMedia:
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
		"total_prompt", s.nPrompt, "spc_cache_hit", job.spcCacheHit, "imc_cache_hit", job.imcCacheHit, "kv_used", kvUsed)
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

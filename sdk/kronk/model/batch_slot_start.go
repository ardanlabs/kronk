package model

import (
	"context"
	"fmt"
	"time"

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
	case job.imc != nil:
		var ok bool
		cacheIdx, ok = e.startSlotIMCStaleCheck(s, job)
		if !ok {
			return
		}

		switch {
		case job.imc.mediaBuild:
			cacheIdx, ok = e.startSlotIMCMediaBuild(s, job)
			if !ok {
				return
			}

		case len(job.imc.newCacheTokens) > 0:
			cacheIdx, ok = e.startSlotIMCTextBuild(s, job, cacheIdx)
			if !ok {
				return
			}

		case job.imc.trimPos > 0:
			cacheIdx, ok = e.startSlotIMCTrimOnly(s, job, cacheIdx)
			if !ok {
				return
			}

		case cacheIdx > 0:
			e.model.log(job.ctx, "start-slot", "status", "imc-reuse", "slot", s.id, "seq", s.seqID,
				"cached_tokens", cacheIdx)
		}

		// Re-snapshot slot metadata after cache build/extend/trim may have
		// updated it via CommitSession. startSlotText and slotNeedsMRoPE
		// need the final state, not the pre-build snapshot.
		e.snapshotIMCSlotMetadata(s, job)

	default:
		// Non-IMC mode: clear the slot's sequence and copy from cache if available.
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)

		e.clearIMCMetadataForNonCacheableRequest(s, job)

		switch {
		case job.spc != nil:
			e.model.log(job.ctx, "start-slot", "status", "spc-restore", "dst_seq", s.seqID, "cached_tokens", job.spc.cacheIdx)
			if err := e.model.cache.RestoreSPCToSeq(s.seqID); err != nil {
				e.finishSlot(s, fmt.Errorf("start-slot: %w", err))
				return
			}
			cacheIdx = job.spc.cacheIdx

			e.model.log(job.ctx, "start-slot", "status", "spc-restored", "slot", s.id, "seq", s.seqID, "cached_tokens", cacheIdx)
		}
	}

	s.nPast = cacheIdx

	e.captureIMCHybridSnapshot(s, job, cacheIdx)

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

	spcHit := job.spc != nil
	imcHit := job.imc != nil
	var imcSlot int
	var imcSeq llama.SeqId
	if imcHit {
		imcSlot = job.imc.slotID
		imcSeq = job.imc.seqID
	}

	e.model.log(job.ctx, "batch-engine", "status", "slot-started", "slot", s.id, "seq", s.seqID, "id", job.id,
		"total_prompt", s.nPrompt, "spc_cache_hit", spcHit,
		"imc_active", imcHit, "imc_slot", imcSlot, "imc_seq", imcSeq, "kv_used", kvUsed)
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
	slotHasMedia := job.imc != nil && job.imc.prefixHasMedia
	if e.model.draft != nil && !slotHasMedia {
		draft := e.model.draft
		var needed int
		var cachedLen int

		switch {
		case job.imc != nil && len(job.imc.prefixTokens) > 0:
			cached := job.imc.prefixTokens

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
	if !e.addPrefillChunk(s, e.model.cfg.NBatch) {
		e.finishSlot(s, e.slotCancelError(s))
		return false
	}

	return true
}

// slotNeedsMRoPE returns true if the slot has cached media that was built with
// M-RoPE 4D positions, meaning the suffix text must also use M-RoPE decoding.
func (e *batchEngine) slotNeedsMRoPE(s *slot, job *chatJob) bool {
	if job.imc == nil {
		return false
	}

	// For the initial media build, check the mtmdCtx directly.
	if job.imc.mediaBuild && job.mtmdCtx != 0 {
		return mtmd.DecodeUseMRope(job.mtmdCtx)
	}

	// For follow-up requests, use the snapshot populated in startSlotIMCStaleCheck.
	return job.imc.prefixUseMRoPE
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

	if s.nPrompt > e.model.cfg.ContextWindow {
		err := fmt.Errorf("start-slot: input tokens [%d] exceed context window [%d]", s.nPrompt, e.model.cfg.ContextWindow)
		e.finishSlot(s, err)
		return false
	}

	s.useMRoPE = true

	nBatch := e.model.cfg.NBatch
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
	if !e.addPrefillMediaChunk(s, buf) {
		e.finishSlot(s, e.slotCancelError(s))
		return false
	}

	return true
}

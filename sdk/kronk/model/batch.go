package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// chatJob represents a validated chat request ready for batch processing.
// Created by submitToBatchEngine after request validation and cache lookup.
type chatJob struct {

	// -------------------------------------------------------------------------
	// Request Identity

	id            string              // Unique request ID for logging and responses
	ctx           context.Context     // Request context for cancellation and tracing
	ch            chan<- ChatResponse // Channel for streaming responses back to caller
	queueWaitSpan trace.Span         // Span covering time spent waiting in the queue

	// -------------------------------------------------------------------------
	// Request Content

	d      D        // Original request document (messages, parameters)
	object string   // Request type: ObjectChatText or ObjectChatMedia
	prompt string   // Templated prompt string ready for tokenization
	media  [][]byte // Raw media bytes (images/audio) for vision/audio models
	params Params   // Sampling and generation parameters

	// -------------------------------------------------------------------------
	// MTMD Context

	mtmdCtx mtmd.Context // Multi-modal context for vision/audio processing

	// -------------------------------------------------------------------------
	// System Prompt Cache (SPC)

	spcCacheIdx llama.Pos // Token position where SPC cache ends
	spcCacheHit bool      // True if system prompt was found in cache

	// -------------------------------------------------------------------------
	// Incremental Message Cache (IMC)

	imcID       string      // Cache session ID (from cache_id in request)
	imcSeqID    llama.SeqId // Sequence ID containing cached conversation state
	imcCacheIdx llama.Pos   // Token position where IMC cache ends
	imcCacheHit bool        // True if conversation history was found in cache

	// IMC dedicated slot fields.
	imcNewCacheTokens []llama.Token // New tokens to extend the cache in the slot's sequence
	imcNewTotalCached int           // Total cached tokens after extension
	imcNewMsgIdx      int           // New lastMsgIdxCached after extension
	imcNewMsgsHash    string        // New cachedMsgsHash after extension
	imcClearSeq       bool          // True if sequence must be cleared before decoding (rebuild)
}

// slot represents a processing slot for parallel inference. Each slot can
// process one chat request at a time, with multiple slots enabling concurrent
// request handling within a single model context.
type slot struct {

	// -------------------------------------------------------------------------
	// Identity & Lifecycle

	id     int           // Slot index within the batch engine
	seqID  llama.SeqId   // KV cache sequence ID for this slot
	seqIDs []llama.SeqId // Pre-allocated slice for batch.Add calls
	job    *chatJob      // Current request being processed
	active bool          // True when slot is processing a request
	span   trace.Span    // OpenTelemetry span for request tracing
	proc   *processor    // Response processor for content streaming

	// -------------------------------------------------------------------------
	// Sampling

	sampler        llama.Sampler   // Token sampler with temperature, top-p, etc.
	grammarSampler *grammarSampler // Grammar-constrained sampler (separate from chain)
	sampled        llama.Token     // Most recently sampled token
	iBatch         int32           // Index of this slot's token within the batch

	// -------------------------------------------------------------------------
	// Position & Token Counts

	nPast            llama.Pos // Current position in KV cache
	nPrompt          int       // Total prompt tokens (cached + new)
	reasonTokens     int       // Tokens in reasoning/thinking section
	completionTokens int       // Tokens in completion section

	// -------------------------------------------------------------------------
	// Text Prefill (text-only requests)

	prefillTokens []llama.Token // Tokens awaiting prefill
	nPrefilled    int           // Number of tokens already prefilled
	prefillDone   bool          // True when prefill complete, generation started

	// -------------------------------------------------------------------------
	// MTMD Prefill (vision/audio requests)

	inputChunks  mtmd.InputChunks // Tokenized chunks (text + media interleaved)
	chunkIdx     int              // Index of chunk currently being processed
	chunkTokIdx  int              // Token index within current text chunk (for partial prefill)
	bitmaps      []mtmd.Bitmap    // Image bitmaps to free when done
	useMRoPE     bool             // Model uses M-RoPE 4D positioning
	useNonCausal bool             // Model uses non-causal attention for media

	// -------------------------------------------------------------------------
	// Response Accumulation

	reasonFlag     int             // State: in reasoning section
	completionFlag int             // State: in completion section
	toolFlag       int             // State: in tool call section
	finalContent   strings.Builder // Accumulated completion text
	finalReasoning strings.Builder // Accumulated reasoning text
	finalTooling   strings.Builder // Accumulated tool call JSON
	respToolCalls  []ResponseToolCall
	utf8Buf        []byte // Buffered bytes from partial multi-byte UTF-8 codepoints

	// -------------------------------------------------------------------------
	// Logprobs

	logprobsData   []ContentLogprob // Accumulated logprobs for all tokens
	currentLogprob *ContentLogprob  // Current token's logprob (for streaming)

	// -------------------------------------------------------------------------
	// Metrics

	startTime       time.Time  // Start time for TPS calculation (set after prefill)
	prefillStart    time.Time  // Start time for TTFT calculation
	prefillSpan     trace.Span // Span covering the prefill phase
	tokenGenSpan    trace.Span // Span covering the token generation phase
}

func (s *slot) reset() {
	// Note: seqID is NOT reset - it's assigned once during slot creation
	// and remains stable for the lifetime of the slot.

	s.job = nil
	s.nPast = 0
	s.nPrompt = 0
	s.reasonTokens = 0
	s.completionTokens = 0
	s.reasonFlag = 0
	s.completionFlag = 0
	s.toolFlag = 0
	s.finalContent.Reset()
	s.finalReasoning.Reset()
	s.finalTooling.Reset()
	s.respToolCalls = nil
	s.utf8Buf = s.utf8Buf[:0]
	s.span = nil
	s.iBatch = -1
	s.sampled = 0
	s.active = false
	s.prefillDone = false
	s.prefillTokens = nil
	s.nPrefilled = 0
	s.logprobsData = nil
	s.currentLogprob = nil
	s.grammarSampler = nil
	s.prefillStart = time.Time{}
	s.prefillSpan = nil
	s.tokenGenSpan = nil

	// MTMD fields.
	s.inputChunks = 0
	s.chunkIdx = 0
	s.chunkTokIdx = 0
	s.bitmaps = nil
	s.useMRoPE = false
	s.useNonCausal = false

	if s.proc != nil {
		s.proc.resetState()
	}
}

// batchEngine manages parallel inference slots.
type batchEngine struct {
	model      *Model
	nSlots     int
	slots      []*slot
	batch      llama.Batch
	requestQ   chan *chatJob
	wakeCh     chan struct{}
	shutdownCh chan struct{}
	wg         sync.WaitGroup
	stopped    atomic.Bool
}

// newBatchEngine creates a new batch engine for parallel inference.
func newBatchEngine(m *Model, nSlots int) *batchEngine {
	// Create batch buffer.
	nCtx := llama.NCtx(m.lctx)
	batch := llama.BatchInit(int32(nCtx), 0, int32(nSlots))

	// Calculate sequence offset based on reserved cache sequences.
	// SPC uses seq 0, slots start after.
	// IMC uses dedicated slot/seq binding — no separate cache sequences.
	var cacheSeqs int
	switch {
	case m.cfg.SystemPromptCache:
		cacheSeqs = m.spcMaxSeqs
	case m.cfg.IncrementalCache:
		// IMC uses dedicated slot/seq binding — no separate cache sequences.
		cacheSeqs = 0
	}

	// Initialize slots.
	slots := make([]*slot, nSlots)
	for i := range slots {
		seqID := llama.SeqId(i + cacheSeqs)
		slots[i] = &slot{
			id:     i,
			seqID:  seqID,
			seqIDs: []llama.SeqId{seqID}, // Pre-allocate for batchAdd
			proc:   newProcessor(m),
		}
	}

	return &batchEngine{
		model:      m,
		nSlots:     nSlots,
		slots:      slots,
		batch:      batch,
		requestQ:   make(chan *chatJob, nSlots*2),
		wakeCh:     make(chan struct{}, 1),
		shutdownCh: make(chan struct{}),
	}
}

// start begins the batch processing loop.
func (e *batchEngine) start(ctx context.Context) {
	e.wg.Add(1)
	go e.processLoop(ctx)
	e.model.log(ctx, "batch-engine", "status", "started", "slots", e.nSlots)
}

// stop signals shutdown and waits for completion.
func (e *batchEngine) stop(ctx context.Context) {
	if !e.stopped.CompareAndSwap(false, true) {
		e.wg.Wait() // Still wait for processLoop to exit
		return
	}

	close(e.shutdownCh)
	e.wg.Wait()

	// Free samplers - batch is freed separately in Unload.
	for _, s := range e.slots {
		if s.sampler != 0 {
			llama.SamplerFree(s.sampler)
			s.sampler = 0
		}
	}

	e.model.log(ctx, "batch-engine", "status", "stopped")
}

// freeBatch frees the batch buffer. Called from Model.Unload.
func (e *batchEngine) freeBatch() {
	llama.BatchFree(e.batch)
}

// submit adds a job to the processing queue.
func (e *batchEngine) submit(job *chatJob) error {
	select {
	case e.requestQ <- job:
		select {
		case e.wakeCh <- struct{}{}:
		default:
		}
		return nil

	case <-e.shutdownCh:
		return fmt.Errorf("submit: engine shutting down")

	case <-job.ctx.Done():
		return job.ctx.Err()
	}
}

// processLoop is the main batch processing goroutine using a signal-based wake
// algorithm. Instead of polling at a fixed interval, it wakes immediately when
// new requests arrive on requestQ, eliminating up to 1ms latency on request
// pickup. When slots are actively generating, it polls at 100µs for low-latency
// token streaming. When idle, it backs off to 5ms to reduce CPU usage.
func (e *batchEngine) processLoop(ctx context.Context) {
	defer e.wg.Done()

	buf := make([]byte, 32*1024)

	const (
		activeInterval = 100 * time.Microsecond // Fast poll when slots are generating
		idleInterval   = 5 * time.Millisecond   // Slow poll when no active slots
	)

	timer := time.NewTimer(idleInterval)
	defer timer.Stop()

	for {
		select {
		case <-e.shutdownCh:
			e.drainSlots()
			return

		case <-e.wakeCh:
			if !timer.Stop() {
				select {
				case <-timer.C:

				default:
				}
			}

			// Coalesce multiple wake signals to avoid redundant iterations.
		drain:
			for {
				select {
				case <-e.wakeCh:

				default:
					break drain
				}
			}

		case <-timer.C:
		}

		switch e.hasActiveSlots() || len(e.requestQ) > 0 {
		case true:
			e.processBatch(ctx, buf)
			timer.Reset(activeInterval)

		case false:
			timer.Reset(idleInterval)
		}
	}
}

// hasActiveSlots returns true if any slot is currently processing.
func (e *batchEngine) hasActiveSlots() bool {
	for _, s := range e.slots {
		if s.active {
			return true
		}
	}
	return false
}

// processBatch handles one iteration of the batch processing loop.
func (e *batchEngine) processBatch(ctx context.Context, buf []byte) {
	// Clear the batch.
	e.batch.Clear()

	// Continue prefill for text-only slots.
	for _, s := range e.slots {
		if !s.active || s.prefillTokens == nil {
			continue
		}

		// Check if client cancelled.
		if s.job.ctx.Err() != nil {
			e.finishSlot(s, s.job.ctx.Err())
			continue
		}

		// addPrefillChunk returns false if shutdown or context cancelled.
		if !e.addPrefillChunk(s) {
			e.finishSlot(s, e.slotCancelError(s))
			continue
		}
	}

	// Continue prefill for media slots (separate loop since they may need separate decode calls).
	for _, s := range e.slots {
		if !s.active || s.inputChunks == 0 {
			continue
		}

		// Check if client cancelled.
		if s.job.ctx.Err() != nil {
			e.finishSlot(s, s.job.ctx.Err())
			continue
		}

		// Process next chunk of media request.
		// Note: addPrefillMediaChunk calls finishSlot on error, so we just continue.
		if !e.addPrefillMediaChunk(s, buf) {
			continue
		}
	}

	// Add tokens from active slots that have completed prefill.
	for _, s := range e.slots {
		if !s.active || !s.prefillDone {
			continue
		}

		// Check if client cancelled.
		if s.job.ctx.Err() != nil {
			e.finishSlot(s, s.job.ctx.Err())
			continue
		}

		s.iBatch = e.batch.NTokens
		e.batch.Add(s.sampled, s.nPast, s.seqIDs, true)
		s.nPast++
	}

	// Fill empty slots from queue.
	e.fillSlots()

	// Nothing to process.
	if e.batch.NTokens == 0 {
		return
	}

	// Defensive check: batch tokens must not exceed NBatch.
	nBatch := e.model.cfg.NBatch
	if int(e.batch.NTokens) > nBatch {
		e.model.log(ctx, "process-batch", "ERROR", "batch-overflow",
			"batch_tokens", e.batch.NTokens,
			"nbatch_limit", nBatch,
			"slots", e.nSlots)

		// Log per-slot state for debugging.
		for _, s := range e.slots {
			if s.active {
				e.model.log(ctx, "process-batch", "slot-state",
					"slot", s.id,
					"prefill_remaining", max(0, len(s.prefillTokens)-s.nPrefilled),
					"prefill_done", s.prefillDone,
					"n_past", s.nPast,
					"i_batch", s.iBatch)
			}
		}

		// Fail all active slots with descriptive error.
		overflowErr := fmt.Errorf("process-batch: %d tokens exceeds NBatch limit of %d", e.batch.NTokens, nBatch)
		for _, s := range e.slots {
			if s.active {
				e.finishSlot(s, overflowErr)
			}
		}

		return
	}

	// Lock to prevent concurrent decode with cache population.
	e.model.decodeMu.Lock()
	ret, err := llama.Decode(e.model.lctx, e.batch)
	e.model.decodeMu.Unlock()

	if err != nil || ret != 0 {
		e.logDecodeError(ctx, ret, err)

		// Fail all active slots to prevent infinite retry loop.
		decodeErr := decodeError(ret, err)
		for _, s := range e.slots {
			if s.active {
				e.finishSlot(s, decodeErr)
			}
		}
		return
	}

	// Sample tokens for each active slot.
	for _, s := range e.slots {
		if s.iBatch < 0 || !s.active {
			continue
		}

		e.processSlotToken(s, buf)
	}
}

// fillSlots assigns pending requests to available slots.
func (e *batchEngine) fillSlots() {
	if e.model.cfg.IncrementalCache {
		e.fillSlotsIMC()
		return
	}

	for _, s := range e.slots {
		if s.active {
			continue
		}

		// Try to get a request from the queue.
		select {
		case job := <-e.requestQ:
			e.startSlot(s, job)
			return // Only prefill one slot per iteration to avoid exceeding NBatch

		default:
			return
		}
	}
}

// fillSlotsIMC routes IMC jobs to their dedicated slots. Each cache_id is
// bound to a specific slot, so jobs must wait for their assigned slot.
func (e *batchEngine) fillSlotsIMC() {
	select {
	case job := <-e.requestQ:
		// Find the dedicated slot for this job's cache_id.
		if job.imcID != "" {
			e.model.cacheMu.RLock()
			session, exists := e.model.imcSessions[job.imcID]
			e.model.cacheMu.RUnlock()

			if exists && session.slotID < len(e.slots) {
				s := e.slots[session.slotID]
				if !s.active {
					e.startSlot(s, job)
					return
				}

				// Dedicated slot is busy — put job back for retry.
				select {
				case e.requestQ <- job:
				default:
					e.finishSlot(s, fmt.Errorf("fillSlots: IMC queue full, dropping request"))
				}
				return
			}
		}

		// No dedicated slot found (new session or no cache_id).
		// Assign to any available slot.
		for _, s := range e.slots {
			if !s.active {
				e.startSlot(s, job)
				return
			}
		}

		// All slots busy — put job back.
		select {
		case e.requestQ <- job:
		default:
		}

	default:
	}
}

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
	if e.model.cfg.IncrementalCache && job.imcID != "" {
		e.model.cacheMu.RLock()
		session, exists := e.model.imcSessions[job.imcID]
		if exists {
			cacheIdx = llama.Pos(session.totalTokensCached)
		}
		e.model.cacheMu.RUnlock()

		// Decode new cache extension tokens into the slot's sequence if any.
		if len(job.imcNewCacheTokens) > 0 {
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

			if err := e.model.decodeTokensIntoCache(job.ctx, job.imcNewCacheTokens, s.seqID, int(cacheIdx)); err != nil {
				e.finishSlot(s, fmt.Errorf("start-slot: imc extend: %w", err))
				return
			}

			cacheIdx = llama.Pos(job.imcNewTotalCached)

			// Update session state now that tokens are decoded.
			e.model.cacheMu.Lock()
			if session, exists := e.model.imcSessions[job.imcID]; exists {
				session.cachedMsgsHash = job.imcNewMsgsHash
				session.totalTokensCached = job.imcNewTotalCached
				session.lastMsgIdxCached = job.imcNewMsgIdx
				session.lastUsed = time.Now()
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
		} else if cacheIdx > 0 {
			e.model.log(job.ctx, "start-slot", "status", "imc-reuse", "slot", s.id, "seq", s.seqID,
				"cached_tokens", cacheIdx)
		}
	} else {
		// Non-IMC mode: clear the slot's sequence and copy from cache if available.
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)

		switch {
		case job.spcCacheHit:
			e.model.log(job.ctx, "start-slot", "status", "spc-copy", "src_seq", job.imcSeqID, "dst_seq", s.seqID, "cached_tokens", job.spcCacheIdx)
			if err := e.model.copyCachesToSeq(s.seqID, job.imcSeqID); err != nil {
				e.finishSlot(s, fmt.Errorf("start-slot: %w", err))
				return
			}
			cacheIdx = job.spcCacheIdx
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

// addPrefillChunk adds the next chunk of prefill tokens to the batch.
// Returns false only on shutdown or context cancellation; true otherwise.
func (e *batchEngine) addPrefillChunk(s *slot) bool {
	if s.prefillTokens == nil || s.nPrefilled >= len(s.prefillTokens) {
		return true
	}

	// Check for cancellation before processing chunk.
	select {
	case <-e.shutdownCh:
		return false

	case <-s.job.ctx.Done():
		return false

	default:
	}

	prefillStart := time.Now()

	nBatch := e.model.cfg.NBatch
	remaining := len(s.prefillTokens) - s.nPrefilled

	// Limit chunk size to available space in batch (total across all slots
	// must not exceed NBatch).
	availableInBatch := nBatch - int(e.batch.NTokens)
	if availableInBatch <= 0 {
		s.iBatch = -1
		return true
	}

	chunkSize := min(remaining, availableInBatch)

	// Add chunk of tokens to batch.
	for i := range chunkSize {
		tok := s.prefillTokens[s.nPrefilled+i]
		isLast := s.nPrefilled+i == len(s.prefillTokens)-1
		e.batch.Add(tok, s.nPast, s.seqIDs, isLast)
		s.nPast++
	}
	s.nPrefilled += chunkSize

	prefillDuration := time.Since(prefillStart)
	metrics.AddPrefillTime(e.model.modelInfo.ID, prefillDuration)

	// Check if prefill is complete.
	if s.nPrefilled >= len(s.prefillTokens) {
		s.iBatch = e.batch.NTokens - 1
		s.prefillTokens = nil
		if s.span.IsRecording() {
			s.span.SetAttributes(attribute.String("prefill-nonmedia", prefillDuration.String()))
		}
		return true
	}

	s.iBatch = -1
	return true
}

// addPrefillMediaChunk processes the next chunk of a media request.
// For text chunks, tokens are added to the shared batch.
// For image chunks, embeddings are encoded and decoded separately.
// Returns false if cancelled; true otherwise (even if still prefilling).
func (e *batchEngine) addPrefillMediaChunk(s *slot, buf []byte) bool {
	numChunks := int(mtmd.InputChunksSize(s.inputChunks))

	// Check if all chunks have been processed.
	if s.chunkIdx >= numChunks {
		return true
	}

	// Check for cancellation.
	select {
	case <-e.shutdownCh:
		return false

	case <-s.job.ctx.Done():
		return false

	default:
	}

	prefillStart := time.Now()
	chunk := mtmd.InputChunksGet(s.inputChunks, uint64(s.chunkIdx))
	chunkType := mtmd.InputChunkGetType(chunk)
	nTokens := mtmd.InputChunkGetNTokens(chunk)

	switch chunkType {
	case mtmd.InputChunkTypeText:
		tokens := mtmd.InputChunkGetTokensText(chunk)
		if len(tokens) == 0 {
			s.chunkIdx++
			s.chunkTokIdx = 0
			return true
		}

		nBatch := e.model.cfg.NBatch

		switch s.useMRoPE {
		case true:
			// M-RoPE: process all tokens via separate decode (doesn't use shared batch).
			for start := s.chunkTokIdx; start < len(tokens); start += nBatch {
				end := min(start+nBatch, len(tokens))
				batchTokens := tokens[start:end]

				if err := e.decodeTextMRoPE(s, batchTokens); err != nil {
					e.finishSlot(s, fmt.Errorf("decode text chunk (M-RoPE) failed: %w", err))
					return false
				}
			}
			s.chunkTokIdx = 0
			s.chunkIdx++

		case false:
			// Non-M-RoPE: add tokens to shared batch with capacity check.
			remaining := len(tokens) - s.chunkTokIdx
			availableInBatch := nBatch - int(e.batch.NTokens)

			if availableInBatch <= 0 {
				s.iBatch = -1
				return true
			}

			chunkSize := min(remaining, availableInBatch)
			isLastChunk := s.chunkIdx == numChunks-1

			for i := range chunkSize {
				tokIdx := s.chunkTokIdx + i
				isLast := tokIdx == len(tokens)-1 && isLastChunk
				e.batch.Add(tokens[tokIdx], s.nPast, s.seqIDs, isLast)
				s.nPast++
			}
			s.chunkTokIdx += chunkSize

			// Check if text chunk is complete.
			switch s.chunkTokIdx >= len(tokens) {
			case true:
				s.chunkTokIdx = 0
				s.chunkIdx++

			case false:
				s.iBatch = -1
				return true
			}
		}

		// Check if this was the last chunk.
		switch s.chunkIdx >= numChunks {
		case true:
			switch s.useMRoPE {
			case true:
				// M-RoPE text uses separate decode, so we must sample the first
				// token immediately since nothing was added to the shared batch.
				if !e.sampleFirstToken(s, buf) {
					return false
				}
			case false:
				// Non-M-RoPE text was added to shared batch, sample after decode.
				s.iBatch = e.batch.NTokens - 1
			}
			s.inputChunks = 0
			if s.span.IsRecording() {
				s.span.SetAttributes(attribute.String("prefill-media", time.Since(prefillStart).String()))
			}
		case false:
			s.iBatch = -1
		}

	case mtmd.InputChunkTypeImage:
		e.model.log(s.job.ctx, "prefill-media", "status", "encoding-image",
			"slot", s.id, "chunk", s.chunkIdx, "tokens", nTokens)

		// Step 1: Encode the image chunk (runs through vision encoder).
		if err := mtmd.EncodeChunk(s.job.mtmdCtx, chunk); err != nil {
			e.finishSlot(s, fmt.Errorf("encode image chunk failed: %w", err))
			return false
		}

		// Step 2: Retrieve the computed embeddings.
		nEmbd := llama.ModelNEmbdInp(e.model.model)
		embedSize := nEmbd * int32(nTokens)
		embd, err := mtmd.GetOutputEmbd(s.job.mtmdCtx, embedSize)
		if err != nil {
			e.finishSlot(s, fmt.Errorf("get image embeddings failed: %w", err))
			return false
		}

		// Step 3: Decode embeddings into the LLM's KV cache.
		// This uses a separate decode call since embeddings can't batch with tokens.
		switch s.useMRoPE {
		case true:
			imageTokens := mtmd.InputChunkGetTokensImage(chunk)
			nx := int32(mtmd.ImageTokensGetNX(imageTokens))
			ny := int32(mtmd.ImageTokensGetNY(imageTokens))

			e.model.log(s.job.ctx, "prefill-media", "status", "decoding-image-mrope",
				"slot", s.id, "nx", nx, "ny", ny)

			if err := e.decodeEmbeddingsMRoPE(s, embd, nEmbd, int32(nTokens), nx, ny); err != nil {
				e.finishSlot(s, fmt.Errorf("decode image embeddings (M-RoPE) failed: %w", err))
				return false
			}

		case false:
			if err := e.decodeEmbeddingsNormal(s, embd, nEmbd, int32(nTokens)); err != nil {
				e.finishSlot(s, fmt.Errorf("decode image embeddings failed: %w", err))
				return false
			}
		}

		s.chunkIdx++

		// Check if this was the last chunk.
		switch s.chunkIdx >= numChunks {
		case true:
			// Image chunks use separate decode, so we must sample the first
			// token immediately since nothing was added to the shared batch.
			if !e.sampleFirstToken(s, buf) {
				return false
			}
			s.inputChunks = 0
			if s.span.IsRecording() {
				s.span.SetAttributes(attribute.String("prefill-media", time.Since(prefillStart).String()))
			}
		case false:
			s.iBatch = -1
		}

		metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(prefillStart))

	case mtmd.InputChunkTypeAudio:
		e.model.log(s.job.ctx, "prefill-media", "status", "encoding-audio",
			"slot", s.id, "chunk", s.chunkIdx, "tokens", nTokens)

		// Step 1: Encode the audio chunk (runs through audio encoder).
		if err := mtmd.EncodeChunk(s.job.mtmdCtx, chunk); err != nil {
			e.finishSlot(s, fmt.Errorf("encode audio chunk failed: %w", err))
			return false
		}

		// Step 2: Retrieve the computed embeddings.
		nEmbd := llama.ModelNEmbdInp(e.model.model)
		embedSize := nEmbd * int32(nTokens)
		embd, err := mtmd.GetOutputEmbd(s.job.mtmdCtx, embedSize)
		if err != nil {
			e.finishSlot(s, fmt.Errorf("get audio embeddings failed: %w", err))
			return false
		}

		// Step 3: Decode embeddings into the LLM's KV cache.
		// Audio uses standard linear positioning (not M-RoPE).
		if err := e.decodeEmbeddingsNormal(s, embd, nEmbd, int32(nTokens)); err != nil {
			e.finishSlot(s, fmt.Errorf("decode audio embeddings failed: %w", err))
			return false
		}

		s.chunkIdx++

		// Check if this was the last chunk.
		switch s.chunkIdx >= numChunks {
		case true:
			// Audio uses separate decode, so sample first token immediately.
			if !e.sampleFirstToken(s, buf) {
				return false
			}
			s.inputChunks = 0
			if s.span.IsRecording() {
				s.span.SetAttributes(attribute.String("prefill-media", time.Since(prefillStart).String()))
			}

		case false:
			s.iBatch = -1
		}

		metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(prefillStart))
	}

	return true
}

// processSlotToken samples and processes a token for a slot.
func (e *batchEngine) processSlotToken(s *slot, buf []byte) {
	// Sample the next token. If grammar is active, use grammar-aware sampling.
	var token llama.Token
	switch {
	case s.grammarSampler != nil:
		token = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, s.iBatch)

	default:
		token = llama.SamplerSample(s.sampler, e.model.lctx, s.iBatch)
	}

	e.handleSampledToken(s, token, s.iBatch, buf)
}

// handleSampledToken processes a sampled token through the full pipeline:
// logprobs extraction, grammar/sampler acceptance, EOG check, state machine,
// streaming, and token counting. Used by both processSlotToken and sampleFirstToken.
func (e *batchEngine) handleSampledToken(s *slot, token llama.Token, iBatch int32, buf []byte) {
	// Extract logprobs BEFORE accepting - Accept modifies sampler state.
	// Reset currentLogprob each token; it's used for streaming.
	s.currentLogprob = nil
	if s.job.params.Logprobs {
		logprob, err := extractLogprobs(e.model.lctx, e.model.vocab, token, iBatch, s.job.params.TopLogprobs, buf)
		switch {
		case err != nil:
			e.model.log(s.job.ctx, "batch-engine", "status", "logprobs-error", "slot", s.id, "error", err.Error())

		case logprob != nil:
			s.currentLogprob = logprob
			s.logprobsData = append(s.logprobsData, *logprob)
		}
	}

	// Accept token on both samplers. Grammar sampler is accepted separately
	// to avoid the crash that occurs when grammar is in the chain.
	if s.grammarSampler != nil {
		s.grammarSampler.Accept(token)
	}

	llama.SamplerAccept(s.sampler, token)

	// Check for end of generation.
	if llama.VocabIsEOG(e.model.vocab, token) {
		e.finishSlot(s, nil)
		return
	}

	// Convert token to text, buffering partial UTF-8 codepoints.
	l := llama.TokenToPiece(e.model.vocab, token, buf, 0, true)

	s.utf8Buf = append(s.utf8Buf, buf[:l]...)

	complete, remainder := extractCompleteUTF8(s.utf8Buf)

	// Convert to string BEFORE mutating the buffer. The complete slice
	// shares the same backing array as s.utf8Buf, so we must copy via
	// string() first to avoid corruption.
	var content string
	if len(complete) > 0 {
		content = string(complete)
	}

	if len(remainder) > 0 {
		s.utf8Buf = append(s.utf8Buf[:0], remainder...)
	} else {
		s.utf8Buf = s.utf8Buf[:0]
	}

	s.sampled = token

	if !s.prefillDone {
		s.prefillDone = true
		s.startTime = time.Now() // Start TPS clock after prefill, when first output token is generated

		// Record TTFT and end the prefill span.
		ttft := time.Since(s.prefillStart)
		metrics.AddTimeToFirstToken(e.model.modelInfo.ID, ttft)

		if s.prefillSpan != nil {
			if s.prefillSpan.IsRecording() {
				s.prefillSpan.SetAttributes(attribute.String("ttft", ttft.String()))
			}
			s.prefillSpan.End()
			s.prefillSpan = nil
		}

		// Start token generation span.
		_, s.tokenGenSpan = otel.AddSpan(s.job.ctx, "token-generation",
			attribute.Int("slot", s.id),
		)
	}

	// If no complete UTF-8 codepoints are ready, count the token using the
	// current flags (partial bytes can't trigger a state transition) and skip
	// the processor and streaming.
	if len(content) == 0 {
		switch {
		case s.reasonFlag > 0:
			s.reasonTokens++
		default:
			s.completionTokens++
		}

		outputTokens := s.reasonTokens + s.completionTokens

		if outputTokens >= s.job.params.MaxTokens {
			e.finishSlot(s, nil)
			return
		}

		s.iBatch = -1
		return
	}

	// Process through the state machine.
	isGPT := e.model.modelInfo.IsGPTModel
	var resp response
	var eog bool

	switch isGPT {
	case true:
		resp, eog = s.proc.stepGPT(content)

	default:
		resp, eog = s.proc.stepStandard(content)
	}

	if eog {
		e.finishSlot(s, nil)
		return
	}

	// Update flags based on response status.
	switch resp.status {
	case statusReasoning:
		s.reasonFlag++
		s.completionFlag = 0
		s.toolFlag = 0

	case statusCompletion:
		s.completionFlag++
		s.reasonFlag = 0
		s.toolFlag = 0

	case statusTooling:
		s.toolFlag++
		s.reasonFlag = 0
		s.completionFlag = 0

	default:
		// No streamable content (statusNone) - skip without counting.
		// This happens for control tokens like <|end|> which shouldn't be counted.
		s.iBatch = -1
		return
	}

	// Count the token after the state machine has updated the flags so
	// that attribution (reasoning vs completion) reflects the actual
	// section this token belongs to.
	switch {
	case s.reasonFlag > 0:
		s.reasonTokens++
	default:
		s.completionTokens++
	}

	outputTokens := s.reasonTokens + s.completionTokens

	if outputTokens >= s.job.params.MaxTokens {
		e.finishSlot(s, nil)
		return
	}

	// Store content for final response.
	switch {
	case s.reasonFlag > 0:
		s.finalReasoning.WriteString(resp.content)

	case s.toolFlag > 0:
		s.finalTooling.WriteString(resp.content)

	default:
		s.finalContent.WriteString(resp.content)
	}

	// Stream response if not tooling.
	if s.toolFlag == 0 {
		// Skip unnecessary CRLF at mode transitions.
		if e.model.isUnncessaryCRLF(s.reasonFlag, s.completionFlag, resp.content) {
			s.iBatch = -1
			return
		}

		// Per OpenAI spec, usage is only sent in the final response, not deltas.
		err := e.model.sendDeltaResponse(s.job.ctx, s.job.ch, s.job.id, s.job.object, 0, "", resp.content, s.reasonFlag, outputTokens, s.currentLogprob)
		if err != nil {
			e.finishSlot(s, err)
			return
		}
	}

	s.iBatch = -1
}

// finishSlot completes a slot and sends the final response.
func (e *batchEngine) finishSlot(s *slot, err error) {
	if !s.active {
		return
	}

	ctx := s.job.ctx
	jobID := s.job.id
	slotID := s.id
	seqID := s.seqID

	defer func() {
		close(s.job.ch)

		if s.prefillSpan != nil {
			s.prefillSpan.End()
			s.prefillSpan = nil
		}

		if s.tokenGenSpan != nil {
			s.tokenGenSpan.SetAttributes(
				attribute.Int("output_tokens", s.reasonTokens+s.completionTokens),
			)
			s.tokenGenSpan.End()
			s.tokenGenSpan = nil
		}

		s.span.End()
		e.freeSlotResources(s)
		s.reset()

		remaining := e.model.activeStreams.Add(-1)

		e.model.log(ctx, "batch-engine",
			"status", "slot-finished",
			"slot", slotID,
			"seq", seqID,
			"id", jobID,
			"active_streams", remaining,
		)
	}()

	var elapsed time.Duration
	if !s.startTime.IsZero() {
		elapsed = time.Since(s.startTime)
	}

	// IMC dedicated slot mode: trim generated tokens but keep cached prefix.
	// Non-IMC mode: clear the entire sequence.
	if e.model.cfg.IncrementalCache && s.job.imcID != "" {
		e.model.cacheMu.RLock()
		session, exists := e.model.imcSessions[s.job.imcID]
		var trimPos llama.Pos
		if exists {
			trimPos = llama.Pos(session.totalTokensCached)
		}
		e.model.cacheMu.RUnlock()

		if trimPos > 0 {
			llama.MemorySeqRm(e.model.mem, s.seqID, trimPos, -1)
			e.model.log(ctx, "finish-slot", "status", "imc-trim", "slot", slotID, "seq", seqID, "trim_pos", trimPos)
		}
	} else {
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
	}

	// Handle error case.
	if err != nil {
		usage := Usage{
			PromptTokens:     s.nPrompt,
			ReasoningTokens:  s.reasonTokens,
			CompletionTokens: s.completionTokens,
			OutputTokens:     s.reasonTokens + s.completionTokens,
			TotalTokens:      s.nPrompt + s.reasonTokens + s.completionTokens,
		}

		e.model.sendErrorResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, "", err, usage)

		return
	}

	// Flush any remaining buffered UTF-8 bytes into the final accumulators.
	// Only emit complete codepoints; drop any trailing incomplete sequence
	// to avoid injecting replacement characters into the final response.
	if len(s.utf8Buf) > 0 {
		complete, _ := extractCompleteUTF8(s.utf8Buf)
		if len(complete) > 0 {
			leftover := string(complete)
			switch {
			case s.reasonFlag > 0:
				s.finalReasoning.WriteString(leftover)
			case s.toolFlag > 0:
				s.finalTooling.WriteString(leftover)
			default:
				s.finalContent.WriteString(leftover)
			}
		}
		s.utf8Buf = s.utf8Buf[:0]
	}

	// Process tool calls if any. Token counts are already tracked
	// per-token in processSlotToken, so no re-tokenization needed.
	if s.toolFlag > 0 {
		content := strings.TrimSuffix(s.finalTooling.String(), "\n")
		if len(content) > 0 {
			switch {
			case e.model.modelInfo.IsGPTModel:
				s.respToolCalls = parseGPTToolCall(content)

			default:
				s.respToolCalls = parseToolCall(content)
			}
		}
	}

	// Calculate final metrics.
	outputTokens := s.reasonTokens + s.completionTokens
	totalTokens := s.nPrompt + outputTokens

	var tokensPerSecond float64
	if elapsed.Seconds() > 0 {
		tokensPerSecond = float64(outputTokens) / elapsed.Seconds()
	}

	usage := Usage{
		PromptTokens:     s.nPrompt,
		ReasoningTokens:  s.reasonTokens,
		CompletionTokens: s.completionTokens,
		OutputTokens:     outputTokens,
		TotalTokens:      totalTokens,
		TokensPerSecond:  tokensPerSecond,
	}

	// Add span attributes and end span.
	s.span.SetAttributes(
		attribute.Int("prompt_tokens", s.nPrompt),
		attribute.Int("reasoning_tokens", s.reasonTokens),
		attribute.Int("completion_tokens", s.completionTokens),
		attribute.Int("output_tokens", outputTokens),
		attribute.Int("total_tokens", totalTokens),
		attribute.Float64("tokens_per_second", tokensPerSecond),
	)

	// Add metrics.
	metrics.AddChatCompletionsUsage(e.model.modelInfo.ID, s.nPrompt, s.reasonTokens, s.completionTokens, outputTokens, totalTokens, tokensPerSecond)

	// Send final response.
	returnPrompt := ""
	if s.job.params.ReturnPrompt {
		returnPrompt = s.job.prompt
	}

	e.model.sendFinalResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, returnPrompt,
		&s.finalContent, &s.finalReasoning, s.respToolCalls, s.logprobsData, s.job.params.Stream, usage)

	e.model.log(ctx, "batch-engine", "status", "slot-finished", "slot", s.id, "id", s.job.id,
		"total_prompt", s.nPrompt, "output_tokens", outputTokens, "time", elapsed.String())
}

func (e *batchEngine) freeSlotResources(s *slot) {
	if s.sampler != 0 {
		llama.SamplerFree(s.sampler)
		s.sampler = 0
	}

	if s.grammarSampler != nil {
		s.grammarSampler.Free()
		s.grammarSampler = nil
	}

	// Free MTMD resources.
	if s.inputChunks != 0 {
		mtmd.InputChunksFree(s.inputChunks)
		s.inputChunks = 0
	}

	for _, b := range s.bitmaps {
		if b != 0 {
			mtmd.BitmapFree(b)
		}
	}
	s.bitmaps = nil

	// Free mtmdCtx from the job if present.
	if s.job != nil && s.job.mtmdCtx != 0 {
		mtmd.Free(s.job.mtmdCtx)
		s.job.mtmdCtx = 0
	}
}

// extractCompleteUTF8 separates a byte slice into complete UTF-8 codepoints
// and any trailing bytes that form an incomplete (but valid prefix of a)
// multi-byte sequence. This handles multi-byte characters (like emoji) that
// get split across multiple BPE tokens.
//
// Bytes that can never form valid UTF-8 (lone continuation bytes, overlong
// encodings, etc.) are passed through in complete rather than buffered
// indefinitely.
func extractCompleteUTF8(b []byte) (complete []byte, remainder []byte) {
	if utf8.Valid(b) {
		return b, nil
	}

	n := len(b)
	i := n

	for i > 0 {
		i--
		c := b[i]

		if c < 0x80 {
			break
		}

		if c&0xC0 != 0x80 {
			var expected int
			switch {
			case c&0xE0 == 0xC0:
				expected = 2
			case c&0xF0 == 0xE0:
				expected = 3
			case c&0xF8 == 0xF0:
				expected = 4
			default:
				break
			}

			have := n - i
			if expected > 0 && have < expected {
				return b[:i], b[i:]
			}

			break
		}
	}

	return b, nil
}

// slotCancelError returns an appropriate error for a cancelled slot.
// Uses context error if available, otherwise returns a shutdown error.
func (e *batchEngine) slotCancelError(s *slot) error {
	if err := s.job.ctx.Err(); err != nil {
		return err
	}
	return errors.New("engine shutting down")
}

// drainSlots finishes all active slots and pending jobs during shutdown.
func (e *batchEngine) drainSlots() {
	ctx := context.Background()

	activeCount := 0
	for _, s := range e.slots {
		if s.active {
			activeCount++
		}
	}

	pendingCount := len(e.requestQ)

	e.model.log(ctx, "batch-engine", "status", "drain-started", "active_slots", activeCount, "pending_jobs", pendingCount)

	for _, s := range e.slots {
		if s.active {
			e.finishSlot(s, fmt.Errorf("drain-slots: engine shutting down"))
		}
	}

	// Drain pending jobs that were never assigned to a slot.
	drained := 0
	for {
		select {
		case job := <-e.requestQ:
			if job.queueWaitSpan != nil {
				job.queueWaitSpan.End()
			}

			if job.mtmdCtx != 0 {
				mtmd.Free(job.mtmdCtx)
			}

			close(job.ch)
			e.model.activeStreams.Add(-1)
			drained++

		default:
			e.model.log(ctx, "batch-engine", "status", "drain-finished", "drained_pending", drained)
			return
		}
	}
}

// logDecodeError logs detailed KV cache diagnostics when decode fails.
func (e *batchEngine) logDecodeError(ctx context.Context, ret int32, err error) {
	nCtx := llama.NCtx(e.model.lctx)

	// Collect per-slot diagnostics.
	var totalTokens llama.Pos
	slotInfo := make([]string, 0, e.nSlots+1)

	// Check system prompt cache (seq 0).
	if sysMax, sysErr := llama.MemorySeqPosMax(e.model.mem, 0); sysErr == nil && sysMax >= 0 {
		slotInfo = append(slotInfo, fmt.Sprintf("sys[0]=%d", sysMax+1))
		totalTokens += sysMax + 1
	}

	// Check each slot's sequence.
	for _, s := range e.slots {
		if !s.active {
			continue
		}
		posMax, posErr := llama.MemorySeqPosMax(e.model.mem, s.seqID)
		if posErr == nil && posMax >= 0 {
			tokens := posMax + 1
			slotInfo = append(slotInfo, fmt.Sprintf("slot[%d,seq=%d]=%d", s.id, s.seqID, tokens))
			totalTokens += tokens
		}
	}

	e.model.log(ctx, "batch-engine",
		"status", "decode-error",
		"ret", ret,
		"err", err,
		"n_ctx", nCtx,
		"kv_used", totalTokens,
		"batch_tokens", e.batch.NTokens,
		"active_slots", len(slotInfo),
		"slot_usage", strings.Join(slotInfo, ","),
	)
}

// decodeError returns a human-readable error message for llama_decode return codes.
// Return codes from llama.cpp:
//
//	0  - success
//	1  - could not find a KV slot for the batch (try reducing batch size or increase context)
//	2  - aborted
//	-1 - invalid input batch
//	<-1 - fatal error
func decodeError(ret int32, err error) error {
	var msg string
	switch ret {
	case 1:
		msg = "unable to process request: the context window is full. Please reduce the input size or increase the context window"
	case 2:
		msg = "request was cancelled"
	case -1:
		msg = "unable to process request: the input could not be processed. Please try reducing the input size or context length"
	default:
		switch {
		case ret < -1:
			msg = "an internal error occurred while processing your request"
		default:
			msg = "an unexpected error occurred while processing your request"
		}
	}

	if err != nil {
		return fmt.Errorf("%s: %w", msg, err)
	}
	return errors.New(msg)
}

// =============================================================================
// MTMD BATCH DECODE HELPERS
// =============================================================================

// decodeTextMRoPE decodes text tokens for M-RoPE models.
// M-RoPE uses 4D positions: [dim0, dim1, dim2, dim3] where each dimension has
// n_tokens entries. For text: dim0=linear position, dims1-3=0.
func (e *batchEngine) decodeTextMRoPE(s *slot, tokens []llama.Token) error {
	n := int32(len(tokens))
	if n == 0 {
		return nil
	}

	batch := llama.BatchInit(n, 0, 1)

	// Save original pos pointer so BatchFree doesn't free Go memory.
	origPos := batch.Pos

	// Copy tokens to batch.
	tokenSlice := unsafeSlice(batch.Token, int(n))
	copy(tokenSlice, tokens)

	// Allocate 4D position array for M-RoPE.
	posData := make([]llama.Pos, n*4)
	pos0 := s.nPast
	for i := range n {
		posData[i] = pos0 + llama.Pos(i) // dim 0: linear position
		posData[i+n] = 0                 // dim 1: 0 for text
		posData[i+n*2] = 0               // dim 2: 0 for text
		posData[i+n*3] = 0               // dim 3: 0 for text
	}
	batch.Pos = &posData[0]

	nSeqIDSlice := unsafeSlice(batch.NSeqId, int(n))
	seqIDPtrs := unsafeSlice(batch.SeqId, int(n))
	logitsSlice := unsafeSlice(batch.Logits, int(n))

	for i := range n {
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = s.seqID
		logitsSlice[i] = 0
	}

	if n > 0 {
		logitsSlice[n-1] = 1
	}

	batch.NTokens = n

	e.model.decodeMu.Lock()
	ret, err := llama.Decode(e.model.lctx, batch)
	e.model.decodeMu.Unlock()

	// Restore and free.
	batch.Pos = origPos
	llama.BatchFree(batch)

	if err != nil || ret != 0 {
		return decodeError(ret, err)
	}

	s.nPast += llama.Pos(n)
	return nil
}

// decodeEmbeddingsNormal decodes image embeddings with standard linear positioning.
// Used for non-M-RoPE models where positions are simply sequential integers.
func (e *batchEngine) decodeEmbeddingsNormal(s *slot, embd []float32, nEmbd, nTokens int32) error {
	batch := llama.BatchInit(nTokens, nEmbd, 1)
	defer llama.BatchFree(batch)

	embdSlice := unsafeSlice(batch.Embd, int(nTokens*nEmbd))
	copy(embdSlice, embd)

	posSlice := unsafeSlice(batch.Pos, int(nTokens))
	nSeqIDSlice := unsafeSlice(batch.NSeqId, int(nTokens))
	seqIDPtrs := unsafeSlice(batch.SeqId, int(nTokens))
	logitsSlice := unsafeSlice(batch.Logits, int(nTokens))

	for i := range nTokens {
		posSlice[i] = s.nPast + llama.Pos(i)
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = s.seqID
		logitsSlice[i] = 0
	}

	if nTokens > 0 {
		logitsSlice[nTokens-1] = 1
	}

	batch.NTokens = nTokens

	e.model.decodeMu.Lock()
	if s.useNonCausal {
		llama.SetCausalAttn(e.model.lctx, false)
	}
	ret, err := llama.Decode(e.model.lctx, batch)
	if s.useNonCausal {
		llama.SetCausalAttn(e.model.lctx, true)
	}
	e.model.decodeMu.Unlock()

	if err != nil || ret != 0 {
		return decodeError(ret, err)
	}

	s.nPast += llama.Pos(nTokens)
	return nil
}

// decodeEmbeddingsMRoPE decodes image embeddings with M-RoPE 2D positioning.
// For M-RoPE, positions are laid out as 4 contiguous arrays:
//
//	[dim0: n_tokens] [dim1: n_tokens] [dim2: n_tokens] [dim3: n_tokens]
//
// For an image grid of nx columns × ny rows:
//   - dim0 (temporal): pos_0 for all tokens
//   - dim1 (row/y):    pos_0 + y
//   - dim2 (col/x):    pos_0 + x
//   - dim3 (unused):   0
//
// CRITICAL: Position advancement after the image must be max(nx, ny), not n_tokens,
// to avoid position overlap with subsequent text tokens.
func (e *batchEngine) decodeEmbeddingsMRoPE(s *slot, embd []float32, nEmbd, nTokens int32, nx, ny int32) error {
	// For M-RoPE, we need 4x the position slots (4D positions).
	nPosPerEmbd := int32(4)

	batch := llama.BatchInit(nTokens, nEmbd, 1)

	embdSlice := unsafeSlice(batch.Embd, int(nTokens*nEmbd))
	copy(embdSlice, embd)

	// Save original pos pointer so BatchFree doesn't try to free Go memory.
	origPos := batch.Pos

	// Allocate our own position array for M-RoPE (4D).
	posData := make([]llama.Pos, nTokens*nPosPerEmbd)

	// Set up 2D M-RoPE positions for image grid.
	pos0 := s.nPast
	for y := range ny {
		for x := range nx {
			i := y*nx + x
			if i >= nTokens {
				break
			}
			// dim 0: constant pos_0 (temporal/base position)
			posData[i] = pos0
			// dim 1: y position (row)
			posData[i+nTokens] = pos0 + llama.Pos(y)
			// dim 2: x position (column)
			posData[i+nTokens*2] = pos0 + llama.Pos(x)
			// dim 3: unused (always 0)
			posData[i+nTokens*3] = 0
		}
	}
	batch.Pos = &posData[0]

	nSeqIDSlice := unsafeSlice(batch.NSeqId, int(nTokens))
	seqIDPtrs := unsafeSlice(batch.SeqId, int(nTokens))
	logitsSlice := unsafeSlice(batch.Logits, int(nTokens))

	for i := range nTokens {
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = s.seqID
		logitsSlice[i] = 0
	}

	if nTokens > 0 {
		logitsSlice[nTokens-1] = 1
	}

	batch.NTokens = nTokens

	e.model.decodeMu.Lock()
	if s.useNonCausal {
		llama.SetCausalAttn(e.model.lctx, false)
	}

	ret, err := llama.Decode(e.model.lctx, batch)
	if s.useNonCausal {
		llama.SetCausalAttn(e.model.lctx, true)
	}

	e.model.decodeMu.Unlock()

	// Restore original pos pointer before freeing to avoid freeing Go memory.
	batch.Pos = origPos
	llama.BatchFree(batch)

	if err != nil || ret != 0 {
		return decodeError(ret, err)
	}

	// For M-RoPE, n_pos is max(nx, ny) to avoid position overlap.
	nPos := max(ny, nx)
	s.nPast += llama.Pos(nPos)

	return nil
}

// sampleFirstToken samples the first output token after prefill completes.
// This is called when the last chunk used a separate decode path (M-RoPE text
// or image embeddings) and nothing was added to the shared batch.
// Returns false if slot finished (EOG or error), true otherwise.
func (e *batchEngine) sampleFirstToken(s *slot, buf []byte) bool {
	// Sample from last logits position (-1).
	var token llama.Token
	switch {
	case s.grammarSampler != nil:
		token = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, -1)

	default:
		token = llama.SamplerSample(s.sampler, e.model.lctx, -1)
	}

	// Process through full pipeline (logprobs, accept, stream, count).
	// This may call finishSlot on EOG/error/maxTokens.
	wasActive := s.active
	e.handleSampledToken(s, token, -1, buf)

	// Return false if slot was finished by handleSampledToken.
	return s.active == wasActive && s.active
}

// unsafeSlice creates a Go slice from a C pointer. This is used to access
// batch arrays allocated by llama.cpp.
func unsafeSlice[T any](ptr *T, length int) []T {
	if ptr == nil || length <= 0 {
		return nil
	}
	return unsafe.Slice(ptr, length)
}

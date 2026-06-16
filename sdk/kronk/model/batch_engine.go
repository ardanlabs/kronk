package model

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

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

	// pendingJobs holds jobs that were dequeued from requestQ but couldn't
	// be assigned to a slot yet (e.g., all slots busy, media slot occupied).
	// Checked before reading requestQ in fillSlots.
	pendingJobs []*chatJob

	// Pre-allocated M-RoPE batch and position buffer for vision model text
	// chunks. Avoids per-call BatchInit/BatchFree and posData allocation in
	// decodeTextMRoPE.
	mropeBatch    llama.Batch
	mropeOrigPos  *llama.Pos
	mropePosData  []llama.Pos
	mropeHasBatch bool
}

// newBatchEngine creates a new batch engine for parallel inference.
func newBatchEngine(m *Model, nSlots int) *batchEngine {
	// Create batch buffer.
	nCtx := llama.NCtx(m.lctx)
	batch := llama.BatchInit(int32(nCtx), 0, int32(nSlots))

	// Initialize slots. Each slot owns a state machine instance produced
	// by the model's parser plugin. State machines are stateful
	// per-slot — never share one across slots.
	slots := make([]*slot, nSlots)
	for i := range slots {
		seqID := llama.SeqId(i)
		slots[i] = &slot{
			id:           i,
			seqID:        seqID,
			seqIDs:       []llama.SeqId{seqID}, // Pre-allocate for batchAdd
			specAccEMA:   1.0,                  // Start optimistic for adaptive draft sizing
			stateMachine: m.parser.NewStateMachine(),
		}
	}

	e := batchEngine{
		model:      m,
		nSlots:     nSlots,
		slots:      slots,
		batch:      batch,
		requestQ:   make(chan *chatJob, nSlots*2),
		wakeCh:     make(chan struct{}, 1),
		shutdownCh: make(chan struct{}),
	}

	// Pre-allocate M-RoPE batch for vision model text chunk decoding.
	nBatch := m.cfg.NBatch()
	if nBatch > 0 {
		e.mropeBatch = llama.BatchInit(int32(nBatch), 0, 1)
		e.mropeOrigPos = e.mropeBatch.Pos
		e.mropePosData = make([]llama.Pos, nBatch*4)
		e.mropeHasBatch = true
		m.log(context.Background(), "batch-engine", "status", "mrope-batch-alloc", "nbatch", nBatch)
	}

	return &e
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

	if e.mropeHasBatch {
		e.mropeBatch.Pos = e.mropeOrigPos
		llama.BatchFree(e.mropeBatch)
		e.mropeHasBatch = false
	}
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

		switch e.hasActiveSlots() || len(e.requestQ) > 0 || len(e.pendingJobs) > 0 {
		case true:
			e.processBatch(ctx, buf)
			timer.Reset(activeInterval)

		case false:
			timer.Reset(idleInterval)
		}
	}
}

// processBatch handles one iteration of the batch processing loop.
func (e *batchEngine) processBatch(ctx context.Context, buf []byte) {
	// Clear the batch.
	e.batch.Clear()

	// MTP: every iteration starts with no slot having claimed a target
	// batch range. The add sites (prefill / gen / spec) re-claim it as
	// they push to e.batch, and the post-decode mirror step consumes it.
	mtpDraft := e.model.draft != nil && e.model.draft.mtp()
	if mtpDraft {
		for _, s := range e.slots {
			s.mtpHasBatch = false
			s.targetBatchCount = 0
		}
	}

	// Prefill draft model for slots that just completed target prefill.
	// Use the slot's per-request job context so log entries carry the
	// current request UUID instead of the long-running batch loop's UUID,
	// which is stale once a slot is reused for a new request.
	if e.model.draft != nil {
		for _, s := range e.slots {
			if !s.active || !s.prefillDone || !s.draftPrefillNeeded {
				continue
			}

			if err := e.prefillDraft(s.job.ctx, s); err != nil {
				e.finishSlot(s, err)
			}
		}
	}

	// Add generation tokens first. Each slot that has completed prefill needs
	// exactly 1 token in the batch. Adding these before prefill chunks ensures
	// addPrefillChunk sees the correct available space and won't overflow.
	for _, s := range e.slots {
		if !s.active || !s.prefillDone {
			continue
		}

		// Check if client cancelled.
		if s.job.ctx.Err() != nil {
			e.finishSlot(s, s.job.ctx.Err())
			continue
		}

		// M-RoPE slots require 4D positions (dim0=linear, dims1-3=0 for text).
		// The shared batch only writes 1D positions via batch.Add, so decode
		// the generation token through the dedicated M-RoPE path and sample
		// from the last logits position (-1) of the M-RoPE batch.
		if s.useMRoPE {
			if err := e.decodeTextMRoPE(s, []llama.Token{s.sampled}); err != nil {
				e.finishSlot(s, fmt.Errorf("mrope generation decode: %w", err))
				continue
			}

			var token llama.Token
			switch {
			case s.grammarSampler != nil:
				token = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, -1)
			default:
				token = llama.SamplerSample(s.sampler, e.model.lctx, -1)
			}
			e.handleSampledToken(s, token, -1, buf)
			continue
		}

		// Speculative decoding: generate draft tokens and add them all
		// to the shared batch for verification in a single forward pass.
		// Only for text slots that completed draft prefill (draftNPast > 0).
		// MTP path is additionally skipped when mtpDisabledForRequest
		// (IMC cache-hit, see batch_slot_start.go).
		mtpUsable := e.model.draft != nil && e.model.draft.mtp() && !s.mtpDisabledForRequest
		canSpec := e.model.draft != nil && !s.draftPrefillNeeded && s.draftNPast > 0 &&
			(!e.model.draft.mtp() || mtpUsable)
		if canSpec {
			// Per-mode dispatch: classic uses token-only drafting, MTP
			// uses the hidden-state head. Adding a new strategy means
			// implementing generate, not editing this call site.
			draftTokens := e.model.draft.generate(e, s)
			if len(draftTokens) > 0 {
				s.specBasePast = s.nPast
				s.specBaseBatch = e.batch.NTokens
				s.specDraftTokens = draftTokens

				// Add s.sampled + all draft tokens with logits=true.
				e.batch.Add(s.sampled, s.nPast, s.seqIDs, true)
				for i, tok := range draftTokens {
					e.batch.Add(tok, s.nPast+llama.Pos(1+i), s.seqIDs, true)
				}

				// MTP: claim the slot's range in the target batch so the
				// post-verify mirror knows where the pre-norm rows live.
				// targetBatchCount is the FULL spec range (1+nDraft);
				// verifySpec rewrites it to 1+accepted before mirroring.
				if mtpDraft {
					s.targetBatchStart = s.specBaseBatch
					s.targetBatchBasePos = s.specBasePast
					s.targetBatchCount = int32(1 + len(draftTokens))
					s.mtpHasBatch = true
				}

				// Hybrid target models need a state snapshot BEFORE the
				// spec decode so verifySpeculativeTokens can roll back
				// a partial rejection. MemorySeqRm can trim the
				// transformer KV but cannot rewind the per-sequence
				// recurrent state; without snapshot/restore the next
				// llama_decode fails with -1 on the leftover advanced
				// recurrent state. No-op for dense / pure-attention
				// targets (those rollback fine via MemorySeqRm).
				if e.model.modelInfo.Type == ModelTypeHybrid {
					if err := e.captureTargetSpecSnapshot(s); err != nil {
						e.model.log(s.job.ctx, "speculative", "status", "snapshot-error",
							"slot", s.id, "err", err)
						// Clear specSnapshot length so verify falls back
						// to MemorySeqRm (which will likely fail on a
						// partial reject, but full-accept rounds still work).
						s.specSnapshot = s.specSnapshot[:0]
					}
				}

				// Don't advance nPast here — verification handles it.
				s.iBatch = -1
				continue
			}
		}

		s.iBatch = e.batch.NTokens
		// MTP: claim the slot's single-token range in the target batch.
		if mtpDraft {
			s.targetBatchStart = s.iBatch
			s.targetBatchBasePos = s.nPast
			s.targetBatchCount = 1
			s.mtpHasBatch = true
		}
		e.batch.Add(s.sampled, s.nPast, s.seqIDs, true)
		s.nPast++
	}

	// Continue prefill for text-only slots using round-robin allocation.
	// Pull NUBatch tokens from each slot in turn to prevent one slot from
	// starving others by consuming the entire tray.
	//
	// MTP correctness constraint: addPrefillChunk records a slot's pre-norm
	// range as a single (targetBatchStart, targetBatchCount) tuple, which
	// assumes the slot's rows in e.batch are contiguous. If two or more
	// slots are prefilling and the outer loop runs a second pass, the
	// rows of any one slot become interleaved with another slot's rows
	// and the mirror step would copy the wrong pre-norm hidden states
	// into the draft KV. To keep the recorded range contiguous, cap the
	// outer loop at one pass when MTP is active and more than one slot
	// is prefilling. Single-slot prefill keeps looping (rows are
	// trivially contiguous), so single-request prefill throughput is
	// unaffected.
	chunkLimit := e.model.cfg.NUBatch()
	mtpLimitOnePass := false
	if mtpDraft {
		nPrefillSlots := 0
		for _, s := range e.slots {
			if s.active && s.prefillTokens != nil {
				nPrefillSlots++
				if nPrefillSlots > 1 {
					mtpLimitOnePass = true
					break
				}
			}
		}
	}
	for {
		before := e.batch.NTokens
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
			if !e.addPrefillChunk(s, chunkLimit) {
				e.finishSlot(s, e.slotCancelError(s))
				continue
			}
		}

		// Stop when no tokens were added (all slots done or tray full).
		if e.batch.NTokens == before {
			break
		}

		// MTP multi-slot prefill: only one pass to keep each slot's rows
		// contiguous in e.batch (see comment above).
		if mtpLimitOnePass {
			break
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

	// Fill empty slots from queue.
	e.fillSlots(buf)

	// Nothing to process.
	if e.batch.NTokens == 0 {
		return
	}

	// Defensive check: batch tokens must not exceed NBatch.
	nBatch := e.model.cfg.NBatch()
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
	if err == nil && ret == 0 {
		llama.Synchronize(e.model.lctx)
	}
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

	// Post-decode is dispatched in up to three passes. Pass 1 is the
	// universal "sample every non-spec slot" loop that runs in EVERY
	// scenario (no drafter, separate-GGUF spec, single-slot MTP,
	// multi-slot MTP). Passes 2A and 2B are spec-only — they are
	// no-ops when no slot drafted this round, which is always the
	// case for non-spec models.
	//
	// Pass 1 — Non-spec slots: optional MTP mirror (gated by
	//          mtpDraft && s.mtpHasBatch && !s.mtpDisabledForRequest)
	//          + processSlotToken. Sole post-decode work for non-spec
	//          models. The target logit buffer is fully intact here.
	//
	// Pass 2A — Spec slots only (s.specDraftTokens != nil), Phase A
	//          (verifySpeculativeTokens): read target logits, accept /
	//          reject drafts, stream accepted drafts, sample the bonus
	//          token. Strictly read-only on the target logit buffer.
	//
	// Pass 2B — Spec slots with s.specPendingFinalize, Phase B
	//          (finalizeSpeculativeTokens): rollback (incl. hybrid
	//          restore, which can wipe target logits), draft KV
	//          rollback, MTP mirror, set s.nPast, stream bonus token.
	//
	// Why the 2A/2B split exists (the MTP + hybrid multi-slot fix):
	// on a hybrid target (transformer + recurrent layers), a partial-
	// rejection spec round runs restoreTargetSpecSnapshot in Phase B,
	// which RE-DECODES the accepted prefix on the target context.
	// That re-decode replaces the target's per-context logit buffer
	// with logits for only the re-decoded rows — every other batch
	// row's logits are invalidated. With nseq-max > 1, if one spec
	// slot's Phase B ran before another spec slot's verify read its
	// logits, the second slot would crash in llama_sampler_sample
	// (GGML_ASSERT logits != nullptr at llama-sampler.cpp:850).
	// Running every spec slot's Phase A first (Pass 2A) and every
	// Phase B afterwards (Pass 2B) keeps multi-slot MTP + hybrid
	// safe. Under nseq-max == 1 the ordering Phase A → Phase B for a
	// single spec slot is functionally identical to the old
	// monolithic verifySpeculativeTokens, so this split has no
	// behavioral effect on the single-slot path.

	// Pass 1: sample for all non-spec slots.
	for _, s := range e.slots {
		if !s.active {
			continue
		}
		if s.specDraftTokens != nil {
			continue
		}

		// MTP non-spec path (prefill chunk or plain gen): mirror BEFORE
		// processSlotToken because handleSampledToken may finishSlot →
		// reset(), which would clear mtpHasBatch. Mirror only needs the
		// just-decoded target pre-norm buffer (still valid here). Skip
		// for MTP-disabled requests (IMC hit) — without the mirror
		// pendingH stays empty and generateDraftTokensMTP short-circuits,
		// so we'd just be paying decode cost for nothing.
		if mtpDraft && s.mtpHasBatch && !s.mtpDisabledForRequest {
			if err := e.mirrorTargetBatchToMTPDraft(s, int(s.targetBatchCount)); err != nil {
				e.model.log(s.job.ctx, "speculative", "status", "mtp-mirror-error",
					"slot", s.id, "err", err)
				e.finishSlot(s, fmt.Errorf("mtp mirror: %w", err))
				continue
			}
		}

		if s.iBatch < 0 {
			continue
		}

		e.processSlotToken(s, buf)
	}

	// Pass 2A: Phase A of speculative verification for every spec slot
	// — read target logits, decide accept/reject, sample bonus token.
	// Strictly read-only on the target context's logit buffer, so
	// running this for all spec slots before any Phase B is safe.
	for _, s := range e.slots {
		if !s.active {
			continue
		}
		if s.specDraftTokens == nil {
			continue
		}
		e.verifySpeculativeTokens(s, buf)
	}

	// Pass 2B: Phase B of speculative verification — rollback, hybrid
	// restore, draft KV rollback, MTP mirror, set s.nPast, stream the
	// bonus token. finalizeSpeculativeTokens is a no-op for slots that
	// short-circuited in Phase A (EOG) or that aren't pending.
	for _, s := range e.slots {
		if !s.active {
			continue
		}
		if !s.specPendingFinalize {
			continue
		}
		e.finalizeSpeculativeTokens(s, buf)
	}
}

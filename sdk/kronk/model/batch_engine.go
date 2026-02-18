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
}

// newBatchEngine creates a new batch engine for parallel inference.
func newBatchEngine(m *Model, nSlots int) *batchEngine {
	// Create batch buffer.
	nCtx := llama.NCtx(m.lctx)
	batch := llama.BatchInit(int32(nCtx), 0, int32(nSlots))

	// Initialize slots.
	slots := make([]*slot, nSlots)
	for i := range slots {
		seqID := llama.SeqId(i)
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

// processBatch handles one iteration of the batch processing loop.
func (e *batchEngine) processBatch(ctx context.Context, buf []byte) {
	// Clear the batch.
	e.batch.Clear()

	// Prefill draft model for slots that just completed target prefill.
	if e.model.draft != nil {
		for _, s := range e.slots {
			if !s.active || !s.prefillDone || !s.draftPrefillNeeded {
				continue
			}

			if err := e.prefillDraft(ctx, s); err != nil {
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

		// Speculative decoding: generate draft tokens and add them all
		// to the shared batch for verification in a single forward pass.
		// Only for text slots that completed draft prefill (draftNPast > 0).
		if e.model.draft != nil && !s.draftPrefillNeeded && s.draftNPast > 0 {
			draftTokens := e.generateDraftTokens(s)
			if len(draftTokens) > 0 {
				s.specBasePast = s.nPast
				s.specBaseBatch = e.batch.NTokens
				s.specDraftTokens = draftTokens

				// Add s.sampled + all draft tokens with logits=true.
				e.batch.Add(s.sampled, s.nPast, s.seqIDs, true)
				for i, tok := range draftTokens {
					e.batch.Add(tok, s.nPast+llama.Pos(1+i), s.seqIDs, true)
				}

				// Don't advance nPast here — verification handles it.
				s.iBatch = -1
				continue
			}
		}

		s.iBatch = e.batch.NTokens
		e.batch.Add(s.sampled, s.nPast, s.seqIDs, true)
		s.nPast++
	}

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

	// Verify speculative tokens or sample normally for each active slot.
	for _, s := range e.slots {
		if !s.active {
			continue
		}

		// Speculative path: verify draft tokens against target predictions.
		if s.specDraftTokens != nil {
			e.verifySpeculativeTokens(s, buf)
			continue
		}

		if s.iBatch < 0 {
			continue
		}

		e.processSlotToken(s, buf)
	}
}

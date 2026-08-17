package model

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// batchEngine manages parallel generation inference slots.
type batchEngine struct {
	model       *Model
	nSlots      int
	slots       []*slot
	batch       llama.Batch
	requestQ    chan *chatJob
	wakeCh      chan struct{}
	admissionCh chan struct{}
	shutdownCh  chan struct{}
	admissionMu sync.RWMutex
	loopDone    chan struct{}
	cleanupOnce sync.Once
	stopped     atomic.Bool

	// pendingJobs holds jobs that were dequeued from requestQ but couldn't
	// be assigned to a slot yet (e.g., all slots busy, media slot occupied).
	// Checked before reading requestQ in fillSlots.
	pendingJobs []*chatJob

	// batchReleased quarantines slots released after the current shared batch
	// starts assembling. Their staged rows remain in batch until decode, so the
	// slot's stable seqID must not be reassigned in the same iteration. After
	// decode, those sequences are cleared again to remove any staged rows that
	// were written after finishSlot's initial clear.
	batchReleased   []bool
	batchAssembling bool
	batchIteration  uint64
	imcPrepNext     int
	prefillNext     int
	mediaNext       int
	diagnostics     atomic.Pointer[BatchEngineSnapshot]
	speculation     speculation.Controller

	// Diagnostics below are owned by processLoop and copied into diagnostics
	// at batch-loop boundaries for concurrent observers.
	diagnosticPrefillStart    int
	diagnosticPrefillSelected int
	diagnosticIMCStart        int
	diagnosticIMCSelected     int
	diagnosticGenerationRows  int
	diagnosticGeneration      []BatchGenerationContribution
	diagnosticLastPublished   time.Time

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
	nBatch := m.cfg.EffectiveNBatch()
	batch := llama.BatchInit(int32(nBatch), 0, int32(nSlots))

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
			stateMachine: m.parser.NewStateMachine(),
		}
		slots[i].classic.Reset()
	}

	e := batchEngine{
		model:                     m,
		nSlots:                    nSlots,
		slots:                     slots,
		batch:                     batch,
		requestQ:                  make(chan *chatJob, nSlots*m.cfg.QueueDepth()),
		wakeCh:                    make(chan struct{}, 1),
		admissionCh:               make(chan struct{}),
		shutdownCh:                make(chan struct{}),
		loopDone:                  make(chan struct{}),
		batchReleased:             make([]bool, nSlots),
		diagnosticPrefillSelected: -1,
		diagnosticIMCSelected:     -1,
	}
	e.speculation = newSpeculationController(&e)
	e.publishDiagnostics(true)

	// Pre-allocate M-RoPE batch for vision model text chunk decoding.
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
	go e.processLoop(ctx)
	e.model.log(ctx, "batch-engine", "status", "started", "slots", e.nSlots)
}

// stop closes admission, signals shutdown, and waits for completion.
func (e *batchEngine) stop(ctx context.Context) error {
	if !e.stopped.CompareAndSwap(false, true) {
		select {
		case <-e.loopDone:
			e.cleanupSamplers()
			return nil

		case <-ctx.Done():
			return fmt.Errorf("stop batch engine: %w", ctx.Err())
		}
	}

	// Prevent new submissions, then wait for submissions that were already in
	// progress to commit or observe admissionCh. Only after that barrier is it
	// safe to let processLoop drain the request queue.
	close(e.admissionCh)
	e.admissionMu.Lock()
	close(e.shutdownCh)
	e.admissionMu.Unlock()

	select {
	case <-e.loopDone:
		e.cleanupSamplers()

	case <-ctx.Done():
		return fmt.Errorf("stop batch engine: %w", ctx.Err())
	}

	e.model.log(ctx, "batch-engine", "status", "stopped")
	return nil
}

func (e *batchEngine) cleanupSamplers() {
	e.cleanupOnce.Do(func() {
		// Free samplers - batch is freed separately in Unload.
		for _, s := range e.slots {
			if s.sampler != 0 {
				llama.SamplerFree(s.sampler)
				s.sampler = 0
			}
		}
	})
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
	if e.stopped.Load() {
		return fmt.Errorf("submit: engine shutting down")
	}

	e.admissionMu.RLock()
	defer e.admissionMu.RUnlock()

	select {
	case e.requestQ <- job:
		select {
		case e.wakeCh <- struct{}{}:
		default:
		}
		return nil

	case <-e.admissionCh:
		return fmt.Errorf("submit: engine shutting down")

	case <-job.ctx.Done():
		return job.ctx.Err()
	}
}

// processLoop is the main batch processing goroutine. Active work continues
// immediately; when idle, the loop sleeps until submit signals new work.
func (e *batchEngine) processLoop(ctx context.Context) {
	defer close(e.loopDone)

	buf := make([]byte, 32*1024)

	for {
		select {
		case <-e.shutdownCh:
			e.drainSlots()
			return

		default:
		}

		if e.hasActiveSlots() || len(e.requestQ) > 0 || len(e.pendingJobs) > 0 {
			e.processBatch(ctx, buf)
			continue
		}

		select {
		case <-e.shutdownCh:
			e.drainSlots()
			return

		case <-e.wakeCh:
		}
	}
}

// processBatch handles one iteration of the batch processing loop.
func (e *batchEngine) processBatch(ctx context.Context, buf []byte) {
	e.batchIteration++
	iteration := e.batchIteration
	e.diagnosticPrefillStart = e.prefillNext
	e.diagnosticPrefillSelected = -1
	e.diagnosticIMCStart = e.imcPrepNext
	e.diagnosticIMCSelected = -1
	e.diagnosticGenerationRows = 0
	e.diagnosticGeneration = e.diagnosticGeneration[:0]
	defer func() {
		e.publishDiagnostics(!e.hasActiveSlots())
	}()

	// Clear the batch.
	e.batch.Clear()
	for i := range e.batchReleased {
		e.batchReleased[i] = false
	}

	e.speculation.BeginBatch()

	// A slot released after this point is quarantined until the next iteration
	// so two requests can never contribute rows for the same stable seqID to
	// one llama.Decode.
	e.batchAssembling = true

	// Bind queued jobs to every available slot before advancing text IMC
	// preparation. Text builds and extensions return from startSlot without
	// staging shared rows, so requests arriving while another prompt is being
	// prepared can still claim free slots immediately. If another start path did
	// stage rows during admission, defer direct IMC decoding until the next
	// iteration rather than mutating context behind an assembled batch.
	e.fillSlots(buf)
	if e.batch.NTokens == 0 && e.hasIMCPreparation() {
		e.advanceIMCPreparation(buf)
	}

	e.speculation.Prepare()

	// Add generation tokens first. An ordinary slot contributes one row, while
	// a speculative slot can contribute the sampled token plus its draft rows.
	// Adding these before prefill keeps output responsive and lets prefill use
	// only the tray capacity that remains.
	batchRowsBeforeGeneration := e.batch.NTokens
	trackPrefillSchedule := len(e.prefillSlotIDs()) > 0
	var generationContributions []string
	if trackPrefillSchedule {
		generationContributions = make([]string, 0, len(e.slots))
	}
	for _, s := range e.slots {
		if !s.active || !s.prefillDone {
			continue
		}

		// Check if client cancelled.
		if s.job.ctx.Err() != nil {
			e.finishSlot(s, s.job.ctx.Err())
			continue
		}

		if int(s.nPast) >= e.model.cfg.ContextWindow() {
			e.finishSlot(s, fmt.Errorf("generation reached context window of %d tokens", e.model.cfg.ContextWindow()))
			continue
		}

		// A newly admitted request may have filled the logical batch during
		// startSlot. Defer existing generation rows to the next iteration rather
		// than overflowing the batch; the new request's prefill is decoded first.
		if int(e.batch.NTokens) >= e.model.cfg.EffectiveNBatch() {
			s.iBatch = -1
			e.diagnosticGeneration = append(e.diagnosticGeneration, BatchGenerationContribution{
				SlotID: s.id,
				Mode:   "deferred-nbatch",
			})
			if trackPrefillSchedule {
				generationContributions = append(generationContributions,
					fmt.Sprintf("slot=%d,rows=0,mode=deferred-nbatch", s.id))
			}
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

			token := e.sampleSlotToken(s, -1)
			if trackPrefillSchedule {
				generationContributions = append(generationContributions,
					fmt.Sprintf("slot=%d,rows=1,mode=mrope-direct", s.id))
			}
			e.diagnosticGeneration = append(e.diagnosticGeneration, BatchGenerationContribution{
				SlotID: s.id,
				Rows:   1,
				Mode:   "mrope-direct",
			})
			e.handleSampledToken(s, token, -1, buf)
			continue
		}

		generation, err := e.speculation.PlanGeneration(s.id)
		if err != nil {
			e.finishSlot(s, err)
			continue
		}
		if len(generation.Candidates) > 0 {
			batchStart := e.batch.NTokens
			if err := e.batch.Add(s.sampled, s.nPast, s.seqIDs, true); err != nil {
				e.finishSlot(s, fmt.Errorf("add speculative base token: %w", err))
				continue
			}
			addFailed := false
			for i, tok := range generation.Candidates {
				if err := e.batch.Add(tok, s.nPast+llama.Pos(1+i), s.seqIDs, true); err != nil {
					e.batch.NTokens = batchStart
					e.finishSlot(s, fmt.Errorf("add speculative draft token %d: %w", i, err))
					addFailed = true
					break
				}
			}
			if addFailed {
				continue
			}

			targetRange := speculation.TargetRange{
				Start:   batchStart,
				Count:   int32(1 + len(generation.Candidates)),
				BasePos: s.nPast,
			}
			if err := e.speculation.CommitGeneration(s.id, generation.Candidates, targetRange); err != nil {
				e.batch.NTokens = batchStart
				e.finishSlot(s, err)
				continue
			}
			if trackPrefillSchedule {
				generationContributions = append(generationContributions,
					fmt.Sprintf("slot=%d,rows=%d,mode=%s", s.id, targetRange.Count, generation.Mode))
			}
			e.diagnosticGeneration = append(e.diagnosticGeneration, BatchGenerationContribution{
				SlotID: s.id,
				Rows:   int(targetRange.Count),
				Mode:   generation.Mode,
			})
			s.iBatch = -1
			continue
		}

		s.iBatch = e.batch.NTokens
		if err := e.batch.Add(s.sampled, s.nPast, s.seqIDs, true); err != nil {
			e.finishSlot(s, fmt.Errorf("add generation token: %w", err))
			continue
		}
		e.speculation.TargetRowsStaged(s.id, speculation.TargetRange{
			Start:   s.iBatch,
			Count:   1,
			BasePos: s.nPast,
		})
		if trackPrefillSchedule {
			generationContributions = append(generationContributions,
				fmt.Sprintf("slot=%d,rows=1,mode=ordinary", s.id))
		}
		e.diagnosticGeneration = append(e.diagnosticGeneration, BatchGenerationContribution{
			SlotID: s.id,
			Rows:   1,
			Mode:   "ordinary",
		})
		s.nPast++
	}
	generationRows := e.batch.NTokens
	e.diagnosticGenerationRows = int(generationRows - batchRowsBeforeGeneration)

	// Continue ordinary text prefill from one slot. The cursor remains on that
	// owner across decode iterations until its prompt is complete, minimizing
	// time-to-first-token for one request without delaying output rows from slots
	// already generating. Completion advances ownership to the next active
	// prefilling slot. Speculation implementations receive each range as one
	// contiguous contribution.
	prefillSlots := e.prefillSlotIDs()
	selectorStart := e.prefillNext
	s, idx := e.nextPrefillSlot()
	e.diagnosticPrefillStart = selectorStart
	if s != nil && int(e.batch.NTokens) >= e.model.cfg.EffectiveNBatch() {
		e.model.log(s.job.ctx, "batch-engine", "status", "prefill-deferred",
			"iteration", iteration,
			"slot", s.id,
			"prefill_slots", fmt.Sprintf("%v", prefillSlots),
			"generation_contributions", fmt.Sprintf("%v", generationContributions),
			"selector_start", selectorStart,
			"selector_selected", idx,
			"selector_next", e.prefillNext,
			"generation_rows", generationRows,
			"tray_tokens", e.batch.NTokens,
			"nbatch", e.model.cfg.EffectiveNBatch(),
			"nubatch", e.model.cfg.EffectiveNUBatch())
	}
	if s != nil && int(e.batch.NTokens) < e.model.cfg.EffectiveNBatch() {
		beforeSlot := e.batch.NTokens
		if !e.addPrefillChunk(s, e.model.cfg.PrefillBatchSize()) {
			if s.job != nil {
				e.finishSlot(s, e.slotCancelError(s))
			}
		} else if e.batch.NTokens > beforeSlot {
			e.diagnosticPrefillSelected = idx
			prefillComplete := s.prefillTokens == nil
			if prefillComplete {
				e.prefillNext = (idx + 1) % len(e.slots)
			} else {
				e.prefillNext = idx
			}
			e.model.log(s.job.ctx, "batch-engine", "status", "prefill-scheduled",
				"iteration", iteration,
				"slot", s.id,
				"prefill_slots", fmt.Sprintf("%v", prefillSlots),
				"generation_contributions", fmt.Sprintf("%v", generationContributions),
				"chunk_tokens", e.batch.NTokens-beforeSlot,
				"prefill_remaining", max(0, len(s.prefillTokens)-s.nPrefilled),
				"prefill_complete", prefillComplete,
				"selector_start", selectorStart,
				"selector_selected", idx,
				"selector_next", e.prefillNext,
				"next_slot", e.prefillNext,
				"generation_rows", generationRows,
				"tray_tokens", e.batch.NTokens,
				"nbatch", e.model.cfg.EffectiveNBatch(),
				"nubatch", e.model.cfg.EffectiveNUBatch())
		}
	}

	// Process at most one media unit per iteration. Text that can share the
	// tray consumes only capacity left by generation and ordinary prefill.
	// Image/audio and M-RoPE work use separate decode calls, so defer them until
	// after the shared tray has decoded and published generation output.
	mediaSlot, mediaIdx := e.nextMediaSlot()
	if mediaSlot != nil && e.mediaChunkUsesSharedBatch(mediaSlot) {
		e.processMediaSlot(mediaSlot, mediaIdx, buf)
		mediaSlot = nil
	}

	// Nothing to process.
	if e.batch.NTokens == 0 {
		e.batchAssembling = false
		if mediaSlot != nil {
			e.processMediaSlot(mediaSlot, mediaIdx, buf)
		}
		return
	}

	// Publish the assembled tray before native decode so a slow decode remains
	// visible to diagnostics clients while it is in progress.
	e.publishDiagnostics(false)

	// Defensive check: batch tokens must not exceed NBatch.
	nBatch := e.model.cfg.EffectiveNBatch()
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

		e.batchAssembling = false
		return
	}

	// Lock to prevent concurrent decode with cache population.
	e.model.decodeMu.Lock()
	ret, err := llama.Decode(e.model.lctx, e.batch)
	if err == nil && ret == 0 {
		llama.Synchronize(e.model.lctx)
	}
	for i, released := range e.batchReleased {
		if released {
			llama.MemorySeqRm(e.model.mem, e.slots[i].seqID, -1, -1)
		}
	}
	e.model.decodeMu.Unlock()
	e.batchAssembling = false

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

	e.speculation.AfterTargetDecode(buf)

	if mediaSlot != nil {
		e.processMediaSlot(mediaSlot, mediaIdx, buf)
	}
}

func needsTargetSpecSnapshot(modelType ModelType, rollbackDepth uint32, draftCount int) bool {
	return modelType == ModelTypeHybrid && int(rollbackDepth) < draftCount
}

func (e *batchEngine) maxDraftForSlot(s *slot, configured int) int {
	maxDraft := min(configured, e.model.cfg.ContextWindow()-int(s.nPast)-2)

	remainingTokens := s.job.params.MaxTokens - (s.reasonTokens + s.completionTokens)
	if remainingTokens > 0 {
		maxDraft = min(maxDraft, remainingTokens-1)
	}

	return max(maxDraft, 0)
}

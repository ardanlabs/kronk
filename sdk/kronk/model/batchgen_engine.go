package model

import (
	"context"
	"errors"
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
	batch       *extendedBatch
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

	// Pre-allocated extended batch for isolated media work. Unsupported,
	// unknown, non-causal, or capacity-constrained contributions use it.
	mropeBatch *extendedBatch
}

// newBatchEngine creates a new batch engine for parallel inference.
func newBatchEngine(m *Model, nSlots int) (*batchEngine, error) {
	batch, err := newExtendedBatch(m.lctx)
	if err != nil {
		return nil, fmt.Errorf("new batch engine: %w", err)
	}

	mropeBatch, err := newExtendedBatch(m.lctx)
	if err != nil {
		_ = batch.free()
		return nil, fmt.Errorf("new batch engine M-RoPE batch: %w", err)
	}

	// Initialize slots. Each slot owns a state machine instance produced
	// by the model's parser plugin. State machines are stateful
	// per-slot — never share one across slots.
	slots := make([]*slot, nSlots)
	for i := range slots {
		seqID := llama.SeqId(i)
		slots[i] = &slot{
			id:           i,
			seqID:        seqID,
			seqIDs:       []llama.SeqId{seqID}, // Pre-allocate for batch entries.
			stateMachine: m.parser.NewStateMachine(),
		}
		slots[i].classic.Reset()
	}

	e := batchEngine{
		model:                     m,
		nSlots:                    nSlots,
		slots:                     slots,
		batch:                     batch,
		mropeBatch:                mropeBatch,
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

	m.log(context.Background(), "batch-engine", "status", "mrope-batch-alloc", "nbatch", m.cfg.EffectiveNBatch())

	return &e, nil
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
	_ = e.batch.free()
	_ = e.mropeBatch.free()
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
	e.batch.clear()
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
	if e.batch.len() == 0 && e.hasIMCPreparation() {
		e.advanceIMCPreparation(buf)
	}

	e.speculation.Prepare()

	// Add generation tokens first. An ordinary slot contributes one row, while
	// a speculative slot can contribute the sampled token plus its draft rows.
	// Adding these before prefill keeps output responsive and lets prefill use
	// only the tray capacity that remains.
	batchRowsBeforeGeneration := e.batch.len()
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
		if e.batch.len() >= e.model.cfg.EffectiveNBatch() {
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

		// llama.cpp expands a token's linear position into its M-RoPE text
		// position, so M-RoPE generation can use the shared target tray. Media
		// M-RoPE requests do not speculate until draft position compatibility is
		// proven, so stage exactly one ordinary target row for them.
		if s.useMRoPE {
			idx, err := e.batch.addToken(s.sampled, s.nPast, s.seqIDs, extendedBatchOutputLogits)
			if err != nil {
				e.finishSlot(s, fmt.Errorf("add M-RoPE generation token: %w", err))
				continue
			}
			s.iBatch = idx
			e.speculation.TargetRowsStaged(s.id, speculation.TargetRange{
				Start:   s.iBatch,
				Count:   1,
				BasePos: s.nPast,
			})
			if trackPrefillSchedule {
				generationContributions = append(generationContributions,
					fmt.Sprintf("slot=%d,rows=1,mode=mrope-shared", s.id))
			}
			e.diagnosticGeneration = append(e.diagnosticGeneration, BatchGenerationContribution{
				SlotID: s.id,
				Rows:   1,
				Mode:   "mrope-shared",
			})
			s.nPast++
			continue
		}

		generation, err := e.speculation.PlanGeneration(s.id)
		if err != nil {
			e.finishSlot(s, err)
			continue
		}
		if len(generation.Candidates) > 0 {
			batchStart := e.batch.len()
			if _, err := e.batch.addToken(s.sampled, s.nPast, s.seqIDs, extendedBatchOutputLogits); err != nil {
				e.finishSlot(s, fmt.Errorf("add speculative base token: %w", err))
				continue
			}
			addFailed := false
			for i, tok := range generation.Candidates {
				if _, err := e.batch.addToken(tok, s.nPast+llama.Pos(1+i), s.seqIDs, extendedBatchOutputLogits); err != nil {
					rollbackErr := e.batch.truncate(batchStart)
					e.finishSlot(s, fmt.Errorf("add speculative draft token %d: %w", i, errors.Join(err, rollbackErr)))
					addFailed = true
					break
				}
			}
			if addFailed {
				continue
			}

			targetRange := speculation.TargetRange{
				Start:   int32(batchStart),
				Count:   int32(1 + len(generation.Candidates)),
				BasePos: s.nPast,
			}
			if err := e.speculation.CommitGeneration(s.id, generation.Candidates, targetRange); err != nil {
				rollbackErr := e.batch.truncate(batchStart)
				e.finishSlot(s, errors.Join(err, rollbackErr))
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

		idx, err := e.batch.addToken(s.sampled, s.nPast, s.seqIDs, extendedBatchOutputLogits)
		if err != nil {
			e.finishSlot(s, fmt.Errorf("add generation token: %w", err))
			continue
		}
		s.iBatch = idx
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
	generationRows := e.batch.len()
	e.diagnosticGenerationRows = generationRows - batchRowsBeforeGeneration

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
	if s != nil && e.batch.len() >= e.model.cfg.EffectiveNBatch() {
		e.model.log(s.job.ctx, "batch-engine", "status", "prefill-deferred",
			"iteration", iteration,
			"slot", s.id,
			"prefill_slots", fmt.Sprintf("%v", prefillSlots),
			"generation_contributions", fmt.Sprintf("%v", generationContributions),
			"selector_start", selectorStart,
			"selector_selected", idx,
			"selector_next", e.prefillNext,
			"generation_rows", generationRows,
			"tray_tokens", e.batch.len(),
			"nbatch", e.model.cfg.EffectiveNBatch(),
			"nubatch", e.model.cfg.EffectiveNUBatch())
	}
	if s != nil && e.batch.len() < e.model.cfg.EffectiveNBatch() {
		beforeSlot := e.batch.len()
		if !e.addPrefillChunk(s, e.model.cfg.PrefillBatchSize()) {
			if s.job != nil {
				e.finishSlot(s, e.slotCancelError(s))
			}
		} else if e.batch.len() > beforeSlot {
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
				"chunk_tokens", e.batch.len()-beforeSlot,
				"prefill_remaining", max(0, len(s.prefillTokens)-s.nPrefilled),
				"prefill_complete", prefillComplete,
				"selector_start", selectorStart,
				"selector_selected", idx,
				"selector_next", e.prefillNext,
				"next_slot", e.prefillNext,
				"generation_rows", generationRows,
				"tray_tokens", e.batch.len(),
				"nbatch", e.model.cfg.EffectiveNBatch(),
				"nubatch", e.model.cfg.EffectiveNUBatch())
		}
	}

	// Process at most one media contribution per iteration. The planner admits
	// compatible causal text or embedding rows to the shared tray and leaves
	// unsupported, unknown, non-causal, or capacity-constrained work on the
	// existing isolated path.
	mediaSlot, mediaIdx := e.nextMediaSlot()
	var stagedMedia *stagedMediaContribution
	if mediaSlot != nil {
		plan := e.planMediaSlot(mediaSlot)
		e.model.log(mediaSlot.job.ctx, "batch-engine", "status", "media-plan",
			"iteration", iteration,
			"slot", mediaSlot.id,
			"mode", plan.mode.String(),
			"rows", plan.rows,
			"input-mixing", e.model.modelInfo.profile.Batch.InputMixing,
			"mrope", mediaSlot.useMRoPE,
			"non-causal", mediaSlot.useNonCausal)
		switch plan.mode {
		case batchExecutionShared:
			var err error
			stagedMedia, err = e.stageMediaSlot(mediaSlot, plan)
			if err != nil {
				e.finishSlot(mediaSlot, err)
				e.mediaNext = (mediaIdx + 1) % len(e.slots)
			}
			mediaSlot = nil

		case batchExecutionDeferred:
			mediaSlot = nil
		}
	}

	// Nothing to process.
	if e.batch.len() == 0 {
		e.batchAssembling = false
		if mediaSlot != nil {
			e.processIsolatedMediaSlot(mediaSlot, mediaIdx, buf)
		}
		return
	}

	// Publish the assembled tray before native decode so a slow decode remains
	// visible to diagnostics clients while it is in progress.
	e.publishDiagnostics(false)

	// Defensive check: batch tokens must not exceed NBatch.
	nBatch := e.model.cfg.EffectiveNBatch()
	if e.batch.len() > nBatch {
		e.model.log(ctx, "process-batch", "ERROR", "batch-overflow",
			"batch_tokens", e.batch.len(),
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
		overflowErr := fmt.Errorf("process-batch: %d tokens exceeds NBatch limit of %d", e.batch.len(), nBatch)
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
	ret, err := e.batch.process(llama.ProcessTypeDecode)
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
	if stagedMedia != nil && stagedMedia.slot.active {
		stagedMedia.commit()
		e.mediaNext = (mediaIdx + 1) % len(e.slots)
	}

	e.speculation.AfterTargetDecode(buf)

	if mediaSlot != nil {
		e.processIsolatedMediaSlot(mediaSlot, mediaIdx, buf)
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

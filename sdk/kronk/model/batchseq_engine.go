package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
)

var errBatchSeqStopped = errors.New("batchseq engine stopped")

type batchSeqResult struct {
	outputs [][]float32
	err     error
}

type batchSeqJob struct {
	ctx         context.Context
	items       []batchSeqItem
	outputWidth int
	next        int
	outputs     [][]float32
	resultCh    chan batchSeqResult
	once        sync.Once
	queuedAt    time.Time
	started     bool
}

func newBatchSeqJob(ctx context.Context, items []batchSeqItem, outputWidth int) *batchSeqJob {
	return &batchSeqJob{
		ctx:         ctx,
		items:       items,
		outputWidth: outputWidth,
		outputs:     make([][]float32, len(items)),
		resultCh:    make(chan batchSeqResult, 1),
		queuedAt:    time.Now(),
	}
}

func (j *batchSeqJob) complete(outputs [][]float32, err error) {
	j.once.Do(func() {
		j.resultCh <- batchSeqResult{outputs: outputs, err: err}
	})
}

type batchSeqEvaluateFunc func(job *batchSeqJob) (outputs [][]float32, fatal bool, err error)

// batchSeqEngine serializes embedding and reranking sequence batches on
// one llama context. It is intentionally separate from the generation batch
// engine, whose slots retain long-lived generation state. Already queued
// requests are coalesced without an artificial delay and scheduled by item in
// round-robin order.
type batchSeqEngine struct {
	lctx        llama.Context
	mem         llama.Memory // Zero for stateless reranking contexts.
	batch       llama.Batch
	maxSeq      int
	maxTokens   int
	requestQ    chan *batchSeqJob
	admissionCh chan struct{}
	shutdownCh  chan struct{}
	doneCh      chan struct{}
	wg          sync.WaitGroup
	stopped     atomic.Bool
	batchFreed  atomic.Bool
	hasBatch    bool

	submitMu  sync.Mutex
	submitWG  sync.WaitGroup
	accepting bool

	evaluate  batchSeqEvaluateFunc
	modelID   string
	operation string

	errMu sync.RWMutex
	err   error
}

// initBatchSeqRuntime initializes the context and engine used by supported
// embedding and reranking models. The caller retains ownership of m.model.
func initBatchSeqRuntime(ctx context.Context, m *Model) error {
	lctx, err := llama.InitFromModel(m.model, m.ctxParams)
	if err != nil {
		return fmt.Errorf("init-batchseq-runtime: init context: %w", err)
	}

	mem, err := llama.GetMemory(lctx)
	if err != nil {
		return errors.Join(
			fmt.Errorf("init-batchseq-runtime: get memory: %w", err),
			freeBatchSeqContext(lctx),
		)
	}

	if mem != 0 {
		if err := llama.MemoryClear(mem, true); err != nil {
			return errors.Join(
				fmt.Errorf("init-batchseq-runtime: clear memory: %w", err),
				freeBatchSeqContext(lctx),
			)
		}
	}

	engine, err := newBatchSeqEngine(lctx, mem, m.cfg.QueueDepth())
	if err != nil {
		return errors.Join(
			fmt.Errorf("init-batchseq-runtime: %w", err),
			freeBatchSeqContext(lctx),
		)
	}

	if err := m.applyAdapters(lctx); err != nil {
		freeErr := engine.freeBatch()
		contextErr := freeBatchSeqContext(lctx)
		return errors.Join(fmt.Errorf("init-batchseq-runtime: %w", err), freeErr, contextErr)
	}

	m.lctx = lctx
	m.mem = mem
	engine.modelID = m.modelInfo.ID
	if m.modelInfo.IsEmbedModel {
		engine.operation = "embedding"
	} else {
		engine.operation = "rerank"
	}
	m.batchSeq = engine
	m.batchSeq.start()
	m.log(ctx, "batchseq-runtime", "status", "initialized", "sequences", engine.maxSeq, "tokens", engine.maxTokens)

	return nil
}

// freeBatchSeqContext waits for native work before releasing a sequence-batch
// context. Callers must stop the owner goroutine and free its reusable batch
// before calling this function.
func freeBatchSeqContext(lctx llama.Context) error {
	if lctx == 0 {
		return nil
	}

	syncErr := llama.Synchronize(lctx)
	freeErr := llama.Free(lctx)

	return errors.Join(syncErr, freeErr)
}

func newBatchSeqEngine(lctx llama.Context, mem llama.Memory, queueDepth int) (*batchSeqEngine, error) {
	if lctx == 0 {
		return nil, fmt.Errorf("new-batchseq-engine: invalid context")
	}

	maxSeq := int(llama.NSeqMax(lctx))
	if maxSeq <= 0 {
		return nil, fmt.Errorf("new-batchseq-engine: context has no sequence capacity")
	}

	maxTokens := batchSeqTokenLimit(int(llama.NBatch(lctx)), int(llama.NUBatch(lctx)))
	if maxTokens <= 0 {
		return nil, fmt.Errorf("new-batchseq-engine: context has no batch capacity")
	}

	e := batchSeqEngine{
		lctx:        lctx,
		mem:         mem,
		batch:       llama.BatchInit(int32(maxTokens), 0, 1),
		maxSeq:      maxSeq,
		maxTokens:   maxTokens,
		requestQ:    make(chan *batchSeqJob, max(queueDepth, 1)),
		admissionCh: make(chan struct{}),
		shutdownCh:  make(chan struct{}),
		doneCh:      make(chan struct{}),
		hasBatch:    true,
		accepting:   true,
	}
	e.evaluate = e.evaluateJob

	return &e, nil
}

func (e *batchSeqEngine) start() {
	e.wg.Add(1)
	go e.processLoop()
}

func (e *batchSeqEngine) stop() error {
	if e.stopped.CompareAndSwap(false, true) {
		e.closeAdmission(errBatchSeqStopped)
		close(e.shutdownCh)
	}
	e.wg.Wait()

	return e.freeBatch()
}

func (e *batchSeqEngine) freeBatch() error {
	if !e.hasBatch || !e.batchFreed.CompareAndSwap(false, true) {
		return nil
	}

	return llama.BatchFree(e.batch)
}

func (e *batchSeqEngine) run(ctx context.Context, items []batchSeqItem, outputWidth int) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("batchseq-run: no items")
	}
	if outputWidth <= 0 {
		return nil, fmt.Errorf("batchseq-run: output width must be positive")
	}
	job := newBatchSeqJob(ctx, items, outputWidth)

	if err := e.beginSubmit(); err != nil {
		return nil, err
	}

	if err := e.enqueue(ctx, job); err != nil {
		return nil, err
	}

	select {
	case result := <-job.resultCh:
		return result.outputs, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.doneCh:
		select {
		case result := <-job.resultCh:
			return result.outputs, result.err
		default:
			return nil, e.stoppedErr()
		}
	}
}

// enqueue transfers an admitted submission to the bounded owner queue and
// releases its admission accounting on every exit path.
func (e *batchSeqEngine) enqueue(ctx context.Context, job *batchSeqJob) error {
	defer e.submitWG.Done()

	select {
	case e.requestQ <- job:
	case <-ctx.Done():
		return ctx.Err()
	case <-e.admissionCh:
		return e.stoppedErr()
	}

	return nil
}

func (e *batchSeqEngine) processLoop() {
	defer e.wg.Done()
	defer close(e.doneCh)

	active := make([]*batchSeqJob, 0, e.maxSeq)

	for {
		if e.stopped.Load() {
			err := e.stoppedErr()
			completeBatchSeqJobs(active, err)
			e.drain(err)
			e.terminate(err)
			return
		}

		if len(active) == 0 {
			select {
			case <-e.shutdownCh:
				err := e.stoppedErr()
				e.drain(err)
				e.terminate(err)
				return

			case job := <-e.requestQ:
				if e.stopped.Load() {
					err := e.stoppedErr()
					job.complete(nil, err)
					e.terminate(err)
					return
				}
				active = append(active, job)
			}
		}

		active = e.collectBatchSeqJobs(active)
		active = completeCanceledBatchSeqJobs(active)
		if len(active) == 0 {
			continue
		}

		schedule, remaining, err := scheduleBatchSeq(active, e.maxSeq, e.maxTokens)
		if err != nil {
			completeBatchSeqJobs(active, err)
			e.terminate(err)
			return
		}
		active = remaining
		for _, failure := range schedule.failed {
			failure.job.complete(nil, failure.err)
		}

		if len(schedule.entries) == 0 {
			for _, job := range schedule.done {
				job.complete(job.outputs, nil)
			}
			continue
		}

		items := make([]batchSeqItem, len(schedule.entries))
		for i, entry := range schedule.entries {
			items[i] = entry.item
		}
		if e.stopped.Load() {
			err := e.stoppedErr()
			completeBatchSeqJobs(batchSeqScheduleJobs(schedule), err)
			completeBatchSeqJobs(active, err)
			e.drain(err)
			e.terminate(err)
			return
		}
		batchJob := newBatchSeqJob(context.Background(), items, schedule.outputWidth)
		for _, entry := range schedule.entries {
			if !entry.job.started {
				entry.job.started = true
				metrics.ObserveBatchSeqQueueWait(e.modelID, e.operation, time.Since(entry.job.queuedAt))
			}
		}
		outputs, fatal, err := e.evaluate(batchJob)
		if err != nil {
			metrics.ObserveBatchSeqBatch(e.modelID, e.operation, "error", len(items))
			affected := batchSeqScheduleJobs(schedule)
			completeBatchSeqJobs(affected, err)
			active = removeBatchSeqJobs(active, affected)
			if fatal {
				completeBatchSeqJobs(active, err)
				e.terminate(err)
				return
			}
			continue
		}
		if len(outputs) != len(schedule.entries) {
			metrics.ObserveBatchSeqBatch(e.modelID, e.operation, "error", len(items))
			err := fmt.Errorf("batchseq-process: evaluator returned %d outputs for %d items", len(outputs), len(schedule.entries))
			completeBatchSeqJobs(batchSeqScheduleJobs(schedule), err)
			completeBatchSeqJobs(active, err)
			e.terminate(err)
			return
		}
		metrics.ObserveBatchSeqBatch(e.modelID, e.operation, "ok", len(items))

		for i, entry := range schedule.entries {
			if entry.job.ctx.Err() == nil {
				entry.job.outputs[entry.itemOffset] = outputs[i]
			}
		}
		for _, job := range schedule.done {
			if err := job.ctx.Err(); err != nil {
				job.complete(nil, err)
				continue
			}
			job.complete(job.outputs, nil)
		}
		active = completeCanceledBatchSeqJobs(active)
	}
}

func (e *batchSeqEngine) collectBatchSeqJobs(active []*batchSeqJob) []*batchSeqJob {
	// Snapshot the queue so sustained arrivals cannot postpone evaluation.
	queued := len(e.requestQ)
	for range queued {
		active = append(active, <-e.requestQ)
	}

	return active
}

func completeCanceledBatchSeqJobs(jobs []*batchSeqJob) []*batchSeqJob {
	active := jobs[:0]
	for _, job := range jobs {
		if err := job.ctx.Err(); err != nil {
			job.complete(nil, err)
			continue
		}
		active = append(active, job)
	}

	return active
}

func completeBatchSeqJobs(jobs []*batchSeqJob, err error) {
	for _, job := range jobs {
		job.complete(nil, err)
	}
}

func batchSeqScheduleJobs(schedule batchSeqSchedule) []*batchSeqJob {
	jobs := make([]*batchSeqJob, 0, len(schedule.entries))
	seen := make(map[*batchSeqJob]struct{}, len(schedule.entries))
	for _, entry := range schedule.entries {
		if _, exists := seen[entry.job]; exists {
			continue
		}
		seen[entry.job] = struct{}{}
		jobs = append(jobs, entry.job)
	}

	return jobs
}

func removeBatchSeqJobs(jobs, remove []*batchSeqJob) []*batchSeqJob {
	removed := make(map[*batchSeqJob]struct{}, len(remove))
	for _, job := range remove {
		removed[job] = struct{}{}
	}

	remaining := jobs[:0]
	for _, job := range jobs {
		if _, exists := removed[job]; !exists {
			remaining = append(remaining, job)
		}
	}

	return remaining
}

func (e *batchSeqEngine) evaluateJob(job *batchSeqJob) ([][]float32, bool, error) {
	outputs := make([][]float32, len(job.items))

	for next := 0; next < len(job.items); {
		if err := job.ctx.Err(); err != nil {
			return nil, false, err
		}

		plan, err := planBatchSeqItems(job.items, next, e.maxSeq, e.maxTokens)
		if err != nil {
			return nil, false, err
		}
		if len(plan.entries) == 0 {
			return nil, false, fmt.Errorf("batchseq-evaluate: planner made no progress")
		}

		if e.mem != 0 {
			if err := llama.MemoryClear(e.mem, true); err != nil {
				return nil, true, fmt.Errorf("batchseq-evaluate: clear memory: %w", err)
			}
		}
		if err := e.batch.Clear(); err != nil {
			return nil, true, fmt.Errorf("batchseq-evaluate: clear batch: %w", err)
		}

		for _, entry := range plan.entries {
			seqIDs := []llama.SeqId{entry.seqID}
			for pos, token := range entry.tokens {
				if err := e.batch.Add(token, llama.Pos(pos), seqIDs, true); err != nil {
					return nil, true, fmt.Errorf("batchseq-evaluate: add item[%d] token at pos %d: %w", entry.itemIndex, pos, err)
				}
			}
		}

		ret, err := llama.Decode(e.lctx, e.batch)
		if err != nil {
			return nil, true, fmt.Errorf("batchseq-evaluate: decode: %w", err)
		}
		if ret != 0 {
			return nil, true, fmt.Errorf("batchseq-evaluate: decode returned %d", ret)
		}

		for _, entry := range plan.entries {
			raw, err := llama.GetEmbeddingsSeq(e.lctx, entry.seqID, int32(job.outputWidth))
			if err != nil {
				return nil, true, fmt.Errorf("batchseq-evaluate: get output for item[%d]: %w", entry.itemIndex, err)
			}
			if len(raw) != job.outputWidth {
				return nil, true, fmt.Errorf("batchseq-evaluate: item[%d] returned %d outputs, expected %d", entry.itemIndex, len(raw), job.outputWidth)
			}

			output := make([]float32, len(raw))
			copy(output, raw)
			outputs[entry.itemOffset] = output
		}

		next = plan.next
	}

	return outputs, false, nil
}

func (e *batchSeqEngine) drain(err error) {
	for {
		select {
		case job := <-e.requestQ:
			job.complete(nil, err)
		default:
			return
		}
	}
}

func (e *batchSeqEngine) beginSubmit() error {
	e.submitMu.Lock()
	defer e.submitMu.Unlock()

	if !e.accepting {
		return e.stoppedErr()
	}

	e.submitWG.Add(1)

	return nil
}

func (e *batchSeqEngine) closeAdmission(err error) {
	e.submitMu.Lock()
	defer e.submitMu.Unlock()

	if !e.accepting {
		return
	}

	e.accepting = false
	if err != nil {
		e.errMu.Lock()
		e.err = err
		e.errMu.Unlock()
	}
	close(e.admissionCh)
}

func (e *batchSeqEngine) terminate(err error) {
	e.closeAdmission(err)
	e.submitWG.Wait()
	e.drain(err)
}

func (e *batchSeqEngine) engineErr() error {
	e.errMu.RLock()
	defer e.errMu.RUnlock()

	return e.err
}

func (e *batchSeqEngine) stoppedErr() error {
	if err := e.engineErr(); err != nil {
		return err
	}

	return errBatchSeqStopped
}

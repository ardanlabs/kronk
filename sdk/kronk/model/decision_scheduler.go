package model

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var errDecisionEngineStopped = errors.New("decision engine stopped")

type decisionEvaluateFunc func(entries []decisionScheduledEntry) (outputs [][][]float32, fatal bool, err error)

type decisionJobResult struct {
	outputs [][][]float32
	err     error
}

type decisionJob struct {
	ctx      context.Context
	work     []decisionWork
	next     int
	outputs  [][][]float32
	resultCh chan decisionJobResult
	once     sync.Once
}

func newDecisionJob(ctx context.Context, work []decisionWork) *decisionJob {
	return &decisionJob{
		ctx:      ctx,
		work:     work,
		outputs:  make([][][]float32, len(work)),
		resultCh: make(chan decisionJobResult, 1),
	}
}

func (j *decisionJob) complete(outputs [][][]float32, err error) {
	j.once.Do(func() {
		j.resultCh <- decisionJobResult{outputs: outputs, err: err}
	})
}

// decisionScheduler serializes access to one llama context and coalesces
// already queued decision work under distinct sequence IDs.
type decisionScheduler struct {
	engine      *decisionEngine
	evaluate    decisionEvaluateFunc
	requestQ    chan *decisionJob
	admissionCh chan struct{}
	shutdownCh  chan struct{}
	doneCh      chan struct{}
	wg          sync.WaitGroup
	stopped     atomic.Bool

	submitMu  sync.Mutex
	submitWG  sync.WaitGroup
	accepting bool

	errMu sync.RWMutex
	err   error
}

func newDecisionScheduler(engine *decisionEngine, queueDepth int) *decisionScheduler {
	return &decisionScheduler{
		engine:      engine,
		evaluate:    engine.evaluate,
		requestQ:    make(chan *decisionJob, max(engine.maxSequences*queueDepth, 1)),
		admissionCh: make(chan struct{}),
		shutdownCh:  make(chan struct{}),
		doneCh:      make(chan struct{}),
		accepting:   true,
	}
}

func (s *decisionScheduler) start() {
	s.wg.Add(1)
	go s.processLoop()
}

func (s *decisionScheduler) stop() {
	if s.stopped.CompareAndSwap(false, true) {
		s.closeAdmission(errDecisionEngineStopped)
		close(s.shutdownCh)
	}
	s.wg.Wait()
}

func (s *decisionScheduler) run(ctx context.Context, work []decisionWork) ([][][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	job := newDecisionJob(ctx, work)
	if err := s.beginSubmit(); err != nil {
		return nil, err
	}
	if err := s.enqueue(ctx, job); err != nil {
		return nil, err
	}

	select {
	case result := <-job.resultCh:
		return result.outputs, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.doneCh:
		select {
		case result := <-job.resultCh:
			return result.outputs, result.err
		default:
			return nil, s.stoppedErr()
		}
	}
}

func (s *decisionScheduler) enqueue(ctx context.Context, job *decisionJob) error {
	defer s.submitWG.Done()

	select {
	case s.requestQ <- job:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.admissionCh:
		return s.stoppedErr()
	}

	return nil
}

func (s *decisionScheduler) processLoop() {
	defer s.wg.Done()
	defer close(s.doneCh)

	active := make([]*decisionJob, 0, s.engine.maxSequences)

	for {
		if s.stopped.Load() {
			err := s.stoppedErr()
			completeDecisionJobs(active, err)
			s.drain(err)
			s.terminate(err)
			return
		}

		if len(active) == 0 {
			select {
			case <-s.shutdownCh:
				err := s.stoppedErr()
				s.drain(err)
				s.terminate(err)
				return

			case job := <-s.requestQ:
				if s.stopped.Load() {
					err := s.stoppedErr()
					job.complete(nil, err)
					s.terminate(err)
					return
				}
				active = append(active, job)
			}
		}

		active = s.collect(active)
		active = completeCanceledDecisionJobs(active)
		if len(active) == 0 {
			continue
		}

		schedule, remaining, err := scheduleDecision(active, s.engine.maxSequences, s.engine.maxTokens, s.engine.maxOutputs)
		if err != nil {
			completeDecisionJobs(active, err)
			s.terminate(err)
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

		if s.stopped.Load() {
			err := s.stoppedErr()
			completeDecisionJobs(decisionScheduleJobs(schedule), err)
			completeDecisionJobs(active, err)
			s.drain(err)
			s.terminate(err)
			return
		}

		outputs, fatal, err := s.evaluate(schedule.entries)
		if err != nil {
			affected := decisionScheduleJobs(schedule)
			completeDecisionJobs(affected, err)
			active = removeDecisionJobs(active, affected)
			if fatal {
				completeDecisionJobs(active, err)
				s.terminate(err)
				return
			}
			continue
		}
		if len(outputs) != len(schedule.entries) {
			err := fmt.Errorf("decision scheduler returned %d outputs for %d work items", len(outputs), len(schedule.entries))
			completeDecisionJobs(decisionScheduleJobs(schedule), err)
			completeDecisionJobs(active, err)
			s.terminate(err)
			return
		}

		for i, entry := range schedule.entries {
			if entry.job.ctx.Err() == nil {
				entry.job.outputs[entry.workOffset] = outputs[i]
			}
		}
		for _, job := range schedule.done {
			if err := job.ctx.Err(); err != nil {
				job.complete(nil, err)
				continue
			}
			job.complete(job.outputs, nil)
		}
		active = completeCanceledDecisionJobs(active)
	}
}

func (s *decisionScheduler) collect(active []*decisionJob) []*decisionJob {
	queued := len(s.requestQ)
	for range queued {
		active = append(active, <-s.requestQ)
	}

	return active
}

func completeCanceledDecisionJobs(jobs []*decisionJob) []*decisionJob {
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

func completeDecisionJobs(jobs []*decisionJob, err error) {
	for _, job := range jobs {
		job.complete(nil, err)
	}
}

func decisionScheduleJobs(schedule decisionSchedule) []*decisionJob {
	jobs := make([]*decisionJob, 0, len(schedule.entries))
	seen := make(map[*decisionJob]struct{}, len(schedule.entries))
	for _, entry := range schedule.entries {
		if _, exists := seen[entry.job]; exists {
			continue
		}
		seen[entry.job] = struct{}{}
		jobs = append(jobs, entry.job)
	}

	return jobs
}

func removeDecisionJobs(jobs, removedJobs []*decisionJob) []*decisionJob {
	removed := make(map[*decisionJob]struct{}, len(removedJobs))
	for _, job := range removedJobs {
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

func (s *decisionScheduler) drain(err error) {
	for {
		select {
		case job := <-s.requestQ:
			job.complete(nil, err)
		default:
			return
		}
	}
}

func (s *decisionScheduler) beginSubmit() error {
	s.submitMu.Lock()
	defer s.submitMu.Unlock()

	if !s.accepting {
		return s.stoppedErr()
	}

	s.submitWG.Add(1)

	return nil
}

func (s *decisionScheduler) closeAdmission(err error) {
	s.submitMu.Lock()
	defer s.submitMu.Unlock()

	if !s.accepting {
		return
	}

	s.accepting = false
	if err != nil {
		s.errMu.Lock()
		s.err = err
		s.errMu.Unlock()
	}
	close(s.admissionCh)
}

func (s *decisionScheduler) terminate(err error) {
	s.stopped.Store(true)
	s.closeAdmission(err)
	s.submitWG.Wait()
	s.drain(err)
}

func (s *decisionScheduler) stoppedErr() error {
	s.errMu.RLock()
	defer s.errMu.RUnlock()

	if s.err != nil {
		return s.err
	}

	return errDecisionEngineStopped
}

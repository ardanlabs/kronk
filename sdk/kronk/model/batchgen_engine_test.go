package model

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/sdk/applog"
)

func TestBatchEngineStopRejectsBlockedSubmissionsAndDrainsAcceptedJobs(t *testing.T) {
	const queueSize = 4

	e := batchEngine{
		model:       &Model{log: applog.DiscardLogger},
		requestQ:    make(chan *chatJob, queueSize),
		wakeCh:      make(chan struct{}, 1),
		admissionCh: make(chan struct{}),
		shutdownCh:  make(chan struct{}),
		loopDone:    make(chan struct{}),
	}

	accepted := make(map[*chatJob]bool)
	for range queueSize {
		job := &chatJob{ctx: t.Context()}
		if err := e.submit(job); err != nil {
			t.Fatalf("submit initial job: %v", err)
		}
		accepted[job] = true
	}

	const blockedSubmissions = 64
	results := make(chan error, blockedSubmissions)
	var submitters sync.WaitGroup
	submitters.Add(blockedSubmissions)
	for range blockedSubmissions {
		go func() {
			defer submitters.Done()
			results <- e.submit(&chatJob{ctx: t.Context()})
		}()
	}
	runtime.Gosched()

	drained := make(chan *chatJob, queueSize)
	go func() {
		<-e.shutdownCh
		for {
			select {
			case job := <-e.requestQ:
				drained <- job
			default:
				close(drained)
				close(e.loopDone)
				return
			}
		}
	}()

	if err := e.stop(t.Context()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	submitters.Wait()
	close(results)

	for err := range results {
		if err == nil {
			t.Error("blocked submit error = nil, want shutdown error")
		}
	}
	for job := range drained {
		if !accepted[job] {
			t.Errorf("drained job %p was not accepted before shutdown", job)
		}
		delete(accepted, job)
	}
	if len(accepted) != 0 {
		t.Errorf("%d accepted jobs were not drained", len(accepted))
	}
	if err := e.submit(&chatJob{ctx: t.Context()}); err == nil {
		t.Error("submit after stop succeeded")
	}
}

func TestBatchEngineStopCanResumeWaitingAfterTimeout(t *testing.T) {
	e := batchEngine{
		model:       &Model{log: applog.DiscardLogger},
		requestQ:    make(chan *chatJob),
		admissionCh: make(chan struct{}),
		shutdownCh:  make(chan struct{}),
		loopDone:    make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := e.stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("first stop error = %v, want %v", err, context.Canceled)
	}

	close(e.loopDone)
	if err := e.stop(t.Context()); err != nil {
		t.Fatalf("second stop: %v", err)
	}
}

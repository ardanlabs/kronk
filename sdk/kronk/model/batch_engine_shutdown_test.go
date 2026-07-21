package model

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// newTestEngine builds a batchEngine with only the channels populated; submit
// and the shutdown synchronization touch no other field.
func newTestEngine(qcap int) *batchEngine {
	return &batchEngine{
		requestQ:   make(chan *chatJob, qcap),
		wakeCh:     make(chan struct{}, 1),
		shutdownCh: make(chan struct{}),
	}
}

// newTestEngineWithModel adds a minimal Model, needed by tests that drive
// drainQueue/failJob (they log and decrement activeStreams).
func newTestEngineWithModel(qcap int) *batchEngine {
	e := newTestEngine(qcap)
	e.model = &Model{log: noopLog}
	return e
}

// TestSubmitRejectsAfterShutdown verifies submit rejects every job once stop
// has set stopped and closed shutdownCh, since nothing reads requestQ again.
func TestSubmitRejectsAfterShutdown(t *testing.T) {
	const attempts = 200

	e := newTestEngine(attempts)
	e.stopped.Store(true)
	close(e.shutdownCh)

	accepted := 0
	for range attempts {
		if err := e.submit(&chatJob{ctx: context.Background()}); err == nil {
			accepted++
		}
	}

	if accepted != 0 {
		t.Errorf("submit accepted = %d, want 0 (jobs the engine will never complete)", accepted)
	}
}

// TestSubmitStopRaceFreezesQueue verifies that a submit racing stop cannot
// leave a job on requestQ after stop's locked drain, since no reader remains
// to complete it.
func TestSubmitStopRaceFreezesQueue(t *testing.T) {
	const trials = 2000

	stranded := 0
	acceptedButNotQueued := 0

	for range trials {
		e := newTestEngine(4)

		var wg sync.WaitGroup
		wg.Add(1)

		var accepted bool
		go func() {
			defer wg.Done()
			accepted = e.submit(&chatJob{ctx: context.Background()}) == nil
		}()

		// stop(): signal shutdown, lock out submits, then drain by hand
		// (drainQueue's failJob needs a Model; the ordering is the same).
		e.stopped.Store(true)
		close(e.shutdownCh)
		e.submitMu.Lock()

		drained := 0
		for draining := true; draining; {
			select {
			case <-e.requestQ:
				drained++
			default:
				draining = false
			}
		}
		e.submitMu.Unlock()

		wg.Wait()

		if len(e.requestQ) > 0 {
			stranded++
		}
		if accepted && drained == 0 {
			acceptedButNotQueued++
		}
	}

	if stranded != 0 {
		t.Errorf("jobs stranded on requestQ = %d, want 0", stranded)
	}
	if acceptedButNotQueued != 0 {
		t.Errorf("submits accepted but not queued = %d, want 0", acceptedButNotQueued)
	}
}

// TestDrainQueueCompletesJobs verifies drainQueue completes every job it takes
// off requestQ: closing job.ch and decrementing activeStreams, the mechanism
// that releases a blocked caller and lets Model.Unload's drain reach zero.
func TestDrainQueueCompletesJobs(t *testing.T) {
	const jobs = 8

	e := newTestEngineWithModel(jobs)
	e.model.activeStreams.Store(jobs)

	// Buffered channels so failJob's error send completes without a reader.
	chans := make([]chan ChatResponse, jobs)
	for i := range chans {
		chans[i] = make(chan ChatResponse, 1)
		e.requestQ <- &chatJob{id: "test-job", ctx: context.Background(), ch: chans[i], object: ObjectChatText}
	}

	drained := e.drainQueue(fmt.Errorf("drain-test: engine shutting down"))

	if drained != jobs {
		t.Errorf("drained = %d, want %d", drained, jobs)
	}
	if got := e.model.activeStreams.Load(); got != 0 {
		t.Errorf("activeStreams = %d, want 0", got)
	}
	for i, ch := range chans {
		<-ch // the buffered error response
		if _, ok := <-ch; ok {
			t.Errorf("job %d channel not closed", i)
		}
	}
}

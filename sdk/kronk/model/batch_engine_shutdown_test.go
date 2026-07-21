package model

// These tests pin the ownership contract of batchEngine.submit:
//
//	A nil return from submit means the batch engine has taken ownership of
//	the job and WILL complete it (finishSlot or failJob, either of which
//	closes job.ch and decrements activeStreams).
//
// The contract matters because ChatStreaming's batching=true path returns
// without closing ch and without decrementing activeStreams (chat.go) —
// it hands both responsibilities to the engine. A job the engine accepts
// but never completes therefore hangs the caller forever on ch AND pins
// activeStreams above zero, which makes Model.Unload fail with
// "cannot unload N active streams" and wedges the pool entry.
//
// At the model layer stop() and submit() can run concurrently: Model.Unload
// calls batch.stop() before its own activeStreams drain. End to end, though,
// production reaches submit only through Kronk.acquireModel, which holds the
// shutdown lock while it gates on shutdownFlag and increments the SDK-level
// activeStreams, and Kronk.Unload drains those streams under that same lock
// before it ever calls model.Unload — so the concurrent window is closed one
// layer up today. These tests pin the batch engine's OWN invariant so it
// stops silently depending on that caller: stop must freeze requestQ and
// complete every job it finds, whether or not an upstream lock also guards
// the door.

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// newTestEngine builds a batchEngine with only the channels populated.
// submit and the shutdown synchronization touch no other field, so no
// llama context or Model is needed.
func newTestEngine(qcap int) *batchEngine {
	return &batchEngine{
		requestQ:   make(chan *chatJob, qcap),
		wakeCh:     make(chan struct{}, 1),
		shutdownCh: make(chan struct{}),
	}
}

// newTestEngineWithModel is newTestEngine plus a minimal Model, needed by any
// test that drives drainQueue/failJob (they log, touch metrics, and decrement
// activeStreams). A discard logger and the zero-value ModelInfo are enough;
// metrics keyed by an empty model ID are harmless in a unit test.
func newTestEngineWithModel(qcap int) *batchEngine {
	e := newTestEngine(qcap)
	e.model = &Model{log: applog.DiscardLogger}
	return e
}

// TestSubmitRejectsAfterShutdown is the deterministic form of the defect.
//
// It puts the engine in the state stop() leaves behind — stopped set,
// shutdownCh closed, processLoop gone — and requires submit to reject
// every job, because nothing will ever read requestQ again.
//
// Selecting on shutdownCh alone is not enough to get this right: with the
// buffer non-full, `requestQ <- job` and a closed `<-shutdownCh` are both
// ready, and Go picks a ready case uniformly at random. Without the
// stopped check this fails on roughly half of the attempts.
func TestSubmitRejectsAfterShutdown(t *testing.T) {
	const attempts = 200

	e := newTestEngine(attempts)

	// Exactly what stop() establishes, in stop()'s order.
	e.stopped.Store(true)
	close(e.shutdownCh)

	accepted := 0
	for range attempts {
		job := &chatJob{ctx: context.Background()}
		if err := e.submit(job); err == nil {
			accepted++
		}
	}

	if accepted != 0 {
		t.Fatalf("submit accepted %d/%d jobs after shutdown; each is a job the "+
			"engine will never complete, so the caller blocks forever on job.ch "+
			"and activeStreams never reaches zero (Model.Unload then fails)",
			accepted, attempts)
	}
}

// TestSubmitStopRaceFreezesQueue reproduces the production interleaving:
// Model.Unload calls batch.stop() while an in-flight ChatStreaming
// goroutine is calling submit.
//
// It follows stop()'s sequence — set stopped, close shutdownCh, lock out
// submits, then drain — and asserts the property that sequence exists to
// provide: once the drain under the write lock has run, requestQ
// is frozen, so no job can arrive afterwards with no reader left to
// complete it.
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
			job := &chatJob{ctx: context.Background()}
			accepted = e.submit(job) == nil
		}()

		// stop(): signal shutdown, lock out submits, then drain. Draining by
		// hand rather than via drainQueue, whose failJob needs a Model; the
		// ordering under test is the same, and TestDrainQueueCompletesJobs
		// covers the failJob completion half separately.
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

		// The queue is frozen and the drain is over. Anything that shows
		// up now was accepted by submit but has no reader left.
		if len(e.requestQ) > 0 {
			stranded++
		}

		// An accepted job must have been in the queue for the drain to
		// find, since failJob is what closes ch and releases the caller.
		if accepted && drained == 0 {
			acceptedButNotQueued++
		}
	}

	if stranded > 0 || acceptedButNotQueued > 0 {
		t.Fatalf("queue not frozen by stop's sequence: %d/%d trials enqueued a job "+
			"after the locked drain, %d/%d reported success without the job "+
			"reaching the drain. Either way submit returned nil (caller set "+
			"batching=true and returned without closing ch) for a job nothing will "+
			"complete, so the request hangs and Model.Unload cannot drain "+
			"activeStreams", stranded, trials, acceptedButNotQueued, trials)
	}
}

// TestDrainQueueCompletesJobs pins the completion half of the ownership
// contract: for every job drainQueue takes off requestQ it must close job.ch
// and decrement activeStreams. That is the exact mechanism that releases a
// caller blocked on ch and lets Model.Unload's drain loop reach zero — freezing
// the queue (the other two tests) is only useful if the frozen-out jobs are
// then actually completed.
func TestDrainQueueCompletesJobs(t *testing.T) {
	const jobs = 8

	e := newTestEngineWithModel(jobs)

	// One activeStream per in-flight job, as ChatStreaming establishes before
	// handing the job to the engine.
	e.model.activeStreams.Store(int32(jobs))

	// Each job carries a buffered response channel so failJob's error send
	// completes without a reader, mirroring the buffered channel ChatStreaming
	// hands over.
	chans := make([]chan ChatResponse, jobs)
	for i := range chans {
		ch := make(chan ChatResponse, 1)
		chans[i] = ch
		e.requestQ <- &chatJob{
			id:     "test-job",
			ctx:    context.Background(),
			ch:     ch,
			object: ObjectChatText,
		}
	}

	drained := e.drainQueue(fmt.Errorf("drain-test: engine shutting down"))

	if drained != jobs {
		t.Fatalf("drainQueue drained %d jobs, want %d", drained, jobs)
	}

	if got := e.model.activeStreams.Load(); got != 0 {
		t.Fatalf("activeStreams = %d after draining %d jobs, want 0; a caller whose "+
			"stream is never decremented pins the count above zero and Model.Unload "+
			"cannot drain it", got, jobs)
	}

	// Every channel must be closed. A closed channel yields ok=false once its
	// buffered value is consumed; a channel left open would block here, so the
	// receive-with-ok both proves closure and can't hang the test.
	for i, ch := range chans {
		<-ch // the buffered error response failJob sent
		if _, ok := <-ch; ok {
			t.Fatalf("job %d channel not closed by failJob; the caller blocks on it forever", i)
		}
	}
}

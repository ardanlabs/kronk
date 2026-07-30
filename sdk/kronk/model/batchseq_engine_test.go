package model

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestNewBatchSeqEngineRejectsInvalidContext(t *testing.T) {
	if _, err := newBatchSeqEngine(0, 0, 1); err == nil {
		t.Fatal("newBatchSeqEngine: expected error")
	}
}

func TestBatchSeqEngineRun(t *testing.T) {
	e := newTestBatchSeqEngine(1, func(job *batchSeqJob) ([][]float32, bool, error) {
		return [][]float32{{1, 2}}, false, nil
	})
	e.start()
	defer func() {
		if err := e.stop(); err != nil {
			t.Errorf("stop: unexpected error: %v", err)
		}
	}()

	items := []batchSeqItem{{index: 0, tokens: []llama.Token{1}}}
	got, err := e.run(context.Background(), items, 2)
	if err != nil {
		t.Fatalf("run: unexpected error: %v", err)
	}
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != 1 || got[0][1] != 2 {
		t.Fatalf("run: got %v, want [[1 2]]", got)
	}
}

func TestBatchSeqEngineCoalescesQueuedRequests(t *testing.T) {
	var calls atomic.Int32
	e := newTestBatchSeqEngine(2, func(job *batchSeqJob) ([][]float32, bool, error) {
		calls.Add(1)
		outputs := make([][]float32, len(job.items))
		for i, item := range job.items {
			outputs[i] = []float32{float32(item.index)}
		}
		return outputs, false, nil
	})

	first := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 10, tokens: []llama.Token{1}}}, 1)
	second := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 20, tokens: []llama.Token{2}}}, 1)
	e.requestQ <- first
	e.requestQ <- second
	e.start()
	defer func() {
		if err := e.stop(); err != nil {
			t.Errorf("stop: unexpected error: %v", err)
		}
	}()

	firstResult := <-first.resultCh
	secondResult := <-second.resultCh
	if firstResult.err != nil {
		t.Fatalf("first result: unexpected error: %v", firstResult.err)
	}
	if secondResult.err != nil {
		t.Fatalf("second result: unexpected error: %v", secondResult.err)
	}
	if got := firstResult.outputs[0][0]; got != 10 {
		t.Errorf("first output: got %v, want %v", got, 10)
	}
	if got := secondResult.outputs[0][0]; got != 20 {
		t.Errorf("second output: got %v, want %v", got, 20)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("evaluation calls: got %d, want %d", got, 1)
	}
}

func TestBatchSeqEngineCancellationDoesNotCorruptSharedBatch(t *testing.T) {
	evaluating := make(chan struct{})
	release := make(chan struct{})
	e := newTestBatchSeqEngine(2, func(job *batchSeqJob) ([][]float32, bool, error) {
		close(evaluating)
		<-release
		return [][]float32{{10}, {20}}, false, nil
	})

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	first := newBatchSeqJob(firstCtx, []batchSeqItem{{index: 10, tokens: []llama.Token{1}}}, 1)
	second := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 20, tokens: []llama.Token{2}}}, 1)
	e.requestQ <- first
	e.requestQ <- second
	e.start()
	defer func() {
		if err := e.stop(); err != nil {
			t.Errorf("stop: unexpected error: %v", err)
		}
	}()

	select {
	case <-evaluating:
	case <-time.After(time.Second):
		t.Fatal("shared evaluation did not start")
	}
	cancelFirst()
	close(release)

	firstResult := <-first.resultCh
	if !errors.Is(firstResult.err, context.Canceled) {
		t.Errorf("first result error: got %v, want %v", firstResult.err, context.Canceled)
	}
	if firstResult.outputs != nil {
		t.Errorf("first outputs: got %v, want nil", firstResult.outputs)
	}

	secondResult := <-second.resultCh
	if secondResult.err != nil {
		t.Fatalf("second result: unexpected error: %v", secondResult.err)
	}
	if got := secondResult.outputs[0][0]; got != 20 {
		t.Errorf("second output: got %v, want %v", got, 20)
	}
}

func TestBatchSeqEngineRoundRobinAcrossBatches(t *testing.T) {
	var calls atomic.Int32
	var batches [][]int
	e := newTestBatchSeqEngine(2, func(job *batchSeqJob) ([][]float32, bool, error) {
		calls.Add(1)
		indexes := make([]int, len(job.items))
		outputs := make([][]float32, len(job.items))
		for i, item := range job.items {
			indexes[i] = item.index
			outputs[i] = []float32{float32(item.index)}
		}
		batches = append(batches, indexes)
		return outputs, false, nil
	})
	e.maxSeq = 2
	e.maxTokens = 2

	first := newBatchSeqJob(context.Background(), []batchSeqItem{
		{index: 10, tokens: []llama.Token{1}},
		{index: 11, tokens: []llama.Token{2}},
		{index: 12, tokens: []llama.Token{3}},
	}, 1)
	second := newBatchSeqJob(context.Background(), []batchSeqItem{
		{index: 20, tokens: []llama.Token{4}},
	}, 1)
	e.requestQ <- first
	e.requestQ <- second
	e.start()
	defer func() {
		if err := e.stop(); err != nil {
			t.Errorf("stop: unexpected error: %v", err)
		}
	}()

	firstResult := <-first.resultCh
	secondResult := <-second.resultCh
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("results: first error %v, second error %v", firstResult.err, secondResult.err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("evaluation calls: got %d, want %d", got, 2)
	}
	wantBatches := [][]int{{10, 20}, {11, 12}}
	for i, want := range wantBatches {
		if !slices.Equal(batches[i], want) {
			t.Errorf("batch[%d]: got %v, want %v", i, batches[i], want)
		}
	}
	for i, want := range []float32{10, 11, 12} {
		if got := firstResult.outputs[i][0]; got != want {
			t.Errorf("first output[%d]: got %v, want %v", i, got, want)
		}
	}
}

func TestBatchSeqEngineSeparatesOutputWidths(t *testing.T) {
	var widths []int
	e := newTestBatchSeqEngine(2, func(job *batchSeqJob) ([][]float32, bool, error) {
		widths = append(widths, job.outputWidth)
		outputs := make([][]float32, len(job.items))
		for i := range outputs {
			outputs[i] = make([]float32, job.outputWidth)
		}
		return outputs, false, nil
	})

	first := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 10, tokens: []llama.Token{1}}}, 2)
	second := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 20, tokens: []llama.Token{2}}}, 3)
	e.requestQ <- first
	e.requestQ <- second
	e.start()
	defer func() {
		if err := e.stop(); err != nil {
			t.Errorf("stop: unexpected error: %v", err)
		}
	}()

	firstResult := <-first.resultCh
	secondResult := <-second.resultCh
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("results: first error %v, second error %v", firstResult.err, secondResult.err)
	}
	if !slices.Equal(widths, []int{2, 3}) {
		t.Errorf("widths: got %v, want %v", widths, []int{2, 3})
	}
	if got := len(firstResult.outputs[0]); got != 2 {
		t.Errorf("first width: got %d, want %d", got, 2)
	}
	if got := len(secondResult.outputs[0]); got != 3 {
		t.Errorf("second width: got %d, want %d", got, 3)
	}
}

func TestBatchSeqEngineCancellationDiscardsPartialOutputs(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	var calls atomic.Int32

	e := newTestBatchSeqEngine(2, func(job *batchSeqJob) ([][]float32, bool, error) {
		switch calls.Add(1) {
		case 1:
			close(firstStarted)
			<-releaseFirst
		case 2:
			close(secondStarted)
			<-releaseSecond
		}

		outputs := make([][]float32, len(job.items))
		for i, item := range job.items {
			outputs[i] = []float32{float32(item.index)}
		}
		return outputs, false, nil
	})
	e.maxSeq = 2

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	first := newBatchSeqJob(firstCtx, []batchSeqItem{
		{index: 10, tokens: []llama.Token{1}},
		{index: 11, tokens: []llama.Token{2}},
		{index: 12, tokens: []llama.Token{3}},
	}, 1)
	second := newBatchSeqJob(context.Background(), []batchSeqItem{
		{index: 20, tokens: []llama.Token{4}},
		{index: 21, tokens: []llama.Token{5}},
	}, 1)
	e.requestQ <- first
	e.requestQ <- second
	e.start()
	defer func() {
		if err := e.stop(); err != nil {
			t.Errorf("stop: unexpected error: %v", err)
		}
	}()

	<-firstStarted
	close(releaseFirst)
	<-secondStarted
	cancelFirst()
	close(releaseSecond)

	firstResult := <-first.resultCh
	if !errors.Is(firstResult.err, context.Canceled) {
		t.Errorf("first result error: got %v, want %v", firstResult.err, context.Canceled)
	}
	if firstResult.outputs != nil {
		t.Errorf("first outputs: got %v, want nil", firstResult.outputs)
	}

	secondResult := <-second.resultCh
	if secondResult.err != nil {
		t.Fatalf("second result: unexpected error: %v", secondResult.err)
	}
	for i, want := range []float32{20, 21} {
		if got := secondResult.outputs[i][0]; got != want {
			t.Errorf("second output[%d]: got %v, want %v", i, got, want)
		}
	}
}

func TestBatchSeqEngineFatalErrorReachesBacklog(t *testing.T) {
	wantErr := errors.New("native failure")
	e := newTestBatchSeqEngine(3, func(job *batchSeqJob) ([][]float32, bool, error) {
		return nil, true, wantErr
	})
	e.maxSeq = 2

	jobs := []*batchSeqJob{
		newBatchSeqJob(context.Background(), []batchSeqItem{{index: 10, tokens: []llama.Token{1}}}, 1),
		newBatchSeqJob(context.Background(), []batchSeqItem{{index: 20, tokens: []llama.Token{2}}}, 1),
		newBatchSeqJob(context.Background(), []batchSeqItem{{index: 30, tokens: []llama.Token{3}}}, 1),
	}
	for _, job := range jobs {
		e.requestQ <- job
	}
	e.start()

	for i, job := range jobs {
		result := <-job.resultCh
		if !errors.Is(result.err, wantErr) {
			t.Errorf("job[%d] error: got %v, want %v", i, result.err, wantErr)
		}
	}
	if _, err := e.run(context.Background(), []batchSeqItem{{index: 40, tokens: []llama.Token{4}}}, 1); !errors.Is(err, wantErr) {
		t.Errorf("future run error: got %v, want %v", err, wantErr)
	}
	if err := e.stop(); err != nil {
		t.Errorf("stop: unexpected error: %v", err)
	}
}

func TestBatchSeqEngineCollectsQueueSnapshot(t *testing.T) {
	e := newTestBatchSeqEngine(2, nil)
	first := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 10, tokens: []llama.Token{1}}}, 1)
	second := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 20, tokens: []llama.Token{2}}}, 1)
	third := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 30, tokens: []llama.Token{3}}}, 1)
	e.requestQ <- first
	e.requestQ <- second

	sent := make(chan struct{})
	go func() {
		e.requestQ <- third
		close(sent)
	}()

	active := e.collectBatchSeqJobs(nil)
	if len(active) != 2 || active[0] != first || active[1] != second {
		t.Errorf("active: got %v, want first and second jobs", active)
	}
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("blocked sender did not refill queue")
	}
	if got := <-e.requestQ; got != third {
		t.Errorf("queued job: got %p, want %p", got, third)
	}
}

func TestBatchSeqEngineFatalError(t *testing.T) {
	wantErr := errors.New("native failure")
	e := newTestBatchSeqEngine(1, func(job *batchSeqJob) ([][]float32, bool, error) {
		return nil, true, wantErr
	})
	e.start()

	items := []batchSeqItem{{index: 0, tokens: []llama.Token{1}}}
	if _, err := e.run(context.Background(), items, 1); !errors.Is(err, wantErr) {
		t.Fatalf("run error: got %v, want %v", err, wantErr)
	}
	if _, err := e.run(context.Background(), items, 1); !errors.Is(err, wantErr) {
		t.Fatalf("second run error: got %v, want %v", err, wantErr)
	}

	if err := e.stop(); err != nil {
		t.Fatalf("stop: unexpected error: %v", err)
	}
}

func TestBatchSeqEngineShutdownDrainsQueue(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32

	e := newTestBatchSeqEngine(2, func(job *batchSeqJob) ([][]float32, bool, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return [][]float32{{1}}, false, nil
	})
	e.start()

	items := []batchSeqItem{{index: 0, tokens: []llama.Token{1}}}
	firstCh := make(chan error, 1)
	go func() {
		_, err := e.run(context.Background(), items, 1)
		firstCh <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first evaluation did not start")
	}

	secondJob := newBatchSeqJob(context.Background(), items, 1)
	e.requestQ <- secondJob

	stopCh := make(chan error, 1)
	go func() {
		stopCh <- e.stop()
	}()
	<-e.shutdownCh
	close(release)

	if err := <-firstCh; err != nil {
		t.Errorf("first run: unexpected error: %v", err)
	}
	secondResult := <-secondJob.resultCh
	if !errors.Is(secondResult.err, errBatchSeqStopped) {
		t.Errorf("second run error: got %v, want %v", secondResult.err, errBatchSeqStopped)
	}

	select {
	case err := <-stopCh:
		if err != nil {
			t.Errorf("stop: unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("engine did not stop")
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("evaluation calls: got %d, want %d", got, 1)
	}
}

func TestBatchSeqEngineTerminationWaitsForSubmitters(t *testing.T) {
	e := newTestBatchSeqEngine(1, nil)
	if err := e.beginSubmit(); err != nil {
		t.Fatalf("beginSubmit: unexpected error: %v", err)
	}

	terminated := make(chan struct{})
	go func() {
		e.terminate(errBatchSeqStopped)
		close(terminated)
	}()

	<-e.admissionCh

	select {
	case <-terminated:
		t.Fatal("termination completed before admitted submitter exited")
	default:
	}

	job := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 0, tokens: []llama.Token{1}}}, 1)
	e.requestQ <- job
	e.submitWG.Done()

	select {
	case <-terminated:
	case <-time.After(time.Second):
		t.Fatal("termination did not complete")
	}

	result := <-job.resultCh
	if !errors.Is(result.err, errBatchSeqStopped) {
		t.Errorf("job error: got %v, want %v", result.err, errBatchSeqStopped)
	}
	if got := len(e.requestQ); got != 0 {
		t.Errorf("queued jobs: got %d, want %d", got, 0)
	}
}

func TestBatchSeqEngineFullQueueCallerCancellation(t *testing.T) {
	e := newTestBatchSeqEngine(1, nil)
	queued := newBatchSeqJob(context.Background(), []batchSeqItem{{index: 0, tokens: []llama.Token{1}}}, 1)
	e.requestQ <- queued

	ctx, cancel := context.WithCancel(t.Context())
	if err := e.beginSubmit(); err != nil {
		t.Fatalf("beginSubmit: unexpected error: %v", err)
	}

	enqueueCh := make(chan error, 1)
	go func() {
		job := newBatchSeqJob(ctx, []batchSeqItem{{index: 1, tokens: []llama.Token{2}}}, 1)
		enqueueCh <- e.enqueue(ctx, job)
	}()

	select {
	case err := <-enqueueCh:
		t.Fatalf("enqueue returned before cancellation: %v", err)
	default:
	}

	cancel()
	if err := <-enqueueCh; !errors.Is(err, context.Canceled) {
		t.Errorf("enqueue error: got %v, want %v", err, context.Canceled)
	}

	terminated := make(chan struct{})
	go func() {
		e.terminate(errBatchSeqStopped)
		close(terminated)
	}()

	select {
	case <-terminated:
	case <-time.After(time.Second):
		t.Fatal("termination blocked after canceled submission")
	}

	result := <-queued.resultCh
	if !errors.Is(result.err, errBatchSeqStopped) {
		t.Errorf("queued job error: got %v, want %v", result.err, errBatchSeqStopped)
	}
}

func TestBatchSeqEngineShutdownReleasesBlockedSubmitter(t *testing.T) {
	evaluating := make(chan struct{})
	release := make(chan struct{})
	e := newTestBatchSeqEngine(1, func(job *batchSeqJob) ([][]float32, bool, error) {
		close(evaluating)
		<-release
		return [][]float32{{1}}, false, nil
	})
	e.start()

	items := []batchSeqItem{{index: 0, tokens: []llama.Token{1}}}
	firstCh := make(chan error, 1)
	go func() {
		_, err := e.run(context.Background(), items, 1)
		firstCh <- err
	}()
	<-evaluating

	secondCh := make(chan error, 1)
	go func() {
		_, err := e.run(context.Background(), items, 1)
		secondCh <- err
	}()

	deadline := time.Now().Add(time.Second)
	for len(e.requestQ) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("second request did not enter the queue")
		}
		runtime.Gosched()
	}

	if err := e.beginSubmit(); err != nil {
		t.Fatalf("beginSubmit: unexpected error: %v", err)
	}
	blockedCh := make(chan error, 1)
	go func() {
		job := newBatchSeqJob(context.Background(), items, 1)
		blockedCh <- e.enqueue(context.Background(), job)
	}()

	stopCh := make(chan error, 1)
	go func() {
		stopCh <- e.stop()
	}()
	<-e.shutdownCh

	if err := <-blockedCh; !errors.Is(err, errBatchSeqStopped) {
		t.Errorf("blocked enqueue error: got %v, want %v", err, errBatchSeqStopped)
	}
	close(release)

	if err := <-firstCh; err != nil {
		t.Errorf("in-flight run: unexpected error: %v", err)
	}
	if err := <-secondCh; !errors.Is(err, errBatchSeqStopped) {
		t.Errorf("queued run error: got %v, want %v", err, errBatchSeqStopped)
	}
	if err := <-stopCh; err != nil {
		t.Errorf("stop: unexpected error: %v", err)
	}
}

func TestBatchSeqEngineStopRejectsNextQueuedJob(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32

	e := newTestBatchSeqEngine(2, func(job *batchSeqJob) ([][]float32, bool, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return [][]float32{{1}}, false, nil
	})
	e.start()

	items := []batchSeqItem{{index: 0, tokens: []llama.Token{1}}}
	firstCh := make(chan error, 1)
	go func() {
		_, err := e.run(context.Background(), items, 1)
		firstCh <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first evaluation did not start")
	}

	e.submitMu.Lock()
	stopCh := make(chan error, 1)
	go func() {
		stopCh <- e.stop()
	}()

	deadline := time.Now().Add(time.Second)
	for !e.stopped.Load() && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if !e.stopped.Load() {
		e.submitMu.Unlock()
		t.Fatal("stop did not begin")
	}

	secondJob := newBatchSeqJob(context.Background(), items, 1)
	e.requestQ <- secondJob
	close(release)

	if err := <-firstCh; err != nil {
		t.Errorf("first run: unexpected error: %v", err)
	}
	secondResult := <-secondJob.resultCh
	e.submitMu.Unlock()

	if !errors.Is(secondResult.err, errBatchSeqStopped) {
		t.Errorf("second run error: got %v, want %v", secondResult.err, errBatchSeqStopped)
	}
	if err := <-stopCh; err != nil {
		t.Errorf("stop: unexpected error: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("evaluation calls: got %d, want %d", got, 1)
	}
}

func newTestBatchSeqEngine(queueDepth int, evaluate batchSeqEvaluateFunc) *batchSeqEngine {
	return &batchSeqEngine{
		maxSeq:      4,
		maxTokens:   32,
		requestQ:    make(chan *batchSeqJob, queueDepth),
		admissionCh: make(chan struct{}),
		shutdownCh:  make(chan struct{}),
		doneCh:      make(chan struct{}),
		evaluate:    evaluate,
		accepting:   true,
	}
}

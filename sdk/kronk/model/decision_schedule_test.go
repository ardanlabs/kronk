package model

import (
	"context"
	"errors"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestScheduleDecisionRoundRobin(t *testing.T) {
	jobA := newDecisionJob(context.Background(), []decisionWork{
		decisionTestWork(2, 1),
		decisionTestWork(4, 1),
	})
	jobB := newDecisionJob(context.Background(), []decisionWork{
		decisionTestWork(3, 2),
		decisionTestWork(5, 1),
	})

	schedule, remaining, err := scheduleDecision([]*decisionJob{jobA, jobB}, 3, 20, 10)
	if err != nil {
		t.Fatalf("scheduleDecision: %v", err)
	}
	if len(schedule.entries) != 3 {
		t.Fatalf("entries: got %d, want 3", len(schedule.entries))
	}

	wantJobs := []*decisionJob{jobA, jobB, jobA}
	wantOffsets := []int{0, 0, 1}
	for i, entry := range schedule.entries {
		if entry.job != wantJobs[i] || entry.workOffset != wantOffsets[i] {
			t.Errorf("entry[%d]: got job=%p offset=%d, want job=%p offset=%d", i, entry.job, entry.workOffset, wantJobs[i], wantOffsets[i])
		}
	}
	if schedule.nTokens != 9 || schedule.nOutputs != 4 {
		t.Errorf("capacity: got tokens=%d outputs=%d, want tokens=9 outputs=4", schedule.nTokens, schedule.nOutputs)
	}
	if len(schedule.done) != 1 || schedule.done[0] != jobA {
		t.Errorf("done: got %v, want jobA", schedule.done)
	}
	if len(remaining) != 1 || remaining[0] != jobB {
		t.Errorf("remaining: got %v, want jobB", remaining)
	}
}

func TestScheduleDecisionRespectsTokenAndOutputCapacity(t *testing.T) {
	tooLarge := newDecisionJob(context.Background(), []decisionWork{decisionTestWork(8, 1)})
	fits := newDecisionJob(context.Background(), []decisionWork{decisionTestWork(3, 2)})
	deferred := newDecisionJob(context.Background(), []decisionWork{decisionTestWork(2, 2)})

	schedule, remaining, err := scheduleDecision([]*decisionJob{tooLarge, fits, deferred}, 3, 5, 3)
	if err != nil {
		t.Fatalf("scheduleDecision: %v", err)
	}
	if len(schedule.failed) != 1 || schedule.failed[0].job != tooLarge {
		t.Fatalf("failed: got %v, want tooLarge", schedule.failed)
	}
	if len(schedule.entries) != 1 || schedule.entries[0].job != fits {
		t.Fatalf("entries: got %v, want fits", schedule.entries)
	}
	if len(remaining) != 1 || remaining[0] != deferred {
		t.Fatalf("remaining: got %v, want deferred", remaining)
	}
}

func TestScheduleDecisionIsolatesJointWork(t *testing.T) {
	regular := newDecisionJob(context.Background(), []decisionWork{decisionTestWork(2, 1)})
	joint := newDecisionJob(context.Background(), []decisionWork{{
		tokens:        make([]llama.Token, 4),
		decisionOrder: make([]int32, 4),
		jointScores:   2,
	}})
	after := newDecisionJob(context.Background(), []decisionWork{decisionTestWork(2, 1)})

	first, remaining, err := scheduleDecision([]*decisionJob{joint, regular, after}, 3, 20, 20)
	if err != nil {
		t.Fatalf("schedule joint first: %v", err)
	}
	if len(first.entries) != 1 || first.entries[0].job != joint || first.nOutputs != 4 {
		t.Fatalf("joint schedule: got entries=%v outputs=%d", first.entries, first.nOutputs)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining jobs: got %d, want 2", len(remaining))
	}

	second, _, err := scheduleDecision([]*decisionJob{regular, joint, after}, 3, 20, 20)
	if err != nil {
		t.Fatalf("schedule regular first: %v", err)
	}
	if len(second.entries) != 2 || second.entries[0].job != regular || second.entries[1].job != after {
		t.Fatalf("regular schedule should defer joint work: got %v", second.entries)
	}
}

func TestDecisionSchedulerCoalescesQueuedJobs(t *testing.T) {
	engine := decisionEngine{
		maxSequences: 4,
		maxTokens:    100,
		maxOutputs:   10,
	}
	scheduler := newDecisionScheduler(&engine, 1)

	batchWidths := make(chan int, 1)
	scheduler.evaluate = func(entries []decisionScheduledEntry) ([][][]float32, bool, error) {
		batchWidths <- len(entries)
		outputs := make([][][]float32, len(entries))
		for i, entry := range entries {
			outputs[i] = make([][]float32, len(entry.work.readouts))
		}
		return outputs, false, nil
	}

	jobs := []*decisionJob{
		newDecisionJob(context.Background(), []decisionWork{decisionTestWork(2, 1)}),
		newDecisionJob(context.Background(), []decisionWork{decisionTestWork(3, 1)}),
		newDecisionJob(context.Background(), []decisionWork{decisionTestWork(4, 1)}),
	}
	for _, job := range jobs {
		scheduler.requestQ <- job
	}

	scheduler.start()
	t.Cleanup(scheduler.stop)

	for i, job := range jobs {
		result := <-job.resultCh
		if result.err != nil {
			t.Fatalf("job[%d]: %v", i, result.err)
		}
	}

	if got := <-batchWidths; got != len(jobs) {
		t.Fatalf("batch width: got %d, want %d", got, len(jobs))
	}
}

func TestDecisionSchedulerStopDrainsQueuedJobs(t *testing.T) {
	engine := decisionEngine{
		maxSequences: 1,
		maxTokens:    100,
		maxOutputs:   10,
	}
	scheduler := newDecisionScheduler(&engine, 2)

	evaluating := make(chan struct{})
	release := make(chan struct{})
	scheduler.evaluate = func(entries []decisionScheduledEntry) ([][][]float32, bool, error) {
		close(evaluating)
		<-release
		return make([][][]float32, len(entries)), false, nil
	}

	first := newDecisionJob(context.Background(), []decisionWork{decisionTestWork(2, 1)})
	second := newDecisionJob(context.Background(), []decisionWork{decisionTestWork(2, 1)})
	scheduler.requestQ <- first
	scheduler.start()
	<-evaluating
	scheduler.requestQ <- second

	stopped := make(chan struct{})
	go func() {
		scheduler.stop()
		close(stopped)
	}()
	<-scheduler.admissionCh
	close(release)
	<-stopped

	if result := <-first.resultCh; result.err != nil {
		t.Errorf("active job: got error %v, want success", result.err)
	}
	if result := <-second.resultCh; !errors.Is(result.err, errDecisionEngineStopped) {
		t.Errorf("queued job: got error %v, want %v", result.err, errDecisionEngineStopped)
	}
	if _, err := scheduler.run(context.Background(), []decisionWork{decisionTestWork(2, 1)}); !errors.Is(err, errDecisionEngineStopped) {
		t.Errorf("new job: got error %v, want %v", err, errDecisionEngineStopped)
	}
}

func TestDecisionSchedulerContinuesAfterRecoverableEvaluationError(t *testing.T) {
	engine := decisionEngine{
		maxSequences: 1,
		maxTokens:    100,
		maxOutputs:   10,
	}
	scheduler := newDecisionScheduler(&engine, 2)

	recoverableErr := errors.New("no KV slot")
	evaluations := 0
	scheduler.evaluate = func(entries []decisionScheduledEntry) ([][][]float32, bool, error) {
		evaluations++
		if evaluations == 1 {
			return nil, false, recoverableErr
		}
		outputs := make([][][]float32, len(entries))
		for i, entry := range entries {
			outputs[i] = make([][]float32, len(entry.work.readouts))
		}
		return outputs, false, nil
	}

	first := newDecisionJob(context.Background(), []decisionWork{decisionTestWork(2, 1)})
	second := newDecisionJob(context.Background(), []decisionWork{decisionTestWork(2, 1)})
	scheduler.requestQ <- first
	scheduler.requestQ <- second
	scheduler.start()
	t.Cleanup(scheduler.stop)

	if result := <-first.resultCh; !errors.Is(result.err, recoverableErr) {
		t.Fatalf("first job: got %v, want %v", result.err, recoverableErr)
	}
	if result := <-second.resultCh; result.err != nil {
		t.Fatalf("second job: got %v, want success", result.err)
	}
	if scheduler.stopped.Load() {
		t.Fatal("scheduler stopped after recoverable error")
	}
}

func decisionTestWork(tokens, readouts int) decisionWork {
	work := decisionWork{
		tokens: make([]llama.Token, tokens),
	}
	for i := range readouts {
		work.readouts = append(work.readouts, decisionReadout{
			position:   min(i, tokens-1),
			candidates: []llama.Token{1},
		})
	}
	return work
}

package modeltraining_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modeltraining "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining"
	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/tmprltest"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/mocks"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// This exercises the real stage implementations without a Temporal server or
// a native model. Only model computation is substituted.
func TestManagedWorkflowStages(t *testing.T) {
	for _, test := range []struct {
		name          string
		base, adapted int
		status        tmprl.Status
	}{
		{"accepted-at-threshold", 16, 18, tmprl.StatusCompleted},
		{"below-threshold", 16, 17, StatusNeedsImprovement},
		{"regressed-against-base", 19, 18, StatusNeedsImprovement},
	} {
		t.Run(test.name, func(t *testing.T) {
			modelPath := filepath.Join(t.TempDir(), "model.gguf")
			if err := os.WriteFile(modelPath, []byte("frozen model"), 0o600); err != nil {
				t.Fatal(err)
			}
			services := newServices(t, &mocks.Client{}, serviceConfig{
				DatasetRoot: t.TempDir(), Models: map[string]string{"gemma": modelPath},
				Trainer: workflowTrainer{base: test.base, adapted: test.adapted},
			})
			b := services.Training
			var source strings.Builder
			for i := range 60 {
				r := Record{Prompt: fmt.Sprintf("Question %d?", i), Target: fmt.Sprintf("Answer %d", i), Evidence: fmt.Sprintf("Evidence %d", i)}
				if err := json.NewEncoder(&source).Encode(r); err != nil {
					t.Fatal(err)
				}
			}
			imported, err := services.Datasets.ImportDataset(t.Context(), ImportInput{Name: "prepared.jsonl"}, strings.NewReader(source.String()))
			if err != nil {
				t.Fatal(err)
			}
			input := request("gemma")
			input.DatasetID = imported.ID
			input.Training.LoRARank = 8
			input.Training.LoRAAlpha = 8
			var suite testsuite.WorkflowTestSuite
			env := suite.NewTestWorkflowEnvironment()
			// The in-process harness only consumes session capacity here; real
			// worker concurrency is exercised by the server-backed tests below.
			env.SetWorkerOptions(worker.Options{EnableSessionWorker: true, MaxConcurrentSessionExecutionSize: 1})
			env.RegisterWorkflowWithOptions(modeltraining.Workflow, workflow.RegisterOptions{Name: workflowName})
			env.RegisterActivityWithOptions(b.ExecuteStage, activity.RegisterOptions{Name: modeltraining.SessionActivityName})
			env.ExecuteWorkflow(workflowName, workflowInput{Request: input, StageTimeout: time.Minute, Stages: []tmprl.Stage{StageSplit, StageTrain, StageEvaluate, StagePublish}, JobTimeout: time.Hour, QueueTimeout: time.Minute})
			if err := env.GetWorkflowError(); err != nil {
				t.Fatal(err)
			}
			var result tmprl.Run[Request, Artifacts]
			if err := env.GetWorkflowResult(&result); err != nil {
				t.Fatal(err)
			}
			accepted := test.status == tmprl.StatusCompleted
			if result.Status != test.status || result.Artifacts.Accepted != accepted {
				t.Fatalf("workflow result: %+v", result)
			}
			data, err := os.ReadFile(result.Artifacts.Report)
			if err != nil {
				t.Fatal(err)
			}
			var report EvaluationReport
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			if report.Total != 20 || report.BaseCorrect != test.base || report.AdapterCorrect != test.adapted || report.Accepted != accepted || len(report.Answers) != 20 {
				t.Fatalf("evaluation report: %+v", report)
			}
			adapter := filepath.Join(filepath.Dir(result.Artifacts.Report), "adapter.gguf")
			if !accepted {
				if result.Artifacts.Adapter != "" {
					t.Fatalf("rejected adapter exposed: %s", result.Artifacts.Adapter)
				}
				if _, err := os.Stat(adapter); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("rejected adapter published: %v", err)
				}
				return
			}
			data, err = os.ReadFile(result.Artifacts.Adapter)
			if err != nil || string(data) != "trained adapter" || result.Artifacts.Adapter != adapter {
				t.Fatalf("published adapter: %q, %v", data, err)
			}
		})
	}
}

type workflowTrainer struct{ base, adapted int }

func (workflowTrainer) Train(ctx context.Context, job TrainingJob, progress func(tmprl.Progress)) (TrainingCandidate, error) {
	if len(job.Dataset.Train) != 30 || len(job.Dataset.Validation) != 10 || len(job.Dataset.Test) != 0 {
		return TrainingCandidate{}, fmt.Errorf("incorrect splits or protected test data exposed to training")
	}
	path := filepath.Join(job.Workdir, "candidate.gguf")
	if err := os.WriteFile(path, []byte("trained adapter"), 0o600); err != nil {
		return TrainingCandidate{}, err
	}
	return TrainingCandidate{Path: path, BestEpoch: 2, ValidationLoss: 0.25}, nil
}

func (t workflowTrainer) Predict(ctx context.Context, job TrainingJob, adapter string, records []Record) ([]string, error) {
	correct := t.base
	if adapter != "" {
		data, err := os.ReadFile(adapter)
		if err != nil || string(data) != "trained adapter" {
			return nil, fmt.Errorf("candidate snapshot: %q, %v", data, err)
		}
		correct = t.adapted
	}
	answers := make([]string, len(records))
	for i, r := range records {
		answers[i] = "incorrect answer"
		if i < correct {
			answers[i] = "  " + strings.ToUpper(r.Target) + "\n"
		}
	}
	return answers, nil
}

// These integration tests bootstrap or reuse Temporal through tmprltest.
// They use unique queues and IDs; no shared histories are reset.
func liveBusiness(t *testing.T, backend modeltraining.Backend) *modeltraining.Business {
	t.Helper()
	env := tmprltest.New(t)
	b := newServices(t, env.Client, serviceConfig{TaskQueue: env.TaskQueue, Backend: backend, StageTimeout: 30 * time.Second}).Training
	w, err := b.NewWorker()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	return b
}

// Each output depends on the preceding durable artifact, so skipping a stage,
// dropping data, or reordering stages cannot produce the expected adapter.
func stageOutput(r LegacyStepRequest) Artifacts {
	switch r.Stage {
	case StagePrepare:
		return Artifacts{Dataset: "prepared:" + r.Request.Sources[1]}
	case StageTrain:
		return Artifacts{Checkpoint: "trained:" + r.Artifacts.Dataset}
	case StageEvaluate:
		return Artifacts{Report: "evaluated:" + r.Artifacts.Checkpoint, Accepted: true}
	case StagePublish:
		return Artifacts{Adapter: "published:" + r.Artifacts.Report}
	}
	return Artifacts{}
}

func pollRun(t *testing.T, b *modeltraining.Business, ctx context.Context, e tmprl.Execution, status tmprl.Status) tmprl.Run[Request, Artifacts] {
	t.Helper()
	for {
		r, err := b.Get(ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status == status {
			return r
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for %s; last run: %+v", status, r)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestTemporalPauseResumeProgressAndHistory(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var preparations atomic.Int32
	b := liveBusiness(t, backendFunc(func(ctx context.Context, r LegacyStepRequest, publish func(tmprl.Progress)) (Artifacts, error) {
		if r.Stage == StagePrepare {
			if preparations.Add(1) != 1 {
				return Artifacts{}, temporal.NewNonRetryableApplicationError("preparation already committed", "repeated-stage", nil)
			}
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return Artifacts{}, ctx.Err()
			}
			for i := range 101 {
				publish(tmprl.Progress{Completed: i, Total: 100, Message: "preparing"})
			}
		}
		return stageOutput(r), nil
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	input := request("model-a")
	input.Sources[1] = " document-b "
	run, err := b.Start(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = b.Stop(cleanup, run.Execution)
	})
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	updates, unsubscribe := b.Subscribe(run.Execution)
	defer unsubscribe()
	other, unsubOther := b.Subscribe(tmprl.Execution{WorkflowID: "other", RunID: "other"})
	defer unsubOther()
	if err := b.Pause(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
	pollRun(t, b, ctx, run.Execution, tmprl.StatusPauseRequested)
	close(release)
	paused := pollRun(t, b, ctx, run.Execution, tmprl.StatusPaused)
	if paused.Stage != "" || paused.Artifacts.Dataset != "prepared:document-b" || paused.Artifacts.Checkpoint != "" {
		t.Fatalf("pause boundary: %+v", paused)
	}
	select {
	case progress := <-updates:
		if progress.Execution != run.Execution || progress.Stage != StagePrepare || progress.Completed != 100 {
			t.Fatalf("latest progress: %+v", progress)
		}
	default:
		t.Fatal("missing progress")
	}
	select {
	case p := <-other:
		t.Fatalf("cross-run progress: %+v", p)
	default:
	}
	unsubscribe()
	unsubscribe()
	if _, open := <-updates; open {
		t.Fatal("subscription not closed")
	}
	if err := b.Resume(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
	completed, err := b.Wait(ctx, run.Execution)
	if err != nil {
		t.Fatal(err)
	}
	want := Artifacts{Dataset: "prepared:document-b", Checkpoint: "trained:prepared:document-b", Report: "evaluated:trained:prepared:document-b", Adapter: "published:evaluated:trained:prepared:document-b", Accepted: true}
	if completed.Execution != run.Execution || completed.Status != tmprl.StatusCompleted || completed.Artifacts != want || completed.Request.Training != input.Training || input.Sources[1] != " document-b " {
		t.Fatalf("result or caller input changed: %+v", completed)
	}
	stored, err := b.Get(ctx, run.Execution)
	if err != nil || !reflect.DeepEqual(stored, completed) {
		t.Fatalf("durable result: %+v %v", stored, err)
	}
	// Visibility indexing is eventually consistent. Check the public paged API.
	found := false
	for !found {
		var token []byte
		for {
			runs, next, err := b.History(ctx, 2, token)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range runs {
				if e == run.Execution {
					found = true
				}
			}
			if len(next) == 0 {
				break
			}
			token = next
		}
		if found {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("run missing from history")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func TestTemporalRejectedAndInvalidArtifacts(t *testing.T) {
	for _, mode := range []string{"reject", "invalid", "backend-failure"} {
		t.Run(mode, func(t *testing.T) {
			b := liveBusiness(t, backendFunc(func(ctx context.Context, r LegacyStepRequest, publish func(tmprl.Progress)) (Artifacts, error) {
				output := stageOutput(r)
				if mode == "backend-failure" && r.Stage == StageTrain {
					return Artifacts{}, errors.New("training device unavailable")
				}
				if mode == "invalid" && r.Stage == StageTrain {
					return Artifacts{}, nil
				}
				if mode == "reject" && r.Stage == StageEvaluate {
					output.Accepted = false
					output.Adapter = "must-not-leak"
				}
				return output, nil
			}))
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			r, err := b.Start(ctx, request("model-a"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := b.Wait(ctx, r.Execution)
			if mode == "invalid" || mode == "backend-failure" {
				if err == nil {
					t.Fatal("invalid stage output succeeded")
				}
				stored, getErr := b.Get(ctx, r.Execution)
				if getErr != nil || stored.Status != tmprl.StatusFailed || stored.Artifacts.Dataset != "prepared:document-b" || stored.Error == "" {
					t.Fatalf("failure state: %+v %v", stored, getErr)
				}
				wantAttempts := int32(1)
				if mode == "backend-failure" {
					wantAttempts = 3
				}
				if stored.Stage != StageTrain || stored.Stages[0].Status != tmprl.StatusCompleted || stored.Stages[1].Status != tmprl.StatusFailed || stored.Stages[1].Attempt != wantAttempts || stored.Stages[1].ClosedAt.IsZero() || stored.Stages[1].LastError == "" {
					t.Fatalf("failed stage diagnostics: %+v", stored)
				}
				if mode == "backend-failure" && !strings.Contains(stored.Error, "training device unavailable") {
					t.Fatalf("polling lost the backend cause: %+v", stored)
				}
				return
			}
			if err != nil || result.Status != StatusNeedsImprovement || result.Artifacts.Adapter != "" || result.Artifacts.Report != "evaluated:trained:prepared:document-b" {
				t.Fatalf("rejected candidate published: %+v %v", result, err)
			}
		})
	}
}

func TestTemporalStopIsPermanent(t *testing.T) {
	started := make(chan struct{}, 1)
	b := liveBusiness(t, backendFunc(func(ctx context.Context, r LegacyStepRequest, publish func(tmprl.Progress)) (Artifacts, error) {
		if r.Stage == StageTrain {
			publish(tmprl.Progress{Completed: 37, Total: 100, Message: "training"})
			started <- struct{}{}
			<-ctx.Done()
			return Artifacts{}, ctx.Err()
		}
		return stageOutput(r), nil
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	r, err := b.Start(ctx, request("model-a"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for {
		state, err := b.Get(ctx, r.Execution)
		if err != nil {
			t.Fatal(err)
		}
		if state.Progress != nil && state.Progress.Completed == 37 {
			if state.Progress.Stage != StageTrain || state.Progress.Total != 100 || state.Progress.Execution != r.Execution {
				t.Fatalf("heartbeat progress: %+v", state.Progress)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("heartbeat progress not persisted")
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err := b.Stop(ctx, r.Execution); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Wait(ctx, r.Execution); !temporal.IsCanceledError(err) {
		t.Fatalf("stop: %v", err)
	}
	stored, err := b.Get(ctx, r.Execution)
	if err != nil || stored.Status != tmprl.StatusStopped || stored.Stage != "" || stored.Artifacts.Dataset != "prepared:document-b" || stored.Artifacts.Adapter != "" {
		t.Fatalf("stopped state: %+v %v", stored, err)
	}
	if err := b.Resume(ctx, r.Execution); err == nil {
		t.Fatal("resumed a permanently stopped run")
	}
	// An explicit ID cannot silently create a second run after stopping.
	input := request("model-a")
	input.ID = r.WorkflowID
	if _, err := b.Start(ctx, input); err == nil {
		t.Fatal("reused a closed workflow ID")
	}
}

func TestTemporalRetryKeepsOperationIdentity(t *testing.T) {
	var attempts atomic.Int32
	var firstKey atomic.Value
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	b := liveBusiness(t, backendFunc(func(ctx context.Context, r LegacyStepRequest, publish func(tmprl.Progress)) (Artifacts, error) {
		if r.Stage == StageTrain {
			if attempts.Add(1) == 1 {
				firstKey.Store(r.Key)
				return Artifacts{}, errors.New("temporary engine failure")
			}
			if r.Key != firstKey.Load().(string) || r.Key != r.RunID+"/train" {
				return Artifacts{}, temporal.NewNonRetryableApplicationError("operation identity changed", "bad-key", nil)
			}
			publish(tmprl.Progress{Completed: 7, Total: 20, Message: "retry is running"})
			select {
			case <-release:
			case <-ctx.Done():
				return Artifacts{}, ctx.Err()
			}
		}
		return stageOutput(r), nil
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	r, err := b.Start(ctx, request("model-a"))
	if err != nil {
		t.Fatal(err)
	}
	for {
		state, err := b.Get(ctx, r.Execution)
		if err != nil {
			t.Fatal(err)
		}
		if state.Stage == StageTrain && state.Stages[1].Attempt == 2 && !state.Stages[1].LastHeartbeatAt.IsZero() && state.Progress != nil && state.Progress.Completed == 7 {
			if !strings.Contains(state.Stages[1].LastError, "temporary engine failure") || state.Stages[0].Status != tmprl.StatusCompleted || state.Stages[1].StartedAt.IsZero() {
				t.Fatalf("retry diagnostics: %+v", state)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("retry diagnostics not visible: %+v", state)
		case <-time.After(50 * time.Millisecond):
		}
	}
	unblock()
	result, err := b.Wait(ctx, r.Execution)
	if err != nil || result.Status != tmprl.StatusCompleted || result.Artifacts.Adapter != "published:evaluated:trained:prepared:document-b" {
		t.Fatalf("retry result: %+v %v", result, err)
	}
	stored, err := b.Get(ctx, r.Execution)
	if err != nil || stored.Stages[1].Attempt != 2 || !strings.Contains(stored.Stages[1].LastError, "temporary engine failure") {
		t.Fatalf("durable retry diagnostics: %+v %v", stored, err)
	}
}

type legacyStepFunc struct {
	name tmprl.Stage
	run  func(context.Context, StepContext) (tmprl.StepResult[Artifacts], error)
}

func (s legacyStepFunc) Name() string { return string(s.name) }
func (s legacyStepFunc) Execute(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
	return s.run(ctx, input)
}

// Exercise the public worker constructor against Temporal, not the workflow
// simulator. Arbitrary stage names prove registration, rather than a built-in
// stage switch, defines execution. A paused job must retain its worker slot.
func TestTemporalWholeJobWorkerOwnership(t *testing.T) {
	for _, workerCount := range []int{1, 2} {
		t.Run(fmt.Sprintf("workers-%d", workerCount), func(t *testing.T) {
			env := tmprltest.New(t)
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			type event struct {
				job, worker string
				stage       tmprl.Stage
			}
			started := make(chan event, workerCount+1)
			var mu sync.Mutex
			var events []event
			releases := make(map[string]chan struct{})
			closeRelease := make(map[string]func())
			for i := range workerCount + 1 {
				id := fmt.Sprintf("%s-job-%d", env.TaskQueue, i)
				ch := make(chan struct{})
				releases[id] = ch
				closeRelease[id] = sync.OnceFunc(func() { close(ch) })
				defer closeRelease[id]()
			}
			plan := []string{"load", "optimize", "score", "deliver"}
			var business *modeltraining.Business
			for i := range workerCount {
				workerID := fmt.Sprintf("worker-%d", i)
				stages := make([]TrainingStep, len(plan))
				for index, name := range plan {
					stages[index] = legacyStepFunc{name: tmprl.Stage(name), run: func(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
						want := strings.Join(plan[:index], "/")
						if input.Request.Artifacts.Checkpoint != want {
							return tmprl.StepResult[Artifacts]{}, fmt.Errorf("incoming checkpoint: got %q, want %q", input.Request.Artifacts.Checkpoint, want)
						}
						e := event{job: input.Request.WorkflowID, worker: workerID, stage: tmprl.Stage(name)}
						mu.Lock()
						events = append(events, e)
						mu.Unlock()
						if index == 0 {
							started <- e
							select {
							case <-releases[e.job]:
							case <-ctx.Done():
								return tmprl.StepResult[Artifacts]{}, ctx.Err()
							}
						}
						output := input.Request.Artifacts
						output.Checkpoint = strings.Join(plan[:index+1], "/")
						return tmprl.StepResult[Artifacts]{Artifacts: output}, nil
					}}
				}
				business = newServices(t, env.Client, serviceConfig{TaskQueue: env.TaskQueue, LegacySteps: stages, StageTimeout: 30 * time.Second}).Training
				w, err := business.NewWorker()
				if err != nil {
					t.Fatal(err)
				}
				if err := w.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(w.Stop)
			}
			var runs []tmprl.Run[Request, Artifacts]
			for i := range workerCount {
				input := request("controlled")
				input.ID = fmt.Sprintf("%s-job-%d", env.TaskQueue, i)
				run, err := business.Start(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				runs = append(runs, run)
			}
			var first event
			workers := make(map[string]bool)
			for i := range workerCount {
				select {
				case e := <-started:
					workers[e.worker] = true
					if i == 0 {
						first = e
					}
				case <-ctx.Done():
					t.Fatal("jobs did not run concurrently on available workers", ctx.Err())
				}
			}
			if len(workers) != workerCount {
				t.Fatalf("active workers: got %d, want %d", len(workers), workerCount)
			}
			var firstRun tmprl.Run[Request, Artifacts]
			for _, run := range runs {
				if run.WorkflowID == first.job {
					firstRun = run
				}
			}
			if err := business.Pause(ctx, firstRun.Execution); err != nil {
				t.Fatal(err)
			}
			pollRun(t, business, ctx, firstRun.Execution, tmprl.StatusPauseRequested)
			closeRelease[first.job]()
			pollRun(t, business, ctx, firstRun.Execution, tmprl.StatusPaused)
			input := request("controlled")
			input.ID = fmt.Sprintf("%s-job-%d", env.TaskQueue, workerCount)
			queued, err := business.Start(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			runs = append(runs, queued)
			// A single-activity concurrency cap would admit this job while the
			// first is paused; a whole-job reservation must not admit it.
			select {
			case e := <-started:
				t.Fatalf("reserved worker took another job: %+v", e)
			case <-time.After(500 * time.Millisecond):
			}
			if err := business.Resume(ctx, firstRun.Execution); err != nil {
				t.Fatal(err)
			}
			for _, release := range closeRelease {
				release()
			}
			for _, run := range runs {
				result, err := business.Wait(ctx, run.Execution)
				if err != nil || result.Status != tmprl.StatusCompleted || result.Artifacts.Checkpoint != "load/optimize/score/deliver" {
					t.Fatalf("job result: %+v, %v", result, err)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			byJob := make(map[string][]event)
			byWorker := make(map[string][]string)
			for _, e := range events {
				byJob[e.job] = append(byJob[e.job], e)
				byWorker[e.worker] = append(byWorker[e.worker], e.job)
			}
			for job, entries := range byJob {
				if len(entries) != len(plan) {
					t.Fatalf("job %s stages: got %d, want %d", job, len(entries), len(plan))
				}
				for i, e := range entries {
					if e.stage != tmprl.Stage(plan[i]) || e.worker != entries[0].worker {
						t.Fatalf("job changed worker or stage order: %+v", entries)
					}
				}
				t.Logf("job %s executed %v on %s", job, plan, entries[0].worker)
			}
			for worker, jobs := range byWorker {
				seen := make(map[string]bool)
				last := ""
				for _, job := range jobs {
					if job != last {
						if seen[job] {
							t.Fatalf("worker %s interleaved jobs: %v", worker, jobs)
						}
						seen[job] = true
						last = job
					}
				}
			}
		})
	}
}

func TestTemporalWorkerRecoveryKeepsCommittedStages(t *testing.T) {
	env := tmprltest.New(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var captures atomic.Int32
	stages := func(workerID string) []TrainingStep {
		return []TrainingStep{
			legacyStepFunc{name: "capture", run: func(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
				if captures.Add(1) != 1 {
					return tmprl.StepResult[Artifacts]{}, temporal.NewNonRetryableApplicationError("committed stage repeated", "repeated-stage", nil)
				}
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return tmprl.StepResult[Artifacts]{}, ctx.Err()
				}
				return tmprl.StepResult[Artifacts]{Artifacts: Artifacts{Checkpoint: "committed by " + workerID}}, nil
			}},
			legacyStepFunc{name: "finish", run: func(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
				if input.Request.Artifacts.Checkpoint != "committed by original" {
					return tmprl.StepResult[Artifacts]{}, fmt.Errorf("committed artifacts lost: %+v", input.Request.Artifacts)
				}
				output := input.Request.Artifacts
				output.Adapter = "finished by " + workerID
				return tmprl.StepResult[Artifacts]{Artifacts: output}, nil
			}},
		}
	}
	b := newServices(t, env.Client, serviceConfig{TaskQueue: env.TaskQueue, LegacySteps: stages("original")}).Training
	w, err := b.NewWorker()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	stop := sync.OnceFunc(w.Stop)
	t.Cleanup(stop)
	run, err := b.Start(ctx, request("controlled"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := b.Pause(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
	pollRun(t, b, ctx, run.Execution, tmprl.StatusPauseRequested)
	unblock()
	pollRun(t, b, ctx, run.Execution, tmprl.StatusPaused)
	stop()
	pollCtx, stopPoll := context.WithTimeout(ctx, 2*time.Second)
	lastKnown, err := b.Get(pollCtx, run.Execution)
	stopPoll()
	if err != nil || lastKnown.Status != tmprl.StatusPaused || lastKnown.Stages[0].Status != tmprl.StatusCompleted || lastKnown.Artifacts.Checkpoint != "committed by original" {
		t.Fatalf("workerless paused job: %+v, %v", lastKnown, err)
	}
	// Allow orchestration to detect worker loss while no session execution
	// capacity exists. This keeps the recovering state observable until replaced.
	observer := worker.New(env.Client, env.TaskQueue, worker.Options{LocalActivityWorkerOnly: true})
	observer.RegisterWorkflowWithOptions(modeltraining.Workflow, workflow.RegisterOptions{Name: modeltraining.WorkflowName})
	if err := observer.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observer.Stop)
	recovering := pollRun(t, b, ctx, run.Execution, tmprl.StatusRecovering)
	if recovering.SessionAttempt != 2 || !recovering.ReservationDeadline.IsZero() || recovering.WorkerHost != "" || recovering.Stages[0].Status != tmprl.StatusCompleted {
		t.Fatalf("recovering job: %+v", recovering)
	}
	replacement := newServices(t, env.Client, serviceConfig{TaskQueue: env.TaskQueue, LegacySteps: stages("replacement")}).Training
	w, err = replacement.NewWorker()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	if err := replacement.Resume(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
	result, err := replacement.Wait(ctx, run.Execution)
	if err != nil || result.Status != tmprl.StatusCompleted || result.Artifacts.Adapter != "finished by replacement" || captures.Load() != 1 || result.SessionAttempt != 2 || result.Stages[0].Status != tmprl.StatusCompleted || result.Stages[1].Status != tmprl.StatusCompleted {
		t.Fatalf("recovered job: %+v, captures=%d, error=%v", result, captures.Load(), err)
	}
}

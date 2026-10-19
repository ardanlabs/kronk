package training_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/tmprltest"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/mocks"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// This exercises the real stage implementations without a Temporal server or
// a native model. Only model computation is substituted.
func TestManagedWorkflowStages(t *testing.T) {
	for _, test := range []struct {
		name          string
		base, adapted int
		status        Status
	}{
		{"accepted-at-threshold", 16, 18, StatusCompleted},
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
			env.RegisterWorkflowWithOptions(training.Workflow, workflow.RegisterOptions{Name: workflowName})
			env.RegisterActivityWithOptions(b.RunStage, activity.RegisterOptions{Name: activityName})
			env.ExecuteWorkflow(workflowName, workflowInput{Request: input, StageTimeout: time.Minute})
			if err := env.GetWorkflowError(); err != nil {
				t.Fatal(err)
			}
			var result Run
			if err := env.GetWorkflowResult(&result); err != nil {
				t.Fatal(err)
			}
			accepted := test.status == StatusCompleted
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

func (workflowTrainer) Train(ctx context.Context, job TrainingJob, progress func(Progress)) (TrainingCandidate, error) {
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
func liveBusiness(t *testing.T, backend training.Backend) *training.Business {
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
func stageOutput(r StageRequest) Artifacts {
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

func pollRun(t *testing.T, b *training.Business, ctx context.Context, e Execution, status Status) Run {
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
	b := liveBusiness(t, backendFunc(func(ctx context.Context, r StageRequest, publish func(Progress)) (Artifacts, error) {
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
				publish(Progress{Completed: i, Total: 100, Message: "preparing"})
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
	other, unsubOther := b.Subscribe(Execution{WorkflowID: "other", RunID: "other"})
	defer unsubOther()
	if err := b.Pause(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
	pollRun(t, b, ctx, run.Execution, StatusPauseRequested)
	close(release)
	paused := pollRun(t, b, ctx, run.Execution, StatusPaused)
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
	if completed.Execution != run.Execution || completed.Status != StatusCompleted || completed.Artifacts != want || completed.Request.Training != input.Training || input.Sources[1] != " document-b " {
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
			b := liveBusiness(t, backendFunc(func(ctx context.Context, r StageRequest, publish func(Progress)) (Artifacts, error) {
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
				if getErr != nil || stored.Status != StatusFailed || stored.Artifacts.Dataset != "prepared:document-b" || stored.Error == "" {
					t.Fatalf("failure state: %+v %v", stored, getErr)
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
	b := liveBusiness(t, backendFunc(func(ctx context.Context, r StageRequest, publish func(Progress)) (Artifacts, error) {
		if r.Stage == StageTrain {
			publish(Progress{Completed: 37, Total: 100, Message: "training"})
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
	if err != nil || stored.Status != StatusStopped || stored.Stage != "" || stored.Artifacts.Dataset != "prepared:document-b" || stored.Artifacts.Adapter != "" {
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
	b := liveBusiness(t, backendFunc(func(ctx context.Context, r StageRequest, publish func(Progress)) (Artifacts, error) {
		if r.Stage == StageTrain {
			if attempts.Add(1) == 1 {
				firstKey.Store(r.Key)
				return Artifacts{}, errors.New("temporary engine failure")
			}
			if r.Key != firstKey.Load().(string) || r.Key != r.RunID+"/train" {
				return Artifacts{}, temporal.NewNonRetryableApplicationError("operation identity changed", "bad-key", nil)
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
	result, err := b.Wait(ctx, r.Execution)
	if err != nil || result.Status != StatusCompleted || result.Artifacts.Adapter != "published:evaluated:trained:prepared:document-b" {
		t.Fatalf("retry result: %+v %v", result, err)
	}
}

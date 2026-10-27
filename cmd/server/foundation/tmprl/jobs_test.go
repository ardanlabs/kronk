package tmprl_test

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

// TestJobsLifecycle exercises the production runner against real Temporal with
// controlled handlers. Each job has two stages with two steps apiece.
func TestJobsLifecycle(t *testing.T) {
	for _, test := range []struct {
		name          string
		mode          string
		at            int
		failures      int
		wantStatus    tmprl.Status
		wantCompleted int
		wantAttempts  [4]int
	}{
		{name: "all-pass", wantStatus: tmprl.StatusCompleted, wantCompleted: 4, wantAttempts: [4]int{1, 1, 1, 1}},
		{name: "first-stage-first-step-fails", mode: "permanent", at: 1, wantStatus: tmprl.StatusFailed, wantAttempts: [4]int{1}},
		{name: "first-stage-second-step-fails", mode: "permanent", at: 2, wantStatus: tmprl.StatusFailed, wantCompleted: 1, wantAttempts: [4]int{1, 1}},
		{name: "second-stage-first-step-fails", mode: "permanent", at: 3, wantStatus: tmprl.StatusFailed, wantCompleted: 2, wantAttempts: [4]int{1, 1, 1}},
		{name: "second-stage-second-step-fails", mode: "permanent", at: 4, wantStatus: tmprl.StatusFailed, wantCompleted: 3, wantAttempts: [4]int{1, 1, 1, 1}},
		{name: "first-step-recovers", mode: "retry", at: 1, failures: 1, wantStatus: tmprl.StatusCompleted, wantCompleted: 4, wantAttempts: [4]int{2, 1, 1, 1}},
		{name: "last-step-recovers-on-final-attempt", mode: "retry", at: 4, failures: 2, wantStatus: tmprl.StatusCompleted, wantCompleted: 4, wantAttempts: [4]int{1, 1, 1, 3}},
		{name: "retries-exhausted", mode: "retry", at: 3, failures: 3, wantStatus: tmprl.StatusFailed, wantCompleted: 2, wantAttempts: [4]int{1, 1, 3}},
		{name: "step-timeout-exhausts-retries", mode: "step-timeout", at: 3, wantStatus: tmprl.StatusFailed, wantCompleted: 2, wantAttempts: [4]int{1, 1, 3}},
		{name: "heartbeat-error-with-live-session", mode: "heartbeat-timeout", at: 3, wantStatus: tmprl.StatusFailed, wantCompleted: 2, wantAttempts: [4]int{1, 1, 3}},
		{name: "stage-deadline", mode: "stage-timeout", at: 3, wantStatus: tmprl.StatusFailed, wantCompleted: 2, wantAttempts: [4]int{1, 1, 1}},
		{name: "cancel-running-step", mode: "cancel", at: 3, wantStatus: tmprl.StatusStopped, wantCompleted: 2, wantAttempts: [4]int{1, 1, 1}},
		{name: "pause-at-step-boundary-and-resume", mode: "pause", at: 2, wantStatus: tmprl.StatusCompleted, wantCompleted: 4, wantAttempts: [4]int{1, 1, 1, 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			job := tmprl.Job{
				Name: "lifecycle", QueueTimeout: 30 * time.Second, ReservationTimeout: time.Minute, SessionAttempts: 1,
			}
			if test.mode == "heartbeat-timeout" {
				job.SessionAttempts = 2 // A healthy session must not consume recovery attempts.
			}
			keys := []string{"first/one", "first/two", "second/one", "second/two"}
			var mu sync.Mutex
			var attempts [4]int
			entered, release := make(chan struct{}, 1), make(chan struct{})
			handlers := make(map[string]tmprl.StepHandler[string, []string])
			for stageIndex, name := range []string{"first", "second"} {
				stage := tmprl.Stage{Name: name, Timeout: 30 * time.Second}
				for stepIndex, name := range []string{"one", "two"} {
					index := stageIndex*2 + stepIndex
					step := tmprl.Step{
						Name: name, Timeout: 10 * time.Second, HeartbeatTimeout: 200 * time.Millisecond,
						Retry: tmprl.Retry{MaximumAttempts: 3, InitialInterval: 100 * time.Millisecond, MaximumInterval: 200 * time.Millisecond, BackoffCoefficient: 2},
					}
					if index+1 == test.at {
						switch test.mode {
						case "step-timeout":
							step.Timeout = time.Second
						case "stage-timeout":
							stage.Timeout = time.Second
						}
					}
					stage.Steps = append(stage.Steps, step)
					handlers[keys[index]] = tmprl.StepFunc[string, []string](func(ctx context.Context, input tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
						mu.Lock()
						attempts[index]++
						attempt := attempts[index]
						mu.Unlock()
						if index+1 == test.at {
							switch test.mode {
							case "permanent":
								return tmprl.StepResult[[]string]{}, temporal.NewNonRetryableApplicationError("controlled failure", "controlled-failure", nil)
							case "heartbeat-timeout":
								return tmprl.StepResult[[]string]{}, temporal.NewHeartbeatTimeoutError()
							case "retry":
								if attempt <= test.failures {
									return tmprl.StepResult[[]string]{}, temporal.NewApplicationError("controlled failure", "controlled-failure")
								}
							case "step-timeout", "stage-timeout":
								<-ctx.Done()
								return tmprl.StepResult[[]string]{}, ctx.Err()
							case "cancel", "pause":
								entered <- struct{}{}
								select {
								case <-ctx.Done():
									return tmprl.StepResult[[]string]{}, ctx.Err()
								case <-release:
								}
							}
						}
						return tmprl.StepResult[[]string]{Artifacts: append(slices.Clone(input.Request.Artifacts), keys[index])}, nil
					})
				}
				job.Stages = append(job.Stages, stage)
			}
			j, err := tmprl.New(env.Client, tmprl.Config[string, []string]{
				Names:     tmprl.Names{Workflow: "example.Lifecycle", Activity: "example.Step", Memo: "example.state", Query: "example-state", Control: "example-control"},
				TaskQueue: env.TaskQueue, StorageRoot: t.TempDir(), StorageDirectory: "lifecycle", Steps: handlers, Log: env.Log,
			})
			if err != nil {
				t.Fatal(err)
			}
			w := j.NewWorker()
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
			run, err := j.Submit(ctx, "lifecycle-"+rand.Text(), tmprl.JobInput[string]{Job: &job, Request: "controlled"})
			if err != nil {
				t.Fatal(err)
			}
			if test.mode == "cancel" || test.mode == "pause" {
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				if test.mode == "cancel" {
					if err := j.Stop(ctx, run.Execution); err != nil {
						t.Fatal(err)
					}
				} else {
					awaitStatus := func(want tmprl.Status) {
						t.Helper()
						ticker := time.NewTicker(20 * time.Millisecond)
						defer ticker.Stop()
						for {
							state, err := j.Get(ctx, run.Execution)
							if err != nil {
								t.Fatal(err)
							}
							if state.Status == want {
								return
							}
							select {
							case <-ctx.Done():
								t.Fatalf("status: got %s, want %s: %v", state.Status, want, ctx.Err())
							case <-ticker.C:
							}
						}
					}
					if err := j.Pause(ctx, run.Execution); err != nil {
						t.Fatal(err)
					}
					awaitStatus(tmprl.StatusPauseRequested)
					close(release)
					awaitStatus(tmprl.StatusPaused)
					mu.Lock()
					pausedAttempts := attempts
					mu.Unlock()
					if pausedAttempts != [4]int{1, 1, 0, 0} {
						t.Fatalf("paused attempts: got %v, want [1 1 0 0]", pausedAttempts)
					}
					if err := j.Resume(ctx, run.Execution); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, waitErr := j.Wait(ctx, run.Execution)
			if (waitErr != nil) != (test.wantStatus != tmprl.StatusCompleted) {
				t.Fatalf("Wait: got %v, want status %s", waitErr, test.wantStatus)
			}
			if test.wantStatus == tmprl.StatusStopped && !temporal.IsCanceledError(waitErr) {
				t.Fatalf("Wait: got %v, want cancellation", waitErr)
			}
			if test.mode == "step-timeout" || test.mode == "heartbeat-timeout" {
				if _, ok := errors.AsType[*temporal.TimeoutError](waitErr); !ok {
					t.Fatalf("Wait: got %v, want timeout", waitErr)
				}
			}
			if test.wantStatus == tmprl.StatusFailed && test.mode != "step-timeout" && test.mode != "heartbeat-timeout" {
				wantType := "controlled-failure"
				if test.mode == "stage-timeout" {
					wantType = "stage-expired"
				}
				failure, ok := errors.AsType[*temporal.ApplicationError](waitErr)
				if !ok || failure.Type() != wantType {
					t.Fatalf("Wait: got %v, want application failure %s", waitErr, wantType)
				}
			}
			closed, err := j.Get(ctx, run.Execution)
			if err != nil {
				t.Fatal(err)
			}
			if closed.Status != test.wantStatus || closed.Execution != run.Execution || closed.Request != "controlled" || closed.ClosedAt.IsZero() {
				t.Fatalf("closed run: got %+v, want status %s with original identity/request and closure time", closed, test.wantStatus)
			}
			if closed.SessionAttempt != 1 || closed.WorkerHost == "" {
				t.Errorf("worker reservation: got attempt %d, host %q, want one identified reservation", closed.SessionAttempt, closed.WorkerHost)
			}
			if !slices.Equal(closed.Artifacts, keys[:test.wantCompleted]) {
				t.Errorf("committed steps: got %v, want %v", closed.Artifacts, keys[:test.wantCompleted])
			}
			if waitErr == nil && (result.Status != closed.Status || !slices.Equal(result.Artifacts, closed.Artifacts)) {
				t.Errorf("Wait/Get mismatch: got %+v, want %+v", result, closed)
			}
			if test.wantStatus != tmprl.StatusCompleted && closed.Error == "" {
				t.Error("closed error: got empty, want failure diagnostics")
			}
			mu.Lock()
			gotAttempts := attempts
			mu.Unlock()
			if gotAttempts != test.wantAttempts {
				t.Errorf("attempts: got %v, want %v", gotAttempts, test.wantAttempts)
			}
			if len(closed.Stages) != 2 {
				t.Fatalf("stages: got %d, want 2", len(closed.Stages))
			}
			for stageIndex, stage := range closed.Stages {
				wantStage := tmprl.StatusQueued
				if test.wantCompleted >= (stageIndex+1)*2 {
					wantStage = tmprl.StatusCompleted
				} else if test.wantStatus != tmprl.StatusCompleted && stageIndex == (test.at-1)/2 {
					wantStage = test.wantStatus
				}
				if stage.Status != wantStage || len(stage.Steps) != 2 {
					t.Fatalf("stage %s: got %+v, want status %s and 2 steps", stage.Name, stage, wantStage)
				}
				for stepIndex, step := range stage.Steps {
					index := stageIndex*2 + stepIndex
					wantStep := tmprl.StatusQueued
					if index < test.wantCompleted {
						wantStep = tmprl.StatusCompleted
					} else if test.wantStatus != tmprl.StatusCompleted && index+1 == test.at {
						wantStep = test.wantStatus
					}
					if step.Status != wantStep {
						t.Errorf("step %s/%s: got %s, want %s", stage.Name, step.Name, step.Status, wantStep)
					}
				}
			}
		})
	}
}

// A job with scalar inputs/artifacts establishes that the foundation has no
// training payload, acceptance policy, storage layout, or step-name dependency.
func TestJobsApplicationIndependent(t *testing.T) {
	for _, test := range []struct {
		name    string
		outcome tmprl.Status
		want    int
	}{
		{name: "all-steps", want: 34},
		{name: "application-outcome", outcome: "review_required", want: 39},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			policy := tmprl.Step{
				Timeout: time.Minute, HeartbeatTimeout: 5 * time.Second,
				Retry: tmprl.Retry{MaximumAttempts: 2, InitialInterval: time.Millisecond, MaximumInterval: time.Second, BackoffCoefficient: 2},
			}
			add, multiply, subtract := policy, policy, policy
			add.Name, multiply.Name, subtract.Name = "add", "multiply", "subtract"
			job := tmprl.Job{
				Name: "arithmetic", QueueTimeout: time.Minute, ReservationTimeout: time.Minute, SessionAttempts: 2,
				Stages: []tmprl.Stage{
					{Name: "calculate", Timeout: time.Minute, Steps: []tmprl.Step{add, multiply}},
					{Name: "finish", Timeout: time.Minute, Steps: []tmprl.Step{subtract}},
				},
			}
			j, err := tmprl.New(env.Client, tmprl.Config[int, int]{
				Names:     tmprl.Names{Workflow: "example.Arithmetic", Activity: "example.Calculate", Memo: "example.state", Query: "example-state", Control: "example-control"},
				TaskQueue: env.TaskQueue, StorageRoot: t.TempDir(), StorageDirectory: "calculations", Log: env.Log,
				Outcomes: []tmprl.Status{"review_required"},
				Steps: map[string]tmprl.StepHandler[int, int]{
					"calculate/add": tmprl.StepFunc[int, int](func(ctx context.Context, input tmprl.StepContext[int, int]) (tmprl.StepResult[int], error) {
						return tmprl.StepResult[int]{Artifacts: input.Request.Request + 7}, nil
					}),
					"calculate/multiply": tmprl.StepFunc[int, int](func(ctx context.Context, input tmprl.StepContext[int, int]) (tmprl.StepResult[int], error) {
						input.Publish(tmprl.Progress{Completed: 1, Total: 1})
						return tmprl.StepResult[int]{Artifacts: input.Request.Artifacts * 3, Status: test.outcome}, nil
					}),
					"finish/subtract": tmprl.StepFunc[int, int](func(ctx context.Context, input tmprl.StepContext[int, int]) (tmprl.StepResult[int], error) {
						return tmprl.StepResult[int]{Artifacts: input.Request.Artifacts - 5}, nil
					}),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			run, err := j.Submit(ctx, "arithmetic-"+rand.Text(), tmprl.JobInput[int]{Job: &job, Request: 6})
			if err != nil {
				t.Fatal(err)
			}
			queued, err := j.Get(ctx, run.Execution)
			if err != nil || queued.Status != tmprl.StatusQueued || queued.Request != 6 {
				t.Fatalf("workerless state: got %+v, %v, want queued request 6", queued, err)
			}
			w := j.NewWorker()
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			defer w.Stop()
			result, err := j.Wait(ctx, run.Execution)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus, wantFinish := tmprl.StatusCompleted, tmprl.StatusCompleted
			if test.outcome != "" {
				wantStatus, wantFinish = test.outcome, tmprl.StatusQueued
			}
			if result.Artifacts != test.want || result.Status != wantStatus || result.Request != 6 || result.SessionAttempt != 1 || result.WorkerHost == "" {
				t.Fatalf("result: got %+v, want artifacts %d, status %s, request 6 in one reservation", result, test.want, wantStatus)
			}
			if len(result.Stages) != 2 || result.Stages[0].Steps[1].Status != tmprl.StatusCompleted || result.Stages[1].Status != wantFinish {
				t.Fatalf("step plan: got %+v, want second stage %s", result.Stages, wantFinish)
			}
			closed, err := j.Get(ctx, run.Execution)
			if err != nil || closed.Status != wantStatus || closed.Artifacts != test.want || closed.Execution != run.Execution {
				t.Fatalf("closed state: got %+v, %v, want %s with artifacts %d", closed, err, wantStatus, test.want)
			}
		})
	}
}

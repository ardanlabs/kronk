package tmprl_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
)

// TestJobsWorkerRecovery kills a worker process, losing its session and workflow
// cache. The replacement must replay history and resume only unfinished work.
func TestJobsWorkerRecovery(t *testing.T) {
	const probe = "MOXIE_TMPRL_RECOVERY_QUEUE"
	if queue := os.Getenv(probe); queue != "" {
		env := newTestEnvironment(t)
		jobs := recoveryJobs(t, env, queue, os.Getenv("MOXIE_TMPRL_RECOVERY_STORAGE"), true)
		w := jobs.NewWorker()
		if err := w.Run(nil); err != nil {
			t.Fatal(err)
		}
		return
	}

	for _, test := range []struct {
		name       string
		heartbeat  time.Duration
		sessions   int
		checkpoint bool
		accepted   bool
		stageWait  bool
	}{
		{name: "step-heartbeat-first", heartbeat: 2 * time.Second, sessions: 2},
		{name: "session-heartbeat-first", heartbeat: 10 * time.Second, sessions: 2},
		{name: "session-attempts-exhausted", heartbeat: 2 * time.Second, sessions: 1},
		{name: "checkpoint-wait", heartbeat: 2 * time.Second, sessions: 2, checkpoint: true},
		{name: "checkpoint-accepted", heartbeat: 2 * time.Second, sessions: 2, checkpoint: true, accepted: true},
		{name: "stage-wait", heartbeat: 2 * time.Second, sessions: 2, checkpoint: true, stageWait: true},
		{name: "stage-accepted", heartbeat: 2 * time.Second, sessions: 2, checkpoint: true, accepted: true, stageWait: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			testWorkerRecovery(t, test.heartbeat, test.sessions, test.checkpoint, test.accepted, test.stageWait)
		})
	}
}

func testWorkerRecovery(t *testing.T, heartbeat time.Duration, sessions int, checkpoint, accepted, stageWait bool) {
	t.Helper()
	env := newTestEnvironment(t)
	root := t.TempDir()
	jobs := recoveryJobs(t, env, env.TaskQueue, root, false)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestJobsWorkerRecovery$", "-test.v")
	cmd.Env = append(os.Environ(), "MOXIE_TMPRL_RECOVERY_QUEUE="+env.TaskQueue, "MOXIE_TMPRL_RECOVERY_STORAGE="+root)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr logBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error(err)
			}
			if err := cmd.Wait(); err != nil {
				env.Log(context.Background(), "recovery probe exited", "error", err)
			}
		}
		if t.Failed() {
			t.Logf("worker stderr:\n%s", stderr.String())
		}
	})
	entered := make(chan struct{})
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "recovery-step-entered" {
				close(entered)
			}
			env.Log(context.Background(), "recovery worker output", "line", line)
		}
		if err := scanner.Err(); err != nil {
			env.Log(context.Background(), "reading recovery worker output", "error", err)
		}
	}()
	plan := recoveryPlan()
	plan.SessionAttempts = sessions
	if checkpoint {
		plan.Stages[0].Steps[0].WaitAfter = "review"
	}
	for i := range plan.Stages[0].Steps {
		plan.Stages[0].Steps[i].HeartbeatTimeout = heartbeat
	}
	if stageWait {
		finish := plan.Stages[0].Steps[1]
		plan.Stages[0].Steps = plan.Stages[0].Steps[:1]
		plan.Stages[0].Steps[0].WaitAfter = ""
		plan.Stages[0].WaitAfter, plan.Stages[0].Timeout = "review", time.Second
		plan.Stages = append(plan.Stages, tmprl.Stage{Name: "next", Timeout: time.Minute, Steps: []tmprl.Step{finish}})
	}
	run, err := jobs.Submit(ctx, "recovery-"+rand.Text(), tmprl.JobInput[string]{Job: &plan, Request: "original"})
	if err != nil {
		t.Fatal(err)
	}
	var decision tmprl.Decision
	if checkpoint {
		waiting := awaitCheckpointState(t, ctx, jobs, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status == tmprl.StatusWaiting })
		decision = tmprl.Decision{CheckpointID: waiting.Checkpoint.ID, SubmissionID: "accepted-review"}
		if accepted {
			if _, err := jobs.Decide(ctx, run.Execution, decision); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !checkpoint || accepted {
		select {
		case <-entered:
		case <-readDone:
			t.Fatalf("worker exited before unfinished step: %s", stderr.String())
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	before, err := jobs.Get(ctx, run.Execution)
	if err != nil || !slices.Equal(before.Artifacts, []string{"committed"}) || before.Stages[0].Steps[0].Status != tmprl.StatusCompleted {
		t.Fatalf("before crash: got %+v, %v, want first step committed", before, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-readDone
	if err := cmd.Wait(); err == nil {
		t.Fatal("worker exit: got success, want killed process")
	}
	w := jobs.NewWorker()
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	if checkpoint && !accepted {
		recovered := awaitCheckpointState(t, ctx, jobs, run.Execution, func(s tmprl.Run[string, []string]) bool {
			return s.Status == tmprl.StatusWaiting && s.SessionAttempt == 2
		})
		if recovered.Checkpoint.ID != decision.CheckpointID || recovered.Stages[0].Steps[0].Status != tmprl.StatusCompleted {
			t.Fatalf("lost checkpoint: %+v", recovered)
		}
		if stageWait && (recovered.Stages[0].Status != tmprl.StatusCompleted || !recovered.Stages[0].ClosedAt.Equal(before.Stages[0].ClosedAt)) {
			t.Fatalf("completed stage changed on recovery: %+v", recovered)
		}
		if _, err := jobs.Decide(ctx, run.Execution, decision); err != nil {
			t.Fatal(err)
		}
	}
	result, err := jobs.Wait(ctx, run.Execution)
	if sessions == 1 {
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("exhausted recovery: got %v, want terminal failure", err)
		}
		closed, err := jobs.Get(ctx, run.Execution)
		if err != nil || closed.Status != tmprl.StatusFailed || closed.SessionAttempt != 1 || !slices.Equal(closed.Artifacts, []string{"committed"}) || closed.Error == "" {
			t.Fatalf("exhausted polling: got %+v, %v, want failed first session with committed artifacts", closed, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionAttempt != 2 || result.Status != tmprl.StatusCompleted || !slices.Equal(result.Artifacts, []string{"committed", "recovered"}) {
		t.Fatalf("recovery: got %+v, want second session and committed/recovered artifacts", result)
	}
	lastStage := result.Stages[len(result.Stages)-1]
	if result.Stages[0].Steps[0].Attempt != 1 || lastStage.Steps[len(lastStage.Steps)-1].Status != tmprl.StatusCompleted {
		t.Fatalf("recovered steps: got %+v, want committed first step and completed second step", result.Stages)
	}
	if checkpoint && (len(result.Decisions) != 1 || result.Decisions[0].SubmissionID != decision.SubmissionID || result.Checkpoint != nil) {
		t.Fatalf("lost receipt: %+v", result)
	}
	closed, err := jobs.Get(ctx, run.Execution)
	if err != nil || closed.Status != result.Status || !slices.Equal(closed.Artifacts, result.Artifacts) || closed.Execution != run.Execution {
		t.Fatalf("recovered polling: got %+v, %v, want terminal workflow result", closed, err)
	}
}

func recoveryJobs(t *testing.T, env *Environment, queue, root string, child bool) *tmprl.Jobs[string, []string] {
	t.Helper()
	cfg := tmprl.Config[string, []string]{
		Names:     tmprl.Names{Workflow: "example.Recovery", Activity: "example.RecoveryStep", Memo: "example.recovery", Query: "recovery-state", Control: "recovery-control"},
		TaskQueue: queue, StorageRoot: root, StorageDirectory: "recovery", Log: env.Log,
		Steps: map[string]tmprl.StepHandler[string, []string]{
			"work/commit": tmprl.StepFunc[string, []string](func(context.Context, tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
				if !child {
					return tmprl.StepResult[[]string]{}, temporal.NewNonRetryableApplicationError("committed step executed again", "repeated-step", nil)
				}
				return tmprl.StepResult[[]string]{Artifacts: []string{"committed"}}, nil
			}),
			"work/finish": tmprl.StepFunc[string, []string](func(ctx context.Context, input tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
				if child {
					fmt.Println("recovery-step-entered")
					<-ctx.Done()
					return tmprl.StepResult[[]string]{}, ctx.Err()
				}
				if input.Request.Key != input.Request.RunID+"/"+input.Request.Stage+"/finish" || input.Request.Request != "original" || !slices.Equal(input.Request.Artifacts, []string{"committed"}) {
					return tmprl.StepResult[[]string]{}, temporal.NewNonRetryableApplicationError("recovery lost input identity or committed artifacts", "invalid-recovery", nil)
				}
				if input.Request.Decisions != nil && (len(input.Request.Decisions) != 1 || input.Request.Decisions[0].SubmissionID != "accepted-review") {
					return tmprl.StepResult[[]string]{}, errors.New("recovery lost accepted input")
				}
				return tmprl.StepResult[[]string]{Artifacts: []string{"committed", "recovered"}}, nil
			}),
		},
	}
	cfg.Steps["next/finish"] = cfg.Steps["work/finish"]
	jobs, err := tmprl.New(env.Client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return jobs
}

func recoveryPlan() tmprl.Job {
	step := tmprl.Step{
		Name: "commit", Timeout: 30 * time.Second, HeartbeatTimeout: 10 * time.Second,
		Retry: tmprl.Retry{MaximumAttempts: 1, InitialInterval: time.Millisecond, MaximumInterval: time.Second, BackoffCoefficient: 1},
	}
	finish := step
	finish.Name = "finish"
	return tmprl.Job{
		Name: "recovery", QueueTimeout: 30 * time.Second, ReservationTimeout: time.Minute, SessionAttempts: 2,
		Stages: []tmprl.Stage{{Name: "work", Timeout: time.Minute, Steps: []tmprl.Step{step, finish}}},
	}
}

// TestJobsDeadlines checks terminal polling when no worker can reserve a job,
// and when active or paused work exhausts the reservation budget.
func TestJobsDeadlines(t *testing.T) {
	for _, mode := range []string{"queue-expired", "stop-queued", "reservation-expired", "paused-reservation-expired"} {
		t.Run(mode, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			entered, release := make(chan struct{}, 1), make(chan struct{})
			jobs, err := tmprl.New(env.Client, tmprl.Config[string, int]{
				Names:     tmprl.Names{Workflow: "example.Deadline", Activity: "example.DeadlineStep", Memo: "example.deadline", Query: "deadline-state", Control: "deadline-control"},
				TaskQueue: env.TaskQueue, StorageRoot: t.TempDir(), StorageDirectory: "deadlines", Log: env.Log,
				Steps: map[string]tmprl.StepHandler[string, int]{
					"work/hold": tmprl.StepFunc[string, int](func(ctx context.Context, _ tmprl.StepContext[string, int]) (tmprl.StepResult[int], error) {
						entered <- struct{}{}
						select {
						case <-ctx.Done():
							return tmprl.StepResult[int]{}, ctx.Err()
						case <-release:
							return tmprl.StepResult[int]{Artifacts: 17}, nil
						}
					}),
					"work/never": tmprl.StepFunc[string, int](func(context.Context, tmprl.StepContext[string, int]) (tmprl.StepResult[int], error) {
						return tmprl.StepResult[int]{}, temporal.NewNonRetryableApplicationError("step ran past reservation", "unexpected-step", nil)
					}),
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			options := jobs.WorkerOptions()
			queued := mode == "queue-expired" || mode == "stop-queued"
			options.EnableSessionWorker = !queued
			w := worker.New(env.Client, env.TaskQueue, options)
			jobs.Register(w)
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
			plan := recoveryPlan()
			plan.SessionAttempts = 1
			plan.Stages[0].Steps[0].Name, plan.Stages[0].Steps[1].Name = "hold", "never"
			if mode == "queue-expired" {
				plan.QueueTimeout = time.Second
			} else if !queued {
				plan.ReservationTimeout = 3 * time.Second
			}
			run, err := jobs.Submit(ctx, "deadline-"+rand.Text(), tmprl.JobInput[string]{Job: &plan})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "stop-queued" {
				awaitRunStatus(t, ctx, jobs, run.Execution, tmprl.StatusQueued)
				if err := jobs.Stop(ctx, run.Execution); err != nil {
					t.Fatal(err)
				}
			} else if !queued {
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				if mode == "paused-reservation-expired" {
					if err := jobs.Pause(ctx, run.Execution); err != nil {
						t.Fatal(err)
					}
					awaitRunStatus(t, ctx, jobs, run.Execution, tmprl.StatusPauseRequested)
					close(release)
					awaitRunStatus(t, ctx, jobs, run.Execution, tmprl.StatusPaused)
				}
			}
			_, waitErr := jobs.Wait(ctx, run.Execution)
			if waitErr == nil || errors.Is(waitErr, context.DeadlineExceeded) {
				t.Fatalf("Wait: got %v, want terminal Temporal failure", waitErr)
			}
			want := tmprl.StatusFailed
			if mode == "stop-queued" {
				want = tmprl.StatusStopped
				if !temporal.IsCanceledError(waitErr) {
					t.Fatalf("Wait: got %v, want canceled workflow", waitErr)
				}
			} else if !queued {
				failure, ok := errors.AsType[*temporal.ApplicationError](waitErr)
				if !ok || failure.Type() != "reservation-expired" {
					t.Fatalf("Wait: got %v, want reservation-expired", waitErr)
				}
			} else if _, ok := errors.AsType[*temporal.TimeoutError](waitErr); !ok {
				t.Fatalf("Wait: got %v, want queue timeout", waitErr)
			}
			closed, err := jobs.Get(ctx, run.Execution)
			if err != nil || closed.Status != want || closed.ClosedAt.IsZero() || closed.Error == "" {
				t.Fatalf("terminal polling: got %+v, %v, want %s with closure diagnostics", closed, err, want)
			}
			if mode == "paused-reservation-expired" && (closed.Artifacts != 17 || closed.Stages[0].Steps[0].Status != tmprl.StatusCompleted) {
				t.Fatalf("paused artifacts: got %+v, want committed value 17", closed)
			}
			if closed.Stages[0].Steps[1].Status != tmprl.StatusQueued {
				t.Fatalf("next step: got %s, want queued", closed.Stages[0].Steps[1].Status)
			}
		})
	}
}

func awaitRunStatus[R, A any](t *testing.T, ctx context.Context, jobs *tmprl.Jobs[R, A], execution tmprl.Execution, want tmprl.Status) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		run, err := jobs.Get(ctx, execution)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("polling: got %s, want %s: %v", run.Status, want, ctx.Err())
		case <-ticker.C:
		}
	}
}

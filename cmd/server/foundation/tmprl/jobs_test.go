package tmprl_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

func TestJobsInPlacePauseResumeAndStop(t *testing.T) {
	for _, action := range []string{"resume", "stop"} {
		t.Run(action, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			entered, permit := make(chan int, 3), make(chan struct{})
			var invocations atomic.Int32
			handler := tmprl.StepFunc[string, []string](func(ctx context.Context, in tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
				if in.Request.Step == "finish" {
					return tmprl.StepResult[[]string]{Artifacts: append(slices.Clone(in.Request.Artifacts), "finish")}, nil
				}
				invocations.Add(1)
				var saved []string
				for unit := 1; unit <= 3; unit++ {
					entered <- unit
					select {
					case <-ctx.Done():
						return tmprl.StepResult[[]string]{}, ctx.Err()
					case <-permit:
					}
					saved = append(saved, fmt.Sprint(unit))
					if err := in.PausePoint(ctx); err != nil {
						return tmprl.StepResult[[]string]{}, err
					}
				}
				return tmprl.StepResult[[]string]{Artifacts: saved}, nil
			})
			j, err := tmprl.New(env.Client, tmprl.Config[string, []string]{
				Names:     tmprl.Names{Workflow: "example.InPlace", Activity: "example.InPlaceStep", Memo: "inplace.state", Query: "inplace-query", Control: "inplace-control"},
				TaskQueue: env.TaskQueue, StorageRoot: t.TempDir(), StorageDirectory: "inplace", Log: env.Log,
				Steps: map[string]tmprl.StepHandler[string, []string]{"work/commit": handler, "work/finish": handler},
			})
			if err != nil {
				t.Fatal(err)
			}
			w := j.NewWorker()
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
			plan := recoveryPlan()
			run, err := j.Submit(ctx, "inplace-"+rand.Text(), tmprl.JobInput[string]{Job: &plan, ControlStage: "work", PauseInPlace: true})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			pause := tmprl.ControlRequest{RequestID: "pause", Action: "pause"}
			if _, err := j.Control(ctx, run.Execution, pause); err != nil {
				t.Fatal(err)
			}
			requested := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status == tmprl.StatusPauseRequested })
			if requested.ControlAcknowledgedRevision != 0 {
				t.Fatal("acknowledged an in-flight unit as paused")
			}
			permit <- struct{}{}
			paused := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status == tmprl.StatusPaused })
			if paused.Step != "commit" || paused.ControlAcknowledgedRevision != 1 || paused.ControlCheckpoint != "" || paused.WorkerHost == "" {
				t.Fatalf("pause did not retain the active step and worker: %+v", paused)
			}
			select {
			case <-entered:
				t.Fatal("started another unit while paused")
			case <-time.After(150 * time.Millisecond):
			}
			if action == "stop" {
				if err := j.Stop(ctx, run.Execution); err != nil {
					t.Fatal(err)
				}
				stopped := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status.Terminal() })
				if stopped.Status != tmprl.StatusStopped || len(stopped.Artifacts) != 0 {
					t.Fatalf("Stop continued or published paused work: %+v", stopped)
				}
				return
			}
			if _, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: "resume", Action: "resume", ExpectedRevision: 1}); err != nil {
				t.Fatal(err)
			}
			handle, err := env.Client.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{WorkflowID: run.WorkflowID, RunID: run.RunID,
				UpdateName: "inplace-control.paused", Args: []any{"work", "commit", int64(1)}, WaitForStage: client.WorkflowUpdateStageCompleted})
			if err != nil {
				t.Fatal(err)
			}
			var acknowledged bool
			if err := handle.Get(ctx, &acknowledged); err != nil || acknowledged {
				t.Fatalf("late acknowledgement reinstated an old pause: %t %v", acknowledged, err)
			}
			close(permit)
			completed, err := j.Wait(ctx, run.Execution)
			if err != nil || !slices.Equal(completed.Artifacts, []string{"1", "2", "3", "finish"}) || invocations.Load() != 1 {
				t.Fatalf("Resume restarted or lost work: %+v, invocations=%d, err=%v", completed, invocations.Load(), err)
			}
		})
	}
}

func TestJobsOrderedControls(t *testing.T) {
	env := newTestEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	entered, release := make(chan struct{}, 1), make(chan struct{})
	handler := tmprl.StepFunc[string, []string](func(ctx context.Context, in tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
		if in.Request.Step == "commit" {
			select {
			case entered <- struct{}{}:
			default:
			}
			select {
			case <-release:
			case <-ctx.Done():
				return tmprl.StepResult[[]string]{}, ctx.Err()
			}
		}
		return tmprl.StepResult[[]string]{Artifacts: append(slices.Clone(in.Request.Artifacts), in.Request.Step)}, nil
	})
	j, err := tmprl.New(env.Client, tmprl.Config[string, []string]{
		Names:     tmprl.Names{Workflow: "example.Ordered", Activity: "example.OrderedStep", Memo: "ordered.state", Query: "ordered-query", Control: "ordered-control"},
		TaskQueue: env.TaskQueue, StorageRoot: t.TempDir(), StorageDirectory: "ordered", Log: env.Log,
		Steps: map[string]tmprl.StepHandler[string, []string]{"work/commit": handler, "work/finish": handler},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := j.NewWorker()
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	plan := recoveryPlan()
	plan.Stages[0].Steps[0].WaitAfter = "review"
	run, err := j.Submit(ctx, "ordered-"+rand.Text(), tmprl.JobInput[string]{Job: &plan, ControlStage: "work"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	pause := tmprl.ControlRequest{RequestID: "pause-1", Action: "pause"}
	receipt, err := j.Control(ctx, run.Execution, pause)
	if err != nil || receipt.Revision != 1 || receipt.ControlRequest != pause {
		t.Fatalf("pause receipt: %+v %v", receipt, err)
	}
	awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool {
		return s.Status == tmprl.StatusPauseRequested && s.ControlRevision == 1
	}) // An executing activity has not acknowledged a safe point.
	resume := tmprl.ControlRequest{RequestID: "resume-1", Action: "resume", ExpectedRevision: 1}
	if r, err := j.Control(ctx, run.Execution, resume); err != nil || r.Revision != 2 {
		t.Fatalf("resume: %+v %v", r, err)
	}
	if r, err := j.Control(ctx, run.Execution, pause); err != nil || r != receipt {
		t.Fatalf("old receipt replay: %+v %v", r, err)
	}
	for _, request := range []tmprl.ControlRequest{
		{RequestID: "pause-1", Action: "resume"},
		{RequestID: "stale-pause", Action: "pause"},
	} {
		if _, err := j.Control(ctx, run.Execution, request); !errors.Is(err, tmprl.ErrControlConflict) {
			t.Fatalf("conflicting request accepted: %+v %v", request, err)
		}
	}
	state := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool {
		return s.ControlRevision == 2 && s.ControlAction == "resume"
	})
	if state.Status != tmprl.StatusRunning {
		t.Fatalf("receipt replay reapplied an old pause: %+v", state)
	}
	results := make(chan error, 2)
	for _, id := range []string{"tab-a", "tab-b"} {
		go func() {
			r, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: id, Action: "pause", ExpectedRevision: 2})
			if err == nil && r.Revision != 3 {
				err = fmt.Errorf("revision = %d, want 3", r.Revision)
			}
			results <- err
		}()
	}
	accepted, conflicts := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			accepted++
		case errors.Is(err, tmprl.ErrControlConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatalf("competing commands: %d accepted, %d conflicts", accepted, conflicts)
	}
	close(release)
	state = awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status == tmprl.StatusPaused })
	w.Stop()
	replacement := j.NewWorker()
	if err := replacement.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(replacement.Stop)
	if r, err := j.Control(ctx, run.Execution, pause); err != nil || r != receipt {
		t.Fatalf("receipt after worker replacement: %+v %v", r, err)
	}
	if r, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: "resume-2", Action: "resume", ExpectedRevision: 3}); err != nil || r.Revision != 4 {
		t.Fatalf("second cycle: %+v %v", r, err)
	}
	state = awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.ControlRevision == 4 && s.Checkpoint != nil })
	checkpoint := *state.Checkpoint
	if state.Status != tmprl.StatusWaiting {
		t.Fatalf("resume bypassed review: %+v", state)
	}
	if r, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: "pause-3", Action: "pause", ExpectedRevision: 4}); err != nil || r.Revision != 5 {
		t.Fatalf("pause before review acceptance: %+v %v", r, err)
	}
	if _, err := j.Decide(ctx, run.Execution, tmprl.Decision{CheckpointID: checkpoint.ID, SubmissionID: "review-1"}); err != nil {
		t.Fatal(err)
	}
	state = awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Checkpoint == nil && s.Status == tmprl.StatusPaused })
	if len(state.Decisions) != 1 || len(state.Artifacts) != 1 {
		t.Fatalf("review acceptance bypassed pause: %+v", state)
	}
	if r, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: "resume-3", Action: "resume", ExpectedRevision: 5}); err != nil || r.Revision != 6 {
		t.Fatalf("continue after accepted review: %+v %v", r, err)
	}
	if completed, err := j.Wait(ctx, run.Execution); err != nil || !slices.Equal(completed.Artifacts, []string{"commit", "finish"}) {
		t.Fatalf("completion: %+v %v", completed, err)
	}
	if r, err := j.Control(ctx, run.Execution, pause); err != nil || r != receipt {
		t.Fatalf("closed receipt: %+v %v", r, err)
	}
	if _, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: "after-close", Action: "pause", ExpectedRevision: 6}); !errors.Is(err, tmprl.ErrControlConflict) {
		t.Fatalf("closed run accepted a new control: %v", err)
	}
}

func awaitCheckpointState(t *testing.T, ctx context.Context, j *tmprl.Jobs[string, []string], e tmprl.Execution, match func(tmprl.Run[string, []string]) bool) tmprl.Run[string, []string] {
	t.Helper()
	for {
		run, err := j.Get(ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		if match(run) {
			return run
		}
		select {
		case <-ctx.Done():
			t.Fatalf("state never matched: %+v: %v", run, ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestJobsSafePointControls(t *testing.T) {
	for _, outcome := range []string{"continue", "stop", "missing checkpoint", "changed checkpoint", "retry budget"} {
		t.Run(outcome, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			entered, permit := make(chan int, 4), make(chan struct{})
			root := t.TempDir()
			var invocations atomic.Int32
			handler := tmprl.StepFunc[string, []string](func(ctx context.Context, in tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
				if invocation := invocations.Add(1); outcome == "retry budget" && invocation != 2 {
					return tmprl.StepResult[[]string]{}, errors.New("controlled retry-budget failure")
				}
				var saved []string
				data, err := in.Workspace.ReadFile("cursor.json")
				switch {
				case err == nil:
					if err := json.Unmarshal(data, &saved); err != nil {
						return tmprl.StepResult[[]string]{}, err
					}
				case !errors.Is(err, os.ErrNotExist):
					return tmprl.StepResult[[]string]{}, err
				}
				for len(saved) < 3 {
					entered <- len(saved)
					select {
					case <-permit:
					case <-ctx.Done():
						return tmprl.StepResult[[]string]{}, ctx.Err()
					}
					saved = append(saved, fmt.Sprintf("unit-%d", len(saved)))
					if err := tmprl.WriteJSON(in.Workspace, "cursor.json", saved); err != nil {
						return tmprl.StepResult[[]string]{}, err
					}
					if err := in.SafePoint(ctx, "cursor.json"); err != nil {
						return tmprl.StepResult[[]string]{}, err
					}
				}
				return tmprl.StepResult[[]string]{Artifacts: saved}, nil
			})
			j, err := tmprl.New(env.Client, tmprl.Config[string, []string]{
				Names:     tmprl.Names{Workflow: "example.SafePoint", Activity: "example.SafePointStep", Memo: "safe.state", Query: "safe-query", Control: "safe-control"},
				TaskQueue: env.TaskQueue, StorageRoot: root, StorageDirectory: "safe", Log: env.Log,
				Steps: map[string]tmprl.StepHandler[string, []string]{"work/commit": handler},
			})
			if err != nil {
				t.Fatal(err)
			}
			w := j.NewWorker()
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
			plan := recoveryPlan()
			plan.Stages[0].Steps = plan.Stages[0].Steps[:1]
			if outcome == "retry budget" {
				plan.Stages[0].Steps[0].Retry.MaximumAttempts = 2
			}
			run, err := j.Submit(ctx, "safe-"+rand.Text(), tmprl.JobInput[string]{Job: &plan, ControlStage: "work"})
			if err != nil {
				t.Fatal(err)
			}
			for cycle := range 2 {
				select {
				case index := <-entered:
					if index != cycle {
						t.Fatalf("saved unit repeated: got %d, want %d", index, cycle)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				if _, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: fmt.Sprintf("pause-%d", cycle), Action: "pause", ExpectedRevision: int64(cycle * 2)}); err != nil {
					t.Fatal(err)
				}
				requested := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status == tmprl.StatusPauseRequested })
				if requested.ControlAcknowledgedRevision == requested.ControlRevision {
					t.Fatal("in-flight work was incorrectly acknowledged as paused")
				}
				permit <- struct{}{}
				paused := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status == tmprl.StatusPaused })
				path := filepath.Join(root, "safe", run.RunID, "steps", "work")
				data, err := os.ReadFile(filepath.Join(path, "cursor.json"))
				if err != nil {
					t.Fatal(err)
				}
				var saved []string
				if err := json.Unmarshal(data, &saved); err != nil || len(saved) != cycle+1 || paused.ControlAcknowledgedRevision != int64(cycle*2+1) || paused.Step != "commit" || paused.ControlCheckpoint != fmt.Sprintf("work/commit/cursor.json:%x", sha256.Sum256(data)) {
					t.Fatalf("pause did not identify committed sub-step: %+v, %q, %v", paused, data, err)
				}
				if _, err := os.Stat(filepath.Join(path, "commit.json")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("suspension incorrectly completed the step: %v", err)
				}
				if cycle == 0 {
					w.Stop()
					replacement := j.NewWorker()
					if err := replacement.Start(); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(replacement.Stop)
					state, err := j.Get(ctx, run.Execution)
					if err != nil || state.Status != tmprl.StatusPaused || state.ControlAcknowledgedRevision != 1 {
						t.Fatalf("worker replacement lost pause: %+v, %v", state, err)
					}
				}
				if outcome == "stop" && cycle == 1 {
					if err := j.Stop(ctx, run.Execution); err != nil {
						t.Fatal(err)
					}
					stopped := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status.Terminal() })
					if stopped.Status != tmprl.StatusStopped {
						t.Fatalf("Stop resumed or failed preparation: %+v", stopped)
					}
					select {
					case <-entered:
						t.Fatal("Stop launched another unit")
					default:
					}
					return
				}
				switch outcome {
				case "missing checkpoint":
					if err := os.Remove(filepath.Join(path, "cursor.json")); err != nil {
						t.Fatal(err)
					}
				case "changed checkpoint":
					if err := os.WriteFile(filepath.Join(path, "cursor.json"), []byte(`[]`), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: fmt.Sprintf("resume-%d", cycle), Action: "resume", ExpectedRevision: int64(cycle*2 + 1)}); err != nil {
					t.Fatal(err)
				}
				if outcome != "continue" && outcome != "stop" {
					failed, err := j.Wait(ctx, run.Execution)
					if err == nil {
						t.Fatalf("Continue hid %s: %+v", outcome, failed)
					}
					if outcome == "retry budget" {
						if invocations.Load() != 3 {
							t.Fatalf("Continue refreshed failed-attempt budget: %d", invocations.Load())
						}
					} else if failure, ok := errors.AsType[*temporal.ApplicationError](err); !ok || failure.Type() != "invalid-step-checkpoint" {
						t.Fatalf("did not reject damaged resume checkpoint: %v", err)
					}
					select {
					case <-entered:
						t.Fatal("failed continuation launched another unit")
					default:
					}
					return
				}
			}
			select {
			case index := <-entered:
				if index != 2 {
					t.Fatalf("Continue repeated committed work: %d", index)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			permit <- struct{}{}
			completed, err := j.Wait(ctx, run.Execution)
			if err != nil || completed.Execution != run.Execution || !slices.Equal(completed.Artifacts, []string{"unit-0", "unit-1", "unit-2"}) || completed.Stages[0].Steps[0].Attempt != 1 {
				t.Fatalf("Continue lost/duplicated work or charged voluntary pauses as failures: %+v, %v", completed, err)
			}
		})
	}
}

// TestJobsDrain models host shutdowns: each process drains before stopping its
// worker. Draining finishes the running unit, saves it, and suspends without a
// recovery attempt; the next process continues the saved checkpoint. Paused
// and active-only budgets must not count idle time between processes.
func TestJobsDrain(t *testing.T) {
	env := newTestEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	entered, permit := make(chan int, 8), make(chan struct{})
	root := t.TempDir()
	handler := tmprl.StepFunc[string, []string](func(ctx context.Context, in tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
		var saved []string
		if data, err := in.Workspace.ReadFile("cursor.json"); err == nil {
			if err := json.Unmarshal(data, &saved); err != nil {
				return tmprl.StepResult[[]string]{}, err
			}
		}
		for len(saved) < 4 {
			entered <- len(saved)
			select {
			case <-permit:
			case <-ctx.Done():
				return tmprl.StepResult[[]string]{}, ctx.Err()
			}
			saved = append(saved, fmt.Sprintf("unit-%d", len(saved)))
			if err := tmprl.WriteJSON(in.Workspace, "cursor.json", saved); err != nil {
				return tmprl.StepResult[[]string]{}, err
			}
			if err := in.SafePoint(ctx, "cursor.json"); err != nil {
				return tmprl.StepResult[[]string]{}, err
			}
		}
		return tmprl.StepResult[[]string]{Artifacts: saved}, nil
	})
	config := tmprl.Config[string, []string]{
		Names:     tmprl.Names{Workflow: "example.Drain", Activity: "example.DrainStep", Memo: "drain.state", Query: "drain-query", Control: "drain-control"},
		TaskQueue: env.TaskQueue, StorageRoot: root, StorageDirectory: "drain", Log: env.Log,
		Steps: map[string]tmprl.StepHandler[string, []string]{"work/commit": handler},
	}
	// Each process has its own Jobs, as after a restart.
	start := func() (*tmprl.Jobs[string, []string], func()) {
		j, err := tmprl.New(env.Client, config)
		if err != nil {
			t.Fatal(err)
		}
		w := j.NewWorker()
		if err := w.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(w.Stop)
		return j, w.Stop
	}
	unit := func(want int) {
		t.Helper()
		select {
		case got := <-entered:
			if got != want {
				t.Fatalf("unit %d started, want %d: saved work repeated or skipped", got, want)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	j, stop := start()
	plan := recoveryPlan()
	plan.Stages[0].Steps = plan.Stages[0].Steps[:1]
	plan.SessionAttempts = 1 // A drain must not consume the only recovery attempt.
	plan.Stages[0].Timeout = 30 * time.Second
	run, err := j.Submit(ctx, "drain-"+rand.Text(), tmprl.JobInput[string]{Job: &plan, ControlStage: "work"})
	if err != nil {
		t.Fatal(err)
	}

	// First shutdown: Drain waits for the in-flight unit to save.
	unit(0)
	drained := make(chan error, 1)
	go func() { drained <- j.Drain(ctx) }()
	select {
	case err := <-drained:
		t.Fatalf("Drain returned while a unit was in flight: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	permit <- struct{}{}
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	stop()
	time.Sleep(time.Second) // Offline time must not count toward active budgets.

	// Second process: records the drained suspension (the stopped worker could
	// not), then continues the saved unit without a user Continue.
	j, stop = start()
	unit(1)
	resumed, err := j.Get(ctx, run.Execution)
	if err != nil || resumed.SessionAttempt != 1 || resumed.Stages[0].Steps[0].Attempt != 1 {
		t.Fatalf("drain consumed a recovery or step attempt: %+v, %v", resumed, err)
	}
	// Pause, then shut down while paused: the run must stay paused.
	if _, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: "pause", Action: "pause"}); err != nil {
		t.Fatal(err)
	}
	permit <- struct{}{}
	paused := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool {
		return s.Status == tmprl.StatusPaused && s.WorkerHost == ""
	})
	if paused.SessionAttempt != 1 || paused.Stages[0].Deadline != (time.Time{}) {
		t.Fatalf("paused run held its worker, consumed an attempt, or kept a deadline: %+v", paused)
	}
	if err := j.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	stop()

	// Third process: paused work stays paused until Continue.
	j, _ = start()
	select {
	case got := <-entered:
		t.Fatalf("restart ignored Pause and started unit %d", got)
	case <-time.After(500 * time.Millisecond):
	}
	if _, err := j.Control(ctx, run.Execution, tmprl.ControlRequest{RequestID: "continue", Action: "resume", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	unit(2)
	permit <- struct{}{}
	unit(3)
	permit <- struct{}{}
	completed, err := j.Wait(ctx, run.Execution)
	if err != nil || !slices.Equal(completed.Artifacts, []string{"unit-0", "unit-1", "unit-2", "unit-3"}) || completed.SessionAttempt != 1 || completed.Stages[0].Steps[0].Attempt != 1 {
		t.Fatalf("restart lost, duplicated, or charged work: %+v, %v", completed, err)
	}
	stage := completed.Stages[0]
	if active := stage.ActiveElapsed; active <= 0 || active >= stage.ClosedAt.Sub(stage.StartedAt)-time.Second {
		t.Fatalf("stage budget counted idle time: active %v of %v", active, stage.ClosedAt.Sub(stage.StartedAt))
	}
}

func TestWriteJSONReplacement(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, value := range []string{"long original checkpoint", "new"} {
		if err := tmprl.WriteJSON(root, "state.json", value); err != nil {
			t.Fatal(err)
		}
		data, err := root.ReadFile("state.json")
		var got string
		if err != nil || json.Unmarshal(data, &got) != nil || got != value {
			t.Fatalf("incomplete replacement: %s, %v", data, err)
		}
	}
	if err := tmprl.WriteJSON(root, "state.json", make(chan int)); err == nil {
		t.Fatal("accepted unencodable checkpoint")
	}
	data, err := root.ReadFile("state.json")
	if err != nil || string(data) != `"new"` {
		t.Fatalf("failed write damaged committed state: %s, %v", data, err)
	}
}

func TestJobsProgressTiming(t *testing.T) {
	env := newTestEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var broker tmprl.ProgressBroker[tmprl.Execution, tmprl.Progress]
	observations := make(chan []tmprl.Progress, 1)
	release := make(chan struct{})
	handler := tmprl.StepFunc[string, []string](func(ctx context.Context, in tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
		updates, unsubscribe := broker.Subscribe(in.Request.Execution)
		defer unsubscribe()
		var captured []tmprl.Progress
		for _, p := range []tmprl.Progress{{Completed: 12, Total: 20}, {Completed: 16, Total: 20}, {Completed: 18, Total: 30}, {Completed: 5, Total: 30}, {Completed: 7, Total: 30}} {
			in.Publish(p)
			captured = append(captured, <-updates)
		}
		observations <- captured
		select {
		case <-release:
			return tmprl.StepResult[[]string]{}, nil
		case <-ctx.Done():
			return tmprl.StepResult[[]string]{}, ctx.Err()
		}
	})
	j, err := tmprl.New(env.Client, tmprl.Config[string, []string]{
		Names:     tmprl.Names{Workflow: "example.Timing", Activity: "example.TimingStep", Memo: "timing.snapshot", Query: "timing-query", Control: "timing-control"},
		TaskQueue: env.TaskQueue, StorageRoot: t.TempDir(), StorageDirectory: "timing", Log: env.Log, Progress: &broker,
		Steps: map[string]tmprl.StepHandler[string, []string]{"work/commit": handler},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := j.NewWorker()
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Stop)
	plan := recoveryPlan()
	plan.Stages[0].Steps = plan.Stages[0].Steps[:1]
	run, err := j.Submit(ctx, "timing-"+rand.Text(), tmprl.JobInput[string]{Job: &plan})
	if err != nil {
		t.Fatal(err)
	}
	var samples []tmprl.Progress
	select {
	case samples = <-observations:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if samples[0].InitialCompleted != 12 || samples[0].Remaining() != 0 || samples[1].InitialCompleted != 12 || samples[1].Remaining() <= 0 || !samples[1].StartedAt.Equal(samples[0].StartedAt) {
		t.Fatalf("resumed work timing: %+v", samples)
	}
	if samples[2].InitialCompleted != 18 || samples[2].Remaining() != 0 || samples[3].InitialCompleted != 5 || samples[3].Remaining() != 0 || samples[4].InitialCompleted != 5 || samples[4].Remaining() <= 0 {
		t.Fatalf("phase reset timing: %+v", samples)
	}
	snapshot := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool {
		return s.Progress != nil && s.Progress.Completed == 7 && s.Progress.InitialCompleted == 5
	})
	if !snapshot.Progress.StartedAt.Equal(samples[4].StartedAt) || !snapshot.Progress.UpdatedAt.Equal(samples[4].UpdatedAt) || snapshot.Progress.Remaining() <= 0 {
		t.Fatalf("Temporal heartbeat lost timing: %+v", snapshot.Progress)
	}
	close(release)
	if _, err := j.Wait(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
}

func TestDecisionValidate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		decision tmprl.Decision
		valid    bool
	}{
		{"empty payload", tmprl.Decision{CheckpointID: "checkpoint", SubmissionID: "submission"}, true},
		{"limits", tmprl.Decision{CheckpointID: strings.Repeat("c", 512), SubmissionID: strings.Repeat("s", 200), Payload: json.RawMessage(`"` + strings.Repeat("p", (64<<10)-2) + `"`)}, true},
		{"missing checkpoint", tmprl.Decision{SubmissionID: "submission"}, false},
		{"missing submission", tmprl.Decision{CheckpointID: "checkpoint"}, false},
		{"blank submission", tmprl.Decision{CheckpointID: "checkpoint", SubmissionID: " "}, false},
		{"checkpoint too long", tmprl.Decision{CheckpointID: strings.Repeat("c", 513), SubmissionID: "s"}, false},
		{"submission too long", tmprl.Decision{CheckpointID: "c", SubmissionID: strings.Repeat("s", 201)}, false},
		{"payload too long", tmprl.Decision{CheckpointID: "c", SubmissionID: "s", Payload: json.RawMessage(`"` + strings.Repeat("p", (64<<10)-1) + `"`)}, false},
		{"malformed payload", tmprl.Decision{CheckpointID: "c", SubmissionID: "s", Payload: json.RawMessage(`{`)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.decision.Validate()
			if tc.valid && err != nil || !tc.valid && !errors.Is(err, tmprl.ErrInvalidArgument) {
				t.Fatalf("Validate = %v; valid %t", err, tc.valid)
			}
		})
	}
}

func TestJobsStageCheckpoint(t *testing.T) {
	for _, action := range []string{"accept", "stop", "reservation-expired"} {
		t.Run(action, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			handler := tmprl.StepFunc[string, []string](func(_ context.Context, in tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
				return tmprl.StepResult[[]string]{Artifacts: append(slices.Clone(in.Request.Artifacts), in.Request.Stage)}, nil
			})
			j, err := tmprl.New(env.Client, tmprl.Config[string, []string]{
				Names:     tmprl.Names{Workflow: "example.StageGate", Activity: "example.StageStep", Memo: "stage.snapshot", Query: "stage-query", Control: "stage-control"},
				TaskQueue: env.TaskQueue, StorageRoot: t.TempDir(), StorageDirectory: "stage-gates", Log: env.Log,
				Steps: map[string]tmprl.StepHandler[string, []string]{"work/commit": handler, "next/finish": handler},
			})
			if err != nil {
				t.Fatal(err)
			}
			w := j.NewWorker()
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
			plan := recoveryPlan()
			first := &plan.Stages[0]
			last := first.Steps[1]
			first.Steps, first.WaitAfter, first.Timeout = first.Steps[:1], "next-stage", time.Second
			plan.Stages = append(plan.Stages, tmprl.Stage{Name: "next", Timeout: time.Minute, Steps: []tmprl.Step{last}})
			if action == "reservation-expired" {
				plan.ReservationTimeout = 3 * time.Second
			}
			run, err := j.Submit(ctx, "stage-"+rand.Text(), tmprl.JobInput[string]{Job: &plan})
			if err != nil {
				t.Fatal(err)
			}
			waiting := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status == tmprl.StatusWaiting })
			if waiting.Checkpoint.Step != "" || waiting.Stages[0].Status != tmprl.StatusCompleted || waiting.Stages[0].ClosedAt.IsZero() || !waiting.Stages[1].StartedAt.IsZero() {
				t.Fatalf("stage boundary: %+v", waiting)
			}
			if err := j.Resume(ctx, run.Execution); err != nil {
				t.Fatal(err)
			}
			// Advance past the completed stage's budget; waiting belongs to the
			// reservation, not to the already-finished stage.
			wait, stop := context.WithDeadline(ctx, waiting.Stages[0].Deadline.Add(100*time.Millisecond))
			<-wait.Done()
			stop()
			still, err := j.Get(ctx, run.Execution)
			if err != nil || still.Status != tmprl.StatusWaiting || !slices.Equal(still.Artifacts, []string{"work"}) {
				t.Fatalf("stage wait: %+v, %v", still, err)
			}
			switch action {
			case "accept":
				_, err = j.Decide(ctx, run.Execution, tmprl.Decision{CheckpointID: waiting.Checkpoint.ID, SubmissionID: "next"})
			case "stop":
				err = j.Stop(ctx, run.Execution)
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := j.Wait(ctx, run.Execution)
			if action == "accept" {
				if err != nil || !slices.Equal(result.Artifacts, []string{"work", "next"}) {
					t.Fatalf("continued: %+v, %v", result, err)
				}
			} else if err == nil || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected terminal cancellation/expiry: %v", err)
			}
		})
	}
}

func TestJobsCheckpoints(t *testing.T) {
	for _, mode := range []string{"accept", "stop", "stage-expired", "reservation-expired"} {
		t.Run(mode, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			handler := tmprl.StepFunc[string, []string](func(ctx context.Context, in tmprl.StepContext[string, []string]) (tmprl.StepResult[[]string], error) {
				if in.Request.Step == "commit" {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
						return tmprl.StepResult[[]string]{}, ctx.Err()
					}
				}
				if in.Request.Request != "immutable" {
					return tmprl.StepResult[[]string]{}, errors.New("request changed")
				}
				for _, decision := range in.Request.Decisions {
					if string(decision.Payload) != `{"accepted":"sample-a"}` {
						return tmprl.StepResult[[]string]{}, errors.New("accepted input was lost")
					}
				}
				return tmprl.StepResult[[]string]{Artifacts: append(slices.Clone(in.Request.Artifacts), fmt.Sprintf("%s:%d", in.Request.Step, len(in.Request.Decisions)))}, nil
			})
			j, err := tmprl.New(env.Client, tmprl.Config[string, []string]{
				Names:     tmprl.Names{Workflow: "example.Checkpoints", Activity: "example.CheckpointStep", Memo: "example.checkpoints", Query: "checkpoint-state", Control: "checkpoint-control"},
				TaskQueue: env.TaskQueue, StorageRoot: t.TempDir(), StorageDirectory: "checkpoints", Log: env.Log,
				Steps: map[string]tmprl.StepHandler[string, []string]{"work/commit": handler, "work/review": handler, "work/finish": handler},
			})
			if err != nil {
				t.Fatal(err)
			}
			w := j.NewWorker()
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
			plan := recoveryPlan()
			plan.Stages[0].Steps[0].WaitAfter = "first-review"
			review := plan.Stages[0].Steps[0]
			review.Name, review.WaitAfter = "review", "second-review"
			plan.Stages[0].Steps = []tmprl.Step{plan.Stages[0].Steps[0], review, plan.Stages[0].Steps[1]}
			if mode == "stage-expired" {
				plan.Stages[0].Timeout = 2 * time.Second
			}
			if mode == "reservation-expired" {
				plan.ReservationTimeout = 2 * time.Second
			}
			run, err := j.Submit(ctx, "checkpoint-"+rand.Text(), tmprl.JobInput[string]{Job: &plan, Request: "immutable"})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if mode == "accept" {
				if _, err := j.Decide(ctx, run.Execution, tmprl.Decision{CheckpointID: "not-pending", SubmissionID: "early"}); err == nil {
					t.Fatal("accepted a premature decision")
				}
				wrong, err := tmprl.New(env.Client, tmprl.Config[string, []string]{
					Names:     tmprl.Names{Workflow: "example.Other", Activity: "other.Step", Memo: "other.memo", Query: "other-query", Control: "other-control"},
					TaskQueue: env.TaskQueue, StorageRoot: t.TempDir(), StorageDirectory: "other", Log: env.Log,
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := wrong.Decide(ctx, run.Execution, tmprl.Decision{CheckpointID: "c", SubmissionID: "s"}); !errors.Is(err, tmprl.ErrUnsupportedWorkflow) {
					t.Fatalf("wrong workflow type: %v", err)
				}
			}
			close(release)
			waiting := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status == tmprl.StatusWaiting })
			if waiting.Checkpoint == nil || waiting.Checkpoint.Name != "first-review" || waiting.Status.Terminal() || !slices.Equal(waiting.Artifacts, []string{"commit:0"}) || waiting.Stages[0].Steps[0].Status != tmprl.StatusCompleted || waiting.Stages[0].Steps[1].Status != tmprl.StatusQueued {
				t.Fatalf("invalid boundary: %+v", waiting)
			}
			if mode != "accept" {
				if mode == "stop" {
					if err := j.Stop(ctx, run.Execution); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := j.Wait(ctx, run.Execution); err == nil || errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("expected terminal stop/deadline: %v", err)
				}
				closed, err := j.Get(ctx, run.Execution)
				if err != nil || closed.Checkpoint != nil || !slices.Equal(closed.Artifacts, []string{"commit:0"}) || closed.Stages[0].Steps[1].Status != tmprl.StatusQueued {
					t.Fatalf("closed boundary: %+v, %v", closed, err)
				}
				return
			}
			// A plain Resume, including a pause/resume pair, cannot release review.
			if err := j.Pause(ctx, run.Execution); err != nil {
				t.Fatal(err)
			}
			if err := j.Resume(ctx, run.Execution); err != nil {
				t.Fatal(err)
			}
			// Query observes both preceding signals on this execution before assertions.
			var live tmprl.Run[string, []string]
			if err := tmprl.Query(ctx, env.Client, run.Execution, "checkpoint-state", &live); err != nil {
				t.Fatal(err)
			}
			if live.Checkpoint == nil || live.Status != tmprl.StatusWaiting || !slices.Equal(live.Artifacts, waiting.Artifacts) {
				t.Fatalf("resume bypassed review: %+v", live)
			}
			decision := tmprl.Decision{CheckpointID: waiting.Checkpoint.ID, SubmissionID: "accept-first", Payload: json.RawMessage(`{"accepted":"sample-a"}`)}
			bad := decision
			bad.CheckpointID += "-stale"
			if _, err := j.Decide(ctx, run.Execution, bad); err == nil {
				t.Fatal("accepted stale checkpoint")
			}
			if err := j.Pause(ctx, run.Execution); err != nil {
				t.Fatal(err)
			}
			if err := tmprl.Query(ctx, env.Client, run.Execution, "checkpoint-state", &live); err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			for range 2 {
				wg.Go(func() {
					receipt, err := j.Decide(ctx, run.Execution, decision)
					if err != nil || !reflect.DeepEqual(receipt, decision) {
						t.Errorf("receipt: %+v, %v", receipt, err)
					}
				})
			}
			wg.Wait()
			paused := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool { return s.Status == tmprl.StatusPaused })
			if paused.Checkpoint != nil || len(paused.Decisions) != 1 || !slices.Equal(paused.Artifacts, waiting.Artifacts) {
				t.Fatalf("acceptance bypassed pause: %+v", paused)
			}
			bad = decision
			bad.Payload = json.RawMessage(`{"accepted":"changed"}`)
			if _, err := j.Decide(ctx, run.Execution, bad); err == nil {
				t.Fatal("accepted conflicting payload")
			}
			bad = decision
			bad.SubmissionID = "different"
			if _, err := j.Decide(ctx, run.Execution, bad); err == nil {
				t.Fatal("accepted conflicting submission")
			}
			if err := j.Resume(ctx, run.Execution); err != nil {
				t.Fatal(err)
			}
			second := awaitCheckpointState(t, ctx, j, run.Execution, func(s tmprl.Run[string, []string]) bool {
				return s.Checkpoint != nil && s.Checkpoint.Name == "second-review"
			})
			if !slices.Equal(second.Artifacts, []string{"commit:0", "review:1"}) {
				t.Fatalf("lost input or duplicated work: %+v", second)
			}
			if _, err := j.Decide(ctx, run.Execution, decision); err != nil {
				t.Fatal(err)
			}
			if err := tmprl.Query(ctx, env.Client, run.Execution, "checkpoint-state", &live); err != nil {
				t.Fatal(err)
			}
			if live.Checkpoint == nil || live.Checkpoint.ID != second.Checkpoint.ID {
				t.Fatal("old receipt released the next checkpoint")
			}
			canceled, stop := context.WithCancel(ctx)
			stop()
			decision.CheckpointID, decision.SubmissionID = second.Checkpoint.ID, "accept-second"
			if _, err := j.Decide(canceled, run.Execution, decision); err == nil {
				t.Fatal("canceled caller unexpectedly succeeded")
			}
			if _, err := j.Decide(ctx, run.Execution, decision); err != nil {
				t.Fatal(err)
			}
			result, err := j.Wait(ctx, run.Execution)
			if err != nil || result.Request != "immutable" || result.Status != tmprl.StatusCompleted || len(result.Decisions) != 2 || result.Checkpoint != nil || !slices.Equal(result.Artifacts, []string{"commit:0", "review:1", "finish:2"}) {
				t.Fatalf("completed: %+v, %v", result, err)
			}
		})
	}
}

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

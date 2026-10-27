package modeltraining_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

// Exercise queueing without workers, repeat-safe controls, durable stage state,
// visibility snapshots and reservation expiration through the production API.
func TestTemporalManagementLifecycle(t *testing.T) {
	for _, expire := range []bool{false, true} {
		name := "resume"
		if expire {
			name = "expire-while-paused"
		}
		t.Run(name, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			entered := make(chan struct{})
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			finish := make(chan struct{})
			unblockFinish := sync.OnceFunc(func() { close(finish) })
			defer unblockFinish()
			stages := []TrainingStep{
				legacyStepFunc{name: "capture", run: func(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
						return tmprl.StepResult[Artifacts]{}, ctx.Err()
					}
					return tmprl.StepResult[Artifacts]{Artifacts: Artifacts{Checkpoint: "captured"}}, nil
				}},
				legacyStepFunc{name: "finish", run: func(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
					select {
					case <-finish:
					case <-ctx.Done():
						return tmprl.StepResult[Artifacts]{}, ctx.Err()
					}
					return tmprl.StepResult[Artifacts]{Artifacts: input.Request.Artifacts}, nil
				}},
			}
			timeout := time.Minute
			if expire {
				timeout = 5 * time.Second
			}
			b := newServices(t, env.Client, serviceConfig{TaskQueue: env.TaskQueue, LegacySteps: stages, JobTimeout: timeout}).Training
			run, err := b.Start(ctx, request("controlled"))
			if err != nil || run.Status != tmprl.StatusQueued || len(run.Stages) != 2 {
				t.Fatalf("submitted job: got %+v, %v, want queued two-stage job", run, err)
			}
			// No worker has ever polled this queue. Get must not query one.
			pollCtx, stopPoll := context.WithTimeout(ctx, 2*time.Second)
			queued, err := b.Get(pollCtx, run.Execution)
			stopPoll()
			if err != nil || queued.Status != tmprl.StatusQueued || queued.Request.Model != "controlled" || queued.StartedAt.IsZero() {
				t.Fatalf("workerless queued job: got %+v, %v", queued, err)
			}
			w, err := b.NewWorker()
			if err != nil {
				t.Fatal(err)
			}
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(w.Stop)
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
			paused := pollRun(t, b, ctx, run.Execution, tmprl.StatusPaused)
			if paused.ReservationDeadline.IsZero() || paused.UpdatedAt.IsZero() || paused.SessionAttempt != 1 || paused.WorkerHost == "" || paused.Stages[0].Status != tmprl.StatusCompleted || paused.Stages[0].ClosedAt.IsZero() || paused.Stages[1].Status != tmprl.StatusQueued || paused.Artifacts.Checkpoint != "captured" {
				t.Fatalf("paused job diagnostics: %+v", paused)
			}
			// A consistent query after the duplicate signal observes its processing;
			// simply reading an older memo could otherwise hide the regression.
			if err := b.Pause(ctx, run.Execution); err != nil {
				t.Fatal(err)
			}
			value, err := env.Client.QueryWorkflow(ctx, run.WorkflowID, run.RunID, StateQuery)
			if err != nil {
				t.Fatal(err)
			}
			var repeated tmprl.Run[Request, Artifacts]
			if err := value.Get(&repeated); err != nil || repeated.Status != tmprl.StatusPaused {
				t.Fatalf("repeated pause: got %+v, %v, want paused", repeated, err)
			}
			found := false
			for !found {
				var token []byte
				for {
					runs, next, err := b.List(ctx, tmprl.ListOptions{PageSize: 100, NextPageToken: token, Statuses: []tmprl.Status{tmprl.StatusPaused}})
					if err != nil {
						t.Fatal(err)
					}
					for _, listed := range runs {
						if listed.Execution == run.Execution {
							found = listed.Request.Model == "controlled" && listed.Stages[0].Status == tmprl.StatusCompleted && listed.ReservationDeadline.Equal(paused.ReservationDeadline)
						}
					}
					if len(next) == 0 {
						break
					}
					token = next
				}
				if !found {
					select {
					case <-ctx.Done():
						t.Fatal("paused snapshot not visible", ctx.Err())
					case <-time.After(50 * time.Millisecond):
					}
				}
			}
			if expire {
				_, err := b.Wait(ctx, run.Execution)
				var application *temporal.ApplicationError
				if !errors.As(err, &application) || application.Type() != "reservation-expired" {
					t.Fatalf("expiration error: got %v, want reservation-expired", err)
				}
				closed, err := b.Get(ctx, run.Execution)
				if err != nil || closed.Status != tmprl.StatusFailed || closed.ClosedAt.IsZero() || closed.SessionAttempt != 1 || !strings.Contains(closed.Error, "paused time counts") || closed.Stages[1].Status != tmprl.StatusQueued {
					t.Fatalf("expired job: got %+v, %v", closed, err)
				}
				return
			}
			if err := b.Resume(ctx, run.Execution); err != nil {
				t.Fatal(err)
			}
			running := pollRun(t, b, ctx, run.Execution, tmprl.StatusRunning)
			if running.Stage != "finish" || running.Stages[0].Status != tmprl.StatusCompleted {
				t.Fatalf("resumed job: %+v", running)
			}
			if err := b.Resume(ctx, run.Execution); err != nil {
				t.Fatal(err)
			}
			unblockFinish()
			closed, err := b.Wait(ctx, run.Execution)
			if err != nil || closed.Status != tmprl.StatusCompleted || closed.Stages[1].Status != tmprl.StatusCompleted || closed.Stages[1].Attempt != 1 || closed.Stage != "" {
				t.Fatalf("completed job: got %+v, %v", closed, err)
			}
		})
	}
}

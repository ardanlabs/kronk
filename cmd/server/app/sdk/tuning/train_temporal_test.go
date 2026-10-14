package tuning

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

// These integration tests run real workflows/activities against an existing
// server. They use unique queues and IDs; no containers or histories are reset.
func liveBusiness(t *testing.T, backend Backend) *Business {
	t.Helper()
	address := os.Getenv("TEMPORAL_ADDRESS")
	if address == "" {
		t.Skip("set TEMPORAL_ADDRESS to run tests against a real Temporal server")
	}
	c, err := client.Dial(client.Options{HostPort: address, Namespace: os.Getenv("TEMPORAL_NAMESPACE")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	b, err := New(c, Config{TaskQueue: "moxie-tuning-test-" + rand.Text(), Backend: backend, StageTimeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
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

func pollRun(t *testing.T, b *Business, ctx context.Context, e Execution, status Status) Run {
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

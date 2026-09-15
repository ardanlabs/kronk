package finetune

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type testBackend struct {
	preflight func(context.Context, Request) error
	run       func(context.Context, Request, ProgressFunc) (Result, error)
}

func (backend testBackend) Preflight(ctx context.Context, request Request) error {
	if backend.preflight != nil {
		return backend.preflight(ctx, request)
	}
	return nil
}

func (backend testBackend) Run(ctx context.Context, request Request, progress ProgressFunc) (Result, error) {
	return backend.run(ctx, request, progress)
}

func TestManagerRunsOneBackgroundTrainingRunAndPersistsIt(t *testing.T) {
	release := make(chan struct{})
	backend := testBackend{run: func(ctx context.Context, request Request, progress ProgressFunc) (Result, error) {
		progress(Progress{Stage: StageTraining, Completed: 1, Total: 2, Message: "Training"})
		select {
		case <-release:
			adapter := filepath.Join(request.Output, "artifacts", "adapter-f32.gguf")
			if err := os.MkdirAll(filepath.Dir(adapter), 0o700); err != nil {
				return Result{}, err
			}
			if err := os.WriteFile(adapter, []byte("adapter"), 0o600); err != nil {
				return Result{}, err
			}
			checkpoint := filepath.Join(request.Output, "checkpoints", "epoch-01")
			if err := os.MkdirAll(checkpoint, 0o700); err != nil {
				return Result{}, err
			}
			return Result{Adapter: adapter, SelectedEpoch: 1, SelectedCheckpoint: checkpoint, Status: StatusCompleted}, nil
		case <-ctx.Done():
			return Result{Status: StatusStopped}, ctx.Err()
		}
	}}
	manager := newTestManager(t, backend)
	updates := make(chan RunInfo, 8)
	initial, err := manager.Start(context.Background(), Request{Dataset: "dataset.jsonl", Model: "qwen3-4b"}, func(info RunInfo) {
		updates <- info
	})
	if err != nil {
		t.Fatal(err)
	}
	if initial.Status != StatusPreparing || initial.Output == "" {
		t.Fatalf("initial = %+v", initial)
	}
	training := waitForStatus(t, updates, StatusTraining)
	if training.Progress.Completed != 1 {
		t.Fatalf("training = %+v", training)
	}
	if _, err := manager.Start(context.Background(), Request{Dataset: "other.jsonl", Model: "gemma3-270m"}, nil); err == nil || !strings.Contains(err.Error(), "already active") {
		t.Fatalf("second Start() error = %v", err)
	}
	close(release)
	completed := waitForStatus(t, updates, StatusCompleted)
	if completed.Adapter == "" || completed.SelectedCheckpoint == "" || completed.Progress.Stage != StageComplete {
		t.Fatalf("completed = %+v", completed)
	}
	history, err := manager.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != initial.ID || history[0].Status != StatusCompleted {
		t.Fatalf("history = %+v", history)
	}
}

func TestManagerStopsAndRecoversRuns(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	backend := testBackend{run: func(ctx context.Context, request Request, _ ProgressFunc) (Result, error) {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return Result{Status: StatusStopped}, ctx.Err()
	}}
	manager := newTestManager(t, backend)
	updates := make(chan RunInfo, 4)
	if _, err := manager.Start(context.Background(), Request{Dataset: "dataset.jsonl", Model: "gemma3-270m"}, func(info RunInfo) {
		updates <- info
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := manager.Stop(); err != nil {
		t.Fatal(err)
	}
	if stopped := waitForStatus(t, updates, StatusStopped); stopped.Error != context.Canceled.Error() {
		t.Fatalf("stopped = %+v", stopped)
	}

	interrupted := RunInfo{
		Schema: runStateSchema, ID: "interrupted", Dataset: "data.jsonl", Model: "qwen3-4b",
		Output: filepath.Join(manager.root, "interrupted"), Status: StatusTraining,
		StartedAt: time.Now().Add(-time.Hour), UpdatedAt: time.Now().Add(-time.Hour),
	}
	if err := mkdirAndWriteRun(interrupted); err != nil {
		t.Fatal(err)
	}
	history, err := manager.Recover()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[1].Status != StatusFailed || !strings.Contains(history[1].Error, "ended before") {
		t.Fatalf("recovered history = %+v", history)
	}
}

func TestManagerResumesSameRunWithPersistedRuntime(t *testing.T) {
	requests := make(chan Request, 2)
	backend := testBackend{run: func(_ context.Context, request Request, _ ProgressFunc) (Result, error) {
		requests <- request
		checkpoint := filepath.Join(request.Output, "checkpoints", "epoch-01")
		if err := os.MkdirAll(checkpoint, 0o700); err != nil {
			return Result{}, err
		}
		if len(requests) == 1 {
			return Result{
				Status: StatusFailed, Error: "training interrupted", Resumable: true,
				SelectedEpoch: 1, SelectedCheckpoint: checkpoint,
			}, errors.New("training interrupted")
		}
		adapter := filepath.Join(request.Output, "adapter.gguf")
		if err := os.WriteFile(adapter, []byte("adapter"), 0o600); err != nil {
			return Result{}, err
		}
		return Result{
			Status: StatusCompleted, Adapter: adapter,
			SelectedEpoch: 1, SelectedCheckpoint: checkpoint,
		}, nil
	}}
	manager := newTestManager(t, backend)
	runtime := Runtime{LlamaServer: "llama-server", Port: 1234, ContextSize: 512, GPULayers: 4}
	updates := make(chan RunInfo, 8)
	initial, err := manager.Start(t.Context(), Request{Dataset: "dataset.jsonl", Model: "gemma.gguf", Runtime: runtime}, func(info RunInfo) {
		updates <- info
	})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitForStatus(t, updates, StatusFailed)
	if !failed.Resumable {
		t.Fatalf("failed run = %+v, want resumable", failed)
	}

	resumed, err := manager.Resume(t.Context(), initial.ID, func(info RunInfo) { updates <- info })
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ID != initial.ID || resumed.Output != initial.Output {
		t.Fatalf("resumed run = %+v, want same ID and output as %+v", resumed, initial)
	}
	completed := waitForStatus(t, updates, StatusCompleted)
	if completed.ID != initial.ID {
		t.Fatalf("completed run ID = %q, want %q", completed.ID, initial.ID)
	}
	first := <-requests
	second := <-requests
	if first.Runtime != runtime || second.Runtime != runtime || second.Output != first.Output {
		t.Fatalf("run requests = first %+v, resumed %+v", first, second)
	}
	history, err := manager.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("history contains %d runs, want 1", len(history))
	}
}

func TestManagerRejectsRunWithoutAResumableCheckpoint(t *testing.T) {
	manager := newTestManager(t, testBackend{run: func(context.Context, Request, ProgressFunc) (Result, error) {
		return Result{Status: StatusFailed}, errors.New("failed before training")
	}})
	updates := make(chan RunInfo, 4)
	initial, err := manager.Start(t.Context(), Request{Dataset: "dataset.jsonl", Model: "gemma.gguf"}, func(info RunInfo) {
		updates <- info
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, updates, StatusFailed)
	if _, err := manager.Resume(t.Context(), initial.ID, nil); err == nil || !strings.Contains(err.Error(), "no completed training checkpoint") {
		t.Fatalf("Resume() error = %v", err)
	}
}

func TestManagerRejectsInvalidRunBeforeCreatingOutput(t *testing.T) {
	backend := testBackend{
		preflight: func(context.Context, Request) error { return errors.New("model assets are missing") },
		run:       func(context.Context, Request, ProgressFunc) (Result, error) { return Result{}, nil },
	}
	manager := newTestManager(t, backend)
	if _, err := manager.Start(context.Background(), Request{Dataset: "dataset.jsonl", Model: "qwen3-4b"}, nil); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("Start() error = %v", err)
	}
	history, err := manager.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("history = %+v", history)
	}
}

func TestManagerRejectsBackendArtifactOutsideManagedRun(t *testing.T) {
	backend := testBackend{run: func(_ context.Context, _ Request, _ ProgressFunc) (Result, error) {
		return Result{Adapter: "/unmanaged/adapter.gguf", Status: StatusCompleted}, nil
	}}
	manager := newTestManager(t, backend)
	updates := make(chan RunInfo, 3)
	if _, err := manager.Start(context.Background(), Request{Dataset: "dataset.jsonl", Model: "qwen3-4b"}, func(info RunInfo) {
		updates <- info
	}); err != nil {
		t.Fatal(err)
	}
	failed := waitForStatus(t, updates, StatusFailed)
	if failed.Adapter != "" || !strings.Contains(failed.Error, "outside the managed run") {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestManagerRejectsMissingAndSymlinkedBackendArtifacts(t *testing.T) {
	tests := map[string]func(Request) (string, error){
		"missing": func(request Request) (string, error) {
			return filepath.Join(request.Output, "missing.gguf"), nil
		},
		"symlink": func(request Request) (string, error) {
			outside := filepath.Join(t.TempDir(), "adapter.gguf")
			if err := os.WriteFile(outside, []byte("adapter"), 0o600); err != nil {
				return "", err
			}
			link := filepath.Join(request.Output, "adapter.gguf")
			return link, os.Symlink(outside, link)
		},
	}
	for name, artifact := range tests {
		t.Run(name, func(t *testing.T) {
			backend := testBackend{run: func(_ context.Context, request Request, _ ProgressFunc) (Result, error) {
				adapter, err := artifact(request)
				return Result{Adapter: adapter, Status: StatusCompleted}, err
			}}
			manager := newTestManager(t, backend)
			updates := make(chan RunInfo, 3)
			if _, err := manager.Start(context.Background(), Request{Dataset: "dataset.jsonl", Model: "qwen3-4b"}, func(info RunInfo) {
				updates <- info
			}); err != nil {
				t.Fatal(err)
			}
			if failed := waitForStatus(t, updates, StatusFailed); failed.Adapter != "" {
				t.Fatalf("failed run retained adapter: %+v", failed)
			}
		})
	}
}

func TestManagerRequiresSelectedCheckpointDirectory(t *testing.T) {
	root := t.TempDir()
	checkpointFile := filepath.Join(root, "checkpoint")
	if err := os.WriteFile(checkpointFile, []byte("checkpoint"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateResultPaths(root, Result{SelectedCheckpoint: checkpointFile}); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("file checkpoint error = %v", err)
	}

	checkpointDirectory := filepath.Join(root, "epoch-01")
	if err := os.Mkdir(checkpointDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateResultPaths(root, Result{SelectedCheckpoint: checkpointDirectory}); err != nil {
		t.Fatalf("directory checkpoint error = %v", err)
	}

	checkpointLink := filepath.Join(root, "checkpoint-link")
	if err := os.Symlink(checkpointDirectory, checkpointLink); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := validateResultPaths(root, Result{SelectedCheckpoint: checkpointLink}); err == nil {
		t.Fatal("symlinked checkpoint was accepted")
	}
}

func TestManagerRejectsInvalidBackendProgress(t *testing.T) {
	backend := testBackend{run: func(_ context.Context, _ Request, progress ProgressFunc) (Result, error) {
		progress(Progress{Stage: StageTraining, Completed: 2, Total: 1})
		return Result{Status: StatusCompleted}, nil
	}}
	manager := newTestManager(t, backend)
	updates := make(chan RunInfo, 3)
	if _, err := manager.Start(context.Background(), Request{Dataset: "dataset.jsonl", Model: "qwen3-4b"}, func(info RunInfo) {
		updates <- info
	}); err != nil {
		t.Fatal(err)
	}
	failed := waitForStatus(t, updates, StatusFailed)
	if !strings.Contains(failed.Error, "invalid progress") {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestManagerRejectsInvalidBackendResult(t *testing.T) {
	backend := testBackend{run: func(_ context.Context, _ Request, _ ProgressFunc) (Result, error) {
		return Result{Status: "running"}, nil
	}}
	manager := newTestManager(t, backend)
	updates := make(chan RunInfo, 3)
	if _, err := manager.Start(context.Background(), Request{Dataset: "dataset.jsonl", Model: "qwen3-4b"}, func(info RunInfo) {
		updates <- info
	}); err != nil {
		t.Fatal(err)
	}
	failed := waitForStatus(t, updates, StatusFailed)
	if !strings.Contains(failed.Error, "invalid terminal status") {
		t.Fatalf("failed = %+v", failed)
	}
}

func newTestManager(t *testing.T, backend Backend) *Manager {
	t.Helper()
	manager, err := NewManager(t.TempDir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func waitForStatus(t *testing.T, updates <-chan RunInfo, status Status) RunInfo {
	t.Helper()
	var observed []RunInfo
	for {
		select {
		case info := <-updates:
			observed = append(observed, info)
			if info.Status == status {
				return info
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %q; observed %+v", status, observed)
		}
	}
}

func mkdirAndWriteRun(info RunInfo) error {
	if err := os.Mkdir(info.Output, 0o755); err != nil {
		return err
	}
	return writeRunInfo(info)
}

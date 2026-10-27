package train_test

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/train"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

type trainer struct {
	t    *testing.T
	path string
	err  error
	loss float64
}

func (s trainer) Train(_ context.Context, job training.TrainingJob, _ func(tmprl.Progress)) (training.TrainingCandidate, error) {
	if len(job.Dataset.Test) != 0 {
		s.t.Fatal("protected rows exposed to training")
	}
	return training.TrainingCandidate{Path: s.path, BestEpoch: 1, TrainingLoss: s.loss}, s.err
}
func (trainer) Predict(context.Context, training.TrainingJob, string, []training.Record) ([]string, error) {
	panic("unexpected prediction")
}

func TestNonfiniteTrainingLossDoesNotCommitCandidate(t *testing.T) {
	for _, loss := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		workspace, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer workspace.Close()
		scratch := t.TempDir()
		path := filepath.Join(scratch, "candidate.gguf")
		if err := os.WriteFile(path, []byte("adapter"), 0o600); err != nil {
			t.Fatal(err)
		}
		job := training.TrainingJob{Workdir: scratch, Training: training.Training{MaximumEpochs: 1}}
		_, err = train.Run(t.Context(), workspace, trainer{t: t, path: path, loss: loss}, job, nil)
		var app *temporal.ApplicationError
		if !errors.As(err, &app) || !app.NonRetryable() || app.Type() != "invalid-step-output" {
			t.Fatalf("loss %v: got %v, want nonretryable invalid output", loss, err)
		}
		for _, file := range []string{"candidate.gguf", "candidate.json"} {
			if _, err := workspace.Stat(file); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid loss published %s: %v", file, err)
			}
		}
	}
}

func TestCandidateConfinementAndProtectedRows(t *testing.T) {
	for _, escape := range []bool{false, true} {
		t.Run(map[bool]string{false: "inside", true: "outside"}[escape], func(t *testing.T) {
			workspace, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer workspace.Close()
			scratch := t.TempDir()
			path := filepath.Join(scratch, "candidate.gguf")
			if escape {
				path = filepath.Join(t.TempDir(), "candidate.gguf")
			}
			if err := os.WriteFile(path, []byte("adapter"), 0o600); err != nil {
				t.Fatal(err)
			}
			job := training.TrainingJob{Workdir: scratch, Training: training.Training{MaximumEpochs: 1}, Dataset: training.TrainingDataset{Test: []training.Record{{Prompt: "secret"}}}}
			output, err := train.Run(t.Context(), workspace, trainer{t: t, path: path}, job, func(tmprl.Progress) {})
			if escape {
				if err == nil {
					t.Fatal("escaped candidate accepted")
				}
				if _, err := workspace.Stat("candidate.gguf"); !os.IsNotExist(err) {
					t.Fatalf("candidate published: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output.Checkpoint)
			if err != nil || string(data) != "adapter" {
				t.Fatalf("candidate: %q, %v", data, err)
			}
			incoming := training.Artifacts{Dataset: "dataset", Checkpoint: "old", Report: "report", Adapter: "adapter", Accepted: true}
			var step training.TrainingStep = train.Step{}
			result, err := step.Execute(t.Context(), training.StepContext{Workspace: workspace, Trainer: trainer{t: t, path: path}, Job: job, Artifacts: incoming})
			if err != nil {
				t.Fatal(err)
			}
			incoming.Checkpoint = output.Checkpoint
			if step.Name() != "train" || result.Status != "" || result.Artifacts != incoming {
				t.Fatalf("stage result = %+v, want artifacts %+v and continuing status", result, incoming)
			}
		})
	}
}

func TestFailureDoesNotCommitCandidate(t *testing.T) {
	failure := errors.New("optimizer failed")
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "optimizer-error", true: "cancellation"}[cancelled], func(t *testing.T) {
			workspace, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer workspace.Close()
			scratch := t.TempDir()
			path := filepath.Join(scratch, "candidate.gguf")
			if err := os.WriteFile(path, []byte("uncommitted candidate"), 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			providerErr, want := failure, failure
			if cancelled {
				cancel()
				providerErr, want = nil, context.Canceled
			}
			job := training.TrainingJob{Workdir: scratch, Training: training.Training{MaximumEpochs: 1}}
			_, err = train.Run(ctx, workspace, trainer{t: t, path: path, err: providerErr}, job, func(tmprl.Progress) {})
			if !errors.Is(err, want) {
				t.Fatalf("training error = %v, want %v", err, want)
			}
			for _, file := range []string{"candidate.gguf", "candidate.json"} {
				if _, err := workspace.Stat(file); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("failed training published %s: %v", file, err)
				}
			}
		})
	}
}

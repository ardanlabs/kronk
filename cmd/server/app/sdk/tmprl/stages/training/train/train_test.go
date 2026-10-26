package train_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/train"
)

type trainer struct {
	t    *testing.T
	path string
	err  error
}

func (s trainer) Train(_ context.Context, job model.TrainingJob, _ func(model.Progress)) (model.TrainingCandidate, error) {
	if len(job.Dataset.Test) != 0 {
		s.t.Fatal("protected rows exposed to training")
	}
	return model.TrainingCandidate{Path: s.path, BestEpoch: 1}, s.err
}
func (trainer) Predict(context.Context, model.TrainingJob, string, []model.Record) ([]string, error) {
	panic("unexpected prediction")
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
			job := model.TrainingJob{Workdir: scratch, Training: model.Training{MaximumEpochs: 1}, Dataset: model.TrainingDataset{Test: []model.Record{{Prompt: "secret"}}}}
			output, err := train.Run(t.Context(), workspace, trainer{t: t, path: path}, job, func(model.Progress) {})
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
			job := model.TrainingJob{Workdir: scratch, Training: model.Training{MaximumEpochs: 1}}
			_, err = train.Run(ctx, workspace, trainer{t: t, path: path, err: providerErr}, job, func(model.Progress) {})
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

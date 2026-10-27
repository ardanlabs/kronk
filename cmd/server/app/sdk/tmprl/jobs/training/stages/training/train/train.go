// Package train produces and persists a candidate without exposing test rows
// to model optimization or checkpoint selection.
package train

import (
	"context"
	"math"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Run trains in the caller-owned scratch directory and commits a validated
// candidate to workspace. The caller owns both directory lifetimes.
func Run(ctx context.Context, workspace *os.Root, trainer training.Trainer, job training.TrainingJob, publish func(tmprl.Progress)) (training.Artifacts, error) {
	job.Dataset.Test = nil
	candidate, err := trainer.Train(ctx, job, publish)
	if err != nil {
		return training.Artifacts{}, err
	}
	if candidate.BestEpoch < 1 || candidate.BestEpoch > job.Training.MaximumEpochs || math.IsNaN(candidate.TrainingLoss) || math.IsInf(candidate.TrainingLoss, 0) || math.IsNaN(candidate.ValidationLoss) || math.IsInf(candidate.ValidationLoss, 0) {
		return training.Artifacts{}, training.InvalidOutput("train")
	}
	relative, err := filepath.Rel(job.Workdir, candidate.Path)
	if err != nil {
		return training.Artifacts{}, err
	}
	private, err := os.OpenRoot(job.Workdir)
	if err != nil {
		return training.Artifacts{}, err
	}
	defer private.Close()
	file, err := private.Open(relative)
	if err != nil {
		return training.Artifacts{}, err
	}
	defer file.Close()
	if err := CopyArtifact(ctx, workspace, "candidate.gguf", file); err != nil {
		return training.Artifacts{}, err
	}
	candidate.Path = filepath.Join(workspace.Name(), "candidate.gguf")
	if err := tmprl.WriteJSON(workspace, "candidate.json", candidate); err != nil {
		return training.Artifacts{}, err
	}
	return training.Artifacts{Checkpoint: candidate.Path}, nil
}

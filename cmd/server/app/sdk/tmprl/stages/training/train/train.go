// Package train produces and persists a candidate without exposing test rows
// to model optimization or checkpoint selection.
package train

import (
	"context"
	"math"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"
)

// Run trains in the caller-owned scratch directory and commits a validated
// candidate to workspace. The caller owns both directory lifetimes.
func Run(ctx context.Context, workspace *os.Root, trainer model.Trainer, job model.TrainingJob, publish func(model.Progress)) (model.Artifacts, error) {
	job.Dataset.Test = nil
	candidate, err := trainer.Train(ctx, job, publish)
	if err != nil {
		return model.Artifacts{}, err
	}
	if candidate.BestEpoch < 1 || candidate.BestEpoch > job.Training.MaximumEpochs || math.IsNaN(candidate.ValidationLoss) || math.IsInf(candidate.ValidationLoss, 0) {
		return model.Artifacts{}, model.InvalidOutput(model.StageTrain)
	}
	relative, err := filepath.Rel(job.Workdir, candidate.Path)
	if err != nil {
		return model.Artifacts{}, err
	}
	private, err := os.OpenRoot(job.Workdir)
	if err != nil {
		return model.Artifacts{}, err
	}
	defer private.Close()
	file, err := private.Open(relative)
	if err != nil {
		return model.Artifacts{}, err
	}
	defer file.Close()
	if err := CopyArtifact(ctx, workspace, "candidate.gguf", file); err != nil {
		return model.Artifacts{}, err
	}
	candidate.Path = filepath.Join(workspace.Name(), "candidate.gguf")
	if err := dataset.WriteJSON(workspace, "candidate.json", candidate); err != nil {
		return model.Artifacts{}, err
	}
	return model.Artifacts{Checkpoint: candidate.Path}, nil
}

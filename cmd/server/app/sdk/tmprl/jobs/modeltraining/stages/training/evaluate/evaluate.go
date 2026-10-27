// Package evaluate compares a trained candidate with the frozen base model
// against protected test answers and persists the acceptance decision.
package evaluate

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training/train"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Run snapshots the candidate into the caller-owned scratch directory and
// writes the report to workspace. The caller owns both directory lifetimes.
func Run(ctx context.Context, workspace *os.Root, trainer training.Trainer, job training.TrainingJob) (training.Artifacts, error) {
	file, err := workspace.Open("candidate.gguf")
	if err != nil {
		return training.Artifacts{}, err
	}
	defer file.Close()
	private, err := os.OpenRoot(job.Workdir)
	if err != nil {
		return training.Artifacts{}, err
	}
	defer private.Close()
	if err := train.CopyArtifact(ctx, private, "candidate.gguf", file); err != nil {
		return training.Artifacts{}, err
	}
	base, err := trainer.Predict(ctx, job, "", job.Dataset.Test)
	if err != nil {
		return training.Artifacts{}, err
	}
	adapted, err := trainer.Predict(ctx, job, filepath.Join(job.Workdir, "candidate.gguf"), job.Dataset.Test)
	if err != nil {
		return training.Artifacts{}, err
	}
	if len(base) != len(job.Dataset.Test) || len(adapted) != len(base) {
		return training.Artifacts{}, training.InvalidOutput("evaluate")
	}
	report := EvaluationReport{Total: len(base)}
	for i, record := range job.Dataset.Test {
		report.Answers = append(report.Answers, EvaluationAnswer{Prompt: record.Prompt, Expected: record.Target, BaseAnswer: base[i], AdapterAnswer: adapted[i]})
		if training.Key(base[i]) == training.Key(record.Target) {
			report.BaseCorrect++
		}
		if training.Key(adapted[i]) == training.Key(record.Target) {
			report.AdapterCorrect++
		}
	}
	report.Accepted = report.Total >= 20 && report.AdapterCorrect*10 >= report.Total*9 && report.AdapterCorrect >= report.BaseCorrect
	if err := tmprl.WriteJSON(workspace, "evaluation.json", report); err != nil {
		return training.Artifacts{}, err
	}
	return training.Artifacts{Report: filepath.Join(workspace.Name(), "evaluation.json"), Accepted: report.Accepted}, nil
}

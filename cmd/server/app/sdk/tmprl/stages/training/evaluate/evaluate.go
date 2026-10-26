// Package evaluate compares a trained candidate with the frozen base model
// against protected test answers and persists the acceptance decision.
package evaluate

import (
	"context"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/train"
)

// Run snapshots the candidate into the caller-owned scratch directory and
// writes the report to workspace. The caller owns both directory lifetimes.
func Run(ctx context.Context, workspace *os.Root, trainer model.Trainer, job model.TrainingJob) (model.Artifacts, error) {
	file, err := workspace.Open("candidate.gguf")
	if err != nil {
		return model.Artifacts{}, err
	}
	defer file.Close()
	private, err := os.OpenRoot(job.Workdir)
	if err != nil {
		return model.Artifacts{}, err
	}
	defer private.Close()
	if err := train.CopyArtifact(ctx, private, "candidate.gguf", file); err != nil {
		return model.Artifacts{}, err
	}
	base, err := trainer.Predict(ctx, job, "", job.Dataset.Test)
	if err != nil {
		return model.Artifacts{}, err
	}
	adapted, err := trainer.Predict(ctx, job, filepath.Join(job.Workdir, "candidate.gguf"), job.Dataset.Test)
	if err != nil {
		return model.Artifacts{}, err
	}
	if len(base) != len(job.Dataset.Test) || len(adapted) != len(base) {
		return model.Artifacts{}, model.InvalidOutput(model.StageEvaluate)
	}
	report := model.EvaluationReport{Total: len(base)}
	for i, record := range job.Dataset.Test {
		report.Answers = append(report.Answers, model.EvaluationAnswer{Prompt: record.Prompt, Expected: record.Target, BaseAnswer: base[i], AdapterAnswer: adapted[i]})
		if dataset.Key(base[i]) == dataset.Key(record.Target) {
			report.BaseCorrect++
		}
		if dataset.Key(adapted[i]) == dataset.Key(record.Target) {
			report.AdapterCorrect++
		}
	}
	report.Accepted = report.Total >= 20 && report.AdapterCorrect*10 >= report.Total*9 && report.AdapterCorrect >= report.BaseCorrect
	if err := dataset.WriteJSON(workspace, "evaluation.json", report); err != nil {
		return model.Artifacts{}, err
	}
	return model.Artifacts{Report: filepath.Join(workspace.Name(), "evaluation.json"), Accepted: report.Accepted}, nil
}

// Package publish releases only candidates accepted by evaluation.
package publish

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/train"
)

// Run requires agreement between the workflow and persisted evaluation report
// before publishing an adapter in the caller-owned workspace.
func Run(ctx context.Context, workspace *os.Root, incoming training.Artifacts) (training.Artifacts, error) {
	file, err := workspace.Open("candidate.gguf")
	if err != nil {
		return training.Artifacts{}, err
	}
	defer file.Close()
	data, err := workspace.ReadFile("evaluation.json")
	if err != nil {
		return training.Artifacts{}, err
	}
	var report evaluate.EvaluationReport
	if err := json.Unmarshal(data, &report); err != nil {
		return training.Artifacts{}, err
	}
	if !report.Accepted || !incoming.Accepted {
		return training.Artifacts{}, training.InvalidOutput("publish")
	}
	if err := train.CopyArtifact(ctx, workspace, "adapter.gguf", file); err != nil {
		return training.Artifacts{}, err
	}
	return training.Artifacts{Adapter: filepath.Join(workspace.Name(), "adapter.gguf")}, nil
}

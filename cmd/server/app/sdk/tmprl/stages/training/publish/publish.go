// Package publish releases only candidates accepted by evaluation.
package publish

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/train"
)

// Run requires agreement between the workflow and persisted evaluation report
// before publishing an adapter in the caller-owned workspace.
func Run(ctx context.Context, workspace *os.Root, incoming model.Artifacts) (model.Artifacts, error) {
	file, err := workspace.Open("candidate.gguf")
	if err != nil {
		return model.Artifacts{}, err
	}
	defer file.Close()
	data, err := workspace.ReadFile("evaluation.json")
	if err != nil {
		return model.Artifacts{}, err
	}
	var report model.EvaluationReport
	if err := json.Unmarshal(data, &report); err != nil {
		return model.Artifacts{}, err
	}
	if !report.Accepted || !incoming.Accepted {
		return model.Artifacts{}, model.InvalidOutput(model.StagePublish)
	}
	if err := train.CopyArtifact(ctx, workspace, "adapter.gguf", file); err != nil {
		return model.Artifacts{}, err
	}
	return model.Artifacts{Adapter: filepath.Join(workspace.Name(), "adapter.gguf")}, nil
}

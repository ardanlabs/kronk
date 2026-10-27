package training

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// Evaluation reads the persisted report, including a rejected candidate's report.
func (b *Business) Evaluation(ctx context.Context, e tmprl.Execution) (evaluate.EvaluationReport, error) {
	file, err := b.openArtifact(ctx, e, false)
	if err != nil {
		return evaluate.EvaluationReport{}, err
	}
	defer file.Close()
	var report evaluate.EvaluationReport
	err = json.NewDecoder(tmprl.ContextReader{Context: ctx, Reader: file}).Decode(&report)
	return report, err
}

// OpenAdapter streams an accepted, published adapter. The caller closes it.
func (b *Business) OpenAdapter(ctx context.Context, e tmprl.Execution) (io.ReadCloser, error) {
	return b.openArtifact(ctx, e, true)
}

func (b *Business) openArtifact(ctx context.Context, e tmprl.Execution, adapter bool) (*os.File, error) {
	run, err := b.jobs.Get(ctx, e)
	if err != nil {
		return nil, err
	}
	name := "evaluation.json"
	if run.Artifacts.Training.Report == "" || adapter && (run.Status != tmprl.StatusCompleted || !run.Artifacts.Training.Accepted || run.Artifacts.Training.Adapter == "") {
		return nil, fmt.Errorf("%w: training artifact is not available", tmprl.ErrInvalidArgument)
	}
	if adapter {
		name = "adapter.gguf"
	}
	root, err := os.OpenRoot(b.cfg.Imports.Root())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(filepath.Join(storageDirectory, run.RunID, "steps", "training", name))
}

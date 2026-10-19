// Package split validates and locks a run's training, validation, and test data.
package split

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/provider"
	"go.temporal.io/sdk/temporal"
)

// State is the persisted split identity shared by subsequent training stages.
type State struct {
	Fingerprint string                `json:"fingerprint"`
	Dataset     model.TrainingDataset `json:"dataset"`
}

// Run checks model context support and publishes the immutable split identity.
// The caller owns workspace and supplies the fingerprint of its resolved inputs.
func Run(ctx context.Context, workspace *os.Root, trainer model.Trainer, job model.TrainingJob, records []model.Record, fingerprint string) (model.Artifacts, error) {
	splits, err := dataset.Split(records)
	if err != nil {
		return model.Artifacts{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
	}
	job.Dataset = splits
	if err := provider.Preflight(ctx, trainer, job); err != nil {
		if errors.Is(err, model.ErrInvalidArgument) {
			return model.Artifacts{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
		}
		return model.Artifacts{}, err
	}
	state := State{Fingerprint: fingerprint, Dataset: splits}
	if data, err := workspace.ReadFile("dataset.json"); err == nil {
		var previous struct {
			Fingerprint string `json:"fingerprint"`
		}
		if err := json.Unmarshal(data, &previous); err != nil {
			return model.Artifacts{}, err
		}
		if previous.Fingerprint != fingerprint {
			return model.Artifacts{}, fmt.Errorf("training input changed")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return model.Artifacts{}, err
	}
	if err := dataset.WriteJSON(workspace, "dataset.json", state); err != nil {
		return model.Artifacts{}, err
	}
	return model.Artifacts{Dataset: filepath.Join(workspace.Name(), "dataset.json")}, nil
}

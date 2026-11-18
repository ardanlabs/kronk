// Package split validates and locks a run's training, validation, and test data.
package split

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

// State is the persisted split identity shared by subsequent training steps.
type State struct {
	Fingerprint string                   `json:"fingerprint"`
	Dataset     training.TrainingDataset `json:"dataset"`
}

// Run checks model context support and publishes the immutable split identity.
// The caller owns workspace and supplies the fingerprint of its resolved inputs.
func Run(ctx context.Context, workspace *os.Root, trainer training.Trainer, job training.TrainingJob, records []training.Record, fingerprint string) (training.Artifacts, error) {
	splits, err := training.Split(records)
	if err != nil {
		count := 0
		for _, record := range records {
			if !record.ValidationPrompt && !record.IndependentPrompt {
				count++
			}
		}
		if training.ValidateRecords(records) == nil && count < training.MinimumTrainingRecords {
			issue := struct {
				Code     string `json:"code"`
				Count    int    `json:"count"`
				Required int    `json:"required"`
			}{"training-records-insufficient", count, training.MinimumTrainingRecords}
			if saveErr := tmprl.WriteJSON(workspace, "requirements.json", issue); saveErr != nil {
				return training.Artifacts{}, saveErr
			}
			return training.Artifacts{}, temporal.NewNonRetryableApplicationError(err.Error(), issue.Code, err)
		}
		return training.Artifacts{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
	}
	job.Dataset = splits
	if trainer == nil {
		return training.Artifacts{}, fmt.Errorf("%w: trainer is required", training.ErrInvalidArgument)
	}
	if err := trainer.Validate(ctx, job); err != nil {
		if errors.Is(err, training.ErrInvalidArgument) {
			return training.Artifacts{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
		}
		return training.Artifacts{}, err
	}
	state := State{Fingerprint: fingerprint, Dataset: splits}
	if data, err := workspace.ReadFile("dataset.json"); err == nil {
		var previous struct {
			Fingerprint string `json:"fingerprint"`
		}
		if err := json.Unmarshal(data, &previous); err != nil {
			return training.Artifacts{}, err
		}
		if previous.Fingerprint != fingerprint {
			return training.Artifacts{}, fmt.Errorf("training input changed")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return training.Artifacts{}, err
	}
	if err := tmprl.WriteJSON(workspace, "dataset.json", state); err != nil {
		return training.Artifacts{}, err
	}
	return training.Artifacts{Dataset: filepath.Join(workspace.Name(), "dataset.json")}, nil
}

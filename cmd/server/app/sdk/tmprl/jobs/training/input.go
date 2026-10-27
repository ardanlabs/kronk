package training

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/imports"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

func (b *Business) validate(ctx context.Context, r Request) (Request, error) {
	r.ID, r.Model, r.DatasetID = strings.TrimSpace(r.ID), strings.TrimSpace(r.Model), strings.TrimSpace(r.DatasetID)
	if len(r.ID) > 200 || b.cfg.Models[r.Model] == "" {
		return Request{}, fmt.Errorf("%w: approved model and ID of at most 200 bytes are required", tmprl.ErrInvalidArgument)
	}
	if (r.Prepare == nil) == (r.DatasetID == "") {
		return Request{}, fmt.Errorf("%w: select exactly one document preparation or prepared dataset", tmprl.ErrInvalidArgument)
	}
	if r.Training == (optimization.Training{}) {
		r.Training = optimization.Training{MaximumEpochs: 3, BatchSize: 2, LearningRate: 0.0002, Seed: 3407}
	}
	if r.Training.LoRARank == 0 {
		r.Training.LoRARank = 8
	}
	if r.Training.LoRAAlpha == 0 {
		r.Training.LoRAAlpha = 8
	}
	t := r.Training
	if t.MaximumEpochs < 1 || t.MaximumEpochs > 100 || t.BatchSize < 1 || t.BatchSize > 128 || t.Seed < 0 || t.LoRARank < 1 || t.LoRAAlpha <= 0 || math.IsNaN(t.LoRAAlpha) || math.IsInf(t.LoRAAlpha, 0) || t.LearningRate <= 0 || t.LearningRate > 1 || math.IsNaN(t.LearningRate) || math.IsInf(t.LearningRate, 0) {
		return Request{}, fmt.Errorf("%w: invalid training hyperparameters", tmprl.ErrInvalidArgument)
	}
	if r.Prepare != nil {
		if b.cfg.Completer == nil {
			return Request{}, fmt.Errorf("%w: document preparation requires a completer", tmprl.ErrInvalidArgument)
		}
		prepared, err := preparation.ValidateRequest(ctx, b.cfg.Imports, *r.Prepare)
		if err != nil {
			return Request{}, err
		}
		r.Prepare = &prepared
	} else if _, err := b.records(ctx, r, Artifacts{}, ""); err != nil {
		return Request{}, err
	}
	return r, nil
}

func invalidInput(message string) error {
	return temporal.NewNonRetryableApplicationError(message, "invalid-training", nil)
}

func stepError(err error) error {
	if errors.Is(err, tmprl.ErrInvalidArgument) || errors.Is(err, imports.ErrNotFound) {
		return temporal.NewNonRetryableApplicationError(err.Error(), "invalid-training", err)
	}
	return err
}

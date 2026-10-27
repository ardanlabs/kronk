package training_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	job "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/imports"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/mocks"
)

func TestRejectInvalidRequests(t *testing.T) {
	storage, err := imports.New(imports.Config{DatasetRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	// No client expectations: invalid submissions must not contact Temporal.
	business, err := job.New(mocks.NewClient(t), job.Config{Imports: storage, Models: map[string]string{"gemma": "model.gguf"}, Trainer: &trainer{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		input job.Request
	}{
		{name: "missing-source", input: job.Request{Model: "gemma"}},
		{name: "two-sources", input: job.Request{Model: "gemma", DatasetID: "dataset", Prepare: &preparation.PreparationRequest{}}},
		{name: "unapproved-model", input: job.Request{Model: "other", DatasetID: "dataset"}},
		{name: "oversized-id", input: job.Request{ID: strings.Repeat("x", 201), Model: "gemma", DatasetID: "dataset"}},
		{name: "nonfinite-rate", input: job.Request{Model: "gemma", DatasetID: "dataset", Training: optimization.Training{MaximumEpochs: 2, BatchSize: 3, LearningRate: math.NaN()}}},
		{name: "negative-seed", input: job.Request{Model: "gemma", DatasetID: "dataset", Training: optimization.Training{MaximumEpochs: 2, BatchSize: 3, LearningRate: 0.001, Seed: -1}}},
		{name: "missing-completer", input: job.Request{Model: "gemma", Prepare: &preparation.PreparationRequest{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := business.Start(t.Context(), test.input); !errors.Is(err, tmprl.ErrInvalidArgument) {
				t.Fatalf("Start: got %v, want invalid argument", err)
			}
		})
	}
}

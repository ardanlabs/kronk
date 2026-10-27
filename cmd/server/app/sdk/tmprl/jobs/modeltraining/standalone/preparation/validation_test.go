package prepare_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/imports"
	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	prepare "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/standalone/preparation"
	"go.temporal.io/sdk/mocks"
)

func TestPreparationValidateRequest(t *testing.T) {
	b := newPreparationFixture(t, &mocks.Client{}, preparationOptions{DatasetRoot: t.TempDir()})
	imported, err := b.Datasets.ImportDataset(t.Context(), imports.ImportInput{Name: "source.md"}, strings.NewReader(preparationSource))
	if err != nil {
		t.Fatal(err)
	}
	input := PreparationRequest{ID: "  supplied-id  ", DatasetID: imported.ID, Model: "  preparer  ", Instructions: "  answer clearly  ", Goal: GoalQuestionAnswer, ReviewedRecords: []training.Record{{Prompt: "What is a goroutine?", Target: preparationSource, Evidence: preparationSource}}}
	normalized, err := b.Preparation.ValidateRequest(t.Context(), input, PreparationFull)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ID != "supplied-id" || normalized.Model != "preparer" || normalized.Instructions != "answer clearly" || normalized.DatasetID != imported.ID {
		t.Fatalf("got normalized request %+v", normalized)
	}
	normalized.ReviewedRecords[0].Prompt = "Changed?"
	if input.ReviewedRecords[0].Prompt != "What is a goroutine?" || input.Model != "  preparer  " {
		t.Fatalf("validation mutated caller input: %+v", input)
	}
	input.ID = " "
	input.ReviewedRecords = nil
	normalized, err = b.Preparation.ValidateRequest(t.Context(), input, PreparationFull)
	if err != nil || normalized.ID != "" || normalized.ReviewedRecords != nil {
		t.Fatalf("got %+v, %v, want no assigned ID and nil records", normalized, err)
	}
	for _, test := range []struct {
		name   string
		change func(*PreparationRequest)
	}{
		{name: "model", change: func(r *PreparationRequest) { r.Model = " " }},
		{name: "id", change: func(r *PreparationRequest) { r.ID = strings.Repeat("x", 201) }},
		{name: "instructions", change: func(r *PreparationRequest) { r.Instructions = strings.Repeat("界", 2001) }},
		{name: "goal", change: func(r *PreparationRequest) { r.Goal = "unknown" }},
		{name: "preview count", change: func(r *PreparationRequest) { r.PreviewRecords = 1 }},
		{name: "reviewed", change: func(r *PreparationRequest) { r.ReviewedRecords = []training.Record{{Prompt: "Missing evidence?"}} }},
		{name: "size", change: func(r *PreparationRequest) { r.Model = strings.Repeat("x", 64<<10) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := input
			test.change(&request)
			_, err := b.Preparation.ValidateRequest(t.Context(), request, PreparationFull)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("got %v, want invalid argument", err)
			}
			_, standaloneErr := b.Preparation.StartPreparation(t.Context(), request)
			if standaloneErr == nil || standaloneErr.Error() != err.Error() {
				t.Fatalf("got standalone error %v, want %v", standaloneErr, err)
			}
		})
	}
	input.DatasetID = "dataset-" + strings.Repeat("A", 26)
	if _, err := b.Preparation.ValidateRequest(t.Context(), input, PreparationFull); !errors.Is(err, prepare.ErrNotFound) {
		t.Fatalf("got %v, want dataset not found", err)
	}
	var rows strings.Builder
	encoder := json.NewEncoder(&rows)
	for i := range 50 {
		if err := encoder.Encode(training.Record{Prompt: fmt.Sprintf("Question %d?", i), Target: "yes", Evidence: fmt.Sprintf("Source %d says yes.", i)}); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := b.Datasets.ImportDataset(t.Context(), imports.ImportInput{Name: "prepared.jsonl"}, strings.NewReader(rows.String()))
	if err != nil {
		t.Fatal(err)
	}
	input.DatasetID = prepared.ID
	if _, err := b.Preparation.ValidateRequest(t.Context(), input, PreparationFull); !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "prepared JSONL does not need document preparation") {
		t.Fatalf("got %v, want prepared JSONL rejection", err)
	}
}

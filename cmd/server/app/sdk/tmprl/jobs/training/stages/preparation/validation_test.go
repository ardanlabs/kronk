package preparation_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"go.temporal.io/sdk/temporal"
)

func TestValidateRequest(t *testing.T) {
	datasets, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	const evidence = "The harbor opens at dawn."
	dataset, err := datasets.Put(t.Context(), uploads.Input{Name: "source.md"}, strings.NewReader(evidence))
	if err != nil {
		t.Fatal(err)
	}
	input := preparation.PreparationRequest{
		DatasetID: "  " + dataset.ID + "  ", Model: "  preparer  ",
		Instructions: "  answer clearly  ", Goal: preparation.GoalQuestionAnswer,
		ReviewedRecords: []training.Record{{Prompt: "When does the harbor open?", Target: "dawn", Evidence: evidence}},
	}
	normalized, err := preparation.ValidateRequest(t.Context(), datasets, input)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Model != "preparer" || normalized.Instructions != "answer clearly" || normalized.DatasetID != dataset.ID {
		t.Fatalf("got normalized request %+v", normalized)
	}
	normalized.ReviewedRecords[0].Prompt = "Changed?"
	if input.ReviewedRecords[0].Prompt != "When does the harbor open?" || input.Model != "  preparer  " {
		t.Fatalf("validation mutated caller input: %+v", input)
	}
	input.ReviewedRecords = nil
	for _, goal := range []preparation.Goal{preparation.GoalQuestionAnswer, preparation.GoalClassification, preparation.GoalNumericPrediction, preparation.GoalTextGeneration} {
		input.Goal = goal
		normalized, err = preparation.ValidateRequest(t.Context(), datasets, input)
		if err != nil || normalized.ReviewedRecords != nil {
			t.Fatalf("got %+v, %v, want valid goal and nil records", normalized, err)
		}
	}
	for _, test := range []struct {
		name   string
		change func(*preparation.PreparationRequest)
	}{
		{name: "model", change: func(r *preparation.PreparationRequest) { r.Model = " " }},
		{name: "instructions", change: func(r *preparation.PreparationRequest) { r.Instructions = strings.Repeat("界", 2001) }},
		{name: "goal", change: func(r *preparation.PreparationRequest) { r.Goal = "unknown" }},
		{name: "preview count", change: func(r *preparation.PreparationRequest) { r.PreviewRecords = 6 }},
		{name: "negative preview count", change: func(r *preparation.PreparationRequest) { r.PreviewRecords = -1 }},
		{name: "negative question cap", change: func(r *preparation.PreparationRequest) { r.MaxQuestions = -1 }},
		{name: "negative first page", change: func(r *preparation.PreparationRequest) { r.PageStart, r.PageEnd = -1, 5 }},
		{name: "missing last page", change: func(r *preparation.PreparationRequest) { r.PageStart = 11 }},
		{name: "missing first page", change: func(r *preparation.PreparationRequest) { r.PageEnd = 25 }},
		{name: "reversed pages", change: func(r *preparation.PreparationRequest) { r.PageStart, r.PageEnd = 25, 11 }},
		{name: "pages for non-PDF", change: func(r *preparation.PreparationRequest) { r.PageStart, r.PageEnd = 11, 25 }},
		{name: "reviewed", change: func(r *preparation.PreparationRequest) {
			r.ReviewedRecords = []training.Record{{Prompt: "Missing evidence?"}}
		}},
		{name: "too many reviewed", change: func(r *preparation.PreparationRequest) { r.ReviewedRecords = make([]training.Record, 6) }},
		{name: "size", change: func(r *preparation.PreparationRequest) { r.Model = strings.Repeat("x", 64<<10) }},
		{name: "dataset ID", change: func(r *preparation.PreparationRequest) { r.DatasetID = "../escape" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := input
			test.change(&request)
			if _, err := preparation.ValidateRequest(t.Context(), datasets, request); !errors.Is(err, preparation.ErrInvalidArgument) {
				t.Fatalf("got %v, want invalid argument", err)
			}
		})
	}
	input.DatasetID = "upload-" + strings.Repeat("A", 26)
	if _, err := preparation.ValidateRequest(t.Context(), datasets, input); !errors.Is(err, uploads.ErrNotFound) {
		t.Fatalf("got %v, want dataset not found", err)
	}
	var rows strings.Builder
	encoder := json.NewEncoder(&rows)
	for i := range 50 {
		if err := encoder.Encode(training.Record{Prompt: fmt.Sprintf("Question %d?", i), Target: "yes", Evidence: fmt.Sprintf("Source %d says yes.", i)}); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := datasets.Put(t.Context(), uploads.Input{Name: "prepared.jsonl"}, strings.NewReader(rows.String()))
	if err != nil {
		t.Fatal(err)
	}
	input.DatasetID = prepared.ID
	if _, err := preparation.ValidateRequest(t.Context(), datasets, input); !errors.Is(err, preparation.ErrInvalidArgument) || !strings.Contains(err.Error(), "prepared JSONL does not need document preparation") {
		t.Fatalf("got %v, want prepared JSONL rejection", err)
	}
	unsupported, err := datasets.Put(t.Context(), uploads.Input{Name: "source.csv"}, strings.NewReader("value\n42\n"))
	if err != nil {
		t.Fatal(err)
	}
	input.DatasetID = unsupported.ID
	if _, err := preparation.ValidateRequest(t.Context(), datasets, input); !errors.Is(err, preparation.ErrInvalidArgument) {
		t.Fatalf("got %v, want unsupported document rejection", err)
	}
	if _, err := preparation.ValidateRequest(t.Context(), nil, input); !errors.Is(err, preparation.ErrInvalidArgument) {
		t.Fatalf("got %v, want missing dataset service rejection", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := preparation.ValidateRequest(ctx, datasets, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context canceled", err)
	}
}

func TestInvalidOutput(t *testing.T) {
	var applicationError *temporal.ApplicationError
	err := preparation.InvalidOutput("verify")
	if !errors.As(err, &applicationError) || applicationError.Type() != "invalid-step-output" || !applicationError.NonRetryable() {
		t.Fatalf("got %v, want nonretryable invalid-step-output", err)
	}
}

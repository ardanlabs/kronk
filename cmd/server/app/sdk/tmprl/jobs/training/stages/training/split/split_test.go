package split_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/split"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

type trainer struct{ err error }

func (tr trainer) Validate(_ context.Context, job training.TrainingJob) error {
	if len(job.Dataset.Train) < 20 || len(job.Dataset.Validation) < 10 || len(job.Dataset.Test) < 20 {
		return errors.New("missing validation splits")
	}
	return tr.err
}

func (trainer) Train(context.Context, training.TrainingJob, func(tmprl.Progress)) (training.TrainingCandidate, error) {
	panic("unexpected training")
}

func (trainer) Predict(context.Context, training.TrainingJob, string, []training.Record, func(int)) ([]string, error) {
	panic("unexpected prediction")
}

func TestRunLocksDisjointSplits(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var records []training.Record
	for i := range 60 {
		for j := range 2 {
			records = append(records, training.Record{Prompt: fmt.Sprintf("Question %d variant %d?", i, j), Target: fmt.Sprintf("answer%d", i), Evidence: fmt.Sprintf("Source %d says answer%d", i, i)})
		}
	}
	rejected := errors.New("model unavailable")
	if _, err := split.Run(t.Context(), root, trainer{err: rejected}, training.TrainingJob{}, records, "locked"); !errors.Is(err, rejected) {
		t.Fatalf("validation: got %v, want %v", err, rejected)
	}
	if _, err := root.Stat("dataset.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed validation published dataset: %v", err)
	}
	artifacts, err := split.Run(t.Context(), root, trainer{}, training.TrainingJob{}, records, "locked")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(artifacts.Dataset)
	if err != nil {
		t.Fatal(err)
	}
	var state split.State
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state.Fingerprint != "locked" || len(state.Dataset.Train) < 20 || len(state.Dataset.Validation) < 10 || len(state.Dataset.Test) < 20 {
		t.Fatalf("invalid split: %+v", state)
	}
	seen := map[string]string{}
	total := 0
	for name, rows := range map[string][]training.Record{"train": state.Dataset.Train, "validation": state.Dataset.Validation, "test": state.Dataset.Test} {
		for _, r := range rows {
			if previous, ok := seen[r.Evidence]; ok && previous != name {
				t.Fatalf("source leaked from %s to %s: %s", previous, name, r.Evidence)
			}
			seen[r.Evidence] = name
			total++
		}
	}
	if total != len(records) {
		t.Fatalf("lost records: %d of %d", total, len(records))
	}
	incoming := training.Artifacts{Dataset: "old", Checkpoint: "checkpoint", Report: "report", Adapter: "adapter", Accepted: true}
	var step training.TrainingStep = split.Step{}
	result, err := step.Execute(t.Context(), training.StepContext{Workspace: root, Trainer: trainer{}, Records: records, Fingerprint: "locked", Artifacts: incoming})
	if err != nil {
		t.Fatal(err)
	}
	incoming.Dataset = artifacts.Dataset
	if step.Name() != "split" || result.Status != "" || result.Artifacts != incoming {
		t.Fatalf("stage result = %+v, want artifacts %+v and continuing status", result, incoming)
	}
	if _, err := split.Run(t.Context(), root, trainer{}, training.TrainingJob{}, records, "locked"); err != nil {
		t.Fatal(err)
	}
	retry, err := root.ReadFile("dataset.json")
	if err != nil || !reflect.DeepEqual(data, retry) {
		t.Fatalf("retry changed split: %v", err)
	}
	if _, err := split.Run(t.Context(), root, trainer{}, training.TrainingJob{}, records, "changed"); err == nil {
		t.Fatal("accepted changed identity")
	}
	after, err := root.ReadFile("dataset.json")
	if err != nil || !reflect.DeepEqual(data, after) {
		t.Fatalf("rejected input changed artifact: %v", err)
	}
}

func TestRunRejectsUnreadyDataset(t *testing.T) {
	for _, records := range [][]training.Record{nil, {{Prompt: "Question?", Target: "answer", Evidence: "answer"}}, {{Prompt: "Same?", Target: "a", Evidence: "a"}, {Prompt: "Same?", Target: "b", Evidence: "b"}}} {
		root, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		_, err = split.Run(t.Context(), root, nil, training.TrainingJob{}, records, "invalid")
		var app *temporal.ApplicationError
		if !errors.As(err, &app) || !app.NonRetryable() {
			t.Fatalf("expected nonretryable rejection, got %v", err)
		}
		if len(records) < 2 {
			if app.Type() != "training-records-insufficient" {
				t.Fatalf("insufficient input was not identified: %s", app.Type())
			}
			data, err := root.ReadFile("requirements.json")
			var evidence struct {
				Code     string `json:"code"`
				Count    int    `json:"count"`
				Required int    `json:"required"`
			}
			if err != nil || json.Unmarshal(data, &evidence) != nil || evidence.Count != len(records) || evidence.Required != 20 || evidence.Code != "training-records-insufficient" {
				t.Fatalf("missing record-count evidence: %s (%v)", data, err)
			}
		} else if app.Type() != "invalid-training" {
			t.Fatalf("duplicate prompts must not be diagnosed as a record-count failure: %s", app.Type())
		}
		if _, err := root.Stat("dataset.json"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid split persisted: %v", err)
		}
	}
}

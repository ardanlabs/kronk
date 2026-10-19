package split_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/split"
	"go.temporal.io/sdk/temporal"
)

func TestRunLocksDisjointSplits(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var records []model.Record
	for i := range 60 {
		for j := range 2 {
			records = append(records, model.Record{Prompt: fmt.Sprintf("Question %d variant %d?", i, j), Target: fmt.Sprintf("answer%d", i), Evidence: fmt.Sprintf("Source %d says answer%d", i, i)})
		}
	}
	artifacts, err := split.Run(t.Context(), root, nil, model.TrainingJob{}, records, "locked")
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
	for name, rows := range map[string][]model.Record{"train": state.Dataset.Train, "validation": state.Dataset.Validation, "test": state.Dataset.Test} {
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
	if _, err := split.Run(t.Context(), root, nil, model.TrainingJob{}, records, "locked"); err != nil {
		t.Fatal(err)
	}
	retry, err := root.ReadFile("dataset.json")
	if err != nil || !reflect.DeepEqual(data, retry) {
		t.Fatalf("retry changed split: %v", err)
	}
	if _, err := split.Run(t.Context(), root, nil, model.TrainingJob{}, records, "changed"); err == nil {
		t.Fatal("accepted changed identity")
	}
	after, err := root.ReadFile("dataset.json")
	if err != nil || !reflect.DeepEqual(data, after) {
		t.Fatalf("rejected input changed artifact: %v", err)
	}
}

func TestRunRejectsUnreadyDataset(t *testing.T) {
	for _, records := range [][]model.Record{nil, {{Prompt: "Question?", Target: "answer", Evidence: "answer"}}, {{Prompt: "Same?", Target: "a", Evidence: "a"}, {Prompt: "Same?", Target: "b", Evidence: "b"}}} {
		root, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		_, err = split.Run(t.Context(), root, nil, model.TrainingJob{}, records, "invalid")
		var app *temporal.ApplicationError
		if !errors.As(err, &app) || !app.NonRetryable() {
			t.Fatalf("expected nonretryable rejection, got %v", err)
		}
		if _, err := root.Stat("dataset.json"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid split persisted: %v", err)
		}
	}
}

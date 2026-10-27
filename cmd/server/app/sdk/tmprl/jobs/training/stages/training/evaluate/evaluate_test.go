package evaluate_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training/evaluate"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

type trainer struct {
	t             *testing.T
	base, adapted []string
	records       []training.Record
	err           error
}

func (tr trainer) Validate(context.Context, training.TrainingJob) error {
	tr.t.Fatal("evaluation must not repeat split validation")
	return nil
}

func (tr trainer) Train(context.Context, training.TrainingJob, func(tmprl.Progress)) (training.TrainingCandidate, error) {
	tr.t.Fatal("evaluation must not train")
	return training.TrainingCandidate{}, nil
}
func (tr trainer) Predict(_ context.Context, _ training.TrainingJob, adapter string, records []training.Record) ([]string, error) {
	if !reflect.DeepEqual(records, tr.records) {
		tr.t.Fatalf("prediction received non-test rows: %+v", records)
	}
	if adapter == "" {
		return tr.base, tr.err
	}
	data, err := os.ReadFile(adapter)
	if err != nil || string(data) != "candidate bytes" {
		tr.t.Fatalf("candidate snapshot = %q, %v", data, err)
	}
	return tr.adapted, tr.err
}

func TestRunAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		total, base, adapted int
		accepted             bool
	}{
		{"boundary accepted", 20, 18, 18, true},
		{"below ninety percent", 20, 10, 17, false},
		{"base regression", 20, 19, 18, false},
		{"too few protected rows", 19, 0, 19, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := root.WriteFile("candidate.gguf", []byte("candidate bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			job := training.TrainingJob{Workdir: t.TempDir(), Dataset: training.TrainingDataset{Train: []training.Record{{Prompt: "training only"}}, Validation: []training.Record{{Prompt: "validation only"}}}}
			tr := trainer{t: t}
			for i := range tc.total {
				job.Dataset.Test = append(job.Dataset.Test, training.Record{Prompt: fmt.Sprintf("Question %d?", i), Target: "Correct Answer", Evidence: "Correct Answer"})
				base, adapted := "wrong", "wrong"
				if i < tc.base {
					base = " correct\tanswer "
				}
				if i < tc.adapted {
					adapted = "CORRECT  Answer\n"
				}
				tr.base = append(tr.base, base)
				tr.adapted = append(tr.adapted, adapted)
			}
			tr.records = job.Dataset.Test
			artifacts, err := evaluate.Run(t.Context(), root, tr, job)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(artifacts.Report)
			if err != nil {
				t.Fatal(err)
			}
			var report evaluate.EvaluationReport
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			if report.Total != tc.total || report.BaseCorrect != tc.base || report.AdapterCorrect != tc.adapted || report.Accepted != tc.accepted || artifacts.Accepted != tc.accepted || len(report.Answers) != tc.total {
				t.Fatalf("report = %+v; artifacts = %+v", report, artifacts)
			}
			incoming := training.Artifacts{Dataset: "dataset", Checkpoint: "checkpoint", Report: "old", Adapter: "adapter", Accepted: !tc.accepted}
			var step training.TrainingStep = evaluate.Step{}
			result, err := step.Execute(t.Context(), training.StepContext{Workspace: root, Trainer: tr, Job: job, Artifacts: incoming})
			if err != nil {
				t.Fatal(err)
			}
			incoming.Report, incoming.Accepted = artifacts.Report, tc.accepted
			var status tmprl.Status
			if !tc.accepted {
				status = training.StatusNeedsImprovement
			}
			if step.Name() != "evaluate" || result.Status != status || result.Artifacts != incoming {
				t.Fatalf("stage result = %+v, want artifacts %+v and status %q", result, incoming, status)
			}
			for i, answer := range report.Answers {
				if answer.Prompt != job.Dataset.Test[i].Prompt || answer.Expected != "Correct Answer" || answer.BaseAnswer != tr.base[i] || answer.AdapterAnswer != tr.adapted[i] {
					t.Fatalf("lost raw answer %d: %+v", i, answer)
				}
			}
		})
	}
}

func TestRunRejectsPredictionFailures(t *testing.T) {
	failure := errors.New("prediction failed")
	for _, tc := range []struct {
		name          string
		base, adapted []string
		err           error
	}{
		{name: "base count", base: nil, adapted: []string{"a"}},
		{name: "candidate count", base: []string{"a"}, adapted: nil},
		{name: "provider failure", err: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := root.WriteFile("candidate.gguf", []byte("candidate bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			rows := []training.Record{{Prompt: "Question?", Target: "a", Evidence: "a"}}
			_, err = evaluate.Run(t.Context(), root, trainer{t: t, records: rows, base: tc.base, adapted: tc.adapted, err: tc.err}, training.TrainingJob{Workdir: t.TempDir(), Dataset: training.TrainingDataset{Test: rows}})
			if err == nil || tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("error = %v", err)
			}
			if _, err := root.Stat("evaluation.json"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failure published report: %v", err)
			}
		})
	}
}

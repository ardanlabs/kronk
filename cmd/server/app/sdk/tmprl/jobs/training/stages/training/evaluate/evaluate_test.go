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
func (tr trainer) Predict(_ context.Context, _ training.TrainingJob, adapter string, records []training.Record, answered func(int)) ([]string, error) {
	for i := range records {
		if answered != nil {
			answered(i + 1)
		}
	}
	if len(records) == 0 {
		tr.t.Fatal("a dataset without test questions skips the comparison")
	}
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
		{"seventy percent accepted", 20, 10, 14, true},
		{"below seventy percent", 20, 0, 13, false},
		{"empty answer", 20, 0, 19, false},
		{"no gain over base", 20, 14, 14, false},
		{"five points over base", 20, 13, 14, true},
		{"smallest accepted", training.MinimumTestQuestions, 0, training.MinimumTestQuestions, true},
		{"too few protected rows", training.MinimumTestQuestions - 1, 0, training.MinimumTestQuestions - 1, false},
		{"no test questions", 0, 0, 0, true},
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
				// "incorrect answer" shares one of the target's two words.
				base, adapted := "That is an incorrect answer.", "wrong"
				if i < tc.base {
					base = "The  correct\tanswer."
				}
				if i < tc.adapted {
					adapted = "CORRECT  Answer,"
				}
				tr.base = append(tr.base, base)
				tr.adapted = append(tr.adapted, adapted)
			}
			tr.records = job.Dataset.Test
			if tc.name == "empty answer" {
				tr.adapted[19] = "  "
			}
			var progress []tmprl.Progress
			artifacts, err := evaluate.Run(t.Context(), root, tr, job, func(p tmprl.Progress) { progress = append(progress, p) })
			if err != nil {
				t.Fatal(err)
			}
			// The base model answers every question, then the candidate does.
			if len(progress) != 2*tc.total {
				t.Fatalf("progress reports = %+v, want %d", progress, 2*tc.total)
			}
			for i, p := range progress {
				if p.Completed != i+1 || p.Total != 2*tc.total {
					t.Fatalf("progress report %d = %+v, want %d of %d", i, p, i+1, 2*tc.total)
				}
			}
			data, err := os.ReadFile(artifacts.Report)
			if err != nil {
				t.Fatal(err)
			}
			var report evaluate.EvaluationReport
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			// Only a report without test questions is accepted without comparison.
			if report.Total != tc.total || report.BaseCorrect != tc.base || report.AdapterCorrect != tc.adapted || report.Accepted != tc.accepted || artifacts.Accepted != tc.accepted || artifacts.NotCompared != (tc.total == 0) || len(report.Answers) != tc.total {
				t.Fatalf("report = %+v; artifacts = %+v", report, artifacts)
			}
			if tc.total == 0 && len(report.Checks) != 0 {
				t.Fatal("uncompared report must not claim acceptance checks passed")
			}
			if tc.total != 0 {
				if len(report.Checks) != 4 {
					t.Fatalf("missing acceptance evidence: %+v", report.Checks)
				}
				if tc.total == 20 && (report.Checks[0].Required != 14 || report.Checks[1].Required != 1) {
					t.Fatalf("wrong twenty-question requirements: %+v", report.Checks)
				}
				if tc.total == 16 && report.Checks[0].Required != 12 {
					t.Fatalf("fractional requirement must round up: %+v", report.Checks)
				}
				if tc.name == "empty answer" && (report.Checks[2].Passed || report.Checks[2].Actual != 1 || !report.Checks[0].Passed || !report.Checks[1].Passed) {
					t.Fatalf("empty answer must explain rejection despite good scores: %+v", report.Checks)
				}
			}
			incoming := training.Artifacts{Dataset: "dataset", Checkpoint: "checkpoint", Report: "old", Adapter: "adapter", Accepted: !tc.accepted}
			var step training.TrainingStep = evaluate.Step{}
			result, err := step.Execute(t.Context(), training.StepContext{Workspace: root, Trainer: tr, Job: job, Artifacts: incoming})
			if err != nil {
				t.Fatal(err)
			}
			incoming.Report, incoming.Accepted, incoming.NotCompared = artifacts.Report, tc.accepted, tc.total == 0
			var status tmprl.Status
			if !tc.accepted {
				status = training.StatusNeedsImprovement
			}
			if step.Name() != "evaluate" || result.Status != status || result.Artifacts != incoming {
				t.Fatalf("stage result = %+v, want artifacts %+v and status %q", result, incoming, status)
			}
			for i, answer := range report.Answers {
				if answer.AdapterPassed == nil || *answer.AdapterPassed != (i < tc.adapted) {
					t.Fatalf("wrong persisted pass result for answer %d: %+v", i, answer)
				}
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
			_, err = evaluate.Run(t.Context(), root, trainer{t: t, records: rows, base: tc.base, adapted: tc.adapted, err: tc.err}, training.TrainingJob{Workdir: t.TempDir(), Dataset: training.TrainingDataset{Test: rows}}, nil)
			if err == nil || tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("error = %v", err)
			}
			if _, err := root.Stat("evaluation.json"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failure published report: %v", err)
			}
		})
	}
}

func TestRunScoring(t *testing.T) {
	for _, tc := range []struct {
		name, prompt, expected, answer string
		pass                           bool
	}{
		{"reworded with every fact", "What time does Cedar Library open and close on weekdays?", "Cedar Library opens at 9:00 a.m. on weekdays. It closes at 6:00 p.m. on weekdays.", "Cedar Library opens at 9:00 a.m. and closes at 6:00 p.m. on weekdays.", true},
		{"concise answer", "What is the default benchtime?", "The default -benchtime is 1 second.", "1 second", true},
		{"concise answer without its number", "What is the default benchtime?", "The default -benchtime is 1 second.", "second", false},
		{"different number", "How much does printing cost?", "Black-and-white printing costs 10 cents per page.", "Black-and-white printing costs 35 cents per page.", false},
		{"stop word only", "What is the status?", "The expected answer is Ready.", "the", false},
		{"rambling repetition", "What is the status?", "The status is ready.", "The status is ready ready ready ready ready ready ready ready.", false},
		{"empty", "What is the status?", "The status is ready.", "", false},
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
			rows := []training.Record{{Prompt: tc.prompt, Target: tc.expected, Evidence: tc.expected}}
			artifacts, err := evaluate.Run(t.Context(), root, trainer{t: t, records: rows, base: []string{"wrong"}, adapted: []string{tc.answer}}, training.TrainingJob{Workdir: t.TempDir(), Dataset: training.TrainingDataset{Test: rows}}, nil)
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
			if got := report.AdapterCorrect == 1; got != tc.pass || report.BaseCorrect != 0 {
				t.Fatalf("answer %q passes = %v, want %v (base %d)", tc.answer, got, tc.pass, report.BaseCorrect)
			}
		})
	}
}

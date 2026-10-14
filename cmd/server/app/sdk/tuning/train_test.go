package tuning_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tuning"
)

type controlledTrainer struct {
	base, adapted  int
	validationRows int
	legacy         bool
	retry          bool
	attempts       atomic.Int32
	started        chan struct{}
}

// Make the rejected report exceed the old 64 KiB read limit.
var incorrectAnswer = strings.Repeat("wrong ", 1500)

func (t *controlledTrainer) Train(ctx context.Context, job TrainingJob, publish func(Progress)) (TrainingCandidate, error) {
	validationRows := t.validationRows
	if validationRows == 0 {
		validationRows = 20
	}
	trainRows := 20
	if t.legacy {
		trainRows, validationRows = 90, 30
	}
	if len(job.Dataset.Test) != 0 || len(job.Dataset.Train) != trainRows || len(job.Dataset.Validation) != validationRows {
		return TrainingCandidate{}, fmt.Errorf("wrong or leaked splits")
	}
	for _, record := range job.Dataset.Train {
		if record.ValidationPrompt || record.IndependentPrompt || t.legacy && !record.UserApproved {
			return TrainingCandidate{}, fmt.Errorf("evaluation record used for training")
		}
	}
	for _, record := range job.Dataset.Validation {
		if record.IndependentPrompt || record.UserApproved || !t.legacy && !record.ValidationPrompt {
			return TrainingCandidate{}, fmt.Errorf("protected record used for validation")
		}
	}
	attempt := t.attempts.Add(1)
	if t.started != nil {
		close(t.started)
		<-ctx.Done()
		return TrainingCandidate{}, ctx.Err()
	}
	if t.retry && attempt == 1 {
		return TrainingCandidate{}, errors.New("temporary computation failure")
	}
	publish(Progress{Completed: 1, Total: 1, Message: "Controlled optimizer completed"})
	path := filepath.Join(job.Workdir, "adapter.gguf")
	if err := os.WriteFile(path, []byte("controlled adapter"), 0o600); err != nil {
		return TrainingCandidate{}, err
	}
	return TrainingCandidate{Path: path, BestEpoch: 1, TrainingLoss: 0.7, ValidationLoss: 0.8}, nil
}

func (t *controlledTrainer) Predict(ctx context.Context, job TrainingJob, adapter string, records []Record) ([]string, error) {
	count := 20
	if t.legacy {
		count = 30
	}
	if len(records) != count {
		return nil, fmt.Errorf("wrong protected split")
	}
	correct := t.base
	if adapter != "" {
		data, err := os.ReadFile(adapter)
		if err != nil || string(data) != "controlled adapter" {
			return nil, fmt.Errorf("candidate was not preserved: %v", err)
		}
		correct = t.adapted
	}
	answers := make([]string, len(records))
	for i, r := range records {
		if r.ValidationPrompt || r.UserApproved || !t.legacy && !r.IndependentPrompt {
			return nil, fmt.Errorf("not a protected record")
		}
		answers[i] = incorrectAnswer
		if i < correct {
			answers[i] = "  " + strings.ToUpper(r.Target) + "  "
		}
	}
	return answers, ctx.Err()
}

func trainingFixture(t *testing.T) string {
	t.Helper()
	var source strings.Builder
	for i := range 20 {
		for split := range 3 {
			r := Record{Prompt: fmt.Sprintf("Device %02d question variant %d?", i, split), Target: fmt.Sprintf("Code-%02d", i), Evidence: fmt.Sprintf("Device %02d has Code-%02d.", i, i), FactID: fmt.Sprintf("fact-%02d", i), ValidationPrompt: split == 1, IndependentPrompt: split == 2}
			if err := json.NewEncoder(&source).Encode(r); err != nil {
				t.Fatal(err)
			}
		}
	}
	return source.String()
}

func trainingBusiness(t *testing.T, trainer Trainer) (*Business, string) {
	t.Helper()
	root := t.TempDir()
	model := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(model, []byte("controlled model"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := livePreparation(t, completerFunc(func(context.Context, CompletionRequest) (Completion, error) {
		return Completion{}, errors.New("unexpected preparation computation")
	}), func(cfg *Config) {
		cfg.DatasetRoot = root
		cfg.Models = map[string]string{"gemma": model}
		cfg.Trainer = trainer
	})
	dataset, err := b.ImportDataset(t.Context(), ImportInput{Name: "prepared.jsonl"}, strings.NewReader(trainingFixture(t)))
	if err != nil {
		t.Fatal(err)
	}
	return b, dataset.ID
}

func TestTemporalManagedTraining(t *testing.T) {
	for _, test := range []struct {
		name          string
		base, adapted int
		status        Status
		retry         bool
	}{
		{"accepted-boundary-with-retry", 17, 18, StatusCompleted, true},
		{"below-threshold", 12, 17, StatusNeedsImprovement, false},
		{"base-regression", 19, 18, StatusNeedsImprovement, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			trainer := &controlledTrainer{base: test.base, adapted: test.adapted, retry: test.retry}
			b, id := trainingBusiness(t, trainer)
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			run, err := b.StartTraining(ctx, TrainingRequest{DatasetID: id, Model: "gemma"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := b.Wait(ctx, run.Execution)
			if err != nil || result.Status != test.status {
				t.Fatalf("training: %+v %v", result, err)
			}
			if _, err := b.WaitPreparation(ctx, run.Execution); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("preparation wait accepted training execution: %v", err)
			}
			report, err := b.GetTrainingEvaluation(ctx, run.Execution)
			if err != nil {
				t.Fatal(err)
			}
			if report.BaseCorrect != test.base || report.AdapterCorrect != test.adapted || report.Total != 20 || report.Accepted != (test.status == StatusCompleted) {
				t.Fatalf("report: %+v", report)
			}
			if len(report.Answers) != 20 {
				t.Fatalf("missing answer evidence: %d", len(report.Answers))
			}
			seen := make(map[string]bool)
			baseMatches, adapterMatches := 0, 0
			for _, answer := range report.Answers {
				var device, variant int
				if _, err := fmt.Sscanf(answer.Prompt, "Device %d question variant %d?", &device, &variant); err != nil || device < 0 || device >= 20 || variant != 2 || seen[answer.Prompt] || answer.Expected != fmt.Sprintf("Code-%02d", device) {
					t.Fatalf("incorrect question/target evidence: %+v", answer)
				}
				seen[answer.Prompt] = true
				original := "  " + strings.ToUpper(answer.Expected) + "  "
				if answer.BaseAnswer == original {
					baseMatches++
				} else if answer.BaseAnswer != incorrectAnswer {
					t.Fatal("base answer was changed")
				}
				if answer.AdapterAnswer == original {
					adapterMatches++
				} else if answer.AdapterAnswer != incorrectAnswer {
					t.Fatal("adapter answer was changed")
				}
			}
			if baseMatches != test.base || adapterMatches != test.adapted {
				t.Fatalf("answer evidence disagrees with totals: %d/%d", baseMatches, adapterMatches)
			}
			if test.status == StatusCompleted {
				reader, err := b.OpenTrainingAdapter(ctx, run.Execution)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(reader)
				reader.Close()
				if err != nil || string(data) != "controlled adapter" {
					t.Fatalf("published adapter: %q %v", data, err)
				}
			} else if result.Artifacts.Adapter != "" {
				t.Fatal("rejected adapter published")
			} else if _, err := b.OpenTrainingAdapter(ctx, run.Execution); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("rejected adapter is accessible: %v", err)
			}
			if test.retry && trainer.attempts.Load() != 2 {
				t.Fatalf("retry attempts: %d", trainer.attempts.Load())
			}
			closed, err := b.Get(ctx, run.Execution)
			if err != nil || closed.Status != result.Status || closed.Artifacts != result.Artifacts {
				t.Fatalf("durable result: %+v %v", closed, err)
			}
			for _, invalid := range []TrainingRequest{{Model: "unapproved", DatasetID: id}, {Model: "gemma"}, {Model: "gemma", DatasetID: id, Preparation: run.Execution}, {Model: "gemma", Preparation: run.Execution}} {
				if _, err := b.StartTraining(ctx, invalid); !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("accepted invalid request: %+v %v", invalid, err)
				}
			}
		})
	}
}

func TestTemporalManagedTrainingCancellation(t *testing.T) {
	trainer := &controlledTrainer{started: make(chan struct{})}
	b, id := trainingBusiness(t, trainer)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	run, err := b.StartTraining(ctx, TrainingRequest{DatasetID: id, Model: "gemma"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-trainer.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := b.Stop(ctx, run.Execution); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Wait(ctx, run.Execution); err == nil {
		t.Fatal("canceled training succeeded")
	}
	result, err := b.Get(ctx, run.Execution)
	if err != nil || result.Status != StatusStopped || result.Artifacts.Adapter != "" {
		t.Fatalf("stopped run: %+v %v", result, err)
	}
}

func TestTemporalManagedLegacyTraining(t *testing.T) {
	b, _ := trainingBusiness(t, &controlledTrainer{legacy: true, adapted: 30})
	var source strings.Builder
	for fact := range 30 {
		for variant := range 5 {
			r := Record{Prompt: fmt.Sprintf("Fact %d variant %d?", fact, variant), Target: "yes", Evidence: fmt.Sprintf("Fact %d says yes.", fact), FactID: fmt.Sprintf("fact-%d", fact), UserApproved: variant < 3}
			if err := json.NewEncoder(&source).Encode(r); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	dataset, err := b.ImportDataset(ctx, ImportInput{Name: "legacy.jsonl"}, strings.NewReader(source.String()))
	if err != nil {
		t.Fatal(err)
	}
	run, err := b.StartTraining(ctx, TrainingRequest{DatasetID: dataset.ID, Model: "gemma"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := b.Wait(ctx, run.Execution)
	if err != nil || result.Status != StatusCompleted {
		t.Fatalf("legacy training: %+v %v", result, err)
	}
	report, err := b.GetTrainingEvaluation(ctx, run.Execution)
	if err != nil || !report.Accepted || report.Total != 30 || report.AdapterCorrect != 30 {
		t.Fatalf("legacy evaluation: %+v %v", report, err)
	}
}

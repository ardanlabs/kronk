package training_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	job "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/imports"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/temporal"
)

type trainer struct {
	mode     string
	attempts atomic.Int32
}

func (t *trainer) Train(ctx context.Context, input optimization.TrainingJob, publish func(tmprl.Progress)) (optimization.TrainingCandidate, error) {
	if len(input.Dataset.Train) != 20 || len(input.Dataset.Validation) < 10 || len(input.Dataset.Test) != 0 {
		return optimization.TrainingCandidate{}, errors.New("incorrect optimization splits or protected test leakage")
	}
	if t.attempts.Add(1) == 1 && t.mode == "retry" {
		return optimization.TrainingCandidate{}, errors.New("temporary optimizer failure")
	}
	if t.mode == "fail" {
		return optimization.TrainingCandidate{}, temporal.NewNonRetryableApplicationError("invalid optimizer output", "invalid-step-output", nil)
	}
	path := filepath.Join(input.Workdir, "adapter.gguf")
	if err := os.WriteFile(path, []byte("controlled adapter"), 0o600); err != nil {
		return optimization.TrainingCandidate{}, err
	}
	publish(tmprl.Progress{Completed: 1, Total: 1})
	return optimization.TrainingCandidate{Path: path, BestEpoch: 1, TrainingLoss: 0.7, ValidationLoss: 0.8}, ctx.Err()
}

func (t *trainer) Predict(ctx context.Context, _ optimization.TrainingJob, adapter string, records []optimization.Record) ([]string, error) {
	if len(records) != 20 {
		return nil, errors.New("incorrect protected evaluation split")
	}
	correct := 16
	if adapter != "" {
		data, err := os.ReadFile(adapter)
		if err != nil || string(data) != "controlled adapter" {
			return nil, fmt.Errorf("candidate content changed: %v", err)
		}
		correct = 18
		if t.mode == "reject" {
			correct = 17
		}
	}
	answers := make([]string, len(records))
	for i, record := range records {
		if !record.IndependentPrompt || record.ValidationPrompt {
			return nil, errors.New("evaluation used unprotected records")
		}
		answers[i] = "wrong"
		if i < correct {
			answers[i] = record.Target
		}
	}
	return answers, ctx.Err()
}

type completer struct{}

func (completer) Complete(_ context.Context, r preparation.CompletionRequest) (preparation.Completion, error) {
	prompt := r.Messages[1].Content
	var output any
	switch {
	case strings.Contains(prompt, "[RECORD 1]"):
		outcomes := make(map[string]string)
		for i := range strings.Count(prompt, "[RECORD ") {
			outcomes[fmt.Sprintf("record_%d", i+1)] = "pass"
		}
		output = outcomes
	case strings.Contains(prompt, "Locked target:"):
		_, target, _ := strings.Cut(prompt, "Locked target: ")
		var index int
		if _, err := fmt.Sscanf(target, "Device %d", &index); err != nil {
			return preparation.Completion{}, err
		}
		question := fmt.Sprintf("Which calibration code belongs to device %02d?", index)
		if strings.Contains(prompt, "Do not repeat: "+question) {
			question = fmt.Sprintf("For device %02d, what is its calibration code?", index)
		}
		output = map[string]string{"record_1": question}
	default:
		_, source, _ := strings.Cut(prompt, "SOURCE:\n")
		var rows []map[string]any
		for i, section := range strings.Split(source, "[PASSAGE ")[1:] {
			_, passage, _ := strings.Cut(section, "]\n")
			var index int
			if _, err := fmt.Sscanf(passage, "Device %d", &index); err != nil {
				return preparation.Completion{}, err
			}
			rows = append(rows, map[string]any{"prompt": fmt.Sprintf("What calibration code applies to device %02d?", index), "evidence_id": i + 1, "target_id": 1})
		}
		output = map[string]any{"records": rows}
	}
	data, err := json.Marshal(output)
	return preparation.Completion{Content: string(data)}, err
}

func source(t *testing.T, document bool) (string, string) {
	t.Helper()
	var data strings.Builder
	for i := range 20 {
		if document {
			fmt.Fprintf(&data, "Device %02d has calibration code C%02d.\n\n", i, i)
			continue
		}
		for variant := range 3 {
			r := optimization.Record{Prompt: fmt.Sprintf("Device %02d question variant %d?", i, variant), Target: fmt.Sprintf("Code-%02d", i), Evidence: fmt.Sprintf("Device %02d has Code-%02d.", i, i), FactID: fmt.Sprintf("fact-%02d", i), ValidationPrompt: variant == 1, IndependentPrompt: variant == 2}
			if err := json.NewEncoder(&data).Encode(r); err != nil {
				t.Fatal(err)
			}
		}
	}
	if document {
		return "source.md", data.String()
	}
	return "source.jsonl", data.String()
}

// This exercises production submission, runner, and concrete steps on Temporal.
// Only model computation is substituted; no model is loaded or trained.
func TestTrainingJob(t *testing.T) {
	for _, test := range []struct {
		name     string
		document bool
		mode     string
		status   tmprl.Status
	}{
		{name: "document-to-published-adapter", document: true, status: tmprl.StatusCompleted},
		{name: "prepared-input-with-retry", mode: "retry", status: tmprl.StatusCompleted},
		{name: "evaluation-rejected", mode: "reject", status: optimization.StatusNeedsImprovement},
		{name: "permanent-training-failure", mode: "fail", status: tmprl.StatusFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			storage, err := imports.New(imports.Config{DatasetRoot: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			name, data := source(t, test.document)
			dataset, err := storage.ImportDataset(ctx, imports.ImportInput{Name: name}, strings.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			model := filepath.Join(t.TempDir(), "model.gguf")
			if err := os.WriteFile(model, []byte("controlled model"), 0o600); err != nil {
				t.Fatal(err)
			}
			compute := &trainer{mode: test.mode}
			business, err := job.New(env.Client, job.Config{TaskQueue: env.TaskQueue, Imports: storage, Models: map[string]string{"gemma": model}, Trainer: compute, Completer: completer{}, Log: env.Log})
			if err != nil {
				t.Fatal(err)
			}
			request := job.Request{Model: "gemma", DatasetID: dataset.ID}
			if test.document {
				request.DatasetID = ""
				request.Prepare = &preparation.PreparationRequest{DatasetID: dataset.ID, Model: "generator", Goal: preparation.GoalQuestionAnswer}
			}
			run, err := business.Start(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			wantTraining := optimization.Training{MaximumEpochs: 3, BatchSize: 2, LearningRate: 0.0002, Seed: 3407, LoRARank: 8, LoRAAlpha: 8}
			if run.Request.Training != wantTraining {
				t.Fatalf("default training parameters: got %+v, want %+v", run.Request.Training, wantTraining)
			}
			// Mutating the returned snapshot cannot change durable execution input.
			run.Definition.Stages[0].Name = "mutated"
			worker := business.Jobs().NewWorker()
			if err := worker.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(worker.Stop)
			result, err := business.Jobs().Wait(ctx, run.Execution)
			if (err != nil) != (test.mode == "fail") {
				t.Fatalf("Wait: got %v, want failure=%v", err, test.mode == "fail")
			}
			if err != nil {
				result, err = business.Jobs().Get(ctx, run.Execution)
				if err != nil {
					t.Fatal(err)
				}
			}
			if result.Status != test.status || result.Artifacts.Training.Dataset == "" || result.SessionAttempt != 1 || result.Definition.Stages[0].Name == "mutated" {
				t.Fatalf("result: got %+v, want status=%s with locked dataset and one reservation", result, test.status)
			}
			wantAttempts := int32(1)
			if test.mode == "retry" {
				wantAttempts = 2
			}
			if got := compute.attempts.Load(); got != wantAttempts {
				t.Fatalf("training attempts: got %d, want %d", got, wantAttempts)
			}
			if test.mode != "fail" {
				report, err := business.Evaluation(ctx, run.Execution)
				if err != nil || report.Total != 20 || report.Accepted != (test.status == tmprl.StatusCompleted) {
					t.Fatalf("evaluation: got %+v, %v", report, err)
				}
			}
			adapter, err := business.OpenAdapter(ctx, run.Execution)
			if test.status == tmprl.StatusCompleted {
				if err != nil {
					t.Fatal(err)
				}
				data, readErr := io.ReadAll(adapter)
				if err := errors.Join(readErr, adapter.Close()); err != nil || string(data) != "controlled adapter" {
					t.Fatalf("adapter: got %q, %v", data, err)
				}
			} else if !errors.Is(err, tmprl.ErrInvalidArgument) {
				t.Fatalf("unpublished adapter: got %v, want invalid argument", err)
			}
			wantSteps := []string{"training/split", "training/train"}
			if test.mode != "fail" {
				wantSteps = append(wantSteps, "training/evaluate")
			}
			if test.status == tmprl.StatusCompleted {
				wantSteps = append(wantSteps, "training/publish")
			}
			if test.document {
				wantSteps = append([]string{"preparation/extract", "preparation/generate", "preparation/verify"}, wantSteps...)
				if result.Artifacts.Preparation == nil || result.Artifacts.Preparation.RecordCount != 50 {
					t.Fatalf("preparation: got %+v, want 50 records", result.Artifacts.Preparation)
				}
			}
			assertSteps(t, env, run.Execution, wantSteps)
		})
	}
}

func assertSteps(t *testing.T, env *Environment, execution tmprl.Execution, want []string) {
	t.Helper()
	it := env.Client.GetWorkflowHistory(t.Context(), execution.WorkflowID, execution.RunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	var got []string
	queue := ""
	for it.HasNext() {
		event, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		scheduled := event.GetActivityTaskScheduledEventAttributes()
		if scheduled.GetActivityType().GetName() != "moxie.training.Step" {
			continue
		}
		var request tmprl.StepRequest[job.Request, job.Artifacts]
		if err := converter.GetDefaultDataConverter().FromPayloads(scheduled.GetInput(), &request); err != nil {
			t.Fatal(err)
		}
		got = append(got, request.Stage+"/"+request.Step)
		if queue != "" && scheduled.GetTaskQueue().GetName() != queue {
			t.Fatal("steps were not assigned to the same worker session")
		}
		queue = scheduled.GetTaskQueue().GetName()
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("steps: got %v, want %v", got, want)
	}
}

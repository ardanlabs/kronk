package training_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	job "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	"github.com/stretchr/testify/mock"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/mocks"
	"go.temporal.io/sdk/temporal"
)

type trainer struct {
	mode     string
	attempts atomic.Int32
}

func (t *trainer) Validate(ctx context.Context, _ optimization.TrainingJob) error {
	return ctx.Err()
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

func (completer) Complete(_ context.Context, r providers.CompletionRequest) (providers.Completion, error) {
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
			return providers.Completion{}, err
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
				return providers.Completion{}, err
			}
			rows = append(rows, map[string]any{"prompt": fmt.Sprintf("What calibration code applies to device %02d?", index), "evidence_id": i + 1, "target_id": 1})
		}
		output = map[string]any{"records": rows}
	}
	data, err := json.Marshal(output)
	return providers.Completion{Content: string(data)}, err
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
			name, data := source(t, test.document)
			// Generated datasets can exceed the upload limit and live in a separate root.
			storage, err := uploads.New(uploads.Config{Root: t.TempDir(), MaximumBytes: int64(len(data))})
			if err != nil {
				t.Fatal(err)
			}
			dataset, err := storage.Put(ctx, uploads.Input{Name: name}, strings.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			model := filepath.Join(t.TempDir(), "model.gguf")
			if err := os.WriteFile(model, []byte("controlled model"), 0o600); err != nil {
				t.Fatal(err)
			}
			compute := &trainer{mode: test.mode}
			trainingJob, err := job.New(env.Client, job.Config{TaskQueue: env.TaskQueue, Uploads: storage, StorageRoot: t.TempDir(), Models: map[string]string{"gemma": model}, Trainer: compute, Completer: completer{}, Log: env.Log})
			if err != nil {
				t.Fatal(err)
			}
			request := job.Request{Model: "gemma", DatasetID: dataset.ID}
			if test.document {
				request.DatasetID = ""
				request.Prepare = &preparation.PreparationRequest{DatasetID: dataset.ID, Model: "generator", Goal: preparation.GoalQuestionAnswer}
			}
			run, err := trainingJob.Start(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			wantTraining := optimization.Training{MaximumEpochs: 3, BatchSize: 2, LearningRate: 0.0002, Seed: 3407, LoRARank: 8, LoRAAlpha: 8}
			if run.Request.Training != wantTraining {
				t.Fatalf("default training parameters: got %+v, want %+v", run.Request.Training, wantTraining)
			}
			if _, err := trainingJob.Evaluation(ctx, run.Execution); !errors.Is(err, tmprl.ErrInvalidArgument) {
				t.Fatalf("queued evaluation: got %v, want artifact unavailable", err)
			}
			if adapter, err := trainingJob.OpenAdapter(ctx, run.Execution); adapter != nil || !errors.Is(err, tmprl.ErrInvalidArgument) {
				t.Fatalf("queued adapter: got %v, %v, want artifact unavailable", adapter, err)
			}
			request.ID = run.WorkflowID
			if _, err := trainingJob.Start(ctx, request); err == nil {
				t.Fatal("duplicate submission: got success, want already started")
			} else if _, ok := errors.AsType[*serviceerror.WorkflowExecutionAlreadyStarted](err); !ok {
				t.Fatalf("duplicate submission: got %v, want already started", err)
			}
			// Mutating the returned snapshot cannot change durable execution input.
			run.Definition.Stages[0].Name = "mutated"
			worker := trainingJob.Jobs().NewWorker()
			if err := worker.Start(); err != nil {
				t.Fatal(err)
			}
			stopWorker := sync.OnceFunc(worker.Stop)
			t.Cleanup(stopWorker)
			result, err := trainingJob.Jobs().Wait(ctx, run.Execution)
			if (err != nil) != (test.mode == "fail") {
				t.Fatalf("Wait: got %v, want failure=%v", err, test.mode == "fail")
			}
			if err != nil {
				result, err = trainingJob.Jobs().Get(ctx, run.Execution)
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
			// Endpoint reads must still work when no worker is available.
			stopWorker()
			polled, err := trainingJob.Jobs().Get(ctx, run.Execution)
			if err != nil || polled.Status != test.status || !reflect.DeepEqual(polled.Artifacts, result.Artifacts) {
				t.Fatalf("workerless polling: got %+v, %v, want terminal result artifacts", polled, err)
			}
			if test.mode != "fail" {
				report, err := trainingJob.Evaluation(ctx, run.Execution)
				if err != nil || report.Total != 20 || report.Accepted != (test.status == tmprl.StatusCompleted) {
					t.Fatalf("evaluation: got %+v, %v", report, err)
				}
			}
			adapter, err := trainingJob.OpenAdapter(ctx, run.Execution)
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
			// Visibility is eventually consistent. Follow every opaque page token,
			// including empty filtered pages, rather than assuming an empty page ends it.
			ticker := time.NewTicker(50 * time.Millisecond)
			defer ticker.Stop()
			found := false
			for !found {
				var next []byte
				for {
					page, token, err := trainingJob.Jobs().List(ctx, tmprl.ListOptions{PageSize: 100, NextPageToken: next, Statuses: []tmprl.Status{test.status}})
					if err != nil {
						t.Fatal(err)
					}
					for _, listed := range page {
						if listed.Status != test.status {
							t.Fatalf("filtered list: got %s, want %s", listed.Status, test.status)
						}
						if listed.Execution == run.Execution {
							found = true
							if listed.ClosedAt.IsZero() || !reflect.DeepEqual(listed.Artifacts, result.Artifacts) {
								t.Fatalf("listed run: got %+v, want closed run with result artifacts", listed)
							}
						}
					}
					if len(token) == 0 || found {
						break
					}
					next = token
				}
				if !found {
					select {
					case <-ctx.Done():
						t.Fatalf("listing completed run: %v", ctx.Err())
					case <-ticker.C:
					}
				}
			}
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

func TestRejectInvalidRequests(t *testing.T) {
	storage, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	// No client expectations: invalid submissions must not contact Temporal.
	trainingJob, err := job.New(mocks.NewClient(t), job.Config{Uploads: storage, StorageRoot: t.TempDir(), Models: map[string]string{"gemma": "model.gguf"}, Trainer: &trainer{}})
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
			if _, err := trainingJob.Start(t.Context(), test.input); !errors.Is(err, tmprl.ErrInvalidArgument) {
				t.Fatalf("Start: got %v, want invalid argument", err)
			}
		})
	}
}

func TestPreparedJSONLDataset(t *testing.T) {
	var source strings.Builder
	for i := range 60 {
		row := optimization.Record{Prompt: fmt.Sprintf("Question %03d?", i), Target: fmt.Sprintf("Answer %03d.", i), Evidence: fmt.Sprintf("Source passage %03d supports this answer and preserves its provenance.", i)}
		if err := json.NewEncoder(&source).Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name, data string
		accepted   bool
	}{
		{name: "prepared.jsonl", data: source.String(), accepted: true},
		{name: "unsupported.txt", data: source.String()},
		{name: "invalid.jsonl", data: "not JSON"},
		{name: "missing.jsonl", data: `{"prompt":"Question?","target":"Answer."}` + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertPreparedDataset(t, test.name, test.data, test.accepted)
		})
	}
}

func TestDatasetReadinessBoundaries(t *testing.T) {
	for _, test := range []struct {
		name                                         string
		facts, variants                              int
		approvedVariants                             int
		sharedEvidence, approved, factMode, accepted bool
	}{
		{name: "distinct-evidence", facts: 50, variants: 1, accepted: true},
		{name: "normalized-shared-evidence", facts: 50, variants: 1, sharedEvidence: true},
		{name: "all-approved-training-only", facts: 50, variants: 1, approved: true},
		{name: "fact-boundary", facts: 30, variants: 5, factMode: true, accepted: true},
		{name: "fact-too-few-facts", facts: 29, variants: 5, factMode: true},
		{name: "fact-too-few-variants", facts: 30, variants: 4, factMode: true},
		{name: "fact-all-approved", facts: 30, variants: 5, factMode: true, approved: true},
		{name: "fact-mixed-approved", facts: 30, variants: 5, factMode: true, approvedVariants: 3, accepted: true},
		{name: "fact-insufficient-unapproved", facts: 30, variants: 5, factMode: true, approvedVariants: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			var source strings.Builder
			for fact := range test.facts {
				for variant := range test.variants {
					r := optimization.Record{Prompt: fmt.Sprintf("Question %d variant %d?", fact, variant), Target: "yes", Evidence: fmt.Sprintf("Evidence %d says yes.", fact), UserApproved: test.approved || variant < test.approvedVariants}
					if test.sharedEvidence {
						r.Evidence = "Shared evidence says yes."
						if fact%2 == 0 {
							r.Evidence = " SHARED  evidence\n says yes. "
						}
					}
					if test.factMode {
						r.FactID = fmt.Sprintf("fact-%d", fact)
					}
					if err := json.NewEncoder(&source).Encode(r); err != nil {
						t.Fatal(err)
					}
				}
			}
			assertPreparedDataset(t, "records.jsonl", source.String(), test.accepted)
		})
	}
}

func TestDatasetFactContractValidation(t *testing.T) {
	var records []optimization.Record
	for i := range 20 {
		r := optimization.Record{Prompt: fmt.Sprintf("Training question %02d?", i), Target: fmt.Sprintf("Answer %02d.", i), Evidence: fmt.Sprintf("Evidence for fact %02d.", i), FactID: fmt.Sprintf("fact-%02d", i)}
		records = append(records, r)
	}
	for i := range 10 {
		r := records[i]
		r.Prompt = fmt.Sprintf("Validation question %02d?", i)
		r.ValidationPrompt = true
		records = append(records, r)
	}
	for i := range 20 {
		r := records[i]
		r.Prompt = fmt.Sprintf("Independent question %02d?", i)
		r.IndependentPrompt = true
		records = append(records, r)
	}
	encode := func(rows []optimization.Record) string {
		var data strings.Builder
		for _, r := range rows {
			if err := json.NewEncoder(&data).Encode(r); err != nil {
				t.Fatal(err)
			}
		}
		return data.String()
	}
	valid := encode(records)
	t.Run("valid", func(t *testing.T) {
		assertPreparedDataset(t, "facts.jsonl", valid, true)
	})
	allowed := slices.Clone(records)
	allowed[20].Evidence = "  " + strings.ToUpper(allowed[0].Evidence) + "\n"
	allowed[20].Target = "  " + strings.ToUpper(allowed[0].Target) + "\n"
	for i := range allowed {
		if allowed[i].FactID == "fact-01" {
			allowed[i].Evidence = allowed[0].Evidence // Same evidence, different target is different knowledge.
		}
	}
	t.Run("normalized-facts", func(t *testing.T) {
		assertPreparedDataset(t, "normalized-facts.jsonl", encode(allowed), true)
	})
	for name, change := range map[string]func([]optimization.Record) []optimization.Record{
		"duplicate-prompt": func(rows []optimization.Record) []optimization.Record { rows[20].Prompt = rows[0].Prompt; return rows },
		"conflicting-fact": func(rows []optimization.Record) []optimization.Record {
			rows[20].Target = "Different answer."
			return rows
		},
		"duplicate-knowledge": func(rows []optimization.Record) []optimization.Record {
			for i := range rows {
				if rows[i].FactID == "fact-01" {
					rows[i].Evidence = "  " + strings.ToUpper(rows[0].Evidence) + "\n"
					rows[i].Target = "  " + strings.ToUpper(rows[0].Target) + "\n"
				}
			}
			return rows
		},
		"approved-evaluation": func(rows []optimization.Record) []optimization.Record { rows[20].UserApproved = true; return rows },
		"mixed-fact-mode":     func(rows []optimization.Record) []optimization.Record { rows[0].FactID = ""; return rows },
		"missing-validation":  func(rows []optimization.Record) []optimization.Record { return append(rows[:20], rows[21:]...) },
		"missing-test":        func(rows []optimization.Record) []optimization.Record { return rows[:49] },
	} {
		t.Run(name, func(t *testing.T) {
			assertPreparedDataset(t, name+".jsonl", encode(change(slices.Clone(records))), false)
		})
	}
	first, rest, _ := strings.Cut(valid, "\n")
	for name, invalid := range map[string]string{
		"blank-line":     valid + "\n",
		"trailing-value": first + " {}\n" + rest,
		"unknown-field":  strings.Replace(valid, `"prompt":`, `"unknown":true,"prompt":`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			assertPreparedDataset(t, "invalid.jsonl", invalid, false)
		})
	}
}

// assertPreparedDataset tests training policy after a content-agnostic upload.
// Accepted datasets reach submission, but never execute a workflow or trainer.
func assertPreparedDataset(t *testing.T, name, data string, accepted bool) {
	t.Helper()
	store, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.Put(t.Context(), uploads.Input{Name: name}, strings.NewReader(data))
	if err != nil {
		t.Fatalf("Put: got %v, want successful upload", err)
	}
	c := mocks.NewClient(t)
	compute := &trainer{}
	trainingJob, err := job.New(c, job.Config{Uploads: store, StorageRoot: t.TempDir(), Models: map[string]string{"gemma": "model.gguf"}, Trainer: compute})
	if err != nil {
		t.Fatal(err)
	}
	wantErr := tmprl.ErrInvalidArgument
	if accepted {
		wantErr = errors.New("submission reached Temporal")
		c.On("ExecuteWorkflow", mock.Anything, mock.Anything, "moxie.training.Job", mock.Anything).Return(nil, wantErr).Once()
	}
	_, err = trainingJob.Start(t.Context(), job.Request{Model: "gemma", DatasetID: file.ID})
	if !errors.Is(err, wantErr) {
		t.Errorf("Start: got %v, want %v", err, wantErr)
	}
	if !accepted {
		c.AssertNotCalled(t, "ExecuteWorkflow", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	}
	if got := compute.attempts.Load(); got != 0 {
		t.Errorf("training attempts: got %d, want 0", got)
	}
}

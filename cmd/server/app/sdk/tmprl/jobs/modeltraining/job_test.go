package modeltraining_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/definition"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/imports"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/temporal"
)

type jobHandler func(context.Context, StepContext) (tmprl.StepResult[Artifacts], error)

func (f jobHandler) Execute(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
	return f(ctx, input)
}

func jobModel(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(path, []byte("controlled model"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Real Temporal, actual preparation/training step implementations, and only the
// model computation substituted. No preparation worker or second workflow exists.
func TestTemporalYAMLDocumentJob(t *testing.T) {
	env := newTestEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	completer := completerFunc(func(ctx context.Context, r preparation.CompletionRequest) (preparation.Completion, error) {
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
	})
	s := newServices(t, env.Client, serviceConfig{TaskQueue: env.TaskQueue, Models: map[string]string{"gemma": jobModel(t)}, Trainer: &controlledTrainer{base: 16, adapted: 18, validationRows: 10}, Completer: completer})
	var document strings.Builder
	for i := range 20 {
		fmt.Fprintf(&document, "Device %02d has calibration code C%02d.\n\n", i, i)
	}
	source, err := s.Datasets.ImportDataset(ctx, imports.ImportInput{Name: "devices.md"}, strings.NewReader(document.String()))
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.Training.StartJob(ctx, JobRequest{Model: "gemma", Prepare: &preparation.PreparationRequest{DatasetID: source.ID, Model: "generator", Goal: preparation.GoalQuestionAnswer}})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := s.Training.Get(ctx, run.Execution)
	if err != nil || queued.Definition == nil || len(queued.Stages) != 2 || len(queued.Stages[0].Steps) != 3 || len(queued.Stages[1].Steps) != 4 {
		t.Fatalf("job plan: %+v, %v", queued, err)
	}
	w, err := s.Training.NewWorker()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	result, err := s.Training.Wait(ctx, run.Execution)
	if err != nil || result.Status != tmprl.StatusCompleted || !result.Artifacts.Accepted || result.Artifacts.Preparation == nil || result.Artifacts.Preparation.RecordCount != 50 {
		t.Fatalf("job result: %+v, %v", result, err)
	}
	if data, err := os.ReadFile(result.Artifacts.Adapter); err != nil || string(data) != "controlled adapter" {
		t.Fatalf("adapter: %q, %v", data, err)
	}
	for _, stage := range result.Stages {
		if stage.Status != tmprl.StatusCompleted || stage.StartedAt.IsZero() || stage.ClosedAt.IsZero() || stage.Deadline.IsZero() {
			t.Fatalf("stage: %+v", stage)
		}
		for _, step := range stage.Steps {
			if step.Status != tmprl.StatusCompleted || step.Attempt != 1 {
				t.Fatalf("step: %+v", step)
			}
		}
	}
	it := env.Client.GetWorkflowHistory(ctx, run.WorkflowID, run.RunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	var steps []string
	queue := ""
	for it.HasNext() {
		event, err := it.Next()
		if err != nil {
			t.Fatal(err)
		}
		scheduled := event.GetActivityTaskScheduledEventAttributes()
		if scheduled.GetActivityType().GetName() != JobStepActivityName {
			continue
		}
		var request tmprl.StepRequest[Request, Artifacts]
		if err := converter.GetDefaultDataConverter().FromPayloads(scheduled.GetInput(), &request); err != nil {
			t.Fatal(err)
		}
		steps = append(steps, request.Stage+"/"+request.Step)
		if queue == "" {
			queue = scheduled.GetTaskQueue().GetName()
		}
		if scheduled.GetTaskQueue().GetName() != queue {
			t.Fatal("job steps were not routed to one worker session")
		}
	}
	want := []string{"preparation/extract", "preparation/generate", "preparation/verify", "training/split", "training/train", "training/evaluate", "training/publish"}
	if !reflect.DeepEqual(steps, want) || result.SessionAttempt != 1 {
		t.Fatalf("steps: got %v, want %v; sessions=%d", steps, want, result.SessionAttempt)
	}
}

func TestTemporalJobStepPolicies(t *testing.T) {
	for _, mode := range []string{"retry-timeout", "exhaust-timeout", "stage-deadline", "pause", "paused-stage-deadline", "reservation-deadline", "recover"} {
		t.Run(mode, func(t *testing.T) {
			env := newTestEnvironment(t)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			job, err := definition.Load()
			if err != nil {
				t.Fatal(err)
			}
			job.ReservationTimeout, job.QueueTimeout = time.Minute, time.Minute
			job.Stages = job.Stages[1:]
			job.Stages[0].Timeout = time.Minute
			job.Stages[0].Steps = job.Stages[0].Steps[:2]
			job.Stages[0].Steps[0].Name, job.Stages[0].Steps[1].Name = "capture", "finish"
			first := &job.Stages[0].Steps[0]
			first.Timeout, first.HeartbeatTimeout = 2*time.Second, time.Second
			first.Retry.MaximumAttempts = 2
			if mode == "exhaust-timeout" {
				first.Retry.MaximumAttempts = 1
			}
			if mode == "stage-deadline" {
				first.Timeout = 10 * time.Second
				job.Stages[0].Timeout = 2 * time.Second
			}
			pausing := mode == "pause" || mode == "paused-stage-deadline" || mode == "reservation-deadline" || mode == "recover"
			if pausing {
				first.Timeout = 20 * time.Second
			}
			if mode == "paused-stage-deadline" {
				job.Stages[0].Timeout = 2 * time.Second
			}
			if mode == "reservation-deadline" {
				job.ReservationTimeout = 2 * time.Second
			}
			stepTimeout := first.Timeout
			entered, release := make(chan struct{}), make(chan struct{})
			var attempts atomic.Int32
			handlers := map[string]StepHandler{
				"training/capture": jobHandler(func(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
					attempt := attempts.Add(1)
					if attempt == 1 {
						close(entered)
					}
					if pausing {
						select {
						case <-release:
						case <-ctx.Done():
							return tmprl.StepResult[Artifacts]{}, ctx.Err()
						}
					} else if attempt == 1 {
						<-ctx.Done()
						return tmprl.StepResult[Artifacts]{}, ctx.Err()
					}
					if activity.GetInfo(ctx).StartToCloseTimeout != stepTimeout {
						return tmprl.StepResult[Artifacts]{}, errors.New("submitted step timeout changed")
					}
					return tmprl.StepResult[Artifacts]{Artifacts: Artifacts{Checkpoint: "captured"}}, nil
				}),
				"training/finish": jobHandler(func(ctx context.Context, input StepContext) (tmprl.StepResult[Artifacts], error) {
					return tmprl.StepResult[Artifacts]{Artifacts: input.Request.Artifacts}, nil
				}),
			}
			s := newServices(t, env.Client, serviceConfig{TaskQueue: env.TaskQueue, Models: map[string]string{"gemma": jobModel(t)}, Job: &job, Steps: handlers})
			dataset, err := s.Datasets.ImportDataset(ctx, imports.ImportInput{Name: "prepared.jsonl"}, strings.NewReader(trainingFixture(t)))
			if err != nil {
				t.Fatal(err)
			}
			run, err := s.Training.StartJob(ctx, JobRequest{Model: "gemma", DatasetID: dataset.ID})
			if err != nil {
				t.Fatal(err)
			}
			// Neither caller-owned configuration nor the returned plan may mutate
			// the policies captured by this already-submitted execution.
			first.Timeout = 42 * time.Second
			run.Definition.Stages[0].Steps[0].Timeout = 43 * time.Second
			w, err := s.Training.NewWorker()
			if err != nil {
				t.Fatal(err)
			}
			if err := w.Start(); err != nil {
				t.Fatal(err)
			}
			stop := sync.OnceFunc(w.Stop)
			defer stop()
			var originalDeadline time.Time
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if pausing {
				if err := s.Training.Pause(ctx, run.Execution); err != nil {
					t.Fatal(err)
				}
				pollRun(t, s.Training, ctx, run.Execution, tmprl.StatusPauseRequested)
				close(release)
				paused := pollRun(t, s.Training, ctx, run.Execution, tmprl.StatusPaused)
				originalDeadline = paused.Stages[0].Deadline
				if paused.Stages[0].Steps[0].Status != tmprl.StatusCompleted || paused.Stages[0].Steps[1].Status != tmprl.StatusQueued {
					t.Fatalf("paused steps: %+v", paused)
				}
				if err := s.Training.Pause(ctx, run.Execution); err != nil {
					t.Fatal(err)
				}
				if mode == "recover" {
					stop()
					pollCtx, stopPoll := context.WithTimeout(ctx, 2*time.Second)
					lastKnown, err := s.Training.Get(pollCtx, run.Execution)
					stopPoll()
					if err != nil || lastKnown.Stages[0].Steps[0].Status != tmprl.StatusCompleted {
						t.Fatalf("workerless job: %+v, %v", lastKnown, err)
					}
					replacement, err := s.Training.NewWorker()
					if err != nil {
						t.Fatal(err)
					}
					if err := replacement.Start(); err != nil {
						t.Fatal(err)
					}
					defer replacement.Stop()
				}
				if mode == "pause" || mode == "recover" {
					if err := s.Training.Resume(ctx, run.Execution); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, waitErr := s.Training.Wait(ctx, run.Execution)
			if mode == "stage-deadline" || mode == "exhaust-timeout" || mode == "paused-stage-deadline" || mode == "reservation-deadline" {
				if waitErr == nil {
					t.Fatal("timeout succeeded")
				}
				stored, err := s.Training.Get(ctx, run.Execution)
				wantFirst := tmprl.StatusFailed
				if pausing {
					wantFirst = tmprl.StatusCompleted
				}
				if err != nil || stored.Status != tmprl.StatusFailed || stored.Stages[0].Steps[0].Status != wantFirst || stored.Stages[0].Steps[1].Status != tmprl.StatusQueued || attempts.Load() != 1 {
					t.Fatalf("failed job: %+v, %v; attempts %d", stored, err, attempts.Load())
				}
				if mode != "exhaust-timeout" {
					wantType := "stage-expired"
					if mode == "reservation-deadline" {
						wantType = "reservation-expired"
					}
					var failure *temporal.ApplicationError
					if !errors.As(waitErr, &failure) || failure.Type() != wantType {
						t.Fatalf("deadline cause: %v", waitErr)
					}
				}
				return
			}
			wantAttempts := int32(2)
			if pausing {
				wantAttempts = 1
			}
			stored, err := s.Training.Get(ctx, run.Execution)
			if waitErr != nil || err != nil || result.Status != tmprl.StatusCompleted || stored.Stages[0].Steps[0].Attempt != wantAttempts || result.Artifacts.Checkpoint != "captured" || stored.Definition.Stages[0].Steps[0].Timeout != stepTimeout {
				t.Fatalf("completed job: %+v, %v / %+v, %v", result, waitErr, stored, err)
			}
			if mode == "recover" && (result.SessionAttempt != 2 || attempts.Load() != 1 || !result.Stages[0].Deadline.Equal(originalDeadline)) {
				t.Fatalf("recovered job reset committed work or stage budget: %+v", result)
			}
		})
	}
}

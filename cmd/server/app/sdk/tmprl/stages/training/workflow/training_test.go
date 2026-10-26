package training_test

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/prepare/workflow"
	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/provider"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/workflow"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/stretchr/testify/mock"
	commonpb "go.temporal.io/api/common/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/mocks"
)

const (
	workflowName            = training.WorkflowName
	activityName            = training.ActivityName
	stateQuery              = training.StateQuery
	controlSignal           = training.ControlSignal
	preparationWorkflowName = prepare.WorkflowName
)

type workflowInput = training.Input

type backendFunc func(context.Context, StageRequest, func(Progress)) (Artifacts, error)

func (f backendFunc) RunStage(ctx context.Context, r StageRequest, p func(Progress)) (Artifacts, error) {
	return f(ctx, r, p)
}

func request(model string) Request {
	return Request{Sources: []string{"document-a", "document-b"}, Model: model, Output: "output-a", Training: Training{MaximumEpochs: 7, BatchSize: 3, LearningRate: 0.0002, Seed: 19}}
}

func TestInvalidInputs(t *testing.T) {
	b := newServices(t, &mocks.Client{}, serviceConfig{}).Training
	if _, err := b.NewWorker(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("worker accepted a missing backend: %v", err)
	}
	for _, change := range []func(*Request){
		func(r *Request) { r.Model = " " },
		func(r *Request) { r.Output = "" },
		func(r *Request) { r.Sources = []string{"a", " "} },
		func(r *Request) { r.Training.LearningRate = math.NaN() },
		func(r *Request) { r.Training.LearningRate = math.Inf(1) },
		func(r *Request) { r.Training.MaximumEpochs = 101 },
		func(r *Request) { r.Training.BatchSize = 0 },
	} {
		r := request("model-a")
		change(&r)
		if _, err := b.Start(t.Context(), r); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("invalid request: %v", err)
		}
	}
	for _, call := range []func() error{
		func() error { return b.Pause(t.Context(), Execution{}) },
		func() error { return b.Resume(t.Context(), Execution{}) },
		func() error { return b.Stop(t.Context(), Execution{}) },
		func() error { _, err := b.Get(t.Context(), Execution{}); return err },
		func() error { _, err := b.Wait(t.Context(), Execution{}); return err },
		func() error { _, _, err := b.History(t.Context(), 0, nil); return err },
	} {
		if err := call(); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("invalid API input: %v", err)
		}
	}
}

func TestControlWorkflowTypes(t *testing.T) {
	lookupErr := errors.New("workflow lookup failed")
	for _, kind := range []string{workflowName, preparationWorkflowName, "unrelated.Workflow", "lookup-error"} {
		for _, command := range []string{"pause", "resume", "stop", "stop-preparation"} {
			t.Run(kind+"/"+command, func(t *testing.T) {
				c := &mocks.Client{}
				e := Execution{WorkflowID: "workflow", RunID: "run"}
				description := &workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: &workflowpb.WorkflowExecutionInfo{Type: &commonpb.WorkflowType{Name: kind}}}
				var describeErr error
				if kind == "lookup-error" {
					describeErr = lookupErr
				}
				c.On("DescribeWorkflowExecution", mock.Anything, e.WorkflowID, e.RunID).Return(description, describeErr).Maybe()
				c.On("SignalWorkflow", mock.Anything, e.WorkflowID, e.RunID, controlSignal, command).Return(nil).Maybe()
				c.On("CancelWorkflow", mock.Anything, e.WorkflowID, e.RunID).Return(nil).Maybe()
				services := newServices(t, c, serviceConfig{})
				b := services.Training
				var err error
				allowed := kind == workflowName && command != "stop-preparation" || kind == preparationWorkflowName && command == "stop-preparation"
				switch command {
				case "pause":
					err = b.Pause(t.Context(), e)
				case "resume":
					err = b.Resume(t.Context(), e)
				case "stop":
					err = b.Stop(t.Context(), e)
				case "stop-preparation":
					err = services.Preparation.StopPreparation(t.Context(), e)
				}
				switch {
				case describeErr != nil:
					if !errors.Is(err, lookupErr) {
						t.Fatalf("lookup failure was not propagated: %v", err)
					}
				case allowed:
					if err != nil {
						t.Fatalf("valid control rejected: %v", err)
					}
				default:
					if !errors.Is(err, ErrInvalidArgument) {
						t.Fatalf("wrong workflow type accepted: %v", err)
					}
				}
			})
		}
	}
}

func TestTrainerStartupDiagnostics(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); errors.Is(err, os.ErrNotExist) {
		t.Skip("requires /bin/sh")
	}
	executable := filepath.Join(t.TempDir(), "llama-server")
	script := "#!/bin/sh\nprintf '%06000d' 0 >&2\nprintf '\\nGGUF adapter rejected\\n' >&2\nexit 1\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	trainer := provider.NewGoMLX(executable)
	_, err := trainer.Predict(t.Context(), TrainingJob{Model: "model.gguf"}, "adapter.gguf", []Record{{Prompt: "Question?"}})
	if err == nil || !strings.Contains(err.Error(), "GGUF adapter rejected") || len(err.Error()) > 4300 {
		t.Fatalf("missing or unbounded startup diagnostics: %v", err)
	}
}

func TestRealGemmaPredictionContext(t *testing.T) {
	model := os.Getenv("MOXIE_TEST_GEMMA_MODEL")
	if model == "" {
		t.Skip("set MOXIE_TEST_GEMMA_MODEL to test real inference context limits")
	}
	header, err := gguf.ReadHeaderBytes(model)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := gguf.ParseMetadata(header)
	if err != nil {
		t.Fatal(err)
	}
	capacity, err := gguf.ParseInt64(metadata, "gemma3.context_length")
	if err != nil || capacity < 1024 {
		t.Fatalf("invalid Gemma context metadata: %d %v", capacity, err)
	}
	trainer := provider.NewGoMLX(os.Getenv("MOXIE_TEST_LLAMA_SERVER"))
	for _, test := range []struct {
		name    string
		words   int
		tooLong bool
	}{
		{"over-old-limit", 600, false},
		{"no-answer-room", int(capacity) - 128, true},
		{"over-model-limit", int(capacity) + 128, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompt := strings.Repeat("hello ", test.words) + "Reply with only the word OK."
			answers, err := trainer.Predict(t.Context(), TrainingJob{Model: model}, "", []Record{{Prompt: prompt}})
			if test.tooLong {
				if !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("oversized question not rejected as invalid input: %v", err)
				}
			} else if err != nil || len(answers) != 1 || strings.TrimSpace(answers[0]) == "" {
				t.Fatalf("supported long question failed: %q %v", answers, err)
			}
		})
	}
}

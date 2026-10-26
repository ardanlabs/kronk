package training

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/prepare/workflow"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/dataset"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/provider"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

var ErrInvalidArgument = model.ErrInvalidArgument
var ErrNotFound = dataset.ErrNotFound

// Backend performs model-specific computation, never presentation work. It
// must honor ctx and deduplicate side effects using StageRequest.Key because
// Temporal activities can execute more than once. Progress may be concurrent.
// Paths/references are resolved on the worker, not by an HTTP handler. No legacy
// fineapp types are exposed by this SDK.
// This optional seam supports alternative pipelines. StartTraining uses the
// SDK's built-in preparation-to-GoMLX pipeline instead.
type Backend interface {
	RunStage(context.Context, StageRequest, func(Progress)) (Artifacts, error)
}

type Config struct {
	TaskQueue    string
	StageTimeout time.Duration
	// Backend executes the optional alternative pipeline submitted through Start.
	// Workers using a Trainer do not require it.
	Backend     Backend
	Datasets    *dataset.Business
	Preparation *prepare.Business
	Progress    *tmprl.ProgressBroker[Execution, Progress]
	// Models maps approved IDs to local Gemma 3 GGUFs. Trainer defaults to the
	// SDK GoMLX implementation. These host settings never become workflow inputs.
	Models      map[string]string
	Trainer     Trainer
	LlamaServer string
}

type Business struct {
	client   client.Client
	cfg      Config
	progress *tmprl.ProgressBroker[Execution, Progress]
}

// New borrows the Temporal client. The host owns client/worker shutdown.
func New(c client.Client, cfg Config) (*Business, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: client is required", ErrInvalidArgument)
	}
	if cfg.Datasets == nil || cfg.Preparation == nil {
		return nil, fmt.Errorf("%w: dataset and preparation services are required", ErrInvalidArgument)
	}
	if cfg.TaskQueue == "" {
		cfg.TaskQueue = "moxie-tuning"
	}
	if strings.TrimSpace(cfg.TaskQueue) == "" || len(cfg.TaskQueue) > 200 {
		return nil, fmt.Errorf("%w: invalid task queue", ErrInvalidArgument)
	}
	if cfg.StageTimeout == 0 {
		cfg.StageTimeout = 24 * time.Hour
	}
	if cfg.StageTimeout < 5*time.Second {
		return nil, fmt.Errorf("%w: stage timeout must be at least five seconds", ErrInvalidArgument)
	}
	cfg.Models = maps.Clone(cfg.Models)
	for id, path := range cfg.Models {
		if strings.TrimSpace(id) == "" || path == "" {
			return nil, fmt.Errorf("%w: model IDs and paths are required", ErrInvalidArgument)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		cfg.Models[id] = absolute
	}
	if cfg.Trainer == nil && len(cfg.Models) > 0 {
		cfg.Trainer = provider.NewGoMLX(cfg.LlamaServer)
	}
	if cfg.Progress == nil {
		cfg.Progress = new(tmprl.ProgressBroker[Execution, Progress])
	}
	return &Business{client: c, cfg: cfg, progress: cfg.Progress}, nil
}

// NewWorker registers training execution. The host must Start/Run and Stop it.
func (b *Business) NewWorker() (worker.Worker, error) {
	if !b.Enabled() {
		return nil, fmt.Errorf("%w: training backend is required", ErrInvalidArgument)
	}
	w := tmprl.NewWorker(b.client, b.cfg.TaskQueue, b.WorkerOptions(), b.Register)
	return w, nil
}

func (b *Business) Enabled() bool { return b.cfg.Backend != nil || b.cfg.Trainer != nil }

// WorkerOptions limits concurrent training activities when using a trainer.
func (b *Business) WorkerOptions() worker.Options {
	options := worker.Options{WorkerStopTimeout: 5 * time.Second}
	if b.cfg.Trainer != nil {
		options.MaxConcurrentActivityExecutionSize = 1
	}
	return options
}

// Register attaches the training workflow and activity to a host-owned worker.
func (b *Business) Register(w worker.Registry) {
	if b.Enabled() {
		w.RegisterWorkflowWithOptions(Workflow, workflow.RegisterOptions{Name: WorkflowName})
		w.RegisterActivityWithOptions(b.RunStage, activity.RegisterOptions{Name: ActivityName})
	}
}

func (b *Business) publishProgress(update Progress) {
	b.progress.Publish(update.Execution, update)
}

// Subscribe receives local training progress.
func (b *Business) Subscribe(e Execution) (<-chan Progress, func()) {
	return b.progress.Subscribe(e)
}

func (b *Business) Start(ctx context.Context, input Request) (Run, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Model = strings.TrimSpace(input.Model)
	input.Output = strings.TrimSpace(input.Output)
	input.Sources = slices.Clone(input.Sources)
	for i := range input.Sources {
		input.Sources[i] = strings.TrimSpace(input.Sources[i])
	}
	if err := validate(input); err != nil {
		return Run{}, err
	}
	if input.ID == "" {
		input.ID = "tuning-" + rand.Text()
	}
	execution, err := tmprl.Start(ctx, b.client, client.StartWorkflowOptions{
		ID: input.ID, TaskQueue: b.cfg.TaskQueue,
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}, workflowName, workflowInput{Request: input, StageTimeout: b.cfg.StageTimeout})
	if err != nil {
		return Run{}, fmt.Errorf("start tuning: %w", err)
	}
	return Run{Execution: execution, Request: input, Status: StatusRunning}, nil
}

func validate(r Request) error {
	managed := r.DatasetID != "" || r.Preparation != (Execution{})
	switch {
	case len(r.ID) > 200 || r.Model == "" || (!managed && (r.Output == "" || len(r.Sources) == 0)):
		return fmt.Errorf("%w: model, output, sources and an ID of at most 200 bytes are required", ErrInvalidArgument)
	case r.Training.MaximumEpochs < 1 || r.Training.MaximumEpochs > 100 || r.Training.BatchSize < 1 || r.Training.BatchSize > 128 || r.Training.Seed < 0:
		return fmt.Errorf("%w: invalid epoch, batch size or seed bounds", ErrInvalidArgument)
	case r.Training.LearningRate <= 0 || r.Training.LearningRate > 1 || math.IsNaN(r.Training.LearningRate) || math.IsInf(r.Training.LearningRate, 0):
		return fmt.Errorf("%w: learning rate must be finite and in (0,1]", ErrInvalidArgument)
	}
	if slices.Contains(r.Sources, "") {
		return fmt.Errorf("%w: empty source", ErrInvalidArgument)
	}
	return nil
}

// Pause takes effect after the active stage commits. Resume continues that same
// run; it does not restart a stopped run or repeat completed stages.
func (b *Business) Pause(ctx context.Context, e Execution) error  { return b.control(ctx, e, "pause") }
func (b *Business) Resume(ctx context.Context, e Execution) error { return b.control(ctx, e, "resume") }
func (b *Business) control(ctx context.Context, e Execution, command string) error {
	return model.ExecutionError(tmprl.Signal(ctx, b.client, e, controlSignal, command, workflowName))
}

// Stop requests cooperative cancellation of training. Wait/Get establish when
// the execution is closed.
func (b *Business) Stop(ctx context.Context, e Execution) error {
	return model.ExecutionError(tmprl.Stop(ctx, b.client, e, workflowName))
}

func (b *Business) Wait(ctx context.Context, e Execution) (Run, error) {
	var result Run
	err := tmprl.Wait(ctx, b.client, e, &result, workflowName)
	if errors.Is(err, tmprl.ErrUnsupportedWorkflow) {
		return Run{}, fmt.Errorf("%w: not a tuning workflow", ErrInvalidArgument)
	}
	return result, model.ExecutionError(err)
}

// Get reads closed runs from durable history, never from a stale query handler.
// A running run requires a worker to answer its state query.
func (b *Business) Get(ctx context.Context, e Execution) (Run, error) {
	if err := model.ValidateExecution(e); err != nil {
		return Run{}, err
	}
	description, err := b.client.DescribeWorkflowExecution(ctx, e.WorkflowID, e.RunID)
	if err != nil {
		return Run{}, err
	}
	info := description.GetWorkflowExecutionInfo()
	if info.GetType().GetName() != workflowName {
		return Run{}, fmt.Errorf("%w: not a tuning workflow", ErrInvalidArgument)
	}
	if info.GetStatus() == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		var result Run
		if err := tmprl.Query(ctx, b.client, e, stateQuery, &result); err != nil {
			return Run{}, err
		}
		// Heartbeats expose worker progress to polling heads in other processes.
		// Ignore an old stage's heartbeat if the query has already advanced.
		for _, pending := range description.GetPendingActivities() {
			if pending.GetHeartbeatDetails() == nil {
				continue
			}
			var progress Progress
			if err := converter.GetDefaultDataConverter().FromPayloads(pending.GetHeartbeatDetails(), &progress); err != nil {
				return Run{}, err
			}
			if progress.Execution == result.Execution && progress.Stage == result.Stage {
				result.Progress = &progress
			}
		}
		return result, nil
	}
	result := Run{Execution: e, Status: StatusFailed, StartedAt: info.GetStartTime().AsTime(), ClosedAt: info.GetCloseTime().AsTime()}
	it := b.client.GetWorkflowHistory(ctx, e.WorkflowID, e.RunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	for it.HasNext() {
		event, err := it.Next()
		if err != nil {
			return Run{}, err
		}
		switch event.GetEventType() {
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
			var input workflowInput
			if err := converter.GetDefaultDataConverter().FromPayloads(event.GetWorkflowExecutionStartedEventAttributes().GetInput(), &input); err != nil {
				return Run{}, err
			}
			result.Request = input.Request
		case enumspb.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:
			if err := converter.GetDefaultDataConverter().FromPayloads(event.GetActivityTaskCompletedEventAttributes().GetResult(), &result.Artifacts); err != nil {
				return Run{}, err
			}
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED:
			if err := converter.GetDefaultDataConverter().FromPayloads(event.GetWorkflowExecutionCompletedEventAttributes().GetResult(), &result); err != nil {
				return Run{}, err
			}
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED:
			result.Status = StatusStopped
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED:
			failure := event.GetWorkflowExecutionFailedEventAttributes().GetFailure()
			for failure != nil {
				result.Error = failure.GetMessage()
				failure = failure.GetCause()
			}
		}
	}
	return result, nil
}

// History pages through training executions, including active runs.
func (b *Business) History(ctx context.Context, pageSize int, next []byte) ([]Execution, []byte, error) {
	runs, token, err := tmprl.History(ctx, b.client, workflowName, pageSize, next)
	if errors.Is(err, tmprl.ErrInvalidPageSize) {
		return nil, nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return runs, token, err
}

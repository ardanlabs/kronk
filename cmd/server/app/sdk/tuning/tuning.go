package tuning

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

var ErrInvalidArgument = errors.New("invalid tuning input")
var ErrCompletionTruncated = errors.New("preparation completion reached its token limit")

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
	// Workers using a Completer or Trainer do not require it.
	Backend Backend
	// DatasetRoot must be shared by import callers and preparation workers.
	// The default is ~/.moxie/tuning, separate from fineapp's managed datasets.
	DatasetRoot        string
	MaximumImportBytes int64
	Extractor          DocumentExtractor
	Completer          ModelCompleter
	// Models maps approved IDs to local Gemma 3 GGUFs. Trainer defaults to the
	// SDK GoMLX implementation. These host settings never become workflow inputs.
	Models      map[string]string
	Trainer     Trainer
	LlamaServer string
}

type Business struct {
	client   client.Client
	cfg      Config
	progress progressBroker
}

// New borrows the Temporal client. The host owns client/worker shutdown.
func New(c client.Client, cfg Config) (*Business, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: client is required", ErrInvalidArgument)
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
	if cfg.DatasetRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		cfg.DatasetRoot = filepath.Join(home, ".moxie", "tuning")
	}
	root, err := filepath.Abs(cfg.DatasetRoot)
	if err != nil {
		return nil, err
	}
	cfg.DatasetRoot = root
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
		cfg.Trainer = goMLXTrainer{executable: cfg.LlamaServer}
	}
	if cfg.MaximumImportBytes == 0 {
		cfg.MaximumImportBytes = 100 << 20
	}
	if cfg.MaximumImportBytes < 1 || cfg.MaximumImportBytes > 100<<20 {
		return nil, fmt.Errorf("%w: import limit must be 1–100 MiB", ErrInvalidArgument)
	}
	return &Business{client: c, cfg: cfg}, nil
}

// NewWorker registers SDK execution. The host must Start/Run and Stop it.
func (b *Business) NewWorker() (worker.Worker, error) {
	if b.cfg.Backend == nil && b.cfg.Completer == nil && b.cfg.Trainer == nil {
		return nil, fmt.Errorf("%w: training backend or preparation completer is required", ErrInvalidArgument)
	}
	options := worker.Options{WorkerStopTimeout: 5 * time.Second}
	if b.cfg.Trainer != nil {
		options.MaxConcurrentActivityExecutionSize = 1
	}
	w := worker.New(b.client, b.cfg.TaskQueue, options)
	if b.cfg.Backend != nil || b.cfg.Trainer != nil {
		w.RegisterWorkflowWithOptions(runWorkflow, workflow.RegisterOptions{Name: workflowName})
		w.RegisterActivityWithOptions(b.runStage, activity.RegisterOptions{Name: activityName})
	}
	if b.cfg.Completer != nil {
		w.RegisterWorkflowWithOptions(preparationWorkflow, workflow.RegisterOptions{Name: preparationWorkflowName})
		w.RegisterActivityWithOptions(b.prepareDataset, activity.RegisterOptions{Name: preparationActivityName})
	}
	return w, nil
}

// Subscribe receives progress for both tuning stages and preparation jobs.
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
	execution, err := b.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID: input.ID, TaskQueue: b.cfg.TaskQueue,
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}, workflowName, workflowInput{Request: input, StageTimeout: b.cfg.StageTimeout})
	if err != nil {
		return Run{}, fmt.Errorf("start tuning: %w", err)
	}
	return Run{WorkflowID: execution.GetID(), RunID: execution.GetRunID(), Request: input, Status: StatusRunning}, nil
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

func validateExecution(e Execution) error {
	if e.WorkflowID == "" || e.RunID == "" {
		return fmt.Errorf("%w: workflow and run IDs are required", ErrInvalidArgument)
	}
	return nil
}

func (b *Business) validateWorkflow(ctx context.Context, e Execution, kinds ...string) error {
	if err := validateExecution(e); err != nil {
		return err
	}
	description, err := b.client.DescribeWorkflowExecution(ctx, e.WorkflowID, e.RunID)
	if err != nil {
		return err
	}
	if !slices.Contains(kinds, description.GetWorkflowExecutionInfo().GetType().GetName()) {
		return fmt.Errorf("%w: unsupported workflow type for this control", ErrInvalidArgument)
	}
	return nil
}

// Pause takes effect after the active stage commits. Resume continues that same
// run; it does not restart a stopped run or repeat completed stages.
func (b *Business) Pause(ctx context.Context, e Execution) error  { return b.control(ctx, e, "pause") }
func (b *Business) Resume(ctx context.Context, e Execution) error { return b.control(ctx, e, "resume") }
func (b *Business) control(ctx context.Context, e Execution, command string) error {
	if err := b.validateWorkflow(ctx, e, workflowName); err != nil {
		return err
	}
	return b.client.SignalWorkflow(ctx, e.WorkflowID, e.RunID, controlSignal, command)
}

// Stop requests cooperative cancellation of training or preparation. Wait/Get
// (or WaitPreparation/GetPreparation) establish when the execution is closed.
func (b *Business) Stop(ctx context.Context, e Execution) error {
	if err := b.validateWorkflow(ctx, e, workflowName, preparationWorkflowName); err != nil {
		return err
	}
	return b.client.CancelWorkflow(ctx, e.WorkflowID, e.RunID)
}

func (b *Business) Wait(ctx context.Context, e Execution) (Run, error) {
	if err := validateExecution(e); err != nil {
		return Run{}, err
	}
	description, err := b.client.DescribeWorkflowExecution(ctx, e.WorkflowID, e.RunID)
	if err != nil {
		return Run{}, err
	}
	if description.GetWorkflowExecutionInfo().GetType().GetName() != workflowName {
		return Run{}, fmt.Errorf("%w: not a tuning workflow", ErrInvalidArgument)
	}
	var result Run
	err = b.client.GetWorkflow(ctx, e.WorkflowID, e.RunID).Get(ctx, &result)
	return result, err
}

// Get reads closed runs from durable history, never from a stale query handler.
// A running run requires a worker to answer its state query.
func (b *Business) Get(ctx context.Context, e Execution) (Run, error) {
	if err := validateExecution(e); err != nil {
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
		value, err := b.client.QueryWorkflow(ctx, e.WorkflowID, e.RunID, stateQuery)
		if err != nil {
			return Run{}, err
		}
		var result Run
		if err := value.Get(&result); err != nil {
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
	return b.history(ctx, workflowName, pageSize, next)
}

func (b *Business) history(ctx context.Context, kind string, pageSize int, next []byte) ([]Execution, []byte, error) {
	if pageSize < 1 || pageSize > 100 {
		return nil, nil, fmt.Errorf("%w: page size must be 1–100", ErrInvalidArgument)
	}
	response, err := b.client.ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{PageSize: int32(pageSize), NextPageToken: next, Query: "WorkflowType = '" + kind + "'"})
	if err != nil {
		return nil, nil, err
	}
	runs := make([]Execution, 0, len(response.GetExecutions()))
	for _, info := range response.GetExecutions() {
		runs = append(runs, Execution{WorkflowID: info.GetExecution().GetWorkflowId(), RunID: info.GetExecution().GetRunId()})
	}
	return runs, response.GetNextPageToken(), nil
}

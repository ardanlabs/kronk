// Package prepare exposes standalone preparation and preview workflows for the
// model-training job. Computation belongs to its preparation stage.
package prepare

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/imports"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/sdk/applog"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

var ErrInvalidArgument = preparation.ErrInvalidArgument
var ErrCompletionTruncated = preparation.ErrCompletionTruncated
var ErrNotFound = imports.ErrNotFound

type Config struct {
	TaskQueue    string
	StageTimeout time.Duration
	Datasets     *imports.Business
	Extractor    preparation.DocumentExtractor
	Completer    preparation.ModelCompleter
	Progress     *tmprl.ProgressBroker[tmprl.Execution, tmprl.Progress]
	Log          applog.Logger
}

// Business owns preparation requests, activities, and persisted results.
type Business struct {
	client   client.Client
	cfg      Config
	progress *tmprl.ProgressBroker[tmprl.Execution, tmprl.Progress]
}

// New borrows the client and dataset service. The host owns worker shutdown.
func New(c client.Client, cfg Config) (*Business, error) {
	if c == nil || cfg.Datasets == nil {
		return nil, fmt.Errorf("%w: client and dataset service are required", ErrInvalidArgument)
	}
	if cfg.TaskQueue == "" {
		cfg.TaskQueue = "moxie-preparation"
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
	if cfg.Progress == nil {
		cfg.Progress = new(tmprl.ProgressBroker[tmprl.Execution, tmprl.Progress])
	}
	if cfg.Log == nil {
		cfg.Log = applog.FmtLogger
	}
	return &Business{client: c, cfg: cfg, progress: cfg.Progress}, nil
}

// TaskQueue identifies the preparation worker pool. Training must use a
// separate queue so preparation cannot consume a reserved training worker.
func (b *Business) TaskQueue() string { return b.cfg.TaskQueue }

func (b *Business) Enabled() bool { return b.cfg.Completer != nil }

func (b *Business) NewWorker() (worker.Worker, error) {
	if !b.Enabled() {
		return nil, fmt.Errorf("%w: preparation completer is required", ErrInvalidArgument)
	}
	return tmprl.NewWorker(b.client, b.cfg.TaskQueue, worker.Options{WorkerStopTimeout: 5 * time.Second, MaxConcurrentActivityExecutionSize: 1}, b.Register), nil
}

// Register attaches preparation execution to a host-owned worker.
func (b *Business) Register(w worker.Registry) {
	if b.Enabled() {
		w.RegisterWorkflowWithOptions(Workflow, workflow.RegisterOptions{Name: WorkflowName})
		w.RegisterActivityWithOptions(b.prepareDataset, activity.RegisterOptions{Name: ActivityName})
	}
}

func (b *Business) Subscribe(e tmprl.Execution) (<-chan tmprl.Progress, func()) {
	return b.progress.Subscribe(e)
}

func (b *Business) publishProgress(update tmprl.Progress) {
	b.progress.Publish(update.Execution, update)
}

func (b *Business) StartPreview(ctx context.Context, input preparation.PreparationRequest) (preparation.PreparationRun, error) {
	return b.startPreparation(ctx, input, preparation.PreparationPreview)
}
func (b *Business) StartPreparation(ctx context.Context, input preparation.PreparationRequest) (preparation.PreparationRun, error) {
	return b.startPreparation(ctx, input, preparation.PreparationFull)
}

// ValidateRequest normalizes and validates a preparation request and its imported
// dataset without starting a workflow or assigning an ID. The returned request
// owns a copy of ReviewedRecords.
func (b *Business) ValidateRequest(ctx context.Context, input preparation.PreparationRequest, kind preparation.PreparationKind) (preparation.PreparationRequest, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Model = strings.TrimSpace(input.Model)
	input.Instructions = strings.TrimSpace(input.Instructions)
	input.ReviewedRecords = slices.Clone(input.ReviewedRecords)
	if input.Model == "" || len(input.ID) > 200 || utf8.RuneCountInString(input.Instructions) > 2000 {
		return preparation.PreparationRequest{}, fmt.Errorf("%w: invalid preparation model, ID or instructions", ErrInvalidArgument)
	}
	switch input.Goal {
	case preparation.GoalQuestionAnswer, preparation.GoalClassification, preparation.GoalNumericPrediction, preparation.GoalTextGeneration:
	default:
		return preparation.PreparationRequest{}, fmt.Errorf("%w: unsupported preparation goal", ErrInvalidArgument)
	}
	switch kind {
	case preparation.PreparationPreview:
		if input.PreviewRecords < 1 || input.PreviewRecords > 5 || len(input.ReviewedRecords) != 0 {
			return preparation.PreparationRequest{}, fmt.Errorf("%w: preview requires 1–5 samples and no reviewed records", ErrInvalidArgument)
		}
	case preparation.PreparationFull:
		if input.PreviewRecords != 0 || len(input.ReviewedRecords) > 5 {
			return preparation.PreparationRequest{}, fmt.Errorf("%w: full preparation accepts at most five reviewed records, not a preview count", ErrInvalidArgument)
		}
		if err := training.ValidateReviewedRecords(input.ReviewedRecords); err != nil {
			return preparation.PreparationRequest{}, fmt.Errorf("%w: reviewed records: %s", ErrInvalidArgument, err)
		}
	}
	data, err := json.Marshal(input)
	if err != nil {
		return preparation.PreparationRequest{}, err
	}
	if len(data) > 64<<10 {
		return preparation.PreparationRequest{}, fmt.Errorf("%w: preparation input exceeds 64 KiB", ErrInvalidArgument)
	}
	dataset, err := b.cfg.Datasets.DatasetSource(ctx, input.DatasetID)
	if err != nil {
		return preparation.PreparationRequest{}, err
	}
	if dataset.Format == ".jsonl" {
		return preparation.PreparationRequest{}, fmt.Errorf("%w: prepared JSONL does not need document preparation", ErrInvalidArgument)
	}
	return input, nil
}

func (b *Business) startPreparation(ctx context.Context, input preparation.PreparationRequest, kind preparation.PreparationKind) (preparation.PreparationRun, error) {
	input, err := b.ValidateRequest(ctx, input, kind)
	if err != nil {
		return preparation.PreparationRun{}, err
	}
	if input.ID == "" {
		input.ID = "preparation-" + rand.Text()
	}
	run, err := tmprl.Start(ctx, b.client, client.StartWorkflowOptions{ID: input.ID, TaskQueue: b.cfg.TaskQueue, WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowExecutionErrorWhenAlreadyStarted: true}, preparationWorkflowName, preparation.PreparationInput{Request: input, Kind: kind, Timeout: b.cfg.StageTimeout})
	if err != nil {
		return preparation.PreparationRun{}, err
	}
	b.cfg.Log(ctx, "preparation job submitted", "workflow_id", run.WorkflowID, "run_id", run.RunID, "task_queue", b.cfg.TaskQueue, "kind", kind, "dataset_id", input.DatasetID)
	return preparation.PreparationRun{Execution: run, Request: input, Kind: kind, Status: tmprl.StatusRunning}, nil
}

// PreparationHistory pages through preview and full preparation executions,
// including active runs. Use GetPreparation to retrieve each execution's details.
func (b *Business) PreparationHistory(ctx context.Context, pageSize int, next []byte) ([]tmprl.Execution, []byte, error) {
	runs, token, err := tmprl.History(ctx, b.client, preparationWorkflowName, pageSize, next)
	if errors.Is(err, tmprl.ErrInvalidPageSize) {
		return nil, nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return runs, token, err
}

func (b *Business) WaitPreparation(ctx context.Context, e tmprl.Execution) (preparation.PreparationRun, error) {
	var result preparation.PreparationRun
	err := tmprl.Wait(ctx, b.client, e, &result, preparationWorkflowName)
	if errors.Is(err, tmprl.ErrUnsupportedWorkflow) {
		return preparation.PreparationRun{}, fmt.Errorf("%w: not a preparation workflow", ErrInvalidArgument)
	}
	return result, preparation.ExecutionError(err)
}

// GetPreparation never queries a closed execution: its durable result or
// terminal history is authoritative, even if no worker remains running.
func (b *Business) GetPreparation(ctx context.Context, e tmprl.Execution) (preparation.PreparationRun, error) {
	if err := preparation.ValidateExecution(e); err != nil {
		return preparation.PreparationRun{}, err
	}
	description, err := b.client.DescribeWorkflowExecution(ctx, e.WorkflowID, e.RunID)
	if err != nil {
		return preparation.PreparationRun{}, err
	}
	info := description.GetWorkflowExecutionInfo()
	if info.GetType().GetName() != preparationWorkflowName {
		return preparation.PreparationRun{}, fmt.Errorf("%w: not a preparation workflow", ErrInvalidArgument)
	}
	switch info.GetStatus() {
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		var result preparation.PreparationRun
		err := b.client.GetWorkflow(ctx, e.WorkflowID, e.RunID).Get(ctx, &result)
		return result, err
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		var result preparation.PreparationRun
		if err := tmprl.Query(ctx, b.client, e, StateQuery, &result); err != nil {
			return preparation.PreparationRun{}, err
		}
		for _, pending := range description.GetPendingActivities() {
			if pending.GetHeartbeatDetails() == nil {
				continue
			}
			var progress tmprl.Progress
			if err := converter.GetDefaultDataConverter().FromPayloads(pending.GetHeartbeatDetails(), &progress); err != nil {
				return preparation.PreparationRun{}, err
			}
			if progress.Execution == e {
				result.Progress = &progress
			}
		}
		return result, nil
	}
	result := preparation.PreparationRun{Execution: e, Status: tmprl.StatusFailed, StartedAt: info.GetStartTime().AsTime(), ClosedAt: info.GetCloseTime().AsTime()}
	it := b.client.GetWorkflowHistory(ctx, e.WorkflowID, e.RunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	for it.HasNext() {
		event, err := it.Next()
		if err != nil {
			return preparation.PreparationRun{}, err
		}
		switch event.GetEventType() {
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
			var input preparation.PreparationInput
			if err := converter.GetDefaultDataConverter().FromPayloads(event.GetWorkflowExecutionStartedEventAttributes().GetInput(), &input); err != nil {
				return preparation.PreparationRun{}, err
			}
			result.Request = input.Request
			result.Kind = input.Kind
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED:
			result.Status = tmprl.StatusStopped
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

func (b *Business) StopPreparation(ctx context.Context, e tmprl.Execution) error {
	return preparation.ExecutionError(tmprl.Stop(ctx, b.client, e, preparationWorkflowName))
}

// OpenPreparationRecords lets any head stream persisted JSONL without knowing
// storage layout. The caller must close the reader. Source paths in a Temporal
// payload can never escape DatasetRoot, including through symlinks.
func (b *Business) OpenPreparationRecords(ctx context.Context, e tmprl.Execution) (io.ReadCloser, error) {
	run, err := b.GetPreparation(ctx, e)
	if err != nil {
		return nil, err
	}
	return b.OpenRecords(run)
}

// OpenRecords opens records from an already resolved preparation result.
func (b *Business) OpenRecords(run preparation.PreparationRun) (io.ReadCloser, error) {
	if run.Status != tmprl.StatusCompleted || run.Result == nil {
		return nil, fmt.Errorf("%w: preparation has no completed records", ErrInvalidArgument)
	}
	path, err := filepath.Rel(b.cfg.Datasets.Root(), run.Result.RecordsPath)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(b.cfg.Datasets.Root())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(path)
}

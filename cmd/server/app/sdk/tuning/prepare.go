package tuning

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
)

func (b *Business) StartPreview(ctx context.Context, input PreparationRequest) (PreparationRun, error) {
	return b.startPreparation(ctx, input, PreparationPreview)
}
func (b *Business) StartPreparation(ctx context.Context, input PreparationRequest) (PreparationRun, error) {
	return b.startPreparation(ctx, input, PreparationFull)
}

func (b *Business) startPreparation(ctx context.Context, input PreparationRequest, kind PreparationKind) (PreparationRun, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.Model = strings.TrimSpace(input.Model)
	input.Instructions = strings.TrimSpace(input.Instructions)
	input.ReviewedRecords = slices.Clone(input.ReviewedRecords)
	if input.Model == "" || len(input.ID) > 200 || utf8.RuneCountInString(input.Instructions) > 2000 {
		return PreparationRun{}, fmt.Errorf("%w: invalid preparation model, ID or instructions", ErrInvalidArgument)
	}
	switch input.Goal {
	case GoalQuestionAnswer, GoalClassification, GoalNumericPrediction, GoalTextGeneration:
	default:
		return PreparationRun{}, fmt.Errorf("%w: unsupported preparation goal", ErrInvalidArgument)
	}
	switch kind {
	case PreparationPreview:
		if input.PreviewRecords < 1 || input.PreviewRecords > 5 || len(input.ReviewedRecords) != 0 {
			return PreparationRun{}, fmt.Errorf("%w: preview requires 1–5 samples and no reviewed records", ErrInvalidArgument)
		}
	case PreparationFull:
		if input.PreviewRecords != 0 || len(input.ReviewedRecords) > 5 {
			return PreparationRun{}, fmt.Errorf("%w: full preparation accepts at most five reviewed records, not a preview count", ErrInvalidArgument)
		}
		if err := ValidateReviewedRecords(input.ReviewedRecords); err != nil {
			return PreparationRun{}, fmt.Errorf("%w: reviewed records: %s", ErrInvalidArgument, err)
		}
	}
	data, err := json.Marshal(input)
	if err != nil {
		return PreparationRun{}, err
	}
	if len(data) > 64<<10 {
		return PreparationRun{}, fmt.Errorf("%w: preparation input exceeds 64 KiB", ErrInvalidArgument)
	}
	dataset, err := b.DatasetSource(ctx, input.DatasetID)
	if err != nil {
		return PreparationRun{}, err
	}
	if dataset.Format == ".jsonl" {
		return PreparationRun{}, fmt.Errorf("%w: prepared JSONL does not need document preparation", ErrInvalidArgument)
	}
	if input.ID == "" {
		input.ID = "preparation-" + rand.Text()
	}
	run, err := b.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: input.ID, TaskQueue: b.cfg.TaskQueue, WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowExecutionErrorWhenAlreadyStarted: true}, preparationWorkflowName, preparationInput{Request: input, Kind: kind, Timeout: b.cfg.StageTimeout})
	if err != nil {
		return PreparationRun{}, err
	}
	return PreparationRun{WorkflowID: run.GetID(), RunID: run.GetRunID(), Request: input, Kind: kind, Status: StatusRunning}, nil
}

// PreparationHistory pages through preview and full preparation executions,
// including active runs. Use GetPreparation to retrieve each execution's details.
func (b *Business) PreparationHistory(ctx context.Context, pageSize int, next []byte) ([]Execution, []byte, error) {
	return b.history(ctx, preparationWorkflowName, pageSize, next)
}

func (b *Business) WaitPreparation(ctx context.Context, e Execution) (PreparationRun, error) {
	if err := validateExecution(e); err != nil {
		return PreparationRun{}, err
	}
	description, err := b.client.DescribeWorkflowExecution(ctx, e.WorkflowID, e.RunID)
	if err != nil {
		return PreparationRun{}, err
	}
	if description.GetWorkflowExecutionInfo().GetType().GetName() != preparationWorkflowName {
		return PreparationRun{}, fmt.Errorf("%w: not a preparation workflow", ErrInvalidArgument)
	}
	var result PreparationRun
	err = b.client.GetWorkflow(ctx, e.WorkflowID, e.RunID).Get(ctx, &result)
	return result, err
}

// GetPreparation never queries a closed execution: its durable result or
// terminal history is authoritative, even if no worker remains running.
func (b *Business) GetPreparation(ctx context.Context, e Execution) (PreparationRun, error) {
	if err := validateExecution(e); err != nil {
		return PreparationRun{}, err
	}
	description, err := b.client.DescribeWorkflowExecution(ctx, e.WorkflowID, e.RunID)
	if err != nil {
		return PreparationRun{}, err
	}
	info := description.GetWorkflowExecutionInfo()
	if info.GetType().GetName() != preparationWorkflowName {
		return PreparationRun{}, fmt.Errorf("%w: not a preparation workflow", ErrInvalidArgument)
	}
	switch info.GetStatus() {
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		var result PreparationRun
		err := b.client.GetWorkflow(ctx, e.WorkflowID, e.RunID).Get(ctx, &result)
		return result, err
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		value, err := b.client.QueryWorkflow(ctx, e.WorkflowID, e.RunID, stateQuery)
		if err != nil {
			return PreparationRun{}, err
		}
		var result PreparationRun
		if err := value.Get(&result); err != nil {
			return PreparationRun{}, err
		}
		for _, pending := range description.GetPendingActivities() {
			if pending.GetHeartbeatDetails() == nil {
				continue
			}
			var progress Progress
			if err := converter.GetDefaultDataConverter().FromPayloads(pending.GetHeartbeatDetails(), &progress); err != nil {
				return PreparationRun{}, err
			}
			if progress.Execution == e {
				result.Progress = &progress
			}
		}
		return result, nil
	}
	result := PreparationRun{Execution: e, Status: StatusFailed, StartedAt: info.GetStartTime().AsTime(), ClosedAt: info.GetCloseTime().AsTime()}
	it := b.client.GetWorkflowHistory(ctx, e.WorkflowID, e.RunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	for it.HasNext() {
		event, err := it.Next()
		if err != nil {
			return PreparationRun{}, err
		}
		switch event.GetEventType() {
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
			var input preparationInput
			if err := converter.GetDefaultDataConverter().FromPayloads(event.GetWorkflowExecutionStartedEventAttributes().GetInput(), &input); err != nil {
				return PreparationRun{}, err
			}
			result.Request = input.Request
			result.Kind = input.Kind
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

func (b *Business) StopPreparation(ctx context.Context, e Execution) error {
	if err := b.validateWorkflow(ctx, e, preparationWorkflowName); err != nil {
		return err
	}
	return b.client.CancelWorkflow(ctx, e.WorkflowID, e.RunID)
}

// OpenPreparationRecords lets any head stream persisted JSONL without knowing
// storage layout. The caller must close the reader. Source paths in a Temporal
// payload can never escape DatasetRoot, including through symlinks.
func (b *Business) OpenPreparationRecords(ctx context.Context, e Execution) (io.ReadCloser, error) {
	run, err := b.GetPreparation(ctx, e)
	if err != nil {
		return nil, err
	}
	return b.openPreparationRecords(run)
}

func (b *Business) openPreparationRecords(run PreparationRun) (io.ReadCloser, error) {
	if run.Status != StatusCompleted || run.Result == nil {
		return nil, fmt.Errorf("%w: preparation has no completed records", ErrInvalidArgument)
	}
	path, err := filepath.Rel(b.cfg.DatasetRoot, run.Result.RecordsPath)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(b.cfg.DatasetRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(path)
}

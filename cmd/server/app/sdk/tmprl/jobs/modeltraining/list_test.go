package modeltraining_test

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	modeltraining "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/stretchr/testify/mock"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/mocks"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestListMetadataAndPagination(t *testing.T) {
	started := time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC)
	closed := started.Add(time.Hour)
	info := &workflowpb.WorkflowExecutionInfo{
		Execution: &commonpb.WorkflowExecution{WorkflowId: "old-training", RunId: "old-run"},
		Type:      &commonpb.WorkflowType{Name: modeltraining.WorkflowName},
		Status:    enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED,
		StartTime: timestamppb.New(started), CloseTime: timestamppb.New(closed),
	}
	next := []byte{0, 255, 12}
	c := mocks.NewClient(t)
	c.On("ListWorkflow", mock.Anything, &workflowservice.ListWorkflowExecutionsRequest{
		PageSize: 100, Query: "WorkflowType = 'moxie.tuning.Run'",
	}).Return(&workflowservice.ListWorkflowExecutionsResponse{
		Executions: []*workflowpb.WorkflowExecutionInfo{info}, NextPageToken: next,
	}, nil).Once()
	c.On("ListWorkflow", mock.Anything, &workflowservice.ListWorkflowExecutionsRequest{
		PageSize: 100, Query: "WorkflowType = 'moxie.tuning.Run'", NextPageToken: next,
	}).Return(&workflowservice.ListWorkflowExecutionsResponse{}, nil).Once()
	b := newServices(t, c, serviceConfig{}).Training
	runs, token, err := b.List(t.Context(), tmprl.ListOptions{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs: got %d, want 1", len(runs))
	}
	run := runs[0]
	if run.WorkflowID != "old-training" || run.RunID != "old-run" || run.Status != tmprl.StatusCompleted || !run.StartedAt.Equal(started) || !run.ClosedAt.Equal(closed) || !reflect.DeepEqual(run.Request, modeltraining.Request{}) {
		t.Errorf("metadata-only run: got %+v, want completed old-training/old-run with visibility timestamps and no request", run)
	}
	if !bytes.Equal(token, next) {
		t.Errorf("token: got %v, want %v", token, next)
	}
	runs, token, err = b.List(t.Context(), tmprl.ListOptions{PageSize: 100, NextPageToken: token})
	if err != nil || len(runs) != 0 || len(token) != 0 {
		t.Fatalf("last page: got %v, %v, %v, want empty page and token", runs, token, err)
	}
	// Only ListWorkflow has mock expectations. Any Describe, Query, Get, or
	// history lookup fails the test, including fallback enrichment for old jobs.
}

func TestListStatusFiltering(t *testing.T) {
	for _, test := range []struct {
		name     string
		statuses []tmprl.Status
		want     []tmprl.Status
	}{
		{name: "all", want: []tmprl.Status{tmprl.StatusRunning, tmprl.StatusCompleted, tmprl.StatusStopped, tmprl.StatusFailed}},
		{name: "terminal", statuses: []tmprl.Status{tmprl.StatusStopped, tmprl.StatusFailed}, want: []tmprl.Status{tmprl.StatusStopped, tmprl.StatusFailed}},
		{name: "empty-page", statuses: []tmprl.Status{tmprl.StatusPaused}},
		{name: "all-known-statuses", statuses: []tmprl.Status{tmprl.StatusQueued, tmprl.StatusRecovering, tmprl.StatusRunning, tmprl.StatusPauseRequested, tmprl.StatusPaused, tmprl.StatusCompleted, training.StatusNeedsImprovement, tmprl.StatusStopped, tmprl.StatusFailed}, want: []tmprl.Status{tmprl.StatusRunning, tmprl.StatusCompleted, tmprl.StatusStopped, tmprl.StatusFailed}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var infos []*workflowpb.WorkflowExecutionInfo
			for _, status := range []enumspb.WorkflowExecutionStatus{
				enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
				enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED,
				enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED,
				enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED,
			} {
				infos = append(infos, &workflowpb.WorkflowExecutionInfo{
					Execution: &commonpb.WorkflowExecution{WorkflowId: "job", RunId: status.String()},
					Type:      &commonpb.WorkflowType{Name: modeltraining.WorkflowName},
					Status:    status,
				})
			}
			c := mocks.NewClient(t)
			next := []byte{255, 0, 99}
			c.On("ListWorkflow", mock.Anything, &workflowservice.ListWorkflowExecutionsRequest{
				PageSize: 10, Query: "WorkflowType = 'moxie.tuning.Run'",
			}).Return(&workflowservice.ListWorkflowExecutionsResponse{Executions: infos, NextPageToken: next}, nil).Once()
			b := newServices(t, c, serviceConfig{}).Training
			runs, token, err := b.List(t.Context(), tmprl.ListOptions{PageSize: 10, Statuses: test.statuses})
			if err != nil {
				t.Fatal(err)
			}
			var got []tmprl.Status
			for _, run := range runs {
				got = append(got, run.Status)
			}
			if !reflect.DeepEqual(got, test.want) || !bytes.Equal(token, next) {
				t.Errorf("filtered page: got %v, %v, want %v, %v", got, token, test.want, next)
			}
		})
	}
}

func TestListSnapshotsAndTerminalStatus(t *testing.T) {
	for _, test := range []struct {
		name      string
		logical   tmprl.Status
		lifecycle enumspb.WorkflowExecutionStatus
		want      tmprl.Status
	}{
		{name: "queued", logical: tmprl.StatusQueued, lifecycle: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, want: tmprl.StatusQueued},
		{name: "paused", logical: tmprl.StatusPaused, lifecycle: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, want: tmprl.StatusPaused},
		{name: "recovering", logical: tmprl.StatusRecovering, lifecycle: enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, want: tmprl.StatusRecovering},
		{name: "rejected", logical: training.StatusNeedsImprovement, lifecycle: enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED, want: training.StatusNeedsImprovement},
		{name: "stale-running-canceled", logical: tmprl.StatusRunning, lifecycle: enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED, want: tmprl.StatusStopped},
		{name: "stale-paused-terminated", logical: tmprl.StatusPaused, lifecycle: enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED, want: tmprl.StatusFailed},
		{name: "stale-running-completed", logical: tmprl.StatusRunning, lifecycle: enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED, want: tmprl.StatusCompleted},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := tmprl.Run[modeltraining.Request, modeltraining.Artifacts]{
				Status: test.logical, Request: modeltraining.Request{Model: "model-from-snapshot"}, Stage: modeltraining.StageTrain,
				Stages: []tmprl.StageState{{Name: modeltraining.StageSplit, Status: tmprl.StatusCompleted}, {Name: modeltraining.StageTrain, Status: tmprl.StatusRunning}},
			}
			payload, err := converter.GetDefaultDataConverter().ToPayload(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			info := &workflowpb.WorkflowExecutionInfo{
				Execution: &commonpb.WorkflowExecution{WorkflowId: "snapshot-job", RunId: "snapshot-run"},
				Type:      &commonpb.WorkflowType{Name: modeltraining.WorkflowName}, Status: test.lifecycle,
				Memo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"moxie.training.run": payload}},
			}
			c := mocks.NewClient(t)
			c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{
				Executions: []*workflowpb.WorkflowExecutionInfo{info},
			}, nil).Once()
			b := newServices(t, c, serviceConfig{}).Training
			runs, _, err := b.List(t.Context(), tmprl.ListOptions{PageSize: 1, Statuses: []tmprl.Status{test.want}})
			if err != nil || len(runs) != 1 {
				t.Fatalf("snapshot page: got %v, %v, want one run", runs, err)
			}
			if runs[0].Status != test.want || runs[0].Request.Model != snapshot.Request.Model || runs[0].WorkflowID != "snapshot-job" || runs[0].RunID != "snapshot-run" {
				t.Errorf("snapshot run: got %+v, want status %s and snapshot model with visibility execution IDs", runs[0], test.want)
			}
			if test.want == tmprl.StatusFailed || test.want == tmprl.StatusStopped {
				if runs[0].Stages[0].Status != tmprl.StatusCompleted || runs[0].Stages[1].Status != test.want {
					t.Errorf("terminal stages: got %+v, want committed split and %s training", runs[0].Stages, test.want)
				}
			}
		})
	}
}

func TestListInvalidOptions(t *testing.T) {
	for _, test := range []struct {
		name    string
		options tmprl.ListOptions
	}{
		{name: "zero", options: tmprl.ListOptions{}},
		{name: "negative", options: tmprl.ListOptions{PageSize: -1}},
		{name: "too-large", options: tmprl.ListOptions{PageSize: 101}},
		{name: "empty-status", options: tmprl.ListOptions{PageSize: 1, Statuses: []tmprl.Status{""}}},
		{name: "unknown-status", options: tmprl.ListOptions{PageSize: 1, Statuses: []tmprl.Status{tmprl.StatusRunning, "unknown"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := mocks.NewClient(t)
			b := newServices(t, c, serviceConfig{}).Training
			runs, token, err := b.List(t.Context(), test.options)
			if !errors.Is(err, modeltraining.ErrInvalidArgument) || runs != nil || token != nil {
				t.Fatalf("invalid options: got %v, %v, %v, want nil results and ErrInvalidArgument", runs, token, err)
			}
		})
	}
}

func TestListClientError(t *testing.T) {
	want := errors.New("visibility unavailable")
	c := mocks.NewClient(t)
	c.On("ListWorkflow", mock.Anything, mock.Anything).Return(nil, want).Once()
	b := newServices(t, c, serviceConfig{}).Training
	runs, token, err := b.List(t.Context(), tmprl.ListOptions{PageSize: 1})
	if !errors.Is(err, want) || runs != nil || token != nil {
		t.Fatalf("client failure: got %v, %v, %v, want nil results and wrapped client error", runs, token, err)
	}
}

package tmprl_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

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

func newSnapshotJobs(t *testing.T, c *mocks.Client) *tmprl.Jobs[string, []string] {
	t.Helper()
	j, err := tmprl.New(c, tmprl.Config[string, []string]{
		Names:     tmprl.Names{Workflow: "example.Export", Activity: "example.Step", Memo: "example.snapshot", Query: "example-query", Control: "example-control"},
		TaskQueue: "snapshot-tests", StorageRoot: t.TempDir(), StorageDirectory: "exports", Outcomes: []tmprl.Status{"rejected"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.AssertExpectations(t) })
	return j
}

func snapshotInfo(t *testing.T, id string, status enumspb.WorkflowExecutionStatus, state *tmprl.Run[string, []string]) *workflowpb.WorkflowExecutionInfo {
	t.Helper()
	info := &workflowpb.WorkflowExecutionInfo{
		Execution: &commonpb.WorkflowExecution{WorkflowId: "export-" + id, RunId: "run-" + id},
		Type:      &commonpb.WorkflowType{Name: "example.Export"}, Status: status,
		StartTime: timestamppb.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
	}
	if status != enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		info.CloseTime = timestamppb.New(info.StartTime.AsTime().Add(time.Hour))
	}
	if state != nil {
		payload, err := converter.GetDefaultDataConverter().ToPayload(state)
		if err != nil {
			t.Fatal(err)
		}
		info.Memo = &commonpb.Memo{Fields: map[string]*commonpb.Payload{"example.snapshot": payload}}
	}
	return info
}

func TestJobsListStatusPages(t *testing.T) {
	statuses := []tmprl.Status{tmprl.StatusQueued, tmprl.StatusRunning, tmprl.StatusRecovering, tmprl.StatusPauseRequested, tmprl.StatusPaused, tmprl.StatusCompleted, tmprl.StatusStopped, tmprl.StatusFailed, "rejected"}
	var infos []*workflowpb.WorkflowExecutionInfo
	for _, status := range statuses {
		server := enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING
		switch status {
		case tmprl.StatusCompleted, "rejected":
			server = enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED
		case tmprl.StatusStopped:
			server = enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED
		case tmprl.StatusFailed:
			server = enumspb.WORKFLOW_EXECUTION_STATUS_FAILED
		}
		infos = append(infos, snapshotInfo(t, string(status), server, &tmprl.Run[string, []string]{Status: status, Request: "request-" + string(status), Artifacts: []string{"artifact-" + string(status)}}))
	}
	for _, test := range []struct {
		name    string
		filters []tmprl.Status
		infos   []*workflowpb.WorkflowExecutionInfo
		want    []tmprl.Status
	}{
		{name: "unfiltered", infos: infos, want: statuses},
		{name: "short-exact-running", filters: []tmprl.Status{tmprl.StatusRunning}, infos: infos, want: []tmprl.Status{tmprl.StatusRunning}},
		{name: "empty-filtered", filters: []tmprl.Status{tmprl.StatusPaused}, infos: infos[:4], want: []tmprl.Status{}},
		{name: "custom-outcome-not-completed", filters: []tmprl.Status{"rejected"}, infos: infos, want: []tmprl.Status{"rejected"}},
		{name: "completed-not-custom-outcome", filters: []tmprl.Status{tmprl.StatusCompleted}, infos: infos, want: []tmprl.Status{tmprl.StatusCompleted}},
		{name: "multiple-exact-statuses", filters: []tmprl.Status{tmprl.StatusRecovering, tmprl.StatusPaused}, infos: infos, want: []tmprl.Status{tmprl.StatusRecovering, tmprl.StatusPaused}},
		{name: "empty-server-page", infos: nil, want: []tmprl.Status{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := new(mocks.Client)
			j := newSnapshotJobs(t, c)
			c.On("ListWorkflow", mock.Anything, &workflowservice.ListWorkflowExecutionsRequest{
				PageSize: 17, NextPageToken: []byte{0, 255, 7}, Query: "WorkflowType = 'example.Export'",
			}).Return(&workflowservice.ListWorkflowExecutionsResponse{Executions: test.infos, NextPageToken: []byte{9, 0, 128}}, nil).Once()
			runs, next, err := j.List(t.Context(), tmprl.ListOptions{PageSize: 17, NextPageToken: []byte{0, 255, 7}, Statuses: test.filters})
			if err != nil {
				t.Fatal(err)
			}
			got := make([]tmprl.Status, 0, len(runs))
			for _, run := range runs {
				got = append(got, run.Status)
				if run.WorkflowID != "export-"+string(run.Status) || run.RunID != "run-"+string(run.Status) || run.Request != "request-"+string(run.Status) || !reflect.DeepEqual(run.Artifacts, []string{"artifact-" + string(run.Status)}) {
					t.Errorf("snapshot: got %+v, want status-specific identity/request/artifacts", run)
				}
			}
			if !reflect.DeepEqual(got, test.want) || !reflect.DeepEqual(next, []byte{9, 0, 128}) {
				t.Errorf("page: got %v token %v, want %v token [9 0 128]", got, next, test.want)
			}
		})
	}
}

func TestJobsListTerminalMetadata(t *testing.T) {
	for _, test := range []struct {
		name   string
		server enumspb.WorkflowExecutionStatus
		want   tmprl.Status
	}{
		{"completed", enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED, tmprl.StatusCompleted},
		{"canceled", enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED, tmprl.StatusStopped},
		{"failed", enumspb.WORKFLOW_EXECUTION_STATUS_FAILED, tmprl.StatusFailed},
		{"terminated", enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED, tmprl.StatusFailed},
		{"timed-out", enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT, tmprl.StatusFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := new(mocks.Client)
			j := newSnapshotJobs(t, c)
			info := snapshotInfo(t, "closed", test.server, &tmprl.Run[string, []string]{WorkflowID: "stale", RunID: "old", Status: tmprl.StatusRunning})
			c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{Executions: []*workflowpb.WorkflowExecutionInfo{info}}, nil).Once()
			runs, next, err := j.List(t.Context(), tmprl.ListOptions{PageSize: 1, Statuses: []tmprl.Status{test.want}})
			if err != nil || len(runs) != 1 {
				t.Fatalf("List: got %v, %v, want one terminal snapshot", runs, err)
			}
			run := runs[0]
			if run.Status != test.want || run.WorkflowID != "export-closed" || run.RunID != "run-closed" || !run.StartedAt.Equal(info.StartTime.AsTime()) || !run.UpdatedAt.Equal(run.StartedAt) || !run.ClosedAt.Equal(info.CloseTime.AsTime()) || len(next) != 0 {
				t.Errorf("terminal metadata: got %+v token %v, want authoritative identity/times/status %s", run, next, test.want)
			}
		})
	}
}

func TestJobsListErrors(t *testing.T) {
	for _, options := range []tmprl.ListOptions{{PageSize: -1}, {PageSize: 0}, {PageSize: 101}, {PageSize: 1, Statuses: []tmprl.Status{"unknown"}}, {PageSize: 1, Statuses: []tmprl.Status{tmprl.StatusRunning, ""}}} {
		c := new(mocks.Client)
		j := newSnapshotJobs(t, c)
		runs, next, err := j.List(t.Context(), options)
		if !errors.Is(err, tmprl.ErrInvalidArgument) || runs != nil || next != nil {
			t.Errorf("options %+v: got %v, %v, %v, want nil page and invalid argument", options, runs, next, err)
		}
	}
	t.Run("rpc-error", func(t *testing.T) {
		c := new(mocks.Client)
		j := newSnapshotJobs(t, c)
		cause := errors.New("visibility unavailable")
		c.On("ListWorkflow", mock.Anything, mock.Anything).Return(nil, cause).Once()
		if runs, next, err := j.List(t.Context(), tmprl.ListOptions{PageSize: 100}); !errors.Is(err, cause) || runs != nil || next != nil {
			t.Errorf("List: got %v, %v, %v, want nil page and wrapped cause", runs, next, err)
		}
	})
	t.Run("malformed-memo", func(t *testing.T) {
		c := new(mocks.Client)
		j := newSnapshotJobs(t, c)
		info := snapshotInfo(t, "bad", enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, nil)
		info.Memo = &commonpb.Memo{Fields: map[string]*commonpb.Payload{"example.snapshot": {Metadata: map[string][]byte{"encoding": []byte("json/plain")}, Data: []byte("{")}}}
		good := snapshotInfo(t, "good", enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, &tmprl.Run[string, []string]{Status: tmprl.StatusRunning})
		c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{Executions: []*workflowpb.WorkflowExecutionInfo{good, info}, NextPageToken: []byte("unused")}, nil).Once()
		if runs, next, err := j.List(t.Context(), tmprl.ListOptions{PageSize: 2}); err == nil || runs != nil || next != nil {
			t.Errorf("List: got %v, %v, %v, want nil page and decoding error", runs, next, err)
		}
	})
}

func TestJobsListWithoutMemo(t *testing.T) {
	c := new(mocks.Client)
	j := newSnapshotJobs(t, c)
	active := snapshotInfo(t, "active", enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, nil)
	closed := snapshotInfo(t, "closed", enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED, nil)
	c.On("ListWorkflow", mock.Anything, mock.Anything).Return(&workflowservice.ListWorkflowExecutionsResponse{Executions: []*workflowpb.WorkflowExecutionInfo{active, closed}}, nil).Once()
	runs, _, err := j.List(t.Context(), tmprl.ListOptions{PageSize: 2})
	if err != nil || len(runs) != 2 {
		t.Fatalf("List: got %v, %v, want two metadata-only runs", runs, err)
	}
	if runs[0].Status != tmprl.StatusRunning || !runs[0].StateStale || runs[0].WorkflowID != "export-active" || runs[1].Status != tmprl.StatusCompleted || runs[1].StateStale || runs[1].ClosedAt.IsZero() {
		t.Errorf("metadata-only snapshots: got %+v, want stale running and closed completed metadata", runs)
	}
}

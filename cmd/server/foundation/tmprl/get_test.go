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
	failurepb "go.temporal.io/api/failure/v1"
	historypb "go.temporal.io/api/history/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/mocks"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestJobsGetSnapshotWithoutWorker(t *testing.T) {
	for _, status := range []tmprl.Status{tmprl.StatusQueued, tmprl.StatusRunning, tmprl.StatusRecovering, tmprl.StatusPauseRequested, tmprl.StatusPaused, tmprl.StatusWaiting} {
		t.Run(string(status), func(t *testing.T) {
			c := new(mocks.Client)
			j := newSnapshotJobs(t, c)
			state := tmprl.Run[string, []string]{
				Status: status, Request: "immutable request", Artifacts: []string{"committed-a", "committed-c"},
				Definition: &tmprl.Job{Name: "persisted-plan"}, SessionAttempt: 3, WorkerHost: "former-worker",
				Stage: "publish", Step: "upload", UpdatedAt: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC),
			}
			if status == tmprl.StatusWaiting {
				state.Checkpoint = &tmprl.Checkpoint{ID: "run-snapshot/publish/upload/review", Name: "review", Stage: "publish", Step: "upload"}
				state.Decisions = []tmprl.Decision{{CheckpointID: "previous", SubmissionID: "accepted"}}
			}
			info := snapshotInfo(t, "snapshot", enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, &state)
			e := tmprl.Execution{WorkflowID: "export-snapshot", RunID: "run-snapshot"}
			c.On("DescribeWorkflowExecution", mock.Anything, e.WorkflowID, e.RunID).Return(&workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: info}, nil).Once()
			got, err := j.Get(t.Context(), e)
			if err != nil {
				t.Fatal(err)
			}
			state.Execution, state.StartedAt = e, info.StartTime.AsTime()
			if !reflect.DeepEqual(got, state) {
				t.Errorf("Get: got %+v, want %+v", got, state)
			}
			// Any worker query or history call is unexpected on this strict mock.
		})
	}
}

func TestJobsGetCheckpointFromHistory(t *testing.T) {
	c := new(mocks.Client)
	j := newSnapshotJobs(t, c)
	e := tmprl.Execution{WorkflowID: "export-history", RunID: "run-history"}
	info := snapshotInfo(t, "history", enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, nil)
	state := tmprl.Run[string, []string]{
		Status: tmprl.StatusWaiting, Request: "immutable", Artifacts: []string{"preview"},
		Checkpoint: &tmprl.Checkpoint{ID: "run-history/prepare/preview/review", Name: "review", Stage: "prepare", Step: "preview"},
		Decisions:  []tmprl.Decision{{CheckpointID: "previous", SubmissionID: "accepted"}},
	}
	payload, err := converter.GetDefaultDataConverter().ToPayload(state)
	if err != nil {
		t.Fatal(err)
	}
	c.On("DescribeWorkflowExecution", mock.Anything, e.WorkflowID, e.RunID).Return(&workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: info}, nil)
	c.On("QueryWorkflow", mock.Anything, e.WorkflowID, e.RunID, "example-query").Return(nil, errors.New("worker unavailable"))
	it := mocks.NewHistoryEventIterator(t)
	it.On("HasNext").Return(true).Once()
	it.On("Next").Return(&historypb.HistoryEvent{
		EventType: enumspb.EVENT_TYPE_WORKFLOW_PROPERTIES_MODIFIED,
		Attributes: &historypb.HistoryEvent_WorkflowPropertiesModifiedEventAttributes{WorkflowPropertiesModifiedEventAttributes: &historypb.WorkflowPropertiesModifiedEventAttributes{
			UpsertedMemo: &commonpb.Memo{Fields: map[string]*commonpb.Payload{"example.snapshot": payload}},
		}},
	}, nil).Once()
	it.On("HasNext").Return(false).Once()
	c.On("GetWorkflowHistory", mock.Anything, e.WorkflowID, e.RunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT).Return(it)
	got, err := j.Get(t.Context(), e)
	state.Execution, state.StartedAt, state.UpdatedAt, state.StateStale = e, info.StartTime.AsTime(), info.StartTime.AsTime(), true
	if err != nil || !reflect.DeepEqual(got, state) {
		t.Fatalf("history checkpoint: got %+v, %v, want %+v", got, err, state)
	}
}

func TestJobsGetPendingDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name         string
		change       func(*tmprl.Run[string, []string], *workflowservice.DescribeWorkflowExecutionResponse, *tmprl.Progress)
		wantStale    bool
		wantProgress bool
	}{
		{name: "fresh", wantProgress: true},
		{name: "pending-workflow-task", wantProgress: true, wantStale: true, change: func(_ *tmprl.Run[string, []string], d *workflowservice.DescribeWorkflowExecutionResponse, _ *tmprl.Progress) {
			d.PendingWorkflowTask = &workflowpb.PendingWorkflowTaskInfo{}
		}},
		{name: "old-heartbeat", wantProgress: true, wantStale: true, change: func(_ *tmprl.Run[string, []string], d *workflowservice.DescribeWorkflowExecutionResponse, _ *tmprl.Progress) {
			d.PendingActivities[0].LastHeartbeatTime = timestamppb.New(time.Now().Add(-48 * time.Hour))
		}},
		{name: "expired-reservation", wantProgress: true, wantStale: true, change: func(s *tmprl.Run[string, []string], _ *workflowservice.DescribeWorkflowExecutionResponse, _ *tmprl.Progress) {
			s.ReservationDeadline = time.Now().Add(-48 * time.Hour)
		}},
		{name: "expired-stage", wantProgress: true, wantStale: true, change: func(s *tmprl.Run[string, []string], _ *workflowservice.DescribeWorkflowExecutionResponse, _ *tmprl.Progress) {
			s.Stages[1].Deadline = time.Now().Add(-48 * time.Hour)
		}},
		{name: "wrong-workflow-progress", change: func(_ *tmprl.Run[string, []string], _ *workflowservice.DescribeWorkflowExecutionResponse, p *tmprl.Progress) {
			p.WorkflowID = "another-workflow"
		}},
		{name: "wrong-run-progress", change: func(_ *tmprl.Run[string, []string], _ *workflowservice.DescribeWorkflowExecutionResponse, p *tmprl.Progress) {
			p.RunID = "earlier-run"
		}},
		{name: "wrong-stage-progress", change: func(_ *tmprl.Run[string, []string], _ *workflowservice.DescribeWorkflowExecutionResponse, p *tmprl.Progress) {
			p.Stage = "prepare"
		}},
		{name: "wrong-step-progress", change: func(_ *tmprl.Run[string, []string], _ *workflowservice.DescribeWorkflowExecutionResponse, p *tmprl.Progress) {
			p.Step = "index"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := new(mocks.Client)
			j := newSnapshotJobs(t, c)
			e := tmprl.Execution{WorkflowID: "export-diagnostics", RunID: "run-diagnostics"}
			state := tmprl.Run[string, []string]{
				Execution: e, Status: tmprl.StatusRunning, Stage: "publish", Step: "upload",
				Definition: &tmprl.Job{Stages: []tmprl.Stage{{Name: "publish", Steps: []tmprl.Step{{Name: "upload", HeartbeatTimeout: 24 * time.Hour}}}}},
				Stages: []tmprl.StageState{
					{Name: "prepare", Status: tmprl.StatusCompleted, Attempt: 2, LastError: "previous-stage", Steps: []tmprl.StepState{{Name: "upload", Attempt: 2}}},
					{Name: "publish", Status: tmprl.StatusRunning, Steps: []tmprl.StepState{{Name: "index", Status: tmprl.StatusCompleted, Attempt: 1, LastError: "previous-step"}, {Name: "upload", Status: tmprl.StatusRunning}}},
				},
			}
			stamp := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
			progress := tmprl.Progress{Execution: e, Stage: "publish", Step: "upload", Completed: 7, Total: 19, Message: "uploading shard", InitialCompleted: 3, StartedAt: stamp, UpdatedAt: stamp.Add(2 * time.Minute)}
			description := &workflowservice.DescribeWorkflowExecutionResponse{PendingActivities: []*workflowpb.PendingActivityInfo{{
				ActivityType: &commonpb.ActivityType{Name: "example.Step"}, Attempt: 4,
				LastHeartbeatTime: timestamppb.New(time.Now().Add(-time.Hour)),
				LastFailure:       &failurepb.Failure{Message: "retry upload", Cause: &failurepb.Failure{Message: "disk unavailable"}},
			}}}
			if test.change != nil {
				test.change(&state, description, &progress)
			}
			payloads, err := converter.GetDefaultDataConverter().ToPayloads(progress)
			if err != nil {
				t.Fatal(err)
			}
			description.PendingActivities[0].HeartbeatDetails = payloads
			description.WorkflowExecutionInfo = snapshotInfo(t, "diagnostics", enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, &state)
			c.On("DescribeWorkflowExecution", mock.Anything, e.WorkflowID, e.RunID).Return(description, nil).Once()
			got, err := j.Get(t.Context(), e)
			if err != nil {
				t.Fatal(err)
			}
			if got.StateStale != test.wantStale || (got.Progress != nil) != test.wantProgress {
				t.Errorf("diagnostics: got stale %t progress %+v, want stale %t progress present %t", got.StateStale, got.Progress, test.wantStale, test.wantProgress)
			}
			if test.wantProgress && !reflect.DeepEqual(got.Progress, &progress) {
				t.Errorf("progress: got %+v, want %+v", got.Progress, progress)
			}
			if len(got.Stages) != 2 || len(got.Stages[1].Steps) != 2 {
				t.Fatalf("stages: got %+v, want two stages and two publish steps", got.Stages)
			}
			stage, step := got.Stages[1], got.Stages[1].Steps[1]
			heartbeat := description.PendingActivities[0].LastHeartbeatTime.AsTime()
			if stage.Attempt != 4 || step.Attempt != 4 || stage.LastError != "retry upload: disk unavailable" || step.LastError != stage.LastError || !stage.LastHeartbeatAt.Equal(heartbeat) || !step.LastHeartbeatAt.Equal(heartbeat) {
				t.Errorf("active diagnostics: got stage %+v step %+v, want attempt 4, heartbeat %s and nested failure", stage, step, heartbeat)
			}
			if !reflect.DeepEqual(got.Stages[0], state.Stages[0]) || !reflect.DeepEqual(got.Stages[1].Steps[0], state.Stages[1].Steps[0]) {
				t.Errorf("inactive diagnostics: got %+v, want inactive entries unchanged from %+v", got.Stages, state.Stages)
			}
		})
	}
}

func TestJobsGetErrors(t *testing.T) {
	for _, e := range []tmprl.Execution{{}, {WorkflowID: "export-only"}, {RunID: "run-only"}} {
		c := new(mocks.Client)
		j := newSnapshotJobs(t, c)
		if _, err := j.Get(t.Context(), e); !errors.Is(err, tmprl.ErrInvalidExecution) {
			t.Errorf("execution %+v: got %v, want invalid execution", e, err)
		}
	}
	for _, test := range []struct {
		name    string
		change  func(*workflowservice.DescribeWorkflowExecutionResponse)
		wantErr error
	}{
		{name: "wrong-workflow", wantErr: tmprl.ErrUnsupportedWorkflow, change: func(d *workflowservice.DescribeWorkflowExecutionResponse) {
			d.WorkflowExecutionInfo.Type.Name = "another.Workflow"
		}},
		{name: "missing-info", wantErr: tmprl.ErrUnsupportedWorkflow, change: func(d *workflowservice.DescribeWorkflowExecutionResponse) { d.WorkflowExecutionInfo = nil }},
		{name: "malformed-memo", change: func(d *workflowservice.DescribeWorkflowExecutionResponse) {
			d.WorkflowExecutionInfo.Memo.Fields["example.snapshot"].Data = []byte("{")
		}},
		{name: "malformed-heartbeat", change: func(d *workflowservice.DescribeWorkflowExecutionResponse) {
			d.PendingActivities = []*workflowpb.PendingActivityInfo{{ActivityType: &commonpb.ActivityType{Name: "example.Step"}, HeartbeatDetails: &commonpb.Payloads{Payloads: []*commonpb.Payload{{Metadata: map[string][]byte{"encoding": []byte("json/plain")}, Data: []byte("{")}}}}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := new(mocks.Client)
			j := newSnapshotJobs(t, c)
			d := &workflowservice.DescribeWorkflowExecutionResponse{WorkflowExecutionInfo: snapshotInfo(t, "errors", enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, &tmprl.Run[string, []string]{Status: tmprl.StatusRunning})}
			test.change(d)
			c.On("DescribeWorkflowExecution", mock.Anything, "export-errors", "run-errors").Return(d, nil).Once()
			_, err := j.Get(t.Context(), tmprl.Execution{WorkflowID: "export-errors", RunID: "run-errors"})
			if err == nil || (test.wantErr != nil && !errors.Is(err, test.wantErr)) {
				t.Errorf("Get: got %v, want error %v", err, test.wantErr)
			}
		})
	}
	t.Run("describe-error", func(t *testing.T) {
		c := new(mocks.Client)
		j := newSnapshotJobs(t, c)
		cause := errors.New("describe unavailable")
		c.On("DescribeWorkflowExecution", mock.Anything, "export-errors", "run-errors").Return(nil, cause).Once()
		if _, err := j.Get(t.Context(), tmprl.Execution{WorkflowID: "export-errors", RunID: "run-errors"}); !errors.Is(err, cause) {
			t.Errorf("Get: got %v, want describe cause", err)
		}
	})
}

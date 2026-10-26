package tmprl

import (
	"slices"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	failurepb "go.temporal.io/api/failure/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
)

// runFromInfo reads a durable snapshot without contacting a worker.
func (j *Jobs[R, A]) runFromInfo(info *workflowpb.WorkflowExecutionInfo) (Run[R, A], bool, error) {
	var state Run[R, A]
	payload, found := info.GetMemo().GetFields()[j.cfg.Names.Memo]
	if found {
		if err := converter.GetDefaultDataConverter().FromPayload(payload, &state); err != nil {
			return Run[R, A]{}, false, err
		}
	}
	j.reconcileRun(&state, info)
	if !found && info.GetStatus() == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		state.Status, state.StateStale = StatusRunning, true
	}
	return state, found, nil
}

// reconcileRun makes server metadata authoritative even if a memo or result
// predates closure, while retaining declared application outcomes on success.
func (j *Jobs[R, A]) reconcileRun(state *Run[R, A], info *workflowpb.WorkflowExecutionInfo) {
	state.Execution = Execution{WorkflowID: info.GetExecution().GetWorkflowId(), RunID: info.GetExecution().GetRunId()}
	if state.StartedAt.IsZero() && info.GetStartTime() != nil {
		state.StartedAt = info.GetStartTime().AsTime()
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = state.StartedAt
	}
	if state.ClosedAt.IsZero() && info.GetCloseTime() != nil {
		state.ClosedAt = info.GetCloseTime().AsTime()
	}
	switch info.GetStatus() {
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		if !slices.Contains(j.cfg.Outcomes, state.Status) {
			state.Status = StatusCompleted
		}
		state.Stage, state.Step = "", ""
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		state.Status, state.Stage, state.Step = StatusStopped, "", ""
	case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		state.Status = StatusFailed
		if state.Error == "" {
			state.Error = "job terminated"
		}
	case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		state.Status = StatusFailed
		if state.Error == "" {
			state.Error = "job workflow timed out"
		}
	default:
		state.Status = StatusFailed
		if state.Error == "" {
			state.Error = "job workflow failed"
		}
	}
	if state.Status == StatusFailed || state.Status == StatusStopped {
		CloseJobStages(state)
	}
}

func (j *Jobs[R, A]) resultActivity(name string) bool {
	return name == j.cfg.Names.Activity || slices.Contains(j.cfg.ResultActivities, name)
}

func (j *Jobs[R, A]) knownActivity(name string) bool {
	return j.resultActivity(name) || slices.Contains(j.cfg.ArtifactActivities, name)
}

func (j *Jobs[R, A]) enrichRun(state *Run[R, A], description *workflowservice.DescribeWorkflowExecutionResponse) error {
	state.StateStale = description.GetPendingWorkflowTask() != nil
	if !state.ReservationDeadline.IsZero() && time.Now().After(state.ReservationDeadline) {
		state.StateStale = true
	}
	for _, pending := range description.GetPendingActivities() {
		name := pending.GetActivityType().GetName()
		heartbeatTimeout := 5 * time.Second
		if name == j.cfg.Names.Activity && state.Definition != nil {
			for _, stage := range state.Definition.Stages {
				for _, step := range stage.Steps {
					if stage.Name == string(state.Stage) && step.Name == state.Step {
						heartbeatTimeout = step.HeartbeatTimeout
					}
				}
			}
		}
		// Include session heartbeats even while paused without an active step.
		if pending.GetLastHeartbeatTime() != nil && time.Since(pending.GetLastHeartbeatTime().AsTime()) > heartbeatTimeout {
			state.StateStale = true
		}
		if !j.knownActivity(name) {
			continue
		}
		for i := range state.Stages {
			stage := &state.Stages[i]
			if stage.Name != state.Stage {
				continue
			}
			stage.Attempt = pending.GetAttempt()
			if pending.GetLastHeartbeatTime() != nil {
				stage.LastHeartbeatAt = pending.GetLastHeartbeatTime().AsTime()
			}
			stage.LastError = failureMessage(pending.GetLastFailure())
			if !stage.Deadline.IsZero() && time.Now().After(stage.Deadline) {
				state.StateStale = true
			}
			for k := range stage.Steps {
				step := &stage.Steps[k]
				if step.Name == state.Step {
					step.Attempt, step.LastHeartbeatAt, step.LastError = stage.Attempt, stage.LastHeartbeatAt, stage.LastError
				}
			}
		}
		if pending.GetHeartbeatDetails() != nil {
			var progress Progress
			if err := converter.GetDefaultDataConverter().FromPayloads(pending.GetHeartbeatDetails(), &progress); err != nil {
				return err
			}
			if progress.Execution == state.Execution && progress.Stage == state.Stage && progress.Step == state.Step {
				state.Progress = &progress
			}
		}
	}
	return nil
}

func failureMessage(failure *failurepb.Failure) string {
	var messages []string
	for failure != nil {
		messages = append(messages, failure.GetMessage())
		failure = failure.GetCause()
	}
	return strings.Join(messages, ": ")
}

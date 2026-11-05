package tmprl

import (
	"context"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/converter"
)

// Get reads durable snapshots and enriches active work with server heartbeat and
// retry diagnostics. Legacy runs use a bounded query, then last-known history,
// so polling remains available without a live worker.
func (j *Jobs[R, A]) Get(ctx context.Context, e Execution) (Run[R, A], error) {
	if err := e.Validate(); err != nil {
		return Run[R, A]{}, err
	}
	description, err := j.client.DescribeWorkflowExecution(ctx, e.WorkflowID, e.RunID)
	if err != nil {
		return Run[R, A]{}, err
	}
	info := description.GetWorkflowExecutionInfo()
	if info.GetType().GetName() != j.cfg.Names.Workflow {
		return Run[R, A]{}, ErrUnsupportedWorkflow
	}
	result, snapshot, err := j.runFromInfo(info)
	if err != nil {
		return Run[R, A]{}, err
	}
	if info.GetStatus() == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		if snapshot {
			err := j.enrichRun(&result, description)
			return result, err
		}
		queryCtx, cancel := context.WithTimeout(ctx, time.Second)
		var live Run[R, A]
		queryErr := Query(queryCtx, j.client, e, j.cfg.Names.Query, &live)
		cancel()
		if queryErr == nil {
			j.reconcileRun(&live, info)
			err := j.enrichRun(&live, description)
			return live, err
		}
		if ctx.Err() != nil {
			return Run[R, A]{}, ctx.Err()
		}
		j.cfg.Log(ctx, "job live state unavailable; using history", "workflow_id", e.WorkflowID, "run_id", e.RunID, "error", queryErr)
		result.StateStale = true
	}
	type scheduledActivity struct {
		name  string
		stage StageName
		step  string
	}
	activities := make(map[int64]scheduledActivity)
	diagnostics := make(map[string]StageState)
	it := j.client.GetWorkflowHistory(ctx, e.WorkflowID, e.RunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	for it.HasNext() {
		event, err := it.Next()
		if err != nil {
			return Run[R, A]{}, err
		}
		switch event.GetEventType() {
		case enumspb.EVENT_TYPE_WORKFLOW_PROPERTIES_MODIFIED:
			if payload := event.GetWorkflowPropertiesModifiedEventAttributes().GetUpsertedMemo().GetFields()[j.cfg.Names.Memo]; payload != nil {
				if err := converter.GetDefaultDataConverter().FromPayload(payload, &result); err != nil {
					return Run[R, A]{}, err
				}
			}
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED:
			var input JobInput[R]
			if err := converter.GetDefaultDataConverter().FromPayloads(event.GetWorkflowExecutionStartedEventAttributes().GetInput(), &input); err != nil {
				return Run[R, A]{}, err
			}
			result.Request = input.Request
			if result.Definition == nil {
				result.Definition = input.Job
			}
		case enumspb.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED:
			scheduled := event.GetActivityTaskScheduledEventAttributes()
			name := scheduled.GetActivityType().GetName()
			if !j.knownActivity(name) {
				continue
			}
			// Legacy stage requests share this JSON shape, but omit step.
			var request StepRequest[R, A]
			if err := converter.GetDefaultDataConverter().FromPayloads(scheduled.GetInput(), &request); err != nil {
				return Run[R, A]{}, err
			}
			activities[event.GetEventId()] = scheduledActivity{name, StageName(request.Stage), request.Step}
			if !snapshot {
				result.Stage, result.Step = StageName(request.Stage), request.Step
			}
		case enumspb.EVENT_TYPE_ACTIVITY_TASK_STARTED:
			started := event.GetActivityTaskStartedEventAttributes()
			if activity, ok := activities[started.GetScheduledEventId()]; ok {
				key := string(activity.stage)
				if activity.step != "" {
					key += "/" + activity.step
				}
				diagnostics[key] = StageState{Attempt: started.GetAttempt(), LastError: failureMessage(started.GetLastFailure())}
			}
		case enumspb.EVENT_TYPE_ACTIVITY_TASK_COMPLETED:
			completed := event.GetActivityTaskCompletedEventAttributes()
			activity, ok := activities[completed.GetScheduledEventId()]
			if !ok {
				continue
			}
			if j.resultActivity(activity.name) {
				var output StepResult[A]
				if err := converter.GetDefaultDataConverter().FromPayloads(completed.GetResult(), &output); err != nil {
					return Run[R, A]{}, err
				}
				result.Artifacts = output.Artifacts
			} else if err := converter.GetDefaultDataConverter().FromPayloads(completed.GetResult(), &result.Artifacts); err != nil {
				return Run[R, A]{}, err
			}
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED:
			var completed Run[R, A]
			if err := converter.GetDefaultDataConverter().FromPayloads(event.GetWorkflowExecutionCompletedEventAttributes().GetResult(), &completed); err != nil {
				return Run[R, A]{}, err
			}
			result = completed
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED:
			result.Error = failureMessage(event.GetWorkflowExecutionFailedEventAttributes().GetFailure())
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED:
			result.Error = "job terminated: " + event.GetWorkflowExecutionTerminatedEventAttributes().GetReason()
		case enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT:
			result.Error = "job workflow timed out"
		}
	}
	for i := range result.Stages {
		stage := &result.Stages[i]
		for k := range stage.Steps {
			step := &stage.Steps[k]
			if latest, ok := diagnostics[string(stage.Name)+"/"+step.Name]; ok {
				step.Attempt = latest.Attempt
				if step.LastError == "" {
					step.LastError = latest.LastError
				}
			}
		}
		if latest, ok := diagnostics[string(stage.Name)]; ok {
			stage.Attempt = latest.Attempt
			if stage.LastError == "" {
				stage.LastError = latest.LastError
			}
		}
	}
	j.reconcileRun(&result, info)
	if info.GetStatus() == enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING {
		if err := j.enrichRun(&result, description); err != nil {
			return Run[R, A]{}, err
		}
		result.StateStale = true
	}
	if ctx.Err() != nil {
		return Run[R, A]{}, ctx.Err()
	}
	return result, nil
}

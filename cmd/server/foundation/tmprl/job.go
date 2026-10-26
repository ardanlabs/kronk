package tmprl

import (
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/definition"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// QueuedJob constructs the initial snapshot for a validated submitted plan.
func QueuedJob[R, A any](input JobInput[R]) Run[R, A] {
	state := Run[R, A]{Request: input.Request, Definition: input.Job, Status: StatusQueued}
	for _, stage := range input.Job.Stages {
		entry := StageState{Name: Stage(stage.Name), Status: StatusQueued}
		for _, step := range stage.Steps {
			entry.Steps = append(entry.Steps, StepState{Name: step.Name, Status: StatusQueued})
		}
		state.Stages = append(state.Stages, entry)
	}
	return state
}

// RunJob owns one worker reservation across every stage and step. Its only
// execution plan is the submitted definition, never mutable deployment defaults.
func RunJob[R, A any](ctx workflow.Context, input JobInput[R], names Names) (result Run[R, A], err error) {
	if input.Job == nil {
		return Run[R, A]{}, temporal.NewNonRetryableApplicationError("missing job definition", "invalid-job", nil)
	}
	if err := input.Job.Validate(); err != nil {
		return Run[R, A]{}, temporal.NewNonRetryableApplicationError(err.Error(), "invalid-job", nil)
	}
	info := workflow.GetInfo(ctx)
	state := QueuedJob[R, A](input)
	state.Execution = Execution{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID}
	state.StartedAt = workflow.Now(ctx)
	log := workflow.GetLogger(ctx)
	log.Info("job started", "job", input.Job.Name, "stages", len(state.Stages), "reservation_timeout", input.Job.ReservationTimeout)
	defer func() {
		state.ClosedAt = workflow.Now(ctx)
		if err != nil {
			state.Status = StatusFailed
			if ctx.Err() != nil {
				state.Status = StatusStopped
			}
			state.Error = err.Error()
			CloseJobStages(&state)
		}
		cleanup, cancel := workflow.NewDisconnectedContext(ctx)
		defer cancel()
		if saveErr := PersistRun(cleanup, names.Memo, &state); saveErr != nil {
			log.Error("persisting terminal job state", "error", saveErr)
			if err == nil {
				err = saveErr
			}
		}
		log.Info("job closed", "status", state.Status, "stage", state.Stage, "step", state.Step, "error", err)
		result = state
	}()
	if err := workflow.SetQueryHandler(ctx, names.Query, func() (Run[R, A], error) { return state, nil }); err != nil {
		return Run[R, A]{}, err
	}
	work, cancelWork := workflow.WithCancel(ctx)
	defer cancelWork()
	paused, reserved := false, false
	var controlErr error
	controls := workflow.GetSignalChannel(work, names.Control)
	workflow.Go(work, func(ctx workflow.Context) {
		for {
			var command string
			if !controls.Receive(ctx, &command) {
				return
			}
			switch command {
			case "pause":
				if paused {
					continue
				}
				paused = true
				state.Status = StatusPauseRequested
			case "resume":
				if !paused {
					continue
				}
				paused = false
				switch {
				case reserved:
					state.Status = StatusRunning
				case state.SessionAttempt > 1:
					state.Status = StatusRecovering
				default:
					state.Status = StatusQueued
				}
			default:
				log.Warn("job control ignored", "command", command)
				continue
			}
			log.Info("job control applied", "command", command, "status", state.Status, "stage", state.Stage, "step", state.Step)
			if controlErr = PersistRun(ctx, names.Memo, &state); controlErr != nil {
				cancelWork()
				return
			}
		}
	})
	stageIndex, stepIndex := 0, 0
	for attempt := 1; attempt <= input.Job.SessionAttempts; attempt++ {
		state.SessionAttempt = attempt
		if attempt > 1 {
			state.Status = StatusRecovering
		}
		state.WorkerHost, state.ReservationDeadline = "", time.Time{}
		if err := PersistRun(work, names.Memo, &state); err != nil {
			return Run[R, A]{}, err
		}
		log.Info("job waiting for worker", "session_attempt", attempt)
		session, err := workflow.CreateSession(work, &workflow.SessionOptions{
			CreationTimeout: input.Job.QueueTimeout, ExecutionTimeout: input.Job.ReservationTimeout + input.Job.QueueTimeout, HeartbeatTimeout: 5 * time.Second,
		})
		if err != nil {
			if controlErr != nil {
				return Run[R, A]{}, controlErr
			}
			return Run[R, A]{}, fmt.Errorf("reserving job worker: %w", err)
		}
		reserved = true
		si := workflow.GetSessionInfo(session)
		state.WorkerHost, state.ReservationDeadline = si.HostName, workflow.Now(ctx).Add(input.Job.ReservationTimeout)
		log.Info("job worker reserved", "session_id", si.SessionID, "worker_host", si.HostName, "session_attempt", attempt, "reservation_deadline", state.ReservationDeadline)
		execution, cancelExecution := workflow.WithCancel(session)
		timer, cancelTimer := workflow.WithCancel(work)
		expired := false
		workflow.Go(timer, func(ctx workflow.Context) {
			if workflow.NewTimer(ctx, input.Job.ReservationTimeout).Get(ctx, nil) == nil {
				expired = true
				cancelExecution()
			}
		})
		err = runJobStages(execution, input, names, &state, &paused, &stageIndex, &stepIndex)
		cancelTimer()
		cancelExecution()
		reserved = false
		failed := si.SessionState == workflow.SessionStateFailed
		workflow.CompleteSession(session)
		log.Info("job worker released", "session_id", si.SessionID, "session_failed", failed)
		if controlErr != nil {
			return Run[R, A]{}, controlErr
		}
		if expired || !workflow.Now(ctx).Before(state.ReservationDeadline) {
			return Run[R, A]{}, temporal.NewNonRetryableApplicationError("job worker reservation expired; paused time counts toward the deadline", "reservation-expired", nil)
		}
		if err == nil {
			if state.Status == StatusRunning || state.Status == StatusPauseRequested {
				state.Status = StatusCompleted
			}
			state.Stage, state.Step = "", ""
			return state, nil
		}
		if ctx.Err() != nil || !failed || attempt == input.Job.SessionAttempts {
			return Run[R, A]{}, err
		}
		state.Error = "worker lost; recovering unfinished step"
		stage := &state.Stages[stageIndex]
		stage.Status = StatusRecovering
		stage.Steps[stepIndex].Status, stage.Steps[stepIndex].LastError = StatusRecovering, state.Error
		log.Warn("job worker lost", "stage", state.Stage, "step", state.Step, "session_attempt", attempt)
	}
	return Run[R, A]{}, fmt.Errorf("job worker recovery exhausted")
}

func awaitJobResume[R, A any](ctx workflow.Context, names Names, state *Run[R, A], paused *bool) error {
	if !*paused {
		return nil
	}
	state.Status, state.Step = StatusPaused, ""
	if err := PersistRun(ctx, names.Memo, state); err != nil {
		return err
	}
	workflow.GetLogger(ctx).Info("job paused; worker remains reserved", "stage", state.Stage, "reservation_deadline", state.ReservationDeadline)
	return workflow.Await(ctx, func() bool { return !*paused })
}

func runJobStages[R, A any](ctx workflow.Context, input JobInput[R], names Names, state *Run[R, A], paused *bool, stageIndex, stepIndex *int) error {
	for *stageIndex < len(input.Job.Stages) {
		plan := input.Job.Stages[*stageIndex]
		stage := &state.Stages[*stageIndex]
		state.Stage = stage.Name
		if stage.StartedAt.IsZero() {
			if err := awaitJobResume(ctx, names, state, paused); err != nil {
				return err
			}
			stage.StartedAt = workflow.Now(ctx)
			stage.Deadline = stage.StartedAt.Add(plan.Timeout)
		}
		stage.Status = StatusRunning
		stageCtx, cancel := workflow.WithCancel(ctx)
		timerCtx, cancelTimer := workflow.WithCancel(ctx)
		remaining := stage.Deadline.Sub(workflow.Now(ctx))
		expired := remaining <= 0
		if !expired {
			workflow.Go(timerCtx, func(ctx workflow.Context) {
				if workflow.NewTimer(ctx, remaining).Get(ctx, nil) == nil {
					expired = true
					cancel()
				}
			})
		}
		workflow.GetLogger(ctx).Info("job stage executing", "stage", stage.Name, "deadline", stage.Deadline, "next_step", *stepIndex)
		var err error
		if !expired {
			err = runJobSteps(stageCtx, plan, names, state, stage, paused, stepIndex)
		}
		cancelTimer()
		cancel()
		if expired || !workflow.Now(ctx).Before(stage.Deadline) {
			return temporal.NewNonRetryableApplicationError(fmt.Sprintf("stage %s exceeded its total deadline, including retries and pauses", stage.Name), "stage-expired", nil)
		}
		if err != nil {
			return err
		}
		stage.Status, stage.ClosedAt = StatusCompleted, workflow.Now(ctx)
		workflow.GetLogger(ctx).Info("job stage committed", "stage", stage.Name, "elapsed", stage.ClosedAt.Sub(stage.StartedAt))
		(*stageIndex)++
		*stepIndex = 0
		if state.Status.Terminal() {
			return nil
		}
		if err := PersistRun(ctx, names.Memo, state); err != nil {
			return err
		}
	}
	return nil
}

func runJobSteps[R, A any](ctx workflow.Context, plan definition.Stage, names Names, state *Run[R, A], stage *StageState, paused *bool, index *int) error {
	for *index < len(plan.Steps) {
		if err := awaitJobResume(ctx, names, state, paused); err != nil {
			return err
		}
		policy := plan.Steps[*index]
		step := &stage.Steps[*index]
		state.Status, state.Step, state.Error = StatusRunning, policy.Name, ""
		step.Status = StatusRunning
		if step.StartedAt.IsZero() {
			step.StartedAt = workflow.Now(ctx)
		}
		step.Attempt = 1
		if err := PersistRun(ctx, names.Memo, state); err != nil {
			return err
		}
		stepCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: policy.Timeout, HeartbeatTimeout: policy.HeartbeatTimeout, WaitForCancellation: true,
			RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: policy.Retry.MaximumAttempts, InitialInterval: policy.Retry.InitialInterval, BackoffCoefficient: policy.Retry.BackoffCoefficient, MaximumInterval: policy.Retry.MaximumInterval},
		})
		request := StepRequest[R, A]{Execution: state.Execution, Key: state.RunID + "/" + plan.Name + "/" + policy.Name, Stage: plan.Name, Step: policy.Name, Request: state.Request, Artifacts: state.Artifacts}
		var output StepResult[A]
		if err := workflow.ExecuteActivity(stepCtx, names.Activity, request).Get(ctx, &output); err != nil {
			return err
		}
		state.Artifacts = output.Artifacts
		step.Status, step.Attempt, step.ClosedAt = StatusCompleted, output.Attempt, workflow.Now(ctx)
		workflow.GetLogger(ctx).Info("job step committed", "stage", plan.Name, "step", policy.Name, "attempt", output.Attempt, "elapsed", step.ClosedAt.Sub(step.StartedAt), "outcome", output.Status)
		(*index)++
		if output.Status != "" {
			state.Status = output.Status
			return nil
		}
		if err := PersistRun(ctx, names.Memo, state); err != nil {
			return err
		}
	}
	return nil
}

// CloseJobStages marks only unfinished active work with the terminal job state.
func CloseJobStages[R, A any](state *Run[R, A]) {
	for i := range state.Stages {
		stage := &state.Stages[i]
		if stage.Status != StatusRunning && stage.Status != StatusRecovering {
			continue
		}
		stage.Status, stage.ClosedAt, stage.LastError = state.Status, state.ClosedAt, state.Error
		for j := range stage.Steps {
			step := &stage.Steps[j]
			if step.Status == StatusRunning || step.Status == StatusRecovering {
				step.Status, step.ClosedAt, step.LastError = state.Status, state.ClosedAt, state.Error
			}
		}
	}
}

// PersistRun stores the last-known snapshot independently of live worker queries.
func PersistRun[R, A any](ctx workflow.Context, memo string, state *Run[R, A]) error {
	state.UpdatedAt = workflow.Now(ctx)
	return workflow.UpsertMemo(ctx, map[string]any{memo: *state})
}

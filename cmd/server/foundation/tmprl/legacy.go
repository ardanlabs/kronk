package tmprl

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// LegacyInput preserves the flat-stage execution plan used before job definitions.
type LegacyInput[R any] struct {
	Request      R             `json:"request"`
	Stages       []Stage       `json:"stages,omitzero"`
	StageTimeout time.Duration `json:"stage_timeout"`
	JobTimeout   time.Duration `json:"job_timeout,omitzero"`
	QueueTimeout time.Duration `json:"queue_timeout,omitzero"`
}

// LegacyStageRequest preserves the historical stage payload without a step field.
type LegacyStageRequest[R, A any] struct {
	Execution
	Key       string `json:"key"`
	Stage     Stage  `json:"stage"`
	Request   R      `json:"request"`
	Artifacts A      `json:"artifacts"`
}

// QueuedLegacyRun constructs the initial snapshot for a flat-stage plan.
func QueuedLegacyRun[R, A any](input LegacyInput[R]) Run[R, A] {
	state := Run[R, A]{Request: input.Request, Status: StatusQueued}
	for _, stage := range input.Stages {
		state.Stages = append(state.Stages, StageState{Name: stage, Status: StatusQueued})
	}
	return state
}

// RunLegacyStages preserves flat-stage session histories, including command
// ordering, activity payloads, retries, timers and control signals. The application
// supplies the historical protocol names and invalid-plan failure type.
func RunLegacyStages[R, A any](ctx workflow.Context, input LegacyInput[R], names Names, invalidPlanType string) (result Run[R, A], err error) {
	if len(input.Stages) == 0 || input.StageTimeout <= 0 || input.JobTimeout <= 0 || input.QueueTimeout <= 0 {
		return Run[R, A]{}, temporal.NewNonRetryableApplicationError("missing job execution plan", invalidPlanType, nil)
	}
	info := workflow.GetInfo(ctx)
	state := QueuedLegacyRun[R, A](input)
	state.Execution = Execution{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID}
	state.StartedAt = workflow.Now(ctx)
	log := workflow.GetLogger(ctx)
	log.Info("job started", "stages", input.Stages, "stage_timeout", input.StageTimeout, "reservation_timeout", input.JobTimeout)
	defer func() {
		if err != nil {
			state.Status = StatusFailed
			if ctx.Err() != nil {
				state.Status = StatusStopped
			}
			state.Error = err.Error()
			for i := range state.Stages {
				if state.Stages[i].Name == state.Stage && state.Stages[i].Status != StatusCompleted {
					state.Stages[i].Status = state.Status
					state.Stages[i].LastError = state.Error
					state.Stages[i].ClosedAt = workflow.Now(ctx)
				}
			}
			log.Error("job closed", "status", state.Status, "stage", state.Stage, "error", err)
		} else {
			log.Info("job closed", "status", state.Status)
		}
		if state.Status == StatusStopped {
			state.Stage = ""
		}
		state.ClosedAt = workflow.Now(ctx)
		cleanup, cancel := workflow.NewDisconnectedContext(ctx)
		defer cancel()
		if saveErr := PersistRun(cleanup, names.Memo, &state); saveErr != nil {
			// Preserve Temporal's original failure type, especially cancellation.
			log.Error("persisting terminal job state", "error", saveErr)
			if err == nil {
				err = saveErr
			}
		}
		result = state
	}()
	if err := workflow.SetQueryHandler(ctx, names.Query, func() (Run[R, A], error) { return state, nil }); err != nil {
		return Run[R, A]{}, err
	}
	if err := PersistRun(ctx, names.Memo, &state); err != nil {
		return Run[R, A]{}, err
	}
	work, cancelWork := workflow.WithCancel(ctx)
	defer cancelWork()
	paused, reserved := false, false
	var controlErr error
	controls := workflow.GetSignalChannel(ctx, names.Control)
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
			log.Info("job control applied", "command", command, "status", state.Status, "stage", state.Stage)
			if controlErr = PersistRun(ctx, names.Memo, &state); controlErr != nil {
				cancelWork()
				return
			}
		}
	})
	work = workflow.WithActivityOptions(work, workflow.ActivityOptions{
		StartToCloseTimeout: input.StageTimeout, HeartbeatTimeout: 5 * time.Second,
		WaitForCancellation: true,
		RetryPolicy:         &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3},
	})
	index := 0
	for attempt := 1; attempt <= 3; attempt++ {
		state.SessionAttempt = attempt
		if attempt > 1 {
			state.Status = StatusRecovering
		} else if !paused {
			state.Status = StatusQueued
		}
		state.WorkerHost = ""
		state.ReservationDeadline = time.Time{}
		if err := PersistRun(work, names.Memo, &state); err != nil {
			return Run[R, A]{}, err
		}
		log.Info("job waiting for worker", "session_attempt", attempt, "next_stage", input.Stages[index])
		// The application timer below owns the visible reservation deadline.
		// Give the SDK watchdog room for the creation handshake so its timeout
		// cannot be mistaken for worker loss just before that deadline.
		session, err := workflow.CreateSession(work, &workflow.SessionOptions{
			CreationTimeout: input.QueueTimeout, ExecutionTimeout: input.JobTimeout + input.QueueTimeout, HeartbeatTimeout: 5 * time.Second,
		})
		if err != nil {
			if controlErr != nil {
				return Run[R, A]{}, controlErr
			}
			return Run[R, A]{}, fmt.Errorf("reserving job worker: %w", err)
		}
		reserved = true
		si := workflow.GetSessionInfo(session)
		state.WorkerHost = si.HostName
		state.ReservationDeadline = workflow.Now(ctx).Add(input.JobTimeout)
		log.Info("job worker reserved", "session_id", si.SessionID, "worker_host", si.HostName, "session_attempt", attempt, "reservation_deadline", state.ReservationDeadline)
		stageCtx, cancelStage := workflow.WithCancel(session)
		timerCtx, cancelTimer := workflow.WithCancel(work)
		expired := false
		workflow.Go(timerCtx, func(ctx workflow.Context) {
			if workflow.NewTimer(ctx, input.JobTimeout).Get(ctx, nil) == nil {
				expired = true
				cancelStage()
			}
		})
		err = runLegacyStages(stageCtx, input, names, &state, &paused, &index)
		cancelTimer()
		cancelStage()
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
			state.Stage = ""
			return state, nil
		}
		if ctx.Err() != nil || !failed || attempt == 3 {
			return Run[R, A]{}, err
		}
		state.Error = "worker lost; recovering unfinished stage"
		if state.Stage != "" {
			state.Stages[index].Status = StatusRecovering
			state.Stages[index].LastError = state.Error
		}
		log.Warn("job worker lost; recovering unfinished stage", "stage", state.Stage, "session_attempt", attempt)
	}
	return Run[R, A]{}, fmt.Errorf("job session recovery exhausted")
}

func runLegacyStages[R, A any](ctx workflow.Context, input LegacyInput[R], names Names, state *Run[R, A], paused *bool, index *int) error {
	for *index < len(input.Stages) {
		if *paused {
			state.Status = StatusPaused
			state.Stage = ""
			if err := PersistRun(ctx, names.Memo, state); err != nil {
				return err
			}
			workflow.GetLogger(ctx).Info("job paused; worker remains reserved", "reservation_deadline", state.ReservationDeadline)
			if err := workflow.Await(ctx, func() bool { return !*paused }); err != nil {
				return err
			}
		}
		state.Status = StatusRunning
		state.Stage = input.Stages[*index]
		state.Error = ""
		stage := &state.Stages[*index]
		stage.Status = StatusRunning
		stage.StartedAt = workflow.Now(ctx)
		stage.Attempt = 1
		if err := PersistRun(ctx, names.Memo, state); err != nil {
			return err
		}
		request := LegacyStageRequest[R, A]{Execution: state.Execution, Key: state.RunID + "/" + string(state.Stage), Stage: state.Stage, Request: input.Request, Artifacts: state.Artifacts}
		var output StepResult[A]
		if err := workflow.ExecuteActivity(ctx, names.Activity, request).Get(ctx, &output); err != nil {
			return err
		}
		state.Artifacts = output.Artifacts
		stage.Attempt = output.Attempt
		stage.Status = StatusCompleted
		stage.ClosedAt = workflow.Now(ctx)
		(*index)++
		workflow.GetLogger(ctx).Info("job stage committed", "stage", stage.Name, "elapsed", stage.ClosedAt.Sub(stage.StartedAt), "outcome", output.Status)
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

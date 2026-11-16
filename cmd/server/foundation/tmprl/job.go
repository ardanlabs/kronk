package tmprl

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const sessionHeartbeatTimeout = 5 * time.Second

// errReleaseWorker ends a reservation without failing the job or consuming a
// recovery attempt: an idle paused/review wait, or a step suspended for host
// shutdown. Only opted-in control executions (see idleRelease) return it.
var errReleaseWorker = errors.New("release idle job worker")

// QueuedJob constructs the initial snapshot for a validated submitted plan.
func QueuedJob[R, A any](input JobInput[R]) Run[R, A] {
	state := Run[R, A]{Request: input.Request, Definition: input.Job, Status: StatusQueued, ControlStage: input.ControlStage}
	for _, stage := range input.Job.Stages {
		entry := StageState{Name: StageName(stage.Name), Status: StatusQueued}
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
		state.Checkpoint = nil
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
	if input.ControlStage != "" {
		if err := workflow.SetUpdateHandler(work, names.Control+".ordered", func(ctx workflow.Context, request ControlRequest) (ControlReceipt, error) {
			if request.Validate() != nil || ctx.Err() != nil || state.Status.Terminal() ||
				state.ControlRevision != request.ExpectedRevision ||
				state.Stage != "" && string(state.Stage) != input.ControlStage ||
				(request.Action == "pause") == paused {
				return ControlReceipt{}, temporal.NewNonRetryableApplicationError(ErrControlConflict.Error(), "control-conflict", nil)
			}
			previous, wasPaused := state, paused
			paused = request.Action == "pause"
			state.ControlRevision++
			state.ControlAction = request.Action
			switch {
			case state.Checkpoint != nil:
				state.Status = StatusWaiting
				if paused {
					state.ControlAcknowledgedRevision = state.ControlRevision
					state.ControlCheckpoint = state.Checkpoint.ID
				}
			case paused:
				state.Status = StatusPauseRequested
			case reserved:
				state.Status = StatusRunning
			case state.SessionAttempt > 1:
				state.Status = StatusRecovering
			default:
				state.Status = StatusQueued
			}
			if err := PersistRun(ctx, names.Memo, &state); err != nil {
				state, paused = previous, wasPaused
				return ControlReceipt{}, err
			}
			return ControlReceipt{ControlRequest: request, Revision: state.ControlRevision}, nil
		}); err != nil {
			return Run[R, A]{}, err
		}
	}
	checkpointEnabled := false
	for _, stage := range input.Job.Stages {
		checkpointEnabled = checkpointEnabled || stage.WaitAfter != ""
		for _, step := range stage.Steps {
			checkpointEnabled = checkpointEnabled || step.WaitAfter != ""
		}
	}
	if checkpointEnabled {
		if err := workflow.SetUpdateHandler(work, names.Control+".decide", func(ctx workflow.Context, decision Decision) (Decision, error) {
			return acceptCheckpointDecision(ctx, names, &state, &paused, decision)
		}); err != nil {
			return Run[R, A]{}, err
		}
	}
	controls := workflow.GetSignalChannel(work, names.Control)
	workflow.Go(work, func(ctx workflow.Context) {
		for {
			var command string
			if !controls.Receive(ctx, &command) {
				return
			}
			if input.ControlStage != "" {
				log.Warn("unordered control ignored; use the ordered update protocol")
				continue
			}
			switch command {
			case "pause":
				if paused {
					continue
				}
				paused = true
				state.Status = StatusPauseRequested
				if state.Checkpoint != nil {
					state.Status = StatusWaiting
				}
			case "resume":
				if !paused {
					continue
				}
				paused = false
				switch {
				case state.Checkpoint != nil:
					state.Status = StatusWaiting
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
	// Ordered-control executions release their worker while paused, in review,
	// or after host shutdown. Only the first reservation is bounded by the queue
	// timeout, and stage budgets count active time. Older histories replay with
	// the reservation behavior they recorded.
	idle := input.ControlStage != "" && workflow.GetVersion(work, "preparation-idle-release", workflow.DefaultVersion, 1) == 1
	reservedOnce := false
	stageIndex, stepIndex := 0, 0
	for attempt := 1; attempt <= input.Job.SessionAttempts; {
		state.SessionAttempt = attempt
		if attempt > 1 {
			state.Status = StatusRecovering
		}
		state.WorkerHost, state.ReservationDeadline = "", time.Time{}
		if err := PersistRun(work, names.Memo, &state); err != nil {
			return Run[R, A]{}, err
		}
		log.Info("job waiting for worker", "session_attempt", attempt)
		queueTimeout := input.Job.QueueTimeout
		if idle && reservedOnce {
			queueTimeout = 0 // A started run waits for its worker without a limit.
		}
		session, err := workflow.CreateSession(work, &workflow.SessionOptions{
			CreationTimeout: queueTimeout, ExecutionTimeout: input.Job.ReservationTimeout + input.Job.QueueTimeout, HeartbeatTimeout: sessionHeartbeatTimeout,
		})
		if err != nil {
			if controlErr != nil {
				return Run[R, A]{}, controlErr
			}
			return Run[R, A]{}, fmt.Errorf("reserving job worker: %w", err)
		}
		reserved, reservedOnce = true, true
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
		err = runJobStages(execution, input, names, &state, &paused, idle, &stageIndex, &stepIndex)
		// A crashed worker's step heartbeat can expire before its session
		// heartbeat. Allow session failure detection before releasing it; closing
		// the session immediately would suppress recovery of unfinished work.
		// Version the extra timer so pre-change histories remain replayable.
		if timeout, ok := errors.AsType[*temporal.TimeoutError](err); ok && timeout.TimeoutType() == enumspb.TIMEOUT_TYPE_HEARTBEAT && si.SessionState == workflow.SessionStateOpen &&
			workflow.GetVersion(work, "step-heartbeat-session-loss", workflow.DefaultVersion, 1) == 1 {
			log.Warn("job step heartbeat lost; checking worker session", "stage", state.Stage, "step", state.Step)
			if _, waitErr := workflow.AwaitWithTimeout(execution, sessionHeartbeatTimeout, func() bool {
				return si.SessionState == workflow.SessionStateFailed
			}); waitErr != nil && si.SessionState != workflow.SessionStateFailed {
				err = waitErr
			}
		}
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
		if errors.Is(err, errReleaseWorker) {
			if ctx.Err() != nil {
				// Stop raced the release: close as stopped with the Stop's error.
				return Run[R, A]{}, ctx.Err()
			}
			// Idle or drained: no work is in flight, so this does not consume a
			// recovery attempt. Wait without a worker or a deadline.
			state.WorkerHost, state.ReservationDeadline = "", time.Time{}
			if stageIndex < len(state.Stages) {
				state.Stages[stageIndex].Deadline = time.Time{}
			}
			if err := PersistRun(work, names.Memo, &state); err != nil {
				return Run[R, A]{}, err
			}
			log.Info("job worker released while idle", "status", state.Status, "stage", state.Stage, "step", state.Step)
			if err := workflow.Await(work, func() bool { return !paused && state.Checkpoint == nil }); err != nil {
				return Run[R, A]{}, err
			}
			continue
		}
		if ctx.Err() != nil || !failed || attempt == input.Job.SessionAttempts {
			return Run[R, A]{}, err
		}
		state.Error = "worker lost; recovering unfinished step"
		stage := &state.Stages[stageIndex]
		if stage.Status != StatusCompleted {
			stage.Status = StatusRecovering
		}
		if stepIndex < len(stage.Steps) && stage.Steps[stepIndex].Status != StatusCompleted {
			stage.Steps[stepIndex].Status, stage.Steps[stepIndex].LastError = StatusRecovering, state.Error
		}
		log.Warn("job worker lost", "stage", state.Stage, "step", state.Step, "session_attempt", attempt)
		attempt++
	}
	return Run[R, A]{}, fmt.Errorf("job worker recovery exhausted")
}

func acceptCheckpointDecision[R, A any](ctx workflow.Context, names Names, state *Run[R, A], paused *bool, decision Decision) (Decision, error) {
	if err := decision.Validate(); err != nil {
		return Decision{}, err
	}
	for _, accepted := range state.Decisions {
		if accepted.CheckpointID == decision.CheckpointID {
			if accepted.SubmissionID == decision.SubmissionID && bytes.Equal(accepted.Payload, decision.Payload) {
				return accepted, nil
			}
			return Decision{}, fmt.Errorf("%w: checkpoint decision conflicts with its accepted receipt", ErrInvalidArgument)
		}
	}
	if state.Checkpoint == nil || state.Checkpoint.ID != decision.CheckpointID || state.Status.Terminal() {
		return Decision{}, fmt.Errorf("%w: decision does not match a pending checkpoint", ErrInvalidArgument)
	}
	previous := *state
	decision.Payload = bytes.Clone(decision.Payload)
	state.Decisions = append(slices.Clone(state.Decisions), decision)
	state.Checkpoint = nil
	state.Status = StatusRunning
	if *paused {
		state.Status = StatusPaused
	}
	if err := PersistRun(ctx, names.Memo, state); err != nil {
		*state = previous
		return Decision{}, err
	}
	return decision, nil
}

func awaitCheckpoint[R, A any](ctx workflow.Context, names Names, state *Run[R, A], paused *bool, idle bool, stage, step, name string) error {
	id := state.RunID + "/" + stage + "/" + step + "/" + name
	for _, accepted := range state.Decisions {
		if accepted.CheckpointID == id {
			return awaitJobResume(ctx, names, state, paused, idle)
		}
	}
	state.Checkpoint = &Checkpoint{ID: id, Name: name, Stage: stage, Step: step}
	state.Status, state.Step, state.Error = StatusWaiting, step, ""
	if err := PersistRun(ctx, names.Memo, state); err != nil {
		return err
	}
	if idle {
		// Review can take any amount of time; wait for it without a worker.
		// The accepted decision is found above after the next reservation.
		return errReleaseWorker
	}
	if err := workflow.Await(ctx, func() bool { return state.Checkpoint == nil }); err != nil {
		return err
	}
	return awaitJobResume(ctx, names, state, paused, idle)
}

// awaitJobResume acknowledges Pause. Idle-release executions return
// errReleaseWorker so the caller waits for Continue without a worker.
func awaitJobResume[R, A any](ctx workflow.Context, names Names, state *Run[R, A], paused *bool, idle bool) error {
	if !*paused {
		return nil
	}
	state.Status = StatusPaused
	if state.ControlStage == "" {
		state.Step = ""
	} else {
		state.ControlAcknowledgedRevision = state.ControlRevision
		if state.ControlCheckpoint == "" {
			state.ControlCheckpoint = string(state.Stage) + "/" + state.Step + ":step-boundary"
		}
	}
	if err := PersistRun(ctx, names.Memo, state); err != nil {
		return err
	}
	if idle {
		return errReleaseWorker
	}
	workflow.GetLogger(ctx).Info("job paused; worker remains reserved", "stage", state.Stage, "reservation_deadline", state.ReservationDeadline)
	return workflow.Await(ctx, func() bool { return !*paused })
}

func runJobStages[R, A any](ctx workflow.Context, input JobInput[R], names Names, state *Run[R, A], paused *bool, idle bool, stageIndex, stepIndex *int) error {
	for *stageIndex < len(input.Job.Stages) {
		plan := input.Job.Stages[*stageIndex]
		stage := &state.Stages[*stageIndex]
		state.Stage = stage.Name
		// A completed stage awaiting authorization must not restart its steps or
		// its deadline when the worker session is replaced.
		if plan.WaitAfter != "" && stage.Status == StatusCompleted {
			if err := awaitCheckpoint(ctx, names, state, paused, idle, plan.Name, "", plan.WaitAfter); err != nil {
				return err
			}
			(*stageIndex)++
			*stepIndex = 0
			continue
		}
		if stage.StartedAt.IsZero() {
			if err := awaitJobResume(ctx, names, state, paused, idle); err != nil {
				return err
			}
			stage.StartedAt = workflow.Now(ctx)
			stage.Deadline = stage.StartedAt.Add(plan.Timeout)
		}
		resumed := workflow.Now(ctx)
		if idle {
			// Count only active time: project the remaining budget from now.
			stage.Deadline = resumed.Add(plan.Timeout - stage.ActiveElapsed)
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
			err = runJobSteps(stageCtx, plan, names, state, stage, paused, idle, stepIndex)
		}
		cancelTimer()
		cancel()
		if idle {
			// Charge this reservation's active time on every exit, including a
			// crash, so idle waits between reservations are the only uncounted time.
			stage.ActiveElapsed += workflow.Now(ctx).Sub(resumed)
			if errors.Is(err, errReleaseWorker) && !expired {
				return err
			}
		}
		if expired || !workflow.Now(ctx).Before(stage.Deadline) {
			return temporal.NewNonRetryableApplicationError(fmt.Sprintf("stage %s exceeded its total deadline, including retries and pauses", stage.Name), "stage-expired", nil)
		}
		if err != nil {
			return err
		}
		stage.Status, stage.ClosedAt = StatusCompleted, workflow.Now(ctx)
		workflow.GetLogger(ctx).Info("job stage committed", "stage", stage.Name, "elapsed", stage.ClosedAt.Sub(stage.StartedAt))
		if plan.WaitAfter != "" && !state.Status.Terminal() {
			if err := awaitCheckpoint(ctx, names, state, paused, idle, plan.Name, "", plan.WaitAfter); err != nil {
				return err
			}
		}
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

func runJobSteps[R, A any](ctx workflow.Context, plan Stage, names Names, state *Run[R, A], stage *StageState, paused *bool, idle bool, index *int) error {
	for *index < len(plan.Steps) {
		policy := plan.Steps[*index]
		step := &stage.Steps[*index]
		if policy.WaitAfter != "" && step.Status == StatusCompleted {
			if err := awaitCheckpoint(ctx, names, state, paused, idle, plan.Name, policy.Name, policy.WaitAfter); err != nil {
				return err
			}
			(*index)++
			continue
		}
		if err := awaitJobResume(ctx, names, state, paused, idle); err != nil {
			return err
		}
		state.Status, state.Step, state.Error = StatusRunning, policy.Name, ""
		step.Status = StatusRunning
		if step.StartedAt.IsZero() {
			step.StartedAt = workflow.Now(ctx)
		}
		safePoints := state.ControlStage == plan.Name && workflow.GetVersion(ctx, "preparation-safe-points", workflow.DefaultVersion, 1) == 1
		if !safePoints || step.Attempt == 0 {
			step.Attempt = 1
		}
		resumeCheckpoint := ""
		if safePoints && strings.HasPrefix(state.ControlCheckpoint, plan.Name+"/"+policy.Name+"/") {
			resumeCheckpoint = state.ControlCheckpoint
			// The dispatched request retains the expected checkpoint. A later
			// session recovery must not compare newer work with this older digest.
			state.ControlCheckpoint = ""
		}
		if err := PersistRun(ctx, names.Memo, state); err != nil {
			return err
		}
		maximumAttempts := policy.Retry.MaximumAttempts
		if safePoints {
			maximumAttempts -= step.Attempt - 1
		}
		stepCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: policy.Timeout, HeartbeatTimeout: policy.HeartbeatTimeout, WaitForCancellation: true,
			RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: maximumAttempts, InitialInterval: policy.Retry.InitialInterval, BackoffCoefficient: policy.Retry.BackoffCoefficient, MaximumInterval: policy.Retry.MaximumInterval},
		})
		request := StepRequest[R, A]{Execution: state.Execution, Key: state.RunID + "/" + plan.Name + "/" + policy.Name, Stage: plan.Name, Step: policy.Name, Request: state.Request, Artifacts: state.Artifacts, Decisions: state.Decisions, SafePoints: safePoints, ResumeCheckpoint: resumeCheckpoint}
		var output StepResult[A]
		if err := workflow.ExecuteActivity(stepCtx, names.Activity, request).Get(ctx, &output); err != nil {
			if suspended, ok := errors.AsType[*temporal.ApplicationError](err); safePoints && ctx.Err() == nil && ok && (suspended.Type() == "step-suspended" || idle && suspended.Type() == "step-drained") {
				var reference string
				var attempt int32
				if detailsErr := suspended.Details(&reference, &attempt); detailsErr != nil || reference == "" || attempt < 1 || attempt > maximumAttempts {
					return err
				}
				// A voluntary suspension is not a failure. Preserve all preceding
				// failed attempts rather than refreshing their budget on Continue.
				step.Attempt += attempt - 1
				state.ControlCheckpoint = reference
				if suspended.Type() == "step-drained" {
					// Host shutdown. The next worker continues this saved
					// checkpoint without a user Continue or a recovery attempt.
					step.Status, step.LastError = StatusRecovering, "worker shut down at a saved checkpoint"
					state.Status = StatusRecovering
					return errReleaseWorker
				}
				if err := awaitJobResume(ctx, names, state, paused, idle); err != nil {
					return err
				}
				continue // Same step, no whole-step commit and no review bypass.
			}
			return err
		}
		state.Artifacts = output.Artifacts
		attempt := output.Attempt
		if safePoints {
			attempt += step.Attempt - 1
		}
		step.Status, step.Attempt, step.ClosedAt = StatusCompleted, attempt, workflow.Now(ctx)
		workflow.GetLogger(ctx).Info("job step committed", "stage", plan.Name, "step", policy.Name, "attempt", output.Attempt, "elapsed", step.ClosedAt.Sub(step.StartedAt), "outcome", output.Status)
		if output.Status != "" {
			(*index)++
			state.Status = output.Status
			return nil
		}
		if policy.WaitAfter != "" {
			if err := awaitCheckpoint(ctx, names, state, paused, idle, plan.Name, policy.Name, policy.WaitAfter); err != nil {
				return err
			}
		}
		(*index)++
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

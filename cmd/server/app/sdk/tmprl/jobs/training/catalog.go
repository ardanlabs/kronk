package training

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/api/serviceerror"
)

// CatalogObservation is a bounded projection, not a checkpoint or an execution
// input. Preview text, decisions, source contents and private paths stay out of SQL.
type CatalogObservation struct {
	State          State `json:"state"`
	Prepared       bool  `json:"prepared"`
	RecordCount    int   `json:"record_count"`
	TrainingReady  bool  `json:"training_ready"`
	SkippedRecords int   `json:"skipped_records,omitzero"`
	NotCompared    bool  `json:"not_compared,omitzero"`
}

type submitCommand struct {
	InputSHA256 string                  `json:"input_sha256"`
	Submission  tmprl.JobInput[Request] `json:"submission"`
}

// Namespace identifies the client's configured namespace; it is never inferred
// from a workflow ID or supplied by an HTTP caller.
func (j *Job) Namespace() string { return j.cfg.Namespace }

func jobIdentity(namespace, workflowID string) string {
	return fmt.Sprintf("job-%x", sha256.Sum256([]byte(namespace+"\x00"+workflowID)))
}

func requestDigest(v any) string {
	data, _ := json.Marshal(v)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func commandPayload(v any) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// PostgreSQL JSONB adds whitespace. Indented JSON is a conservative bound,
	// checked before persisting an intent or making an external call.
	var expanded bytes.Buffer
	if err := json.Indent(&expanded, data, "", "  "); err != nil {
		return nil, err
	}
	if expanded.Len() > moxiedb.MaxPayloadBytes {
		return nil, fmt.Errorf("%w: catalog command exceeds 64 KiB", tmprl.ErrInvalidArgument)
	}
	return data, nil
}

func toDBJob(namespace string, run tmprl.Run[Request, Artifacts]) moxiedb.Job {
	input := run.Request
	mode := "training"
	if input.Prepare != nil {
		mode = "preparation_and_training"
		if input.PreparationOnly {
			mode = "preparation"
		}
	}
	row := moxiedb.Job{JobID: jobIdentity(namespace, run.WorkflowID), Mode: mode}
	if input.Prepare != nil {
		row.SourceUploadID = sql.NullString{String: input.Prepare.DatasetID, Valid: input.Prepare.DatasetID != ""}
		row.TargetDatasetID = sql.NullString{String: input.Prepare.TargetDatasetID, Valid: input.Prepare.TargetDatasetID != ""}
		row.BaseDatasetVersion = sql.NullInt64{Int64: int64(input.Prepare.BaseVersion), Valid: row.TargetDatasetID.Valid}
	} else if saved := input.SavedDataset; saved != nil {
		row.TargetDatasetID = sql.NullString{String: saved.ID, Valid: true}
		row.BaseDatasetVersion = sql.NullInt64{Int64: int64(saved.Version), Valid: true}
	} else {
		row.SourceUploadID = sql.NullString{String: input.DatasetID, Valid: input.DatasetID != ""}
	}
	return row
}

func (j *Job) registerAttempt(ctx context.Context, run tmprl.Run[Request, Artifacts]) (moxiedb.Attempt, error) {
	job := toDBJob(j.cfg.Namespace, run)
	if err := j.cfg.MoxieDB.CreateJob(ctx, job); err != nil {
		return moxiedb.Attempt{}, err
	}
	a := moxiedb.Attempt{AttemptID: job.JobID + "/1", JobID: job.JobID, AttemptNumber: 1,
		TemporalNamespace: j.cfg.Namespace, WorkflowID: run.WorkflowID}
	if err := j.cfg.MoxieDB.CreateAttempt(ctx, a); err != nil {
		return moxiedb.Attempt{}, err
	}
	if run.RunID != "" {
		if err := j.cfg.MoxieDB.LinkAttemptRun(ctx, a.AttemptID, run.RunID); err != nil {
			return moxiedb.Attempt{}, err
		}
	}
	return j.cfg.MoxieDB.GetAttempt(ctx, a.AttemptID)
}

func (j *Job) catalogSubmit(ctx context.Context, input Request) (tmprl.Run[Request, Artifacts], error) {
	id := jobIdentity(j.cfg.Namespace, input.ID) + "/1"
	intentID := id + "/submit"
	digest := requestDigest(input)
	previous, err := j.cfg.MoxieDB.GetIntent(ctx, intentID)
	switch {
	case err == nil:
		var command submitCommand
		if err := json.Unmarshal(previous.Payload, &command); err != nil {
			return tmprl.Run[Request, Artifacts]{}, err
		}
		if command.InputSHA256 != digest {
			return tmprl.Run[Request, Artifacts]{}, fmt.Errorf("%w: submission ID has different input", tmprl.ErrInvalidArgument)
		}
		return j.resolveSubmission(ctx, previous, command)
	case !errors.Is(err, moxiedb.ErrNotFound):
		return tmprl.Run[Request, Artifacts]{}, err
	}
	validated, plan, err := j.submission(ctx, input)
	if err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
	}
	command := submitCommand{InputSHA256: digest, Submission: tmprl.JobInput[Request]{Job: &plan, Request: validated}}
	switch {
	case validated.PreparationOnly:
		command.Submission.ControlStage = "preparation"
	case validated.Prepare == nil:
		command.Submission.ControlStage = "training"
		command.Submission.PauseInPlace = true
	}
	payload, err := commandPayload(command)
	if err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
	}
	a, err := j.registerAttempt(ctx, tmprl.Run[Request, Artifacts]{WorkflowID: input.ID, Request: validated})
	if err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
	}
	intent := moxiedb.Intent{IntentID: intentID, JobID: a.JobID, AttemptID: a.AttemptID, Kind: "submit", Payload: payload, PayloadSHA256: requestDigest(command)}
	if err := j.cfg.MoxieDB.BeginIntent(ctx, intent); err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
	}
	return j.resolveSubmission(ctx, intent, command)
}

func (j *Job) resolveSubmission(ctx context.Context, intent moxiedb.Intent, command submitCommand) (tmprl.Run[Request, Artifacts], error) {
	a, err := j.cfg.MoxieDB.GetAttempt(ctx, intent.AttemptID)
	if err != nil {
		return tmprl.Run[Request, Artifacts]{}, err
	}
	if intent.State == "rejected" {
		return tmprl.Run[Request, Artifacts]{}, fmt.Errorf("%w: submission was rejected", tmprl.ErrInvalidArgument)
	}
	// First resolve an ambiguous receipt. The runner forbids workflow-ID reuse;
	// even racing reconcilers cannot start a second execution of this identity.
	description, err := j.client.DescribeWorkflowExecution(ctx, a.WorkflowID, a.RunID.String)
	var run tmprl.Run[Request, Artifacts]
	switch {
	case err == nil:
		info := description.GetWorkflowExecutionInfo()
		if info.GetType().GetName() != "moxie.training.Job" || info.GetExecution().GetWorkflowId() != a.WorkflowID {
			return run, fmt.Errorf("%w: submission execution identity conflicts", moxiedb.ErrConflict)
		}
		run, err = j.jobs.Get(ctx, tmprl.Execution{WorkflowID: a.WorkflowID, RunID: info.GetExecution().GetRunId()})
	case isMissingExecution(err) && !a.RunID.Valid && intent.State != "accepted":
		run, err = j.jobs.Submit(ctx, a.WorkflowID, command.Submission)
	default:
		return run, executionError(err)
	}
	if err != nil {
		return run, executionError(err)
	}
	if requestDigest(run.Request) != requestDigest(command.Submission.Request) {
		return run, fmt.Errorf("%w: submission execution input conflicts", moxiedb.ErrConflict)
	}
	if err := j.cfg.MoxieDB.LinkAttemptRun(ctx, a.AttemptID, run.RunID); err != nil {
		return run, err
	}
	if err := j.cfg.MoxieDB.SettleIntent(ctx, intent.IntentID, "accepted", sql.NullString{}); err != nil {
		return run, err
	}
	return run, j.observe(ctx, a, run)
}

func isMissingExecution(err error) bool {
	_, ok := errors.AsType[*serviceerror.NotFound](err)
	return ok
}

func (j *Job) observe(ctx context.Context, a moxiedb.Attempt, run tmprl.Run[Request, Artifacts]) error {
	if a.SourceUpdatedAt.Valid && run.UpdatedAt.Before(a.SourceUpdatedAt.Time) ||
		a.ExecutionStatus.Valid && tmprl.Status(a.ExecutionStatus.String).Terminal() && !run.Status.Terminal() {
		return nil
	}
	state := Describe(run)
	state.Issues = j.runIssues(run)
	projection := CatalogObservation{State: state, NotCompared: run.Artifacts.Training.NotCompared}
	if output := run.Artifacts.Preparation; output != nil {
		projection.Prepared = run.Request.Prepare != nil && output.DatasetID == run.Request.Prepare.DatasetID && output.RecordsPath != ""
		projection.RecordCount, projection.TrainingReady = output.RecordCount, output.TrainingReady
		projection.SkippedRecords = output.SavedSkippedRecords
	}
	projection.State.Definition, projection.State.Decisions = nil, nil
	// Keep only the allowlisted failure reason and log, never raw errors or types.
	projection.State.Error, projection.State.WorkerHost, projection.State.FailureTypes = "", "", nil
	// Training options and the saved dataset are shown with the run; the model path is not.
	projection.State.Request = Request{PreparationOnly: run.Request.PreparationOnly, Training: run.Request.Training, SavedDataset: run.Request.SavedDataset}
	projection.State.Artifacts = Artifacts{}
	projection.State.Stages = slices.Clone(run.Stages)
	for i := range projection.State.Stages {
		stage := &projection.State.Stages[i]
		stage.LastError = ""
		stage.Steps = slices.Clone(stage.Steps)
		for k := range stage.Steps {
			stage.Steps[k].LastError = ""
		}
	}
	if run.Progress != nil {
		projection.State.Progress = new(*run.Progress)
		projection.State.Progress.Message = ""
	}
	payload, err := commandPayload(projection)
	if err != nil {
		return err
	}
	o := moxiedb.AttemptObservation{ExecutionStatus: sql.NullString{String: string(run.Status), Valid: run.Status != ""},
		Stage: sql.NullString{String: string(run.Stage), Valid: run.Stage != ""}, Step: sql.NullString{String: run.Step, Valid: run.Step != ""},
		SourceUpdatedAt: sql.NullTime{Time: run.UpdatedAt, Valid: !run.UpdatedAt.IsZero()}, ObservedAt: sql.NullTime{Time: time.Now(), Valid: true},
		StateStale: run.StateStale, StartedAt: sql.NullTime{Time: run.StartedAt, Valid: !run.StartedAt.IsZero()},
		ClosedAt: sql.NullTime{Time: run.ClosedAt, Valid: !run.ClosedAt.IsZero()}, Observation: payload}
	if output := run.Artifacts.Preparation; output != nil && output.SavedDatasetID != "" {
		o.ResultDatasetID = sql.NullString{String: output.SavedDatasetID, Valid: true}
		o.ResultDatasetVersion = sql.NullInt64{Int64: int64(output.SavedVersion), Valid: true}
	}
	err = j.cfg.MoxieDB.UpdateAttemptObservation(ctx, a.AttemptID, a.ObservationRevision, o)
	if errors.Is(err, moxiedb.ErrConflict) {
		return nil // Another observation won; never retry this older observation.
	}
	return err
}

func (j *Job) refreshAttempt(ctx context.Context, a moxiedb.Attempt) error {
	run, err := j.jobs.Get(ctx, tmprl.Execution{WorkflowID: a.WorkflowID, RunID: a.RunID.String})
	if err == nil {
		return j.observe(ctx, a, run)
	}
	o := a.AttemptObservation
	o.StateStale = true
	o.ErrorCode = sql.NullString{String: "execution_unavailable", Valid: true}
	writeErr := j.cfg.MoxieDB.UpdateAttemptObservation(ctx, a.AttemptID, a.ObservationRevision, o)
	if errors.Is(writeErr, moxiedb.ErrConflict) {
		writeErr = nil
	}
	return errors.Join(executionError(err), writeErr)
}

func (j *Job) beginControl(ctx context.Context, e tmprl.Execution, kind string, decision tmprl.Decision) (moxiedb.Intent, error) {
	if err := e.Validate(); err != nil {
		return moxiedb.Intent{}, err
	}
	entry, err := j.cfg.MoxieDB.GetCatalogExecution(ctx, j.cfg.Namespace, e.WorkflowID, e.RunID)
	if errors.Is(err, moxiedb.ErrNotFound) {
		// Adopt a legacy execution only after the runner verifies its exact type.
		run, readErr := j.jobs.Get(ctx, e)
		if readErr != nil {
			return moxiedb.Intent{}, executionError(readErr)
		}
		a, registerErr := j.registerAttempt(ctx, run)
		if registerErr != nil {
			return moxiedb.Intent{}, registerErr
		}
		entry.Attempt = a
	} else if err != nil {
		return moxiedb.Intent{}, err
	}
	payload, err := commandPayload(decision)
	if err != nil {
		return moxiedb.Intent{}, err
	}
	intent := moxiedb.Intent{IntentID: entry.Attempt.AttemptID + "/" + kind + "/" + requestDigest([]string{decision.CheckpointID, decision.SubmissionID}),
		JobID: entry.Attempt.JobID, AttemptID: entry.Attempt.AttemptID, Kind: kind,
		CheckpointID: sql.NullString{String: decision.CheckpointID, Valid: decision.CheckpointID != ""},
		Payload:      payload, PayloadSHA256: requestDigest(decision)}
	if err := j.cfg.MoxieDB.BeginIntent(ctx, intent); err != nil {
		return intent, err
	}
	return j.cfg.MoxieDB.GetIntent(ctx, intent.IntentID)
}

func (j *Job) settleControl(ctx context.Context, intent moxiedb.Intent, err error) error {
	state, code := "accepted", sql.NullString{}
	if err != nil {
		// Timeouts/unavailability are ambiguous. Keep the exact request pending
		// until the runner confirms its outcome; an HTTP error is not rejection.
		if !errors.Is(err, tmprl.ErrInvalidArgument) && !errors.Is(err, tmprl.ErrInvalidExecution) && !errors.Is(err, tmprl.ErrControlConflict) {
			return err
		}
		state, code = "rejected", sql.NullString{String: "invalid_command", Valid: true}
	}
	return errors.Join(err, j.cfg.MoxieDB.SettleIntent(ctx, intent.IntentID, state, code))
}

func (j *Job) decide(ctx context.Context, e tmprl.Execution, kind string, decision tmprl.Decision) (tmprl.Decision, error) {
	if j.cfg.MoxieDB == nil {
		receipt, err := j.jobs.Decide(ctx, e, decision)
		return receipt, executionError(err)
	}
	intent, err := j.beginControl(ctx, e, kind, decision)
	if err != nil {
		return tmprl.Decision{}, err
	}
	switch intent.State {
	case "accepted":
		return decision, nil
	case "rejected":
		return tmprl.Decision{}, fmt.Errorf("%w: command was rejected", tmprl.ErrInvalidArgument)
	}
	receipt, err := j.jobs.Decide(ctx, e, decision)
	return receipt, j.settleControl(ctx, intent, executionError(err))
}

// PreparationControl persists an immutable request before Temporal delivery.
// Accepted receipts are distinct from execution status and safe-point progress.
// Only new preparation-only executions opt in; training/legacy runs are unchanged.
func (j *Job) PreparationControl(ctx context.Context, e tmprl.Execution, request tmprl.ControlRequest) (tmprl.ControlReceipt, error) {
	return j.runControl(ctx, e, request, "preparation")
}

// TrainingControl pauses or resumes a new training-only run without restarting
// its active optimizer. Older and combined runs retain their original protocol.
func (j *Job) TrainingControl(ctx context.Context, e tmprl.Execution, request tmprl.ControlRequest) (tmprl.ControlReceipt, error) {
	return j.runControl(ctx, e, request, "training")
}

func (j *Job) runControl(ctx context.Context, e tmprl.Execution, request tmprl.ControlRequest, stage string) (tmprl.ControlReceipt, error) {
	if err := e.Validate(); err != nil {
		return tmprl.ControlReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return tmprl.ControlReceipt{}, err
	}
	if j.cfg.MoxieDB == nil {
		return tmprl.ControlReceipt{}, ErrUnavailable
	}
	entry, err := j.cfg.MoxieDB.GetCatalogExecution(ctx, j.cfg.Namespace, e.WorkflowID, e.RunID)
	if err != nil {
		if errors.Is(err, moxiedb.ErrNotFound) {
			return tmprl.ControlReceipt{}, ErrNotFound
		}
		return tmprl.ControlReceipt{}, err
	}
	if stage == "" {
		stage = entry.Job.Mode // Durable reconciliation selects the original job's mode.
	}
	if entry.Job.Mode != stage || (stage != "preparation" && stage != "training") {
		return tmprl.ControlReceipt{}, tmprl.ErrControlConflict
	}
	payload, err := commandPayload(request)
	if err != nil {
		return tmprl.ControlReceipt{}, err
	}
	intent := moxiedb.Intent{IntentID: entry.Attempt.AttemptID + "/control/" + requestDigest(request.RequestID),
		JobID: entry.Job.JobID, AttemptID: entry.Attempt.AttemptID, Kind: request.Action,
		Payload: payload, PayloadSHA256: requestDigest(request)}
	previous, readErr := j.cfg.MoxieDB.GetIntent(ctx, intent.IntentID)
	if readErr != nil && !errors.Is(readErr, moxiedb.ErrNotFound) {
		return tmprl.ControlReceipt{}, readErr
	}
	if readErr == nil {
		if previous.PayloadSHA256 != intent.PayloadSHA256 || previous.Kind != intent.Kind {
			return tmprl.ControlReceipt{}, tmprl.ErrControlConflict
		}
		switch previous.State {
		case "accepted":
			return tmprl.ControlReceipt{ControlRequest: request, Revision: request.ExpectedRevision + 1}, nil
		case "rejected":
			return tmprl.ControlReceipt{}, tmprl.ErrControlConflict
		}
	} else {
		run, err := j.jobs.Get(ctx, e)
		if err != nil {
			return tmprl.ControlReceipt{}, executionError(err)
		}
		if run.ControlStage != stage || run.Status.Terminal() || stage == "training" && !run.PauseInPlace || stage == "preparation" && !run.Request.PreparationOnly {
			return tmprl.ControlReceipt{}, tmprl.ErrControlConflict
		}
	}
	if err := j.cfg.MoxieDB.BeginIntent(ctx, intent); err != nil {
		if errors.Is(err, moxiedb.ErrConflict) {
			return tmprl.ControlReceipt{}, tmprl.ErrControlConflict
		}
		return tmprl.ControlReceipt{}, err
	}
	receipt, err := j.jobs.Control(ctx, e, request)
	return receipt, j.settleControl(ctx, intent, executionError(err))
}

// Reconcile resolves durable intents and refreshes active catalog observations.
// It neither owns a scheduler nor starts a new logical job after a disconnection.
func (j *Job) Reconcile(ctx context.Context) error {
	if j.cfg.MoxieDB == nil {
		return nil
	}
	var result error
	intents, err := j.cfg.MoxieDB.PendingIntents(ctx)
	if err != nil {
		return err
	}
	for _, intent := range intents {
		a, err := j.cfg.MoxieDB.GetAttempt(ctx, intent.AttemptID)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if a.TemporalNamespace != j.cfg.Namespace {
			continue
		}
		e := tmprl.Execution{WorkflowID: a.WorkflowID, RunID: a.RunID.String}
		switch intent.Kind {
		case "submit":
			var command submitCommand
			err = json.Unmarshal(intent.Payload, &command)
			if err == nil {
				_, err = j.resolveSubmission(ctx, intent, command)
			}
		case "preview_review", "authorize_training":
			var decision tmprl.Decision
			err = json.Unmarshal(intent.Payload, &decision)
			if err == nil {
				if intent.Kind == "preview_review" {
					_, err = j.ContinuePreview(ctx, e, decision)
				} else {
					_, err = j.StartTraining(ctx, e, decision)
				}
				err = j.settleControl(ctx, intent, err)
			}
		case "stop":
			err = j.settleControl(ctx, intent, executionError(j.jobs.Stop(ctx, e)))
		case "pause", "resume":
			var request tmprl.ControlRequest
			err = json.Unmarshal(intent.Payload, &request)
			if err == nil {
				_, err = j.runControl(ctx, e, request, "")
			}
		default:
			continue
		}
		result = errors.Join(result, err)
	}
	statuses := []string{"", "queued", "running", "recovering", "waiting", "pause_requested", "paused"}
	var attempts []moxiedb.Attempt
	for offset := 0; ; offset += 100 {
		entries, err := j.cfg.MoxieDB.ListCatalog(ctx, moxiedb.CatalogFilter{Namespace: j.cfg.Namespace, Statuses: statuses, Limit: 100, Offset: offset})
		if err != nil {
			return errors.Join(result, err)
		}
		for _, entry := range entries {
			if entry.Attempt.RunID.Valid {
				attempts = append(attempts, entry.Attempt)
			}
		}
		if len(entries) < 100 {
			break
		}
	}
	// Gather before updating: terminal observations remove rows from the active
	// filter and must not shift a later page out of this reconciliation pass.
	for _, attempt := range attempts {
		result = errors.Join(result, j.refreshAttempt(ctx, attempt))
	}
	return result
}

// RunCatalog is host-owned and must be canceled/joined before closing the client
// or database. A service outage is logged and retried, never turned into failure.
func (j *Job) RunCatalog(ctx context.Context) {
	if j.cfg.MoxieDB == nil {
		return
	}
	// Discover pre-catalog history without resubmitting it. Retain the last
	// successful page and retry discovery after outages, independently of known jobs.
	var next []byte
	discovered := false
	for {
		attempt, cancel := context.WithTimeout(ctx, 30*time.Second)
		for !discovered && attempt.Err() == nil {
			runs, token, err := j.jobs.List(attempt, tmprl.ListOptions{PageSize: 100, NextPageToken: next})
			if err != nil {
				j.cfg.Log(ctx, "job catalog history unavailable", "error", err)
				break
			}
			var importErr error
			for _, run := range runs {
				a, err := j.registerAttempt(attempt, run)
				if err == nil {
					err = j.observe(attempt, a, run)
				}
				importErr = errors.Join(importErr, err)
			}
			if importErr != nil {
				j.cfg.Log(ctx, "job catalog history import failed", "error", importErr)
				break
			}
			next, discovered = token, len(token) == 0
		}
		cancel()
		attempt, cancel = context.WithTimeout(ctx, 30*time.Second)
		err := j.Reconcile(attempt)
		cancel()
		if err != nil && ctx.Err() == nil {
			j.cfg.Log(ctx, "job catalog reconciliation pending", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

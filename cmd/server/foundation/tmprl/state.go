package tmprl

import (
	"encoding/json"
	"time"
)

// StageName identifies a stage in a submitted job.
type StageName string

// Status describes execution lifecycle or an application-defined terminal outcome.
type Status string

const (
	StatusQueued         Status = "queued"
	StatusRunning        Status = "running"
	StatusRecovering     Status = "recovering"
	StatusPauseRequested Status = "pause_requested"
	StatusPaused         Status = "paused"
	StatusWaiting        Status = "waiting"
	StatusCompleted      Status = "completed"
	StatusStopped        Status = "stopped"
	StatusFailed         Status = "failed"
)

// Terminal reports whether scheduling has ended. Applications may supply their
// own terminal outcomes; lifecycle names are reserved by the runner.
func (s Status) Terminal() bool {
	switch s {
	case "", StatusQueued, StatusRunning, StatusRecovering, StatusPauseRequested, StatusPaused, StatusWaiting:
		return false
	default:
		return true
	}
}

// JobInput captures the immutable plan and application request in workflow history.
type JobInput[R any] struct {
	Job     *Job `json:"job,omitzero"`
	Request R    `json:"request"`
	// ControlStage opts new executions into ordered controls for this stage.
	// Empty preserves the legacy signal protocol and workflow history.
	ControlStage string `json:"control_stage,omitzero"`
	// PauseInPlace retains the activity and its in-memory computation state at
	// cooperative pause points. Paused time still counts toward its deadlines.
	PauseInPlace bool `json:"pause_in_place,omitzero"`
}

// Run contains durable state. R and A are application-owned requests and artifacts.
// These JSON fields are also the workflow, memo, and polling wire contract.
type Run[R, A any] struct {
	Execution
	Definition                  *Job         `json:"definition,omitzero"`
	Request                     R            `json:"request"`
	Status                      Status       `json:"status"`
	Stage                       StageName    `json:"stage,omitzero"`
	Step                        string       `json:"step,omitzero"`
	Artifacts                   A            `json:"artifacts"`
	Checkpoint                  *Checkpoint  `json:"checkpoint,omitzero"`
	Decisions                   []Decision   `json:"decisions,omitzero"`
	Progress                    *Progress    `json:"progress,omitzero"`
	StartedAt                   time.Time    `json:"started_at"`
	ClosedAt                    time.Time    `json:"closed_at,omitzero"`
	Error                       string       `json:"error,omitzero"`
	FailureTypes                []string     `json:"failure_types,omitzero"` // Application error types in the failure chain, outermost first.
	Stages                      []StageState `json:"stages,omitzero"`
	SessionAttempt              int          `json:"session_attempt,omitzero"`
	WorkerHost                  string       `json:"worker_host,omitzero"`
	ReservationDeadline         time.Time    `json:"reservation_deadline,omitzero"`
	UpdatedAt                   time.Time    `json:"updated_at,omitzero"`
	StateStale                  bool         `json:"state_stale,omitzero"`
	ControlStage                string       `json:"control_stage,omitzero"`
	PauseInPlace                bool         `json:"pause_in_place,omitzero"`
	ControlRevision             int64        `json:"control_revision,omitzero"`
	ControlAction               string       `json:"control_action,omitzero"`
	ControlAcknowledgedRevision int64        `json:"control_acknowledged_revision,omitzero"`
	ControlCheckpoint           string       `json:"control_checkpoint,omitzero"`
}

// StageState records a stage's total budget and its ordered step diagnostics.
type StageState struct {
	Name     StageName   `json:"name"`
	Status   Status      `json:"status"`
	Steps    []StepState `json:"steps,omitzero"`
	Deadline time.Time   `json:"deadline,omitzero"`
	// ActiveElapsed is the budget consumed by earlier worker reservations when
	// paused, review, and offline time do not count. Deadline is then the
	// current reservation's projection of the remaining budget.
	ActiveElapsed   time.Duration `json:"active_elapsed,omitzero"`
	Attempt         int32         `json:"attempt,omitzero"`
	StartedAt       time.Time     `json:"started_at,omitzero"`
	ClosedAt        time.Time     `json:"closed_at,omitzero"`
	LastHeartbeatAt time.Time     `json:"last_heartbeat_at,omitzero"`
	LastError       string        `json:"last_error,omitzero"`
}

// StepState records committed work and the latest execution diagnostics.
type StepState struct {
	Name            string    `json:"name"`
	Status          Status    `json:"status"`
	Attempt         int32     `json:"attempt,omitzero"`
	StartedAt       time.Time `json:"started_at,omitzero"`
	ClosedAt        time.Time `json:"closed_at,omitzero"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at,omitzero"`
	LastError       string    `json:"last_error,omitzero"`
}

// Progress is an identity-bound heartbeat and optional local notification.
type Progress struct {
	Execution
	Stage     StageName `json:"stage"`
	Step      string    `json:"step,omitzero"`
	Completed int       `json:"completed"`
	Total     int       `json:"total"`
	Message   string    `json:"message"`
	// Timing is scoped to one activity attempt and one monotonic work counter.
	// The initial completed count excludes work restored from a checkpoint.
	InitialCompleted int       `json:"initial_completed,omitzero"`
	StartedAt        time.Time `json:"started_at,omitzero"`
	UpdatedAt        time.Time `json:"updated_at,omitzero"`
}

// Remaining estimates current-phase work from timed progress, not a deadline.
// Zero means no estimate is available, or all reported work is complete.
func (p Progress) Remaining() time.Duration {
	if p.StartedAt.IsZero() || !p.UpdatedAt.After(p.StartedAt) || p.InitialCompleted < 0 || p.Completed <= p.InitialCompleted || p.Completed >= p.Total {
		return 0
	}
	return time.Duration(float64(p.UpdatedAt.Sub(p.StartedAt)) * float64(p.Total-p.Completed) / float64(p.Completed-p.InitialCompleted))
}

// StepRequest carries a stable idempotency key and the incoming artifact set.
type StepRequest[R, A any] struct {
	Execution
	Key              string     `json:"key"`
	Stage            string     `json:"stage"`
	Step             string     `json:"step"`
	Request          R          `json:"request"`
	Artifacts        A          `json:"artifacts"`
	Decisions        []Decision `json:"decisions,omitzero"`
	SafePoints       bool       `json:"safe_points,omitzero"`
	PauseInPlace     bool       `json:"pause_in_place,omitzero"`
	ResumeCheckpoint string     `json:"resume_checkpoint,omitzero"`
}

// Checkpoint identifies a required decision after a committed step.
type Checkpoint struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stage string `json:"stage"`
	Step  string `json:"step"`
}

// Decision carries application-validated input separately from the job request.
// Retry the same submission ID and payload to retrieve its acceptance receipt.
type Decision struct {
	CheckpointID string          `json:"checkpoint_id"`
	SubmissionID string          `json:"submission_id"`
	Payload      json.RawMessage `json:"payload,omitzero"`
}

// StepResult commits artifacts and optionally ends the job with a terminal outcome.
type StepResult[A any] struct {
	Artifacts A      `json:"artifacts"`
	Status    Status `json:"status,omitzero"`
	Attempt   int32  `json:"attempt,omitzero"`
}

// Names fixes the persisted Temporal protocol for an application job. Keep these
// names stable across deployments while existing histories can still replay.
type Names struct {
	Workflow string
	Activity string
	Memo     string
	Query    string
	Control  string
}

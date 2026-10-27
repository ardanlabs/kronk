package tmprl

import (
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
	StatusCompleted      Status = "completed"
	StatusStopped        Status = "stopped"
	StatusFailed         Status = "failed"
)

// Terminal reports whether scheduling has ended. Applications may supply their
// own terminal outcomes; lifecycle names are reserved by the runner.
func (s Status) Terminal() bool {
	switch s {
	case "", StatusQueued, StatusRunning, StatusRecovering, StatusPauseRequested, StatusPaused:
		return false
	default:
		return true
	}
}

// JobInput captures the immutable plan and application request in workflow history.
type JobInput[R any] struct {
	Job     *Job `json:"job,omitzero"`
	Request R    `json:"request"`
}

// Run contains durable state. R and A are application-owned requests and artifacts.
// These JSON fields are also the workflow, memo, and polling wire contract.
type Run[R, A any] struct {
	Execution
	Definition          *Job         `json:"definition,omitzero"`
	Request             R            `json:"request"`
	Status              Status       `json:"status"`
	Stage               StageName    `json:"stage,omitzero"`
	Step                string       `json:"step,omitzero"`
	Artifacts           A            `json:"artifacts"`
	Progress            *Progress    `json:"progress,omitzero"`
	StartedAt           time.Time    `json:"started_at"`
	ClosedAt            time.Time    `json:"closed_at,omitzero"`
	Error               string       `json:"error,omitzero"`
	Stages              []StageState `json:"stages,omitzero"`
	SessionAttempt      int          `json:"session_attempt,omitzero"`
	WorkerHost          string       `json:"worker_host,omitzero"`
	ReservationDeadline time.Time    `json:"reservation_deadline,omitzero"`
	UpdatedAt           time.Time    `json:"updated_at,omitzero"`
	StateStale          bool         `json:"state_stale,omitzero"`
}

// StageState records a stage's total budget and its ordered step diagnostics.
type StageState struct {
	Name            StageName   `json:"name"`
	Status          Status      `json:"status"`
	Steps           []StepState `json:"steps,omitzero"`
	Deadline        time.Time   `json:"deadline,omitzero"`
	Attempt         int32       `json:"attempt,omitzero"`
	StartedAt       time.Time   `json:"started_at,omitzero"`
	ClosedAt        time.Time   `json:"closed_at,omitzero"`
	LastHeartbeatAt time.Time   `json:"last_heartbeat_at,omitzero"`
	LastError       string      `json:"last_error,omitzero"`
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
}

// StepRequest carries a stable idempotency key and the incoming artifact set.
type StepRequest[R, A any] struct {
	Execution
	Key       string `json:"key"`
	Stage     string `json:"stage"`
	Step      string `json:"step"`
	Request   R      `json:"request"`
	Artifacts A      `json:"artifacts"`
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

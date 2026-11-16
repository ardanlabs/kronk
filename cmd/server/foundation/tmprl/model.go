// Package tmprl executes and manages application-defined Temporal jobs. It owns
// worker reservations, scheduling, controls, durable state and diagnostics;
// applications supply job definitions, request/artifact types and step handlers.
package tmprl

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidExecution = errors.New("workflow and run IDs are required")

// ErrControlConflict identifies a stale or incompatible control request.
var ErrControlConflict = errors.New("job control conflicts with execution state")

// ControlRequest identifies an immutable command against a workflow control
// revision, not the SQL observation revision. Resume never approves a review.
type ControlRequest struct {
	RequestID        string `json:"request_id"`
	Action           string `json:"action"`
	ExpectedRevision int64  `json:"expected_revision"`
}

func (r ControlRequest) Validate() error {
	if strings.TrimSpace(r.RequestID) == "" || len(r.RequestID) > 200 || r.ExpectedRevision < 0 || r.ExpectedRevision == 1<<63-1 {
		return fmt.Errorf("%w: request ID (1–200 bytes) and a valid control revision are required", ErrInvalidArgument)
	}
	if r.Action != "pause" && r.Action != "resume" {
		return fmt.Errorf("%w: unsupported control action", ErrInvalidArgument)
	}
	return nil
}

// ControlReceipt acknowledges command acceptance, not a saved pause checkpoint.
type ControlReceipt struct {
	ControlRequest
	Revision int64 `json:"revision"`
}

// Execution identifies one run, not just a reusable workflow ID.
type Execution struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
}

// Validate requires both IDs so controls cannot accidentally target another run.
func (e Execution) Validate() error {
	if e.WorkflowID == "" || e.RunID == "" {
		return ErrInvalidExecution
	}
	return nil
}

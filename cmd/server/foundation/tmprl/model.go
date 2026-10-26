// Package tmprl executes and manages application-defined Temporal jobs. It owns
// worker reservations, scheduling, controls, durable state and diagnostics;
// applications supply job definitions, request/artifact types and step handlers.
package tmprl

import "errors"

var ErrInvalidExecution = errors.New("workflow and run IDs are required")

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

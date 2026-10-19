// Package tmprl provides shared Temporal execution operations. Workflow
// definitions, payloads, and policies belong to the workflow packages.
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

package preparation

import (
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

// ValidateExecution validates a durable execution identity.
func ValidateExecution(e tmprl.Execution) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return nil
}

// ExecutionError translates foundation execution errors into invalid input.
func ExecutionError(err error) error {
	if errors.Is(err, tmprl.ErrInvalidExecution) || errors.Is(err, tmprl.ErrUnsupportedWorkflow) || errors.Is(err, tmprl.ErrInvalidArgument) {
		return fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return err
}

// InvalidOutput preserves the historical nonretryable failure type.
func InvalidOutput(step string) error {
	return temporal.NewNonRetryableApplicationError(fmt.Sprintf("invalid output for tuning step %s", step), "invalid-stage-output", nil)
}

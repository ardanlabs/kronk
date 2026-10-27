package modeltraining

import (
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"go.temporal.io/sdk/temporal"
)

// ErrInvalidArgument identifies rejected job inputs and controls.
var ErrInvalidArgument = tmprl.ErrInvalidArgument

func ValidateExecution(e tmprl.Execution) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return nil
}

// ExecutionError translates generic Temporal validation into the domain error.
func ExecutionError(err error) error {
	if errors.Is(err, tmprl.ErrInvalidExecution) || errors.Is(err, tmprl.ErrUnsupportedWorkflow) || errors.Is(err, tmprl.ErrInvalidArgument) {
		return fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return err
}

// InvalidOutput rejects invalid step results without retrying computation.
// The failure type retains its historical value for existing Temporal clients.
func InvalidOutput(step string) error {
	return temporal.NewNonRetryableApplicationError(fmt.Sprintf("invalid output for tuning step %s", step), "invalid-stage-output", nil)
}

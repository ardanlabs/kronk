package model

import (
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl"
	"go.temporal.io/sdk/temporal"
)

func ValidateExecution(e Execution) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return nil
}

// ExecutionError translates generic Temporal validation into the domain error.
func ExecutionError(err error) error {
	if errors.Is(err, tmprl.ErrInvalidExecution) || errors.Is(err, tmprl.ErrUnsupportedWorkflow) {
		return fmt.Errorf("%w: %s", ErrInvalidArgument, err)
	}
	return err
}

// InvalidOutput preserves the activity failure contract for invalid stage results.
func InvalidOutput(stage Stage) error {
	return temporal.NewNonRetryableApplicationError(fmt.Sprintf("invalid output for tuning stage %s", stage), "invalid-stage-output", nil)
}

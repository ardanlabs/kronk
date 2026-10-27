package preparation

import (
	"fmt"

	"go.temporal.io/sdk/temporal"
)

// InvalidOutput reports an invalid step result as a nonretryable failure.
func InvalidOutput(step string) error {
	return temporal.NewNonRetryableApplicationError(fmt.Sprintf("invalid output for preparation step %s", step), "invalid-step-output", nil)
}

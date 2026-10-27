package modeltraining

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// ActivityProgress binds the stage identity before sharing heartbeat progress.
func ActivityProgress(ctx context.Context, execution tmprl.Execution, stage tmprl.StageName, publish func(tmprl.Progress)) (func(tmprl.Progress), func()) {
	report, stop := tmprl.Heartbeat(ctx, tmprl.Progress{Execution: execution, Stage: stage}, publish)
	return func(update tmprl.Progress) {
		update.Execution = execution
		update.Stage = stage
		report(update)
	}, stop
}

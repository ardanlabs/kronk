package prepare

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// activityProgress binds preparation identity to every heartbeat update.
func activityProgress(ctx context.Context, execution tmprl.Execution, stage tmprl.StageName, publish func(tmprl.Progress)) (func(tmprl.Progress), func()) {
	report, stop := tmprl.Heartbeat(ctx, tmprl.Progress{Execution: execution, Stage: stage}, publish)
	return func(update tmprl.Progress) {
		update.Execution = execution
		update.Stage = stage
		report(update)
	}, stop
}

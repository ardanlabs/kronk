package model

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl"
)

// ActivityProgress binds the stage identity before sharing heartbeat progress.
func ActivityProgress(ctx context.Context, execution Execution, stage Stage, publish func(Progress)) (func(Progress), func()) {
	report, stop := tmprl.Heartbeat(ctx, Progress{Execution: execution, Stage: stage}, publish)
	return func(update Progress) {
		update.Execution = execution
		update.Stage = stage
		report(update)
	}, stop
}

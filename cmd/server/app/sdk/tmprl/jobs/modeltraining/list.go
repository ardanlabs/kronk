package modeltraining

import (
	"context"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

// List returns eventually consistent snapshots of training runs without live
// heartbeat or activity-attempt enrichment. Older runs without snapshots remain
// in the list as metadata-only summaries; use Get for their details. Statuses are
// matched exactly after decoding snapshots and reconciling terminal lifecycle.
// Filtering can produce short or empty pages with a nonempty next-page token.
// Continue with that opaque token and the same filters to read subsequent pages.
func (b *Business) List(ctx context.Context, options tmprl.ListOptions) ([]tmprl.Run[Request, Artifacts], []byte, error) {
	runs, next, err := b.jobs.List(ctx, options)
	return runs, next, ExecutionError(err)
}

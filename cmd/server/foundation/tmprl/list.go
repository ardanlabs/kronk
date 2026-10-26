package tmprl

import (
	"context"
	"fmt"
	"slices"

	"go.temporal.io/api/workflowservice/v1"
)

// ListOptions selects one visibility page. PageSize must be between 1 and 100.
// NextPageToken is opaque and must be reused with the same filters.
type ListOptions struct {
	PageSize      int
	NextPageToken []byte
	Statuses      []Status
}

// List returns eventually consistent snapshots with one visibility RPC and no
// worker queries. Exact logical status filtering may leave short or empty pages
// with a next-page token. Use Get for heartbeat diagnostics and legacy details.
func (j *Jobs[R, A]) List(ctx context.Context, options ListOptions) ([]Run[R, A], []byte, error) {
	if options.PageSize < 1 || options.PageSize > 100 {
		return nil, nil, fmt.Errorf("%w: page size must be 1–100", ErrInvalidArgument)
	}
	for _, status := range options.Statuses {
		switch status {
		case StatusQueued, StatusRunning, StatusRecovering, StatusPauseRequested, StatusPaused, StatusCompleted, StatusStopped, StatusFailed:
		default:
			if !slices.Contains(j.cfg.Outcomes, status) {
				return nil, nil, fmt.Errorf("%w: unknown job status %q", ErrInvalidArgument, status)
			}
		}
	}
	response, err := j.client.ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
		PageSize: int32(options.PageSize), NextPageToken: options.NextPageToken,
		Query: "WorkflowType = '" + j.cfg.Names.Workflow + "'",
	})
	if err != nil {
		return nil, nil, fmt.Errorf("listing job runs: %w", err)
	}
	runs := make([]Run[R, A], 0, len(response.GetExecutions()))
	for _, info := range response.GetExecutions() {
		run, _, err := j.runFromInfo(info)
		if err != nil {
			return nil, nil, fmt.Errorf("reading job run snapshot: %w", err)
		}
		if len(options.Statuses) == 0 || slices.Contains(options.Statuses, run.Status) {
			runs = append(runs, run)
		}
	}
	return runs, response.GetNextPageToken(), nil
}

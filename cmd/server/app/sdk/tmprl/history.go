package tmprl

import (
	"context"
	"errors"

	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

// ErrInvalidPageSize indicates a history request outside the supported bounds.
var ErrInvalidPageSize = errors.New("page size must be 1–100")

// History lists executions of one registered workflow type. The client is
// borrowed; the caller owns its lifetime. The returned token is opaque.
// kind must be a registered workflow name, not user-supplied query text.
func History(ctx context.Context, c client.Client, kind string, pageSize int, next []byte) ([]Execution, []byte, error) {
	if pageSize < 1 || pageSize > 100 {
		return nil, nil, ErrInvalidPageSize
	}
	response, err := c.ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
		PageSize: int32(pageSize), NextPageToken: next, Query: "WorkflowType = '" + kind + "'",
	})
	if err != nil {
		return nil, nil, err
	}
	runs := make([]Execution, 0, len(response.GetExecutions()))
	for _, info := range response.GetExecutions() {
		runs = append(runs, Execution{WorkflowID: info.GetExecution().GetWorkflowId(), RunID: info.GetExecution().GetRunId()})
	}
	return runs, response.GetNextPageToken(), nil
}

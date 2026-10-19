package tmprl

import (
	"context"
	"errors"
	"slices"

	"go.temporal.io/sdk/client"
)

var ErrUnsupportedWorkflow = errors.New("unsupported workflow type for this control")

// Start submits a named workflow and returns its exact execution identity.
// Workflow packages supply payloads and start policies; the client is borrowed.
func Start(ctx context.Context, c client.Client, options client.StartWorkflowOptions, name string, input any) (Execution, error) {
	run, err := c.ExecuteWorkflow(ctx, options, name, input)
	if err != nil {
		return Execution{}, err
	}
	return Execution{WorkflowID: run.GetID(), RunID: run.GetRunID()}, nil
}

// ValidateWorkflow prevents controls from targeting an unrelated workflow type.
func ValidateWorkflow(ctx context.Context, c client.Client, e Execution, kinds ...string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	description, err := c.DescribeWorkflowExecution(ctx, e.WorkflowID, e.RunID)
	if err != nil {
		return err
	}
	if !slices.Contains(kinds, description.GetWorkflowExecutionInfo().GetType().GetName()) {
		return ErrUnsupportedWorkflow
	}
	return nil
}

// Stop requests cooperative cancellation. Wait establishes when the run closes;
// cancellation is not termination and does not restart completed work.
func Stop(ctx context.Context, c client.Client, e Execution, kinds ...string) error {
	if err := ValidateWorkflow(ctx, c, e, kinds...); err != nil {
		return err
	}
	return c.CancelWorkflow(ctx, e.WorkflowID, e.RunID)
}

// Signal sends a workflow-defined command after checking its execution type.
func Signal(ctx context.Context, c client.Client, e Execution, name string, value any, kinds ...string) error {
	if err := ValidateWorkflow(ctx, c, e, kinds...); err != nil {
		return err
	}
	return c.SignalWorkflow(ctx, e.WorkflowID, e.RunID, name, value)
}

// Wait decodes the terminal workflow result into output.
func Wait(ctx context.Context, c client.Client, e Execution, output any, kinds ...string) error {
	if err := ValidateWorkflow(ctx, c, e, kinds...); err != nil {
		return err
	}
	return c.GetWorkflow(ctx, e.WorkflowID, e.RunID).Get(ctx, output)
}

// Query decodes live workflow state. Callers decide when querying is appropriate;
// closed executions must be read from their durable results or history instead.
func Query(ctx context.Context, c client.Client, e Execution, name string, output any) error {
	if err := e.Validate(); err != nil {
		return err
	}
	value, err := c.QueryWorkflow(ctx, e.WorkflowID, e.RunID, name)
	if err != nil {
		return err
	}
	return value.Get(output)
}

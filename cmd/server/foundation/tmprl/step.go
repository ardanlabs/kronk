package tmprl

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// ExecuteStep supplies identity, heartbeat, durable result caching and logs.
// It never schedules work. Handlers own application output validation and side
// effect idempotency; a crash before the result commits can repeat computation.
func (j *Jobs[R, A]) ExecuteStep(ctx context.Context, input StepRequest[R, A]) (output StepResult[A], err error) {
	info := activity.GetInfo(ctx)
	log := activity.GetLogger(ctx)
	started := time.Now()
	log.Info("job step started", "stage", input.Stage, "step", input.Step, "attempt", info.Attempt, "operation_key", input.Key, "worker_queue", info.TaskQueue, "deadline", info.Deadline)
	defer func() {
		output.Attempt = info.Attempt
		if err != nil {
			log.Error("job step failed", "stage", input.Stage, "step", input.Step, "attempt", info.Attempt, "elapsed", time.Since(started), "error", err)
		} else {
			log.Info("job step finished", "stage", input.Stage, "step", input.Step, "attempt", info.Attempt, "elapsed", time.Since(started), "outcome", output.Status)
		}
	}()
	if input.Execution != (Execution{WorkflowID: info.WorkflowExecution.ID, RunID: info.WorkflowExecution.RunID}) ||
		!ValidName(input.Stage) || !ValidName(input.Step) ||
		!filepath.IsLocal(input.RunID) || filepath.Base(input.RunID) != input.RunID || input.RunID == "." ||
		input.Key != input.RunID+"/"+input.Stage+"/"+input.Step {
		return output, temporal.NewNonRetryableApplicationError("invalid job step identity", "invalid-step", nil)
	}
	implementation := j.cfg.Steps[input.Stage+"/"+input.Step]
	if implementation == nil {
		return output, temporal.NewNonRetryableApplicationError(fmt.Sprintf("unregistered step %s/%s", input.Stage, input.Step), "unregistered-step", nil)
	}
	report, stop := Heartbeat(ctx, Progress{Execution: input.Execution, Stage: StageName(input.Stage), Step: input.Step}, func(update Progress) {
		j.cfg.Progress.Publish(update.Execution, update)
	})
	defer stop()
	workspace, err := j.openWorkspace(input.RunID, input.Stage)
	if err != nil {
		return output, err
	}
	defer workspace.Close()
	cache := input.Step + ".json"
	if data, readErr := workspace.ReadFile(cache); readErr == nil {
		if err := json.Unmarshal(data, &output); err != nil {
			return output, err
		}
		log.Info("job step commit reused", "stage", input.Stage, "step", input.Step)
		return output, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return output, readErr
	}
	var sample Progress
	var progressMu sync.Mutex
	output, err = implementation.Execute(ctx, StepContext[R, A]{
		Request: input, Workspace: workspace,
		Publish: func(progress Progress) {
			progressMu.Lock()
			defer progressMu.Unlock()
			progress.Execution, progress.Stage, progress.Step = input.Execution, StageName(input.Stage), input.Step
			progress.UpdatedAt = time.Now()
			if sample.StartedAt.IsZero() || progress.Total != sample.Total || progress.Completed < sample.Completed {
				progress.StartedAt, progress.InitialCompleted = progress.UpdatedAt, progress.Completed
			} else {
				progress.StartedAt, progress.InitialCompleted = sample.StartedAt, sample.InitialCompleted
			}
			sample = progress
			log.Debug("job step progress", "stage", input.Stage, "step", input.Step, "completed", progress.Completed, "total", progress.Total)
			report(progress)
		},
	})
	if err != nil {
		return output, err
	}
	if err := ctx.Err(); err != nil {
		return output, err
	}
	if output.Status != "" && output.Status != StatusCompleted && !slices.Contains(j.cfg.Outcomes, output.Status) {
		return StepResult[A]{}, temporal.NewNonRetryableApplicationError("invalid step outcome", "invalid-step-output", nil)
	}
	if err := WriteJSON(workspace, cache, output); err != nil {
		return output, err
	}
	return output, nil
}

// WriteJSON atomically publishes confined state. Readers see the previous
// complete value or the new complete value, never partially written JSON.
func WriteJSON(root *os.Root, path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(path), ".publish-"+rand.Text())
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return root.Rename(temporary, path)
}

package tmprl

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	var safePoint func(context.Context, string) error
	var lastCheckpoint string
	if input.SafePoints {
		defer j.beginSafePointStep()()
		// Validate the acknowledged checkpoint on the first resumed attempt.
		// Later retries may have advanced it legitimately and use the step's own
		// identity/cursor validation. Never reset a missing or changed pause file.
		if input.ResumeCheckpoint != "" && info.Attempt == 1 {
			relative, digest, ok := strings.Cut(input.ResumeCheckpoint, ":")
			prefix := input.Stage + "/" + input.Step + "/"
			checkpoint, scoped := strings.CutPrefix(relative, prefix)
			if !ok || !scoped || !filepath.IsLocal(checkpoint) {
				return output, temporal.NewNonRetryableApplicationError("invalid preparation resume checkpoint", "invalid-step-checkpoint", nil)
			}
			data, err := workspace.ReadFile(checkpoint)
			if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != digest {
				return output, temporal.NewNonRetryableApplicationError("preparation resume checkpoint is missing or changed; saved work was not reset", "invalid-step-checkpoint", nil)
			}
			lastCheckpoint = checkpoint
		}
		safePoint = func(ctx context.Context, checkpoint string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			reference := input.Stage + "/" + input.Step + ":not-started"
			if checkpoint != "" {
				if !filepath.IsLocal(checkpoint) {
					return fmt.Errorf("invalid checkpoint reference")
				}
				data, err := workspace.ReadFile(checkpoint)
				if err != nil {
					return err
				}
				reference = fmt.Sprintf("%s/%s/%s:%x", input.Stage, input.Step, filepath.ToSlash(checkpoint), sha256.Sum256(data))
			}
			lastCheckpoint = checkpoint
			if j.draining() {
				// Host shutdown: same durable suspension as Pause, but the
				// workflow reschedules it on the next worker without a Continue.
				return temporal.NewNonRetryableApplicationError("step suspended for worker shutdown", "step-drained", nil, reference, info.Attempt)
			}
			value, err := j.client.QueryWorkflow(ctx, input.WorkflowID, input.RunID, j.cfg.Names.Query)
			if err != nil {
				return err // Never start another unit using a stale control observation.
			}
			var run Run[R, A]
			if err := value.Get(&run); err != nil {
				return err
			}
			if run.Execution != input.Execution || run.ControlStage != input.Stage || run.Status.Terminal() {
				return fmt.Errorf("checkpoint execution is not active")
			}
			if run.ControlAction == "pause" {
				return temporal.NewNonRetryableApplicationError("step suspended at durable checkpoint", "step-suspended", nil, reference, info.Attempt)
			}
			return ctx.Err()
		}
		if err := safePoint(ctx, lastCheckpoint); err != nil {
			return output, err
		}
	}
	output, err = implementation.Execute(ctx, StepContext[R, A]{
		Request: input, Workspace: workspace,
		SafePoint: safePoint,
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
	if safePoint != nil {
		if err := safePoint(ctx, lastCheckpoint); err != nil {
			return output, err
		}
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
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := root.Rename(temporary, path); err != nil {
		return err
	}
	directory, err := root.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

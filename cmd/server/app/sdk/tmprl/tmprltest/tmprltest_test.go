package tmprltest_test

import (
	"context"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/tmprltest"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func echo(_ workflow.Context, value string) (string, error) { return value, nil }

func TestIndependentWorkers(t *testing.T) {
	queues := make(chan string, 2)
	t.Run("concurrent", func(t *testing.T) {
		for _, value := range []string{"extract-result", "evaluation-result"} {
			t.Run(value, func(t *testing.T) {
				t.Parallel()
				env := tmprltest.New(t)
				queues <- env.TaskQueue
				w := worker.New(env.Client, env.TaskQueue, worker.Options{})
				w.RegisterWorkflow(echo)
				if err := w.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(w.Stop)
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				run, err := env.Client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: env.TaskQueue, TaskQueue: env.TaskQueue}, echo, value)
				if err != nil {
					t.Fatal(err)
				}
				var result string
				if err := run.Get(ctx, &result); err != nil || result != value {
					t.Fatalf("isolated workflow result %q, want %q: %v", result, value, err)
				}
			})
		}
	})
	if len(queues) != 2 {
		t.Fatal("both test environments must initialize")
	}
	if first, second := <-queues, <-queues; first == second {
		t.Fatalf("independent tests shared queue %q", first)
	}
}

func TestUnfinishedWorkflowCleanup(t *testing.T) {
	observer := tmprltest.New(t)
	var run client.WorkflowRun
	t.Run("unfinished", func(t *testing.T) {
		env := tmprltest.New(t)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		var err error
		// No worker: this execution stays queued until the helper cleans it up.
		run, err = env.Client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: env.TaskQueue, TaskQueue: env.TaskQueue}, echo, "never consumed")
		if err != nil {
			t.Fatal(err)
		}
	})
	if run == nil {
		t.Fatal("test workflow was not submitted")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	description, err := observer.Client.DescribeWorkflowExecution(ctx, run.GetID(), run.GetRunID())
	if err != nil {
		t.Fatal(err)
	}
	if got := description.WorkflowExecutionInfo.Status; got != enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED {
		t.Fatalf("unfinished execution survived test cleanup: %s", got)
	}
}

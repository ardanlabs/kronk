package tmprltest_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/tmprltest"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func echo(_ workflow.Context, value string) (string, error) { return value, nil }

// TestBufferedLogs uses subprocesses because an intentionally failing subtest
// would also fail its parent. Both paths exercise the public test environment.
func TestBufferedLogs(t *testing.T) {
	const probe = "MOXIE_TMPRL_LOG_PROBE"
	if mode := os.Getenv(probe); mode != "" {
		env := tmprltest.New(t)
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		env.Log(ctx, "buffered-application-marker", "source", "application")
		logged := func(ctx workflow.Context) error {
			log := workflow.GetLogger(ctx)
			log.Debug("buffered-debug-marker")
			log.Info("buffered-info-marker")
			log.Warn("buffered-warning-marker")
			log.Error("buffered-error-marker")
			return nil
		}
		w := worker.New(env.Client, env.TaskQueue, worker.Options{})
		w.RegisterWorkflow(logged)
		if err := w.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(w.Stop)
		run, err := env.Client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: env.TaskQueue, TaskQueue: env.TaskQueue}, logged)
		if err != nil {
			t.Fatal(err)
		}
		if err := run.Get(ctx, nil); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			env.Log(context.Background(), "buffered-cleanup-marker")
			if mode == "fail" {
				t.Error("intentional log probe failure")
			}
		})
		return
	}
	for _, mode := range []string{"pass", "fail"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBufferedLogs$", "-test.v")
			cmd.Env = append(os.Environ(), probe+"="+mode)
			output, err := cmd.CombinedOutput()
			if (err != nil) != (mode == "fail") {
				t.Fatalf("probe %s: got %v, want failure only for fail probe:\n%s", mode, err, output)
			}
			for _, marker := range []string{"Temporal logs (", "buffered-application-marker", "buffered-debug-marker", "buffered-info-marker", "buffered-warning-marker", "buffered-error-marker", "buffered-cleanup-marker"} {
				if got := strings.Contains(string(output), marker); got != (mode == "fail") {
					t.Errorf("probe %s: marker %q present %t, want %t:\n%s", mode, marker, got, mode == "fail", output)
				}
			}
		})
	}
}

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

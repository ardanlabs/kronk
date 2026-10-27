package tmprl_test

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// Environment owns an isolated test client, queue, and failure-only log buffer.
type Environment struct {
	Client    client.Client
	TaskQueue string
	// Log captures application logs alongside client, worker, and workflow logs.
	Log applog.Logger
}

// newTestEnvironment connects to a ready namespace and registers cleanup.
// Tests own worker registration and must register worker.Stop with t.Cleanup
// after setup, so workers stop before the client closes. Shared Docker state is
// retained until make test-tmprl-down; only per-test resources are cleaned up.
// Logs are buffered per test and printed after cleanup only if the test fails.
func newTestEnvironment(t *testing.T) *Environment {
	t.Helper()
	var buf logBuffer
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("Temporal logs (%s):\n%s", t.Name(), buf.String())
		}
	})
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	address := os.Getenv("TEMPORAL_ADDRESS")
	local := address == ""
	if local {
		var err error
		address, err = ensureContainer(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}
	namespace := cmp.Or(os.Getenv("TEMPORAL_NAMESPACE"), "default")
	tracked := new(executions)
	var c client.Client
	var err error
	for {
		attempt, stop := context.WithTimeout(ctx, 2*time.Second)
		c, err = client.DialContext(attempt, client.Options{HostPort: address, Namespace: namespace, Logger: log.NewStructuredLogger(logger), Interceptors: []interceptor.ClientInterceptor{tracked}})
		if err == nil {
			_, err = c.CheckHealth(attempt, &client.CheckHealthRequest{})
			if err == nil {
				_, err = c.WorkflowService().DescribeNamespace(attempt, &workflowservice.DescribeNamespaceRequest{Namespace: namespace})
			}
			if err != nil {
				c.Close()
			}
		}
		stop()
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			if local {
				logs, done := context.WithTimeout(context.Background(), 5*time.Second)
				output, _ := docker(logs, "logs", "--tail", "80", containerName)
				done()
				t.Logf("Temporal container logs:\n%s", output)
			}
			t.Fatalf("Temporal %s namespace %s not ready: %v: %v", address, namespace, ctx.Err(), err)
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Cleanup(func() {
		defer c.Close()
		tracked.mu.Lock()
		runs := append([]client.WorkflowRun(nil), tracked.runs...)
		tracked.mu.Unlock()
		for _, run := range runs {
			cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.TerminateWorkflow(cleanup, run.GetID(), run.GetRunID(), "test cleanup")
			done()
			var notFound *serviceerror.NotFound
			if err != nil && !errors.As(err, &notFound) {
				t.Errorf("cleaning workflow %s: %v", run.GetID(), err)
			}
		}
	})
	logger.InfoContext(ctx, "Temporal test server", "address", address, "namespace", namespace)
	env := Environment{Client: c, TaskQueue: "moxie-test-" + rand.Text(), Log: logger.InfoContext}
	return &env
}

// =============================================================================

// logBuffer synchronizes concurrent logging with cleanup snapshots.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// =============================================================================

// Track actual submitted runs rather than relying on eventually consistent
// visibility queries to discover executions that need cleanup.
type executions struct {
	interceptor.ClientInterceptorBase
	mu   sync.Mutex
	runs []client.WorkflowRun
}

func (e *executions) InterceptClient(next interceptor.ClientOutboundInterceptor) interceptor.ClientOutboundInterceptor {
	return &executionOutbound{Next: next, executions: e}
}

type executionOutbound struct {
	interceptor.ClientOutboundInterceptorBase
	executions *executions
}

func (c *executionOutbound) ExecuteWorkflow(ctx context.Context, input *interceptor.ClientExecuteWorkflowInput) (client.WorkflowRun, error) {
	run, err := c.Next.ExecuteWorkflow(ctx, input)
	if err == nil {
		c.executions.mu.Lock()
		c.executions.runs = append(c.executions.runs, run)
		c.executions.mu.Unlock()
	}
	return run, err
}

// =============================================================================

func echo(_ workflow.Context, value string) (string, error) { return value, nil }

// TestBufferedLogs uses subprocesses because an intentionally failing subtest
// would also fail its parent. Both paths exercise the test environment.
func TestBufferedLogs(t *testing.T) {
	const probe = "MOXIE_TMPRL_LOG_PROBE"
	if mode := os.Getenv(probe); mode != "" {
		env := newTestEnvironment(t)
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
				env := newTestEnvironment(t)
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
	observer := newTestEnvironment(t)
	var run client.WorkflowRun
	t.Run("unfinished", func(t *testing.T) {
		env := newTestEnvironment(t)
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

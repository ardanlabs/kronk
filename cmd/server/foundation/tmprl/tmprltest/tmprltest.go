// Package tmprltest provides shared Temporal infrastructure and isolated test
// clients/queues, following the service repository's dbtest lifecycle. Direct
// go test runs bootstrap Docker automatically. Set TEMPORAL_ADDRESS to use an
// existing server instead; that server is never managed by this package.
package tmprltest

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/log"
)

// Environment owns an isolated test client, queue, and failure-only log buffer.
type Environment struct {
	Client    client.Client
	TaskQueue string
	// Log captures application logs alongside client, worker, and workflow logs.
	Log applog.Logger
}

// New connects to a ready namespace and registers client/workflow cleanup.
// Tests own worker registration and must register worker.Stop with t.Cleanup
// after New, so workers stop before the client closes. Shared Docker state is
// retained until make test-tmprl-down; only per-test resources are cleaned up.
// Logs are buffered per test and printed after cleanup only if the test fails.
func New(t *testing.T) *Environment {
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

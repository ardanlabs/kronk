package training_test

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
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

// Keep the image and container name in sync with the tmprl Make targets.
const image = "temporalio/temporal:1.9.1"
const containerName = "moxie-tmprl-test"
const ownerLabel = "com.ardanlabs.moxie.tmprl-test"

// Environment owns an isolated test client, queue, and failure-only log buffer.
type Environment struct {
	Client    client.Client
	TaskQueue string
	Log       applog.Logger
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

func docker(ctx context.Context, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker %v: %w: %s", args, err, output)
	}
	return output, nil
}

// ensureContainer leaves shared infrastructure running. Docker's unique name
// arbitrates creation across independent go test package processes.
func ensureContainer(ctx context.Context) (string, error) {
	if _, err := docker(ctx, "info", "--format", "{{.ServerVersion}}"); err != nil {
		return "", fmt.Errorf("start Docker before running Temporal tests: %w", err)
	}
	var lastErr error
	for range 10 {
		output, err := docker(ctx, "inspect", containerName)
		if err != nil {
			_, lastErr = docker(ctx, "run", "-d", "--name", containerName,
				"--label", ownerLabel+"=true", "-p", "127.0.0.1::7233", image,
				"server", "start-dev", "--ip", "0.0.0.0", "--headless",
				"--db-filename", "/tmp/temporal.db")
		} else {
			var containers []struct {
				Config struct {
					Image  string
					Labels map[string]string
				}
				State           struct{ Running bool }
				NetworkSettings struct {
					Ports map[string][]struct{ HostIP, HostPort string }
				}
			}
			if err := json.Unmarshal(output, &containers); err != nil {
				return "", err
			}
			if len(containers) != 1 {
				return "", fmt.Errorf("expected one %s container", containerName)
			}
			c := containers[0]
			if c.Config.Image != image || c.Config.Labels[ownerLabel] != "true" {
				return "", fmt.Errorf("container %s is not our %s test server; refusing to reuse or change it", containerName, image)
			}
			if c.State.Running {
				for _, binding := range c.NetworkSettings.Ports["7233/tcp"] {
					if binding.HostIP == "127.0.0.1" && binding.HostPort != "" {
						return net.JoinHostPort(binding.HostIP, binding.HostPort), nil
					}
				}
				lastErr = fmt.Errorf("container %s has no localhost Temporal port", containerName)
			} else {
				_, lastErr = docker(ctx, "start", containerName)
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("starting Temporal: %w (%v)", ctx.Err(), lastErr)
		case <-time.After(250 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("starting Temporal container: %v", lastErr)
}

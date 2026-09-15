package gomlx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/tokenizer"
)

const maximumServerLogBytes = 64 << 10

func startTokenizerServer(ctx context.Context, request finetune.Request, httpClient *http.Client) (*tokenizer.Client, func() error, error) {
	endpoint, stop, err := startLlamaServer(ctx, request, "", httpClient)
	if err != nil {
		return nil, nil, err
	}
	client, err := tokenizer.New(endpoint, httpClient)
	if err != nil {
		_ = stop()
		return nil, nil, err
	}
	return client, stop, nil
}

func startLlamaServer(ctx context.Context, request finetune.Request, adapter string, httpClient *http.Client) (string, func() error, error) {
	executable, err := resolveLlamaServer(request.Runtime.LlamaServer)
	if err != nil {
		return "", nil, err
	}
	port := request.Runtime.Port
	if port == 0 {
		port, err = availablePort()
		if err != nil {
			return "", nil, err
		}
	}
	contextSize := request.Runtime.ContextSize
	if contextSize == 0 {
		contextSize = 512
	}
	arguments := []string{
		"-m", request.Model,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
		"--ctx-size", strconv.Itoa(contextSize),
		"--n-gpu-layers", strconv.Itoa(request.Runtime.GPULayers),
	}
	if adapter != "" {
		arguments = append(arguments, "--lora", adapter)
	}
	serverContext, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(serverContext, executable, arguments...)
	logs := &limitedBuffer{maximum: maximumServerLogBytes}
	command.Stdout = logs
	command.Stderr = logs
	if err := command.Start(); err != nil {
		cancel()
		return "", nil, fmt.Errorf("start llama-server: %w", err)
	}
	process := newProcessWait(command)
	var stopOnce sync.Once
	var stopErr error
	stop := func() error {
		stopOnce.Do(func() {
			alreadyExited := process.finished()
			if !alreadyExited {
				cancel()
			}
			process.wait()
			err := process.err()
			_, expectedExit := errors.AsType[*exec.ExitError](err)
			if err != nil && (alreadyExited || !expectedExit) {
				stopErr = fmt.Errorf("stop llama-server: %w", err)
			}
		})
		return stopErr
	}

	endpoint := "http://127.0.0.1:" + strconv.Itoa(port)
	healthClient := *httpClient
	healthClient.Timeout = time.Second
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	startupTimer := time.NewTimer(time.Minute)
	defer startupTimer.Stop()
	for {
		ready, healthErr := llamaServerReady(ctx, &healthClient, endpoint)
		if ready {
			return endpoint, stop, nil
		}
		select {
		case <-process.done:
			cancel()
			return "", nil, processExitError(process.err(), logs.String())
		case <-ctx.Done():
			_ = stop()
			return "", nil, context.Cause(ctx)
		case <-startupTimer.C:
			_ = stop()
			return "", nil, fmt.Errorf("llama-server did not become ready: %v: %s", healthErr, logs.String())
		case <-ticker.C:
		}
	}
}

type processWait struct {
	done    chan struct{}
	mu      sync.Mutex
	waitErr error
}

func newProcessWait(command *exec.Cmd) *processWait {
	process := &processWait{done: make(chan struct{})}
	go func() {
		err := command.Wait()
		process.mu.Lock()
		process.waitErr = err
		process.mu.Unlock()
		close(process.done)
	}()
	return process
}

func (process *processWait) finished() bool {
	select {
	case <-process.done:
		return true
	default:
		return false
	}
}

func (process *processWait) wait() {
	<-process.done
}

func (process *processWait) err() error {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.waitErr
}

func processExitError(err error, logs string) error {
	if err == nil {
		return fmt.Errorf("llama-server exited before becoming ready: %s", logs)
	}
	return fmt.Errorf("llama-server exited before becoming ready: %w: %s", err, logs)
}

func resolveLlamaServer(configured string) (string, error) {
	name := configured
	if name == "" {
		name = "llama-server"
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("find llama-server %q: %w", name, err)
	}
	return path, nil
}

func availablePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve llama-server port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, fmt.Errorf("release llama-server port: %w", err)
	}
	return port, nil
}

func llamaServerReady(ctx context.Context, client *http.Client, endpoint string) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/health", nil)
	if err != nil {
		return false, err
	}
	response, err := client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	return response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices, nil
}

type limitedBuffer struct {
	mu      sync.Mutex
	maximum int
	data    strings.Builder
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	remaining := buffer.maximum - buffer.data.Len()
	if remaining > 0 {
		_, _ = buffer.data.Write(data[:min(remaining, len(data))])
	}
	return len(data), nil
}

func (buffer *limitedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return strings.TrimSpace(buffer.data.String())
}

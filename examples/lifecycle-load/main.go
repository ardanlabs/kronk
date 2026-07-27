// This example exercises Kronk's four-stage request lifecycle through a running
// HTTP server. It is a load and timeout diagnostic, not an in-process SDK test.
// The client verifies externally visible behavior while the server's structured
// request-lifecycle logs provide the authoritative stage-level evidence.
//
// The scenario uses one execution slot and two admission permits:
//
//  1. The holder request passes Stages 1-3, enters Stage 4, and keeps the only
//     execution slot occupied.
//  2. The queued request consumes the second admission permit and waits for the
//     occupied slot. Its 300 ms client deadline expires before it receives any
//     inference data. The server should record a Stage 3 cancel because an HTTP
//     client deadline reaches the server as request cancellation.
//  3. The blocked request finds both admission permits occupied. The server's
//     100 ms admission timeout expires, so the server should record a Stage 1
//     timeout with capacity=2 and admitted=2.
//  4. The client cancels the holder, and the server should record Stage 4 cancel
//     and release the slot, stream, and admission permit.
//
// Requirements:
//
//   - Run a Kronk server built from the current source so it includes the
//     request-lifecycle instrumentation.
//
//   - Install the selected model and configure its active model-config entry as
//     shown below. The key must match the model ID sent by this program.
//
//   - Restart the server after changing model configuration.
//
//     Qwen3-0.6B-Q8_0:
//     nseq-max: 1
//     queue-depth: 2
//     admission-timeout: 100ms
//
// Installed servers use ~/.kronk/models/model_config.yaml by default. The
// repository's make kronk-server target uses zarf/kms/model_config.yaml.
//
// Optional environment variables:
//
//   - KRONK_WEB_API_HOST overrides http://localhost:11435.
//   - KRONK_TOKEN supplies the bearer token when inference auth is enabled.
//   - KRONK_LIFECYCLE_MODEL overrides Qwen3-0.6B-Q8_0; configure the matching
//     model ID with the same lifecycle settings above.
//
// Run the example from the root of the project:
//
//	make example-lifecycle-load
//
// After the client reports success, correlate the printed holder, queued, and
// blocked trace IDs with request-lifecycle events in the Kronk server logs.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultHost              = "http://localhost:11435"
	defaultModel             = "Qwen3-0.6B-Q8_0"
	expectedAdmissionTimeout = 100 * time.Millisecond
	queuedTimeout            = 300 * time.Millisecond
	requestWait              = 30 * time.Second
	holderMaxTokens          = 8192
	contenderMaxTokens       = 64

	holderTrace  = "11111111111111111111111111111111"
	queuedTrace  = "22222222222222222222222222222222"
	blockedTrace = "33333333333333333333333333333333"
)

type config struct {
	endpoint string
	model    string
	token    string
}

type requestResult struct {
	status  int
	elapsed time.Duration
	err     error
}

type serverError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (se *serverError) Error() string {
	return fmt.Sprintf("HTTP error %s: %s", se.Code, se.Message)
}

type chatEvent struct {
	Choices []struct {
		Delta *struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type runningRequest struct {
	headers    <-chan struct{}
	firstEvent <-chan struct{}
	done       <-chan requestResult
}

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	printConfiguration(cfg)

	waitCtx, cancelWait := context.WithTimeout(context.Background(), requestWait)
	defer cancelWait()

	client := &http.Client{}

	holderCtx, cancelHolder := context.WithCancel(context.Background())
	defer cancelHolder()
	holder := startRequest(holderCtx, client, cfg, holderTrace, holderMaxTokens)
	if err := waitForFirstEvent(waitCtx, holder); err != nil {
		return fmt.Errorf("holder did not begin streaming: %w", err)
	}
	fmt.Println("\nPASS: holder is streaming from the Kronk server")

	queuedCtx, cancelQueued := context.WithTimeout(context.Background(), queuedTimeout)
	defer cancelQueued()
	queued := startRequest(queuedCtx, client, cfg, queuedTrace, contenderMaxTokens)
	if err := waitForHeaders(waitCtx, queued); err != nil {
		return fmt.Errorf("queued request was not admitted by the server: %w", err)
	}
	fmt.Println("PASS: second request was admitted while the only slot remained occupied")

	blockedCtx, cancelBlocked := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelBlocked()
	blocked := startRequest(blockedCtx, client, cfg, blockedTrace, contenderMaxTokens)

	blockedResult, admitted, err := waitForAdmissionResult(waitCtx, blocked)
	if err != nil {
		return err
	}
	if admitted {
		cancelBlocked()
		return fmt.Errorf("third request was admitted; configure %s with nseq-max: 1, queue-depth: 2, and admission-timeout: %s, then restart the server", cfg.model, expectedAdmissionTimeout)
	}
	if blockedResult.status != http.StatusInternalServerError {
		return fmt.Errorf("third request status: got %d after %s, want %d from admission timeout: %v",
			blockedResult.status, blockedResult.elapsed.Round(time.Millisecond), http.StatusInternalServerError, blockedResult.err)
	}
	var responseErr *serverError
	if !errors.As(blockedResult.err, &responseErr) || responseErr.Code != "internal" ||
		!strings.Contains(responseErr.Message, "chat-streaming-http: stream-response: context deadline exceeded") {
		return fmt.Errorf("third request did not return the expected admission deadline error: %v", blockedResult.err)
	}
	if blockedResult.elapsed < expectedAdmissionTimeout || blockedResult.elapsed > 500*time.Millisecond {
		return fmt.Errorf("third request did not fail within the expected admission timeout window: %s: %v",
			blockedResult.elapsed.Round(time.Millisecond), blockedResult.err)
	}
	fmt.Printf("PASS: third request received the server admission timeout after %s: %v\n",
		blockedResult.elapsed.Round(time.Millisecond), blockedResult.err)

	queuedResult, err := waitForQueuedDeadline(waitCtx, queued, holder)
	if err != nil {
		return err
	}
	if !errors.Is(queuedResult.err, context.DeadlineExceeded) {
		return fmt.Errorf("queued request: got %v, want client deadline exceeded", queuedResult.err)
	}
	fmt.Printf("PASS: second request's client deadline expired after %s before it received inference data\n",
		queuedResult.elapsed.Round(time.Millisecond))

	select {
	case holderResult := <-holder.done:
		return fmt.Errorf("holder finished before cancellation: %v", holderResult.err)
	default:
	}

	cancelHolder()
	holderResult, err := awaitResult(waitCtx, holder.done)
	if err != nil {
		return fmt.Errorf("wait for holder request: %w", err)
	}
	if !errors.Is(holderResult.err, context.Canceled) {
		return fmt.Errorf("holder cancellation: got %v, want %v", holderResult.err, context.Canceled)
	}
	fmt.Printf("PASS: holder was canceled during server inference after %s\n",
		holderResult.elapsed.Round(time.Millisecond))

	fmt.Println("\nPASS: server lifecycle load scenario completed")
	fmt.Println("Confirm these request-lifecycle events in the Kronk server logs:")
	fmt.Println("- holder : Stage 4 started, then Stage 4 cancel")
	fmt.Println("- queued : Stage 3 queued, then Stage 3 cancel, with no Stage 4 started")
	fmt.Println("- blocked: Stage 1 timeout with capacity 2 and admitted 2")
	return nil
}

func loadConfig() (config, error) {
	host := strings.TrimSpace(os.Getenv("KRONK_WEB_API_HOST"))
	if host == "" {
		host = defaultHost
	}
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}

	endpoint, err := url.JoinPath(host, "/v1/chat/completions")
	if err != nil {
		return config{}, fmt.Errorf("build server endpoint: %w", err)
	}
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		return config{}, fmt.Errorf("parse server endpoint %q: %w", endpoint, err)
	}

	modelID := strings.TrimSpace(os.Getenv("KRONK_LIFECYCLE_MODEL"))
	if modelID == "" {
		modelID = defaultModel
	}

	return config{
		endpoint: endpoint,
		model:    modelID,
		token:    strings.TrimSpace(os.Getenv("KRONK_TOKEN")),
	}, nil
}

func printConfiguration(cfg config) {
	fmt.Println("Kronk server lifecycle load configuration")
	fmt.Println("- endpoint          :", cfg.endpoint)
	fmt.Println("- model             :", cfg.model)
	fmt.Println("- authentication    :", map[bool]string{true: "KRONK_TOKEN", false: "disabled"}[cfg.token != ""])
	fmt.Println("- expected slots    : 1")
	fmt.Println("- expected queue    : 2")
	fmt.Println("- expected admission:", expectedAdmissionTimeout)
	fmt.Println("- client queue limit:", queuedTimeout)
	fmt.Println("- holder trace      :", holderTrace)
	fmt.Println("- queued trace      :", queuedTrace)
	fmt.Println("- blocked trace     :", blockedTrace)
	fmt.Printf("\nRequired active server model configuration (restart after changing it):\n%s:\n  nseq-max: 1\n  queue-depth: 2\n  admission-timeout: %s\n",
		cfg.model, expectedAdmissionTimeout)
}

func startRequest(ctx context.Context, client *http.Client, cfg config, traceID string, maxTokens int) runningRequest {
	headers := make(chan struct{})
	firstEvent := make(chan struct{})
	done := make(chan requestResult, 1)

	go func() {
		started := time.Now()
		status, err := streamRequest(ctx, client, cfg, traceID, maxTokens, headers, firstEvent)
		done <- requestResult{status: status, elapsed: time.Since(started), err: err}
	}()

	return runningRequest{headers: headers, firstEvent: firstEvent, done: done}
}

func streamRequest(ctx context.Context, client *http.Client, cfg config, traceID string, maxTokens int, headers chan<- struct{}, firstEvent chan<- struct{}) (int, error) {
	body := map[string]any{
		"model": cfg.model,
		"messages": []map[string]any{
			{"role": "user", "content": "Write the integers from 1 through 10000, one per line. Do not summarize or stop early."},
		},
		"enable_thinking": false,
		"temperature":     0.0,
		"max_tokens":      maxTokens,
		"stream":          true,
	}

	var data bytes.Buffer
	if err := json.NewEncoder(&data).Encode(body); err != nil {
		return 0, fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.endpoint, &data)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Traceparent", fmt.Sprintf("00-%s-%s-01", traceID, traceID[:16]))
	if cfg.token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.token)
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return 0, ctxErr
		}
		return 0, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		response, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if readErr != nil {
			return resp.StatusCode, fmt.Errorf("read HTTP %d response: %w", resp.StatusCode, readErr)
		}

		var responseErr serverError
		if err := json.Unmarshal(response, &responseErr); err != nil {
			return resp.StatusCode, fmt.Errorf("decode HTTP %d response %q: %w", resp.StatusCode, strings.TrimSpace(string(response)), err)
		}
		return resp.StatusCode, &responseErr
	}
	close(headers)

	var first sync.Once
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if line == "data: [DONE]" {
			return resp.StatusCode, nil
		}

		var event chatEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			return resp.StatusCode, fmt.Errorf("decode chat event: %w", err)
		}
		if len(event.Choices) == 0 {
			return resp.StatusCode, errors.New("chat event contained no choices")
		}
		if event.Choices[0].FinishReason != nil && *event.Choices[0].FinishReason == "error" {
			message := "model returned an error event"
			if event.Choices[0].Delta != nil && event.Choices[0].Delta.Content != "" {
				message = event.Choices[0].Delta.Content
			}
			return resp.StatusCode, errors.New(message)
		}
		first.Do(func() { close(firstEvent) })
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return resp.StatusCode, ctxErr
	}
	if err := scanner.Err(); err != nil {
		return resp.StatusCode, fmt.Errorf("read event stream: %w", err)
	}
	return resp.StatusCode, io.ErrUnexpectedEOF
}

func waitForHeaders(ctx context.Context, req runningRequest) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-req.done:
		return fmt.Errorf("request ended before HTTP 200 headers: %v", result.err)
	case <-req.headers:
		return nil
	}
}

func waitForFirstEvent(ctx context.Context, req runningRequest) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-req.done:
		return fmt.Errorf("request ended before its first event: %v", result.err)
	case <-req.firstEvent:
		return nil
	}
}

func waitForAdmissionResult(ctx context.Context, req runningRequest) (requestResult, bool, error) {
	select {
	case <-ctx.Done():
		return requestResult{}, false, fmt.Errorf("wait for admission result: %w", ctx.Err())
	case result := <-req.done:
		return result, false, nil
	case <-req.headers:
		return requestResult{}, true, nil
	}
}

func waitForQueuedDeadline(ctx context.Context, queued runningRequest, holder runningRequest) (requestResult, error) {
	select {
	case <-ctx.Done():
		return requestResult{}, fmt.Errorf("wait for queued request: %w", ctx.Err())
	case result := <-holder.done:
		return requestResult{}, fmt.Errorf("holder finished before the queued deadline: %v", result.err)
	case <-queued.firstEvent:
		return requestResult{}, errors.New("queued request received inference data before its deadline")
	case result := <-queued.done:
		select {
		case <-queued.firstEvent:
			return requestResult{}, errors.New("queued request received inference data before its deadline")
		default:
		}
		select {
		case holderResult := <-holder.done:
			return requestResult{}, fmt.Errorf("holder finished before the queued deadline: %v", holderResult.err)
		default:
		}
		return result, nil
	}
}

func awaitResult(ctx context.Context, result <-chan requestResult) (requestResult, error) {
	select {
	case <-ctx.Done():
		return requestResult{}, ctx.Err()
	case rr := <-result:
		return rr, nil
	}
}

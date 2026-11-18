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
//  4. The client cancels the holder, and the server should record Stage 4 cancel.
//  5. A recovery request completes, proving the slot and admission permit were
//     released after cancellation.
//
// Before canceling the holder, a replacement queued request and another blocked
// request repeat the admission checks, proving queued cancellation and admission
// timeout do not leak permits. Three cycles run by default with unique trace IDs.
// A full PASS requires both client behavior and matching server-stage evidence.
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
//     unsloth/Qwen3-0.6B-Q8_0:
//     nseq-max: 1
//     queue-depth: 2
//     admission-timeout: 100ms
//
// Installed servers use ~/.kronk/models/model_config.yaml by default. The
// source-based reliability workflow selects
// zarf/kms/tools_model_config.yaml explicitly; see .make/tools.mk.
//
// Optional environment variables:
//
//   - KRONK_WEB_API_HOST overrides http://localhost:11435.
//   - KRONK_TOKEN supplies the bearer token when inference auth is enabled.
//   - KRONK_LIFECYCLE_MODEL overrides unsloth/Qwen3-0.6B-Q8_0; configure the
//     matching model ID with the same lifecycle settings above.
//   - KRONK_LIFECYCLE_OUT overrides the artifact parent .tools/lifecycle-load/output.
//   - KRONK_LIFECYCLE_CYCLES overrides the default of 3 (positive integer).
//   - KRONK_SERVER_LOG overrides ~/.kronk/kronk.log.
//
// Run the example from the root of the project:
//
//	make example-lifecycle-load
//
// Each invocation creates a unique timestamped directory under the artifact
// parent containing summary.json, events.ndjson, and tool.log. Previous runs
// are preserved. The actual directory is printed at startup and completion.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"uuid"
)

const (
	defaultHost              = "http://localhost:11435"
	defaultModel             = "unsloth/Qwen3-0.6B-Q8_0"
	defaultOutput            = ".tools/lifecycle-load/output"
	expectedAdmissionTimeout = 100 * time.Millisecond
	queuedTimeout            = 300 * time.Millisecond
	requestWait              = 30 * time.Second
	holderMaxTokens          = 8192
	contenderMaxTokens       = 64
	defaultCycles            = 3
	evidenceWait             = 2 * time.Second
)

type config struct {
	endpoint  string
	model     string
	token     string
	output    string
	serverLog string
	cycles    int
}

type requestResult struct {
	status  int
	elapsed time.Duration
	err     error
}

type requestSummary struct {
	TraceID        string  `json:"trace_id"`
	Status         int     `json:"http_status"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	Outcome        string  `json:"outcome"`
}

type lifecycleSummary struct {
	Status          string                    `json:"status"`
	StartedAt       time.Time                 `json:"started_at"`
	FinishedAt      time.Time                 `json:"finished_at"`
	Endpoint        string                    `json:"endpoint"`
	Model           string                    `json:"model"`
	Cycles          int                       `json:"cycles"`
	CompletedCycles int                       `json:"completed_cycles"`
	Requests        map[string]requestSummary `json:"requests"`
	ServerEvidence  lifecycleServerEvidence   `json:"server_evidence"`
	Error           string                    `json:"error,omitempty"`
}

type lifecycleServerEvidence struct {
	Available     bool   `json:"available"`
	LogPath       string `json:"log_path"`
	StartOffset   int64  `json:"start_offset"`
	BytesScanned  int64  `json:"bytes_scanned"`
	MatchedEvents int    `json:"matched_events"`
	Note          string `json:"note,omitempty"`
}

type lifecycleArtifacts struct {
	dir       string
	serverLog string
	offset    int64
	available bool
	toolLog   *os.File
	events    *os.File
	traces    map[string]string
	records   map[string][]map[string]any
}

type serverError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (se *serverError) Error() string {
	return fmt.Sprintf("HTTP error %s: %s", se.Code, se.Message)
}

type serverErrorResponse struct {
	Error serverError `json:"error"`
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
	done       <-chan struct{}
	result     *requestResult
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
	artifacts, err := newLifecycleArtifacts(cfg.output, cfg.serverLog)
	if err != nil {
		return err
	}
	defer artifacts.close()

	summary := lifecycleSummary{
		Status:    "FAIL",
		StartedAt: time.Now().UTC(),
		Endpoint:  cfg.endpoint,
		Model:     cfg.model,
		Cycles:    cfg.cycles,
		Requests:  make(map[string]requestSummary),
	}
	err = runLifecycle(cfg, artifacts, &summary)
	summary.ServerEvidence = artifacts.collectServerEvidence()
	if err == nil {
		err = artifacts.verifyServerEvidence(&summary)
	}
	if err == nil {
		summary.Status = "PASS"
		artifacts.logf("\nPASS: %d lifecycle cycles verified against client behavior and server stages", summary.CompletedCycles)
	} else {
		summary.Error = err.Error()
		artifacts.logf("%s: %v", summary.Status, err)
	}
	summary.FinishedAt = time.Now().UTC()
	if writeErr := artifacts.writeSummary(summary); writeErr != nil {
		return errors.Join(err, writeErr)
	}
	artifacts.logf("artifacts: %s", artifacts.dir)
	return err
}

func runLifecycle(cfg config, artifacts *lifecycleArtifacts, summary *lifecycleSummary) error {
	printConfiguration(cfg, artifacts)
	client := &http.Client{}
	defer client.CloseIdleConnections()
	for cycle := 1; cycle <= cfg.cycles; cycle++ {
		traces := make(map[string]string)
		artifacts.logf("\nCycle %d/%d", cycle, cfg.cycles)
		for _, role := range []string{"holder", "queued", "blocked", "replacement", "blocked-again", "recovery"} {
			trace := strings.ReplaceAll(uuid.NewV4().String(), "-", "")
			traces[role] = trace
			artifacts.traces[trace] = fmt.Sprintf("cycle-%d/%s", cycle, role)
			artifacts.logf("- %s trace: %s", role, trace)
		}
		if err := runCycle(cfg, client, artifacts, summary, traces); err != nil {
			return fmt.Errorf("cycle %d: %w", cycle, err)
		}
		summary.CompletedCycles++
	}
	return nil
}

func runCycle(cfg config, client *http.Client, artifacts *lifecycleArtifacts, summary *lifecycleSummary, traces map[string]string) error {
	waitCtx, cancelWait := context.WithTimeout(context.Background(), requestWait)
	defer cancelWait()

	holderCtx, cancelHolder := context.WithCancel(waitCtx)
	defer cancelHolder()
	holder := startRequest(holderCtx, client, cfg, traces["holder"], holderMaxTokens)
	if err := waitForFirstEvent(waitCtx, holder); err != nil {
		return fmt.Errorf("holder did not begin streaming: %w", err)
	}
	artifacts.logf("PASS: holder produced content and remains active")

	for _, pair := range [][2]string{{"queued", "blocked"}, {"replacement", "blocked-again"}} {
		queuedRole, blockedRole := pair[0], pair[1]
		queuedCtx, cancelQueued := context.WithTimeout(waitCtx, queuedTimeout)
		defer cancelQueued()
		queued := startRequest(queuedCtx, client, cfg, traces[queuedRole], contenderMaxTokens)
		if err := waitForHeaders(waitCtx, queued); err != nil {
			return fmt.Errorf("%s was not admitted while holder remained active: %w", queuedRole, err)
		}
		artifacts.logf("PASS: %s admitted while holder remained active", queuedRole)

		blockedCtx, cancelBlocked := context.WithTimeout(waitCtx, 2*time.Second)
		defer cancelBlocked()
		blocked := startRequest(blockedCtx, client, cfg, traces[blockedRole], contenderMaxTokens)
		blockedResult, admitted, err := waitForAdmissionResult(waitCtx, blocked)
		if err != nil {
			return err
		}
		if admitted {
			return fmt.Errorf("%s was admitted; configure %s with nseq-max: 1, queue-depth: 2, and admission-timeout: %s, then restart the server", blockedRole, cfg.model, expectedAdmissionTimeout)
		}
		if blockedResult.status != http.StatusTooManyRequests {
			return fmt.Errorf("%s status: got %d after %s, want %d from admission timeout: %v",
				blockedRole, blockedResult.status, blockedResult.elapsed.Round(time.Millisecond), http.StatusTooManyRequests, blockedResult.err)
		}
		responseErr, ok := errors.AsType[*serverError](blockedResult.err)
		if !ok || responseErr.Code != "resource_exhausted" {
			return fmt.Errorf("%s did not return the expected admission deadline error: %v", blockedRole, blockedResult.err)
		}
		if blockedResult.elapsed < expectedAdmissionTimeout || blockedResult.elapsed > 500*time.Millisecond {
			return fmt.Errorf("%s did not fail within the expected admission timeout window: %s: %v",
				blockedRole, blockedResult.elapsed.Round(time.Millisecond), blockedResult.err)
		}
		trace := traces[blockedRole]
		summary.Requests[artifacts.traces[trace]] = summarizeRequest(trace, blockedResult, "stage-1-timeout")
		artifacts.logf("PASS: %s received resource_exhausted after %s", blockedRole, blockedResult.elapsed.Round(time.Millisecond))

		queuedResult, err := waitForQueuedDeadline(waitCtx, queued, holder)
		if err != nil {
			return err
		}
		if !errors.Is(queuedResult.err, context.DeadlineExceeded) {
			return fmt.Errorf("%s: got %v, want client deadline exceeded", queuedRole, queuedResult.err)
		}
		trace = traces[queuedRole]
		summary.Requests[artifacts.traces[trace]] = summarizeRequest(trace, queuedResult, "stage-3-cancel")
		artifacts.logf("PASS: %s deadline expired without generated content while holder remained active", queuedRole)
	}

	select {
	case <-holder.done:
		return fmt.Errorf("holder finished before cancellation: %v", holder.result.err)
	default:
	}

	cancelHolder()
	holderResult, err := awaitResult(waitCtx, holder)
	if err != nil {
		return fmt.Errorf("wait for holder request: %w", err)
	}
	if !errors.Is(holderResult.err, context.Canceled) {
		return fmt.Errorf("holder cancellation: got %v, want %v", holderResult.err, context.Canceled)
	}
	trace := traces["holder"]
	summary.Requests[artifacts.traces[trace]] = summarizeRequest(trace, holderResult, "stage-4-cancel")
	artifacts.logf("PASS: holder was canceled during server inference after %s",
		holderResult.elapsed.Round(time.Millisecond))

	recovery := startRequest(waitCtx, client, cfg, traces["recovery"], contenderMaxTokens)
	if err := waitForFirstEvent(waitCtx, recovery); err != nil {
		return fmt.Errorf("recovery request did not begin streaming: %w", err)
	}
	recoveryResult, err := awaitResult(waitCtx, recovery)
	if err != nil {
		return fmt.Errorf("wait for recovery request: %w", err)
	}
	if recoveryResult.status != http.StatusOK || recoveryResult.err != nil {
		return fmt.Errorf("recovery request failed after cancellation: status=%d err=%v", recoveryResult.status, recoveryResult.err)
	}
	trace = traces["recovery"]
	summary.Requests[artifacts.traces[trace]] = summarizeRequest(trace, recoveryResult, "completed-after-release")
	artifacts.logf("PASS: recovery request completed after holder cancellation in %s", recoveryResult.elapsed.Round(time.Millisecond))
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
	home, _ := os.UserHomeDir()
	output := strings.TrimSpace(os.Getenv("KRONK_LIFECYCLE_OUT"))
	if output == "" {
		output = defaultOutput
	}
	serverLog := strings.TrimSpace(os.Getenv("KRONK_SERVER_LOG"))
	if serverLog == "" {
		serverLog = filepath.Join(home, ".kronk", "kronk.log")
	}
	cycles := defaultCycles
	if value := strings.TrimSpace(os.Getenv("KRONK_LIFECYCLE_CYCLES")); value != "" {
		cycles, err = strconv.Atoi(value)
		if err != nil || cycles < 1 {
			return config{}, errors.New("KRONK_LIFECYCLE_CYCLES must be a positive integer")
		}
	}

	return config{
		endpoint:  endpoint,
		model:     modelID,
		token:     strings.TrimSpace(os.Getenv("KRONK_TOKEN")),
		output:    output,
		serverLog: serverLog,
		cycles:    cycles,
	}, nil
}

func printConfiguration(cfg config, artifacts *lifecycleArtifacts) {
	artifacts.logf("Kronk server lifecycle load configuration")
	artifacts.logf("- artifacts         : %s", artifacts.dir)
	artifacts.logf("- endpoint          : %s", cfg.endpoint)
	artifacts.logf("- model             : %s", cfg.model)
	artifacts.logf("- cycles            : %d", cfg.cycles)
	artifacts.logf("- server log        : %s", cfg.serverLog)
	artifacts.logf("- authentication    : %s", map[bool]string{true: "KRONK_TOKEN", false: "disabled"}[cfg.token != ""])
	artifacts.logf("- expected slots    : 1")
	artifacts.logf("- expected queue    : 2")
	artifacts.logf("- expected admission: %s", expectedAdmissionTimeout)
	artifacts.logf("- client queue limit: %s", queuedTimeout)
	artifacts.logf("\nRequired active server model configuration (restart after changing it):\n%s:\n  nseq-max: 1\n  queue-depth: 2\n  admission-timeout: %s",
		cfg.model, expectedAdmissionTimeout)
}

func startRequest(ctx context.Context, client *http.Client, cfg config, traceID string, maxTokens int) runningRequest {
	headers := make(chan struct{})
	firstEvent := make(chan struct{})
	done := make(chan struct{})
	result := new(requestResult)

	go func() {
		started := time.Now()
		status, err := streamRequest(ctx, client, cfg, traceID, maxTokens, headers, firstEvent)
		*result = requestResult{status: status, elapsed: time.Since(started), err: err}
		close(done)
	}()

	return runningRequest{headers: headers, firstEvent: firstEvent, done: done, result: result}
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

		var responseErr serverErrorResponse
		if err := json.Unmarshal(response, &responseErr); err != nil {
			return resp.StatusCode, fmt.Errorf("decode HTTP %d response %q: %w", resp.StatusCode, strings.TrimSpace(string(response)), err)
		}
		return resp.StatusCode, &responseErr.Error
	}
	close(headers)

	first := sync.OnceFunc(func() { close(firstEvent) })
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
		if event.Choices[0].Delta != nil && event.Choices[0].Delta.Content != "" && event.Choices[0].FinishReason == nil {
			first()
		}
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
	case <-req.done:
		select {
		case <-req.headers:
			return nil
		default:
			return fmt.Errorf("request ended before HTTP 200 headers: %v", req.result.err)
		}
	case <-req.headers:
		return nil
	}
}

func waitForFirstEvent(ctx context.Context, req runningRequest) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-req.done:
		select {
		case <-req.firstEvent:
			return nil
		default:
			return fmt.Errorf("request ended before generated content: %v", req.result.err)
		}
	case <-req.firstEvent:
		return nil
	}
}

func waitForAdmissionResult(ctx context.Context, req runningRequest) (requestResult, bool, error) {
	select {
	case <-ctx.Done():
		return requestResult{}, false, fmt.Errorf("wait for admission result: %w", ctx.Err())
	case <-req.done:
		return *req.result, false, nil
	case <-req.headers:
		return requestResult{}, true, nil
	}
}

func waitForQueuedDeadline(ctx context.Context, queued runningRequest, holder runningRequest) (requestResult, error) {
	select {
	case <-ctx.Done():
		return requestResult{}, fmt.Errorf("wait for queued request: %w", ctx.Err())
	case <-holder.done:
		return requestResult{}, fmt.Errorf("holder finished before the queued deadline: %v", holder.result.err)
	case <-queued.firstEvent:
		return requestResult{}, errors.New("queued request received inference data before its deadline")
	case <-queued.done:
		select {
		case <-queued.firstEvent:
			return requestResult{}, errors.New("queued request received inference data before its deadline")
		default:
		}
		select {
		case <-holder.done:
			return requestResult{}, fmt.Errorf("holder finished before the queued deadline: %v", holder.result.err)
		default:
		}
		return *queued.result, nil
	}
}

func awaitResult(ctx context.Context, req runningRequest) (requestResult, error) {
	select {
	case <-ctx.Done():
		return requestResult{}, ctx.Err()
	case <-req.done:
		return *req.result, nil
	}
}

func summarizeRequest(traceID string, result requestResult, outcome string) requestSummary {
	return requestSummary{
		TraceID:        traceID,
		Status:         result.status,
		ElapsedSeconds: float64(result.elapsed.Round(time.Millisecond)) / float64(time.Second),
		Outcome:        outcome,
	}
}

func newLifecycleArtifacts(dir, serverLog string) (*lifecycleArtifacts, error) {
	var offset int64
	available := false
	note := ""
	info, err := os.Stat(serverLog)
	switch {
	case err == nil:
		offset, available = info.Size(), true
	case errors.Is(err, os.ErrNotExist):
		note = "server log does not exist: " + serverLog
	default:
		note = "cannot inspect server log: " + err.Error()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create lifecycle output: %w", err)
	}
	dir, err = os.MkdirTemp(dir, time.Now().Format("20060102-150405")+"-*")
	if err != nil {
		return nil, fmt.Errorf("create lifecycle run directory: %w", err)
	}
	toolLog, err := os.Create(filepath.Join(dir, "tool.log"))
	if err != nil {
		return nil, fmt.Errorf("create lifecycle tool log: %w", err)
	}
	events, err := os.Create(filepath.Join(dir, "events.ndjson"))
	if err != nil {
		toolLog.Close()
		return nil, fmt.Errorf("create lifecycle event log: %w", err)
	}
	artifacts := &lifecycleArtifacts{
		dir: dir, serverLog: serverLog, offset: offset, available: available,
		toolLog: toolLog, events: events,
		traces: make(map[string]string), records: make(map[string][]map[string]any),
	}
	if note != "" {
		artifacts.logf("server evidence note: %s", note)
	}
	return artifacts, nil
}

func (artifacts *lifecycleArtifacts) close() {
	artifacts.toolLog.Close()
	artifacts.events.Close()
}

func (artifacts *lifecycleArtifacts) logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	fmt.Println(line)
	fmt.Fprintln(artifacts.toolLog, line)
}

func (artifacts *lifecycleArtifacts) writeSummary(summary lifecycleSummary) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lifecycle summary: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(artifacts.dir, "summary.json"), data, 0o644); err != nil {
		return fmt.Errorf("write lifecycle summary: %w", err)
	}
	return nil
}

func (artifacts *lifecycleArtifacts) collectServerEvidence() lifecycleServerEvidence {
	evidence := lifecycleServerEvidence{
		Available: artifacts.available, LogPath: artifacts.serverLog, StartOffset: artifacts.offset,
	}
	clear(artifacts.records)
	if err := artifacts.events.Truncate(0); err != nil {
		evidence.Note = "reset lifecycle evidence: " + err.Error()
		return evidence
	}
	if _, err := artifacts.events.Seek(0, io.SeekStart); err != nil {
		evidence.Note = "seek lifecycle evidence: " + err.Error()
		return evidence
	}
	if !artifacts.available {
		evidence.Note = "server log was unavailable when the probe started; no internal behavior is claimed"
		return evidence
	}
	file, err := os.Open(artifacts.serverLog)
	if err != nil {
		evidence.Available = false
		evidence.Note = "open server log after run: " + err.Error()
		return evidence
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		evidence.Available = false
		evidence.Note = "stat server log after run: " + err.Error()
		return evidence
	}
	if info.Size() < artifacts.offset {
		evidence.Available = false
		evidence.Note = "server log was truncated or replaced during the probe"
		return evidence
	}
	if _, err := file.Seek(artifacts.offset, io.SeekStart); err != nil {
		evidence.Available = false
		evidence.Note = "seek server log: " + err.Error()
		return evidence
	}
	evidence.BytesScanned = info.Size() - artifacts.offset
	encoder := json.NewEncoder(artifacts.events)
	scanner := bufio.NewScanner(io.LimitReader(file, evidence.BytesScanned))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		traceID, _ := event["trace_id"].(string)
		label, exists := artifacts.traces[traceID]
		if !exists {
			continue
		}
		wrapper := map[string]any{
			"scenario": "lifecycle", "source": "kronk-server", "request": label,
			"trace_id": traceID, "event": event,
		}
		if err := encoder.Encode(wrapper); err != nil {
			evidence.Note = "write lifecycle evidence: " + err.Error()
			return evidence
		}
		artifacts.records[label] = append(artifacts.records[label], event)
		evidence.MatchedEvents++
	}
	if err := scanner.Err(); err != nil {
		evidence.Note = "scan server log: " + err.Error()
	}
	if evidence.MatchedEvents == 0 && evidence.Note == "" {
		evidence.Note = "no server events matched this run's lifecycle trace IDs"
	}
	return evidence
}

// verifyServerEvidence allows late cancellation/cleanup logs to arrive, but does
// not turn missing instrumentation into a behavioral failure or a full PASS.
func (artifacts *lifecycleArtifacts) verifyServerEvidence(summary *lifecycleSummary) error {
	deadline := time.NewTimer(evidenceWait)
	defer deadline.Stop()
	tick := time.Tick(50 * time.Millisecond)
	for {
		var missing []string
		for _, label := range slices.Sorted(maps.Keys(summary.Requests)) {
			request := summary.Requests[label]
			gap, err := validateRequestEvidence(artifacts.records[label], request.Outcome)
			if err != nil {
				return fmt.Errorf("server evidence for %s: %w", label, err)
			}
			if gap != "" {
				missing = append(missing, label+": "+gap)
			}
		}
		if len(missing) == 0 && summary.ServerEvidence.Note == "" {
			return nil
		}
		select {
		case <-deadline.C:
			summary.Status = "INCOMPLETE"
			return fmt.Errorf("client checks passed but server evidence is incomplete: %s; %s; check LIFECYCLE_SERVER_LOG and JSON request-lifecycle logging",
				strings.Join(missing, "; "), summary.ServerEvidence.Note)
		case <-tick:
			summary.ServerEvidence = artifacts.collectServerEvidence()
		}
	}
}

func validateRequestEvidence(events []map[string]any, outcome string) (string, error) {
	want := []string{"1/started", "1/complete", "2/started", "2/complete", "3/started", "3/queued"}
	switch outcome {
	case "stage-1-timeout":
		want = []string{"1/started", "1/timeout"}
	case "stage-3-cancel":
		want = append(want, "3/cancel")
	case "stage-4-cancel":
		want = append(want, "3/complete", "4/started", "4/cancel")
	case "completed-after-release":
		want = append(want, "3/complete", "4/started", "4/complete")
	}
	seen := make(map[string]bool)
	last := -1
	var slot, seq any
	for _, event := range events {
		if event["msg"] != "request-lifecycle" {
			continue
		}
		key := fmt.Sprintf("%v/%v", event["stage"], event["status"])
		index := slices.Index(want, key)
		if index == -1 || seen[key] || index < last {
			return "", fmt.Errorf("unexpected, duplicate, or out-of-order stage %s for %s", key, outcome)
		}
		seen[key], last = true, index
		if event["stage"] == float64(1) {
			capacity, hasCapacity := event["capacity"].(float64)
			admitted, hasAdmitted := event["admitted"].(float64)
			if !hasCapacity || !hasAdmitted {
				return "admission capacity/admitted fields missing", nil
			}
			if capacity != 2 || admitted < 0 || admitted > capacity || key == "1/timeout" && admitted != 2 {
				return "", fmt.Errorf("admission capacity=%v admitted=%v, want capacity 2 and saturation on timeout", capacity, admitted)
			}
		}
		if event["stage"] == float64(4) {
			if key == "4/started" {
				slot, seq = event["slot"], event["seq"]
			}
			if slot == nil || seq == nil {
				return "execution slot/sequence fields missing", nil
			}
			if slot != float64(0) || event["slot"] != slot || event["seq"] != seq {
				return "", fmt.Errorf("execution did not retain the only slot and sequence: slot=%v seq=%v", event["slot"], event["seq"])
			}
		}
	}
	var missing []string
	for _, key := range want {
		if key == "3/queued" && outcome != "stage-3-cancel" {
			continue // Holder and recovery may acquire the slot without waiting.
		}
		if !seen[key] {
			missing = append(missing, key)
		}
	}
	return strings.Join(missing, ", "), nil
}

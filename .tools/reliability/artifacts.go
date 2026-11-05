package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	summaryFile = "summary.json"
	eventsFile  = "events.ndjson"
	toolLogFile = "tool.log"
)

type traceRegistry struct {
	mu                sync.RWMutex
	scenario          map[string]string
	cells             map[string]string
	currentCell       string
	repetitions       map[string]int
	currentRepetition int
}

func (registry *traceRegistry) add(traceID, scenario string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.scenario[traceID] = scenario
	registry.cells[traceID] = registry.currentCell
	registry.repetitions[traceID] = registry.currentRepetition
}

func (registry *traceRegistry) setRepetition(repetition int) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.currentRepetition = repetition
}

func (registry *traceRegistry) setCell(cell string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.currentCell = cell
}

func (registry *traceRegistry) lookup(traceID string) (string, string, int, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	scenario, exists := registry.scenario[traceID]
	return scenario, registry.cells[traceID], registry.repetitions[traceID], exists
}

type artifacts struct {
	dir             string
	serverLog       string
	serverLogOffset int64
	serverLogReady  bool
	toolLog         *os.File
	events          *os.File
	traces          *traceRegistry
	mu              sync.Mutex
}

type serverEvidence struct {
	Available       bool                        `json:"available"`
	LogPath         string                      `json:"log_path"`
	StartOffset     int64                       `json:"start_offset"`
	BytesScanned    int64                       `json:"bytes_scanned"`
	MatchedEvents   int                         `json:"matched_events"`
	MTP             mtpServerEvidence           `json:"mtp"`
	Note            string                      `json:"note,omitempty"`
	ImageExecutions []imageExecutionEvidence    `json:"image_executions,omitempty"`
	BatchLifecycle  map[string][]map[string]any `json:"-"`
}

type imageExecutionEvidence struct {
	TraceID   string `json:"trace_id"`
	Tokens    int    `json:"tokens"`
	Execution string `json:"execution"`
}

type mtpServerEvidence struct {
	BackendSelections      []mtpBackendSelection    `json:"backend_selections,omitempty"`
	DraftKVCleared         int                      `json:"draft_kv_cleared"`
	DraftKVClearedTraceIDs []string                 `json:"draft_kv_cleared_trace_ids,omitempty"`
	TargetRollbacks        []targetRollbackEvidence `json:"target_rollbacks,omitempty"`
}

type targetRollbackEvidence struct {
	TraceID  string `json:"trace_id"`
	Count    int    `json:"count,omitempty"`
	Backend  string `json:"backend"`
	Strategy string `json:"strategy"`
	Accepted int    `json:"accepted"`
	Drafted  int    `json:"drafted"`
}

type mtpBackendSelection struct {
	Repetition int    `json:"repetition"`
	Scenario   string `json:"scenario"`
	Cell       string `json:"cell"`
	TraceID    string `json:"trace_id"`
	Message    string `json:"message"`
	Backend    string `json:"backend"`
	Source     string `json:"source"`
}

type evidenceEvent struct {
	Repetition int            `json:"repetition"`
	Scenario   string         `json:"scenario"`
	Cell       string         `json:"cell"`
	Source     string         `json:"source"`
	TraceID    string         `json:"trace_id"`
	Event      map[string]any `json:"event"`
}

func newArtifacts(dir, serverLog string) (*artifacts, error) {
	offset, ready, note := serverLogStart(serverLog)
	if err := os.RemoveAll(dir); err != nil {
		return nil, fmt.Errorf("replace artifact directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact directory: %w", err)
	}

	toolLog, err := os.Create(filepath.Join(dir, toolLogFile))
	if err != nil {
		return nil, fmt.Errorf("create tool log: %w", err)
	}
	events, err := os.Create(filepath.Join(dir, eventsFile))
	if err != nil {
		toolLog.Close()
		return nil, fmt.Errorf("create event log: %w", err)
	}

	art := &artifacts{
		dir:             dir,
		serverLog:       serverLog,
		serverLogOffset: offset,
		serverLogReady:  ready,
		toolLog:         toolLog,
		events:          events,
		traces:          &traceRegistry{scenario: make(map[string]string), cells: make(map[string]string), repetitions: make(map[string]int)},
	}
	if note != "" {
		art.logf("server evidence note: %s", note)
	}
	return art, nil
}

func (art *artifacts) close() {
	art.toolLog.Close()
	art.events.Close()
}

func (art *artifacts) logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	art.mu.Lock()
	defer art.mu.Unlock()
	fmt.Println(line)
	fmt.Fprintln(art.toolLog, line)
}

func (art *artifacts) writeSummary(summary runSummary) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(art.dir, summaryFile), data, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

// Prepend the final outcome without keeping the execution log in memory.
func (art *artifacts) writeLogSummary(summary runSummary) error {
	if err := art.toolLog.Close(); err != nil {
		return fmt.Errorf("close tool log: %w", err)
	}
	source, err := os.Open(filepath.Join(art.dir, toolLogFile))
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.CreateTemp(art.dir, ".tool-log-*")
	if err != nil {
		return err
	}
	defer os.Remove(target.Name())
	defer target.Close()
	passed, failed := 0, 0
	for _, result := range summary.Scenarios {
		if result.Status == "PASS" {
			passed++
		} else if result.Status == "FAIL" {
			failed++
		}
	}
	var header strings.Builder
	fmt.Fprintf(&header, "TL;DR: %s — %d passed, %d failed scenario/cell results.\n", summary.Status, passed, failed)
	if summary.CoverageStatus != "" {
		fmt.Fprintf(&header, "Requested path coverage: %s (separate from behavior verdict).\n", summary.CoverageStatus)
	}
	for _, result := range summary.Scenarios {
		fmt.Fprintf(&header, "  %-4s repetition=%d %s / %s\n", result.Status, result.Repetition, result.Cell, result.Name)
		if result.Phase != "" {
			fmt.Fprintf(&header, "       phase=%s\n", result.Phase)
		}
		for _, coverage := range result.Coverage {
			fmt.Fprintf(&header, "       coverage: %s — %s (%s)\n", coverage.Status, coverage.Path, coverage.Note)
		}
		if workload, ok := result.Details["workload"].(string); ok {
			fmt.Fprintf(&header, "       workload=%s\n", workload)
		}
		if turns, ok := result.Details["turns"].([]batchTurnSummary); ok {
			for _, turn := range turns {
				if x := turn.ServerExecution; x != nil {
					wait := "unknown"
					if x.MaximumQueueWaitSeconds != nil {
						wait = fmt.Sprintf("%.6fs", *x.MaximumQueueWaitSeconds)
					}
					fmt.Fprintf(&header, "       turn=%d phase=%s client_overlap=%d inferred_queued=%d server_slots=%v queue_entries=%d max_queue_wait=%s evidence=%d complete/%d incomplete/%d inconsistent\n",
						turn.Turn, turn.Phase, turn.MaximumConcurrency, turn.QueuedRequests, x.AssignedSlots,
						x.ObservedQueueEntries, wait, x.CompleteRequests, x.IncompleteRequests, x.InconsistentRequests)
				}
			}
		}
		if profiles, ok := result.Details["profiles"].([]mtpProfileResult); ok {
			for _, profile := range profiles {
				status := "PASS"
				if len(profile.Failures) > 0 {
					status = "FAIL"
				}
				fmt.Fprintf(&header, "       %-4s %s\n", status, profile.Name)
			}
		}
		for _, key := range []string{"correctness", "concurrent_prefill"} {
			status := "PASS"
			present := false
			switch detail := result.Details[key].(type) {
			case mediaCorrectnessResult:
				present = true
				if len(detail.Failures) > 0 {
					status = "FAIL"
				}
			case mediaPrefillResult:
				present = true
				if len(detail.Failures) > 0 {
					status = "FAIL"
				}
			}
			if present {
				fmt.Fprintf(&header, "       %-4s %s\n", status, key)
			}
		}
		if len(result.Failures) > 0 {
			fmt.Fprintf(&header, "       %s\n", result.Failures[0])
		}
	}
	for _, failure := range summary.Failures {
		fmt.Fprintf(&header, "  FAIL setup: %s\n", failure)
	}
	if summary.Note != "" {
		fmt.Fprintf(&header, "%s\n", summary.Note)
	}
	if !summary.ServerEvidence.Available || summary.ServerEvidence.Note != "" {
		fmt.Fprintf(&header, "Evidence note: %s\n", summary.ServerEvidence.Note)
	}
	if len(summary.PerformanceAggregates) > 0 {
		fmt.Fprintf(&header, "Performance: %d matched groups in summary.json; mean and sample standard deviation across repetitions (n-1).\n", len(summary.PerformanceAggregates))
	}
	header.WriteString("\n========== EXECUTION TIMELINE (final results above include server-evidence checks) ==========\n")
	if _, err := io.WriteString(target, header.String()); err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	return os.Rename(target.Name(), filepath.Join(art.dir, toolLogFile))
}

func (art *artifacts) collectServerEvidence() serverEvidence {
	evidence := serverEvidence{
		Available:   art.serverLogReady,
		LogPath:     art.serverLog,
		StartOffset: art.serverLogOffset,
	}
	if !art.serverLogReady {
		evidence.Note = "server log was unavailable when the suite started; no internal behavior is claimed"
		return evidence
	}

	file, err := os.Open(art.serverLog)
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
	if info.Size() < art.serverLogOffset {
		evidence.Available = false
		evidence.Note = "server log was truncated or replaced during the suite"
		return evidence
	}
	if _, err := file.Seek(art.serverLogOffset, io.SeekStart); err != nil {
		evidence.Available = false
		evidence.Note = "seek server log: " + err.Error()
		return evidence
	}
	evidence.BytesScanned = info.Size() - art.serverLogOffset

	// IMC preparation encodes whole media chunks indivisibly. Only the start
	// event identifies the kind (2 = image); match its successful completion
	// before counting it as image execution evidence.
	type mediaChunkKey struct {
		traceID    string
		seq, chunk float64
	}
	pendingImages := make(map[mediaChunkKey]int)
	encoder := json.NewEncoder(art.events)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		traceID, _ := event["trace_id"].(string)
		scenario, cell, repetition, exists := art.traces.lookup(traceID)
		if !exists || !highValueEvent(event) {
			continue
		}
		recordMTPServerEvidence(&evidence.MTP, scenario, cell, traceID, repetition, event)
		if strings.HasPrefix(scenario, "batch") && event["msg"] == "request-lifecycle" &&
			(event["stage_name"] == "schedule-job" || event["stage_name"] == "execute-in-slot") {
			if evidence.BatchLifecycle == nil {
				evidence.BatchLifecycle = make(map[string][]map[string]any)
			}
			evidence.BatchLifecycle[traceID] = append(evidence.BatchLifecycle[traceID], event)
		}
		// encoding-image is emitted only by processIsolatedMediaChunk on the isolated
		// path; shared image encoding emits encoding-shared instead.
		if event["msg"] == "prefill-media" && event["status"] == "encoding-image" {
			tokens, _ := event["tokens"].(float64)
			evidence.ImageExecutions = append(evidence.ImageExecutions, imageExecutionEvidence{TraceID: traceID, Tokens: int(tokens), Execution: "isolated"})
		}
		if event["msg"] == "prefill-media" && event["status"] == "encoding-shared" {
			tokens, _ := event["tokens"].(float64)
			evidence.ImageExecutions = append(evidence.ImageExecutions, imageExecutionEvidence{TraceID: traceID, Tokens: int(tokens), Execution: "shared"})
		}
		if event["msg"] == "imc-media-cache" {
			seq, hasSeq := event["seq"].(float64)
			chunk, hasChunk := event["chunk"].(float64)
			tokens, _ := event["tokens"].(float64)
			if hasSeq && hasChunk && tokens > 0 && tokens == math.Trunc(tokens) && tokens < float64(math.MaxInt) {
				key := mediaChunkKey{traceID: traceID, seq: seq, chunk: chunk}
				switch event["status"] {
				case "encoding-media-chunk":
					delete(pendingImages, key)
					if event["kind"] == float64(2) {
						pendingImages[key] = int(tokens)
					}
				case "encoded-media-chunk":
					if pendingImages[key] == int(tokens) {
						evidence.ImageExecutions = append(evidence.ImageExecutions, imageExecutionEvidence{TraceID: traceID, Tokens: int(tokens), Execution: "imc"})
					}
					delete(pendingImages, key)
				}
			}
		}
		if err := encoder.Encode(evidenceEvent{Repetition: repetition, Scenario: scenario, Cell: cell, Source: "kronk-server", TraceID: traceID, Event: event}); err != nil {
			evidence.Note = "write server evidence: " + err.Error()
			return evidence
		}
		evidence.MatchedEvents++
	}
	if err := scanner.Err(); err != nil {
		evidence.Note = "scan server log: " + err.Error()
	}
	if evidence.MatchedEvents == 0 && evidence.Note == "" {
		evidence.Note = "no high-value server events matched generated request trace IDs"
	}
	return evidence
}

func recordMTPServerEvidence(evidence *mtpServerEvidence, scenario, cell, traceID string, repetition int, event map[string]any) {
	message, _ := event["msg"].(string)
	status, _ := event["status"].(string)
	if strings.HasPrefix(message, "draft-model-mtp") && status == "loaded" {
		backend, _ := event["backend"].(string)
		source, _ := event["source"].(string)
		evidence.BackendSelections = append(evidence.BackendSelections, mtpBackendSelection{
			Repetition: repetition,
			Scenario:   scenario,
			Cell:       cell,
			TraceID:    traceID,
			Message:    message,
			Backend:    backend,
			Source:     source,
		})
	}
	if message == "speculative" && status == "draft-kv-cleared" {
		evidence.DraftKVCleared++
		evidence.DraftKVClearedTraceIDs = append(evidence.DraftKVClearedTraceIDs, traceID)
	}
	// Current servers report completed recoveries once at request completion.
	if message == "batch-engine" && status == "slot-finished" {
		counts, _ := event["draft_target_rollbacks"].(map[string]any)
		for strategy, value := range counts {
			count, ok := value.(float64)
			if ok && count > 0 && count == math.Trunc(count) && count < float64(math.MaxInt) {
				evidence.TargetRollbacks = append(evidence.TargetRollbacks, targetRollbackEvidence{TraceID: traceID, Strategy: strategy, Count: int(count)})
			}
		}
	}
	if message == "speculative" && status == "target-rollback" {
		accepted, hasAccepted := event["accepted"].(float64)
		drafted, hasDrafted := event["drafted"].(float64)
		if hasAccepted && hasDrafted && accepted >= 0 && drafted > accepted {
			backend, _ := event["backend"].(string)
			strategy, _ := event["strategy"].(string)
			evidence.TargetRollbacks = append(evidence.TargetRollbacks, targetRollbackEvidence{
				TraceID: traceID, Backend: backend, Strategy: strategy, Accepted: int(accepted), Drafted: int(drafted),
			})
		}
	}
}

func serverLogStart(path string) (int64, bool, string) {
	info, err := os.Stat(path)
	if err == nil {
		return info.Size(), true, ""
	}
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, "server log does not exist: " + path
	}
	return 0, false, "cannot inspect server log: " + err.Error()
}

func highValueEvent(event map[string]any) bool {
	message, _ := event["msg"].(string)
	status, _ := event["status"].(string)
	level, _ := event["level"].(string)
	if level == "ERROR" || level == "WARN" {
		return true
	}
	switch message {
	case "request-lifecycle", "imc", "imc-media-cache", "prefill-media", "finish-slot", "draft-model-mtp", "draft-model-mtp-separate", "draft-model-mtp-shared", "batch-sizing":
		return true
	case "batch-engine":
		return status == "slot-started" || status == "slot-finished" || strings.Contains(status, "error") || status == "job-failed"
	case "start-slot":
		return strings.HasPrefix(status, "imc-") && status != "imc-preparation-chunk"
	case "speculative":
		return status == "target-rollback" || status == "draft-kv-cleared" || strings.Contains(status, "failed") || strings.Contains(status, "error") || strings.HasPrefix(status, "mtp-disabled") || status == "mtp-resume"
	case "cache":
		return strings.Contains(status, "failed") || strings.Contains(status, "error") || strings.HasPrefix(status, "mtp-disabled") || status == "mtp-resume"
	default:
		return false
	}
}

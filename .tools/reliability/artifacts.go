package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	mu       sync.RWMutex
	scenario map[string]string
}

func (registry *traceRegistry) add(traceID, scenario string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.scenario[traceID] = scenario
}

func (registry *traceRegistry) lookup(traceID string) (string, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	scenario, exists := registry.scenario[traceID]
	return scenario, exists
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
	Available     bool              `json:"available"`
	LogPath       string            `json:"log_path"`
	StartOffset   int64             `json:"start_offset"`
	BytesScanned  int64             `json:"bytes_scanned"`
	MatchedEvents int               `json:"matched_events"`
	MTP           mtpServerEvidence `json:"mtp"`
	Note          string            `json:"note,omitempty"`
}

type mtpServerEvidence struct {
	BackendSelections      []mtpBackendSelection `json:"backend_selections,omitempty"`
	DraftKVCleared         int                   `json:"draft_kv_cleared"`
	DraftKVClearedTraceIDs []string              `json:"draft_kv_cleared_trace_ids,omitempty"`
}

type mtpBackendSelection struct {
	Scenario string `json:"scenario"`
	TraceID  string `json:"trace_id"`
	Message  string `json:"message"`
	Backend  string `json:"backend"`
	Source   string `json:"source"`
}

type evidenceEvent struct {
	Scenario string         `json:"scenario"`
	Source   string         `json:"source"`
	TraceID  string         `json:"trace_id"`
	Event    map[string]any `json:"event"`
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
		traces:          &traceRegistry{scenario: make(map[string]string)},
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

	encoder := json.NewEncoder(art.events)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		traceID, _ := event["trace_id"].(string)
		scenario, exists := art.traces.lookup(traceID)
		if !exists || !highValueEvent(event) {
			continue
		}
		recordMTPServerEvidence(&evidence.MTP, scenario, traceID, event)
		if err := encoder.Encode(evidenceEvent{Scenario: scenario, Source: "kronk-server", TraceID: traceID, Event: event}); err != nil {
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

func recordMTPServerEvidence(evidence *mtpServerEvidence, scenario, traceID string, event map[string]any) {
	message, _ := event["msg"].(string)
	status, _ := event["status"].(string)
	if strings.HasPrefix(message, "draft-model-mtp") && status == "loaded" {
		backend, _ := event["backend"].(string)
		source, _ := event["source"].(string)
		evidence.BackendSelections = append(evidence.BackendSelections, mtpBackendSelection{
			Scenario: scenario,
			TraceID:  traceID,
			Message:  message,
			Backend:  backend,
			Source:   source,
		})
	}
	if message == "speculative" && status == "draft-kv-cleared" {
		evidence.DraftKVCleared++
		evidence.DraftKVClearedTraceIDs = append(evidence.DraftKVClearedTraceIDs, traceID)
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
	case "request-lifecycle", "imc", "imc-media-cache", "prefill-media", "finish-slot", "draft-model-mtp", "draft-model-mtp-separate", "draft-model-mtp-shared":
		return true
	case "batch-engine":
		return status == "slot-started" || status == "slot-finished" || strings.Contains(status, "error") || status == "job-failed"
	case "start-slot":
		return strings.HasPrefix(status, "imc-") && status != "imc-preparation-chunk"
	case "speculative":
		return status == "draft-kv-cleared" || strings.Contains(status, "failed") || strings.Contains(status, "error") || strings.HasPrefix(status, "mtp-disabled") || status == "mtp-resume"
	case "cache":
		return strings.Contains(status, "failed") || strings.Contains(status, "error") || strings.HasPrefix(status, "mtp-disabled") || status == "mtp-resume"
	default:
		return false
	}
}

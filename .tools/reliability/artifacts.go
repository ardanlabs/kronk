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
	mu          sync.RWMutex
	scenario    map[string]string
	cells       map[string]string
	currentCell string
}

func (registry *traceRegistry) add(traceID, scenario string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.scenario[traceID] = scenario
	registry.cells[traceID] = registry.currentCell
}

func (registry *traceRegistry) setCell(cell string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.currentCell = cell
}

func (registry *traceRegistry) lookup(traceID string) (string, string, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	scenario, exists := registry.scenario[traceID]
	return scenario, registry.cells[traceID], exists
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
	Cell     string `json:"cell"`
	TraceID  string `json:"trace_id"`
	Message  string `json:"message"`
	Backend  string `json:"backend"`
	Source   string `json:"source"`
}

type evidenceEvent struct {
	Scenario string         `json:"scenario"`
	Cell     string         `json:"cell"`
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
		traces:          &traceRegistry{scenario: make(map[string]string), cells: make(map[string]string)},
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
		} else {
			failed++
		}
	}
	var header strings.Builder
	fmt.Fprintf(&header, "TL;DR: %s — %d passed, %d failed scenario/cell results.\n", summary.Status, passed, failed)
	for _, result := range summary.Scenarios {
		fmt.Fprintf(&header, "  %-4s %s / %s\n", result.Status, result.Cell, result.Name)
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

	encoder := json.NewEncoder(art.events)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		traceID, _ := event["trace_id"].(string)
		scenario, cell, exists := art.traces.lookup(traceID)
		if !exists || !highValueEvent(event) {
			continue
		}
		recordMTPServerEvidence(&evidence.MTP, scenario, cell, traceID, event)
		if err := encoder.Encode(evidenceEvent{Scenario: scenario, Cell: cell, Source: "kronk-server", TraceID: traceID, Event: event}); err != nil {
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

func recordMTPServerEvidence(evidence *mtpServerEvidence, scenario, cell, traceID string, event map[string]any) {
	message, _ := event["msg"].(string)
	status, _ := event["status"].(string)
	if strings.HasPrefix(message, "draft-model-mtp") && status == "loaded" {
		backend, _ := event["backend"].(string)
		source, _ := event["source"].(string)
		evidence.BackendSelections = append(evidence.BackendSelections, mtpBackendSelection{
			Scenario: scenario,
			Cell:     cell,
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

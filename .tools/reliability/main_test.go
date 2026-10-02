package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseScenariosOrdersAndDeduplicates(t *testing.T) {
	got, err := parseScenarios("media,mtp,media,long-context")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mtp", "long-context", "media"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseScenarios() = %v, want %v", got, want)
	}
}

func TestCollectServerEvidenceFiltersByTraceAndValue(t *testing.T) {
	dir := t.TempDir()
	serverLog := filepath.Join(dir, "kronk.log")
	initial := []byte("{\"msg\":\"old\",\"trace_id\":\"known\"}\n")
	if err := os.WriteFile(serverLog, initial, 0o644); err != nil {
		t.Fatal(err)
	}

	art, err := newArtifacts(filepath.Join(dir, "out"), serverLog)
	if err != nil {
		t.Fatal(err)
	}
	defer art.close()
	art.traces.add("known", "mtp")

	file, err := os.OpenFile(serverLog, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"msg":"batch-engine","status":"prefill-scheduled","trace_id":"known"}`,
		`{"msg":"batch-engine","status":"slot-finished","trace_id":"foreign"}`,
		`{"msg":"batch-engine","status":"slot-finished","trace_id":"known","slot":2}`,
		`{"msg":"request-lifecycle","status":"complete","trace_id":"known","stage":4}`,
		`{"msg":"draft-model-mtp","status":"loaded","trace_id":"known","backend":"qwen35-own-kv","source":"auto-detected"}`,
		`{"msg":"speculative","status":"draft-kv-cleared","trace_id":"known","slot":2}`,
	}
	for _, line := range lines {
		if _, err := file.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	evidence := art.collectServerEvidence()
	if evidence.MatchedEvents != 4 {
		t.Fatalf("matched events = %d, want 4", evidence.MatchedEvents)
	}
	if evidence.BytesScanned <= 0 {
		t.Fatalf("bytes scanned = %d, want positive", evidence.BytesScanned)
	}
	if len(evidence.MTP.BackendSelections) != 1 {
		t.Fatalf("backend selections = %d, want 1", len(evidence.MTP.BackendSelections))
	}
	selection := evidence.MTP.BackendSelections[0]
	if selection.Backend != "qwen35-own-kv" || selection.Source != "auto-detected" {
		t.Fatalf("backend selection = %#v", selection)
	}
	if evidence.MTP.DraftKVCleared != 1 || !reflect.DeepEqual(evidence.MTP.DraftKVClearedTraceIDs, []string{"known"}) {
		t.Fatalf("draft KV evidence = %#v", evidence.MTP)
	}
	if err := art.events.Sync(); err != nil {
		t.Fatal(err)
	}
	events, err := os.Open(filepath.Join(dir, "out", eventsFile))
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	scanner := bufio.NewScanner(events)
	for scanner.Scan() {
		var event evidenceEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if event.Scenario != "mtp" || event.TraceID != "known" {
			t.Fatalf("event correlation = %#v", event)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestHighValueEventOmitsRepetitiveSchedulerLogs(t *testing.T) {
	if highValueEvent(map[string]any{"msg": "batch-engine", "status": "prefill-scheduled"}) {
		t.Fatal("prefill-scheduled event should be omitted")
	}
	if !highValueEvent(map[string]any{"msg": "start-slot", "status": "imc-reuse"}) {
		t.Fatal("imc-reuse event should be retained")
	}
	if !highValueEvent(map[string]any{"msg": "draft-model-mtp-separate", "status": "loaded"}) {
		t.Fatal("MTP backend selection should be retained")
	}
	if !highValueEvent(map[string]any{"msg": "speculative", "status": "draft-kv-cleared"}) {
		t.Fatal("draft KV cleanup should be retained")
	}
}

func TestValidateMTPServerEvidenceRejectsMissingCleanup(t *testing.T) {
	summary := runSummary{
		Status: "PASS",
		Scenarios: []scenarioResult{{
			Name:   "mtp",
			Status: "PASS",
			Details: map[string]any{"profiles": []mtpProfileResult{{
				Name: "embedded",
				Results: []mtpRequestResult{
					{Request: 1, TraceID: "cleared"},
					{Request: 2, TraceID: "missing"},
				},
			}}},
		}},
		ServerEvidence: serverEvidence{
			Available: true,
			MTP:       mtpServerEvidence{DraftKVClearedTraceIDs: []string{"cleared"}},
		},
	}

	validateMTPServerEvidence(&summary)
	if summary.Status != "FAIL" || summary.Scenarios[0].Status != "FAIL" {
		t.Fatalf("statuses = %s/%s, want FAIL/FAIL", summary.Status, summary.Scenarios[0].Status)
	}
	want := "embedded: request 2 has no draft-kv-cleared server event"
	if !reflect.DeepEqual(summary.Scenarios[0].Failures, []string{want}) {
		t.Fatalf("failures = %v, want %q", summary.Scenarios[0].Failures, want)
	}
}

func TestValidateMTPServerEvidenceRejectsWrongCompanionBackend(t *testing.T) {
	summary := runSummary{
		Status: "PASS",
		Scenarios: []scenarioResult{{
			Name:    "mtp",
			Status:  "PASS",
			Details: map[string]any{"profiles": []mtpProfileResult{}},
		}},
		ServerEvidence: serverEvidence{
			Available: true,
			MTP: mtpServerEvidence{BackendSelections: []mtpBackendSelection{{
				Scenario: "mtp",
				Message:  "draft-model-mtp-separate",
				Backend:  "unexpected",
				Source:   "unexpected",
			}}},
		},
	}

	validateMTPServerEvidence(&summary)
	if summary.Status != "FAIL" || len(summary.Scenarios[0].Failures) != 2 {
		t.Fatalf("summary = %#v, want two backend failures", summary)
	}
}

func TestVerifyLongContextOutputRejectsForeignMarker(t *testing.T) {
	expected := longContextMarkers(4096, 42)
	foreign := longContextMarkers(8192, 42)[0]
	known := map[string]bool{foreign: true}
	for _, marker := range expected {
		known[marker] = true
	}
	output := strings.Join(expected, " ") + " " + foreign
	if err := verifyLongContextOutput(output, expected, known); err == nil {
		t.Fatal("foreign long-context marker was accepted")
	}
}

func TestMaximumBatchConcurrencyUsesGenerationIntervals(t *testing.T) {
	origin := time.Unix(100, 0)
	requests := []batchRequestResult{
		{firstContentAt: origin, finishedAt: origin.Add(4 * time.Second)},
		{firstContentAt: origin.Add(time.Second), finishedAt: origin.Add(3 * time.Second)},
		{firstContentAt: origin.Add(2 * time.Second), finishedAt: origin.Add(5 * time.Second)},
		{firstContentAt: origin.Add(4 * time.Second), finishedAt: origin.Add(6 * time.Second)},
	}
	if got := maximumBatchConcurrency(requests); got != 3 {
		t.Fatalf("maximum concurrency = %d, want 3", got)
	}
}

func TestMaximumEventGapFiltersWindow(t *testing.T) {
	origin := time.Unix(100, 0)
	events := []time.Time{
		origin,
		origin.Add(100 * time.Millisecond),
		origin.Add(600 * time.Millisecond),
		origin.Add(700 * time.Millisecond),
	}
	got := maximumEventGap(events, func(start, end time.Time) bool {
		return !start.Before(origin.Add(100*time.Millisecond)) && !end.After(origin.Add(600*time.Millisecond))
	})
	if got != 0.5 {
		t.Fatalf("maximum event gap = %f, want 0.5", got)
	}
}

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
	art.traces.add("known", "batch")

	file, err := os.OpenFile(serverLog, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"msg":"batch-engine","status":"prefill-scheduled","trace_id":"known"}`,
		`{"msg":"batch-engine","status":"slot-finished","trace_id":"foreign"}`,
		`{"msg":"batch-engine","status":"slot-finished","trace_id":"known","slot":2}`,
		`{"msg":"request-lifecycle","status":"complete","trace_id":"known","stage":4}`,
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
	if evidence.MatchedEvents != 2 {
		t.Fatalf("matched events = %d, want 2", evidence.MatchedEvents)
	}
	if evidence.BytesScanned <= 0 {
		t.Fatalf("bytes scanned = %d, want positive", evidence.BytesScanned)
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
		if event.Scenario != "batch" || event.TraceID != "known" {
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

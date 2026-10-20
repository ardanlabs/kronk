package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

func TestMatrixCommandArtifactsAndCleanup(t *testing.T) {
	for _, defect := range []string{"", "response", "unload"} {
		t.Run("defect="+defect, func(t *testing.T) {
			dir := t.TempDir()
			logPath := filepath.Join(dir, "server.log")
			log, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			const base = "fixture/context"
			const stale = base + "/TOOLS-MTP-OFF-IMC-OFF"
			const protected = base + "/AGENT"
			var mu sync.Mutex
			loaded := map[string]bool{stale: true, protected: true, "unrelated/model": true}
			measured := false
			markers := regexp.MustCompile(`LONGCTX-S\d+-(?:BEGIN|MIDDLE|END)-[0-9A-F]{8}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.URL.Path {
				case "/v1/liveness":
					fmt.Fprint(w, `{"build":"fixture-server","host":"gpu-host"}`)
				case "/v1/kronk/libs":
					fmt.Fprint(w, `{"current":"fixture-libs"}`)
				case "/v1/kronk/models/ps":
					models := []loadedModel{}
					for id := range loaded {
						models = append(models, loadedModel{ID: id, Slots: 1})
					}
					json.NewEncoder(w).Encode(models)
				case "/v1/kronk/models/slots":
					engines := []engineDiagnostic{}
					for id := range loaded {
						engines = append(engines, engineDiagnostic{ModelID: id, MTP: strings.Contains(id, "MTP-ON"), Slots: []slotDiagnostic{{ID: 0, Phase: "idle"}}})
					}
					json.NewEncoder(w).Encode(engines)
				case "/v1/kronk/models/unload":
					var body struct {
						ID string `json:"id"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					if body.ID == protected || body.ID == "unrelated/model" {
						t.Errorf("attempted to unload protected model %s", body.ID)
					}
					if defect == "unload" && measured {
						http.Error(w, "fixture unload failed", http.StatusInternalServerError)
						return
					}
					delete(loaded, body.ID)
					json.NewEncoder(w).Encode(map[string]string{"id": body.ID, "status": "unloaded"})
				case "/v1/tokenize":
					var body struct {
						Input string `json:"input"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					fmt.Fprintf(w, `{"tokens":%d}`, 16+2*len(strings.Fields(body.Input)))
				case "/v1/chat/completions":
					var body struct {
						Model    string    `json:"model"`
						Stream   bool      `json:"stream"`
						Messages []message `json:"messages"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					for id := range loaded {
						if strings.Contains(id, "/TOOLS-") && id != body.Model {
							http.Error(w, "another suite alias is still resident", http.StatusInternalServerError)
							return
						}
					}
					loaded[body.Model] = true
					trace := strings.Split(r.Header.Get("Traceparent"), "-")
					if len(trace) == 4 {
						json.NewEncoder(log).Encode(map[string]string{"msg": "request-lifecycle", "trace_id": trace[1], "model": body.Model})
					}
					prompt := body.Messages[len(body.Messages)-1].Content
					answer := "OK"
					u := usage{PromptTokens: 10, CompletionTokens: 1}
					if prompt != "Reply OK." {
						measured = true
						u = usage{PromptTokens: 512, CompletionTokens: 32}
						if strings.Contains(body.Model, "MTP-ON") {
							u.DraftTokens, u.DraftCoverage = 21, 0.7
						}
						if strings.HasSuffix(body.Model, "IMC-ON") {
							u.PromptTokensDetails.CachedTokens = 400
						}
						if strings.Contains(prompt, "LONGCTX-") {
							answer = strings.Join(markers.FindAllString(prompt, -1), " ")
							if defect == "response" && strings.HasSuffix(body.Model, "MTP-ON-IMC-OFF") {
								answer = "wrong markers"
							}
						} else if parts := strings.SplitN(prompt, "exactly ", 2); len(parts) == 2 {
							answer = strings.Fields(parts[1])[0]
						}
					}
					if body.Stream {
						w.Header().Set("Content-Type", "text/event-stream")
						data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": answer}, "finish_reason": "stop"}}, "usage": u})
						fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
					} else {
						json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}, "usage": u})
					}
				default:
					if strings.HasPrefix(r.URL.Path, "/v1/kronk/models/"+base+"/") {
						fmt.Fprintf(w, `{"model_config":{"incremental-cache":%t}}`, strings.HasSuffix(r.URL.Path, "IMC-ON"))
					} else {
						http.NotFound(w, r)
					}
				}
			}))
			defer server.Close()
			out := filepath.Join(dir, "out")
			previous := os.Args
			os.Args = []string{"reliability", "-matrix", "all", "-scenario", "hybrid-state,long-context", "-hybrid-model", base, "-long-context-model", base,
				"-long-context-stages", "512", "-host", server.URL, "-out", out, "-server-log", logPath, "-timeout", "1s", "-repetitions", "2", "-run-label", "operator-label"}
			defer func() { os.Args = previous }()
			err = run()
			if (err != nil) != (defect != "") {
				t.Fatalf("run error=%v, defect=%q", err, defect)
			}
			data, err := os.ReadFile(filepath.Join(out, summaryFile))
			if err != nil {
				t.Fatal(err)
			}
			var summary runSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			wantResults, wantFailed := 16, 0
			if defect == "response" {
				wantFailed = 2
			}
			if defect == "unload" {
				wantResults, wantFailed = 1, 1
			}
			if len(summary.Scenarios) != wantResults {
				t.Fatalf("results=%d, want %d", len(summary.Scenarios), wantResults)
			}
			failed := 0
			cells := []string{"mtp-on-imc-on", "mtp-on-imc-off", "mtp-off-imc-on", "mtp-off-imc-off"}
			for i, result := range summary.Scenarios {
				if result.Cell != cells[(i%8)/2] || result.Name != []string{"hybrid-state", "long-context"}[i%2] || result.Repetition != i/8+1 {
					t.Fatalf("misordered result %+v", result)
				}
				if result.Status == "FAIL" {
					failed++
				}
			}
			if failed != wantFailed {
				t.Fatalf("failed=%d, want %d: %+v", failed, wantFailed, summary.Scenarios)
			}
			wantAggregates := (wantResults - wantFailed) / 2
			if len(summary.PerformanceAggregates) != wantAggregates {
				t.Fatalf("aggregate groups=%d, want %d", len(summary.PerformanceAggregates), wantAggregates)
			}
			for _, aggregate := range summary.PerformanceAggregates {
				stats := aggregate.Metrics["duration_seconds"]
				if stats.Count != 2 || stats.MissingRepetitions != 0 || stats.SampleStandardDeviation == nil || !reflect.DeepEqual(stats.Repetitions, []int{1, 2}) {
					t.Fatalf("incorrect repetition statistics: %+v", stats)
				}
			}
			if summary.Configuration.Repetitions != 2 || summary.Configuration.RunLabel != "operator-label" || summary.ServerMetadata["/v1/liveness"].(map[string]any)["build"] != "fixture-server" || summary.ToolBuild["go_version"] == "" {
				t.Fatalf("missing run provenance: %+v", summary)
			}
			for _, unload := range summary.ModelUnloads[1:] {
				if unload.Repetition < 1 || unload.Repetition > 2 {
					t.Fatalf("untagged cleanup: %+v", unload)
				}
			}
			if (summary.Note != "") != (defect == "unload") {
				t.Fatalf("unexpected stop note %q", summary.Note)
			}
			mu.Lock()
			if !loaded[protected] || !loaded["unrelated/model"] || defect != "unload" && len(loaded) != 2 {
				t.Errorf("unexpected models remaining: %v", loaded)
			}
			mu.Unlock()
			if len(summary.ModelUnloads) == 0 || summary.ModelUnloads[0].Cell != "setup" {
				t.Fatalf("missing setup cleanup: %+v", summary.ModelUnloads)
			}
			data, err = os.ReadFile(filepath.Join(out, toolLogFile))
			if err != nil {
				t.Fatal(err)
			}
			prefix := fmt.Sprintf("TL;DR: %s — %d passed, %d failed scenario/cell results.", summary.Status, wantResults-wantFailed, wantFailed)
			if !strings.HasPrefix(string(data), prefix) {
				t.Fatalf("incorrect tool.log header: %s", data)
			}
			entries, err := os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 3 {
				t.Fatalf("expected just three artifacts, got %v", entries)
			}
			data, err = os.ReadFile(filepath.Join(out, eventsFile))
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
				var event evidenceEvent
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					t.Fatal(err)
				}
				seen[fmt.Sprintf("%d/%s/%s", event.Repetition, event.Cell, event.Scenario)] = true
				model, _ := event.Event["model"].(string)
				if !strings.HasSuffix(model, strings.ToUpper("TOOLS-"+event.Cell)) {
					t.Fatalf("event tagged with wrong cell: %+v", event)
				}
			}
			for _, result := range summary.Scenarios {
				if !seen[fmt.Sprintf("%d/%s/%s", result.Repetition, result.Cell, result.Name)] {
					t.Errorf("missing events for %s/%s", result.Cell, result.Name)
				}
			}
		})
	}
}

func TestPerformanceSummaryStatistics(t *testing.T) {
	summary := runSummary{Configuration: config{Repetitions: 4}}
	for i, value := range []float64{1, 2, 9} {
		var serverTPS *float64
		if i != 1 {
			v := float64(i * 6)
			serverTPS = &v
		}
		for _, workload := range []string{"text", "images"} {
			factor := 1.0
			if workload == "images" {
				factor = 100
			}
			result := scenarioResult{Name: "batch/" + workload, Cell: "mtp-off-imc-on", Status: "PASS", Repetition: i + 1, DurationSeconds: value * factor}
			result.Details = map[string]any{
				"workload": workload,
				"turns": []batchTurnSummary{
					{Turn: 1, Phase: "history", Complete: true, ElapsedSeconds: value * factor},
					{Turn: 2, Phase: "history", Complete: true, ElapsedSeconds: value * factor * 10},
					{Turn: 1, Phase: "repeat", Complete: true, ElapsedSeconds: value * factor * 20},
					{Turn: 3, Phase: "history", Complete: false, ElapsedSeconds: 1000},
				},
				"requests": []batchRequestResult{
					{Session: 1, Turn: 1, Phase: "history", FinishReason: "stop", ElapsedSeconds: value * factor, ServerTokensPerSecond: serverTPS},
					{Session: 2, Turn: 1, Phase: "history", FinishReason: "stop", ElapsedSeconds: value * factor * 10},
					{Session: 3, Turn: 1, Phase: "history", Error: "failed", ElapsedSeconds: 1000},
				},
			}
			summary.Scenarios = append(summary.Scenarios, result)
		}
	}
	summary.Scenarios = append(summary.Scenarios,
		scenarioResult{Name: "batch/text", Cell: "mtp-off-imc-on", Repetition: 4, Status: "FAIL", DurationSeconds: 999, Details: map[string]any{"workload": "text", "turns": []batchTurnSummary{{Turn: 1, Phase: "history", Complete: true, ElapsedSeconds: 50}}}},
		scenarioResult{Name: "batch/text", Cell: "mtp-off-imc-off", Repetition: 1, Status: "PASS", DurationSeconds: 100},
	)
	art, err := newArtifacts(filepath.Join(t.TempDir(), "out"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer art.close()
	aggregatePerformance(&summary)
	if err := art.writeSummary(summary); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(art.dir, summaryFile))
	if err != nil {
		t.Fatal(err)
	}
	var saved runSummary
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.PerformanceAggregates) != 14 {
		t.Fatalf("aggregate groups=%d, want 14", len(saved.PerformanceAggregates))
	}
	for _, aggregate := range saved.PerformanceAggregates {
		metric := "elapsed_seconds"
		if aggregate.Scope == "scenario" {
			metric = "duration_seconds"
		}
		stats := aggregate.Metrics[metric]
		if aggregate.Status == "FAIL" || aggregate.Cell == "mtp-off-imc-off" {
			want := 50.0
			if aggregate.Cell == "mtp-off-imc-off" {
				want = 100
			}
			if stats.Count != 1 || stats.Mean != want || stats.SampleStandardDeviation != nil || stats.MissingRepetitions != 3 {
				t.Fatalf("single sample or failed-run separation incorrect: %+v", aggregate)
			}
			continue
		}
		factor := 1.0
		if aggregate.Workload == "images" {
			factor = 100
		}
		if aggregate.Turn == 2 || aggregate.Session == 2 {
			factor *= 10
		}
		if aggregate.Phase == "repeat" {
			factor *= 20
		}
		if stats.Count != 3 || stats.Mean != 4*factor || stats.SampleStandardDeviation == nil || math.Abs(*stats.SampleStandardDeviation-math.Sqrt(19)*factor) > 1e-9 || stats.MissingRepetitions != 1 || !reflect.DeepEqual(stats.Repetitions, []int{1, 2, 3}) {
			t.Fatalf("wrong matching or sample statistics: %+v / %+v", aggregate, stats)
		}
		if aggregate.Scope == "batch-request" && aggregate.Session == 1 {
			server := aggregate.Metrics["server_tokens_per_second"]
			if server == nil || server.Count != 2 || server.Mean != 6 || server.MissingRepetitions != 2 || math.Abs(*server.SampleStandardDeviation-math.Sqrt(72)) > 1e-9 || !reflect.DeepEqual(server.Repetitions, []int{1, 3}) {
				t.Fatalf("absent metrics must not become zeros; supplied zero must count: %+v", server)
			}
		}
	}
	aggregatePerformance(&summary)
	repeated, err := json.MarshalIndent(summary, "", "  ")
	if err != nil || string(data) != string(repeated)+"\n" {
		t.Fatal("summary statistics must serialize deterministically")
	}
	single := runSummary{Configuration: config{Repetitions: 1}, Scenarios: summary.Scenarios[:1]}
	aggregatePerformance(&single)
	encoded, err := json.Marshal(single)
	if err != nil || strings.Contains(string(encoded), "performance_aggregates") {
		t.Fatal("single-run summaries must omit repetition aggregates")
	}
}

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

func TestMatrixSelection(t *testing.T) {
	cfg, _, err := parseConfig([]string{"-scenario", "media", "-matrix", "MTPoff/IMCon,MTPon/IMCoff,MTPoff/IMCon", "-media-model", "fixture/vision/TOOLS-MTP-ON-IMC-ON", "-hybrid-model", "unused/custom/alias"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Matrix, []modeCell{{"off", "on"}, {"on", "off"}}) || cfg.MediaModel != "fixture/vision/TOOLS-MTP-OFF-IMC-ON" {
		t.Fatalf("incorrect cell selection: %+v", cfg)
	}
	if cfg.BatchProfile != "all" {
		t.Fatalf("default batch profile=%q, want all", cfg.BatchProfile)
	}
	if cfg.Repetitions != 1 {
		t.Fatalf("default repetitions=%d, want 1", cfg.Repetitions)
	}
	cfg.Token = "do-not-record-this-token"
	encoded, err := json.Marshal(cfg)
	if err != nil || strings.Contains(string(encoded), cfg.Token) {
		t.Fatal("configuration must not persist authentication tokens")
	}
	for _, args := range [][]string{
		{"-repetitions", "0"},
		{"-repetitions", "-1"},
		{"-matrix", "all", "-mtp", "on"},
		{"-matrix", "MTPon/invalid"},
		{"-matrix", "MTPon/IMCon,"},
		{"-matrix", "IMCoff/MTPon"},
		{"-matrix", "MTPon/MTPoff,IMCoff/IMCoff"},
		{"-matrix", "on/off"},
		{"-scenario", "media", "-matrix", "all", "-media-model", "fixture/vision/AGENT"},
		{"-batch-profile", "invalid"},
		{"-batch-profile", "images", "-batch-images", "a.jpg"},
		{"-batch-profile", "images", "-batch-images", "a.jpg,a.jpg"},
		{"-batch-profile", "images", "-batch-image-subjects", "deer,deer"},
	} {
		if _, _, err := parseConfig(args); err == nil {
			t.Errorf("accepted invalid options %v", args)
		}
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
	summary.Status, summary.Scenarios[0].Status = "PASS", "PASS"
	summary.Scenarios[0].Failures = nil
	summary.ExpectedModes = map[string]string{"mtp": "off"}
	validateMTPServerEvidence(&summary)
	if summary.Status != "PASS" || len(summary.Scenarios[0].Failures) != 0 {
		t.Fatal("target-only control incorrectly requires draft cleanup")
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

	summary.Scenarios[0].Cell = "mtp-on-imc-on"
	summary.ServerEvidence.MTP.BackendSelections[0].Cell = "mtp-on-imc-on"
	summary.Scenarios = append(summary.Scenarios,
		scenarioResult{Name: "mtp", Cell: "mtp-on-imc-off", Status: "PASS", Details: map[string]any{"profiles": []mtpProfileResult{}}},
		scenarioResult{Name: "mtp", Cell: "mtp-off-imc-on", Status: "PASS", ExpectedModes: map[string]string{"mtp": "off"}},
	)
	summary.ServerEvidence.MTP.BackendSelections = append(summary.ServerEvidence.MTP.BackendSelections,
		mtpBackendSelection{Scenario: "mtp", Cell: "mtp-off-imc-on", Message: "draft-model-mtp", Backend: "unexpected", Source: "unexpected"},
	)
	validateMTPServerEvidence(&summary)
	if summary.Status != "FAIL" || len(summary.Scenarios[0].Failures) != 2 {
		t.Fatalf("summary = %#v, want two backend failures", summary)
	}
	for _, result := range summary.Scenarios[1:] {
		if result.Status != "PASS" || len(result.Failures) != 0 {
			t.Fatalf("other cell contaminated by backend evidence: %+v", result)
		}
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

func TestModeDefaultsMatchToolsConfiguration(t *testing.T) {
	doc, err := modelconfig.Load("../model_config_tools.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, mtp := range []string{"on", "off"} {
		for _, imc := range []string{"on", "off"} {
			cfg, _, err := parseConfig([]string{"-mtp", mtp, "-imc", imc})
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{cfg.MTPEmbeddedModel, cfg.MTPCompanionModel, cfg.HybridModel, cfg.LongContextModel, cfg.BatchModel, cfg.MediaModel} {
				entry, ok := doc.Models[id]
				if !ok || entry.PtrIncrementalCache == nil || *entry.PtrIncrementalCache != (imc == "on") || (entry.Speculation.String() == "mtp") != (mtp == "on") {
					t.Fatalf("%s does not select expected MTP=%s IMC=%s: %+v", id, mtp, imc, entry)
				}
			}
		}
	}
	cfg, _, err := parseConfig([]string{"-mtp", "off", "-media-model", "custom/model/PROFILE"})
	if err != nil || cfg.MediaModel != "custom/model/PROFILE" {
		t.Fatalf("explicit model override changed: %s, %v", cfg.MediaModel, err)
	}
	if _, _, err := parseConfig([]string{"-imc", "auto"}); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

// Exercise the command and its artifacts against a controlled HTTP/SSE server.
// These fixtures validate the tools, not model-backed Kronk behavior.
func TestMediaCommandModesAndStateFailures(t *testing.T) {
	cases := []struct{ name, mtp, imc, defect, failure string }{
		{"on-on", "on", "on", "", ""},
		{"on-off", "on", "off", "", ""},
		{"off-on", "off", "on", "", ""},
		{"off-off", "off", "off", "", ""},
		{"wrong-mtp-config", "on", "on", "wrong-mtp", "observed MTP=false"},
		{"wrong-imc-config", "off", "off", "wrong-imc", "observed MTP=false IMC=true"},
		{"no-reuse", "on", "on", "no-cache", "reported no cached tokens"},
		{"unexpected-reuse", "off", "off", "cache", "IMC off: reported"},
		{"image-fallback", "on", "off", "no-draft", "did not demonstrate MTP"},
		{"unexpected-drafting", "off", "on", "draft", "MTP off: reported drafting"},
		{"bad-append", "off", "off", "append", "appended image response lacks"},
		{"bad-recovery", "off", "off", "recovery", "image recovery lacks"},
		{"stuck-cancellation", "off", "off", "busy", "did not release all slots"},
		{"no-cancellation-content", "off", "off", "no-content", "expected disconnect after content"},
		{"missing-usage", "off", "off", "usage", "lacks positive prompt/completion usage"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const model = "fixture/vision/TOOLS"
			var cancelled atomic.Bool
			var loaded atomic.Bool
			loaded.Store(true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/kronk/models/ps":
					if loaded.Load() {
						fmt.Fprintf(w, `[{"id":%q,"slots":4,"has_projection":true}]`, model)
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/v1/kronk/models/unload":
					loaded.Store(false)
					fmt.Fprintf(w, `{"id":%q,"status":"unloaded"}`, model)
				case "/v1/kronk/models/" + model:
					fmt.Fprintf(w, `{"model_config":{"incremental-cache":%t}}`, tc.imc == "on" || tc.defect == "wrong-imc")
				case "/v1/kronk/models/slots":
					phase := "idle"
					if tc.defect == "busy" && cancelled.Load() {
						phase = "generation"
					}
					fmt.Fprintf(w, `[{"model_id":%q,"mtp":%t,"ndraft":3,"slots":[{"id":0,"phase":%q},{"id":1,"phase":"idle"},{"id":2,"phase":"idle"},{"id":3,"phase":"idle"}]}]`, model, tc.mtp == "on" && tc.defect != "wrong-mtp", phase)
				case "/v1/chat/completions":
					var body struct {
						Stream   bool `json:"stream"`
						Messages []struct {
							Content any `json:"content"`
						} `json:"messages"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					loaded.Store(true)
					if body.Messages[0].Content == "Reply OK." {
						fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`)
						return
					}
					if body.Stream {
						cancelled.Store(true)
						w.Header().Set("Content-Type", "text/event-stream")
						if tc.defect == "no-content" {
							fmt.Fprintln(w, "data: [DONE]")
							return
						}
						fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"MEDIA-CANCELLED"}}]}`)
						w.(http.Flusher).Flush()
						select {
						case <-r.Context().Done():
						case <-time.After(2 * time.Second):
						}
						return
					}
					last := body.Messages[len(body.Messages)-1].Content
					prompt, image := last.(string)
					if !image {
						prompt = last.([]any)[0].(map[string]any)["text"].(string)
					}
					parts := strings.SplitN(prompt, "exactly ", 2)
					answer := strings.Fields(parts[1])[0]
					if strings.Contains(prompt, "this marker and nothing else: ") {
						answer = strings.SplitN(prompt, "this marker and nothing else: ", 2)[1]
					}
					if !image || strings.HasPrefix(answer, "MEDIA-APPEND") {
						answer += " giraffe"
					}
					if tc.defect == "append" && strings.HasPrefix(answer, "MEDIA-APPEND") || tc.defect == "recovery" && strings.HasPrefix(answer, "MEDIA-RECOVERY") {
						answer = "wrong subject and stale prefix"
					}
					u := usage{PromptTokens: 300, CompletionTokens: 20}
					if (tc.imc == "on" && tc.defect != "no-cache") || tc.defect == "cache" {
						u.PromptTokensDetails.CachedTokens = 200
					}
					if (tc.mtp == "on" && tc.defect != "no-draft") || tc.defect == "draft" {
						u.DraftTokens, u.DraftCoverage = 12, 0.6
					}
					if tc.defect == "usage" {
						u = usage{}
					}
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}, "usage": u})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			image := filepath.Join(dir, "image.jpg")
			if err := os.WriteFile(image, []byte("fixture image"), 0o600); err != nil {
				t.Fatal(err)
			}
			previous := os.Args
			os.Args = []string{"reliability", "-scenario", "media", "-media-profile", "correctness", "-media-model", model, "-media-image", image,
				"-host", server.URL, "-out", filepath.Join(dir, "out"), "-server-log", filepath.Join(dir, "absent.log"), "-mtp", tc.mtp, "-imc", tc.imc, "-timeout", "200ms"}
			defer func() { os.Args = previous }()
			err := run()
			if (err != nil) != (tc.failure != "") {
				t.Fatalf("run error=%v, expected failure=%q", err, tc.failure)
			}
			data, err := os.ReadFile(filepath.Join(dir, "out", summaryFile))
			if err != nil {
				t.Fatal(err)
			}
			var summary runSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			if summary.ExpectedModes["mtp"] != tc.mtp || summary.ExpectedModes["imc"] != tc.imc {
				t.Fatalf("wrong recorded modes: %v", summary.ExpectedModes)
			}
			if tc.failure != "" && !strings.Contains(strings.Join(summary.Scenarios[0].Failures, "\n"), tc.failure) {
				t.Fatalf("missing expected failure %q: %v", tc.failure, summary.Scenarios[0].Failures)
			}
		})
	}
}

func TestMediaPrefillCommandTiming(t *testing.T) {
	for _, failImage := range []bool{false, true} {
		t.Run(fmt.Sprint(failImage), func(t *testing.T) {
			const model = "fixture/media/TOOLS"
			var loaded, generating, imaging atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/kronk/models/ps":
					if loaded.Load() {
						fmt.Fprintf(w, `[{"id":%q,"slots":2,"has_projection":true}]`, model)
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/v1/kronk/models/unload":
					loaded.Store(false)
					fmt.Fprintf(w, `{"id":%q,"status":"unloaded"}`, model)
				case "/v1/kronk/models/" + model:
					fmt.Fprint(w, `{"model_config":{"incremental-cache":false,"devices":["fixture-GPU"]}}`)
				case "/v1/kronk/models/slots":
					a, b := "idle", "idle"
					if generating.Load() {
						a = "generation"
					}
					if imaging.Load() {
						b = "media-prefill"
					}
					fmt.Fprintf(w, `[{"model_id":%q,"mtp":false,"prefill_batch_size":512,"nbatch":514,"nubatch":512,"slots":[{"id":0,"phase":%q},{"id":1,"phase":%q}]}]`, model, a, b)
				case "/v1/chat/completions":
					var body struct {
						Stream   bool
						Messages []struct{ Content any }
					}
					json.NewDecoder(r.Body).Decode(&body)
					if !body.Stream {
						loaded.Store(true)
						fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`)
						return
					}
					_, text := body.Messages[0].Content.(string)
					if !text && failImage {
						http.Error(w, "image failed", http.StatusInternalServerError)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					count := 30
					if text {
						generating.Store(true)
						defer generating.Store(false)
						time.Sleep(25 * time.Millisecond)
					} else {
						imaging.Store(true)
						defer imaging.Store(false)
						time.Sleep(90 * time.Millisecond)
						count = 3
					}
					for range count {
						fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"1 "}}]}`)
						fmt.Fprintln(w)
						w.(http.Flusher).Flush()
						time.Sleep(10 * time.Millisecond)
					}
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}],\"usage\":{\"prompt_tokens\":123,\"completion_tokens\":%d,\"tokens_per_second\":432.1,\"time_to_first_token_ms\":17.5}}\n\ndata: [DONE]\n\n", count)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			dir := t.TempDir()
			image := filepath.Join(dir, "image.jpg")
			if err := os.WriteFile(image, []byte("image"), 0o600); err != nil {
				t.Fatal(err)
			}
			previous := os.Args
			os.Args = []string{"reliability", "-scenario", "media", "-media-profile", "prefill", "-media-model", model, "-media-image", image, "-mtp", "off", "-imc", "off", "-host", server.URL, "-out", filepath.Join(dir, "out"), "-server-log", filepath.Join(dir, "missing.log"), "-repetitions", "2"}
			defer func() { os.Args = previous }()
			if err := run(); (err != nil) != failImage {
				t.Fatalf("error=%v, failImage=%t", err, failImage)
			}
			data, err := os.ReadFile(filepath.Join(dir, "out", summaryFile))
			if err != nil {
				t.Fatal(err)
			}
			var summary runSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			foundGeneration := false
			for _, aggregate := range summary.PerformanceAggregates {
				if aggregate.Scope == "media-image" && failImage {
					t.Fatal("failed image stream must not contribute performance samples")
				}
				if aggregate.Scope == "media-generation" {
					foundGeneration = true
					stats := aggregate.Metrics["server_tokens_per_second"]
					if stats.Count != 2 || stats.Mean != 432.1 || stats.SampleStandardDeviation == nil || *stats.SampleStandardDeviation != 0 {
						t.Fatalf("incorrect constant-rate statistics: %+v", stats)
					}
				}
			}
			if !foundGeneration {
				t.Fatal("missing completed text-stream aggregates")
			}
			detail := summary.Scenarios[0].Details["concurrent_prefill"]
			data, _ = json.Marshal(detail)
			var result mediaPrefillResult
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if result.WorkloadComplete == failImage {
				t.Fatalf("wrong coverage: %+v", result)
			}
			if failImage && (result.Image.Error == "" || result.Image.Performance.ElapsedSeconds <= 0 || result.Image.Performance.FinishedAt.IsZero()) {
				t.Fatalf("failed image must retain elapsed timing: %+v", result.Image)
			}
			if !failImage {
				if result.Generation.Usage.CompletionTokens != 30 || result.Image.Usage.CompletionTokens != 3 || result.Image.Performance.FirstContentSeconds < 0.085 || result.Generation.Performance.FirstContentSeconds < 0.020 || result.Generation.Performance.GenerationSeconds < 0.29 || result.Image.Performance.GenerationSeconds < 0.025 {
					t.Fatalf("incorrect content timings: %+v", result)
				}
				if result.Image.Performance.ElapsedSeconds <= result.Image.Performance.GenerationSeconds || *result.Image.Usage.TimeToFirstTokenMS != 17.5 || *result.Image.Usage.TokensPerSecond != 432.1 {
					t.Fatalf("missing server/client distinction: %+v", result.Image)
				}
				if result.Image.Performance.OutputTokensPerSecond != 3/result.Image.Performance.GenerationSeconds {
					t.Fatal("incorrect image rate")
				}
			}
			configurations := summary.Scenarios[0].Details["model_configurations"].(map[string]any)
			if configurations[model].(map[string]any)["scheduler"].(map[string]any)["nbatch"] != float64(514) {
				t.Fatal("missing effective batch configuration")
			}
		})
	}
}

func TestLongContextModesValidateStreamingUsage(t *testing.T) {
	for _, tc := range []struct{ mtp, imc, defect, failure string }{
		{"on", "on", "", ""}, {"on", "off", "", ""},
		{"off", "on", "", ""}, {"off", "off", "", ""},
		{"on", "on", "no-cache", "warm repeat reported cached_tokens=0"},
		{"off", "off", "cache", "IMC off: reported"},
		{"on", "off", "no-draft", "without positive drafting"},
		{"off", "on", "draft", "MTP off: reported drafting"},
		{"off", "off", "usage", "lacks positive prompt/completion usage"},
	} {
		t.Run(tc.mtp+"-"+tc.imc+"-"+tc.defect, func(t *testing.T) {
			const model = "fixture/long-context/TOOLS"
			markerPattern := regexp.MustCompile(`LONGCTX-S\d+-(?:BEGIN|MIDDLE|END)-[0-9A-F]{8}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/kronk/models/ps":
					fmt.Fprintf(w, `[{"id":%q,"slots":1}]`, model)
				case "/v1/kronk/models/" + model:
					fmt.Fprintf(w, `{"model_config":{"incremental-cache":%t,"context-window":8192}}`, tc.imc == "on")
				case "/v1/kronk/models/slots":
					fmt.Fprintf(w, `[{"model_id":%q,"mtp":%t,"slots":[{"id":0,"phase":"idle"}]}]`, model, tc.mtp == "on")
				case "/v1/tokenize":
					var body struct {
						Input string `json:"input"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					fmt.Fprintf(w, `{"tokens":%d}`, 16+2*len(strings.Fields(body.Input)))
				case "/v1/chat/completions":
					var body struct {
						Messages []message `json:"messages"`
					}
					json.NewDecoder(r.Body).Decode(&body)
					output := strings.Join(markerPattern.FindAllString(body.Messages[0].Content, -1), " ")
					u := usage{PromptTokens: 512, CompletionTokens: 32}
					if tc.imc == "on" && tc.defect != "no-cache" || tc.defect == "cache" {
						u.PromptTokensDetails.CachedTokens = 400
					}
					if tc.mtp == "on" && tc.defect != "no-draft" || tc.defect == "draft" {
						u.DraftTokens, u.DraftCoverage = 21, 0.7
					}
					if tc.defect == "usage" {
						u = usage{}
					}
					w.Header().Set("Content-Type", "text/event-stream")
					data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": output}, "finish_reason": "stop"}}, "usage": u})
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			cfg, _, err := parseConfig([]string{"-mtp", tc.mtp, "-imc", tc.imc, "-long-context-model", model, "-long-context-stages", "512"})
			if err != nil {
				t.Fatal(err)
			}
			art, err := newArtifacts(filepath.Join(t.TempDir(), "out"), "")
			if err != nil {
				t.Fatal(err)
			}
			defer art.close()
			rc := runContext{Context: context.Background(), cfg: cfg, client: newAPIClient(server.URL, "", time.Second, art.traces), artifacts: art, scenario: "long-context", observedModes: make(map[string]map[string]bool)}
			result := runLongContext(&rc)
			if tc.failure == "" && len(result.Failures) != 0 || tc.failure != "" && !strings.Contains(strings.Join(result.Failures, "\n"), tc.failure) {
				t.Fatalf("expected failure %q, got %v", tc.failure, result.Failures)
			}
		})
	}
}

func TestBatchImageCommandModesAndHistory(t *testing.T) {
	for _, tc := range []struct{ workload, mtp, imc, defect, failure string }{
		{"text", "off", "on", "", ""},
		{"text", "off", "off", "overlap", "reached concurrency 1"},
		{"images", "on", "on", "", ""},
		{"images", "on", "off", "", ""},
		{"images", "off", "on", "", ""},
		{"images", "off", "off", "", ""},
		{"images", "off", "on", "no-cache", "repeat reported no cached tokens"},
		{"images", "off", "off", "cache", "IMC off: reported"},
		{"images", "on", "off", "no-draft", "without positive drafting"},
		{"images", "off", "on", "draft", "MTP off: reported drafting"},
		{"images", "off", "off", "stale", "foreign/stale subject"},
		{"images", "off", "off", "foreign-marker", "foreign marker"},
		{"images", "on", "off", "repeat", ""},
		{"images", "off", "off", "projection", "requires a projection"},
		{"all", "off", "on", "", ""},
		{"all", "on", "off", "text-marker", "foreign marker"},
		{"", "off", "on", "", ""},
	} {
		t.Run(tc.workload+"-"+tc.mtp+"-"+tc.imc+"-"+tc.defect, func(t *testing.T) {
			const model = "fixture/batch/TOOLS"
			dir := t.TempDir()
			paths := []string{filepath.Join(dir, "giraffe.jpg"), filepath.Join(dir, "astronaut.jpg")}
			for index, subject := range []string{"giraffe", "astronaut"} {
				if err := os.WriteFile(paths[index], []byte(subject), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			serverLog, err := os.Create(filepath.Join(dir, "server.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer serverLog.Close()
			var loaded atomic.Bool
			var mu sync.Mutex
			seen := make(map[string]int)
			sem := make(chan struct{}, 2)
			if tc.defect == "overlap" {
				sem = make(chan struct{}, 1)
			}
			markerPattern := regexp.MustCompile(`BATCH-SESSION-(\d+)-TURN-(\d+)`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/kronk/models/ps":
					if loaded.Load() {
						fmt.Fprintf(w, `[{"id":%q,"slots":2,"has_projection":%t}]`, model, tc.defect != "projection")
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/v1/kronk/models/" + model:
					fmt.Fprintf(w, `{"model_config":{"incremental-cache":%t}}`, tc.imc == "on")
				case "/v1/kronk/models/slots":
					fmt.Fprintf(w, `[{"model_id":%q,"mtp":%t,"slots":[{"id":0,"phase":"idle"},{"id":1,"phase":"idle"}]}]`, model, tc.mtp == "on")
				case "/v1/kronk/models/unload":
					loaded.Store(false)
					fmt.Fprintf(w, `{"id":%q,"status":"unloaded"}`, model)
				case "/v1/tokenize":
					var body struct{ Input string }
					json.NewDecoder(r.Body).Decode(&body)
					if strings.Contains(body.Input, "data:image") {
						t.Error("image payload must not be counted as text during history calibration")
					}
					fmt.Fprintf(w, `{"tokens":%d}`, 2*len(strings.Fields(body.Input)))
				case "/v1/chat/completions":
					var body struct {
						Stream   bool
						Messages []struct {
							Role    string
							Content any
						}
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if !body.Stream {
						loaded.Store(true)
						fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`)
						return
					}
					prompt, subject, firstSubject, imageCount, textTokens := "", "", "", 0, 0
					for _, msg := range body.Messages {
						text, ok := msg.Content.(string)
						if !ok {
							for _, value := range msg.Content.([]any) {
								part := value.(map[string]any)
								if part["type"] == "text" {
									text = part["text"].(string)
								} else if part["type"] == "image_url" {
									url := part["image_url"].(map[string]any)["url"].(string)
									data, err := base64.StdEncoding.DecodeString(strings.SplitN(url, ",", 2)[1])
									if err != nil {
										t.Error(err)
									}
									subject = string(data)
									if imageCount == 0 {
										firstSubject = subject
									}
									imageCount++
								}
							}
						}
						// Include four chat framing tokens per message; the
						// raw-text tokenizer above intentionally excludes them.
						textTokens += 4 + 2*len(strings.Fields(text))
						if msg.Role == "user" {
							prompt = text
						}
					}
					match := markerPattern.FindStringSubmatch(prompt)
					if len(match) != 3 {
						http.Error(w, "missing marker", 400)
						return
					}
					session, _ := strconv.Atoi(match[1])
					turn, _ := strconv.Atoi(match[2])
					if tc.workload == "images" || (tc.workload == "all" || tc.workload == "") && imageCount > 0 {
						wantCount, index := 1, (session-1)%2
						if turn >= 11 {
							wantCount, index = 2, (index+1)%2
						}
						wantSubject := []string{"giraffe", "astronaut"}[index]
						if imageCount != wantCount || subject != wantSubject {
							t.Errorf("session=%d turn=%d: images=%d subject=%q, want %d/%s", session, turn, imageCount, subject, wantCount, wantSubject)
						}
					} else if imageCount != 0 {
						t.Error("text workload included images")
					}
					key := fmt.Sprintf("%t/%s", imageCount > 0, match[0])
					mu.Lock()
					attempt := seen[key]
					seen[key]++
					traceID := strings.Split(r.Header.Get("Traceparent"), "-")[1]
					fmt.Fprintf(serverLog, "{\"msg\":\"request-lifecycle\",\"level\":\"INFO\",\"trace_id\":%q}\n", traceID)
					mu.Unlock()
					if tc.defect == "stale" && turn >= 11 {
						subject = firstSubject
					}
					marker := match[0]
					if tc.defect == "foreign-marker" && attempt > 0 || tc.defect == "text-marker" && imageCount == 0 {
						marker = "BATCH-SESSION-99-TURN-1"
					}
					output := strings.TrimSpace(subject + " " + strings.Repeat(marker+" ", 8))
					if tc.defect == "repeat" && attempt > 0 {
						output = strings.ToUpper(subject) + output[len(subject):] + " extra"
					}
					u := usage{PromptTokens: textTokens + imageCount*384, CompletionTokens: 96}
					if (tc.imc == "on" && (turn > 1 || attempt > 0) && tc.defect != "no-cache") || tc.defect == "cache" {
						u.PromptTokensDetails.CachedTokens = 500
					}
					if (tc.mtp == "on" && tc.defect != "no-draft") || tc.defect == "draft" {
						u.DraftTokens, u.DraftAcceptedTokens, u.DraftCoverage = 64, 48, 0.75
					}
					select {
					case sem <- struct{}{}:
					case <-r.Context().Done():
						return
					}
					defer func() { <-sem }()
					// Model two occupied generation slots; the third request waits.
					time.Sleep(10 * time.Millisecond)
					w.Header().Set("Content-Type", "text/event-stream")
					data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": output}}}})
					fmt.Fprintf(w, "data: %s\n\n", data)
					w.(http.Flusher).Flush()
					time.Sleep(30 * time.Millisecond)
					data, _ = json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{}, "finish_reason": "stop"}}, "usage": u})
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			out := filepath.Join(dir, "out")
			previous := os.Args
			os.Args = []string{"reliability", "-scenario", "batch", "-batch-model", model,
				"-batch-images", strings.Join(paths, ","), "-batch-slots", "2", "-batch-conversations", "3",
				"-mtp", tc.mtp, "-imc", tc.imc, "-host", server.URL, "-out", out, "-server-log", serverLog.Name()}
			if tc.workload != "" {
				os.Args = append(os.Args, "-batch-profile", tc.workload)
			}
			defer func() { os.Args = previous }()
			err = run()
			if (err != nil) != (tc.failure != "") {
				t.Fatalf("error=%v, want failure=%q", err, tc.failure)
			}
			data, err := os.ReadFile(filepath.Join(out, summaryFile))
			if err != nil {
				t.Fatal(err)
			}
			var summary runSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			workloads := []string{tc.workload}
			if tc.workload == "all" || tc.workload == "" {
				workloads = []string{"text", "images"}
			}
			if len(summary.Scenarios) != len(workloads) || len(summary.ModelUnloads) != len(workloads) {
				t.Fatalf("results/unloads=%d/%d, want %d", len(summary.Scenarios), len(summary.ModelUnloads), len(workloads))
			}
			for index, result := range summary.Scenarios {
				workload := workloads[index]
				failure := tc.failure
				if tc.defect == "text-marker" && workload == "images" {
					failure = ""
				}
				if failure != "" && !strings.Contains(strings.Join(result.Failures, "\n"), failure) {
					t.Fatalf("want %q, got %v", failure, result.Failures)
				}
				if result.Details["workload"] != workload {
					t.Fatalf("wrong workload: %v", result.Details)
				}
				if len(workloads) == 2 && result.Name != "batch/"+workload {
					t.Fatalf("wrong result label: %s", result.Name)
				}
				if result.Details["workload_complete"] != (failure == "") {
					t.Fatalf("incorrect workload coverage: %v", result.Details)
				}
				if tc.defect == "overlap" {
					phases := result.Details["turns"].([]any)
					if result.Details["completed_history_turns"] != float64(0) || len(phases) != 1 || phases[0].(map[string]any)["complete"] != true || len(result.Details["requests"].([]any)) != 3 {
						t.Fatalf("failed overlap must retain completed requests without claiming complete workload: %v", result.Details)
					}
				}
				if failure == "" {
					if result.Status != "PASS" {
						t.Fatalf("%s failed: %v", workload, result.Failures)
					}
					wantRequests := 63
					if workload == "images" {
						wantRequests = 69
					}
					requests := result.Details["requests"].([]any)
					if len(requests) != wantRequests {
						t.Fatalf("requests=%d, want %d", len(requests), wantRequests)
					}
					if summary.ModelUnloads[index].Status != "PASS" {
						t.Fatalf("cleanup=%v", summary.ModelUnloads)
					}
					log, err := os.ReadFile(filepath.Join(out, toolLogFile))
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(log), "workload="+workload) {
						t.Fatal("tool log omitted workload")
					}
					for _, raw := range requests {
						request := raw.(map[string]any)
						if request["generation_seconds"].(float64) < 0.025 || request["elapsed_seconds"].(float64) <= request["generation_seconds"].(float64) || request["client_output_tokens_per_second"].(float64) <= 0 {
							t.Fatalf("missing/incorrect request timing: %v", request)
						}
						if workload == "images" && (request["output"] == nil || request["expected_subject"] == nil || request["image_count"] == nil) {
							t.Fatalf("missing image evidence: %v", request)
						}
					}
					for _, raw := range result.Details["turns"].([]any) {
						phase := raw.(map[string]any)
						if phase["complete"] != true || phase["output_tokens"] != float64(288) || phase["elapsed_seconds"].(float64) < 0.075 {
							t.Fatalf("incorrect phase measurements: %v", phase)
						}
						want := 288 / phase["elapsed_seconds"].(float64)
						if phase["output_tokens_per_second"] != want {
							t.Fatalf("phase throughput=%v, want %v", phase["output_tokens_per_second"], want)
						}
					}
				}
			}
			if len(workloads) == 2 {
				data, err := os.ReadFile(filepath.Join(out, eventsFile))
				if err != nil {
					t.Fatal(err)
				}
				counts := make(map[string]int)
				for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
					var event evidenceEvent
					if err := json.Unmarshal([]byte(line), &event); err != nil {
						t.Fatal(err)
					}
					counts[event.Scenario]++
				}
				for _, result := range summary.Scenarios {
					if counts[result.Name] != len(result.Details["requests"].([]any)) {
						t.Fatalf("%s event count=%d, want one per workload request", result.Name, counts[result.Name])
					}
				}
			}
		})
	}
}

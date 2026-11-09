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
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

func TestMatrixCommandArtifactsAndCleanup(t *testing.T) {
	for _, defect := range []string{"", "response", "unload", "pressure", "pressure-response"} {
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
			loads, measuredRequests := 0, 0
			recoveries := make(map[string]int)
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
					if !loaded[body.Model] {
						loads++
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
						measuredRequests++
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
						if strings.Contains(prompt, "HYBRID-RECOVERY-") {
							recoveries[body.Model]++
							if defect == "pressure-response" && strings.HasSuffix(body.Model, "MTP-ON-IMC-OFF") && recoveries[body.Model]%3 == 0 {
								answer = "stale state on later recovery"
							}
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
						fmt.Fprintf(w, `{"model_config":{"incremental-cache":%t,"context-window":8192}}`, strings.HasSuffix(r.URL.Path, "IMC-ON"))
					} else {
						http.NotFound(w, r)
					}
				}
			}))
			defer server.Close()
			out := filepath.Join(dir, "out")
			previous := os.Args
			os.Args = []string{"reliability", "-matrix", "all", "-scenario", "text-state,long-context", "-text-state-model", base, "-long-context-model", base,
				"-long-context-stages", "512", "-host", server.URL, "-out", out, "-server-log", logPath, "-timeout", "1s", "-repetitions", "2", "-run-label", "operator-label"}
			if strings.HasPrefix(defect, "pressure") {
				os.Args = append(os.Args, "-recovery-cycles", "3")
			}
			defer func() { os.Args = previous }()
			err = run()
			if (err != nil) != (defect != "" && defect != "pressure") {
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
			if defect == "response" || defect == "pressure-response" {
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
				if result.Cell != cells[(i%8)/2] || result.Name != []string{"text-state", "long-context"}[i%2] || result.Repetition != i/8+1 {
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
			if strings.HasPrefix(defect, "pressure") && (loads != 16 || measuredRequests != 136) {
				t.Errorf("cycles must execute all requests without reloading: loads=%d, measured=%d", loads, measuredRequests)
			}
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
	got, err := parseScenarios("images,mtp,images,long-context")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"images", "long-context", "mtp"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseScenarios() = %v, want %v", got, want)
	}
}

func TestMatrixSelection(t *testing.T) {
	cfg, _, err := parseConfig([]string{"-scenario", "images", "-matrix", "MTPoff/IMCon,MTPon/IMCoff,MTPoff/IMCon", "-image-model", "fixture/vision/TOOLS-MTP-ON-IMC-ON", "-text-state-model", "unused/custom/alias"})
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
		{"-imc", "auto"},
		{"-matrix", "all", "-mtp", "on"},
		{"-matrix", "MTPon/invalid"},
		{"-matrix", "MTPon/IMCon,"},
		{"-matrix", "IMCoff/MTPon"},
		{"-matrix", "MTPon/MTPoff,IMCoff/IMCoff"},
		{"-matrix", "on/off"},
		{"-scenario", "hybrid-state"},
		{"-scenario", "media"},
		{"-hybrid-model", "fixture/text"},
		{"-media-profile", "correctness"},
		{"-scenario", "images", "-image-profile", "correctness"},
		{"-scenario", "images", "-matrix", "all", "-image-model", "fixture/vision/AGENT"},
		{"-batch-profile", "invalid"},
		{"-batch-profile", "images", "-batch-images", "a.jpg"},
		{"-batch-profile", "images", "-batch-images", "a.jpg,a.jpg"},
		{"-batch-profile", "images", "-batch-image-subjects", "deer,deer"},
		{"-expected-state", "Dense"},
		{"-image-slots", "0"},
		{"-scenario", "images", "-image-profile", "state", "-image-require-isolated-prefill"},
		{"-scenario", "long-context", "-image-require-isolated-prefill"},
		{"-recovery-cycles", "0"},
		{"-mtp-waves", "0"},
		{"-mtp-min-rollback-rounds", "-1"},
		{"-scenario", "images", "-image-profile", "prefill", "-recovery-cycles", "2"},
		{"-scenario", "long-context", "-mtp-waves", "2"},
		{"-mtp", "off", "-mtp-min-rollback-rounds", "2"},
		{"-matrix", "all", "-mtp-min-rollback-rounds", "2"},
	} {
		// Supply explicit selections so these still test their intended invalid
		// workload options rather than failing at the selector requirement.
		defaults := []string{"-scenario", "all"}
		if !slices.Contains(args, "-matrix") {
			defaults = append(defaults, "-mtp", "on", "-imc", "on")
		}
		args = append(defaults, args...)
		if _, _, err := parseConfig(args); err == nil {
			t.Errorf("accepted invalid options %v", args)
		} else if slices.Contains(args, "auto") && !strings.Contains(err.Error(), "invalid matrix cell") {
			t.Errorf("invalid mode rejected for the wrong reason: %v", err)
		}
	}
}

func TestMTPPressureCommand(t *testing.T) {
	for _, tc := range []struct{ name, profile, imc, defect string }{
		{name: "embedded", profile: "embedded", imc: "off"},
		{name: "companion", profile: "companion", imc: "on"},
		{name: "target-only", profile: "embedded", imc: "off"},
		{name: "zero-acceptance", profile: "embedded", imc: "off", defect: "zero-accepted"},
		{name: "late-output", profile: "embedded", imc: "off", defect: "output"},
		{name: "session-leak", profile: "embedded", imc: "off", defect: "foreign"},
		{name: "recovery-failure", profile: "embedded", imc: "off", defect: "recovery"},
		{name: "fallback", profile: "embedded", imc: "off", defect: "fallback"},
		{name: "no-drafts", profile: "embedded", imc: "off", defect: "drafts"},
		{name: "no-logs-required", profile: "embedded", imc: "off", defect: "logs"},
		{name: "no-logs-for-coverage", profile: "embedded", imc: "off", defect: "logs"},
		{name: "missing-round", profile: "embedded", imc: "off", defect: "short"},
		{name: "wrong-request", profile: "embedded", imc: "off", defect: "trace"},
		{name: "no-rejection", profile: "embedded", imc: "off", defect: "accepted"},
		{name: "wrong-backend", profile: "embedded", imc: "off", defect: "backend"},
		{name: "unknown-strategy", profile: "embedded", imc: "off", defect: "strategy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const model = "fixture/pressure/TOOLS"
			dir := t.TempDir()
			logPath := filepath.Join(dir, "server.log")
			log, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			mtp := "on"
			if tc.name == "target-only" {
				mtp = "off"
			}
			var mu sync.Mutex
			loaded, loads, measured := false, 0, 0
			warmTrace := ""
			promptRequests := make(map[string]int)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.URL.Path {
				case "/v1/kronk/models/ps":
					if loaded {
						fmt.Fprintf(w, `[{"id":%q,"slots":2}]`, model)
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/v1/kronk/models/" + model:
					fmt.Fprintf(w, `{"model_config":{"incremental-cache":%t}}`, tc.imc == "on")
				case "/v1/kronk/models/slots":
					fmt.Fprintf(w, `[{"model_id":%q,"mtp":%t,"ndraft":3,"slots":[{"id":0,"phase":"idle"},{"id":1,"phase":"idle"}]}]`, model, mtp == "on")
				case "/v1/kronk/models/unload":
					loaded = false
					fmt.Fprintf(w, `{"id":%q,"status":"unloaded"}`, model)
				case "/v1/tokenize":
					var body struct{ Input string }
					json.NewDecoder(r.Body).Decode(&body)
					fmt.Fprintf(w, `{"tokens":%d}`, len(strings.Fields(body.Input)))
				case "/v1/chat/completions":
					var body struct{ Messages []message }
					json.NewDecoder(r.Body).Decode(&body)
					trace := strings.Split(r.Header.Get("Traceparent"), "-")[1]
					prompt := body.Messages[0].Content
					if !loaded {
						loads++
						loaded = true
					}
					if prompt == "Reply OK." {
						warmTrace = trace
						selection, source := "draft-model-mtp", "auto-detected"
						if tc.profile == "companion" {
							selection, source = "draft-model-mtp-separate", "mtp-drafter-file"
						}
						json.NewEncoder(log).Encode(map[string]any{"msg": selection, "status": "loaded", "backend": "qwen35-own-kv", "source": source, "trace_id": trace})
						fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`)
						return
					}
					measured++
					promptRequests[prompt]++
					// Independent fixture answers: do not derive expected facts from
					// the probe's validator or vocabulary table.
					answer := fmt.Sprintf("MTP-SESSION-0001: orbit, nebula, telescope, galaxy, comet. Wording %d", promptRequests[prompt])
					if strings.HasPrefix(prompt, "REQUEST-2 ") {
						answer = fmt.Sprintf("MTP-SESSION-0002: recipe, skillet, rosemary, pastry, simmer. Wording %d", promptRequests[prompt])
					}
					if strings.HasPrefix(prompt, "Reply with exactly MTP-RECOVERY-") {
						answer = strings.Fields(prompt)[3]
						if tc.defect == "recovery" {
							answer = "stale previous session"
						}
					}
					late := promptRequests[prompt] == 3 && strings.HasPrefix(prompt, "REQUEST-2 ")
					if tc.defect == "output" && late {
						answer = "MTP-SESSION-0002: recipe skillet rosemary pastry incorrect-value"
					}
					if tc.defect == "foreign" && late {
						answer += " MTP-SESSION-0001"
					}
					if mtp == "on" {
						backend, strategy := "qwen35-own-kv", "recurrent-native"
						if tc.profile == "companion" {
							strategy = "recurrent-snapshot-replay"
						}
						for round := range 2 {
							accepted, evidenceTrace := round, trace
							if late {
								switch tc.defect {
								case "short":
									if round == 1 {
										continue
									}
								case "trace":
									evidenceTrace = warmTrace
								case "accepted":
									accepted = 3
								case "backend":
									backend = "classic-own-kv"
								case "strategy":
									strategy = "unknown"
								}
							}
							json.NewEncoder(log).Encode(map[string]any{"msg": "speculative", "status": "target-rollback", "backend": backend, "strategy": strategy, "trace_id": evidenceTrace, "accepted": accepted, "drafted": 3})
						}
						json.NewEncoder(log).Encode(map[string]any{"msg": "speculative", "status": "draft-kv-cleared", "trace_id": trace})
					}
					u := usage{PromptTokens: 250, CompletionTokens: 32}
					if mtp == "on" {
						u.DraftTokens, u.DraftAcceptedTokens, u.DraftCoverage = 24, 16, 0.5
					}
					if tc.defect == "zero-accepted" {
						u.DraftAcceptedTokens, u.DraftAcceptanceRate = 0, 0
					}
					if late && tc.defect == "fallback" {
						u.DraftDisableReason = "target-only-fallback"
					}
					if late && tc.defect == "drafts" {
						u.DraftTokens, u.DraftAcceptedTokens, u.DraftCoverage = 0, 0, 0
					}
					if tc.imc == "on" && promptRequests[prompt] > 1 {
						u.PromptTokensDetails.CachedTokens = 200
					}
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}, "usage": u})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			previous := os.Args
			os.Args = []string{"reliability", "-scenario", "mtp", "-mtp-profile", tc.profile, "-mtp-embedded-model", model, "-mtp-companion-model", model, "-mtp-embedded-slots", "2", "-mtp-waves", "3", "-mtp-prompt-tokens", "64", "-mtp", mtp, "-imc", tc.imc, "-host", server.URL, "-out", filepath.Join(dir, "out"), "-server-log", logPath}
			if mtp == "on" && tc.name != "no-logs-required" {
				os.Args = append(os.Args, "-mtp-min-rollback-rounds", "2")
			}
			if tc.defect == "logs" {
				os.Args = append(os.Args, "-server-log", logPath+"-absent")
			}
			defer func() { os.Args = previous }()
			behaviorFailure := tc.defect == "output" || tc.defect == "foreign" || tc.defect == "recovery" || tc.defect == "fallback" || tc.defect == "drafts"
			incomplete := tc.defect == "short" || tc.defect == "trace" || tc.defect == "accepted" || tc.name == "no-logs-for-coverage"
			if err := run(); (err != nil) != (behaviorFailure || incomplete) {
				t.Fatalf("error=%v, defect=%q", err, tc.defect)
			}
			data, err := os.ReadFile(filepath.Join(dir, "out", summaryFile))
			if err != nil {
				t.Fatal(err)
			}
			var summary runSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			failures := strings.Join(summary.Scenarios[0].Failures, "\n")
			if (summary.Status == "FAIL") != behaviorFailure || (summary.Scenarios[0].Status == "FAIL") != behaviorFailure || (len(failures) > 0) != behaviorFailure {
				t.Fatalf("behavior verdict=%s/%s, failures=%s", summary.Status, summary.Scenarios[0].Status, failures)
			}
			if (summary.CoverageStatus == "INCOMPLETE") != incomplete {
				t.Fatalf("coverage status=%q, want incomplete=%t", summary.CoverageStatus, incomplete)
			}
			logData, err := os.ReadFile(filepath.Join(dir, "out", toolLogFile))
			if err != nil || incomplete && !strings.Contains(string(logData), "Requested path coverage: INCOMPLETE") {
				t.Fatalf("coverage must be visible in tool.log: %s, %v", logData, err)
			}
			profiles := summary.Scenarios[0].Details["profiles"].([]any)
			requests := profiles[0].(map[string]any)["results"].([]any)
			if len(requests) != 6 {
				t.Fatalf("requests=%d, want 6", len(requests))
			}
			if tc.defect == "" && mtp == "on" {
				for _, request := range requests {
					if request.(map[string]any)["rollback_rounds"] != float64(2) {
						t.Fatalf("missing rollback evidence: %v", request)
					}
				}
			}
			mu.Lock()
			if loads != 1 || measured != 7 || loaded {
				t.Errorf("waves did not retain and finally release the context: loads=%d requests=%d loaded=%t", loads, measured, loaded)
			}
			mu.Unlock()
		})
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
		`{"msg":"start-slot","status":"imc-reuse","trace_id":"known"}`,
		`{"msg":"batch-engine","status":"slot-finished","trace_id":"foreign"}`,
		`{"msg":"batch-engine","status":"slot-finished","trace_id":"known","slot":2}`,
		`{"msg":"request-lifecycle","status":"complete","trace_id":"known","stage":4}`,
		`{"msg":"draft-model-mtp-separate","status":"loaded","trace_id":"known","backend":"qwen35-own-kv","source":"auto-detected"}`,
		`{"msg":"speculative","status":"draft-kv-cleared","trace_id":"known","slot":2}`,
		`{"msg":"speculative","status":"target-rollback","trace_id":"known","accepted":1,"drafted":3,"strategy":"recurrent-native"}`,
		`{"msg":"batch-engine","status":"slot-finished","trace_id":"known","draft_target_rollbacks":{"recurrent-native":3,"future-strategy":5,"fraction":2.5,"zero":0,"negative":-4,"string":"9"}}`,
		`{"msg":"batch-engine","status":"slot-finished","trace_id":"foreign","draft_target_rollbacks":{"recurrent-native":100}}`,
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
	if evidence.MatchedEvents != 7 {
		t.Fatalf("matched events = %d, want 7", evidence.MatchedEvents)
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
	summary := runSummary{ServerEvidence: evidence, Scenarios: []scenarioResult{{Name: "mtp", ExpectedModes: map[string]string{"mtp": "on"}, Details: map[string]any{"profiles": []mtpProfileResult{{Name: "embedded", MinRollbackRounds: 2, Results: []mtpRequestResult{{TraceID: "known"}, {TraceID: "missing"}}}}}}}}
	annotateMTPServerEvidence(&summary)
	requests := summary.Scenarios[0].Details["profiles"].([]mtpProfileResult)[0].Results
	if requests[0].RollbackRounds != 8 || !reflect.DeepEqual(requests[0].RollbackStrategies, map[string]int{"recurrent-native": 3, "future-strategy": 5}) || requests[1].RollbackRounds != 0 {
		t.Fatalf("request totals must replace legacy events and exclude unrelated/invalid counts: %+v", requests)
	}
	if got := summary.Scenarios[0].Coverage; len(got) != 2 || got[0].Status != "exercised" || got[1].Status != "not exercised" {
		t.Fatalf("incorrect coverage: %+v", got)
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
	requests = []batchRequestResult{
		{firstContentAt: origin, finishedAt: origin.Add(time.Second)},
		{firstContentAt: origin.Add(time.Second), finishedAt: origin.Add(2 * time.Second)},
	}
	if got := maximumBatchConcurrency(requests); got != 1 {
		t.Fatalf("touching generation intervals overlap: got %d, want 1", got)
	}
}

func TestMaximumEventGapFiltersWindow(t *testing.T) {
	origin := time.Unix(100, 0)
	events := []time.Time{
		origin,
		origin.Add(2 * time.Second),
		origin.Add(2500 * time.Millisecond),
		origin.Add(5 * time.Second),
	}
	got := maximumEventGap(events, func(start, end time.Time) bool {
		return !start.Before(origin.Add(2*time.Second)) && !end.After(origin.Add(2500*time.Millisecond))
	})
	if got != 0.5 {
		t.Fatalf("maximum event gap = %f, want 0.5", got)
	}
}

func TestLowLevelAliasesMatchConfiguration(t *testing.T) {
	doc, err := modelconfig.Load("../../zarf/kms/tools_model_config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, mtp := range []string{"on", "off"} {
		for _, imc := range []string{"on", "off"} {
			cfg, _, err := parseConfig([]string{"-scenario", "all", "-mtp", mtp, "-imc", imc})
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
	cfg, _, err := parseConfig([]string{"-scenario", "images", "-mtp", "off", "-imc", "on", "-image-model", "custom/model/PROFILE"})
	if err != nil || cfg.MediaModel != "custom/model/PROFILE" {
		t.Fatalf("explicit model override changed: %s, %v", cfg.MediaModel, err)
	}
}

// Exercise the command and its artifacts against a controlled HTTP/SSE server.
// These fixtures validate the tools, not model-backed Kronk behavior.
func TestImageStateCommandModesAndStateFailures(t *testing.T) {
	cases := []struct{ name, mtp, imc, defect, failure string }{
		{"on-on", "on", "on", "", ""},
		{"on-off", "on", "off", "", ""},
		{"off-on", "off", "on", "", ""},
		{"off-off", "off", "off", "", ""},
		{"cycles", "off", "on", "cycles", ""},
		{"late-cycle", "off", "off", "late-cycle", "image-state cycle 3: image recovery lacks"},
		{"two-slots", "off", "off", "two-slots", ""},
		{"hybrid", "off", "off", "hybrid", ""},
		{"loaded-slots", "off", "off", "loaded-slots", "loaded model reports 2 slots, want 1"},
		{"diagnostic-slots", "off", "off", "diagnostic-slots", "scheduler reports 2 slots, want 1"},
		{"missing-projection", "off", "off", "missing-projection", "must report has_projection=true"},
		{"false-projection", "off", "off", "false-projection", "must report has_projection=true"},
		{"wrong-state", "off", "off", "wrong-state", "GGUF sequence processing"},
		{"missing-metadata", "off", "off", "missing-metadata", "metadata required"},
		{"wrong-mtp-config", "on", "on", "wrong-mtp", "observed MTP=false"},
		{"wrong-imc-config", "off", "off", "wrong-imc", "observed MTP=false IMC=true"},
		{"no-reuse", "on", "on", "no-cache", "reported no cached tokens"},
		{"unexpected-reuse", "off", "off", "cache", "IMC off: reported"},
		{"image-fallback", "on", "off", "no-draft", "did not demonstrate MTP"},
		{"unexpected-drafting", "off", "on", "draft", "MTP off: reported drafting"},
		{"bad-append", "off", "off", "append", "appended image response lacks"},
		{"bad-repeat-prefix", "off", "off", "repeat-prefix", "repeat image response lacks a nonempty"},
		{"bad-repeat-subject", "off", "off", "repeat-subject", "repeat image response lacks expected terms"},
		{"bad-recovery", "off", "off", "recovery", "image recovery lacks"},
		{"stuck-cancellation", "off", "off", "busy", "did not release all slots"},
		{"no-cancellation-content", "off", "off", "no-content", "expected disconnect after content"},
		{"missing-usage", "off", "off", "usage", "lacks positive prompt/completion usage"},
		{"server-error", "off", "off", "finish-error", "completion finish reason=\"error\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const model = "fixture/vision/TOOLS"
			var cancelled atomic.Bool
			var loaded atomic.Bool
			var cancellations, warmups atomic.Int32
			var observations atomic.Int32
			loaded.Store(true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/kronk/models/ps":
					if loaded.Load() {
						slots := 1
						if tc.defect == "loaded-slots" || tc.defect == "two-slots" {
							slots = 2
						}
						entry := map[string]any{"id": model, "slots": slots}
						json.NewEncoder(w).Encode([]any{entry})
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/v1/kronk/models/unload":
					loaded.Store(false)
					fmt.Fprintf(w, `{"id":%q,"status":"unloaded"}`, model)
				case "/v1/kronk/models/" + model:
					metadata := map[string]string{"general.architecture": "qwen3"}
					if tc.defect == "wrong-state" || tc.defect == "hybrid" {
						metadata = map[string]string{"general.architecture": "qwen35", "qwen35.block_count": "8"}
					}
					if tc.defect == "missing-metadata" {
						metadata = nil
					}
					detail := map[string]any{"metadata": metadata, "model_config": map[string]any{"incremental-cache": tc.imc == "on" || tc.defect == "wrong-imc"}}
					if tc.defect != "missing-projection" {
						detail["has_projection"] = tc.defect != "false-projection"
					}
					json.NewEncoder(w).Encode(detail)
				case "/v1/kronk/models/slots":
					phase := "idle"
					if tc.defect == "busy" && cancelled.Load() {
						phase = "generation"
					}
					slots := []slotDiagnostic{{ID: 0, Phase: phase}}
					if tc.defect == "diagnostic-slots" || tc.defect == "two-slots" {
						slots = append(slots, slotDiagnostic{ID: 1, Phase: "idle"})
					}
					json.NewEncoder(w).Encode([]engineDiagnostic{{ModelID: model, MTP: tc.mtp == "on" && tc.defect != "wrong-mtp", NDraft: 3, Slots: slots}})
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
						warmups.Add(1)
						fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`)
						return
					}
					if body.Stream {
						cancelled.Store(true)
						cancellations.Add(1)
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
					if strings.HasPrefix(answer, "MEDIA-OBSERVATION") && observations.Add(1)%2 == 0 {
						answer += " standing beside a tree"
						if tc.defect == "repeat-prefix" {
							answer = "A giraffe beside a tree"
						}
						if tc.defect == "repeat-subject" {
							answer = "MEDIA-OBSERVATION a boat"
						}
					}
					if tc.defect == "append" && strings.HasPrefix(answer, "MEDIA-APPEND") || tc.defect == "recovery" && strings.HasPrefix(answer, "MEDIA-RECOVERY") {
						answer = "wrong subject and stale prefix"
					}
					if tc.defect == "late-cycle" && cancellations.Load() == 3 && strings.HasPrefix(answer, "MEDIA-RECOVERY") {
						answer = "previous cycle's image state"
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
					finish := "stop"
					if tc.defect == "finish-error" {
						finish = "error"
					}
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": finish}}, "usage": u})
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
			os.Args = []string{"reliability", "-scenario", "images", "-image-profile", "state", "-image-model", model, "-image-file", image,
				"-host", server.URL, "-out", filepath.Join(dir, "out"), "-server-log", filepath.Join(dir, "absent.log"), "-mtp", tc.mtp, "-imc", tc.imc, "-timeout", "200ms"}
			expectedSlots, expectedState := "1", "Attention-only"
			if tc.defect == "two-slots" {
				expectedSlots = "2"
			}
			if tc.defect == "hybrid" {
				expectedState = "Hybrid"
			}
			os.Args = append(os.Args, "-image-slots", expectedSlots, "-expected-state", expectedState)
			if tc.defect == "cycles" || tc.defect == "late-cycle" {
				os.Args = append(os.Args, "-recovery-cycles", "3")
			}
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
			if tc.defect == "cycles" || tc.defect == "late-cycle" {
				cycles := summary.Scenarios[0].Details["correctness_cycles"].([]any)
				if len(cycles) != 3 || cancellations.Load() != 3 || warmups.Load() != 1 {
					t.Fatalf("recovery cycles must retain the loaded context: cycles=%d cancellations=%d loads=%d", len(cycles), cancellations.Load(), warmups.Load())
				}
			}
			if tc.failure != "" && !strings.Contains(strings.Join(summary.Scenarios[0].Failures, "\n"), tc.failure) {
				t.Fatalf("missing expected failure %q: %v", tc.failure, summary.Scenarios[0].Failures)
			}
			var cycles []any
			if cycle, ok := summary.Scenarios[0].Details["correctness"]; ok {
				cycles = []any{cycle}
			} else if recorded, ok := summary.Scenarios[0].Details["correctness_cycles"].([]any); ok {
				cycles = recorded
			}
			for _, cycle := range cycles {
				cycle := cycle.(map[string]any)
				outputs, captured := cycle["outputs_on_failure"].(map[string]any)
				failures, _ := cycle["failures"].([]any)
				if captured != (len(failures) > 0) {
					t.Fatalf("response capture must match failed cycles only: %v", cycle)
				}
				if captured {
					if len(outputs) != 6 || !strings.HasPrefix(outputs["text_only"].(string), "TEXT-ONLY-") {
						t.Fatalf("missing diagnostic response context: %v", outputs)
					}
					switch tc.defect {
					case "repeat-prefix":
						if outputs["repeat_image"] != "A giraffe beside a tree" {
							t.Fatalf("failed repeat output not preserved: %v", outputs)
						}
					case "repeat-subject":
						if outputs["repeat_image"] != "MEDIA-OBSERVATION a boat" {
							t.Fatalf("failed subject output not preserved: %v", outputs)
						}
					case "append", "recovery":
						phase := "appended_image"
						if tc.defect == "recovery" {
							phase = "recovery_image"
						}
						if outputs[phase] != "wrong subject and stale prefix" {
							t.Fatalf("failed state output not preserved: %v", outputs)
						}
					}
				}
			}
		})
	}
}

func TestMediaPrefillCommandTiming(t *testing.T) {
	for _, tc := range []struct {
		name            string
		failImage       bool
		requireIsolated bool
		status          string
		tokens          int
		imcEvidence     string
		wrongTrace      bool
		missingLog      bool
		failure         bool
		cells           string
		failShared      bool
		slowGap         bool
		gapLimit        string
	}{
		{name: "ordinary"},
		{name: "gap-warning", slowGap: true, cells: "38", status: "encoding-image", tokens: 442},
		{name: "gap-explicit-hard-limit", slowGap: true, gapLimit: "1s", failure: true},
		{name: "gap-permissive-hard-limit", slowGap: true, gapLimit: "5s"},
		{name: "image-failed", failImage: true, failure: true},
		{name: "isolated", requireIsolated: true, status: "encoding-image", tokens: 513},
		{name: "equal-limit", requireIsolated: true, status: "encoding-image", tokens: 512, failure: true},
		{name: "below-limit", requireIsolated: true, status: "encoding-image", tokens: 511, failure: true},
		{name: "shared", requireIsolated: true, status: "encoding-shared", tokens: 513, failure: true},
		{name: "text-trace", requireIsolated: true, status: "encoding-image", tokens: 513, wrongTrace: true, failure: true},
		{name: "missing-log", requireIsolated: true, status: "encoding-image", tokens: 513, missingLog: true, failure: true},
		{name: "missing-event", requireIsolated: true, failure: true},
		{name: "imc-image", requireIsolated: true, imcEvidence: "image", tokens: 513},
		{name: "imc-equal-limit", requireIsolated: true, imcEvidence: "image", tokens: 512, failure: true},
		{name: "imc-audio", requireIsolated: true, imcEvidence: "audio", tokens: 513, failure: true},
		{name: "imc-unfinished", requireIsolated: true, imcEvidence: "unfinished", tokens: 513, failure: true},
		{name: "imc-missing-start", requireIsolated: true, imcEvidence: "missing-start", tokens: 513, failure: true},
		{name: "imc-wrong-chunk", requireIsolated: true, imcEvidence: "wrong-chunk", tokens: 513, failure: true},
		{name: "imc-wrong-sequence", requireIsolated: true, imcEvidence: "wrong-sequence", tokens: 513, failure: true},
		{name: "imc-wrong-size", requireIsolated: true, imcEvidence: "wrong-size", tokens: 513, failure: true},
		{name: "imc-text-trace", requireIsolated: true, imcEvidence: "image", tokens: 513, wrongTrace: true, failure: true},
		{name: "selected-two-phases", cells: "41", status: "encoding-shared", tokens: 345},
		{name: "selected-continue-after-failure", cells: "41", status: "encoding-shared", tokens: 345, failShared: true, failure: true},
		{name: "selected-wrong-execution", cells: "41", status: "encoding-image", tokens: 345, failure: true},
		{name: "selected-imc", cells: "57", imcEvidence: "image", tokens: 345},
		{name: "selected-imc-rejects-native", cells: "57", status: "encoding-image", tokens: 345, failure: true},
		{name: "selected-natural-isolation", cells: "38", status: "encoding-image", tokens: 442},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failImage := tc.failImage
			const model = "fixture/media/TOOLS"
			var currentModel atomic.Value
			currentModel.Store(model)
			var loaded, generating, imaging atomic.Bool
			var generationTrace atomic.Value
			dir := t.TempDir()
			logPath := filepath.Join(dir, "server.log")
			if !tc.missingLog {
				if err := os.WriteFile(logPath, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				model := currentModel.Load().(string)
				slots := 2
				if tc.cells == "41" && !strings.Contains(model, "/TOOLS-ISOLATED") {
					slots = 4
				}
				switch r.URL.Path {
				case "/v1/kronk/models/ps":
					if loaded.Load() {
						fmt.Fprintf(w, `[{"id":%q,"slots":%d}]`, model, slots)
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/v1/kronk/models/unload":
					loaded.Store(false)
					fmt.Fprintf(w, `{"id":%q,"status":"unloaded"}`, model)
				case "/v1/kronk/models/" + model:
					architecture := "qwen35"
					if tc.cells == "38" {
						architecture = "qwen2vl"
					}
					fmt.Fprintf(w, `{"has_projection":true,"metadata":{"general.architecture":%q,"qwen35.block_count":"8"},"model_config":{"incremental-cache":%t,"devices":["fixture-GPU"]}}`, architecture, tc.cells == "57")
				case "/v1/kronk/models/slots":
					a, b := "idle", "idle"
					if generating.Load() {
						a = "generation"
					}
					if imaging.Load() {
						b = "media-prefill"
					}
					states := []slotDiagnostic{{ID: 0, Phase: a}, {ID: 1, Phase: b}}
					for id := 2; id < slots; id++ {
						states = append(states, slotDiagnostic{ID: id, Phase: "idle"})
					}
					limit := 512
					if strings.Contains(model, "/TOOLS-ISOLATED") {
						limit = 32
					}
					json.NewEncoder(w).Encode([]engineDiagnostic{{ModelID: model, PrefillBatchSize: limit, NBatch: 514, NUBatch: 512, Slots: states}})
				case "/v1/chat/completions":
					var body struct {
						Model    string
						Stream   bool
						Messages []struct{ Content any }
					}
					json.NewDecoder(r.Body).Decode(&body)
					if !body.Stream {
						if tc.cells != "" && loaded.Load() {
							t.Error("previous phase was not unloaded before loading the next fixture")
						}
						currentModel.Store(body.Model)
						loaded.Store(true)
						fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`)
						return
					}
					_, text := body.Messages[0].Content.(string)
					traceID := strings.Split(r.Header.Get("Traceparent"), "-")[1]
					if text {
						generationTrace.Store(traceID)
					} else if (tc.status != "" || tc.imcEvidence != "") && !tc.missingLog {
						if tc.wrongTrace {
							traceID = generationTrace.Load().(string)
						}
						file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
						if err != nil {
							t.Error(err)
							return
						}
						if tc.imcEvidence == "" {
							status := tc.status
							if tc.cells == "41" && strings.Contains(model, "/TOOLS-ISOLATED") {
								status = "encoding-image"
							}
							fmt.Fprintf(file, "{\"msg\":\"prefill-media\",\"status\":%q,\"trace_id\":%q,\"tokens\":%d}\n", status, traceID, tc.tokens)
						} else {
							kind := 2
							if tc.imcEvidence == "audio" {
								kind = 3
								// An unmatched image on another chunk must not qualify audio.
								fmt.Fprintf(file, "{\"msg\":\"imc-media-cache\",\"status\":\"encoding-media-chunk\",\"trace_id\":%q,\"seq\":0,\"chunk\":9,\"kind\":2,\"tokens\":%d}\n", traceID, tc.tokens)
							}
							if tc.imcEvidence != "missing-start" {
								fmt.Fprintf(file, "{\"msg\":\"imc-media-cache\",\"status\":\"encoding-media-chunk\",\"trace_id\":%q,\"seq\":0,\"chunk\":1,\"kind\":%d,\"tokens\":%d}\n", traceID, kind, tc.tokens)
							}
							seq, chunk, tokens := 0, 1, tc.tokens
							switch tc.imcEvidence {
							case "wrong-sequence":
								seq = 1
							case "wrong-chunk":
								chunk = 2
							case "wrong-size":
								tokens--
							}
							if tc.imcEvidence != "unfinished" {
								fmt.Fprintf(file, "{\"msg\":\"imc-media-cache\",\"status\":\"encoded-media-chunk\",\"trace_id\":%q,\"seq\":%d,\"chunk\":%d,\"tokens\":%d}\n", traceID, seq, chunk, tokens)
							}
						}
						file.Close()
					}
					if !text && (failImage || tc.failShared && !strings.Contains(model, "/TOOLS-ISOLATED")) {
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
					for event := range count {
						if text && tc.slowGap && event == 5 {
							time.Sleep(1100 * time.Millisecond)
						}
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
			image := filepath.Join(dir, "image.jpg")
			if err := os.WriteFile(image, []byte("image"), 0o600); err != nil {
				t.Fatal(err)
			}
			previous := os.Args
			os.Args = []string{"reliability", "-scenario", "images", "-image-profile", "prefill", "-image-slots", "2", "-image-model", model, "-image-file", image, "-mtp", "off", "-imc", "off", "-host", server.URL, "-out", filepath.Join(dir, "out"), "-server-log", logPath, "-repetitions", "2"}
			if tc.cells != "" {
				os.Args = []string{"reliability", "-profiles", "image-prefill", "-cells", testCellIDs(t, tc.cells), "-image-file", image, "-host", server.URL, "-out", filepath.Join(dir, "out"), "-server-log", logPath, "-repetitions", "2"}
			}
			if tc.requireIsolated {
				os.Args = append(os.Args, "-image-require-isolated-prefill")
			}
			if tc.gapLimit != "" {
				os.Args = append(os.Args, "-image-prefill-max-generation-gap", tc.gapLimit)
			}
			defer func() { os.Args = previous }()
			if err := run(); (err != nil) != tc.failure {
				t.Fatalf("error=%v, want failure=%t", err, tc.failure)
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
			if tc.cells != "" {
				want := 2
				if tc.cells == "41" {
					want = 4
				}
				if len(summary.Selection) != 1 || len(summary.Scenarios) != want {
					t.Fatalf("selected pair must retain all its phases/repetitions: %+v", summary)
				}
			}
			for _, scenario := range summary.Scenarios {
				if (scenario.Status == "FAIL") != (tc.failImage || tc.failShared && scenario.Phase == "shared" || tc.gapLimit == "1s") {
					t.Fatalf("wrong scenario verdict: %+v", scenario)
				}
				prefill := scenario.Details["concurrent_prefill"].(map[string]any)
				if prefill["generation_gap_enforced"] != (tc.gapLimit != "") {
					t.Fatalf("wrong generation-gap enforcement: %v", prefill)
				}
				warnings := 0
				for _, coverage := range scenario.Coverage {
					if coverage.Status == "warning" {
						warnings++
						if coverage.Path != "image-prefill UX text gap" {
							t.Fatalf("unexpected warning: %+v", coverage)
						}
					}
				}
				if tc.slowGap && (warnings != 1 || prefill["prefill_maximum_gap_seconds"].(float64) <= 1 || summary.CoverageStatus == "INCOMPLETE") {
					t.Fatalf("slow gap must remain measurable and warn without losing required coverage: %+v", scenario)
				}
				if tc.cells != "" {
					if len(scenario.Coverage)-warnings != 1 || scenario.Phase == "" {
						t.Fatalf("missing phase identity/evidence: %+v", scenario)
					}
					missing := tc.name == "selected-wrong-execution" && scenario.Phase == "shared" || tc.name == "selected-imc-rejects-native"
					for _, coverage := range scenario.Coverage {
						if coverage.Status != "warning" && (coverage.Status == "not exercised") != missing {
							t.Fatalf("wrong path coverage: %+v", scenario)
						}
					}
				}
				if tc.requireIsolated && !tc.failure && prefill["isolated_image_tokens"] != float64(513) {
					t.Fatalf("missing isolated execution evidence: %v", prefill)
				}
				if tc.requireIsolated && tc.failure && (len(scenario.Failures) != 0 || len(scenario.Coverage) != 1 || scenario.Coverage[0].Status != "not exercised" || summary.CoverageStatus != "INCOMPLETE") {
					t.Fatalf("missing isolation coverage must not be a behavior failure: %+v", scenario)
				}
			}
			if tc.slowGap {
				log, err := os.ReadFile(filepath.Join(dir, "out", "tool.log"))
				if err != nil || !strings.Contains(string(log), "WARNING:") || !strings.Contains(string(log), "image-prefill UX text gap") {
					t.Fatalf("missing operator-visible gap warning: %v\n%s", err, log)
				}
			}
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
			firstFailed := failImage || tc.failShared
			if result.WorkloadComplete == firstFailed {
				t.Fatalf("wrong coverage: %+v", result)
			}
			if failImage && (result.Image.Error == "" || result.Image.Performance.ElapsedSeconds <= 0 || result.Image.Performance.FinishedAt.IsZero()) {
				t.Fatalf("failed image must retain elapsed timing: %+v", result.Image)
			}
			if !firstFailed {
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
			if configurations[summary.Scenarios[0].Models[0]].(map[string]any)["scheduler"].(map[string]any)["nbatch"] != float64(514) {
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
		{"off", "off", "finish-error", "stream finish reason=\"error\""},
		{"off", "off", "no-finish", "stream finish reason=\"\""},
		{"off", "off", "missing-capacity", "cannot verify configured context capacity"},
		{"off", "off", "skip-stage", "requested stage 16384 skipped"},
		{"off", "off", "foreign-marker", "marker validation failed"},
	} {
		t.Run(tc.mtp+"-"+tc.imc+"-"+tc.defect, func(t *testing.T) {
			const model = "fixture/long-context/TOOLS"
			markerPattern := regexp.MustCompile(`LONGCTX-S\d+-(?:BEGIN|MIDDLE|END)-[0-9A-F]{8}`)
			var retrievals atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/kronk/models/ps":
					fmt.Fprintf(w, `[{"id":%q,"slots":1}]`, model)
				case "/v1/kronk/models/" + model:
					capacity := 8192
					if tc.defect == "missing-capacity" {
						capacity = 0
					}
					fmt.Fprintf(w, `{"model_config":{"incremental-cache":%t,"context-window":%d}}`, tc.imc == "on", capacity)
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
					if tc.defect == "foreign-marker" {
						output += " LONGCTX-S8192-BEGIN-DEADBEEF"
					}
					if retrievals.Add(1)%2 == 0 {
						output = "\n" + strings.ReplaceAll(output, " ", "\n") + "\n"
					}
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
					finish := "stop"
					if tc.defect == "finish-error" {
						finish = "error"
					} else if tc.defect == "no-finish" {
						finish = ""
					}
					data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": output}, "finish_reason": finish}}, "usage": u})
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			cfg, _, err := parseConfig([]string{"-scenario", "long-context", "-mtp", tc.mtp, "-imc", tc.imc, "-long-context-model", model, "-long-context-stages", "512"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.defect == "skip-stage" {
				cfg.LongContextStages = []int{512, 16384}
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
		{"text", "off", "on", "evidence", ""},
		{"text", "off", "on", "evidence-invalid", ""},
		{"text", "off", "on", "short-output", "produced 7 copies"},
		{"text", "off", "off", "overlap", ""},
		{"images", "off", "off", "overlap", ""},
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
						fmt.Fprintf(w, `[{"id":%q,"slots":2}]`, model)
					} else {
						fmt.Fprint(w, `[]`)
					}
				case "/v1/kronk/models/" + model:
					fmt.Fprintf(w, `{"has_projection":%t,"model_config":{"incremental-cache":%t}}`, tc.defect != "projection", tc.imc == "on")
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
					if strings.HasPrefix(tc.defect, "evidence") || tc.defect == "overlap" {
						// An independent server clock, including valid slot zero and zero wait.
						base := time.Date(2020, 1, 1, 0, 0, turn, 0, time.UTC)
						wait := []time.Duration{0, 2 * time.Millisecond, 40 * time.Millisecond}[session-1]
						slot := (session - 1) % 2
						events := []map[string]any{
							{"stage_name": "schedule-job", "status": "queued", "time": base},
							{"stage_name": "schedule-job", "status": "complete", "time": base.Add(wait), "slot": slot, "queue_wait": wait.String()},
							{"stage_name": "execute-in-slot", "status": "started", "time": base.Add(wait), "slot": slot},
							{"stage_name": "execute-in-slot", "status": "complete", "time": base.Add(wait + 20*time.Millisecond), "slot": slot},
						}
						if tc.defect == "evidence" && session == 2 {
							events[3]["slot"] = 0 // Contradicts this trace's assignment to slot 1.
						}
						if tc.defect == "evidence" && session == 3 {
							events[3]["trace_id"] = "foreign" // Must not complete session 3's timeline.
						}
						if tc.defect == "evidence-invalid" {
							switch session {
							case 1:
								events[1]["slot"] = 0.5
							case 2:
								events[1]["queue_wait"] = "-1ms"
							case 3:
								events[3]["time"] = base.Add(-time.Second)
							}
						}
						for _, event := range append(events, events[1]) { // Identical duplicate is harmless.
							event["msg"] = "request-lifecycle"
							if event["trace_id"] == nil {
								event["trace_id"] = traceID
							}
							if err := json.NewEncoder(serverLog).Encode(event); err != nil {
								t.Error(err)
							}
						}
					}
					mu.Unlock()
					if tc.defect == "stale" && turn >= 11 {
						subject = firstSubject
					}
					marker := match[0]
					if tc.defect == "foreign-marker" && attempt > 0 || tc.defect == "text-marker" && imageCount == 0 {
						marker = "BATCH-SESSION-99-TURN-1"
					}
					output := strings.TrimSpace(subject + " " + strings.Repeat(marker+" ", 8))
					if tc.defect == "short-output" {
						output = strings.TrimSpace(strings.Repeat(marker+" ", 7))
					}
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
			if tc.defect == "overlap" {
				if err != nil {
					t.Fatalf("missing diagnostic progress witness must not fail the command: %v", err)
				}
			} else if err == nil || tc.failure == "" && !strings.Contains(err.Error(), "coverage is incomplete") {
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
				if _, exists := result.Details["requests"]; exists {
					for _, raw := range result.Details["turns"].([]any) {
						turn := raw.(map[string]any)
						x := turn["server_execution"].(map[string]any)
						if turn["queued_requests_basis"] != nil || turn["queued_requests"] != nil {
							t.Fatal("client timing must not infer queue pressure")
						}
						if tc.defect == "evidence" {
							if x["complete_requests"] != float64(1) || x["incomplete_requests"] != float64(1) || x["inconsistent_requests"] != float64(1) || x["observed_queue_entries"] != float64(2) || x["maximum_queue_wait_seconds"] != 0.04 || fmt.Sprint(x["assigned_slots"]) != "[0]" {
								t.Fatalf("incorrect mixed evidence summary: %v", x)
							}
						} else if tc.defect == "evidence-invalid" {
							if x["inconsistent_requests"] != float64(3) || len(x["assigned_slots"].([]any)) != 0 || x["maximum_queue_wait_seconds"] != nil {
								t.Fatalf("malformed evidence must not produce trusted aggregates: %v", x)
							}
						} else if tc.defect == "overlap" {
							if x["complete_requests"] != float64(3) || fmt.Sprint(x["assigned_slots"]) != "[0 1]" || turn["maximum_concurrency"] != float64(1) {
								t.Fatalf("client overlap must remain diagnostic: %v", turn)
							}
						} else if x["incomplete_requests"] != float64(3) || x["maximum_queue_wait_seconds"] != nil || len(x["assigned_slots"].([]any)) != 0 {
							t.Fatalf("missing events must not invent slot/queue evidence: %v", x)
						}
					}
					if tc.defect == "evidence" {
						requests := result.Details["requests"].([]any)
						for _, raw := range requests {
							r := raw.(map[string]any)
							x := r["server_execution"].(map[string]any)
							if r["session"] == float64(1) && (x["slot"] != float64(0) || x["queue_wait_seconds"] != float64(0) || x["status"] != "complete" || x["execution_finished_at"] == nil) {
								t.Fatalf("slot zero and zero wait must be preserved: %v", r)
							}
							if r["session"] == float64(3) && (x["status"] != "incomplete" || x["execution_finished_at"] != nil) {
								t.Fatalf("foreign trace supplied missing finish: %v", r)
							}
						}
					}
				}
				if workload == "text" {
					for _, raw := range result.Details["requests"].([]any) {
						request := raw.(map[string]any)
						if request["error"] != nil {
							if request["output"] == nil {
								t.Fatalf("failed text response omitted output: %v", request)
							}
							if tc.defect == "short-output" {
								marker := fmt.Sprintf("BATCH-SESSION-%02d-TURN-1", int(request["session"].(float64)))
								want := strings.TrimSpace(strings.Repeat(marker+" ", 7))
								if request["output"] != want {
									t.Fatalf("failed output=%q, want %q", request["output"], want)
								}
							}
						}
						if request["error"] == nil && request["output"] != nil {
							t.Fatalf("successful text response should omit output: %v", request)
						}
					}
				}
				if tc.defect == "overlap" {
					phases := result.Details["turns"].([]any)
					wantPhases := 21
					if workload == "images" {
						wantPhases = 23
					}
					if result.Details["completed_history_turns"] != float64(21) || len(phases) != wantPhases || summary.CoverageStatus != "OBSERVED" {
						t.Fatalf("low overlap must finish histories with required scheduling coverage: %v", result.Details)
					}
					log, err := os.ReadFile(filepath.Join(out, toolLogFile))
					if err != nil || !strings.Contains(string(log), "WARNING:") || !strings.Contains(string(log), "coverage: warning — batch generation progress during peer prefill") {
						t.Fatalf("missing progress warning: %s, %v", log, err)
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

// Exercise the emitted scheduling contract with an independent server clock.
// Missing generation progress is a warning; missing ownership/handoff is required coverage.
func TestBatchSchedulingEvidenceReport(t *testing.T) {
	for _, defect := range []string{"", "stalled", "reassigned", "foreign", "stale", "no-wait", "conflict", "serial", "missing"} {
		t.Run(defect, func(t *testing.T) {
			base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			evidence := serverEvidence{Available: true, BatchLifecycle: make(map[string][]map[string]any)}
			var requests []batchRequestResult
			for session := range 3 {
				trace := fmt.Sprintf("trace-%d", session)
				queued, assigned, finished := 0, session, session+4
				slot := session % 2
				if session == 2 {
					queued, assigned, finished = 2, 4, 8
				}
				if defect == "serial" {
					assigned, finished = session*4, (session+1)*4
				}
				if defect == "no-wait" {
					queued = assigned
				}
				if defect == "conflict" && session == 1 {
					slot = 0
				}
				at := func(seconds int) string {
					return base.Add(time.Duration(seconds) * time.Second).Format(time.RFC3339Nano)
				}
				evidence.BatchLifecycle[trace] = []map[string]any{
					{"stage_name": "schedule-job", "status": "queued", "time": at(queued)},
					{"stage_name": "schedule-job", "status": "complete", "time": at(assigned), "slot": float64(slot), "queue_wait": fmt.Sprintf("%ds", assigned-queued)},
					{"stage_name": "execute-in-slot", "status": "started", "time": at(assigned), "slot": float64(slot)},
					{"stage_name": "execute-in-slot", "status": "complete", "time": at(finished), "slot": float64(slot)},
				}
				requests = append(requests, batchRequestResult{Session: session + 1, Turn: 1, Phase: "cold", TraceID: trace, ResponseID: fmt.Sprintf("response-%d", session)})
			}
			if defect == "missing" {
				delete(evidence.BatchLifecycle, "trace-1")
			}
			samples := []engineDiagnostic{
				{Iteration: 10, Slots: []slotDiagnostic{{ID: 0, RequestID: "response-0", Phase: "prefill"}, {ID: 1, RequestID: "response-1", Phase: "generation", GeneratedTokens: 7}}},
				{Iteration: 11, Slots: []slotDiagnostic{{ID: 0, RequestID: "response-0", Phase: "prefill"}, {ID: 1, RequestID: "response-1", Phase: "generation", GeneratedTokens: 9}}},
			}
			switch defect {
			case "stalled":
				samples[1].Slots[1].GeneratedTokens = 7
			case "reassigned":
				samples[1].Slots[1].RequestID = "response-2"
			case "foreign":
				samples[0].Slots[0].RequestID, samples[1].Slots[0].RequestID = "foreign", "foreign"
			case "stale":
				samples[1].Iteration = 10
			}
			summary := runSummary{ServerEvidence: evidence, Scenarios: []scenarioResult{{Name: "batch/text", Status: "PASS", Details: map[string]any{
				"configuration": map[string]any{"slots": 2}, "requests": requests,
				"turns": []batchTurnSummary{{Turn: 1, Phase: "cold", Complete: true, MaximumConcurrency: 1, SchedulerSamples: samples}},
			}}}}
			annotateBatchExecution(&summary)
			scenario := summary.Scenarios[0]
			if scenario.Status != "PASS" || len(scenario.Failures) != 0 {
				t.Fatalf("coverage changed behavior: %+v", scenario)
			}
			wantSlots, wantProgress := "exercised", "exercised"
			switch defect {
			case "stalled", "reassigned", "foreign", "stale":
				wantProgress = "warning"
			case "no-wait", "conflict", "serial", "missing":
				wantSlots = "not exercised"
			}
			if len(scenario.Coverage) != 2 || scenario.Coverage[0].Status != wantSlots || scenario.Coverage[1].Status != wantProgress {
				t.Fatalf("incorrect scheduling coverage: %+v", scenario.Coverage)
			}
			art, err := newArtifacts(filepath.Join(t.TempDir(), "out"), "")
			if err != nil {
				t.Fatal(err)
			}
			defer art.close()
			if err := art.writeLogSummary(summary); err != nil {
				t.Fatal(err)
			}
			log, err := os.ReadFile(filepath.Join(art.dir, toolLogFile))
			if err != nil || !strings.Contains(string(log), "client_response_overlap=1") || strings.Contains(string(log), "inferred_queued") {
				t.Fatalf("incorrect diagnostic labels: %s, %v", log, err)
			}
		})
	}
}

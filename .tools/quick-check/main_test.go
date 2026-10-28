package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestAnswerFacts(t *testing.T) {
	for _, tc := range []struct {
		content  string
		facts    answerFacts
		category string
	}{
		{"RIVER: Alice's destination is (ORANGES). Three giraffes.", answerFacts{required: []string{"River", "Alice", "Orange", "giraffe"}}, ""},
		{"Cherries and deer.", answerFacts{required: []string{"Cherry", "deer"}}, ""},
		{"COBALT\u201183", answerFacts{required: []string{"COBALT-83"}}, ""},
		{"COBALT84", answerFacts{required: []string{"COBALT-83"}}, "CONTENT"},
		{"Riverbank", answerFacts{required: []string{"River"}}, "CONTENT"},
		{"égiraffes", answerFacts{required: []string{"giraffe"}}, "CONTENT"},
		{"A bobcat passed nearby.", answerFacts{foreign: []string{"Bob"}}, ""},
		{"River Alice Orange, not Apples.", answerFacts{required: []string{"River", "Alice", "Orange"}, excluded: []string{"Apple"}}, "CONTENT"},
		{"Mountain Bob Cherries", answerFacts{required: []string{"River"}, foreign: []string{"Cherry"}}, "ISOLATION"},
		{"ORCHID\u201047", answerFacts{foreign: []string{"ORCHID-47"}}, "ISOLATION"},
	} {
		t.Run(tc.content, func(t *testing.T) {
			r := result{Content: tc.content, FinishReason: "stop", Usage: usage{PromptTokens: 10000, CompletionTokens: 20}}
			err := checkAnswer(r, tc.facts, true)
			if tc.category == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || findingCategory(err.Error()) != tc.category {
				t.Fatalf("want %s, got %v", tc.category, err)
			}
		})
	}
}

// Controlled SSE and execution evidence exercise the command, not inference.
// Serial workloads cover continuation, image reuse, cancellation, and partial reports;
// configuration permutations are checked without replaying this fixture.
func TestQuickCheckCommand(t *testing.T) {
	releaseTestRepo(t)
	for _, defect := range []string{"", "content-append", "foreign-session", "interrupt", "images", "images-no-publication", "images-rebuild"} {
		t.Run(defect, func(t *testing.T) {
			images := strings.HasPrefix(defect, "images")
			wantPass := defect == "" || defect == "images"
			root := t.TempDir()
			logPath := filepath.Join(root, "server.log")
			log, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			var mu sync.Mutex
			loaded, calls, cellCalls := false, 0, 0
			model := modelID
			cancelled := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.URL.Path != "/v1/chat/completions" {
					// Wait outside the mutex: the cancelled handler writes cleanup.
					mu.Lock()
					waitForCancel := calls >= 6
					mu.Unlock()
					if req.URL.Path == "/v1/kronk/models/slots" && waitForCancel {
						<-cancelled
					}
					mu.Lock()
					defer mu.Unlock()
					switch req.URL.Path {
					case "/v1/liveness":
						fmt.Fprint(w, `{"vcs_revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","vcs_time":"2026-10-26T00:00:00Z"}`)
					case "/v1/kronk/libs", "/v1/devices":
						fmt.Fprint(w, `{}`)
					case "/v1/tokenize":
						fmt.Fprint(w, `{"tokens":10000}`)
					case "/v1/kronk/models/ps":
						if loaded {
							json.NewEncoder(w).Encode([]map[string]string{{"id": model}})
						} else {
							fmt.Fprint(w, `[]`)
						}
					case "/v1/kronk/models/slots":
						json.NewEncoder(w).Encode([]map[string]any{{"model_id": model, "mtp": true, "ndraft": 5, "slots": []map[string]any{{"id": 0, "phase": "idle"}}}})
					case "/v1/kronk/models/unload":
						loaded = false
						json.NewEncoder(w).Encode(map[string]string{"id": model, "status": "unloaded"})
					default:
						fmt.Fprint(w, `{"has_projection":true,"metadata":{"general.architecture":"qwen35moe","qwen35moe.expert_count":"256"},"model_config":{"incremental-cache":true}}`)
					}
					return
				}
				var body struct {
					Model    string
					Messages []message
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				model, loaded = body.Model, true
				phase := "warmup"
				if body.Messages[0].Content == "Reply OK." {
					cellCalls = 0
				} else {
					phase = []string{"cold", "replay", "append", "fresh-session", "cancel", "recovery"}[cellCalls]
					cellCalls++
				}
				calls++
				mu.Unlock()
				if defect == "interrupt" && phase == "warmup" && calls > 7 {
					process, err := os.FindProcess(os.Getpid())
					if err != nil {
						t.Error(err)
						return
					}
					if err := process.Signal(os.Interrupt); err != nil {
						t.Error(err)
						return
					}
					<-req.Context().Done()
					return
				}
				trace := strings.Split(req.Header.Get("Traceparent"), "-")[1]
				emit := func(msg, status string, fields map[string]any) {
					if fields == nil {
						fields = map[string]any{}
					}
					fields["msg"], fields["status"], fields["trace_id"], fields["time"] = msg, status, trace, time.Now().UTC().Format(time.RFC3339Nano)
					mu.Lock()
					defer mu.Unlock()
					if err := json.NewEncoder(log).Encode(fields); err != nil {
						t.Error(err)
					}
				}
				stable, reused, kind, reason := 10500, 10193, "append", "complete-prefix-append"
				switch phase {
				case "warmup", "cold":
					stable, reused, kind, reason = 10193, 0, "rebuild", "no-complete-prefix"
				case "replay":
					stable, kind, reason = 10193, "exact", "complete-prefix-equal"
				case "fresh-session", "recovery":
					stable, reused, reason = 10223, 10120, "system-cache-prefix"
				case "cancel":
					reused = 10223
				}
				if images {
					switch phase {
					case "replay":
						reason = "logical-plan-equal"
					case "append", "cancel":
						kind, reason = "anchor", "media-prefix-text-replay"
					}
					if defect == "images-rebuild" && (phase == "fresh-session" || phase == "recovery") {
						reused, kind, reason = 0, "rebuild", "no-match"
					}
				}
				for _, stage := range []int{1, 2, 3} {
					emit("request-lifecycle", "complete", map[string]any{"stage": stage, "elapsed": "1ms", "queue_wait": "0s"})
				}
				emit("request-lifecycle", "started", map[string]any{"stage": 4, "slot": 0})
				if phase == "warmup" {
					emit("draft-model-mtp", "loaded", map[string]any{"backend": "qwen35-own-kv"})
				}
				boundary := 0
				if phase == "cold" || phase == "warmup" {
					boundary = 10120
				}
				if images && phase != "warmup" {
					anchor := 0
					if kind == "anchor" || kind == "exact" {
						anchor = reused + 256
					}
					emit("imc-media-cache", "plan-ready", map[string]any{"match_kind": kind, "match_reason": reason, "media_count": 1, "reusable_logical_position": reused, "replay_text_tokens": 7, "anchor_physical_kv": anchor})
					emit("start-slot", "imc-cache-ready", map[string]any{"media": true, "next_logical_position": stable})
					if anchor == 0 {
						emit("imc-media-cache", "encoded-media-chunk", map[string]any{"tokens": 256})
						emit("imc-media-cache", "decoded-media-chunk", map[string]any{"decoded_tokens": 256})
					}
				} else {
					emit("imc", "plan-ready", map[string]any{"match_kind": kind, "match_reason": reason, "stable_tokens": stable, "tail_tokens": 7, "reusable_tokens": reused, "extension_tokens": stable - reused, "system_boundary_tokens": boundary})
				}
				if phase == "cold" && defect != "images-no-publication" {
					emit("imc", "system-cache-published", map[string]any{"tokens": 10120})
				}
				if reused > 0 {
					emit("start-slot", "imc-restore-done", nil)
					emit("start-slot", "imc-draft-restore-done", nil)
				}
				if phase != "replay" {
					emit("start-slot", "imc-snapshot-done", nil)
					emit("start-slot", "imc-draft-snapshot-done", nil)
				}
				emit("batch-engine", "prefill-done", map[string]any{"ttft": "123ms"})
				answer := "River Alice Apple"
				if phase == "append" {
					answer = "River Alice Orange"
					if defect == "content-append" {
						answer += " Not Apple."
					}
				}
				if phase == "fresh-session" {
					answer = "Mountain Bob Lemon"
					if defect == "foreign-session" {
						answer += " Alice"
					}
				}
				if images {
					animal := "giraffe"
					if phase == "fresh-session" || phase == "cancel" {
						animal = "deer"
					}
					answer += " " + animal
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", answer)
				terminal, output := "complete", 90
				if phase == "cancel" {
					w.(http.Flusher).Flush()
					<-req.Context().Done()
					terminal, output = "cancel", 2
					emit("handled error during request", "", map[string]any{"level": "ERROR", "err": map[string]any{"error": map[string]string{"code": "canceled", "message": "context canceled"}}})
				}
				resume := "restored-draft-kv"
				if reused == 0 {
					resume = "fresh-prefill"
				}
				emit("batch-engine", "slot-finished", map[string]any{"total_prompt": stable + 7, "imc_actual_reused_tokens": reused, "output_tokens": output, "draft_tokens": 40, "draft_resume_source": resume, "elapsed": "2.345s"})
				emit("finish-slot", "seq-cleared", nil)
				emit("speculative", "draft-kv-cleared", nil)
				emit("request-lifecycle", terminal, map[string]any{"stage": 4, "slot": 0, "elapsed": "2.5s"})
				if phase == "cancel" {
					close(cancelled)
					return
				}
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":90,\"draft_tokens\":40,\"draft_coverage\":0.5,\"prompt_tokens_details\":{\"cached_tokens\":%d}}}\n\ndata: [DONE]\n\n", stable+7, reused)
			}))
			defer server.Close()
			selection := []string{matrixCells[0].id}
			cellIndex := 0
			if images {
				cellIndex = 1
				selection = []string{matrixCells[cellIndex].id}
			}
			if defect == "interrupt" {
				selection = append(selection, matrixCells[2].id)
			}
			out := filepath.Join(root, "output")
			err = run(options{host: server.URL, out: out, serverLog: logPath, timeout: time.Second, repetitions: 1, cells: selection})
			if (err == nil) != wantPass {
				t.Fatalf("unexpected command result: %v", err)
			}
			paths, _ := filepath.Glob(filepath.Join(out, "*", "summary.json"))
			if len(paths) != 1 {
				t.Fatal("expected one finalized report", paths)
			}
			data, err := os.ReadFile(paths[0])
			if err != nil {
				t.Fatal(err)
			}
			var summary matrixSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			cell := summary.Cells[cellIndex]
			if defect == "interrupt" {
				if summary.Status != "INTERRUPTED" || cell.Status != "PASS" || cell.Measurements == nil || summary.Cells[2].Status != "FAIL" || summary.Cells[2].Measurements != nil || summary.Cells[3].Status != "NOT_RUN" {
					t.Fatalf("lost completed/partial cells: %s", data)
				}
			} else {
				if (cell.Status == "PASS") != wantPass || len(cell.Repetitions) != 1 {
					t.Fatalf("unexpected verdict: %s", data)
				}
				results := cell.Repetitions[0].Results
				if defect == "foreign-session" {
					if len(results) >= 6 || len(cell.Repetitions[0].FindingsByCategory["ISOLATION"]) != 1 {
						t.Fatalf("isolation failure did not stop probes: %s", data)
					}
				} else if len(results) != 6 || ((!images || wantPass) && (results[4]["status"] != "PASS" || results[5]["status"] != "PASS")) {
					t.Fatalf("cancel/recovery probes lost: %s", data)
				}
				if defect == "content-append" && (cell.Measurements != nil || len(cell.Repetitions[0].FindingsByCategory["CONTENT"]) != 1) {
					t.Fatalf("content finding misclassified: %s", data)
				}
				if images && !wantPass && (cell.Measurements != nil || len(cell.Repetitions[0].FindingsByCategory["INFRASTRUCTURE"]) == 0) {
					t.Fatalf("missing image System-cache evidence accepted: %s", data)
				}
				if defect == "images-no-publication" && !strings.Contains(strings.Join(cell.Findings, "\n"), "cold: uncached image build and large System-prefix publication not attested") {
					t.Fatalf("cold image System publication not checked: %s", data)
				}
			}
			if _, err := readComparison(filepath.Dir(paths[0])); err != nil {
				t.Fatal("unreadable artifacts", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if loaded {
				t.Fatal("model left loaded")
			}
		})
	}
}

func TestQuickCheckModelConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(os.Getenv("GITHUB_WORKSPACE"), ".tools", "model_config_tools.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Models map[string]struct {
			Slots       int    `yaml:"nseq-max"`
			Context     int    `yaml:"context-window"`
			IMC         bool   `yaml:"incremental-cache"`
			Speculation string `yaml:"speculation"`
		} `yaml:"models"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	selected, err := selectedCells(nil)
	if err != nil || len(selected) != 32 {
		t.Fatal("default selection must include all 32 cells", selected, err)
	}
	for _, cell := range matrixCells {
		model := "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL"
		if strings.HasPrefix(cell.id, "Dense/") {
			model = "ornith-ai/Ornith-1.5-9B-Q8_0"
		}
		model += "/QUICK-CHECK"
		if cell.noCache {
			model += "-NO-IMC"
		}
		speculation := "mtp"
		if cell.targetOnly {
			model += "-NO-MTP"
			speculation = "disabled"
		}
		slots := 1
		if cell.twoSlots {
			model += "-2S"
			slots = 2
		}
		cfg, ok := config.Models[model]
		if !selected[cell.id] || !ok || cfg.Slots != slots || cfg.Context != 32768 || cfg.IMC == cell.noCache || cfg.Speculation != speculation {
			t.Fatalf("%s: incorrect alias %s: %+v", cell.id, model, cfg)
		}
	}
	selected, err = selectedCells([]string{matrixCells[16].id})
	if err != nil || len(selected) != 1 || !selected[matrixCells[16].id] {
		t.Fatal("explicit Dense selection expanded", selected, err)
	}
}

// A real local origin makes tag recognition deterministic without network mocks.
func releaseTestRepo(t *testing.T) func(...string) string {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	git := func(args ...string) string {
		t.Helper()
		data, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, data)
		}
		return strings.TrimSpace(string(data))
	}
	git("init", "--quiet")
	git("remote", "add", "origin", root)
	return git
}

func TestRunRetentionSafety(t *testing.T) {
	releaseTestRepo(t)
	out := t.TempDir()
	r := comparisonFixture([]float64{1, 4, 10})
	saved := filepath.Join(out, "new-aaaaaaa")
	if err := os.Rename(writeComparisonFixture(t, r), saved); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(saved, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"saved_local_runs", "19990101T000000Z-1"} {
		if err := os.Mkdir(filepath.Join(out, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(saved, filepath.Join(out, "new-link")); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "server.log")
	if err := os.WriteFile(logPath, nil, 0644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Make finalization fail after output creation, before retention.
		paths, _ := filepath.Glob(filepath.Join(out, "20*", "tool.log"))
		for _, path := range paths {
			if err := os.Mkdir(filepath.Join(filepath.Dir(path), "summary.json"), 0755); err != nil && !os.IsExist(err) {
				t.Error(err)
			}
		}
		http.Error(w, "server unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if err := run(options{host: server.URL, out: out, serverLog: logPath, timeout: time.Second, repetitions: 1, cells: []string{matrixCells[0].id}}); err == nil || !strings.Contains(err.Error(), "summary.json: is a directory") {
		t.Fatal("expected finalization failure")
	}
	after, err := os.ReadFile(filepath.Join(saved, "summary.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed finalization changed saved baseline", err)
	}
	for _, name := range []string{"saved_local_runs", "19990101T000000Z-1", "new-link"} {
		if _, err := os.Lstat(filepath.Join(out, name)); err != nil {
			t.Fatal("unmanaged output removed", name, err)
		}
	}
}

func TestReleaseHistory(t *testing.T) {
	git := releaseTestRepo(t)
	paths := make([]string, 2)
	for i, tag := range []string{"v1.0.0", "v1.1.0"} {
		git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "--quiet", "-m", tag)
		git("-c", "tag.gpgsign=false", "tag", tag)
		r := comparisonFixture([]float64{2, 6})
		r.ServerRevision = git("rev-parse", "HEAD")
		paths[i] = writeComparisonFixture(t, r)
	}
	out := filepath.Join(t.TempDir(), "output")
	history := filepath.Join(filepath.Dir(out), "release-history.log")
	args := []string{"-against", "release", "-baseline", paths[0], "-candidate", paths[1], "-out", out}
	for _, flag := range []string{"unknown", "no"} {
		var output bytes.Buffer
		if err := compareCommand(args, &output); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(history)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(data), "Release comparison:") != 1 || !bytes.Contains(data, []byte("v1.0.0 → v1.1.0")) || !bytes.Contains(data, []byte("References updated: "+flag+";")) || !bytes.Contains(data, []byte("Normalized (s)")) || bytes.Contains(data, []byte("WARNING")) {
			t.Fatalf("release history not updated in place: %s", data)
		}
	}
	if err := os.WriteFile(history, []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := compareCommand(args, &output); err == nil {
		t.Fatal("accepted corrupt history")
	}
	if data, err := os.ReadFile(history); err != nil || string(data) != "broken" {
		t.Fatal("overwrote corrupt history", err)
	}
}

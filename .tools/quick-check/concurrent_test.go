package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Exercise the real command/report/comparison with controlled SSE and server
// evidence. This validates the harness, not model quality or Kronk inference.
func TestQuickCheckTwoSlots(t *testing.T) {
	releaseTestRepo(t)
	for _, cell := range matrixCells {
		if cell.twoSlots {
			t.Run(cell.id, func(t *testing.T) { checkTwoSlotFixture(t, cell.id, "") })
		}
	}
	for _, defect := range []string{"serialized", "wrong-recovery-slot", "no-survivor-progress", "system-only-media-append", "missing-cancel-ack", "foreign-session", "content-append", "content-peer-append", "content-survivor", "content-recovery", "content-and-evidence", "plurals"} {
		t.Run(defect, func(t *testing.T) { checkTwoSlotFixture(t, "MoE/2s/IMC+/MTP+/img+", defect) })
	}
	t.Run("uncached-reuse", func(t *testing.T) { checkTwoSlotFixture(t, "MoE/2s/IMC-/MTP+/img+", "uncached-reuse") })
	t.Run("target-only-drafts", func(t *testing.T) { checkTwoSlotFixture(t, "MoE/2s/IMC+/MTP-/img+", "target-only-drafts") })
}

func checkTwoSlotFixture(t *testing.T, selector, defect string) {
	t.Helper()
	passing := defect == "" || defect == "plurals"
	images, noCache, targetOnly := strings.HasSuffix(selector, "img+"), strings.Contains(selector, "IMC-"), strings.Contains(selector, "MTP-")
	denseModel := ""
	if strings.HasPrefix(selector, "Dense/") {
		denseModel = "ornith-ai/Ornith-1.5-9B-Q8_0"
	}
	root := t.TempDir()
	logPath := filepath.Join(root, "server.log")
	serverLog, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer serverLog.Close()
	var mu sync.Mutex
	loaded, model, aCalls := false, "", 0
	active := map[int]bool{}
	var arrivals, prefills sync.WaitGroup
	arrivals.Add(2)
	prefills.Add(2)
	recoveryStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Path != "/v1/chat/completions" {
			mu.Lock()
			defer mu.Unlock()
			switch req.URL.Path {
			case "/v1/liveness":
				fmt.Fprint(w, `{"vcs_revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","vcs_time":"2026-10-26T00:00:00Z","host":"fixture"}`)
			case "/v1/kronk/libs":
				fmt.Fprint(w, `{"current":"fixture"}`)
			case "/v1/devices":
				fmt.Fprint(w, `{"system_ram_bytes":64000,"gpu_total_bytes":64000,"unified_memory":true}`)
			case "/v1/tokenize":
				fmt.Fprint(w, `{"tokens":10000}`)
			case "/v1/kronk/models/ps":
				if loaded {
					json.NewEncoder(w).Encode([]map[string]any{{"id": model}})
				} else {
					fmt.Fprint(w, `[]`)
				}
			case "/v1/kronk/models/unload":
				loaded = false
				json.NewEncoder(w).Encode(map[string]string{"id": model, "status": "unloaded"})
			case "/v1/kronk/models/slots":
				var slots []map[string]any
				for _, id := range []int{7, 19} { // Do not assume slot IDs are 0/1.
					phase := "idle"
					if active[id] {
						phase = "generation"
					}
					slots = append(slots, map[string]any{"id": id, "phase": phase})
				}
				ndraft := 5
				if targetOnly {
					ndraft = 0
				}
				json.NewEncoder(w).Encode([]map[string]any{{"model_id": model, "mtp": !targetOnly, "ndraft": ndraft, "slots": slots}})
			default:
				metadata := map[string]string{"general.architecture": "qwen35moe", "qwen35moe.expert_count": "256"}
				if denseModel != "" {
					metadata = map[string]string{"general.architecture": "qwen35"}
				}
				json.NewEncoder(w).Encode(map[string]any{"has_projection": true, "model_config": map[string]any{"incremental-cache": !noCache, "nseq-max": 2}, "metadata": metadata})
			}
			return
		}
		var body struct {
			Model    string    `json:"model"`
			Messages []message `json:"messages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if denseModel != "" {
			alias := denseModel + "/QUICK-CHECK"
			if noCache {
				alias += "-NO-IMC"
			}
			if targetOnly {
				alias += "-NO-MTP"
			}
			if body.Model != alias+"-2S" {
				t.Errorf("wrong Dense alias: %s", body.Model)
			}
		}
		text, _ := json.Marshal(body.Messages)
		isA := bytes.Contains(text, []byte("River"))
		phase, slot := "warmup", 7
		mu.Lock()
		model, loaded = body.Model, true
		if len(body.Messages) == 2 {
			phase, slot = "fresh-session", 19
			if isA {
				aCalls++
				phase, slot = []string{"cold", "replay", "recovery"}[aCalls-1], 7
				if phase == "recovery" && defect != "wrong-recovery-slot" {
					slot = 19
				}
			}
		} else if len(body.Messages) == 4 {
			phase = "append"
			if !isA {
				phase, slot = "peer-append", 19
			}
		} else if len(body.Messages) == 6 {
			phase = "survivor"
			if !isA {
				phase, slot = "cancel", 19
			}
		}
		active[slot] = true
		mu.Unlock()
		trace := strings.Split(req.Header.Get("Traceparent"), "-")[1]
		emit := func(msg, status string, fields map[string]any) {
			if fields == nil {
				fields = make(map[string]any)
			}
			fields["msg"], fields["status"], fields["trace_id"], fields["slot"] = msg, status, trace, slot
			if _, ok := fields["time"]; !ok {
				fields["time"] = time.Now().UTC().Format(time.RFC3339Nano)
			}
			mu.Lock()
			defer mu.Unlock()
			if err := json.NewEncoder(serverLog).Encode(fields); err != nil {
				t.Error(err)
			}
		}
		for _, stage := range []int{1, 2, 3} {
			emit("request-lifecycle", "complete", map[string]any{"stage": stage, "elapsed": "1ms", "queue_wait": "1ms"})
		}
		emit("request-lifecycle", "started", map[string]any{"stage": 4})
		stable := map[string]int{"cold": 10050, "replay": 10050, "fresh-session": 10070, "append": 10300, "peer-append": 10320, "survivor": 10600, "cancel": 10620, "recovery": 10050}[phase]
		reused := map[string]int{"replay": 10050, "fresh-session": 10000, "append": 10050, "peer-append": 10070, "survivor": 10300, "cancel": 10320, "recovery": 10000}[phase]
		media := images && phase != "warmup" && phase != "cold" && phase != "replay" && phase != "recovery"
		kind, reason := "append", "complete-prefix-append"
		switch phase {
		case "cold":
			kind, reason = "rebuild", "no-match"
		case "replay":
			kind, reason = "exact", "complete-prefix-equal"
		case "fresh-session", "recovery":
			reason = "system-cache-prefix"
		case "append":
			if media {
				kind, reason = "media-append", "text-prefix-media-append"
			}
		case "peer-append":
			if media {
				kind, reason = "media-append", "media-prefix-append"
			}
		case "survivor", "cancel":
			if media {
				kind, reason = "anchor", "media-prefix-text-replay"
			}
		}
		if (defect == "system-only-media-append" || defect == "content-and-evidence") && phase == "append" {
			kind, reason, reused = "append", "system-cache-prefix", 10000
		}
		if phase == "warmup" && !targetOnly {
			emit("draft-model-mtp", "loaded", map[string]any{"backend": "qwen35-own-kv"})
		}
		if noCache {
			reused = 0
			if defect == "uncached-reuse" && phase == "append" {
				reused = 10000
			}
			if media {
				emit("start-slot-media", "tokenized", nil)
				emit("prefill-media", "encoding-image", map[string]any{"tokens": 256})
			}
		} else if phase != "warmup" {
			if phase == "cold" {
				emit("imc", "system-cache-published", map[string]any{"tokens": 10000})
			}
			if media {
				count := 1
				if phase == "peer-append" || phase == "cancel" {
					count = 2
				}
				emit("imc-media-cache", "plan-ready", map[string]any{"match_kind": kind, "match_reason": reason, "media_count": count, "reusable_logical_position": reused, "replay_text_tokens": 50, "anchor_physical_kv": reused})
				emit("start-slot", "imc-cache-ready", map[string]any{"media": true, "next_logical_position": stable})
				if phase != "survivor" && phase != "cancel" {
					emit("imc-media-cache", "encoded-media-chunk", map[string]any{"tokens": 256})
					emit("imc-media-cache", "decoded-media-chunk", map[string]any{"decoded_tokens": 256})
				}
			} else {
				emit("imc", "plan-ready", map[string]any{"match_kind": kind, "match_reason": reason, "stable_tokens": stable, "tail_tokens": 50, "reusable_tokens": reused, "extension_tokens": stable - reused})
			}
			for _, status := range []string{"imc-restore-done", "imc-snapshot-done", "imc-draft-restore-done", "imc-draft-snapshot-done"} {
				if strings.Contains(status, "draft") && targetOnly {
					continue
				}
				emit("start-slot", status, nil)
			}
		}
		appendPair := phase == "append" || phase == "peer-append"
		if appendPair {
			arrivals.Done()
			arrivals.Wait()
		}
		fields := map[string]any{"ttft": "1ms"}
		if defect == "serialized" && phase == "peer-append" {
			fields["time"] = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
		}
		emit("batch-engine", "prefill-done", fields)
		if appendPair {
			prefills.Done()
			prefills.Wait()
		}
		answer := "River Alice Apple"
		if !isA && phase != "warmup" {
			answer = "Mountain Bob Lemon"
		}
		if phase == "append" || phase == "survivor" {
			answer = "River Alice Orange"
		}
		if phase == "peer-append" {
			answer = "Mountain Bob Cherry"
		}
		if media {
			animal := "giraffe"
			if phase == "peer-append" || phase == "cancel" {
				animal = "deer"
			}
			if defect == "plurals" && animal == "giraffe" {
				animal = "giraffes"
			}
			answer += " " + animal
		}
		if defect == "content-"+phase || (defect == "content-and-evidence" && phase == "append") {
			stale := "Apple"
			if phase == "peer-append" {
				stale = "Lemon"
			} else if phase == "recovery" {
				stale = "Orange"
			}
			answer += " Not " + stale + "."
		}
		if defect == "foreign-session" && phase == "peer-append" {
			answer += " River"
		}
		chunk := func(text string) {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", text)
			w.(http.Flusher).Flush()
		}
		w.Header().Set("Content-Type", "text/event-stream")
		chunk(answer)
		if phase == "survivor" {
			select {
			case <-recoveryStarted:
				if defect != "no-survivor-progress" {
					chunk(" Additional checks preserve this conversation.")
				}
			case <-req.Context().Done():
			}
		}
		if phase == "recovery" {
			close(recoveryStarted)
		}
		if phase == "cancel" {
			<-req.Context().Done()
		}
		drafts, accepted, coverage, resume := 10, 5, 0.5, "restored-draft-kv"
		if noCache || phase == "cold" || phase == "warmup" {
			resume = "fresh-prefill"
		}
		if targetOnly {
			drafts, accepted, coverage, resume = 0, 0, 0, ""
			if defect == "target-only-drafts" && phase == "cancel" {
				drafts = 1
			}
		}
		emit("batch-engine", "slot-finished", map[string]any{"elapsed": "2ms", "imc_actual_reused_tokens": reused, "total_prompt": stable + 50, "output_tokens": 20, "draft_tokens": drafts, "draft_accepted_tokens": accepted, "draft_resume_source": resume})
		emit("finish-slot", "seq-cleared", nil)
		if !targetOnly {
			emit("speculative", "draft-kv-cleared", nil)
		}
		terminal := "complete"
		if phase == "cancel" && defect != "missing-cancel-ack" {
			terminal = "cancel"
		}
		emit("request-lifecycle", terminal, map[string]any{"stage": 4, "elapsed": "3ms"})
		mu.Lock()
		active[slot] = false
		mu.Unlock()
		if phase != "cancel" {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":20,\"draft_tokens\":%d,\"draft_accepted_tokens\":%d,\"draft_coverage\":%g,\"prompt_tokens_details\":{\"cached_tokens\":%d}}}\n\ndata: [DONE]\n\n", stable+50, drafts, accepted, coverage, reused)
		}
	}))
	defer server.Close()
	out := filepath.Join(root, "output")
	err = run(options{host: server.URL, out: out, serverLog: logPath, timeout: 2 * time.Second, repetitions: 1, cells: []string{selector}})
	if (err == nil) != passing {
		t.Fatalf("selector=%s defect=%s: %v", selector, defect, err)
	}
	paths, _ := filepath.Glob(filepath.Join(out, "*", "summary.json"))
	if len(paths) != 1 {
		t.Fatal("expected one combined report", paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var summary matrixSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	wantFinding := map[string]string{
		"serialized":               "generation overlap on distinct slots",
		"wrong-recovery-slot":      "must start on cancelled slot",
		"no-survivor-progress":     "continued content after peer cancellation",
		"system-only-media-append": "required media-append/text-prefix-media-append",
		"missing-cancel-ack":       "server terminal status must be cancel",
		"foreign-session":          "[ISOLATION]",
		"uncached-reuse":           "IMC-off execution reported cache planning",
		"target-only-drafts":       "target-only execution reported speculative activity",
	}[defect]
	executed := 0
	for _, cell := range summary.Cells {
		if cell.Status == "NOT_RUN" {
			continue
		}
		executed++
		if wantFinding != "" && !strings.Contains(strings.Join(cell.Findings, "\n"), wantFinding) {
			t.Fatalf("missing expected finding %q: %s", wantFinding, data)
		}
		if (cell.Status == "PASS") != passing || (passing && (len(cell.Repetitions) != 1 || len(cell.Repetitions[0].Results) != 8 || cell.Measurements["workload"]["elapsed_seconds"].Median <= 0)) {
			t.Fatalf("unexpected two-slot report: %s", data)
		}
		if passing && cell.Measurements["workload"]["reference_seconds"].Median <= 0 {
			t.Fatal("calibrated two-slot cell must publish Normalized", string(data))
		}
		if denseModel != "" && cell.ReferenceTokens != denseReferenceTokens(images, noCache, targetOnly, true) {
			t.Fatal("Dense cell borrowed MoE calibration", string(data))
		}
		if strings.HasPrefix(defect, "content-") {
			if len(cell.Repetitions) != 1 || len(cell.Repetitions[0].Results) != 8 || cell.Measurements != nil {
				t.Fatalf("content finding must finish all probes without publishing a passing aggregate: %s", data)
			}
			findings := cell.Repetitions[0].FindingsByCategory
			if len(findings["CONTENT"]) != 1 || len(findings["ISOLATION"]) != 0 || (len(findings["INFRASTRUCTURE"]) > 0) != (defect == "content-and-evidence") {
				t.Fatalf("content and infrastructure findings must remain distinct: %+v", findings)
			}
			for _, phase := range cell.Repetitions[0].Results {
				if phase["phase"] == "cancel" || phase["phase"] == "recovery" && defect != "content-recovery" {
					if phase["status"] != "PASS" {
						t.Fatalf("content finding interfered with cancellation/recovery: %+v", phase)
					}
				}
			}
			if defect != "content-and-evidence" {
				logs, err := os.ReadFile(filepath.Join(filepath.Dir(paths[0]), "tool.log"))
				if err != nil || !bytes.Contains(logs, []byte("IMC/MTP: PASS | Concurrent isolation: PASS | Cancel/recovery: PASS | Images: PASS")) || !bytes.Contains(logs, []byte("Findings: CONTENT=1 | ISOLATION=0 | INFRASTRUCTURE=0")) {
					t.Fatalf("content finding must not be reported as an infrastructure failure: %v\n%s", err, logs)
				}
			}
		}
	}
	if executed != 1 || len(summary.Cells) != len(matrixCells) {
		t.Fatalf("expected one selected cell and remaining NOT_RUN entries: %s", data)
	}
	if passing {
		var comparison bytes.Buffer
		dir := filepath.Dir(paths[0])
		if err := compareCommand([]string{"-baseline", dir, "-candidate", dir, "-out", t.TempDir()}, &comparison); err != nil {
			t.Fatal("two-slot artifacts are not comparable", err, comparison.String())
		}
	}
}

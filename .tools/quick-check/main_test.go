package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These servers exercise the command's contract, not Kronk inference. Each
// defect removes a distinct signal that an apparently successful chat can hide.
func TestQuickCheckCommand(t *testing.T) {
	for _, defect := range []string{"", "no-drafts", "no-cache", "wrong-cache-path", "missing-logs", "truncated-stream", "foreign-session", "two-slots", "cancel-internal-error", "cancel-without-ack", "cancel-without-cleanup", "missing-draft-restore", "actual-reuse-mismatch", "wrong-slot", "uncancelled-error", "dense-fixture", "missing-timing", "invalid-timing"} {
		t.Run(defect, func(t *testing.T) {
			root := t.TempDir()
			logPath := filepath.Join(root, "server.log")
			log, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			loaded, calls := false, 0
			cancelStarted, cancelFinished := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.URL.Path == "/v1/liveness", req.URL.Path == "/v1/kronk/libs":
					fmt.Fprintln(w, `{"build":"fixture"}`)
				case req.URL.Path == "/v1/kronk/models/ps":
					if loaded {
						json.NewEncoder(w).Encode([]map[string]any{{"id": modelID, "slots": 1}})
					} else {
						fmt.Fprintln(w, `[]`)
					}
				case req.URL.Path == "/v1/kronk/models/slots":
					select {
					case <-cancelStarted:
						<-cancelFinished
					default:
					}
					slots := []map[string]any{{"id": 0, "phase": "idle"}}
					if defect == "two-slots" {
						slots = append(slots, map[string]any{"id": 1, "phase": "idle"})
					}
					json.NewEncoder(w).Encode([]map[string]any{{"model_id": modelID, "mtp": true, "ndraft": 5, "slots": slots}})
				case req.URL.Path == "/v1/kronk/models/unload":
					loaded = false
					json.NewEncoder(w).Encode(map[string]any{"id": modelID, "status": "unloaded"})
				case req.URL.Path == "/v1/tokenize":
					var body struct {
						Input string `json:"input"`
					}
					json.NewDecoder(req.Body).Decode(&body)
					json.NewEncoder(w).Encode(map[string]int{"tokens": 200 + 160*strings.Count(body.Input, "Procedure ")})
				case req.URL.Path == "/v1/chat/completions":
					loaded = true
					var body struct {
						Messages []message `json:"messages"`
					}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					phases := []string{"load", "cold", "replay", "append", "fresh-session", "cancel", "recovery"}
					if calls >= len(phases) {
						t.Error("unexpected extra chat")
						return
					}
					phase := phases[calls]
					calls++
					if phase == "append" && (len(body.Messages) != 4 || !strings.Contains(body.Messages[2].Content, "Maren")) {
						t.Error("continuation did not preserve actual assistant response")
					}
					trace := strings.Split(req.Header.Get("Traceparent"), "-")[1]
					kind, reason, cached := "append", "complete-prefix-append", 10193
					stable, tail, boundary := 10500, 7, 0
					switch phase {
					case "load", "cold":
						kind, reason, cached = "rebuild", "no-complete-prefix", 0
						stable, boundary = 10193, 10120
					case "replay":
						kind, reason = "exact", "complete-prefix-equal"
						stable = 10193
					case "fresh-session", "recovery":
						reason, cached, stable = "system-cache-prefix", 10120, 10223
					case "cancel":
						cached = 10223
					}
					if defect == "wrong-cache-path" {
						reason = "system-cache-prefix"
					}
					emit := func(msg, status string, fields map[string]any) {
						if defect == "missing-logs" || (defect == "missing-draft-restore" && status == "imc-draft-restore-done") || (defect == "cancel-without-cleanup" && phase == "cancel" && msg == "finish-slot") {
							return
						}
						if fields == nil {
							fields = make(map[string]any)
						}
						if msg == "request-lifecycle" && fields["stage"] == 2 && phase == "replay" {
							if defect == "missing-timing" {
								delete(fields, "elapsed")
							}
							if defect == "invalid-timing" {
								fields["elapsed"] = "-1ms"
							}
						}
						fields["trace_id"], fields["msg"], fields["status"], fields["time"] = trace, msg, status, time.Now().UTC().Format(time.RFC3339Nano)
						if err := json.NewEncoder(log).Encode(fields); err != nil {
							t.Error(err)
						}
					}
					emit("request-lifecycle", "complete", map[string]any{"stage": 1, "elapsed": "750µs"})
					emit("request-lifecycle", "complete", map[string]any{"stage": 2, "elapsed": "12.5ms"})
					emit("request-lifecycle", "complete", map[string]any{"stage": 3, "queue_wait": "0s"})
					emit("imc", "plan-ready", map[string]any{"match_kind": kind, "match_reason": reason, "reusable_tokens": cached, "stable_tokens": stable, "extension_tokens": stable - cached, "tail_tokens": tail, "system_boundary_tokens": boundary})
					if phase == "load" {
						emit("draft-model-mtp", "loaded", map[string]any{"backend": "qwen35-own-kv"})
					}
					if phase == "cold" {
						emit("imc", "system-cache-published", map[string]any{"tokens": 10120})
						if defect == "uncancelled-error" {
							emit("handled error during request", "", map[string]any{"level": "ERROR", "err": map[string]any{"error": map[string]any{"code": "canceled", "message": "context canceled"}}})
						}
					}
					slot := 0
					if defect == "wrong-slot" && phase == "recovery" {
						slot = 1
					}
					emit("request-lifecycle", "started", map[string]any{"stage": 4, "slot": slot})
					emit("batch-engine", "prefill-done", map[string]any{"ttft": "123ms"})
					if cached > 0 {
						emit("start-slot", "imc-restore-done", nil)
						emit("start-slot", "imc-draft-restore-done", nil)
					}
					if phase != "replay" {
						emit("start-slot", "imc-snapshot-done", nil)
						emit("start-slot", "imc-draft-snapshot-done", nil)
					}
					finish := func(terminal string, output, drafts int) {
						emit("speculative", "draft-kv-cleared", nil)
						emit("finish-slot", "seq-cleared", nil)
						resume := "restored-draft-kv"
						if cached == 0 {
							resume = "fresh-prefill"
						}
						actual := cached
						if defect == "actual-reuse-mismatch" {
							actual = 1
						}
						emit("batch-engine", "slot-finished", map[string]any{"imc_actual_reused_tokens": actual, "output_tokens": output, "draft_tokens": drafts, "draft_resume_source": resume, "elapsed": "2.345s"})
						emit("request-lifecycle", terminal, map[string]any{"stage": 4, "slot": slot, "elapsed": "2.5s"})
					}
					if defect == "no-cache" {
						cached = 0
					}
					text := "ORCHID-47 belongs to Maren, destination Northport. Queue checks and recovery require care."
					if phase == "append" {
						text = "ORCHID-47 belongs to Maren, destination Eastmere. Check the queue before recovery."
					}
					if phase == "replay" {
						text = "For orchid-47, MAREN should review the Northport queue. Wording may vary on replay."
					}
					if phase == "fresh-session" {
						text = "COBALT-83 belongs to Tobin, destination Southbank. Review the pending work."
						if defect == "foreign-session" {
							text += " Maren."
						}
					}
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", text)
					if phase == "cancel" {
						close(cancelStarted)
						w.(http.Flusher).Flush()
						<-req.Context().Done()
						code, terminal := "canceled", "cancel"
						if defect == "cancel-internal-error" {
							code = "internal"
						}
						if defect == "cancel-without-ack" {
							terminal = "complete"
						}
						emit("handled error during request", "", map[string]any{"level": "ERROR", "err": map[string]any{"error": map[string]any{"code": code, "message": "chat-streaming-http: response already committed: context canceled"}}})
						finish(terminal, 2, 3)
						close(cancelFinished)
						return
					}
					if defect == "truncated-stream" && phase == "cold" {
						return
					}
					drafts := 40
					if defect == "no-drafts" {
						drafts = 0
					}
					finish("complete", 90, drafts)
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":90,\"draft_tokens\":%d,\"draft_coverage\":0.5,\"prompt_tokens_details\":{\"cached_tokens\":%d}}}\n\ndata: [DONE]\n\n", stable+tail, drafts, cached)
				case strings.HasPrefix(req.URL.Path, "/v1/kronk/models/"):
					experts := "256"
					if defect == "dense-fixture" {
						experts = "0"
					}
					fmt.Fprintf(w, `{"metadata":{"general.architecture":"qwen35moe","qwen35moe.expert_count":%q},"model_config":{"incremental-cache":true}}`, experts)
				default:
					t.Errorf("unexpected endpoint %s", req.URL.Path)
					http.NotFound(w, req)
				}
			}))
			defer server.Close()
			out := filepath.Join(root, "output")
			err = run(options{host: server.URL, out: out, serverLog: logPath, timeout: time.Second})
			if (err != nil) != (defect != "") {
				t.Fatalf("defect=%q error=%v", defect, err)
			}
			paths, _ := filepath.Glob(filepath.Join(out, "*", "summary.json"))
			if len(paths) != 1 {
				t.Fatalf("expected one report: %v", paths)
			}
			data, err := os.ReadFile(paths[0])
			if err != nil {
				t.Fatal(err)
			}
			var summary struct {
				Status       string `json:"status"`
				SystemTokens int    `json:"system_tokens"`
				Results      []struct {
					Status   string   `json:"status"`
					Findings []string `json:"findings"`
				} `json:"results"`
			}
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			if (summary.Status == "PASS") != (defect == "") {
				t.Fatalf("incorrect report: %s", data)
			}
			logData, err := os.ReadFile(filepath.Join(filepath.Dir(paths[0]), "tool.log"))
			if err != nil {
				t.Fatal(err)
			}
			text := string(logData)
			if !strings.HasPrefix(text, "Quick-check: "+summary.Status+"\n") || !strings.Contains(text, "--- Phase progress ---\n") {
				t.Fatalf("missing log verdict/progress: %s", text)
			}
			if defect == "" {
				for _, required := range []string{"IMC/MTP: PASS", "Serial isolation: PASS", "Cancel/recovery: PASS", "first request", "warmed workload", "phase=recovery elapsed="} {
					if !strings.Contains(text, required) {
						t.Fatalf("log omitted %q: %s", required, text)
					}
				}
				header, _, _ := strings.Cut(text, "\n\n--- Phase progress ---")
				if len(strings.Split(header, "\n")) != 3 {
					t.Fatalf("passing summary must be three lines: %s", header)
				}
			}
			if defect == "no-drafts" && (!strings.Contains(text, "IMC/MTP: FAIL") || !strings.Contains(text, "Serial isolation: UNVERIFIED") || !strings.Contains(text, "Cancel/recovery: UNVERIFIED")) {
				t.Fatalf("partial run overstated log coverage: %s", text)
			}
			if defect == "cancel-without-cleanup" && !strings.Contains(text, "Cancel/recovery: FAIL") {
				t.Fatalf("log omitted cancellation failure: %s", text)
			}
			if defect == "no-drafts" && !strings.Contains(text, "make test-load-mtp-embedded") {
				t.Fatal("missing drafting investigation direction")
			}
			if defect == "cancel-without-cleanup" && !strings.Contains(text, "Isolation/recovery -> make test-load-hybrid") {
				t.Fatal("missing recovery investigation direction")
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if _, ok := fields["server_metadata"]; ok {
				t.Fatal("full server metadata belongs in diagnostic events")
			}
			var phases []map[string]json.RawMessage
			if err := json.Unmarshal(fields["results"], &phases); err != nil {
				t.Fatal(err)
			}
			for _, phase := range phases {
				for _, key := range []string{"content", "execution", "trace_id", "started_at", "usage"} {
					if _, ok := phase[key]; ok {
						t.Fatalf("diagnostic %s leaked into phase summary", key)
					}
				}
			}
			events, err := os.Open(filepath.Join(filepath.Dir(paths[0]), "events.ndjson"))
			if err != nil {
				t.Fatal(err)
			}
			defer events.Close()
			var diagnostic report
			reports := 0
			scanner := bufio.NewScanner(events)
			scanner.Buffer(make([]byte, 65536), 4*1024*1024)
			for scanner.Scan() {
				var event struct {
					Source string `json:"source"`
					Report report `json:"report"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					t.Fatal(err)
				}
				if event.Source == "report" {
					diagnostic = event.Report
					reports++
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			if reports != 1 || diagnostic.Status != summary.Status || diagnostic.Metadata["model"] == nil {
				t.Fatalf("missing diagnostic report: %+v", diagnostic)
			}
			if defect == "" {
				if diagnostic.FirstRequest == nil || diagnostic.FirstRequest.Elapsed <= 0 || diagnostic.FirstRequest.FirstContent <= 0 || diagnostic.FirstRequest.Content == "" {
					t.Fatal("first request timing/answer lost")
				}
				var first map[string]json.RawMessage
				if err := json.Unmarshal(fields["first_request"], &first); err != nil || string(first["status"]) != `"PASS"` {
					t.Fatalf("missing first request metrics: %s (%v)", data, err)
				}
				for _, phase := range append(phases, first) {
					var timings map[string]float64
					if err := json.Unmarshal(phase["server_timings"], &timings); err != nil {
						t.Fatal(err)
					}
					for key, expected := range map[string]float64{"admission_seconds": 0.00075, "prepare_seconds": 0.0125, "queue_wait_seconds": 0, "slot_prefill_ttft_seconds": 0.123, "generation_seconds": 2.345, "slot_execution_seconds": 2.5} {
						if actual, ok := timings[key]; !ok || math.Abs(actual-expected) > 1e-12 {
							t.Fatalf("timing %s: got %v (present %v), want %v", key, actual, ok, expected)
						}
					}
				}
				if len(summary.Results) != 6 || summary.SystemTokens != 10120 || !diagnostic.Results[4].Cancelled || diagnostic.Results[4].Execution.ExpectedErrors != 1 || diagnostic.Results[0].Content == "" || diagnostic.Results[0].TraceID == "" {
					t.Fatalf("incomplete passing evidence: %+v", diagnostic)
				}
				for _, phase := range summary.Results {
					if phase.Status != "PASS" || len(phase.Findings) != 0 {
						t.Fatalf("incorrect phase decision: %+v", phase)
					}
				}
				if string(phases[1]["cached_tokens"]) != "10193" || string(phases[1]["draft_tokens"]) != "40" {
					t.Fatalf("cache/MTP comparison metrics lost: %s", data)
				}
				if _, ok := phases[4]["client_output_tokens_per_second"]; ok {
					t.Fatal("cancelled stream must not report a generation rate")
				}
			}
			if (defect == "missing-draft-restore" || defect == "missing-timing" || defect == "invalid-timing") && (summary.Results[1].Status != "FAIL" || len(summary.Results[1].Findings) == 0) {
				t.Fatalf("missing phase-level failure: %+v", summary)
			}
			if loaded {
				t.Fatal("fixture alias left loaded")
			}
		})
	}
}

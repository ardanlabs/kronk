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
	"slices"
	"strings"
	"testing"
	"time"
)

// These servers exercise the command's contract, not Kronk inference. Each
// defect removes a distinct signal that an apparently successful chat can hide.
func TestQuickCheckCommand(t *testing.T) {
	for _, defect := range []string{"", "single-repetition", "no-drafts", "second-no-drafts", "no-cache", "wrong-cache-path", "missing-logs", "truncated-stream", "foreign-session", "two-slots", "cancel-internal-error", "cancel-without-ack", "cancel-without-cleanup", "missing-draft-restore", "actual-reuse-mismatch", "wrong-slot", "uncancelled-error", "dense-fixture", "missing-timing", "invalid-timing"} {
		t.Run(defect, func(t *testing.T) {
			passing := defect == "" || defect == "single-repetition"
			requested := 3
			if defect == "single-repetition" {
				requested = 1
			}
			root := t.TempDir()
			logPath := filepath.Join(root, "server.log")
			log, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			loaded, calls, unloads := false, 0, 0
			tokenizeCalls := 0
			cancelStarted, cancelFinished := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.URL.Path == "/v1/liveness":
					fmt.Fprintln(w, `{"build":"develop","vcs_revision":"0123456789abcdef0123456789abcdef01234567","vcs_time":"2026-10-19T14:23:45Z","vcs_modified":false,"host":"fixture-server","os":"linux","GOMAXPROCS":12}`)
				case req.URL.Path == "/v1/kronk/libs":
					fmt.Fprintln(w, `{"build":"fixture"}`)
				case req.URL.Path == "/v1/devices":
					fmt.Fprintln(w, `{"system_ram_bytes":68719476736,"gpu_total_bytes":25769803776,"unified_memory":false,"devices":[
{"index":0,"name":"CPU","description":"Fixture CPU","type":"cpu","backend":"CPU","total_bytes":68719476736,"free_bytes":34359738368},
{"index":1,"name":"CUDA0","description":"Fixture GPU","type":"gpu_cuda","backend":"CUDA","total_bytes":25769803776,"free_bytes":21474836480}]}`)
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
					unloads++
					json.NewEncoder(w).Encode(map[string]any{"id": modelID, "status": "unloaded"})
				case req.URL.Path == "/v1/tokenize":
					tokenizeCalls++
					var body struct {
						Input string `json:"input"`
					}
					json.NewDecoder(req.Body).Decode(&body)
					if strings.Count(body.Input, "Procedure ") != 72 {
						t.Error("system prompt must retain its fixed 72 records")
					}
					if !strings.HasPrefix(body.Input, fmt.Sprintf("Quick-check repetition %d.\n\n", tokenizeCalls)) {
						t.Error("repetitions must use distinct, fixed system-prefix markers")
					}
					// Deliberately outside the old sizing range: record, don't resize.
					json.NewEncoder(w).Encode(map[string]int{"tokens": 9876})
				case req.URL.Path == "/v1/chat/completions":
					var body struct {
						Messages []message `json:"messages"`
					}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					phases := []string{"cold", "replay", "append", "fresh-session", "cancel", "recovery"}
					if calls >= 1+3*len(phases) {
						t.Error("unexpected extra chat")
						return
					}
					phase, repetition := "warmup", 1
					if calls > 0 {
						phase, repetition = phases[(calls-1)%len(phases)], (calls-1)/len(phases)+1
						if !loaded {
							t.Error("model must stay resident across workload repetitions")
						}
						if !strings.HasPrefix(body.Messages[0].Content, fmt.Sprintf("Quick-check repetition %d.\n\n", repetition)) {
							t.Error("request must use its repetition's system prefix")
						}
					}
					if phase == "warmup" {
						if loaded {
							t.Error("warm-up request must start with the alias unloaded")
						}
					}
					if phase == "cold" {
						cancelStarted, cancelFinished = make(chan struct{}), make(chan struct{})
					}
					loaded = true
					calls++
					if phase == "append" && (len(body.Messages) != 4 || !strings.Contains(body.Messages[2].Content, "Maren")) {
						t.Error("continuation did not preserve actual assistant response")
					}
					trace := strings.Split(req.Header.Get("Traceparent"), "-")[1]
					kind, reason, cached := "append", "complete-prefix-append", 10193
					stable, tail, boundary := 10500, 7, 0
					switch phase {
					case "warmup", "cold":
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
					if phase == "warmup" {
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
					if defect == "no-drafts" || (defect == "second-no-drafts" && calls > 7) {
						drafts = 0
					}
					finish("complete", 90, drafts)
					// Asymmetric rates catch averaging, pooling phases, and keeping
					// only the last repetition instead of reporting the median.
					rate := []int{19, 7, 41}[repetition-1]
					if phase == "replay" {
						rate *= 2
					}
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":90,\"draft_tokens\":%d,\"draft_coverage\":0.5,\"tokens_per_second\":%d,\"prompt_tokens_details\":{\"cached_tokens\":%d}}}\n\ndata: [DONE]\n\n", stable+tail, drafts, rate, cached)
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
			terminalPath := filepath.Join(root, "terminal.log")
			terminal, err := os.Create(terminalPath)
			if err != nil {
				t.Fatal(err)
			}
			stdout, stderr := os.Stdout, os.Stderr
			os.Stdout, os.Stderr = terminal, terminal
			err = run(options{host: server.URL, out: out, serverLog: logPath, timeout: time.Second, repetitions: requested})
			os.Stdout, os.Stderr = stdout, stderr
			if closeErr := terminal.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if (err == nil) != passing {
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
				Status       string                            `json:"status"`
				CommitTime   string                            `json:"server_commit_time"`
				Requested    int                               `json:"repetitions_requested"`
				Executed     int                               `json:"repetitions_executed"`
				Measurements map[string]map[string]measurement `json:"measurements"`
				Repetitions  []struct {
					Repetition   int    `json:"repetition"`
					Status       string `json:"status"`
					SystemTokens int    `json:"system_tokens"`
					Results      []struct {
						Phase    string   `json:"phase"`
						Status   string   `json:"status"`
						Findings []string `json:"findings"`
					} `json:"results"`
				} `json:"repetitions"`
			}
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			if (summary.Status == "PASS") != passing {
				t.Fatalf("incorrect report: %s", data)
			}
			if summary.CommitTime != "2026-10-19T14:23:45Z" {
				t.Fatalf("server commit timestamp not preserved: %s", data)
			}
			executed := 1
			if defect == "" {
				executed = 3
			} else if defect == "second-no-drafts" {
				executed = 2
			}
			if summary.Requested != requested || summary.Executed != executed || len(summary.Repetitions) != executed || unloads != 1 {
				t.Fatalf("incorrect repetition/reset accounting: unloads=%d summary=%s", unloads, data)
			}
			for i, repetition := range summary.Repetitions {
				if repetition.Repetition != i+1 {
					t.Fatalf("incorrect repetition order: %s", data)
				}
			}
			if defect == "second-no-drafts" && (calls != 8 || summary.Repetitions[0].Status != "PASS" || summary.Repetitions[1].Status != "FAIL" || !strings.Contains(string(data), "1 remaining repetitions unrun")) {
				t.Fatalf("failure after a passing repetition was hidden or retried: calls=%d summary=%s", calls, data)
			}
			if !passing && summary.Measurements != nil {
				t.Fatalf("failed/partial run must not publish performance aggregates: %s", data)
			}
			previous := -1
			for _, key := range []string{"machine", "server_version", "server_build", "installed_library", "model", "repetitions_requested", "repetitions"} {
				position := strings.Index(string(data), "\n  \""+key+"\":")
				if position <= previous {
					t.Fatalf("summary must put environment before requests; misplaced %s: %s", key, data)
				}
				previous = position
			}
			logData, err := os.ReadFile(filepath.Join(filepath.Dir(paths[0]), "tool.log"))
			if err != nil {
				t.Fatal(err)
			}
			text := string(logData)
			if !strings.HasPrefix(text, "Quick-check: "+summary.Status+"\nTested commit: 0123456 from 2026-10-19T14:23:45Z\n") || !strings.Contains(text, "--- Phase progress ---\n") {
				t.Fatalf("missing log verdict/progress: %s", text)
			}
			header, _, _ := strings.Cut(text, "\n\n--- Phase progress ---")
			terminalData, err := os.ReadFile(terminalPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(terminalData) != header+"\n" {
				t.Fatalf("terminal must show only the log header, got: %s", terminalData)
			}
			if passing {
				for _, required := range []string{"IMC/MTP: PASS", "Serial isolation: PASS", "Cancel/recovery: PASS", "warm-up request", "warmed workload", "phase=recovery elapsed="} {
					if !strings.Contains(text, required) {
						t.Fatalf("log omitted %q: %s", required, text)
					}
				}
				if len(strings.Split(header, "\n")) != 5 {
					t.Fatalf("passing summary must be five lines: %s", header)
				}
				sdText := "(SD n/a)"
				if requested > 1 {
					m := summary.Measurements["workload"]["elapsed_seconds"]
					if m.SampleStdDev == nil {
						t.Fatal("missing workload sample standard deviation")
					}
					sdText = fmt.Sprintf("(SD %.3fs)", *m.SampleStdDev)
				}
				if !strings.Contains(header, sdText) {
					t.Fatalf("terminal/log summary omitted standard deviation %q: %s", sdText, header)
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
			var machine map[string]any
			if err := json.Unmarshal(fields["machine"], &machine); err != nil {
				t.Fatal(err)
			}
			if len(machine) != 5 || machine["host"] != "fixture-server" || machine["os"] != "linux" || machine["system_ram_bytes"] != float64(68719476736) || machine["gpu_total_bytes"] != float64(25769803776) || machine["unified_memory"] != false {
				t.Fatalf("server machine identity not preserved: %+v", machine)
			}
			var repetitions []map[string]json.RawMessage
			if err := json.Unmarshal(fields["repetitions"], &repetitions); err != nil {
				t.Fatal(err)
			}
			var phases []map[string]json.RawMessage
			if err := json.Unmarshal(repetitions[0]["results"], &phases); err != nil {
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
					Source     string `json:"source"`
					Report     report `json:"report"`
					Repetition int    `json:"repetition"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					t.Fatal(err)
				}
				if event.Source == "report" {
					diagnostic = event.Report
					reports++
				} else if event.Repetition < 1 || event.Repetition > executed {
					t.Fatalf("event missing repetition identity: %s", scanner.Text())
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			if reports != 1 || diagnostic.Status != summary.Status || diagnostic.Metadata["model"] == nil {
				t.Fatalf("missing diagnostic report: %+v", diagnostic)
			}
			if len(diagnostic.Repetitions) != executed {
				t.Fatalf("repetition diagnostics lost: %+v", diagnostic)
			}
			if passing {
				if tokenizeCalls != requested || calls != 1+6*requested {
					t.Fatalf("expected %d complete sequences, got %d tokenizations, %d chats", requested, tokenizeCalls, calls)
				}
				for _, phase := range []string{"cold", "replay"} {
					want := measurement{Samples: 3, Median: 19, Min: 7, Max: 41}
					// For rates 19, 7, 41, sample variance is 892/3 (n-1
					// denominator), not population variance or spread about median.
					want.SampleStdDev = new(math.Sqrt(892.0 / 3))
					if phase == "replay" {
						want.Median, want.Min, want.Max = 38, 14, 82
						want.SampleStdDev = new(2 * *want.SampleStdDev)
					}
					if requested == 1 {
						want.Samples, want.Min, want.Max, want.SampleStdDev = 1, want.Median, want.Median, nil
					}
					got := summary.Measurements[phase]["server_output_tokens_per_second"]
					if got.Samples != want.Samples || got.Median != want.Median || got.Min != want.Min || got.Max != want.Max {
						t.Fatalf("%s rate statistics: got %+v, want %+v", phase, got, want)
					}
					if (got.SampleStdDev == nil) != (want.SampleStdDev == nil) || (want.SampleStdDev != nil && math.Abs(*got.SampleStdDev-*want.SampleStdDev) > 1e-12) {
						t.Fatalf("incorrect %s sample standard deviation: %+v", phase, got)
					}
				}
				if _, ok := summary.Measurements["cancel"]["server_output_tokens_per_second"]; ok {
					t.Fatal("cancelled requests cannot contribute a final usage rate")
				}
				diagnostic = diagnostic.Repetitions[0]
				if diagnostic.WarmupRequest == nil || diagnostic.WarmupRequest.Elapsed <= 0 || diagnostic.WarmupRequest.FirstContent <= 0 || diagnostic.WarmupRequest.Content == "" {
					t.Fatal("warm-up request timing/answer lost")
				}
				var warmup map[string]json.RawMessage
				if err := json.Unmarshal(fields["warmup_request"], &warmup); err != nil || string(warmup["status"]) != `"PASS"` {
					t.Fatalf("missing warm-up request metrics: %s (%v)", data, err)
				}
				for _, phase := range append(phases, warmup) {
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
				if len(summary.Repetitions[0].Results) != 6 || summary.Repetitions[0].SystemTokens != 9876 || !diagnostic.Results[4].Cancelled || diagnostic.Results[4].Execution.ExpectedErrors != 1 || diagnostic.Results[0].Content == "" || diagnostic.Results[0].TraceID == "" {
					t.Fatalf("incomplete passing evidence: %+v", diagnostic)
				}
				for index, phase := range summary.Repetitions[0].Results {
					if expected := []string{"cold", "replay", "append", "fresh-session", "cancel", "recovery"}[index]; phase.Phase != expected {
						t.Fatalf("request %d: got %s, want %s", index+1, phase.Phase, expected)
					}
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
			if (defect == "missing-draft-restore" || defect == "missing-timing" || defect == "invalid-timing") && (summary.Repetitions[0].Results[1].Status != "FAIL" || len(summary.Repetitions[0].Results[1].Findings) == 0) {
				t.Fatalf("missing phase-level failure: %+v", summary)
			}
			if loaded {
				t.Fatal("fixture alias left loaded")
			}
		})
	}
}

func TestRunRetention(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "output")
	protected := []string{"notes", "saved_local_runs", "19990101T000000Z-1"}
	for _, name := range protected {
		if err := os.MkdirAll(filepath.Join(out, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(root, filepath.Join(out, "19990101T000000Z-2")); err != nil {
		t.Fatal(err)
	}
	protected = append(protected, "19990101T000000Z-2")
	logPath := filepath.Join(root, "server.log")
	if err := os.WriteFile(logPath, nil, 0644); err != nil {
		t.Fatal(err)
	}
	const a = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const b = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	var paths []string
	for index, test := range []struct {
		revision string
		modified *bool
		broken   bool
		keep     []int
	}{
		{a, nil, false, []int{0}},
		{a, new(true), false, []int{1}}, // Replace the only candidate; ignore legacy modified state.
		{b, new(false), false, []int{1, 2}},
		{b, new(true), false, []int{1, 3}},
		{b, nil, false, []int{1, 4}}, // Repeated replacement preserves the baseline.
		{a, nil, false, []int{4, 5}}, // Returning to an older commit still promotes the last candidate.
		{a, new(true), false, []int{4, 6}},
		{"", nil, false, []int{6, 7}},        // Unknown hashes cannot establish equality.
		{b, new(true), true, []int{6, 7, 8}}, // Failed writes must not prune either result.
	} {
		before, _ := filepath.Glob(filepath.Join(out, "20*"))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			switch req.URL.Path {
			case "/v1/liveness":
				if test.broken {
					entries, _ := filepath.Glob(filepath.Join(out, "20*"))
					for _, path := range entries {
						if !slices.Contains(before, path) {
							if err := os.Mkdir(filepath.Join(path, "summary.json"), 0755); err != nil {
								t.Error(err)
							}
						}
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"build": "develop", "vcs_revision": test.revision, "vcs_modified": test.modified})
			case "/v1/kronk/libs", "/v1/devices":
				fmt.Fprintln(w, `{}`)
			case "/v1/kronk/models/ps":
				fmt.Fprintln(w, `[]`)
			default:
				// A failed inference is still a finalized candidate worth retaining.
				http.Error(w, "inference unavailable", http.StatusServiceUnavailable)
			}
		}))
		err := run(options{host: server.URL, out: out, serverLog: logPath, timeout: time.Second, repetitions: 1})
		server.Close()
		if err == nil {
			t.Fatal("expected inference/write failure")
		}
		after, _ := filepath.Glob(filepath.Join(out, "20*"))
		var current string
		for _, path := range after {
			if !slices.Contains(before, path) {
				current = path
			}
		}
		if current == "" {
			t.Fatal("missing new artifacts")
		}
		paths = append(paths, current)
		var expected []string
		for _, kept := range test.keep {
			expected = append(expected, paths[kept])
		}
		slices.Sort(expected)
		if !slices.Equal(after, expected) {
			t.Fatalf("run %d retained %v, want %v", index, after, expected)
		}
		if !test.broken {
			data, err := os.ReadFile(filepath.Join(current, "summary.json"))
			if err != nil {
				t.Fatal(err)
			}
			var summary struct {
				Revision string `json:"server_revision"`
				Version  string `json:"server_version"`
			}
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			version := "unknown"
			if test.revision != "" {
				version = test.revision
			}
			if summary.Revision != test.revision || summary.Version != version {
				t.Fatalf("wrong server provenance: %+v", summary)
			}
			display := "unknown"
			if test.revision != "" {
				display = test.revision[:7]
			}
			text, _ := os.ReadFile(filepath.Join(current, "tool.log"))
			if !strings.HasPrefix(string(text), "Quick-check: FAIL\nTested commit: "+display+" from unknown\n") {
				t.Fatalf("server provenance missing from log: %s", text)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			var started string
			if err := json.Unmarshal(fields["test_started_at"], &started); err != nil {
				t.Fatal(err)
			}
			if _, err := time.Parse(time.RFC3339Nano, started); err != nil || !strings.Contains(string(text), "\nTest started: "+started+"\n") || fields["server_modified"] != nil || strings.Contains(string(text), "+develop") {
				t.Fatal("test timestamp missing or retired modified flag present", string(text))
			}
		}
	}
	for _, name := range protected {
		if _, err := os.Lstat(filepath.Join(out, name)); err != nil {
			t.Fatalf("retention removed protected entry %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(out, ".quick-check.lock"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := run(options{out: out, timeout: time.Second, repetitions: 1}); err == nil || !strings.Contains(err.Error(), "acquire output lock") {
		t.Fatalf("concurrent invocation must not start: %v", err)
	}
}

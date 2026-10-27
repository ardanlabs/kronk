package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// These servers exercise the command's contract, not Kronk inference. Each
// defect removes a distinct signal that an apparently successful chat can hide.
func TestQuickCheckCommand(t *testing.T) {
	git := releaseTestRepo(t)
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "--quiet", "-m", "release")
	git("-c", "tag.gpgsign=false", "tag", "v1.0.0")
	releaseRevision := git("rev-parse", "HEAD")
	for _, defect := range []string{"", "release", "single-repetition", "all-cells", "all-cells-failure", "no-imc-config", "no-imc-reuse", "images-no-imc-no-encode", "mtp-off", "images-mtp-off", "mtp-off-drafts", "mtp-off-cancel-drafts", "mtp-off-no-restore", "no-drafts", "second-no-drafts", "no-cache", "wrong-cache-path", "missing-logs", "truncated-stream", "foreign-session", "two-slots", "cancel-internal-error", "cancel-without-ack", "cancel-without-cleanup", "missing-draft-restore", "actual-reuse-mismatch", "wrong-slot", "uncancelled-error", "dense-fixture", "missing-timing", "invalid-timing", "images", "images-system", "images-system-no-encode", "images-system-no-decode", "images-system-wrong-reuse", "images-system-no-restore", "images-no-encode", "images-no-decode", "images-reencode", "images-no-restore", "images-leak", "images-recovery-reuse", "images-recovery-leak"} {
		t.Run(defect, func(t *testing.T) {
			grouped := strings.HasPrefix(defect, "all-cells")
			images := strings.HasPrefix(defect, "images")
			targetOnly := strings.Contains(defect, "mtp-off")
			noCache := strings.Contains(defect, "no-imc")
			systemReuse := strings.HasPrefix(defect, "images-system") || defect == "images-mtp-off"
			passing := defect == "" || defect == "release" || defect == "single-repetition" || defect == "all-cells" || defect == "images" || defect == "images-system" || defect == "images-recovery-reuse" || defect == "mtp-off" || defect == "images-mtp-off"
			revision, tag := "0123456789abcdef0123456789abcdef01234567", ""
			if defect == "release" {
				revision, tag = releaseRevision, "v1.0.0"
			}
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
			loadedModel := modelID
			tokenizeCalls := 0
			currentSystem := ""
			seenSystems := make(map[string]bool)
			cancelStarted, cancelFinished := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.URL.Path == "/v1/liveness":
					fmt.Fprintf(w, `{"build":"develop","vcs_revision":%q,"vcs_time":"2026-10-19T14:23:45Z","host":"fixture-server","os":"linux","GOMAXPROCS":12}`+"\n", revision)
				case req.URL.Path == "/v1/kronk/libs":
					fmt.Fprintln(w, `{"build":"fixture"}`)
				case req.URL.Path == "/v1/devices":
					fmt.Fprintln(w, `{"system_ram_bytes":68719476736,"gpu_total_bytes":25769803776,"unified_memory":false,"devices":[
{"index":0,"name":"CPU","description":"Fixture CPU","type":"cpu","backend":"CPU","total_bytes":68719476736,"free_bytes":34359738368},
{"index":1,"name":"CUDA0","description":"Fixture GPU","type":"gpu_cuda","backend":"CUDA","total_bytes":25769803776,"free_bytes":21474836480}]}`)
				case req.URL.Path == "/v1/kronk/models/ps":
					if loaded {
						json.NewEncoder(w).Encode([]map[string]any{{"id": loadedModel, "slots": 1}})
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
					ndraft := 5
					if loadedModel == targetOnlyModelID || loadedModel == noCacheTargetOnlyModelID {
						ndraft = 0
					}
					json.NewEncoder(w).Encode([]map[string]any{{"model_id": loadedModel, "mtp": ndraft > 0, "ndraft": ndraft, "slots": slots}})
				case req.URL.Path == "/v1/kronk/models/unload":
					loaded = false
					unloads++
					json.NewEncoder(w).Encode(map[string]any{"id": loadedModel, "status": "unloaded"})
				case req.URL.Path == "/v1/tokenize":
					tokenizeCalls++
					var body struct {
						Input string `json:"input"`
					}
					json.NewDecoder(req.Body).Decode(&body)
					if strings.Count(body.Input, "Procedure ") != 72 {
						t.Error("system prompt must retain its fixed 72 records")
					}
					if !strings.HasPrefix(body.Input, "Quick-check cell ") || !strings.Contains(body.Input, fmt.Sprintf("; repetition %d.\n\n", (tokenizeCalls-1)%requested+1)) || seenSystems[body.Input] {
						t.Error("cells and repetitions must use distinct, fixed system-prefix markers")
					}
					seenSystems[body.Input], currentSystem = true, body.Input
					// Record the tokenizer's count without resizing the fixed text.
					json.NewEncoder(w).Encode(map[string]int{"tokens": 9876})
				case req.URL.Path == "/v1/chat/completions":
					var body struct {
						Model    string    `json:"model"`
						Messages []message `json:"messages"`
					}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					phases := []string{"cold", "replay", "append", "fresh-session", "cancel", "recovery"}
					cellCalls, images, targetOnly, noCache := calls, images, targetOnly, noCache
					if grouped {
						groupCalls := calls % (1 + 12*requested)
						index := 2 * (calls / (1 + 12*requested))
						cellCalls = groupCalls
						if groupCalls >= 1+6*requested {
							index++
							cellCalls = groupCalls - 6*requested
						}
						cell := matrixCells[index]
						images, targetOnly, noCache = cell.images, cell.targetOnly, cell.noCache
					}
					wantModel := modelID
					if targetOnly {
						wantModel = targetOnlyModelID
					}
					if noCache {
						wantModel = noCacheModelID
						if targetOnly {
							wantModel = noCacheTargetOnlyModelID
						}
					}
					if body.Model != wantModel {
						t.Errorf("wrong model alias: got %q, want %q", body.Model, wantModel)
					}
					loadedModel = body.Model
					if cellCalls >= 1+3*len(phases) {
						t.Error("unexpected extra chat")
						return
					}
					phase, repetition := "warmup", 1
					if cellCalls > 0 {
						phase, repetition = phases[(cellCalls-1)%len(phases)], (cellCalls-1)/len(phases)+1
						if !loaded {
							t.Error("model must stay resident across workload repetitions")
						}
						if body.Messages[0].Content != currentSystem {
							t.Error("request must retain its cell/repetition's system prefix")
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
					if phase == "append" && (len(body.Messages) != 4 || !strings.Contains(body.Messages[2].Content.(string), "Maren")) {
						t.Error("continuation did not preserve actual assistant response")
					}
					if images && phase != "warmup" {
						parts, ok := body.Messages[1].Content.([]any)
						if !ok || len(parts) != 2 {
							t.Error("image request must include text and image parts")
							return
						}
						question := parts[0].(map[string]any)["text"].(string)
						if strings.Contains(question, "giraffe") || strings.Contains(question, "deer") {
							t.Error("question leaked the expected image subject")
						}
						url := parts[1].(map[string]any)["image_url"].(map[string]any)["url"].(string)
						data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/jpeg;base64,"))
						fixture := "giraffe.jpg"
						if phase == "fresh-session" || phase == "cancel" {
							fixture = "deer/image10.jpg"
						}
						want, readErr := os.ReadFile(filepath.Join(os.Getenv("GITHUB_WORKSPACE"), "examples/samples", fixture))
						if err != nil || readErr != nil || !bytes.Equal(data, want) {
							t.Error("wrong or invalid image bytes", err, readErr)
						}
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
					if images && phase != "warmup" {
						switch phase {
						case "cold", "fresh-session", "recovery":
							kind, reason, cached = "rebuild", "no-exact-media-plan", 0
							if phase == "recovery" {
								stable = 10193
								if defect == "images-recovery-reuse" {
									kind, reason, cached = "exact", "logical-plan-equal", 9000
								}
							}
						case "replay":
							kind, reason, cached = "exact", "logical-plan-equal", 9000
						case "append", "cancel":
							kind, reason, cached = "anchor", "media-prefix-text-replay", 9000
							if phase == "cancel" {
								cached = 9100
							}
						}
						if systemReuse {
							if phase == "fresh-session" || phase == "recovery" {
								kind, reason, cached = "append", "system-cache-prefix", 10120
								if defect == "images-system-wrong-reuse" {
									cached--
								}
							}
							if phase == "cancel" {
								cached = 10600
							}
						}
					}
					if defect == "wrong-cache-path" {
						reason = "system-cache-prefix"
					}
					if noCache {
						cached = 0
					}
					emit := func(msg, status string, fields map[string]any) {
						if noCache && (msg == "imc" || msg == "imc-media-cache" || strings.HasPrefix(status, "imc-")) {
							return
						}
						if targetOnly && (msg == "draft-model-mtp" || msg == "speculative" || strings.HasPrefix(status, "imc-draft-")) {
							return
						}
						if defect == "mtp-off-no-restore" && status == "imc-restore-done" {
							return
						}
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
					if noCache && images && phase != "warmup" {
						emit("start-slot-media", "tokenized", map[string]any{"cached_tokens": 0, "total_tokens": stable + tail})
						if defect != "images-no-imc-no-encode" || phase != "replay" {
							emit("prefill-media", "encoding-shared", map[string]any{"tokens": 256})
						}
					} else if images && phase != "warmup" {
						anchor := 0
						if cached > 0 && reason != "system-cache-prefix" {
							anchor = 10193
							if phase == "cancel" {
								anchor = 10223
							}
						}
						emit("imc-media-cache", "plan-ready", map[string]any{"match_kind": kind, "match_reason": reason, "media_count": 1, "replay_text_tokens": tail, "reusable_logical_position": cached, "anchor_physical_kv": anchor})
						if kind == "rebuild" || reason == "system-cache-prefix" || (defect == "images-reencode" && phase == "replay") {
							if defect != "images-no-encode" && !(defect == "images-system-no-encode" && reason == "system-cache-prefix") {
								emit("imc-media-cache", "encoded-media-chunk", map[string]any{"tokens": 256})
							}
							if (defect != "images-no-decode" || phase != "recovery") && !(defect == "images-system-no-decode" && reason == "system-cache-prefix") {
								emit("imc-media-cache", "decoded-media-chunk", map[string]any{"decoded_tokens": 256})
							}
							position := 9000
							if phase == "fresh-session" {
								position = 9100
							}
							if reason == "system-cache-prefix" {
								position = 10500
								if phase == "fresh-session" {
									position = 10600
								}
							}
							emit("start-slot", "imc-cache-ready", map[string]any{"media": true, "next_logical_position": position})
						}
					} else {
						emit("imc", "plan-ready", map[string]any{"match_kind": kind, "match_reason": reason, "reusable_tokens": cached, "stable_tokens": stable, "extension_tokens": stable - cached, "tail_tokens": tail, "system_boundary_tokens": boundary})
					}
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
						if defect != "images-no-restore" && !(defect == "images-system-no-restore" && reason == "system-cache-prefix") {
							emit("start-slot", "imc-draft-restore-done", nil)
						}
					}
					if phase != "replay" && !(images && kind == "exact") {
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
						if targetOnly {
							resume = ""
							drafts = 0
							if defect == "mtp-off-cancel-drafts" && terminal == "cancel" {
								drafts = 3
							}
						}
						actual := cached
						if defect == "no-imc-reuse" && terminal == "cancel" {
							actual = 10120
						}
						if defect == "actual-reuse-mismatch" {
							actual = 1
						}
						emit("batch-engine", "slot-finished", map[string]any{"total_prompt": stable + tail, "imc_actual_reused_tokens": actual, "output_tokens": output, "draft_tokens": drafts, "draft_resume_source": resume, "elapsed": "2.345s"})
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
						if defect == "foreign-session" || (defect == "all-cells-failure" && images) {
							text += " Maren."
						}
					}
					if images && phase != "warmup" {
						animal := "giraffe"
						if phase == "fresh-session" || phase == "cancel" {
							animal = "deer"
						}
						if defect == "images-leak" && phase == "fresh-session" {
							animal = "giraffe"
						}
						if defect == "images-recovery-leak" && phase == "recovery" {
							animal = "deer"
						}
						text += " The picture shows a " + animal + "."
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
					coverage := 0.5
					if targetOnly {
						drafts, coverage = 0, 0
						if defect == "mtp-off-drafts" {
							drafts, coverage = 40, 0.5
						}
					}
					finish("complete", 90, drafts)
					// Asymmetric rates catch averaging, pooling phases, and keeping
					// only the last repetition instead of reporting the median.
					rate := []int{19, 7, 41}[repetition-1]
					if phase == "replay" {
						rate *= 2
					}
					fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":90,\"draft_tokens\":%d,\"draft_coverage\":%g,\"tokens_per_second\":%d,\"prompt_tokens_details\":{\"cached_tokens\":%d}}}\n\ndata: [DONE]\n\n", stable+tail, drafts, coverage, rate, cached)
				case strings.HasPrefix(req.URL.Path, "/v1/kronk/models/"):
					experts := "256"
					if defect == "dense-fixture" {
						experts = "0"
					}
					imc := loadedModel != noCacheModelID && loadedModel != noCacheTargetOnlyModelID
					if defect == "no-imc-config" {
						imc = true
					}
					fmt.Fprintf(w, `{"has_projection":true,"metadata":{"general.architecture":"qwen35moe","qwen35moe.expert_count":%q},"model_config":{"incremental-cache":%t}}`, experts, imc)
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
			cellIndex := 0
			if images {
				cellIndex++
			}
			if targetOnly {
				cellIndex += 2
			}
			if noCache {
				cellIndex += 4
			}
			selection := []string{matrixCells[cellIndex].id}
			if grouped {
				selection = nil
				for _, cell := range matrixCells[:8] { // The serial fixture's eight explicit cells.
					selection = append(selection, cell.id)
				}
			}
			err = run(options{host: server.URL, out: out, serverLog: logPath, timeout: time.Second, repetitions: requested, cells: selection})
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
			var matrix struct {
				Status string            `json:"status"`
				Cells  []json.RawMessage `json:"cells"`
			}
			if err := json.Unmarshal(data, &matrix); err != nil {
				t.Fatal(err)
			}
			if defect == "all-cells-failure" {
				if matrix.Status != "FAIL" || unloads != 1 || loaded || calls != 1+6*requested+4 || !bytes.Contains(matrix.Cells[0], []byte(`"status": "PASS"`)) || !bytes.Contains(matrix.Cells[1], []byte(`"status": "FAIL"`)) {
					t.Fatalf("shared-cell failure did not stop and unload its group: %s", data)
				}
				for _, cell := range matrix.Cells[2:] {
					if !bytes.Contains(cell, []byte(`"status": "NOT_RUN"`)) {
						t.Fatal("continued after group failure", string(cell))
					}
				}
				return
			}
			if defect == "all-cells" {
				if matrix.Status != "PASS" || len(matrix.Cells) != 16 || tokenizeCalls != 8*requested || calls != 4+8*6*requested || unloads != 4 || loaded {
					t.Fatalf("combined run lost cells/repetitions or changed lifecycle: chats=%d tokenizations=%d unloads=%d summary=%s", calls, tokenizeCalls, unloads, data)
				}
				for i := 0; i < 8; i += 2 {
					var first, second runSummary
					json.Unmarshal(matrix.Cells[i], &first)
					json.Unmarshal(matrix.Cells[i+1], &second)
					if first.WarmupRequest["status"] != "PASS" || second.WarmupRequest != nil || second.SharedWarmupFrom != first.ID || first.Repetitions[0].SystemSHA256 == second.Repetitions[0].SystemSHA256 {
						t.Fatalf("group lost startup ownership or cell cache independence: %s", data)
					}
				}
				dir := filepath.Dir(paths[0])
				var comparisonText bytes.Buffer
				if err := compareCommand([]string{"-out", t.TempDir(), "-baseline", dir, "-candidate", dir, "-json"}, &comparisonText); err != nil {
					t.Fatal("combined run is not consumable by comparator", err)
				}
				var comparison matrixComparison
				if err := json.Unmarshal(comparisonText.Bytes(), &comparison); err != nil {
					t.Fatal(err)
				}
				if comparison.Status != "PASS" || len(comparison.Cells) != 16 {
					t.Fatal("combined comparison omitted a cell", comparisonText.String())
				}
				for _, cell := range comparison.Cells {
					if strings.Contains(cell.Cell, "/2s/") {
						if cell.Status != "NOT_RUN" {
							t.Fatal("unselected two-slot comparison must be NOT_RUN", cell.Cell)
						}
						continue
					}
					if cell.Status != "PASS" || cell.Baseline.ReferenceTokens != referenceTokens(strings.HasSuffix(cell.Cell, "img+"), strings.Contains(cell.Cell, "IMC-"), strings.Contains(cell.Cell, "MTP-"), false) || cell.Baseline.Executed != requested || len(cell.Metrics) == 0 {
						t.Fatal("cell evidence/normalization mixed", comparisonText.String())
					}
					if strings.Contains(cell.Cell, "MTP-") && strings.Contains(strings.Join(cell.Warnings, "\n"), "MTP backend") {
						t.Fatal("target-only comparison must not require an MTP backend", comparisonText.String())
					}
					if strings.Contains(cell.Cell, "IMC-") {
						input, reference, generated := 51367.0, 51405.0, 1085.0
						if strings.Contains(cell.Cell, "MTP-") {
							generated = 1117
						}
						if strings.HasSuffix(cell.Cell, "img+") {
							input, reference, generated = 51337, 53095, 958
							if strings.Contains(cell.Cell, "MTP-") {
								generated = 865
							}
						}
						want := reference*(5*0.123)/input + generated*(5*2.345)/(5*89)
						found := false
						for _, metric := range cell.Metrics {
							if metric.Metric == "workload.reference_seconds" {
								found = true
								if metric.Baseline == nil || math.Abs(metric.Baseline.Median-want) > 1e-12 {
									t.Fatalf("uncached normalization must count all completed prompts: %+v, want %v", metric.Baseline, want)
								}
							}
						}
						if !found {
							t.Fatal("uncached normalization omitted", cell.Cell)
						}
					}
				}
				return
			}
			if len(matrix.Cells) != 16 || (matrix.Status == "PASS") != passing {
				t.Fatalf("incorrect matrix coverage/status: %s", data)
			}
			for i, cell := range matrix.Cells {
				if i != cellIndex && !bytes.Contains(cell, []byte(`"status": "NOT_RUN"`)) {
					t.Fatalf("unselected cell ran: %s", cell)
				}
			}
			data = matrix.Cells[cellIndex]
			var summary struct {
				Status          string                            `json:"status"`
				CommitTime      string                            `json:"server_commit_time"`
				ReleaseTag      string                            `json:"release_tag"`
				Requested       int                               `json:"repetitions_requested"`
				Executed        int                               `json:"repetitions_executed"`
				Measurements    map[string]map[string]measurement `json:"measurements"`
				ReferenceTokens normalizationReference            `json:"normalization_reference"`
				Repetitions     []struct {
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
			if passing {
				executed = requested
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
				position := strings.Index(string(data), "\""+key+"\":")
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
			version, folder := revision[:7], "new-"+revision[:7]
			if tag != "" {
				version += " (release " + tag + ")"
				folder = "new-" + tag + "-" + revision[:7]
			}
			if filepath.Base(filepath.Dir(paths[0])) != folder || summary.ReleaseTag != tag {
				t.Fatal("release identity missing from folder or summary", paths[0], summary.ReleaseTag)
			}
			if !strings.HasPrefix(text, "Quick-check: "+summary.Status+"\nTested commit: "+version+" from 2026-10-19T14:23:45Z\n") || !strings.Contains(text, "--- Phase progress ---\n") {
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
				for _, required := range []string{"IMC/MTP: PASS", "Serial isolation: PASS", "Cancel/recovery: PASS", "Warm-up:", "Workload s [SD]", "Normalized s [SD]", "phase=recovery elapsed="} {
					if !strings.Contains(text, required) {
						t.Fatalf("log omitted %q: %s", required, text)
					}
				}
				if images {
					if !strings.Contains(header, "Images: PASS") || !strings.Contains(header, "img+") {
						t.Fatal("image cell not identified", header)
					}
				}
				sdText := "[n/a]"
				if requested > 1 {
					m := summary.Measurements["workload"]["elapsed_seconds"]
					if m.SampleStdDev == nil {
						t.Fatal("missing workload sample standard deviation")
					}
					sdText = fmt.Sprintf("[%.2f]", *m.SampleStdDev)
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
					Source     string       `json:"source"`
					Report     matrixReport `json:"report"`
					Cell       string       `json:"cell_id"`
					Repetition int          `json:"repetition"`
				}
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					t.Fatal(err)
				}
				if event.Source == "report" {
					diagnostic = event.Report.Cells[cellIndex]
					reports++
				} else if event.Cell != matrixCells[cellIndex].id || event.Repetition < 1 || event.Repetition > executed {
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
				if images {
					// Completed prompt counts: 10200, 10200, 10507, 10230,
					// 10200. Physical anchors are 10193, not logical 9000.
					input := 30951.0
					if defect == "images-recovery-reuse" {
						input -= 10193
					}
					if systemReuse {
						// The text-only System prefix is physical reuse even though
						// there is no saved media anchor in these two requests.
						input -= 2 * 10120
					}
					if summary.ReferenceTokens != (normalizationReference{InputTokens: 11674, GenerationTokens: 1001}) {
						t.Fatal("image cell did not record its fixed normalization reference", summary.ReferenceTokens)
					}
					want := 11674.0*(5*0.123)/input + 1001.0*(5*2.345)/(5*89)
					if got := summary.Measurements["workload"]["reference_seconds"].Median; math.Abs(got-want) > 1e-12 {
						t.Fatalf("image normalization used logical instead of physical reuse: got %v, want %v", got, want)
					}
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
				wantCached := "10193"
				if images {
					wantCached = "9000"
				}
				wantDrafts := "40"
				if targetOnly {
					wantDrafts = "0"
				}
				if string(phases[1]["cached_tokens"]) != wantCached || string(phases[1]["draft_tokens"]) != wantDrafts {
					t.Fatalf("cache/MTP comparison metrics lost: %s", data)
				}
				if _, ok := phases[4]["client_output_tokens_per_second"]; ok {
					t.Fatal("cancelled stream must not report a generation rate")
				}
			}
			if (defect == "missing-draft-restore" || defect == "missing-timing" || defect == "invalid-timing") && (summary.Repetitions[0].Results[1].Status != "FAIL" || len(summary.Repetitions[0].Results[1].Findings) == 0) {
				t.Fatalf("missing phase-level failure: %+v", summary)
			}
			if systemReuse && !passing {
				results := summary.Repetitions[0].Results
				if len(results) != 6 || results[0].Status != "PASS" || results[1].Status != "PASS" || results[2].Status != "PASS" || results[3].Status != "FAIL" {
					t.Fatalf("System-reuse defect must fail the fresh image, not an earlier phase: %+v", summary)
				}
			}
			if loaded {
				t.Fatal("fixture alias left loaded")
			}
		})
	}
}

func TestRunRetention(t *testing.T) {
	releaseTestRepo(t)
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
	var revisions []string
	for index, test := range []struct {
		revision string
		broken   bool
		keep     []int
	}{
		{a, false, []int{0}},
		{a, false, []int{1}}, // Replace the only candidate.
		{b, false, []int{1, 2}},
		{b, false, []int{1, 3}},
		{b, false, []int{1, 4}}, // Repeated replacement preserves the baseline.
		{a, false, []int{4, 5}}, // Returning to an older commit still promotes the last candidate.
		{a, false, []int{4, 6}},
		{"", false, []int{6, 7}},  // Unknown hashes cannot establish equality.
		{b, true, []int{6, 7, 8}}, // Failed writes must not prune either result.
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
				json.NewEncoder(w).Encode(map[string]any{"build": "develop", "vcs_revision": test.revision})
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
		for _, prefix := range []string{"old-", "new-"} {
			roles, _ := filepath.Glob(filepath.Join(out, prefix+"*"))
			after = append(after, roles...)
		}
		short := test.revision
		if short == "" {
			short = "unknown"
		} else {
			short = short[:7]
		}
		current := filepath.Join(out, "new-"+short)
		if test.broken {
			for _, path := range after {
				if strings.HasPrefix(filepath.Base(path), "20") && !slices.Contains(before, path) {
					current = path
				}
			}
		}
		revisions = append(revisions, short)
		var expected []string
		for _, kept := range test.keep {
			role := "old-"
			if kept == index || (test.broken && kept == index-1) {
				role = "new-"
			}
			path := filepath.Join(out, role+revisions[kept])
			if test.broken && kept == index {
				path = current
			}
			expected = append(expected, path)
		}
		slices.Sort(expected)
		slices.Sort(after)
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
			if _, err := time.Parse(time.RFC3339Nano, started); err != nil || !strings.Contains(string(text), "\nTest started: "+started+"\n") {
				t.Fatal("test timestamp missing or invalid", string(text))
			}
		}
	}
	for _, name := range protected {
		if _, err := os.Lstat(filepath.Join(out, name)); err != nil {
			t.Fatalf("retention removed protected entry %s: %v", name, err)
		}
	}
	t.Run("invalid-saved-timestamp", func(t *testing.T) {
		out := t.TempDir()
		r := comparisonFixture([]float64{1, 4, 10})
		r.TestStartedAt = ""
		path := filepath.Join(out, "new-aaaaaaa")
		if err := os.Rename(writeComparisonFixture(t, r), path); err != nil {
			t.Fatal(err)
		}
		summaryPath := filepath.Join(path, "summary.json")
		before, err := os.ReadFile(summaryPath)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			http.Error(w, "server unavailable", http.StatusServiceUnavailable)
		}))
		defer server.Close()
		err = run(options{host: server.URL, out: out, serverLog: logPath, timeout: time.Second, repetitions: 1})
		if err == nil || !strings.Contains(err.Error(), "missing or invalid test timestamp") {
			t.Fatal("retention accepted a run without its recorded timestamp", err)
		}
		after, err := os.ReadFile(summaryPath)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("invalid timestamp must stop retention without changing the saved run", err)
		}
	})
	if err := os.WriteFile(filepath.Join(out, ".quick-check.lock"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := run(options{out: out, timeout: time.Second, repetitions: 1}); err == nil || !strings.Contains(err.Error(), "acquire output lock") {
		t.Fatalf("concurrent invocation must not start: %v", err)
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

func TestReleaseRetention(t *testing.T) {
	git := releaseTestRepo(t)
	root := git("rev-parse", "--show-toplevel")
	commit := func(label, tag string) string {
		t.Helper()
		git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "--quiet", "-m", label)
		revision := git("rev-parse", "HEAD")
		if tag != "" {
			git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "tag.gpgsign=false", "tag", "-a", tag, "-m", tag)
		}
		return revision
	}
	a, b, c, d, e, f := commit("a", "v1.0.0"), commit("b", ""), commit("c", "v1.1.0"), commit("d", "notes"), commit("e", ""), commit("f", "v1.2.0")
	// Include both annotated and lightweight release tags.
	git("tag", "-d", "v1.1.0")
	git("-c", "tag.gpgsign=false", "tag", "v1.1.0", c)
	t.Run("shallow-checkout-without-tags", func(t *testing.T) {
		clone := filepath.Join(t.TempDir(), "clone")
		git("clone", "--quiet", "--depth=1", "--no-tags", "file://"+root, clone)
		t.Chdir(clone)
		if git("tag", "--list") != "" {
			t.Fatal("fixture unexpectedly has local tags")
		}
		if err := exec.Command("git", "rev-parse", "--verify", a+"^{commit}").Run(); err == nil {
			t.Fatal("fixture unexpectedly has the older release commit")
		}
		tags, err := releaseTags(context.Background())
		if err != nil || len(tags) != 3 || tags[a] != "v1.0.0" || tags[c] != "v1.1.0" || tags[f] != "v1.2.0" || tags[d] != "" {
			t.Fatal("remote annotated/lightweight recognition failed", tags, err)
		}
	})
	out := filepath.Join(root, "output")
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	tags, err := releaseTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{
		a: "v1.0.0-" + a[:7], b: b[:7], c: "v1.1.0-" + c[:7],
		d: d[:7], e: e[:7], f: "v1.2.0-" + f[:7],
	}
	for i, tc := range []struct {
		revision, release, old string
	}{
		{a, "", ""},
		{b, "v1.0.0", ""},
		{b, "v1.0.0", ""},
		{c, "v1.0.0", b},
		{c, "v1.0.0", b},
		{d, "v1.1.0", b},
		{e, "v1.1.0", d},
		{f, "v1.1.0", e},
		{f, "v1.1.0", e},
		{c, "v1.2.0", e}, // Returning to the protected hash replaces it, not a duplicate slot.
		{a, "v1.1.0", f}, // All three slots may hold releases.
		{b, "v1.0.0", c}, // An ordinary commit protects the release role, not every tagged run.
	} {
		r := comparisonFixture([]float64{2 + float64(i), 6 + float64(i), 14 + float64(i)})
		r.ServerRevision = tc.revision
		r.ReleaseTag = tags[tc.revision]
		r.TestStartedAt = time.Date(2026, 10, 22, 0, 0, i, 0, time.UTC).Format(time.RFC3339Nano)
		staged := filepath.Join(out, fmt.Sprintf("20261022T0000%02dZ-%d", i, i))
		if err := os.Rename(writeComparisonFixture(t, r), staged); err != nil {
			t.Fatal(err)
		}
		final, err := retainRuns(context.Background(), out, staged, tags)
		if err != nil || final != filepath.Join(out, "new-"+labels[tc.revision]) {
			t.Fatal(final, err)
		}
		entries, err := os.ReadDir(out)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		want := []string{"new-" + labels[tc.revision]}
		if tc.release != "" {
			want = append(want, "release-"+tc.release)
		}
		if tc.old != "" {
			want = append(want, "old-"+labels[tc.old])
		}
		slices.Sort(want)
		if !slices.Equal(names, want) {
			t.Fatalf("step %d: %v, want %v", i, names, want)
		}
		if tc.release != "" {
			release, err := readComparison(filepath.Join(out, "release-"+tc.release))
			if err != nil {
				t.Fatal(err)
			}
			wantRevision := map[string]string{"v1.0.0": a, "v1.1.0": c, "v1.2.0": f}[tc.release]
			if release.summary.ServerRevision != wantRevision {
				t.Fatal("wrong protected release", release.summary.ServerRevision)
			}
		}
		if i == 0 {
			var output strings.Builder
			if err := compareCommand([]string{"-against", "release", "-out", out}, &output); err == nil {
				t.Fatal("compared without an older release")
			}
		}
		if i == 3 || i == 4 || i == 7 || i == 8 {
			var output strings.Builder
			if err := compareCommand([]string{"-against", "release", "-out", out}, &output); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, "release-history.md"))
			if err != nil {
				t.Fatal(err)
			}
			history := string(data)
			count := 1
			if i >= 7 {
				count = 2
				if !strings.Contains(history, "v1.1.0 → v1.2.0") || !strings.Contains(history, f) {
					t.Fatal("second release comparison missing", history)
				}
			}
			if strings.Count(history, "\n## ") != count || !strings.Contains(history, "v1.0.0 → v1.1.0") || !strings.Contains(history, a) || !strings.Contains(history, c) || !strings.Contains(history, fmt.Sprintf("%.6f [SD", 6+float64(i))) || !strings.Contains(history, "| Warm-up request | 6.000000") {
				t.Fatal("release comparison/history update failed", history)
			}
			if !strings.Contains(history, "machine: `test-machine`; RAM: 24000 bytes; VRAM: 18000 bytes; unified memory: true") || strings.Contains(history, "test-os") || !strings.Contains(history, "library: `test-library`") {
				t.Fatal("incorrect history metadata", history)
			}
			if i == 3 && !strings.Contains(history, "+50.00%") {
				t.Fatal("incorrect timing delta", history)
			}
			// Choosing the commit baseline must not change release history.
			if err := compareCommand([]string{"-against", "commit", "-out", out}, &output); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(filepath.Join(root, "release-history.md"))
			if err != nil || string(after) != history {
				t.Fatal("commit comparison changed release history", err)
			}
			// Comparison reports are files, not managed run directories.
			for _, name := range []string{"comparison.json", "comparison.log"} {
				if err := os.Remove(filepath.Join(out, name)); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	path := filepath.Join(root, "release-history.md")
	history, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	if err := compareCommand([]string{"-against", "release", "-out", out}, &output); err != nil {
		t.Fatal(err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != string(history) {
		t.Fatal("ordinary-commit versus release changed history", err)
	}
	// Broken history is surfaced, never discarded or silently replaced.
	if err := os.WriteFile(path, []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := compareCommand([]string{"-against", "release", "-out", out, "-baseline", filepath.Join(out, "release-v1.0.0"), "-candidate", filepath.Join(out, "old-"+labels[c])}, &output); err == nil || !strings.Contains(err.Error(), "release history") {
		t.Fatal("accepted broken history", err)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != "broken" {
		t.Fatal("overwrote broken history", err)
	}
	// Failure to recognize remote releases preserves every saved output.
	git("remote", "set-url", "origin", filepath.Join(root, "missing-origin"))
	logPath := filepath.Join(root, "server.log")
	if err := os.WriteFile(logPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "server unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	if err := run(options{host: server.URL, out: out, serverLog: logPath, timeout: time.Second, repetitions: 1}); err == nil || !strings.Contains(err.Error(), "release recognition unavailable") {
		t.Fatal("failed lookup treated as an ordinary commit", err)
	}
	for _, name := range []string{"release-v1.0.0", "old-" + labels[c], "new-" + labels[b]} {
		if _, err := os.Stat(filepath.Join(out, name, "summary.json")); err != nil {
			t.Fatal("tag lookup error evicted existing data", err)
		}
	}
}

func TestQuickCheckInterrupt(t *testing.T) {
	if os.Getenv("QUICK_CHECK_INTERRUPT_CHILD") == "1" {
		if err := run(options{host: os.Getenv("QUICK_CHECK_TEST_HOST"), out: os.Getenv("QUICK_CHECK_TEST_OUT"), serverLog: os.Getenv("QUICK_CHECK_TEST_LOG"), timeout: time.Minute, repetitions: 3}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	for _, tc := range []struct {
		name, phase string
		signal      os.Signal
	}{
		{"setup-interrupt", "setup", os.Interrupt},
		{"stream-interrupt", "stream", os.Interrupt},
		{"stream-terminate", "stream", syscall.SIGTERM},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			out := filepath.Join(root, "output")
			if err := os.Mkdir(out, 0755); err != nil {
				t.Fatal(err)
			}
			names := []string{"new-aaaaaaa", "old-bbbbbbb", "release-v1.0.0"}
			for _, name := range names {
				if err := os.Rename(writeComparisonFixture(t, comparisonFixture([]float64{1, 4, 10})), filepath.Join(out, name)); err != nil {
					t.Fatal(err)
				}
			}
			logPath := filepath.Join(root, "server.log")
			if err := os.WriteFile(logPath, nil, 0644); err != nil {
				t.Fatal(err)
			}
			ready, cancelled := make(chan struct{}), make(chan struct{})
			var loaded, unloaded atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				switch req.URL.Path {
				case "/v1/liveness":
					if tc.phase == "setup" {
						close(ready)
						<-req.Context().Done()
						close(cancelled)
						return
					}
					fmt.Fprintln(w, `{}`)
				case "/v1/kronk/libs", "/v1/devices":
					fmt.Fprintln(w, `{}`)
				case "/v1/kronk/models/ps":
					if loaded.Load() {
						json.NewEncoder(w).Encode([]map[string]string{{"id": modelID}})
					} else {
						fmt.Fprintln(w, `[]`)
					}
				case "/v1/chat/completions":
					loaded.Store(true)
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"Hello"}}]}`)
					w.(http.Flusher).Flush()
					close(ready)
					<-req.Context().Done()
					close(cancelled)
				case "/v1/kronk/models/slots":
					json.NewEncoder(w).Encode([]map[string]any{{"model_id": modelID, "slots": []map[string]any{{"id": 0, "phase": "idle"}}}})
				case "/v1/kronk/models/unload":
					loaded.Store(false)
					unloaded.Store(true)
					json.NewEncoder(w).Encode(map[string]string{"id": modelID, "status": "unloaded"})
				default:
					http.NotFound(w, req)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestQuickCheckInterrupt$")
			cmd.Env = append(os.Environ(), "QUICK_CHECK_INTERRUPT_CHILD=1", "QUICK_CHECK_TEST_HOST="+server.URL, "QUICK_CHECK_TEST_OUT="+out, "QUICK_CHECK_TEST_LOG="+logPath)
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-ready:
			case <-ctx.Done():
				cmd.Wait()
				t.Fatal("child did not reach the interruptible request", output.String())
			}
			if err := cmd.Process.Signal(tc.signal); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Wait(); err == nil || ctx.Err() != nil || !strings.Contains(output.String(), "Quick-check interrupted; unfinished output removed; previous runs retained.") || strings.Contains(output.String(), "Quick-check: PASS") {
				t.Fatalf("interruption was not handled cleanly: %v\n%s", err, output.String())
			}
			select {
			case <-cancelled:
			case <-ctx.Done():
				t.Fatal("server request was not cancelled")
			}
			if tc.phase == "stream" && (!unloaded.Load() || loaded.Load()) {
				t.Fatal("interrupted inference did not unload its model", output.String())
			}
			entries, err := os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, entry := range entries {
				got = append(got, entry.Name())
			}
			slices.Sort(names)
			if !slices.Equal(got, names) {
				t.Fatal("interruption changed saved runs or left a lock/partial output", got)
			}
			for _, name := range names {
				if _, err := readComparison(filepath.Join(out, name)); err != nil {
					t.Fatal("interruption damaged a previous run", err)
				}
			}
		})
	}
}

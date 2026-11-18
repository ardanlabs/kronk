package main

import (
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

func TestLifecycleClientAndServerEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutation string
		cycles   string
		want     string
	}{
		{name: "three clean cycles", want: "PASS"},
		{name: "wrong admission code", mutation: "wrong-code", cycles: "1", want: "FAIL"},
		{name: "role-only is not generation", mutation: "role-only", cycles: "1", want: "FAIL"},
		{name: "queued request executed", mutation: "queued-executed", cycles: "1", want: "FAIL"},
		{name: "wrong admission capacity", mutation: "wrong-capacity", cycles: "1", want: "FAIL"},
		{name: "queued permit leaked", mutation: "permit-leak", cycles: "1", want: "FAIL"},
		{name: "missing cancellation evidence", mutation: "missing-cancel", cycles: "1", want: "INCOMPLETE"},
		{name: "sequence changed", mutation: "wrong-sequence", cycles: "1", want: "FAIL"},
		{name: "queue after execution", mutation: "late-queue", cycles: "1", want: "FAIL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			serverLog := filepath.Join(dir, "kronk.log")
			file, err := os.Create(serverLog)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			var mu sync.Mutex
			var requests int
			seen := make(map[string]bool)
			// A real listener exercises the command's configured HTTP endpoint,
			// streaming bodies, and transport cancellation without an SDK/model.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				trace := strings.Split(r.Header.Get("Traceparent"), "-")[1]
				mu.Lock()
				index := requests % 6
				requests++
				if seen[trace] {
					t.Errorf("trace %s was reused", trace)
				}
				seen[trace] = true
				mu.Unlock()
				emit := func(stage int, status string) {
					if tc.mutation == "missing-cancel" && index == 0 && stage == 4 && status == "cancel" {
						return
					}
					event := map[string]any{"msg": "request-lifecycle", "trace_id": trace, "stage": stage, "status": status}
					if stage == 1 {
						event["capacity"], event["admitted"] = 2, 2
						if tc.mutation == "wrong-capacity" {
							event["capacity"] = 3
						}
					}
					if stage == 4 {
						event["slot"], event["seq"] = 0, 0
						if tc.mutation == "wrong-sequence" && status == "cancel" {
							event["seq"] = 1
						}
					}
					mu.Lock()
					defer mu.Unlock()
					if err := json.NewEncoder(file).Encode(event); err != nil {
						t.Errorf("write server evidence: %v", err)
					}
				}
				emit(1, "started")
				if index == 2 || index == 4 || index == 3 && tc.mutation == "permit-leak" {
					select {
					case <-time.After(100 * time.Millisecond):
					case <-r.Context().Done():
						return
					}
					emit(1, "timeout")
					code := "resource_exhausted"
					if tc.mutation == "wrong-code" {
						code = "internal_error"
					}
					w.WriteHeader(http.StatusTooManyRequests)
					if err := json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "No admission permit became available before the deadline"}}); err != nil {
						t.Errorf("write admission error: %v", err)
					}
					return
				}
				emit(1, "complete")
				emit(2, "started")
				emit(2, "complete")
				emit(3, "started")
				queued := index == 1 || index == 3
				if queued {
					emit(3, "queued")
				} else {
					emit(3, "complete")
					emit(4, "started")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
				w.(http.Flusher).Flush()
				if index == 0 && tc.mutation == "role-only" {
					fmt.Fprint(w, "data: [DONE]\n\n")
					return
				}
				if !queued {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\n")
					w.(http.Flusher).Flush()
				}
				if index == 5 {
					emit(4, "complete")
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				<-r.Context().Done()
				if queued {
					if tc.mutation == "queued-executed" {
						emit(4, "started")
					}
					emit(3, "cancel")
					return
				}
				emit(4, "cancel")
				if tc.mutation == "late-queue" {
					emit(3, "queued")
				}
			}))
			defer server.Close()
			parent := filepath.Join(dir, "output")
			t.Setenv("KRONK_WEB_API_HOST", server.URL)
			t.Setenv("KRONK_LIFECYCLE_OUT", parent)
			t.Setenv("KRONK_SERVER_LOG", serverLog)
			t.Setenv("KRONK_LIFECYCLE_CYCLES", tc.cycles)
			t.Setenv("KRONK_LIFECYCLE_MODEL", "fixture")
			t.Setenv("KRONK_TOKEN", "")
			err = run()
			if (err == nil) != (tc.want == "PASS") {
				t.Fatalf("error = %v, want status %s", err, tc.want)
			}
			entries, err := os.ReadDir(parent)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("artifact directories = %d, want 1", len(entries))
			}
			data, err := os.ReadFile(filepath.Join(parent, entries[0].Name(), "summary.json"))
			if err != nil {
				t.Fatal(err)
			}
			var summary lifecycleSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatal(err)
			}
			if summary.Status != tc.want {
				t.Fatalf("status = %s, want %s; error = %s", summary.Status, tc.want, summary.Error)
			}
			if tc.want == "PASS" && (summary.Cycles != 3 || summary.CompletedCycles != 3 || len(summary.Requests) != 18) {
				t.Fatalf("three-cycle run did not complete all requests: %+v", summary)
			}
		})
	}
}

func TestLifecycleEvidenceUsesOnlyAppendedMatchingEvents(t *testing.T) {
	dir := t.TempDir()
	serverLog := filepath.Join(dir, "kronk.log")
	if err := os.WriteFile(serverLog, []byte(`{"msg":"old","trace_id":"11111111111111111111111111111111"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts, err := newLifecycleArtifacts(filepath.Join(dir, "output"), serverLog)
	if err != nil {
		t.Fatal(err)
	}
	defer artifacts.close()
	artifacts.traces["11111111111111111111111111111111"] = "holder"
	artifacts.traces["44444444444444444444444444444444"] = "recovery"
	file, err := os.OpenFile(serverLog, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"msg":"request-lifecycle","trace_id":"44444444444444444444444444444444"}` + "\n" +
		`{"msg":"request-lifecycle","trace_id":"ffffffffffffffffffffffffffffffff"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	evidence := artifacts.collectServerEvidence()
	if evidence.MatchedEvents != 1 {
		t.Fatalf("matched events = %d, want 1", evidence.MatchedEvents)
	}
	evidence = artifacts.collectServerEvidence()
	if evidence.MatchedEvents != 1 {
		t.Fatalf("recollected events = %d, want 1", evidence.MatchedEvents)
	}
	data, err := os.ReadFile(filepath.Join(artifacts.dir, "events.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "\n") != 1 || strings.Contains(string(data), `"msg":"old"`) {
		t.Fatalf("evidence contains duplicate or old events: %s", data)
	}
}

func TestLifecycleArtifactsPreservePreviousRuns(t *testing.T) {
	parent := t.TempDir()
	previous := filepath.Join(parent, "previous.txt")
	if err := os.WriteFile(previous, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	var dirs []string
	for _, status := range []string{"PASS", "FAIL"} {
		artifacts, err := newLifecycleArtifacts(parent, filepath.Join(parent, "absent.log"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(artifacts.close)
		if err := artifacts.writeSummary(lifecycleSummary{Status: status}); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, artifacts.dir)
	}
	if dirs[0] == dirs[1] {
		t.Fatal("two runs used the same artifact directory")
	}
	for i, status := range []string{"PASS", "FAIL"} {
		if filepath.Dir(dirs[i]) != parent {
			t.Fatalf("artifact directory %q is not under %q", dirs[i], parent)
		}
		data, err := os.ReadFile(filepath.Join(dirs[i], "summary.json"))
		if err != nil {
			t.Fatal(err)
		}
		var summary lifecycleSummary
		if err := json.Unmarshal(data, &summary); err != nil {
			t.Fatal(err)
		}
		if summary.Status != status {
			t.Fatalf("run %d status = %q, want %q", i, summary.Status, status)
		}
		for _, name := range []string{"events.ndjson", "tool.log"} {
			if _, err := os.Stat(filepath.Join(dirs[i], name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	data, err := os.ReadFile(previous)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "keep" {
		t.Fatalf("previous artifact = %q, want keep", data)
	}
}

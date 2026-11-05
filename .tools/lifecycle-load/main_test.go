package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLifecycleUsesAdmissionBehaviorNotErrorWording(t *testing.T) {
	for _, code := range []string{"resource_exhausted", "internal_error"} {
		t.Run(code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				trace := strings.Split(r.Header.Get("Traceparent"), "-")[1]
				if trace == blockedTrace {
					time.Sleep(100 * time.Millisecond)
					w.WriteHeader(http.StatusTooManyRequests)
					json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "No admission permit became available before the deadline"}})
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				if trace != queuedTrace {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\n")
					w.(http.Flusher).Flush()
				}
				if trace == recoveryTrace {
					time.Sleep(20 * time.Millisecond)
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			dir := t.TempDir()
			artifacts, err := newLifecycleArtifacts(filepath.Join(dir, "out"), filepath.Join(dir, "absent.log"))
			if err != nil {
				t.Fatal(err)
			}
			defer artifacts.close()
			summary := lifecycleSummary{Requests: make(map[string]requestSummary)}
			err = runLifecycle(config{endpoint: server.URL, model: "fixture"}, artifacts, &summary)
			if (err != nil) != (code != "resource_exhausted") {
				t.Fatalf("error=%v for admission code %s", err, code)
			}
			if err == nil && (len(summary.Requests) != 4 || summary.Requests["recovery"].Outcome != "completed-after-release") {
				t.Fatalf("lifecycle did not complete recovery: %+v", summary.Requests)
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
}

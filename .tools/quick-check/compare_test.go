package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareAutomaticSelection(t *testing.T) {
	parent := t.TempDir()
	add := func(name string, values []float64) string {
		t.Helper()
		dir := filepath.Join(parent, name)
		if err := os.Rename(writeComparisonFixture(t, comparisonFixture(values)), dir); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	// Creation order and file mtimes must not reverse the timestamped run order.
	newer := add("20261021T162309Z-2", []float64{2, 6, 14})
	older := add("20261021T160436Z-9", []float64{1, 4, 10})
	archive := filepath.Join(parent, "saved_local_runs")
	if err := os.Mkdir(archive, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(writeComparisonFixture(t, comparisonFixture([]float64{3})), filepath.Join(archive, "20261021T170000Z-3")); err != nil {
		t.Fatal(err)
	}
	incomplete := filepath.Join(parent, "20261021T180000Z-4")
	if err := os.Mkdir(incomplete, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(incomplete, "summary.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := compareCommand([]string{"-out", parent, "-json"}, &out); err != nil {
		t.Fatal(err)
	}
	var got comparison
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Baseline.Directory != older || got.Candidate.Directory != newer || got.Status != "PASS" {
		t.Fatalf("incorrect automatic ordering: %+v", got)
	}
	// Explicit selection can deliberately reverse the order and bypass discovery.
	out.Reset()
	if err := compareCommand([]string{"-out", parent, "-baseline", newer, "-candidate", older, "-json"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Baseline.Directory != newer || got.Candidate.Directory != older {
		t.Fatal("explicit order was overridden")
	}
	for _, args := range [][]string{{"-out", parent, "-baseline", older}, {"-out", parent, "-candidate", newer}} {
		if err := compareCommand(args, &out); err == nil {
			t.Fatal("accepted only one explicit path")
		}
	}
	third := add("20261021T190000Z-5", []float64{3})
	if err := compareCommand([]string{"-out", parent}, &out); err == nil {
		t.Fatal("silently selected among three runs")
	}
	if err := os.RemoveAll(third); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(newer, filepath.Join(parent, "20261021T160436Z-10")); err != nil {
		t.Fatal(err)
	}
	if err := compareCommand([]string{"-out", parent}, &out); err == nil {
		t.Fatal("guessed order for tied timestamps")
	}
	if err := os.RemoveAll(older); err != nil {
		t.Fatal(err)
	}
	if err := compareCommand([]string{"-out", parent}, &out); err == nil {
		t.Fatal("compared a single run")
	}
}

func TestCompareCommand(t *testing.T) {
	a := comparisonFixture([]float64{1, 4, 10})
	b := comparisonFixture([]float64{2, 6, 14})
	b.ServerRevision = strings.Repeat("b", 40)
	b.Metadata["/v1/liveness"].(map[string]any)["vcs_revision"] = b.ServerRevision
	base, candidate := writeComparisonFixture(t, a), writeComparisonFixture(t, b)
	output := t.TempDir()
	var out bytes.Buffer
	args := []string{"-out", output, "-baseline", base, "-candidate", candidate, "-json"}
	if err := compareCommand(args, &out); err != nil {
		t.Fatal(err)
	}
	first := out.String()
	if !strings.Contains(first, `"saved_run_checks": {`) || strings.Contains(first, `"recorded_correctness"`) {
		t.Fatal("JSON verdict label does not match saved-run checks", first)
	}
	var got comparison
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "PASS" || len(got.Warnings) != 0 {
		t.Fatalf("unexpected verdict/warnings: %+v", got)
	}
	jsonModeLog, err := os.ReadFile(filepath.Join(output, "comparison.log"))
	if err != nil || !bytes.HasPrefix(jsonModeLog, []byte("Comparison status: PASS\n")) {
		t.Fatal("JSON mode must also save a plain-text summary", err)
	}
	rows := map[string]metricComparison{}
	for _, row := range got.Metrics {
		rows[row.Metric] = row
	}
	r := rows["workload.elapsed_seconds"]
	if r.Baseline.Samples != 3 || r.Baseline.Median != 4 || r.Baseline.Min != 1 || r.Baseline.Max != 10 || math.Abs(*r.Baseline.SampleStdDev-math.Sqrt(21)) > 1e-12 ||
		r.Candidate.Median != 6 || *r.Delta != 2 || *r.Percent != 50 {
		t.Fatalf("incorrect asymmetric statistics/direction: %+v", r)
	}
	zero := rows["cold.cached_tokens"]
	if zero.Percent != nil || zero.Delta == nil || *zero.Delta != 0 {
		t.Fatalf("zero denominator: %+v", zero)
	}
	startup := rows["warmup.startup.native_model_load_seconds"]
	if startup.Baseline.Median != 3.5 || startup.Baseline.Samples != 1 || startup.Baseline.SampleStdDev != nil {
		t.Fatalf("startup reconstruction: %+v", startup)
	}
	if rows["warmup.elapsed_seconds"].Baseline.Samples != 1 {
		t.Fatal("warm-up pooled with repetitions")
	}
	out.Reset()
	if err := compareCommand(args, &out); err != nil || out.String() != first {
		t.Fatal("comparison is not deterministic", err)
	}
	out.Reset()
	if err := compareCommand([]string{"-out", output, "-baseline", candidate, "-candidate", base, "-json"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, row := range got.Metrics {
		if row.Metric == "workload.elapsed_seconds" && (*row.Delta != -2 || math.Abs(*row.Percent+100.0/3) > 1e-12) {
			t.Fatal("reversed baseline calculation", row)
		}
	}
	out.Reset()
	if err := compareCommand([]string{"-out", output, "-baseline", base, "-candidate", candidate}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Saved-run checks: baseline PASS / candidate PASS\nBaseline: "+base+"\nBaseline commit:") || !strings.Contains(out.String(), "Warmed workload") || strings.Contains(out.String(), "warmup.elapsed_seconds") || strings.Contains(out.String(), "-json") || strings.Contains(out.String(), "Note:") {
		t.Fatal(out.String())
	}
	saved, err := os.ReadFile(filepath.Join(output, "comparison.json"))
	if err != nil || string(saved) != first {
		t.Fatal("saved full report differs from JSON output", err)
	}
	log, err := os.ReadFile(filepath.Join(output, "comparison.log"))
	if err != nil || string(log) != out.String() || !bytes.Equal(log, jsonModeLog) {
		t.Fatal("saved log must match terminal output in either mode", err)
	}
	for _, dir := range []string{base, candidate} {
		if _, err := os.Stat(filepath.Join(dir, "comparison.json")); !os.IsNotExist(err) {
			t.Fatal("comparison must not be saved inside a run", dir, err)
		}
	}
	got.Warnings = []string{"harness revision: changed", "model configuration.new-field: changed", "installed library: changed"}
	got.Matches = []string{"effective native parameters", "request bodies/order"}
	out.Reset()
	if err := got.writeText(&out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "harness revision") || strings.Contains(out.String(), "model configuration") || !strings.Contains(out.String(), "installed library: changed") {
		t.Fatal(out.String())
	}
	got.Matches = nil
	out.Reset()
	if err := got.writeText(&out); err != nil || !strings.Contains(out.String(), "model configuration") {
		t.Fatal("configuration warning hidden without equivalent effective evidence", err, out.String())
	}
}

func TestCompareRunStatuses(t *testing.T) {
	for _, baselineStatus := range []string{"PASS", "FAIL"} {
		for _, candidateStatus := range []string{"PASS", "FAIL"} {
			t.Run(baselineStatus+"-"+candidateStatus, func(t *testing.T) {
				a, b := comparisonFixture([]float64{1, 4, 10}), comparisonFixture([]float64{2, 6, 14})
				a.Status, b.Status = baselineStatus, candidateStatus
				baseline, candidate := writeComparisonFixture(t, a), writeComparisonFixture(t, b)
				output := t.TempDir()
				var out bytes.Buffer
				err := compareCommand([]string{"-out", output, "-baseline", baseline, "-candidate", candidate}, &out)
				fails := baselineStatus == "FAIL" || candidateStatus == "FAIL"
				status := "PASS"
				if fails {
					status = "FAIL"
				}
				if (err != nil) != fails || !strings.HasPrefix(out.String(), fmt.Sprintf("Comparison status: %s\nSaved-run checks: baseline %s / candidate %s\n", status, baselineStatus, candidateStatus)) {
					t.Fatal(err, out.String())
				}
				log, err := os.ReadFile(filepath.Join(output, "comparison.log"))
				if err != nil || string(log) != out.String() {
					t.Fatal("passing and failed comparisons must save their terminal summary", err)
				}
				data, err := os.ReadFile(filepath.Join(output, "comparison.json"))
				if err != nil {
					t.Fatal(err)
				}
				var got comparison
				if err := json.Unmarshal(data, &got); err != nil {
					t.Fatal(err)
				}
				if got.SavedRunChecks["baseline"] != baselineStatus || got.SavedRunChecks["candidate"] != candidateStatus || (got.Status == "FAIL") != fails {
					t.Fatal(string(data))
				}
			})
		}
	}
}

func TestCompareWarningsAndFailures(t *testing.T) {
	base := writeComparisonFixture(t, comparisonFixture([]float64{1, 4, 10}))
	for _, tc := range []struct {
		name    string
		mutate  func(*report)
		fails   bool
		warning string
	}{
		{"library", func(r *report) { r.Metadata["/v1/kronk/libs"].(map[string]any)["current"] = "different" }, false, "installed library"},
		{"machine", func(r *report) { r.Metadata["/v1/liveness"].(map[string]any)["host"] = "different" }, false, "machine.host"},
		{"unknown-memory", func(r *report) { r.Metadata["/v1/devices"].(map[string]any)["system_ram_bytes"] = 0 }, false, "missing metadata"},
		{"config", func(r *report) { r.Metadata["model"].(map[string]any)["model_config"].(map[string]any)["nseq-max"] = 2 }, false, "model configuration.nseq-max"},
		{"unknown", func(r *report) { r.ServerRevision = "" }, false, "unknown server commit"},
		{"prompt", func(r *report) { r.Repetitions[1].SystemTokens++ }, false, "system token count/hash"},
		{"counts", func(r *report) { r.Repetitions[0].Results[0].Usage.CompletionTokens++ }, false, "counts differ"},
		{"single", func(r *report) { r.Repetitions = r.Repetitions[:1]; r.RepetitionsRequested = 1 }, false, "repetition count"},
		{"failed", func(r *report) { r.Status = "FAIL"; r.Findings = []string{"known failure"} }, true, ""},
		{"partial", func(r *report) { r.Repetitions = r.Repetitions[:2] }, true, ""},
		{"phase", func(r *report) { r.Repetitions[1].Results = r.Repetitions[1].Results[:5] }, true, ""},
		{"missing-timing", func(r *report) { delete(r.Repetitions[1].Results[0].ServerTimings, "prepare_seconds") }, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := comparisonFixture([]float64{2, 6, 14})
			tc.mutate(&r)
			candidate := writeComparisonFixture(t, r)
			var out bytes.Buffer
			err := compareCommand([]string{"-out", t.TempDir(), "-baseline", base, "-candidate", candidate, "-json"}, &out)
			if (err != nil) != tc.fails {
				t.Fatalf("unexpected error %v: %s", err, out.String())
			}
			var got comparison
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err, out.String())
			}
			if tc.fails && (got.Status != "FAIL" || len(got.Metrics) != 0) {
				t.Fatalf("failure received passing timing aggregates: %+v", got)
			}
			if tc.warning != "" && !strings.Contains(strings.Join(got.Warnings, "\n"), tc.warning) {
				t.Fatalf("missing warning %q: %v", tc.warning, got.Warnings)
			}
			out.Reset()
			if err := got.writeText(&out); err != nil {
				t.Fatal(err)
			}
			banner := "WARNING: MACHINE METADATA DIFFERS OR IS MISSING!"
			if tc.name == "machine" || tc.name == "unknown-memory" {
				if strings.Count(out.String(), banner) != 2 || strings.Index(out.String(), banner) > strings.Index(out.String(), "Metric (seconds)") || !strings.HasSuffix(strings.TrimSpace(out.String()), banner) {
					t.Fatal("machine warning must appear before metrics and at the end", out.String())
				}
			} else if strings.Contains(out.String(), banner) {
				t.Fatal("machine warning shown for unrelated changes", out.String())
			}
			if tc.name == "single" {
				for _, row := range got.Metrics {
					if row.Candidate != nil && row.Candidate.SampleStdDev != nil {
						t.Fatal("single sample SD must be null")
					}
				}
			}
		})
	}
}

func TestCompareRejectsBrokenArtifacts(t *testing.T) {
	base := writeComparisonFixture(t, comparisonFixture([]float64{1, 4, 10}))
	for _, name := range []string{"missing-log", "truncated-events", "duplicate-report", "summary-disagrees", "log-disagrees", "startup-disagrees", "request-order"} {
		t.Run(name, func(t *testing.T) {
			r := comparisonFixture([]float64{2, 6, 14})
			if name == "startup-disagrees" {
				r.WarmupRequest.StartupTimings = map[string]float64{"native_model_load_seconds": 99}
			}
			dir := writeComparisonFixture(t, r)
			switch name {
			case "missing-log":
				if err := os.Remove(filepath.Join(dir, "tool.log")); err != nil {
					t.Fatal(err)
				}
			case "truncated-events":
				if err := os.WriteFile(filepath.Join(dir, "events.ndjson"), []byte("{\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "duplicate-report":
				f, err := os.OpenFile(filepath.Join(dir, "events.ndjson"), os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.NewEncoder(f).Encode(map[string]any{"source": "report", "report": r}); err != nil {
					t.Fatal(err)
				}
				f.Close()
			case "summary-disagrees":
				s := r.summary()
				s.Measurements["cold"]["elapsed_seconds"] = measurement{Samples: 3, Median: 999}
				data, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "summary.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			case "log-disagrees":
				if err := os.WriteFile(filepath.Join(dir, "tool.log"), []byte("Quick-check: FAIL\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "request-order":
				path := filepath.Join(dir, "events.ndjson")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.Replace(data, []byte(`"phase":"warmup"`), []byte(`"phase":"replay"`), 1)
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			if err := compareCommand([]string{"-out", t.TempDir(), "-baseline", base, "-candidate", dir}, &out); err == nil {
				t.Fatal("accepted broken artifacts")
			}
		})
	}
}

func comparisonFixture(values []float64) report {
	r := report{Status: "PASS", Workload: "operations-v2", Model: modelID, ServerRevision: strings.Repeat("a", 40), TestStartedAt: "2026-10-21T16:00:00Z", HarnessRevision: "fixed-harness", MTPBackend: "qwen35-own-kv", RepetitionsRequested: len(values), evidenceValidated: true}
	r.Metadata = map[string]any{
		"/v1/liveness":   map[string]any{"host": "test-machine", "os": "test-os", "vcs_revision": r.ServerRevision},
		"/v1/devices":    map[string]any{"system_ram_bytes": 24000, "gpu_total_bytes": 18000, "unified_memory": true},
		"/v1/kronk/libs": map[string]any{"current": "test-library"},
		"model":          map[string]any{"size": 1234, "quantization": "Q2", "file_type": "GGUF", "template": "test-template", "metadata": map[string]any{"architecture": "moe"}, "model_config": map[string]any{"nseq-max": 1}},
	}
	makeResult := func(phase string, v float64) result {
		res := result{Phase: phase, TraceID: phase, Elapsed: v, FirstContent: 0.25, Usage: usage{PromptTokens: 10140, CompletionTokens: 80, ServerTPS: 40}, ServerTimings: map[string]float64{}}
		for _, key := range []string{"admission_seconds", "prepare_seconds", "queue_wait_seconds", "slot_execution_seconds", "slot_prefill_ttft_seconds", "generation_seconds"} {
			res.ServerTimings[key] = 0.125
		}
		if phase == "cancel" {
			res.Cancelled = true
			res.Usage = usage{}
		}
		return res
	}
	warm := makeResult("warmup", 6)
	r.WarmupRequest = &warm
	for i, v := range values {
		rep := report{Status: "PASS", SystemTokens: 10140, SystemSHA256: fmt.Sprintf("hash-%d", i), WorkloadSeconds: v, CancelReleaseSeconds: 0, evidenceValidated: true}
		for _, phase := range []string{"cold", "replay", "append", "fresh-session", "cancel", "recovery"} {
			rep.Results = append(rep.Results, makeResult(phase, v))
		}
		if i == 0 {
			rep.WarmupRequest = r.WarmupRequest
		}
		r.Repetitions = append(r.Repetitions, rep)
	}
	return r
}

func writeComparisonFixture(t *testing.T, r report) string {
	t.Helper()
	dir := t.TempDir()
	data, err := json.Marshal(r.summary())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tool.log"), []byte(r.logSummary()), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "events.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	emit := func(e any) {
		if err := enc.Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	emit(map[string]any{"source": "request", "phase": "warmup", "repetition": 1, "body": map[string]any{"max_tokens": 8}})
	for i, rep := range r.Repetitions {
		for _, res := range rep.Results {
			emit(map[string]any{"source": "request", "phase": res.Phase, "repetition": i + 1, "body": map[string]any{"seed": 42}})
		}
	}
	for _, e := range []map[string]any{
		{"msg": "request started", "time": "2026-10-21T16:00:00Z"},
		{"msg": "loading model from file", "status": "started", "time": "2026-10-21T16:00:01Z"},
		{"msg": "loading model from file", "status": "completed", "time": "2026-10-21T16:00:04.5Z"},
		{"msg": "chat-completions", "time": "2026-10-21T16:00:05.5Z"},
		{"msg": "request completed", "time": "2026-10-21T16:00:06Z"},
		{"msg": "LLAMA-CONTEXT-PARAMS", "values": "NBatch[2052]\nNUBatch[2052]\n"},
	} {
		e["trace_id"] = "warmup"
		emit(map[string]any{"source": "server", "phase": "warmup", "event": e})
	}
	emit(map[string]any{"source": "report", "report": r})
	return dir
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestCompareAutomaticSelection(t *testing.T) {
	parent := t.TempDir()
	add := func(name string) string {
		t.Helper()
		r := comparisonFixture([]float64{2, 6})
		if strings.HasPrefix(name, "new-") {
			r.TestStartedAt = "2026-10-20T16:00:00Z"
		}
		dir := filepath.Join(parent, name)
		if err := os.Rename(writeComparisonFixture(t, r), dir); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	// Managed roles, not timestamps or creation order, select the pair.
	newer, older := add("new-bbbbbbb"), add("old-aaaaaaa")
	add("release-v1.0.0")
	var out bytes.Buffer
	if err := compareCommand([]string{"-out", parent, "-json"}, &out); err != nil {
		t.Fatal(err)
	}
	var got matrixComparison
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Baseline.Directory != older || got.Candidate.Directory != newer || got.Status != "PASS" {
		t.Fatalf("incorrect automatic ordering: %+v", got)
	}
	add("20261021T190000Z-5")
	if err := compareCommand([]string{"-out", parent}, &out); err == nil {
		t.Fatal("accepted an ambiguous run queue")
	}
}

func TestCompareCommand(t *testing.T) {
	baseline := comparisonFixture([]float64{1, 4, 10})
	saved := readComparisonCellFixture(t, baseline).summary.Measurements["workload"]["elapsed_seconds"]
	base := writeComparisonFixture(t, baseline)
	next := comparisonFixture([]float64{2, 6, 14})
	next.Repetitions[0].SystemTokens++
	candidate := writeComparisonFixture(t, next)
	got, err := runComparisonFixture(t, base, candidate)
	if err != nil || got.Status != "PASS" || len(got.Cells) != 1 {
		t.Fatalf("comparison: %+v, %v", got, err)
	}
	if len(got.Cells[0].Warnings) != 0 {
		t.Fatalf("token counts or generated history produced warnings: %v", got.Cells[0].Warnings)
	}
	rows := comparisonMetricsFixture(got.Cells[0])
	r := rows["workload.elapsed_seconds"]
	if r.Baseline == nil || r.Candidate == nil || r.Delta == nil || r.Percent == nil {
		t.Fatalf("missing workload comparison: %+v", r)
	}
	if r.Baseline.Median != saved.Median || r.Baseline.Samples != saved.Samples {
		t.Fatal("comparison did not preserve saved measurements", r.Baseline, saved)
	}
	if r.Baseline.Samples != 3 || r.Baseline.Median != 4 || r.Baseline.Min != 1 || r.Baseline.Max != 10 || r.Baseline.SampleStdDev == nil || math.Abs(*r.Baseline.SampleStdDev-math.Sqrt(21)) > 1e-12 || r.Candidate.Median != 6 || *r.Delta != 2 || *r.Percent != 50 {
		t.Fatalf("incorrect statistics/direction: %+v", r)
	}
	zero := rows["cold.cached_tokens"]
	if zero.Percent != nil || zero.Delta == nil || *zero.Delta != 0 {
		t.Fatalf("zero denominator: %+v", zero)
	}
	startup := rows["warmup.startup.native_model_load_seconds"].Baseline
	if startup == nil || startup.Median != 3.5 || startup.Samples != 1 || startup.SampleStdDev != nil {
		t.Fatalf("saved startup measurement: %+v", startup)
	}
	// A compact queue must reproduce the full-evidence comparison without any
	// original files, including startup samples and normalized metrics.
	queue := t.TempDir()
	for name, dir := range map[string]string{"old-aaaaaaa": base, "new-bbbbbbb": candidate} {
		if err := os.Rename(dir, filepath.Join(queue, name)); err != nil {
			t.Fatal(err)
		}
	}
	compact := filepath.Join(t.TempDir(), "queue")
	if err := snapshotCommand([]string{"-out", queue, "-dest", compact}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := runComparisonFixture(t, filepath.Join(compact, "old-aaaaaaa"), filepath.Join(compact, "new-bbbbbbb"))
	if err != nil || !reflect.DeepEqual(got.Cells[0].Metrics, snapshot.Cells[0].Metrics) || !slices.Equal(got.Cells[0].Warnings, snapshot.Cells[0].Warnings) {
		t.Fatalf("snapshot changed comparison: %+v, %v", snapshot, err)
	}
	path := filepath.Join(compact, "new-bbbbbbb", "comparison-snapshot.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Replace(data, []byte(`"test-os"`), []byte(`"other-os"`), 1), 0600); err != nil {
		t.Fatal(err)
	}
	if err := compareCommand([]string{"-out", compact}, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted a modified snapshot without detecting its checksum mismatch")
	}
}

func TestCompareNormalizedTime(t *testing.T) {
	r := comparisonFixture([]float64{2, 6, 14})
	for i, tokensPerRequest := range []int{21, 51, 101} {
		for j := range r.Repetitions[i].Results {
			res := &r.Repetitions[i].Results[j]
			res.Usage.CompletionTokens = tokensPerRequest
			res.Usage.PromptTokens = []int{100, 200, 400}[i]
			res.Execution.ReusedTokens = []int{20, 100, 200}[i]
			res.ServerTimings["slot_prefill_ttft_seconds"] = []float64{0.2, 0.4, 0.5}[i]
			res.ServerTimings["generation_seconds"] = []float64{0.5, 1, 2}[i]
		}
		// Cancelled work must contribute neither tokens nor duration.
		r.Repetitions[i].Results[4].Usage.CompletionTokens = 999
		r.Repetitions[i].Results[4].ServerTimings["generation_seconds"] = 999
	}
	dir := writeComparisonFixture(t, r)
	got, err := runComparisonFixture(t, dir, dir)
	if err != nil || got.Status != "PASS" {
		t.Fatalf("comparison: %+v, %v", got, err)
	}
	// Effective input rates: 400, 250, 400; generation rates: 40, 50, 50.
	// Fixed 10708/1332 references give 60.07, 69.472, 53.41 seconds.
	rate := comparisonMetricsFixture(got.Cells[0])["workload.reference_seconds"].Candidate
	if rate == nil || rate.Samples != 3 || math.Abs(rate.Median-60.07) > 1e-10 || math.Abs(rate.Min-53.41) > 1e-10 || math.Abs(rate.Max-69.472) > 1e-10 || rate.SampleStdDev == nil || math.Abs(*rate.SampleStdDev-math.Sqrt(65.123508)) > 1e-10 {
		t.Fatalf("incorrect Normalized time: %+v", rate)
	}
	// A reference change preserves each saved value but cannot receive a delta.
	r.ReferenceTokens.InputTokens *= 2
	for i := range r.Repetitions {
		r.Repetitions[i].ReferenceTokens = r.ReferenceTokens
	}
	got, err = runComparisonFixture(t, dir, writeComparisonFixture(t, r))
	if err != nil || got.Status != "PASS" {
		t.Fatalf("comparison: %+v, %v", got, err)
	}
	rows := comparisonMetricsFixture(got.Cells[0])
	normalized := rows["workload.reference_seconds"]
	if normalized.Baseline == nil || normalized.Candidate == nil || normalized.Delta != nil || normalized.Percent != nil || math.Abs(normalized.Baseline.Median-60.07) > 1e-10 || math.Abs(normalized.Candidate.Median-86.84) > 1e-10 {
		t.Fatalf("reference mismatch reinterpreted values or received a delta: %+v", normalized)
	}
	if rows["workload.elapsed_seconds"].Delta == nil {
		t.Fatal("reference mismatch suppressed measured workload comparison")
	}
}

func TestCompareCellStatuses(t *testing.T) {
	for _, statuses := range [][2]string{{"PASS", "FAIL"}, {"FAIL", "PASS"}, {"PASS", "NOT_RUN"}} {
		t.Run(strings.Join(statuses[:], "-"), func(t *testing.T) {
			a, b := comparisonFixture([]float64{1, 4}), comparisonFixture([]float64{2, 6})
			a.Status, b.Status = statuses[0], statuses[1]
			// A common passing cell keeps a partial matrix comparable. Reverse the
			// candidate's cell order to verify that identity, not position, wins.
			image := comparisonFixture([]float64{8, 12})
			image.Images = true
			image.ReferenceTokens = referenceTokens(true, false, false, false)
			for i := range image.Repetitions {
				image.Repetitions[i].Images = true
				image.Repetitions[i].ReferenceTokens = image.ReferenceTokens
			}
			if b.Status == "NOT_RUN" {
				b = report{Status: "NOT_RUN"}
			}
			got, err := runComparisonFixture(t, writeComparisonFixture(t, a, image), writeComparisonFixture(t, image, b))
			fails := statuses[0] == "FAIL" || statuses[1] == "FAIL"
			if (err != nil) != fails || (got.Status == "FAIL") != fails || len(got.Cells) != 2 {
				t.Fatalf("comparison: %+v, %v", got, err)
			}
			var cell comparison
			for _, c := range got.Cells {
				if c.Cell == matrixCells[0].id {
					cell = c
				}
			}
			if cell.Cell != matrixCells[0].id || cell.SavedRunChecks["baseline"] != statuses[0] || cell.SavedRunChecks["candidate"] != statuses[1] {
				t.Fatalf("cell status/identity: %+v", cell)
			}
			if statuses[1] == "NOT_RUN" {
				if cell.Status != "NOT_RUN" || len(cell.Metrics) != 0 {
					t.Fatalf("unrun cell acquired a comparison: %+v", cell)
				}
				return
			}
			if cell.Status != "FAIL" {
				t.Fatalf("failed cell passed: %+v", cell)
			}
			rows := comparisonMetricsFixture(cell)
			for _, name := range []string{"workload.elapsed_seconds", "workload.reference_seconds"} {
				row := rows[name]
				if (row.Baseline != nil) != (statuses[0] == "PASS") || (row.Candidate != nil) != (statuses[1] == "PASS") || row.Delta != nil || row.Percent != nil {
					t.Fatalf("must retain only passing measurements without deltas: %+v", row)
				}
			}
			row := rows["workload.elapsed_seconds"]
			if row.Baseline != nil && row.Baseline.Median != 2.5 || row.Candidate != nil && row.Candidate.Median != 4 {
				t.Fatalf("passing side lost its values: %+v", row)
			}
		})
	}
}

func TestCompareRejectsBrokenArtifacts(t *testing.T) {
	base := writeComparisonFixture(t, comparisonFixture([]float64{1, 4}))
	for _, name := range []string{"summary-disagrees", "incomplete-repetition", "missing-reference"} {
		t.Run(name, func(t *testing.T) {
			baseline := base
			r := comparisonFixture([]float64{2, 6})
			if name == "incomplete-repetition" {
				r.Repetitions[0].Results = r.Repetitions[0].Results[:5]
			}
			if name == "missing-reference" {
				r = comparisonTwoSlotFixture([]float64{2, 6})
				baseline = writeComparisonFixture(t, r)
				r.ReferenceTokens = normalizationReference{}
				for i := range r.Repetitions {
					r.Repetitions[i].ReferenceTokens = r.ReferenceTokens
				}
			}
			dir := writeComparisonFixture(t, r)
			if name == "summary-disagrees" {
				var s matrixSummary
				data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &s); err != nil {
					t.Fatal(err)
				}
				s.Cells[0].Measurements["cold"]["elapsed_seconds"] = measurement{Samples: 2, Median: 999}
				data, err = json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "summary.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			if err := compareCommand([]string{"-out", t.TempDir(), "-baseline", baseline, "-candidate", dir}, &out); err == nil {
				t.Fatal("accepted broken artifacts")
			}
		})
	}
}

func TestCompareTwoSlotArrivalOrder(t *testing.T) {
	r := comparisonTwoSlotFixture([]float64{2, 6})
	base, candidate := writeComparisonFixture(t, r), writeComparisonFixture(t, r)
	path := filepath.Join(candidate, "events.ndjson")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	var requests [][]byte
	for _, line := range lines {
		if bytes.Contains(line, []byte(`"source":"request"`)) {
			requests = append(requests, line)
		}
	}
	slices.Reverse(requests)
	for i, line := range lines {
		if bytes.Contains(line, []byte(`"source":"request"`)) {
			lines[i], requests = requests[0], requests[1:]
		}
	}
	if err := os.WriteFile(path, bytes.Join(lines, []byte("\n")), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := runComparisonFixture(t, base, candidate)
	if err != nil || got.Status != "PASS" || !slices.Contains(got.Cells[0].Matches, "controlled request inputs") {
		t.Fatalf("arrival order invalidated comparison: %+v, %v", got, err)
	}
	row := comparisonMetricsFixture(got.Cells[0])["workload.reference_seconds"]
	if row.Baseline == nil || row.Baseline.Median <= 0 || row.Candidate == nil || row.Candidate.Samples != 2 || row.Delta == nil || *row.Delta != 0 || row.Percent == nil || *row.Percent != 0 {
		t.Fatalf("arrival order changed Normalized metrics: %+v", row)
	}
}

func runComparisonFixture(t *testing.T, baseline, candidate string) (matrixComparison, error) {
	t.Helper()
	var out bytes.Buffer
	err := compareCommand([]string{"-out", t.TempDir(), "-baseline", baseline, "-candidate", candidate, "-json"}, &out)
	var got matrixComparison
	if decodeErr := json.Unmarshal(out.Bytes(), &got); decodeErr != nil {
		t.Fatalf("comparison did not emit JSON: %v (command: %v): %s", decodeErr, err, &out)
	}
	return got, err
}

func comparisonMetricsFixture(cell comparison) map[string]metricComparison {
	rows := make(map[string]metricComparison)
	for _, row := range cell.Metrics {
		rows[row.Metric] = row
	}
	return rows
}

func comparisonTwoSlotFixture(values []float64) report {
	r := comparisonFixture(values)
	r.TwoSlots = true
	r.ID = "MoE/2s/IMC+/MTP+/img-"
	r.ReferenceTokens = referenceTokens(false, false, false, true)
	r.Metadata["model"].(map[string]any)["model_config"] = map[string]any{"nseq-max": 2}
	for i := range r.Repetitions {
		rep := &r.Repetitions[i]
		rep.TwoSlots = true
		rep.ReferenceTokens = r.ReferenceTokens
		old := rep.Results
		rep.Results = []result{old[0], old[1], old[3], old[2], old[2], old[3], old[4], old[5]}
		for j, phase := range []string{"cold", "replay", "fresh-session", "append", "peer-append", "survivor", "cancel", "recovery"} {
			rep.Results[j].Phase = phase
			rep.Results[j].TraceID = phase
		}
	}
	return r
}

func readComparisonCellFixture(t *testing.T, r report) comparisonInput {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(writeComparisonFixture(t, r), "events.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	input, err := readCellComparison(r.summary(), r, data)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func comparisonFixture(values []float64) report {
	r := report{Status: "PASS", Workload: workloadVersion, Model: modelID, ServerRevision: strings.Repeat("a", 40), TestStartedAt: "2026-10-21T16:00:00Z", HarnessRevision: "fixed-harness", MTPBackend: "qwen35-own-kv", RepetitionsRequested: len(values), evidenceValidated: true}
	r.ReferenceTokens = referenceTokens(false, false, false, false)
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
	warm.StartupTimings = map[string]float64{
		"server_request_seconds":    6,
		"pre_chat_seconds":          5.5,
		"native_model_load_seconds": 3.5,
	}
	r.WarmupRequest = &warm
	for i, v := range values {
		rep := report{Status: "PASS", SystemTokens: 10140, SystemSHA256: fmt.Sprintf("hash-%d", i), WorkloadSeconds: v, CancelReleaseSeconds: 0, evidenceValidated: true}
		rep.ReferenceTokens = r.ReferenceTokens
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

func writeComparisonFixture(t *testing.T, cells ...report) string {
	t.Helper()
	first := cells[0]
	for _, cell := range cells {
		if cell.Status != "NOT_RUN" {
			first = cell
			break
		}
	}
	r := matrixReport{Status: "PASS", ServerRevision: first.ServerRevision, ServerCommitTime: first.ServerCommitTime, ReleaseTag: first.ReleaseTag, TestStartedAt: first.TestStartedAt, Cells: cells}
	for i := range r.Cells {
		cell := &r.Cells[i]
		if cell.ID == "" {
			cell.ID = matrixCells[0].id
			if cell.Images {
				cell.ID = matrixCells[1].id
			}
		}
		if cell.Status == "FAIL" {
			r.Status = "FAIL"
		}
	}
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
		if event, ok := e.(map[string]any); ok && event["source"] != "report" {
			event["cell_id"] = first.ID
		}
		if err := enc.Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	for _, cell := range r.Cells {
		if cell.Status == "NOT_RUN" {
			continue
		}
		first = cell
		if cell.WarmupRequest != nil {
			emit(map[string]any{"source": "request", "phase": "warmup", "repetition": 1, "body": map[string]any{"max_tokens": 8}})
		}
		for i, rep := range cell.Repetitions {
			for j, res := range rep.Results {
				emit(map[string]any{"source": "request", "phase": res.Phase, "repetition": i + 1, "body": map[string]any{
					"seed":     42 + i*10 + j,
					"messages": []message{{Role: "assistant", Content: fmt.Sprintf("Previous answer %.2f", res.Elapsed)}, {Role: "user", Content: "Continue."}},
				}})
			}
		}
		if cell.WarmupRequest == nil {
			continue
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
	}
	emit(map[string]any{"source": "report", "report": r})
	return dir
}

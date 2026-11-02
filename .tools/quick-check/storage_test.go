package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromoteFailedRunRetention(t *testing.T) {
	out := filepath.Join(t.TempDir(), "output")
	if err := os.Mkdir(out, 0755); err != nil {
		t.Fatal(err)
	}
	r := comparisonFixture([]float64{2})
	r.Status, r.RepetitionsRequested = "FAIL", 2
	r.Findings = []string{"content finding; remaining repetition unrun"}
	r.Repetitions[0].Status = "FAIL"
	r.Repetitions[0].Findings = []string{"content finding"}
	source := filepath.Join(out, "new-aaaaaaa")
	if err := os.Rename(writeComparisonFixture(t, r, report{ID: matrixCells[1].id, Status: "NOT_RUN"}), source); err != nil {
		t.Fatal(err)
	}
	if err := promoteCommand([]string{"-out", out, "-release", "v1.0.0", "-commit", r.ServerRevision}, &bytes.Buffer{}); err != nil {
		t.Fatalf("promote failed, partial run with unfinished repetitions: %v", err)
	}
	release := filepath.Join(out, "release-v1.0.0")
	for _, name := range []string{"summary.json", "events.ndjson", "tool.log"} {
		original, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		promoted, err := os.ReadFile(filepath.Join(release, name))
		if err != nil || !bytes.Equal(original, promoted) {
			t.Fatalf("promotion changed %s: %v", name, err)
		}
	}
	next := comparisonFixture([]float64{3})
	next.ServerRevision = strings.Repeat("b", 40)
	next.Metadata["/v1/liveness"].(map[string]any)["vcs_revision"] = next.ServerRevision
	candidate := filepath.Join(out, "new-bbbbbbb")
	if err := os.Rename(writeComparisonFixture(t, next), candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := retainRuns(context.Background(), out, candidate); err != nil {
		t.Fatal(err)
	}
	runs, err := savedRuns(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		if run.path == release && run.report.Status == "FAIL" && run.report.ServerRevision == r.ServerRevision {
			return
		}
	}
	t.Fatal("retention discarded or renamed the explicitly promoted failed baseline")
}

func TestRetentionLatestAndPromotedRelease(t *testing.T) {
	out := t.TempDir()
	baseline := comparisonFixture([]float64{2, 6})
	source := filepath.Join(out, "new-aaaaaaa")
	if err := os.Rename(writeComparisonFixture(t, baseline), source); err != nil {
		t.Fatal(err)
	}
	if err := promoteCommand([]string{"-out", out, "-release", "v1.0.0", "-commit", baseline.ServerRevision}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// Exercise the same-commit failure boundary and later success. Neither may
	// replace the promoted baseline; no previous-PASS slot is retained.
	for i, status := range []string{"FAIL", "INTERRUPTED", "PASS"} {
		r := comparisonFixture([]float64{3, 7})
		r.Status = status
		if i == 2 {
			r.ServerRevision = strings.Repeat("b", 40)
		}
		staged := filepath.Join(out, "20261102T000000Z-1")
		if err := os.Rename(writeComparisonFixture(t, r), staged); err != nil {
			t.Fatal(err)
		}
		current, err := retainRuns(context.Background(), out, staged)
		if err != nil || current != filepath.Join(out, "new-"+r.ServerRevision[:7]) {
			t.Fatalf("retention: %s, %v", current, err)
		}
		runs, err := savedRuns(out)
		if err != nil || len(runs) != 2 {
			t.Fatalf("expected exactly latest and release: %+v, %v", runs, err)
		}
		data, err := os.ReadFile(filepath.Join(current, "summary.json"))
		var summary matrixSummary
		if err != nil || json.Unmarshal(data, &summary) != nil || summary.Status != status {
			t.Fatalf("latest attempt not preserved: %s, %v", data, err)
		}
		data, err = os.ReadFile(filepath.Join(out, "release-v1.0.0", "summary.json"))
		if err != nil || json.Unmarshal(data, &summary) != nil || summary.ServerRevision != baseline.ServerRevision || summary.Status != "PASS" {
			t.Fatalf("release baseline changed: %s, %v", data, err)
		}
	}
}

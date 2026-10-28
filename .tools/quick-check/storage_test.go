package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRetentionPassBaselines(t *testing.T) {
	a, b, c, d := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40), strings.Repeat("d", 40)
	type step struct {
		commit, status string
		// Indices identify the exact saved report, not just its commit.
		old, release int
	}
	for _, tc := range []struct {
		name  string
		tags  map[string]string
		steps []step
	}{
		{
			name: "failures-do-not-advance-queue",
			steps: []step{
				{a, "PASS", -1, -1},
				{b, "FAIL", 0, -1},
				{c, "FAIL", 0, -1},
				{c, "PASS", 0, -1},
				{d, "PASS", 3, -1},
			},
		},
		{
			name: "same-commit-preserves-old-pass",
			steps: []step{
				{a, "PASS", -1, -1},
				{a, "FAIL", 0, -1},
				{a, "INTERRUPTED", 0, -1},
				{a, "PASS", -1, -1},
				{b, "PASS", 3, -1},
			},
		},
		{
			name: "release-protection-and-failed-releases",
			tags: map[string]string{a: "v1.0.0", c: "v2.0.0"},
			steps: []step{
				{a, "PASS", -1, -1},
				{b, "PASS", -1, 0},
				{c, "FAIL", 1, 0},
				{c, "INTERRUPTED", 1, 0},
				{a, "FAIL", 1, 0},
				{a, "INTERRUPTED", 1, 0},
				{c, "PASS", 1, 0},
				{d, "PASS", 1, 6},
				{c, "PASS", 7, -1}, // Only PASS deduplicates the release commit.
				{b, "PASS", 7, 8},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			var reports []matrixReport
			for i, s := range tc.steps {
				stamp := time.Date(2026, 10, 22, 0, 0, i, 0, time.UTC)
				r := matrixReport{
					Status: s.status, ServerRevision: s.commit,
					TestStartedAt: stamp.Format(time.RFC3339Nano),
					// Root status is authoritative even with partial/skipped cells.
					Cells: []report{{Status: "NOT_RUN"}},
				}
				reports = append(reports, r)
				staged := filepath.Join(out, fmt.Sprintf("%s-%d", stamp.Format("20060102T150405Z"), i))
				if err := os.Mkdir(staged, 0755); err != nil {
					t.Fatal(err)
				}
				data, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				for name, data := range map[string][]byte{"summary.json": data, "tool.log": nil, "events.ndjson": nil} {
					if err := os.WriteFile(filepath.Join(staged, name), data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				current, err := retainRuns(context.Background(), out, staged, tc.tags)
				if err != nil {
					t.Fatalf("step %d: %v", i, err)
				}
				label := func(commit string) string {
					if tag := tc.tags[commit]; tag != "" {
						return tag + "-" + commit[:7]
					}
					return commit[:7]
				}
				want := map[string]int{"new-" + label(s.commit): i}
				if s.old >= 0 {
					want["old-"+label(reports[s.old].ServerRevision)] = s.old
				}
				if s.release >= 0 {
					want["release-"+tc.tags[reports[s.release].ServerRevision]] = s.release
				}
				if current != filepath.Join(out, "new-"+label(s.commit)) {
					t.Fatalf("step %d: candidate path %s", i, current)
				}
				runs, err := savedRuns(out)
				if err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(out)
				if err != nil || len(entries) != len(want) || len(runs) != len(want) {
					t.Fatalf("step %d: entries %v, runs %v, want %v; error %v", i, entries, runs, want, err)
				}
				for _, run := range runs {
					index, ok := want[filepath.Base(run.path)]
					if !ok {
						t.Fatalf("step %d: unexpected run %s; want %v", i, run.path, want)
					}
					expected := reports[index]
					if run.report.Status != expected.Status || run.report.ServerRevision != expected.ServerRevision || run.report.TestStartedAt != expected.TestStartedAt {
						t.Fatalf("step %d: %s has %+v; want %+v", i, run.path, run.report, expected)
					}
				}
			}
		})
	}
}

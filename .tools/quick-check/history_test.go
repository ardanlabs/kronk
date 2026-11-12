package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryComparison(t *testing.T) {
	for _, tc := range []struct {
		name       string
		modify     func(string) string
		status     string
		workload   bool
		normalized bool
	}{
		{name: "matching history", status: "PASS", workload: true, normalized: true},
		{name: "other machine", modify: func(s string) string { return strings.ReplaceAll(s, "test-machine", "other-machine") }, status: "NOT_COMPARABLE"},
		{name: "other reference", modify: func(s string) string {
			lines := strings.Split(s, "\n")
			lines[2] = "References updated: yes; reference ID: 0000000000000000"
			return strings.Join(lines, "\n")
		}, status: "PARTIAL", workload: true},
		{name: "zero placeholder", modify: func(s string) string {
			return strings.NewReplacer("4.00 [2.83]", "0.00 [0.00]", "2.24 [0.00]", "0.00 [0.00]").Replace(s)
		}, status: "NO_BASELINE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, history := historyComparisonFixture(t)
			original, err := os.ReadFile(history)
			if err != nil {
				t.Fatal(err)
			}
			if tc.modify != nil {
				original = []byte(tc.modify(string(original)))
				if err := os.WriteFile(history, original, 0600); err != nil {
					t.Fatal(err)
				}
			}
			var text bytes.Buffer
			if err := historyCommand([]string{"-out", parent, "-history", history}, &text); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(parent, "comparison.json"))
			if err != nil {
				t.Fatal(err)
			}
			var c matrixComparison
			if err := json.Unmarshal(data, &c); err != nil {
				t.Fatal(err)
			}
			if c.Status != tc.status || len(c.Cells) != 1 || len(c.Cells[0].Metrics) != 2 {
				t.Fatalf("got comparison %s, want %s with two metrics: %s", c.Status, tc.status, data)
			}
			workload, normalized := c.Cells[0].Metrics[0], c.Cells[0].Metrics[1]
			if (workload.Percent != nil) != tc.workload || (normalized.Percent != nil) != tc.normalized {
				t.Fatalf("incorrect delta availability: %s", data)
			}
			if tc.workload && (workload.Baseline.Median != 4 || workload.Candidate.Median != 6 || *workload.Percent != 50) {
				t.Fatalf("want independently calculated 4 → 6 (+50%%), got %s", data)
			}
			if tc.normalized && *normalized.Percent != 0 {
				t.Fatalf("unchanged normalized workload should have zero delta: %s", data)
			}
			for _, name := range []string{"comparison.log", "comparison.md"} {
				data, err := os.ReadFile(filepath.Join(parent, name))
				if err != nil || !bytes.Contains(data, []byte("Positive percentages mean slower")) {
					t.Fatalf("%s missing report/limitations: %v", name, err)
				}
			}
			data, err = os.ReadFile(history)
			if err != nil || !bytes.Equal(original, data) {
				t.Fatalf("read-only comparison changed history: %v", err)
			}
		})
	}
}

func TestHistoryLatestCandidateColumn(t *testing.T) {
	parent, history := historyComparisonFixture(t)
	var out bytes.Buffer
	// Record 4 → 6 first; then simulate another run at 10 seconds.
	if err := historyCommand([]string{"-out", parent, "-history", history, "-release", "v0.2.0"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(parent, "new-candidate")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(writeComparisonFixture(t, comparisonFixture([]float64{8, 12})), filepath.Join(parent, "new-candidate")); err != nil {
		t.Fatal(err)
	}
	if err := historyCommand([]string{"-out", parent, "-history", history}, &out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(parent, "comparison.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c matrixComparison
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	row := c.Cells[0].Metrics[0]
	// Using the first release or the latest Baseline column would give 4 → 10.
	if c.Baseline.Release != "v0.2.0" || row.Baseline.Median != 6 || row.Candidate.Median != 10 || row.Percent == nil || *row.Percent < 66.66 || *row.Percent > 66.67 {
		t.Fatalf("did not use the latest Candidate column: %s", data)
	}
}

func TestHistoryLinuxPlaceholder(t *testing.T) {
	parent, history := historyComparisonFixture(t)
	data, err := os.ReadFile("release-history-linux-vulkan.log")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(history, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := historyCommand([]string{"-out", parent, "-history", history}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Comparison status: NO_BASELINE") || strings.Contains(out.String(), "+Inf") || strings.Contains(out.String(), "+0.00%") {
		t.Fatalf("zero Linux placeholder misreported as a measured baseline:\n%s", &out)
	}
	after, err := os.ReadFile(history)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatalf("placeholder changed: %v", err)
	}
	if err := historyCommand([]string{"-out", parent, "-history", history, "-release", "v0.2.0"}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := historyCommand([]string{"-out", parent, "-history", history}, &out); err != nil {
		t.Fatalf("cannot read first measured release with unrun cells: %v", err)
	}
	if !strings.Contains(out.String(), "Comparison status: PARTIAL") || !strings.Contains(out.String(), "+0.00%") {
		t.Fatalf("first measured release was not used as the next baseline: %s", &out)
	}
}

func TestHistoryRounding(t *testing.T) {
	for _, value := range []float64{2.125, 2.135} {
		parent := t.TempDir()
		history := filepath.Join(parent, "history.log")
		source := writeComparisonFixture(t, comparisonFixture([]float64{value, value}))
		input, err := readComparison(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := recordReleaseStart(history, "v0.1.0", input); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(source, filepath.Join(parent, "new-candidate")); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := historyCommand([]string{"-out", parent, "-history", history}, &out); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(parent, "comparison.json"))
		if err != nil {
			t.Fatal(err)
		}
		var c matrixComparison
		if err := json.Unmarshal(data, &c); err != nil {
			t.Fatal(err)
		}
		for _, metric := range c.Cells[0].Metrics {
			if metric.Percent == nil || *metric.Percent != 0 {
				t.Fatalf("identical run at %v reported a timing change: %s", value, data)
			}
		}
	}
}

func TestHistoryFailedCandidate(t *testing.T) {
	parent, history := historyComparisonFixture(t)
	before, err := os.ReadFile(history)
	if err != nil {
		t.Fatal(err)
	}
	r := comparisonFixture([]float64{4, 8})
	r.Status, r.Findings = "FAIL", []string{"recorded failure"}
	if err := os.RemoveAll(filepath.Join(parent, "new-candidate")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(writeComparisonFixture(t, r), filepath.Join(parent, "new-candidate")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := historyCommand([]string{"-out", parent, "-history", history, "-release", "v0.2.0"}, &out); !errors.Is(err, errComparisonFindings) {
		t.Fatalf("got %v, want recorded failure", err)
	}
	after, err := os.ReadFile(history)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed release changed history: %v", err)
	}
	if !strings.Contains(out.String(), "Comparison status: FAIL") {
		t.Fatalf("failed candidate missing report: %s", &out)
	}
}

func historyComparisonFixture(t *testing.T) (string, string) {
	t.Helper()
	parent := t.TempDir()
	history := filepath.Join(t.TempDir(), "history.log")
	input, err := readComparison(writeComparisonFixture(t, comparisonFixture([]float64{2, 6})))
	if err != nil {
		t.Fatal(err)
	}
	if err := recordReleaseStart(history, "v0.1.0", input); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(writeComparisonFixture(t, comparisonFixture([]float64{4, 8})), filepath.Join(parent, "new-candidate")); err != nil {
		t.Fatal(err)
	}
	return parent, history
}

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func benchmarkFixture(identity, host string) report {
	r := comparisonFixture([]float64{2, 6})
	r.BenchmarkIdentity = identity
	r.Metadata["/v1/liveness"].(map[string]any)["host"] = host
	for i := range r.Repetitions {
		r.Repetitions[i].BenchmarkIdentity = identity
	}
	return r
}

func TestBenchmarkComparisonHistoryAndSnapshot(t *testing.T) {
	// Comparison must consume saved identities, never the caller's environment.
	t.Setenv("QUICK_CHECK_BENCHMARK_ID", "must-not-be-injected")
	for _, tc := range []struct {
		name, aID, bID, aHost, bHost, hardware string
		match                                  bool
	}{
		{"stable-identity", "linux-vulkan", "linux-vulkan", "container-a", "container-b", "", true},
		{"different-identities", "linux-vulkan", "other", "same", "same", "", false},
		{"only-baseline-identity", "linux-vulkan", "", "same", "same", "", false},
		{"only-candidate-identity", "", "linux-vulkan", "same", "same", "", false},
		{"legacy-matching-host", "", "", "mac", "mac", "", true},
		{"legacy-different-host", "", "", "mac-a", "mac-b", "", false},
		{"different-os", "linux-vulkan", "linux-vulkan", "a", "b", "os", false},
		{"different-ram", "linux-vulkan", "linux-vulkan", "a", "b", "system_ram_bytes", false},
		{"different-vram", "linux-vulkan", "linux-vulkan", "a", "b", "gpu_total_bytes", false},
		{"different-unified", "linux-vulkan", "linux-vulkan", "a", "b", "unified_memory", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := benchmarkFixture(tc.aID, tc.aHost), benchmarkFixture(tc.bID, tc.bHost)
			b.ServerRevision = strings.Repeat("b", 40)
			switch tc.hardware {
			case "os":
				b.Metadata["/v1/liveness"].(map[string]any)["os"] = "different"
			case "system_ram_bytes", "gpu_total_bytes":
				b.Metadata["/v1/devices"].(map[string]any)[tc.hardware] = 99999
			case "unified_memory":
				b.Metadata["/v1/devices"].(map[string]any)[tc.hardware] = false
			}
			queue := t.TempDir()
			paths := []string{filepath.Join(queue, "release-v1.0.0"), filepath.Join(queue, "new-bbbbbbb")}
			for i, r := range []report{a, b} {
				if err := os.Rename(writeComparisonFixture(t, r), paths[i]); err != nil {
					t.Fatal(err)
				}
				data, _ := json.Marshal(releasePromotion{[]string{"v1.0.0", "v1.1.0"}[i], r.ServerRevision})
				if err := os.WriteFile(filepath.Join(paths[i], "promotion.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			history := filepath.Join(t.TempDir(), "release-history-linux-vulkan.log")
			compare := func(aPath, bPath string) matrixComparison {
				t.Helper()
				var out bytes.Buffer
				if err := compareCommand([]string{"-baseline", aPath, "-candidate", bPath, "-out", t.TempDir(), "-history", history, "-json"}, &out); err != nil {
					t.Fatal(err)
				}
				var got matrixComparison
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				return got
			}
			got := compare(paths[0], paths[1])
			if (len(got.Cells[0].Warnings) == 0) != tc.match {
				t.Fatalf("unexpected comparability: %+v", got.Cells[0])
			}
			if tc.aID != "" && tc.aID == tc.bID && !slices.Contains(got.Cells[0].Matches, "machine.benchmark_identity") {
				t.Fatal("matching identity not reported")
			}
			data, err := os.ReadFile(history)
			if tc.match {
				machine := tc.bID
				if machine == "" {
					machine = tc.bHost
				}
				if err != nil || !strings.Contains(string(data), "machine: "+machine+"; os: test-os;") {
					t.Fatalf("history missing stable machine: %s, %v", data, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("mismatched machines produced history: %s, %v", data, err)
			}
			compact := filepath.Join(t.TempDir(), "snapshots")
			if err := snapshotCommand([]string{"-out", queue, "-dest", compact}, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			snapshotPaths := []string{filepath.Join(compact, "release-v1.0.0"), filepath.Join(compact, "new-bbbbbbb")}
			for i, path := range snapshotPaths {
				s, err := readSnapshot(path)
				if err != nil || s.Summary.BenchmarkIdentity != []string{tc.aID, tc.bID}[i] || s.Summary.Machine["host"] != []string{tc.aHost, tc.bHost}[i] {
					t.Fatalf("snapshot lost identity/actual host: %+v, %v", s.Summary, err)
				}
			}
			snap := compare(snapshotPaths[0], snapshotPaths[1])
			if !reflect.DeepEqual(got.Cells[0].Metrics, snap.Cells[0].Metrics) || !slices.Equal(got.Cells[0].Warnings, snap.Cells[0].Warnings) {
				t.Fatal("snapshot changed comparison")
			}
		})
	}
}

func TestPromoteBenchmarkHistoryPath(t *testing.T) {
	for _, identity := range []string{"", "linux-vulkan"} {
		t.Run(identity, func(t *testing.T) {
			root := t.TempDir()
			queue := filepath.Join(root, "output")
			if err := os.Mkdir(queue, 0755); err != nil {
				t.Fatal(err)
			}
			history := filepath.Join(root, "selected-history.log")
			for i, tag := range []string{"v1.0.0", "v1.1.0"} {
				host := "mac"
				if identity != "" {
					host = []string{"container-a", "container-b"}[i]
				}
				r := benchmarkFixture(identity, host)
				r.ServerRevision = strings.Repeat([]string{"a", "b"}[i], 40)
				if err := os.Rename(writeComparisonFixture(t, r), filepath.Join(queue, "new-"+r.ServerRevision[:7])); err != nil {
					t.Fatal(err)
				}
				args := []string{"-out", queue, "-release", tag, "-commit", r.ServerRevision, "-history", history}
				if err := promoteCommand(args, &bytes.Buffer{}); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(history)
				if err != nil || !strings.Contains(string(data), "v1.0.0 (starting baseline;") || (i == 1 && !strings.Contains(string(data), "v1.0.0 → v1.1.0")) {
					t.Fatalf("promotion did not forward history path: %s, %v", data, err)
				}
				if i == 0 {
					// The already-promoted path must also honor -history.
					if err := os.Remove(history); err != nil {
						t.Fatal(err)
					}
					if err := promoteCommand(args, &bytes.Buffer{}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := os.Stat(filepath.Join(root, "release-history-macos-metal.log")); !os.IsNotExist(err) {
				t.Fatal("explicit history path wrote default history", err)
			}
		})
	}
}

func TestBenchmarkEvidenceValidation(t *testing.T) {
	valid := writeComparisonFixture(t, benchmarkFixture("linux-vulkan", "actual-host"))
	for _, defect := range []string{"matrix-summary", "cell-summary", "repetition", "snapshot"} {
		t.Run(defect, func(t *testing.T) {
			r := benchmarkFixture("linux-vulkan", "actual-host")
			if defect == "repetition" {
				r.Repetitions[0].BenchmarkIdentity = "other"
			}
			dir := writeComparisonFixture(t, r)
			if defect == "snapshot" {
				s, err := makeSnapshot(dir)
				if err != nil {
					t.Fatal(err)
				}
				s.Summary.Cells[0].BenchmarkIdentity = "other"
				// Even a recomputed checksum cannot bless inconsistent identity.
				s.Digest = snapshotDigest(s)
				data, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				dir = t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "comparison-snapshot.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			} else if defect != "repetition" {
				path := filepath.Join(dir, "summary.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var s matrixSummary
				if err := json.Unmarshal(data, &s); err != nil {
					t.Fatal(err)
				}
				if defect == "matrix-summary" {
					s.BenchmarkIdentity = "other"
				} else {
					s.Cells[0].BenchmarkIdentity = "other"
				}
				data, err = json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := compareCommand([]string{"-baseline", valid, "-candidate", dir, "-out", t.TempDir()}, &bytes.Buffer{}); err == nil {
				t.Fatal("accepted inconsistent saved benchmark identity")
			}
		})
	}
}

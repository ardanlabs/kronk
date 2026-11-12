package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNightlyHistoryReport(t *testing.T) {
	for _, tool := range []string{"bash", "go", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("nightly report requires %s: %v", tool, err)
		}
	}
	parent, source := historyComparisonFixture(t)
	root := t.TempDir()
	history := filepath.Join(root, ".tools/quick-check/release-history-macos-metal.log")
	if err := os.MkdirAll(filepath.Dir(history), 0755); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(history, original, 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "quick-check-bin")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/bash\nshift 2\nexec \"$QUICK_CHECK_BINARY\" \"$@\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../.github/scripts/quick-check.sh")
	if err != nil {
		t.Fatal(err)
	}
	summary := filepath.Join(root, "summary.md")
	for _, action := range []string{"compare", "package"} {
		cmd := exec.CommandContext(t.Context(), "bash", script, action)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "QUICK_CHECK_BINARY="+binary, "RUNNER_TEMP="+root, "QUICK_CHECK_PLATFORM=macos-metal", "QUICK_CHECK_OUTPUT="+parent, "GITHUB_STEP_SUMMARY="+summary)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", action, err, out)
		}
	}
	after, err := os.ReadFile(history)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatalf("nightly touched checked-out history: %v", err)
	}
	markdown, err := os.ReadFile(summary)
	if err != nil || !bytes.Contains(markdown, []byte("+50.00%")) {
		t.Fatalf("summary missing timing comparison: %v\n%s", err, markdown)
	}
	f, err := os.Open(filepath.Join(root, "quick-check-run.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	found := false
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == "compare-release/comparison.log" {
			data, err := io.ReadAll(archive)
			if err != nil || !bytes.Contains(data, []byte("+50.00%")) {
				t.Fatalf("artifact missing timing comparison: %v", err)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("artifact omitted the comparison log")
	}
	if err := os.WriteFile(history, []byte("invalid history\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(parent, "comparison.md"), filepath.Join(parent, "compare-release/comparison.md")} {
		if err := os.WriteFile(path, []byte("STALE REPORT"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.CommandContext(t.Context(), "bash", script, "compare")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "QUICK_CHECK_BINARY="+binary, "RUNNER_TEMP="+root, "QUICK_CHECK_PLATFORM=macos-metal", "QUICK_CHECK_OUTPUT="+parent, "GITHUB_STEP_SUMMARY="+summary)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("accepted invalid history: %s", out)
	}
	markdown, err = os.ReadFile(summary)
	if err != nil || bytes.Contains(markdown, []byte("STALE REPORT")) || !bytes.Contains(markdown, []byte("No comparison report could be generated")) {
		t.Fatalf("failed comparison reused stale report: %v\n%s", err, markdown)
	}
}

func TestPlatformArtifactRetention(t *testing.T) {
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("artifact retention requires %s: %v", tool, err)
		}
	}
	for _, platform := range []string{"macos-metal", "linux-vulkan"} {
		t.Run(platform, func(t *testing.T) {
			root := t.TempDir()
			mock := `#!/bin/bash
set -eu
method=GET query= endpoint=
shift
while (( $# )); do
  case "$1" in
    --method) method=$2; shift 2 ;;
    --jq) query=$2; shift 2 ;;
    repos/*) endpoint=$1; shift ;;
    *) shift ;;
  esac
done
if [[ "$method" == DELETE ]]; then
  printf '%s\n' "$endpoint" >> "$DELETE_LOG"
  exit 0
fi
case "$endpoint" in
  */actions/workflows\?*)
    body='{"workflows":[{"id":11,"path":".github/workflows/quick-check-macos-metal.yml"},{"id":22,"path":".github/workflows/quick-check-linux-vulkan.yml"},{"id":33,"path":".github/workflows/quick-check.yml"},{"id":44,"path":".github/workflows/other.yml"}]}' ;;
  */workflows/11/runs\?*) body='{"workflow_runs":[{"id":101},{"id":100}]}' ;;
  */workflows/22/runs\?*) body='{"workflow_runs":[{"id":202},{"id":201}]}' ;;
  */runs/101/artifacts\?*|*/runs/202/artifacts\?*)
    body="{\"artifacts\":[{\"id\":300,\"expired\":false,\"name\":\"quick-check-$QUICK_CHECK_PLATFORM-run-9-1\"},{\"id\":299,\"expired\":false,\"name\":\"quick-check-$QUICK_CHECK_PLATFORM-run-8-1\"},{\"id\":999,\"expired\":false,\"name\":\"unrelated-artifact\"}]}" ;;
  */runs/100/artifacts\?*|*/runs/201/artifacts\?*)
    [[ ${DISCOVERY_FAIL:-0} != 1 ]] || exit 9
    body="{\"artifacts\":[{\"id\":298,\"expired\":false,\"name\":\"quick-check-$QUICK_CHECK_PLATFORM-run-7-1\"},{\"id\":300,\"expired\":false,\"name\":\"quick-check-$QUICK_CHECK_PLATFORM-run-9-1\"},{\"id\":700,\"expired\":true,\"name\":\"quick-check-$QUICK_CHECK_PLATFORM-run-6-1\"}]}" ;;
  *) echo "Unexpected API read: $endpoint" >&2; exit 1 ;;
esac
printf '%s' "$body" | jq -r "$query"
`
			if err := os.WriteFile(filepath.Join(root, "gh"), []byte(mock), 0755); err != nil {
				t.Fatal(err)
			}
			script, err := filepath.Abs("../../.github/scripts/quick-check.sh")
			if err != nil {
				t.Fatal(err)
			}
			deletions := filepath.Join(root, "deletions.log")
			for _, fail := range []string{"1", "0"} {
				cmd := exec.CommandContext(t.Context(), "bash", script, "prune")
				cmd.Dir = root
				// Upload 301 is confirmed but not yet visible in API listings.
				cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "RUNNER_TEMP="+root, "QUICK_CHECK_PLATFORM="+platform, "GITHUB_REPOSITORY=test/repo", "UPLOADED_ARTIFACT_ID=301", "DELETE_LOG="+deletions, "DISCOVERY_FAIL="+fail)
				out, err := cmd.CombinedOutput()
				if fail == "1" {
					if err == nil {
						t.Fatalf("accepted failed discovery: %s", out)
					}
					if _, err := os.Stat(deletions); !os.IsNotExist(err) {
						t.Fatalf("deleted artifacts after partial discovery failure: %v", err)
					}
					continue
				}
				if err != nil {
					t.Fatalf("prune: %v\n%s", err, out)
				}
				data, err := os.ReadFile(deletions)
				want := "repos/test/repo/actions/artifacts/299\nrepos/test/repo/actions/artifacts/298\n"
				if err != nil || string(data) != want {
					t.Fatalf("got deletions %q (%v), want %q", data, err, want)
				}
			}
		})
	}
}

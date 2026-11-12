package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalReleaseHistory(t *testing.T) {
	for _, tool := range []string{"bash", "git", "go", "jq", "unzip"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("local release history requires %s: %v", tool, err)
		}
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0755); err != nil {
			t.Fatal(err)
		}
	}
	command := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	binary := filepath.Join(root, "quick-check")
	command(".", "go", "build", "-o", binary, ".")
	for _, path := range []string{".github/scripts/quick-check.sh", ".tools/quick-check/release-history.sh"} {
		data, err := os.ReadFile(filepath.Join("../..", path))
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(repo, path), string(data))
	}
	command(repo, "git", "init", "-q")
	for key, value := range map[string]string{"user.name": "Test", "user.email": "test@example.invalid", "commit.gpgSign": "false", "tag.gpgSign": "false", "core.hooksPath": "/dev/null"} {
		command(repo, "git", "config", key, value)
	}
	write(filepath.Join(repo, "cmd/runtime.txt"), "baseline\n")
	command(repo, "git", "add", ".")
	command(repo, "git", "commit", "-qm", "baseline")
	write(filepath.Join(repo, ".tools/quick-check/release-history-macos-metal.log"), "history only\n")
	command(repo, "git", "add", ".")
	command(repo, "git", "commit", "-qm", "history")
	command(repo, "git", "tag", "v0.1.0")
	// The baseline can be measured after the tag when only tooling was added.
	write(filepath.Join(repo, ".tools/measurement.txt"), "measurement tooling\n")
	command(repo, "git", "add", ".")
	command(repo, "git", "commit", "-qm", "measurement tooling")
	baseline := command(repo, "git", "rev-parse", "HEAD")
	if err := os.Remove(filepath.Join(repo, ".tools/quick-check/release-history-macos-metal.log")); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(repo, "cmd/runtime.txt"), "candidate\n")
	command(repo, "git", "add", ".")
	command(repo, "git", "commit", "-qm", "candidate")
	candidate := command(repo, "git", "rev-parse", "HEAD")
	remote := filepath.Join(root, "remote.git")
	command(repo, "git", "init", "--bare", "-q", remote)
	command(repo, "git", "remote", "add", "origin", remote)

	for _, platform := range []string{"macos-metal", "linux-vulkan"} {
		for i, revision := range []string{baseline, candidate} {
			r := comparisonFixture([]float64{2 + float64(i)*2, 6 + float64(i)*2})
			r.ServerRevision = revision
			liveness := r.Metadata["/v1/liveness"].(map[string]any)
			liveness["vcs_revision"], liveness["os"] = revision, "darwin"
			if platform == "linux-vulkan" {
				liveness["os"], r.BenchmarkIdentity = "linux", "linux-vulkan-runner"
				for j := range r.Repetitions {
					r.Repetitions[j].BenchmarkIdentity = r.BenchmarkIdentity
				}
			}
			dir := writeComparisonFixture(t, r)
			if i == 0 {
				input, err := readComparison(dir)
				if err != nil {
					t.Fatal(err)
				}
				if err := recordReleaseStart(filepath.Join(repo, ".tools/quick-check/release-history-"+platform+".log"), "v0.1.0", input); err != nil {
					t.Fatal(err)
				}
			}
			var compressed bytes.Buffer
			gz := gzip.NewWriter(&compressed)
			tarball := tar.NewWriter(gz)
			for _, name := range []string{"summary.json", "events.ndjson", "tool.log"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				if err := tarball.WriteHeader(&tar.Header{Name: "new-" + revision + "/" + name, Mode: 0600, Size: int64(len(data))}); err != nil {
					t.Fatal(err)
				}
				if _, err := tarball.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err := tarball.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			var archive bytes.Buffer
			z := zip.NewWriter(&archive)
			entry, err := z.Create("quick-check-run.tgz")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write(compressed.Bytes()); err != nil {
				t.Fatal(err)
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			name := platform + "-baseline.zip"
			if i == 1 {
				name = platform + "-candidate.zip"
			}
			write(filepath.Join(root, name), archive.String())
		}
	}
	bin := filepath.Join(root, "bin")
	write(filepath.Join(bin, "go"), "#!/bin/bash\nshift 2\nexec \"$QUICK_CHECK_BINARY\" \"$@\"\n")
	write(filepath.Join(bin, "gh"), `#!/bin/bash
set -eu
case "$*" in
  *actions/workflows\?*)
    if [[ "$*" == *linux-vulkan* ]]; then echo 22; else echo 11; fi ;;
  *workflows/11/runs*) echo 101 ;;
  *workflows/22/runs*) echo 202 ;;
  *runs/101/artifacts*) printf '1012\n1011\n' ;;
  *runs/202/artifacts*)
    if [[ ${MISSING_LINUX:-0} != 1 ]]; then echo 2022; fi
    echo 2021 ;;
  *artifacts/1012/zip*) cat "$FIXTURES/macos-metal-candidate.zip" ;;
  *artifacts/1011/zip*) cat "$FIXTURES/macos-metal-baseline.zip" ;;
  *artifacts/2022/zip*) cat "$FIXTURES/linux-vulkan-candidate.zip" ;;
  *artifacts/2021/zip*) cat "$FIXTURES/linux-vulkan-baseline.zip" ;;
  *) echo "Unexpected GitHub call: $*" >&2; exit 1 ;;
esac
`)
	initial := make(map[string]string)
	for _, platform := range []string{"macos-metal", "linux-vulkan"} {
		path := filepath.Join(repo, ".tools/quick-check/release-history-"+platform+".log")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		initial[path] = string(data)
	}
	invoke := func(missing bool, extra ...string) (string, error) {
		args := append([]string{".tools/quick-check/release-history.sh", "v0.2.0"}, extra...)
		cmd := exec.CommandContext(t.Context(), "bash", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "QUICK_CHECK_BINARY="+binary, "FIXTURES="+root, "GITHUB_REPOSITORY=test/repo", "MISSING_LINUX=0")
		if missing {
			cmd.Env = append(cmd.Env, "MISSING_LINUX=1")
		}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := invoke(true); err == nil || !strings.Contains(out, "No available linux-vulkan nightly artifact") {
		t.Fatalf("missing Linux evidence: %v\n%s", err, out)
	}
	for path, want := range initial {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("missing evidence changed %s history: %v", path, err)
		}
	}
	for range 2 {
		if out, err := invoke(false); err != nil {
			t.Fatalf("release history: %v\n%s", err, out)
		}
		for _, platform := range []string{"macos-metal", "linux-vulkan"} {
			data, err := os.ReadFile(filepath.Join(repo, ".tools/quick-check/release-history-"+platform+".log"))
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			if strings.Count(text, "Release comparison: v0.1.0 → v0.2.0") != 1 || !strings.Contains(text, "4.00 [2.83]") || !strings.Contains(text, "6.00 [2.83]") || !strings.Contains(text, "+50.00%") {
				t.Fatalf("incorrect %s history or duplicated entry:\n%s", platform, text)
			}
		}
	}
	before := make(map[string]string)
	for _, platform := range []string{"macos-metal", "linux-vulkan"} {
		path := filepath.Join(repo, ".tools/quick-check/release-history-"+platform+".log")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		before[path] = string(data)
	}
	if out, err := invoke(false, candidate, "v9.9.9"); err == nil || !strings.Contains(out, "not requested v9.9.9") {
		t.Fatalf("wrong baseline: %v\n%s", err, out)
	}
	for path, want := range before {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("wrong baseline changed %s: %v", path, err)
		}
	}
	if heads := command(repo, "git", "ls-remote", "--heads", "origin"); heads != "" {
		t.Fatalf("release history wrote remote branches: %s", heads)
	}
	if head := command(repo, "git", "rev-parse", "HEAD"); head != candidate {
		t.Fatalf("release history created a commit: got %s, want %s", head, candidate)
	}
}

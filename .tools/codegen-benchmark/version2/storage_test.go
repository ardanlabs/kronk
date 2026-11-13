package main

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegenerateLegacyReportAsPlainText(t *testing.T) {
	dir := t.TempDir()
	const legacy = "## Run metadata\n\n```json\n" + `{"started":"2026-11-13T13:30:46-05:00","finished":"2026-11-13T13:35:33-05:00","status":"completed","machine":"original-host (darwin/arm64)"}` + "\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(options{report: true, reportDir: dir}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Started: 2026-11-13T13:30:46-05:00", "Elapsed: 4m47s", "Runner machine: original-host (darwin/arm64)", "Workflow: completed"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("regenerated report missing %q: %s", want, data)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "report.md")); err != nil || string(data) != legacy {
		t.Fatalf("regeneration changed legacy report: %v", err)
	}
}

// Exercise the complete workflow without inference, then play the saved
// submission independently of the temporary agent workspace.
func TestSavedRunsAreRunnableAndRetained(t *testing.T) {
	dir := t.TempDir()
	source := fixtureSource(t)
	fixture := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(fixture, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BENCHMARK_FIXTURE", fixture)
	promptLog := filepath.Join(dir, "sent-prompt.txt")
	t.Setenv("BENCHMARK_PROMPT_LOG", promptLog)
	script := `#!/bin/sh
set -eu
for arg; do :; done
printf '%s' "$arg" > "$BENCHMARK_PROMPT_LOG"
cp "$BENCHMARK_FIXTURE" tictactoe/main.go
if [ "${BENCHMARK_API_ERROR:-}" = yes ]; then
  printf '%s\n' '{"type":"error","error":{"name":"APIError","data":{"message":"input tokens [143702] exceed context window [131072]"}}}'
elif [ "${BENCHMARK_BAD_EVENTS:-}" = yes ]; then
  printf '{\n'
else
  printf '%s\n' '{"type":"step_finish","part":{"tokens":{"input":17,"output":9}}}'
fi
`
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	config := filepath.Join(dir, "models.yaml")
	if err := os.WriteFile(config, []byte("version: 2\nmodels:\n  org/model/BENCHMARK:\n    context-window: 8192\n    sampling-parameters:\n      max_tokens: 1024\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"org/model/BENCHMARK"}]}`)
		}
	}))
	defer server.Close()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	opts := options{host: server.URL, model: "all", configFile: config,
		promptFile: filepath.Join(root, ".tools/codegen-benchmark/version2/tic-tac-toe-v1.md"),
		outputDir:  filepath.Join(dir, "runs"), attempts: 3, feedback: true, steps: 5, timeout: time.Minute}
	historyPath := filepath.Join(dir, "history.txt")
	const history = "Previously recorded run\n"
	if err := os.WriteFile(historyPath, []byte(history), 0o644); err != nil {
		t.Fatal(err)
	}
	var previous string
	var previousReport []byte
	for range 2 {
		if err := run(opts); err != nil {
			t.Fatal(err)
		}
		sent, err := os.ReadFile(promptLog)
		if err != nil || !strings.HasPrefix(string(sent), "# Tic-Tac-Toe — v1") || strings.Contains(string(sent), "Undo") || strings.Contains(string(sent), "writing-go") || !strings.Contains(string(sent), "Do not declare any struct types") || !strings.Contains(string(sent), "without a trailing space or newline") {
			t.Fatalf("incorrect prompt sent to agent: %s %v", sent, err)
		}
		runDir, err := latestRun(opts.outputDir)
		if err != nil || runDir == previous {
			t.Fatalf("new run: %s %v", runDir, err)
		}
		if previous != "" {
			data, err := os.ReadFile(filepath.Join(previous, "report.txt"))
			if err != nil || string(data) != string(previousReport) {
				t.Fatal("previous run report changed")
			}
		}
		models, err := loadResults(runDir)
		if err != nil || len(models) != 1 || len(models[0].Attempts) != 1 {
			t.Fatalf("passing submission should stop repairs: %+v %v", models, err)
		}
		item := models[0].Attempts[0]
		if item.Grade.Passed != item.Grade.Total || item.SourceSHA != fmt.Sprintf("%x", sha256.Sum256([]byte(source))) {
			t.Fatalf("saved grade/source mismatch: %+v", item)
		}
		attempt := filepath.Join(modelDirName(item.Model.ID), "attempt-01")
		if len(item.Grade.Evidence) != 22 || item.Grade.Evidence[0].Actual != item.Grade.Evidence[0].Expected {
			t.Fatal("saved execution evidence missing or incorrect")
		}
		for _, path := range []string{runDir + ".tar.gz",
			filepath.Join(runDir, "run.json"), filepath.Join(runDir, "grader.go.txt"),
			filepath.Join(runDir, "prompt.md"), filepath.Join(runDir, "report.md"),
			filepath.Join(runDir, modelDirName(item.Model.ID), "workspace"),
			filepath.Join(runDir, attempt, "main.go.txt"), filepath.Join(runDir, attempt, "grading"),
			filepath.Join(runDir, attempt, "feedback.md"), filepath.Join(runDir, attempt, "prompt.md"),
			filepath.Join(dir, "history.md")} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("unnecessary artifact retained: %s: %v", path, err)
			}
		}
		report, err := os.ReadFile(filepath.Join(runDir, "report.txt"))
		if err != nil || !strings.Contains(string(report), "37/37") || !strings.Contains(string(report), "Workload: v1") {
			t.Fatalf("report: %s %v", report, err)
		}
		record, err := loadRunRecord(runDir)
		if err != nil || record.Status != "completed" || record.Started.IsZero() || record.Finished.IsZero() || record.Machine == "" {
			t.Fatalf("run record: %+v %v", record, err)
		}
		if err := run(options{report: true, reportDir: runDir}); err != nil {
			t.Fatal(err)
		}
		regenerated, err := os.ReadFile(filepath.Join(runDir, "report.txt"))
		if err != nil || string(regenerated) != string(report) {
			t.Fatalf("regeneration changed report: %v", err)
		}
		program := filepath.Join(runDir, attempt, "program")
		saved, err := os.ReadFile(filepath.Join(program, "main.go"))
		if err != nil || string(saved) != source {
			t.Fatalf("saved source mismatch: %v", err)
		}
		binary := filepath.Join(t.TempDir(), "game")
		output, err := runCommand(t.Context(), program, "go", "build", "-o", binary, ".")
		if err != nil {
			t.Fatalf("saved program build: %v %s", err, output)
		}
		if e := executeScenario(t.Context(), binary, acceptanceScenarios()[0]); !e.Passed {
			t.Fatalf("saved program gameplay: %s", e.Error)
		}
		previous, previousReport = runDir, report
	}
	// Infrastructure failure must retain source without inventing a passing grade.
	t.Setenv("BENCHMARK_BAD_EVENTS", "yes")
	if err := run(opts); err == nil || !strings.Contains(err.Error(), "parsing OpenCode events") {
		t.Fatalf("malformed events: %v", err)
	}
	runDir, err := latestRun(opts.outputDir)
	if err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(runDir, modelDirName("org/model/BENCHMARK"), "attempt-01", "program", "main.go")
	if data, err := os.ReadFile(program); err != nil || string(data) != source {
		t.Fatalf("failed run lost source: %v", err)
	}
	if err := run(options{report: true, outputDir: opts.outputDir}); err != nil {
		t.Fatal("report regeneration:", err)
	}
	record, err := loadRunRecord(runDir)
	if err != nil || record.Status != "failed" || !strings.Contains(record.Error, "parsing OpenCode events") || record.Finished.IsZero() {
		t.Fatalf("regeneration lost failure metadata: %+v %v", record, err)
	}
	report, err := os.ReadFile(filepath.Join(runDir, "report.txt"))
	if err != nil || !strings.Contains(string(report), "No completed submissions.") || strings.Contains(string(report), "37/37") {
		t.Fatalf("failed report: %s %v", report, err)
	}
	// OpenCode can report an API failure while its process still exits zero.
	// Even a correct source snapshot must not turn that failure into a pass.
	t.Setenv("BENCHMARK_BAD_EVENTS", "")
	t.Setenv("BENCHMARK_API_ERROR", "yes")
	if err := run(opts); err == nil || !strings.Contains(err.Error(), "exceed context window") {
		t.Fatalf("API failure did not stop the workflow: %v", err)
	}
	runDir, err = latestRun(opts.outputDir)
	if err != nil {
		t.Fatal(err)
	}
	models, err := loadResults(runDir)
	if err != nil || len(models) != 1 || len(models[0].Attempts) != 1 {
		t.Fatalf("API error should stop further submissions: %+v %v", models, err)
	}
	failed := models[0].Attempts[0]
	if failed.OpenCode.ExitCode != 0 || !strings.Contains(failed.OpenCode.Error, "exceed context window") || failed.Grade.Total != 0 {
		t.Fatalf("API failure was lost or graded: %+v", failed)
	}
	program = filepath.Join(runDir, modelDirName(failed.Model.ID), "attempt-01", "program", "main.go")
	if data, err := os.ReadFile(program); err != nil || string(data) != source {
		t.Fatalf("API failure lost source snapshot: %v", err)
	}
	if err := run(options{report: true, reportDir: runDir}); err != nil {
		t.Fatal(err)
	}
	report, err = os.ReadFile(filepath.Join(runDir, "report.txt"))
	for _, want := range []string{"Workflow: failed", "Agent error", "Not graded", "exceed context window"} {
		if err != nil || !strings.Contains(string(report), want) {
			t.Errorf("API failure report missing %q: %s %v", want, report, err)
		}
	}
	if strings.Contains(string(report), "37/37") {
		t.Fatal("API failure received a passing grade")
	}
	if data, err := os.ReadFile(historyPath); err != nil || string(data) != history {
		t.Fatalf("benchmark modified manual history: %s %v", data, err)
	}
}

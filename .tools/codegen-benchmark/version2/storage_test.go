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
	script := `#!/bin/sh
set -eu
cp "$BENCHMARK_FIXTURE" tictactoe/main.go
if [ "${BENCHMARK_BAD_EVENTS:-}" = yes ]; then
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
	if err := os.WriteFile(config, []byte("version: 2\nmodels:\n  org/model/AGENT:\n    context-window: 8192\n    sampling-parameters:\n      max_tokens: 1024\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"org/model/AGENT"}]}`)
		}
	}))
	defer server.Close()
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	opts := options{host: server.URL, model: "all", configFile: config,
		promptFile: filepath.Join(root, ".tools/codegen-benchmark/version2/tic-tac-toe.md"),
		outputDir:  filepath.Join(dir, "runs"), attempts: 3, feedback: true, steps: 5, timeout: time.Minute}
	var previous string
	var previousReport []byte
	for range 2 {
		if err := run(opts); err != nil {
			t.Fatal(err)
		}
		runDir, err := latestRun(opts.outputDir)
		if err != nil || runDir == previous {
			t.Fatalf("new run: %s %v", runDir, err)
		}
		if previous != "" {
			data, err := os.ReadFile(filepath.Join(previous, "report.md"))
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
			filepath.Join(runDir, modelDirName(item.Model.ID), "workspace"),
			filepath.Join(runDir, attempt, "main.go.txt"), filepath.Join(runDir, attempt, "grading"),
			filepath.Join(runDir, attempt, "feedback.md"), filepath.Join(runDir, attempt, "prompt.md")} {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("unnecessary artifact retained: %s: %v", path, err)
			}
		}
		report, err := os.ReadFile(filepath.Join(runDir, "report.md"))
		if err != nil || !strings.Contains(string(report), "37/37") || !strings.Contains(string(report), item.SourceSHA) {
			t.Fatalf("report: %s %v", report, err)
		}
		record, err := loadRunRecord(runDir)
		if err != nil || record.Status != "completed" || record.Started.IsZero() || record.Finished.IsZero() {
			t.Fatalf("run record: %+v %v", record, err)
		}
		if err := run(options{report: true, reportDir: runDir}); err != nil {
			t.Fatal(err)
		}
		regenerated, err := loadRunRecord(runDir)
		if err != nil || regenerated != record {
			t.Fatalf("regeneration changed run metadata: %+v %v", regenerated, err)
		}
		report, err = os.ReadFile(filepath.Join(runDir, "report.md"))
		if err != nil {
			t.Fatal(err)
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
		e := executeScenario(t.Context(), binary, acceptanceScenarios()[0])
		if !e.Passed {
			t.Fatalf("saved program gameplay: %s", e.Error)
		}
		previous = runDir
		previousReport = report
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
	program := filepath.Join(runDir, modelDirName("org/model/AGENT"), "attempt-01", "program", "main.go")
	data, err := os.ReadFile(program)
	if err != nil || string(data) != source {
		t.Fatalf("failed run lost source: %v", err)
	}
	report, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil || !strings.Contains(string(report), "**failed**") || strings.Contains(string(report), "37/37") {
		t.Fatalf("failed report: %s %v", report, err)
	}
	if _, err := os.Stat(filepath.Join(runDir, modelDirName("org/model/AGENT"), "workspace")); !os.IsNotExist(err) {
		t.Fatal("failed run retained temporary workspace:", err)
	}
	if err := run(options{report: true, outputDir: opts.outputDir}); err != nil {
		t.Fatal("report regeneration:", err)
	}
	record, err := loadRunRecord(runDir)
	if err != nil || record.Status != "failed" || !strings.Contains(record.Error, "parsing OpenCode events") || record.Finished.IsZero() {
		t.Fatalf("regeneration lost failure metadata: %+v %v", record, err)
	}
}

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fake agent needs all three submissions for the basic game, then receives
// a fresh three-submission budget for undo without losing its workspace or session.
func TestUndoSubmissionsHaveIndependentBudget(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []string{"pass", "fail", "agent-error"} {
		t.Run(outcome, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("BENCHMARK_CAPTURE_DIR", dir)
			t.Setenv("BENCHMARK_V1", filepath.Join(root, ".tools/codegen-benchmark/version2/testdata/correct.go"))
			t.Setenv("BENCHMARK_V2", filepath.Join(root, ".tools/codegen-benchmark/version2/testdata/undo/main.go"))
			t.Setenv("BENCHMARK_UNDO_OUTCOME", outcome)
			script := `#!/bin/sh
set -eu
n=0
if [ -f "$HOME/submissions" ]; then n=$(cat "$HOME/submissions"); fi
n=$((n+1))
printf '%s' "$n" > "$HOME/submissions"
if [ "$n" -gt 1 ]; then
  case " $* " in *' --continue '*) ;; *) exit 1 ;; esac
fi
cat > "$BENCHMARK_CAPTURE_DIR/message-$n"
case "$n" in
  1) grep -q 'original v1 task' "$BENCHMARK_CAPTURE_DIR/message-$n" ;;
  4) grep -q 'undo extension' "$BENCHMARK_CAPTURE_DIR/message-$n"
     cmp "$BENCHMARK_V1" tictactoe/main.go ;;
  *) grep -q 'go-parse' "$BENCHMARK_CAPTURE_DIR/message-$n" ;;
esac
if [ "$n" = 4 ] && [ "$BENCHMARK_UNDO_OUTCOME" = agent-error ]; then
  printf '%s\n' '{"type":"error","error":{"name":"APIError","data":{"message":"inference unavailable"}}}'
  exit 0
fi
case "$n" in
  3) cp "$BENCHMARK_V1" tictactoe/main.go ;;
  6) if [ "$BENCHMARK_UNDO_OUTCOME" = pass ]; then
       cp "$BENCHMARK_V2" tictactoe/main.go
     else
       printf 'package main\nfunc broken\n' > tictactoe/main.go
     fi ;;
  *) printf 'package main\nfunc broken\n' > tictactoe/main.go ;;
esac
printf '%s\n' '{"type":"step_finish","part":{"tokens":{"input":17,"output":9}}}'
`
			if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			runDir := t.TempDir()
			record := runRecord{Workload: workloadVersion, Steps: basicGameStep + ", " + undoMoveStep, Started: time.Now(), Status: "running"}
			if err := generateReport(runDir, &record); err != nil {
				t.Fatal(err)
			}
			model := benchmarkModel{ID: "org/model/BENCHMARK", ContextWindow: 131072, OutputLimit: 32000}
			tasks := []benchmarkStep{
				{name: basicGameStep, prompt: "original v1 task", grade: gradeProgram},
				{name: undoMoveStep, prompt: "undo extension", grade: gradeUndoProgram},
			}
			runErr := runModel(root, runDir, server.URL, tasks, model, options{attempts: 3, feedback: true, steps: 5, timeout: time.Minute})
			wantCount := 6
			if outcome == "agent-error" {
				wantCount = 4
				if runErr == nil || !strings.Contains(runErr.Error(), "inference unavailable") {
					t.Fatalf("undo infrastructure failure was lost: %v", runErr)
				}
			} else if runErr != nil {
				t.Fatal(runErr)
			}
			if err := generateReport(runDir, nil); err != nil {
				t.Fatal(err)
			}
			models, err := loadResults(runDir)
			if err != nil || len(models) != 1 || len(models[0].Attempts) != wantCount {
				t.Fatalf("independent budgets: %+v %v", models, err)
			}
			for i, item := range models[0].Attempts {
				wantStep := basicGameStep
				if i >= 3 {
					wantStep = undoMoveStep
				}
				if item.Attempt != i+1 || item.Step != wantStep || (item.Repair != "") != (i%3 != 0) {
					t.Fatalf("incorrect step or repair metadata: %+v", item)
				}
			}
			basic, undo := models[0].Attempts[2], models[0].Attempts[wantCount-1]
			if basic.Grade.Passed != basic.Grade.Total || (undo.Grade.Total > 0 && undo.Grade.Passed == undo.Grade.Total) != (outcome == "pass") {
				t.Fatalf("step outcomes: basic=%+v undo=%+v", basic.Grade, undo.Grade)
			}
			report, err := os.ReadFile(filepath.Join(runDir, "report.txt"))
			if err != nil {
				t.Fatal(err)
			}
			plain := strings.Join(strings.Fields(string(report)), " ")
			wantUndo := "Step2 (Undo move) 3 Not passed"
			if outcome == "pass" {
				wantUndo = "Step2 (Undo move) 3 Passed on submission 3"
			}
			if outcome == "agent-error" {
				wantUndo = "Step2 (Undo move) 1 Agent error Not graded"
			}
			for _, want := range []string{"Workload: v1", "Step1 (basic game) 3 Passed on submission 3 37/37", wantUndo} {
				if !strings.Contains(plain, want) {
					t.Fatalf("report conflated workload outcomes: missing %q in %s", want, report)
				}
			}
		})
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadModelsSelectsOnlyBenchmarkProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools_model_config.yaml")
	config := `version: 2
models:
  org/model/AGENT:
    context-window: 4096
  org/model/TOOLS:
    context-window: 2048
  org/model/BENCHMARK:
    nseq-max: 1
    context-window: 32768
    sampling-parameters:
      max_tokens: 8192
`
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	models, err := loadModels(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "org/model/BENCHMARK" || models[0].ContextWindow != 32768 || models[0].OutputLimit != 8192 {
		t.Fatalf("benchmark profiles: %+v", models)
	}
	if _, err := selectModels(models, "org/model/AGENT"); err == nil {
		t.Fatal("version2 accepted an /AGENT profile")
	}
}

func TestSelectModels(t *testing.T) {
	configured := []benchmarkModel{
		{ID: "org/one/BENCHMARK"},
		{ID: "org/two/BENCHMARK"},
	}

	selected, err := selectModels(configured, "org/two/BENCHMARK,org/one/BENCHMARK,org/two/BENCHMARK")
	if err != nil {
		t.Fatalf("selectModels: %v", err)
	}
	if got, want := len(selected), 2; got != want {
		t.Fatalf("selected models: got %d, want %d", got, want)
	}
	if got, want := selected[0].ID, "org/two/BENCHMARK"; got != want {
		t.Errorf("first model: got %q, want %q", got, want)
	}

	if _, err := selectModels(configured, "org/one"); err == nil {
		t.Fatal("model without /BENCHMARK: got nil error, want error")
	}
	if _, err := selectModels(configured, "org/missing/BENCHMARK"); err == nil {
		t.Fatal("unconfigured model: got nil error, want error")
	}
}

func TestOpenCodeConfig(t *testing.T) {
	model := benchmarkModel{ID: "org/model/BENCHMARK", ContextWindow: 65536, OutputLimit: 8192}
	data, err := openCodeConfig("http://localhost:11435", model, 12)
	if err != nil {
		t.Fatalf("openCodeConfig: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if got, want := config["model"], "kronk/org/model/BENCHMARK"; got != want {
		t.Errorf("model: got %v, want %v", got, want)
	}
	if got, want := config["snapshot"], false; got != want {
		t.Errorf("snapshot: got %v, want %v", got, want)
	}
	providers := config["provider"].(map[string]any)
	kronk := providers["kronk"].(map[string]any)
	options := kronk["options"].(map[string]any)
	if got, want := options["baseURL"], "http://localhost:11435/v1"; got != want {
		t.Errorf("baseURL: got %v, want %v", got, want)
	}
}

func TestOpenCodeAcceptsConfig(t *testing.T) {
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode is not installed")
	}
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot: %v", err)
	}
	model := benchmarkModel{ID: "org/model/BENCHMARK", ContextWindow: 65536, OutputLimit: 8192}
	config, err := openCodeConfig("http://localhost:11435", model, 12)
	if err != nil {
		t.Fatalf("openCodeConfig: %v", err)
	}
	env, err := prepareOpenCodeEnvironment(root, filepath.Join(t.TempDir(), "attempt"), config)
	if err != nil {
		t.Fatalf("prepareOpenCodeEnvironment: %v", err)
	}

	cmd := exec.Command("opencode", "debug", "config", "--pure")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("opencode debug config: %v\n%s", err, output)
	}
	var resolved map[string]any
	if err := json.Unmarshal(output, &resolved); err != nil {
		t.Fatalf("decode resolved config: %v\n%s", err, output)
	}
	if got, want := resolved["model"], "kronk/org/model/BENCHMARK"; got != want {
		t.Errorf("resolved model: got %v, want %v", got, want)
	}
	if got, want := resolved["snapshot"], false; got != want {
		t.Errorf("resolved snapshot: got %v, want %v", got, want)
	}
}

func TestPrepareAttemptDirectoryCreatesRepositoryBoundary(t *testing.T) {
	outer := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", outer)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initialize outer repository: %v\n%s", err, output)
	}
	dir := filepath.Join(outer, "output", "attempt-01")

	programDir, err := prepareAttemptDirectory(dir)
	if err != nil {
		t.Fatalf("prepareAttemptDirectory: %v", err)
	}
	if got, want := programDir, filepath.Join(dir, "tictactoe"); got != want {
		t.Errorf("program directory: got %q, want %q", got, want)
	}

	cmd = exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = programDir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatalf("resolve git root: %v", err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolve attempt directory: %v", err)
	}
	if got != want {
		t.Errorf("git root: got %q, want %q", got, want)
	}
}

func TestPrepareRunDirPreservesPreviousRuns(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "output")
	oldAttempt := filepath.Join(dir, "old-model", "attempt-01")
	if err := os.MkdirAll(oldAttempt, 0o755); err != nil {
		t.Fatalf("create old attempt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldAttempt, "result.json"), []byte("old result"), 0o644); err != nil {
		t.Fatalf("write old result: %v", err)
	}

	got, err := prepareRunDir(dir)
	if err != nil {
		t.Fatalf("prepareRunDir: %v", err)
	}
	second, err := prepareRunDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got == second || filepath.Dir(filepath.Dir(got)) != dir {
		t.Fatalf("run directories not unique and date-grouped: %s %s", got, second)
	}
	data, err := os.ReadFile(filepath.Join(oldAttempt, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old result" {
		t.Fatal("previous output was changed")
	}
}

func TestWarmAndUnloadModel(t *testing.T) {
	const modelID = "org/model/BENCHMARK"
	t.Setenv("KRONK_TOKEN", "benchmark-token")

	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if got, want := r.Method, http.MethodPost; got != want {
			t.Errorf("method: got %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer benchmark-token"; got != want {
			t.Errorf("authorization: got %q, want %q", got, want)
		}

		switch r.URL.Path {
		case "/v1/chat/completions":
			var request struct {
				Model    string `json:"model"`
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
				MaxTokens      int  `json:"max_tokens"`
				EnableThinking bool `json:"enable_thinking"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode warm request: %v", err)
			}
			if got, want := request.Model, modelID; got != want {
				t.Errorf("warm model: got %q, want %q", got, want)
			}
			if got, want := len(request.Messages), 1; got != want {
				t.Errorf("warm messages: got %d, want %d", got, want)
			} else {
				if got, want := request.Messages[0].Role, "user"; got != want {
					t.Errorf("warm role: got %q, want %q", got, want)
				}
				if got, want := request.Messages[0].Content, "hello model"; got != want {
					t.Errorf("warm prompt: got %q, want %q", got, want)
				}
			}
			if got, want := request.MaxTokens, 1; got != want {
				t.Errorf("warm max tokens: got %d, want %d", got, want)
			}
			w.WriteHeader(http.StatusOK)
		case "/v1/kronk/models/unload":
			var request struct {
				ID string `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode unload request: %v", err)
			}
			if got, want := request.ID, modelID; got != want {
				t.Errorf("unload model: got %q, want %q", got, want)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := warmModel(context.Background(), server.URL, modelID); err != nil {
		t.Fatalf("warmModel: %v", err)
	}
	if err := unloadModel(context.Background(), server.URL, modelID); err != nil {
		t.Fatalf("unloadModel: %v", err)
	}
	if got, want := strings.Join(requests, ","), "/v1/chat/completions,/v1/kronk/models/unload"; got != want {
		t.Errorf("requests: got %q, want %q", got, want)
	}
}

func TestParseAgentResult(t *testing.T) {
	events := strings.Join([]string{
		`{"type":"tool_use","part":{"type":"tool","tool":"bash"}}`,
		`{"type":"step_finish","part":{"type":"step-finish","tokens":{"input":100,"output":20,"reasoning":5,"cache":{"read":40}}}}`,
	}, "\n")

	result, err := parseAgentResult([]byte(events))
	if err != nil {
		t.Fatalf("parseAgentResult: %v", err)
	}
	if got, want := result.Turns, 1; got != want {
		t.Errorf("turns: got %d, want %d", got, want)
	}
	if got, want := result.ToolCalls, 1; got != want {
		t.Errorf("tool calls: got %d, want %d", got, want)
	}
	if got, want := result.Usage, (tokenUsage{Input: 100, Output: 20, Reasoning: 5, CacheRead: 40}); got != want {
		t.Errorf("usage: got %+v, want %+v", got, want)
	}
}

func TestGradeProgramMissingSource(t *testing.T) {
	grade := gradeProgram(t.Context(), "")
	if got, want := grade.Passed, 0; got != want {
		t.Errorf("passed checks: got %d, want %d", got, want)
	}
	if got, want := grade.Total, 37; got != want {
		t.Errorf("total checks: got %d, want %d", got, want)
	}
}

func TestRenderReportShowsFinalOutcomeAndCumulativeWork(t *testing.T) {
	report := renderReport("run", []modelResults{{
		Model: benchmarkModel{ID: "org/model/BENCHMARK"},
		Attempts: []result{
			{Attempt: 1, Grade: gradeResult{Passed: 12, Total: 20}, OpenCode: agentResult{WallTime: 3 * time.Second, Turns: 2, ToolCalls: 4, Usage: tokenUsage{Input: 100, Output: 30}}},
			{Attempt: 2, Grade: gradeResult{Passed: 20, Total: 20, Checks: []gradeCheck{{Name: "go-build", Passed: true}}}, OpenCode: agentResult{WallTime: 7 * time.Second, Turns: 5, ToolCalls: 6, Usage: tokenUsage{Input: 200, Output: 50}}},
		},
	}, {
		Model:    benchmarkModel{ID: "org/failed/BENCHMARK"},
		Attempts: []result{{Attempt: 1, Grade: gradeResult{Passed: 19, Total: 20}}},
	}}, runRecord{})

	for _, want := range []string{
		"| `org/model/BENCHMARK` | 2 | 2 | 20/20 (100.00%) | true | 0 |",
		"| `org/model/BENCHMARK` | 10.00 s | 7 | 10 | 300 | 80 | 0 | 0 |",
		"| `org/failed/BENCHMARK` | 1 | Not passed | 19/20 (95.00%) | false | 0 |",
		"each agent turn's final OpenCode usage event",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report: missing %q", want)
		}
	}
	if strings.Contains(report, "Cache write") {
		t.Error("report: contains unsupported cache-write metric")
	}
}

func TestRunSubmissions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		feedback bool
		passAt   int
		want     int
	}{
		{"first submission passes", true, 1, 1},
		{"repair passes", true, 2, 2},
		{"last submission passes", true, 3, 3},
		{"exhausted budget", true, 0, 3},
		{"feedback disabled", false, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := runSubmissions("original requirements", 3, tc.feedback, func(attempt int, prompt string) (result, error) {
				calls++
				if attempt == 1 && prompt != "original requirements" {
					t.Fatalf("initial prompt: %q", prompt)
				}
				if attempt == 2 && (!strings.Contains(prompt, "first-failure") || strings.Contains(prompt, "already-passed")) {
					t.Fatalf("repair feedback: %q", prompt)
				}
				if attempt == 3 && (!strings.Contains(prompt, "second-failure") || strings.Contains(prompt, "first-failure")) {
					t.Fatalf("feedback must use the latest grade: %q", prompt)
				}
				grade := gradeResult{Passed: 19, Total: 20, Checks: []gradeCheck{{Name: "already-passed", Passed: true}}}
				if attempt == 1 {
					grade.Checks = append(grade.Checks, gradeCheck{Name: "first-failure", Detail: "restore final board"})
				} else {
					grade.Checks = append(grade.Checks, gradeCheck{Name: "second-failure", Detail: "preserve replay scores"})
				}
				if attempt == tc.passAt {
					grade.Passed = 20
				}
				return result{Grade: grade}, nil
			})
			if err != nil || calls != tc.want {
				t.Fatalf("calls=%d want=%d error=%v", calls, tc.want, err)
			}
		})
	}
	broken := errors.New("cannot save submission")
	calls := 0
	err := runSubmissions("original", 3, true, func(int, string) (result, error) {
		calls++
		return result{}, broken
	})
	if !errors.Is(err, broken) || calls != 1 {
		t.Fatalf("infrastructure failure: calls=%d error=%v", calls, err)
	}
}

func TestSubmissionLimit(t *testing.T) {
	for _, limit := range []int{0, 4} {
		if err := run(options{attempts: limit}); err == nil || !strings.Contains(err.Error(), "between 1 and 3") {
			t.Errorf("limit %d: got %v", limit, err)
		}
	}
}

// This fake CLI never contacts a model; it checks actual process arguments,
// retained workspace/home, and source snapshots across two submissions.
func TestRunAttemptResumesWorkspace(t *testing.T) {
	binDir := t.TempDir()
	script := `#!/bin/sh
set -eu
continued=false
for arg do
  if [ "$arg" = '--continue' ]; then continued=true; fi
done
if [ -f "$HOME/submitted" ]; then
  [ "$continued" = true ]
  grep -q 'first-version' tictactoe/main.go
  printf 'package main\nfunc second-version\n' > tictactoe/main.go
else
  [ "$continued" = false ]
  touch "$HOME/submitted"
  printf 'package main\nfunc first-version\n' > tictactoe/main.go
fi
printf '%s\n' '{"type":"step_finish","part":{"tokens":{"input":17,"output":9}}}'
`
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	model := benchmarkModel{ID: "org/model/BENCHMARK", ContextWindow: 8192, OutputLimit: 1024}
	for attempt := 1; attempt <= 2; attempt++ {
		item, err := runAttempt(root, runDir, "http://unused.invalid", "feedback", model, attempt, options{steps: 5, timeout: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		if item.OpenCode.ExitCode != 0 || item.OpenCode.Usage.Input != 17 || item.Grade.Passed != 1 {
			t.Fatalf("submission %d: %+v", attempt, item)
		}
	}
	modelDir := filepath.Join(runDir, modelDirName(model.ID))
	first, err := os.ReadFile(filepath.Join(modelDir, "attempt-01", "program", "main.go"))
	if err != nil || !strings.Contains(string(first), "first-version") {
		t.Fatalf("first snapshot: %s %v", first, err)
	}
	second, err := os.ReadFile(filepath.Join(modelDir, "attempt-02", "program", "main.go"))
	if err != nil || !strings.Contains(string(second), "second-version") {
		t.Fatalf("second snapshot: %s %v", second, err)
	}
	loaded, err := loadResults(runDir)
	if err != nil || len(loaded) != 1 || len(loaded[0].Attempts) != 2 {
		t.Fatalf("saved submissions: %+v %v", loaded, err)
	}
	if loaded[0].Attempts[0].Repair != "" || loaded[0].Attempts[1].Repair != "feedback" {
		t.Fatalf("saved repair messages: %+v", loaded[0].Attempts)
	}
	feedback := graderFeedback(loaded[0].Attempts[1].Grade)
	if !strings.Contains(feedback, "go-parse") || !strings.Contains(feedback, "1/37") {
		t.Fatalf("saved grader diagnostics: %s", feedback)
	}
}

func TestRunAttemptTimeout(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte("#!/bin/sh\nexec sleep 20\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	model := benchmarkModel{ID: "org/model/BENCHMARK", ContextWindow: 8192, OutputLimit: 1024}
	item, err := runAttempt(root, t.TempDir(), "http://unused.invalid", "prompt", model, 1, options{steps: 5, timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if item.OpenCode.Error != context.DeadlineExceeded.Error() || item.OpenCode.ExitCode == 0 {
		t.Fatalf("timeout not recorded: %+v", item.OpenCode)
	}
	if item.OpenCode.WallTime > 2*time.Second {
		t.Fatalf("agent exceeded timeout: %s", item.OpenCode.WallTime)
	}
	if item.Grade.Passed == item.Grade.Total {
		t.Fatal("missing program passed grading")
	}
}

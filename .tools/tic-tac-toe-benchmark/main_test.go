package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		t.Fatal("benchmark accepted an /AGENT profile")
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
	version, err := runCommand(t.Context(), programDir, "go", "list", "-m", "-f", "{{.GoVersion}}")
	if err != nil || strings.TrimSpace(version) != "1.27.0" {
		t.Fatalf("workspace Go version: got %q, want 1.27.0: %v", version, err)
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

func TestServerVersions(t *testing.T) {
	for _, tc := range []struct {
		name          string
		libraryStatus int
		libraryBody   string
		wantError     bool
	}{
		{"installed, not latest", http.StatusOK, `{"current":"b12283","latest":"b99999"}`, false},
		{"missing installed version", http.StatusOK, `{"latest":"b99999"}`, true},
		{"access denied", http.StatusForbidden, `{}`, true},
		{"invalid metadata", http.StatusOK, `{`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KRONK_TOKEN", "test-token")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("metadata request: %s %s", r.Method, r.Header.Get("Authorization"))
				}
				switch r.URL.Path {
				case "/v1/liveness":
					_, _ = w.Write([]byte(`{"build":"1.33.4+dev"}`))
				case "/v1/kronk/libs":
					w.WriteHeader(tc.libraryStatus)
					_, _ = w.Write([]byte(tc.libraryBody))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			kronk, llama, err := serverVersions(t.Context(), server.URL)
			if tc.wantError {
				if err == nil {
					t.Fatal("unavailable metadata must not be recorded as a successful version lookup")
				}
				return
			}
			if err != nil || kronk != "1.33.4+dev" || llama != "b12283" {
				t.Fatalf("server versions: Kronk=%q llama.cpp=%q error=%v", kronk, llama, err)
			}
		})
	}
}

func TestParseAgentResult(t *testing.T) {
	events := strings.Join([]string{
		`{"type":"tool_use","part":{"type":"tool","tool":"bash","state":{"status":"error","error":"permission denied"}}}`,
		`{"type":"step_finish","part":{"type":"step-finish","tokens":{"input":100,"output":20,"reasoning":5,"cache":{"read":40}}}}`,
	}, "\n")

	result, err := parseAgentResult([]byte(events))
	if err != nil {
		t.Fatalf("parseAgentResult: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("recoverable tool failure treated as an agent error: %s", result.Error)
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
			{Attempt: 1, Grade: gradeResult{Passed: 12, Total: 20}, OpenCode: agentResult{WallTime: 3 * time.Second, Turns: 2, ToolCalls: 4, Usage: tokenUsage{Input: 100, Output: 30, Reasoning: 7, CacheRead: 40}}},
			{Attempt: 2, Grade: gradeResult{Passed: 20, Total: 20, Checks: []gradeCheck{{Name: "go-build", Passed: true}}}, OpenCode: agentResult{WallTime: 7 * time.Second, Turns: 5, ToolCalls: 6, Usage: tokenUsage{Input: 200, Output: 50, Reasoning: 11, CacheRead: 80}}},
			{Step: undoMoveStep, Attempt: 3, Grade: gradeResult{Passed: 7, Total: 9}, OpenCode: agentResult{WallTime: 11 * time.Second, Usage: tokenUsage{Input: 13, Output: 17, Reasoning: 19, CacheRead: 23}}},
		},
	}, {
		Model:    benchmarkModel{ID: "org/failed/BENCHMARK"},
		Attempts: []result{{Attempt: 1, Grade: gradeResult{Passed: 19, Total: 20}}},
	}, {
		Model:    benchmarkModel{ID: "org/no-generation/BENCHMARK"},
		Attempts: []result{{Attempt: 1, Grade: gradeResult{Passed: 19, Total: 20}, OpenCode: agentResult{Usage: tokenUsage{Input: 29, CacheRead: 31}}}},
	}}, runRecord{Steps: basicGameStep + ", " + undoMoveStep})

	plain := strings.Join(strings.Fields(report), " ")
	for _, want := range []string{
		"Workload: v1",
		"Model Step 1 (Basic game) Step 2 (Undo move)",
		"org/model/BENCHMARK Passed (attempts 2, checks 20/20, time 10s, generated-toks: 98) Failed (attempts 1, checks 7/9, time 11s, generated-toks: 36)",
		"org/failed/BENCHMARK Failed (attempts 1, checks 19/20, time 0s, generated-toks: not reported) Not reached",
		"org/no-generation/BENCHMARK Failed (attempts 1, checks 19/20, time 0s, generated-toks: 0) Not reached",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("report: missing %q in %s", want, report)
		}
	}
	for _, model := range []string{"org/model/BENCHMARK", "org/failed/BENCHMARK"} {
		if strings.Count(report, "\n"+model+" ") != 1 {
			t.Errorf("report must contain one row for %s: %s", model, report)
		}
	}
	for _, unwanted := range []string{"```", "|", "#", "Input tokens", "Failure", "SHA-256"} {
		if strings.Contains(report, unwanted) {
			t.Errorf("report contains unwanted detail %q", unwanted)
		}
	}
}

func TestGraderFeedbackRepeatedScenarioFailures(t *testing.T) {
	grade := gradeResult{Total: 7}
	finding := gradeFinding{Kind: "interaction", Location: "scenario-X-line-1", Step: 2,
		Input: "1\n", Expected: "\x1b[32mX\x1b[0m\n", Actual: "\x1b[32mO\x1b[0m\n", Detail: "unexpected game state"}
	grade.add(finding.Location, false, finding.Detail, finding, finding)
	for _, change := range []func(*gradeFinding){
		func(f *gradeFinding) { f.Location = "scenario-O-line-1" },
		func(f *gradeFinding) { f.Input = "2\n" },
		func(f *gradeFinding) { f.Step = 3 },
	} {
		distinct := finding
		change(&distinct)
		grade.Findings = append(grade.Findings, distinct)
	}
	grade.add("go-build", false, "build failed\ncompiler diagnostic\n")
	grade.add("go-vet", false, "build failed\nvet diagnostic\n")
	grade.add("scenario-passed", true, "")
	before, err := json.Marshal(grade)
	if err != nil {
		t.Fatal(err)
	}
	feedback := graderFeedback(grade)
	for _, want := range []string{
		"1/7 checks. Failed: 3. Not reached: 3.", "(2 identical observations)",
		"scenario-O-line-1 [interaction], step 2", "scenario-X-line-1 [interaction], step 3",
		`Input sent: "1\n"`, `Input sent: "2\n"`,
		`Expected: "\x1b[32mX\x1b[0m\n"`, `Actual: "\x1b[32mO\x1b[0m\n"`,
		"Diagnostic: build failed\ncompiler diagnostic\n", "Diagnostic: build failed\nvet diagnostic\n",
	} {
		if !strings.Contains(feedback, want) {
			t.Errorf("feedback omitted %q: %s", want, feedback)
		}
	}
	if strings.Count(feedback, "Diagnostic: unexpected game state") != 4 || strings.Contains(feedback, "scenario-passed") {
		t.Fatalf("feedback merged distinct evidence or included a passing check: %s", feedback)
	}
	after, err := json.Marshal(grade)
	if err != nil || string(after) != string(before) {
		t.Fatal("feedback changed the recorded grade")
	}
}

func TestGraderFeedbackPreservesLongEvidence(t *testing.T) {
	grade := gradeResult{Total: 3}
	grade.add("long-output", false, "", gradeFinding{Kind: "transcript", Location: "long-output",
		Expected: strings.Repeat("\x1b[32mX\x1b[0m\n", 2500), Actual: strings.Repeat("\x1b[32mO\x1b[0m\n", 2500)})
	diagnostic := strings.Repeat("compiler diagnostic\n", 200)
	grade.add("go-build", false, diagnostic)
	feedback := graderFeedback(grade)
	for _, want := range []string{
		"Not reached: 1", diagnostic,
		`Expected: "` + strings.Repeat(`\x1b[32mX\x1b[0m\n`, 2500) + `"`,
		`Actual: "` + strings.Repeat(`\x1b[32mO\x1b[0m\n`, 2500) + `"`,
	} {
		if !strings.Contains(feedback, want) {
			t.Fatalf("long evidence or diagnostics were lost: expected %d-byte field", len(want))
		}
	}
}

func TestGraderFeedbackExcerpts(t *testing.T) {
	prefix, middle, suffix := strings.Repeat("p", 80), strings.Repeat("m", 120), strings.Repeat("s", 80)
	for _, tc := range []struct {
		name, expected, actual string
		want                   []string
		excerpts               int
	}{
		{"separated changes", prefix + "X" + middle + "Y" + suffix, prefix + "O" + middle + "Z" + suffix,
			[]string{`Expected: "` + strings.Repeat("p", 24) + "X" + strings.Repeat("m", 24) + `"`,
				`Actual: "` + strings.Repeat("m", 24) + "Z" + strings.Repeat("s", 24) + `"`,
				"Omitted prefix=true suffix=true", "#1 E56/A56", "#1 E177/A177"}, 2},
		{"overlapping context", prefix + "XnearY" + suffix, prefix + "OnearZ" + suffix,
			[]string{`Expected: "` + strings.Repeat("p", 24) + "XnearY" + strings.Repeat("s", 24) + `"`,
				`Actual: "` + strings.Repeat("p", 24) + "OnearZ" + strings.Repeat("s", 24) + `"`}, 1},
		{"insertions and deletions", prefix + "\x1b[1;31mX \n" + middle + "\n" + suffix, prefix + "\x1b[31mX\n" + middle + " extra\t" + suffix,
			[]string{`\x1b[1;31mX \n`, `\x1b[31mX\n`, ` extra\t`}, 2},
		{"empty actual", "missing\n", "", []string{`Expected: "missing\n"`, `Actual: ""`, "Omitted prefix=false suffix=false"}, 1},
		{"empty expected", "", "extra\n", []string{`Expected: ""`, `Actual: "extra\n"`}, 1},
		{"unchanged execution output", "same", "same", []string{"Diagnostic: process exited with status 1"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			grade := gradeResult{Total: 1}
			grade.add("scenario-test", false, "", gradeFinding{Kind: "transcript", Location: "scenario-test",
				Expected: tc.expected, Actual: tc.actual, Detail: "process exited with status 1"})
			feedback := graderFeedback(grade)
			for _, want := range tc.want {
				if !strings.Contains(feedback, want) {
					t.Errorf("feedback omitted %q: %s", want, feedback)
				}
			}
			if got := strings.Count(feedback, "\nExcerpt "); got != tc.excerpts {
				t.Errorf("got %d excerpts, want %d: %s", got, tc.excerpts, feedback)
			}
			if strings.Contains(feedback, prefix) || strings.Contains(feedback, middle) || strings.Contains(feedback, suffix) {
				t.Fatalf("feedback repeated distant matching sections: %s", feedback)
			}
		})
	}
}

func TestGraderFeedbackCollapsePreservesOccurrencesAndBoundaries(t *testing.T) {
	expected := strings.Repeat("p", 24) + "X" + strings.Repeat("s", 24)
	actual := strings.Repeat("p", 24) + "O" + strings.Repeat("s", 24)
	grade := gradeResult{Total: 3}
	for i, padding := range []string{"", "", "frame"} {
		name := fmt.Sprintf("scenario-%d", i+1)
		grade.add(name, false, "", gradeFinding{Kind: "transcript", Location: name,
			Input: fmt.Sprintf("%d\n", i+1), Expected: padding + expected + padding, Actual: padding + actual + padding})
	}
	feedback := graderFeedback(grade)
	for _, want := range []string{"#1 E0/A0, #2 E0/A0", "#3 E5/A5", "Omitted prefix=false suffix=false", "Omitted prefix=true suffix=true",
		`Input sent: "1\n"`, `Input sent: "2\n"`, `Input sent: "3\n"`} {
		if !strings.Contains(feedback, want) {
			t.Errorf("feedback lost occurrence identity or boundary: missing %q in %s", want, feedback)
		}
	}
	if strings.Count(feedback, "\nExcerpt ") != 2 || strings.Count(feedback, "Expected: ") != 2 {
		t.Fatalf("identical excerpts were not collapsed, or differing boundaries were merged: %s", feedback)
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
				grade := gradeResult{Passed: 18, Total: 20}
				grade.add("already-passed", true, "")
				if attempt == 1 {
					grade.add("first-failure", false, "restore final board")
				} else {
					grade.add("second-failure", false, "preserve replay scores")
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

// This fake CLI never contacts a model; it checks process arguments, exact stdin
// messages, retained workspace/home, and source snapshots across two submissions.
func TestRunModelResumesWorkspace(t *testing.T) {
	binDir := t.TempDir()
	script := `#!/bin/sh
set -eu
continued=false
thinking=false
for arg do
  if [ "$arg" = '--continue' ]; then continued=true; fi
  if [ "$arg" = '--thinking' ]; then thinking=true; fi
done
[ "$thinking" = true ]
if [ -f "$HOME/submitted" ]; then
  [ "$continued" = true ]
  cat > "$CAPTURE_DIR/message-02"
  grep -q 'go-parse' "$CAPTURE_DIR/message-02"
  grep -q 'first-version' tictactoe/main.go
  printf 'package main\nfunc second-version\n' > tictactoe/main.go
else
  [ "$continued" = false ]
  cat > "$CAPTURE_DIR/message-01"
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
	if err := generateReport(runDir, &runRecord{Workload: workloadVersion, Steps: basicGameStep + ", " + undoMoveStep, Started: time.Now(), Status: "running"}); err != nil {
		t.Fatal(err)
	}
	model := benchmarkModel{ID: "org/model/BENCHMARK", ContextWindow: 8192, OutputLimit: 1024}
	modelDir := filepath.Join(runDir, modelDirName(model.ID))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv("CAPTURE_DIR", binDir)
	prompt := "original task\n" + strings.Repeat("evidence with spaces, \"quotes\" and \\x1b[32m\n", 5000)
	if err := runModel(root, runDir, server.URL, []benchmarkStep{
		{name: basicGameStep, prompt: prompt, grade: gradeProgram},
		{name: undoMoveStep, prompt: "undo extension", grade: gradeUndoProgram},
	}, model,
		options{attempts: 2, feedback: true, steps: 5, timeout: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(modelDir, "workspace")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("model workspace not cleaned up: %v", err)
	}
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
	for _, item := range loaded[0].Attempts {
		if item.Step != basicGameStep || item.OpenCode.ExitCode != 0 || item.OpenCode.Usage.Input != 17 || item.Grade.Passed != 1 {
			t.Fatalf("submission %d: %+v", item.Attempt, item)
		}
	}
	if loaded[0].Attempts[0].Repair != "" || !strings.Contains(loaded[0].Attempts[1].Repair, "go-parse") {
		t.Fatalf("saved repair messages: %+v", loaded[0].Attempts)
	}
	for i, want := range []string{prompt, loaded[0].Attempts[1].Repair} {
		name := []string{"message-01", "message-02"}[i]
		got, err := os.ReadFile(filepath.Join(binDir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s stdin changed: got %d bytes, want %d, error %v", name, len(got), len(want), err)
		}
	}
	report, err := os.ReadFile(filepath.Join(runDir, "report.txt"))
	if err != nil || !strings.Contains(string(report), "Failed (attempts 2, checks 1/37,") || !strings.Contains(string(report), "Not reached") {
		t.Fatalf("Step1 failure must leave Step2 unattempted: %s %v", report, err)
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
	modelDir := filepath.Join(t.TempDir(), modelDirName(model.ID))
	env, err := prepareSession(root, filepath.Join(modelDir, "workspace"), "http://unused.invalid", model, 5)
	if err != nil {
		t.Fatal(err)
	}
	item, err := runAttempt(modelDir, "prompt", env, model, 1, 1, benchmarkStep{name: basicGameStep, grade: gradeProgram}, options{steps: 5, timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("agent timeout must be a model result, not a workflow failure: %v", err)
	}
	if item.OpenCode.Error != context.DeadlineExceeded.Error() || item.OpenCode.ExitCode == 0 {
		t.Fatalf("timeout not recorded: %+v", item.OpenCode)
	}
	if item.OpenCode.WallTime > 2*time.Second {
		t.Fatalf("agent exceeded timeout: %s", item.OpenCode.WallTime)
	}
	if item.Grade.Total != 0 {
		t.Fatal("timed out submission was graded")
	}
}

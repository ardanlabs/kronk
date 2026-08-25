package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectModels(t *testing.T) {
	configured := []benchmarkModel{
		{ID: "org/one/AGENT"},
		{ID: "org/two/AGENT"},
	}

	selected, err := selectModels(configured, "org/two/AGENT,org/one/AGENT,org/two/AGENT")
	if err != nil {
		t.Fatalf("selectModels: %v", err)
	}
	if got, want := len(selected), 2; got != want {
		t.Fatalf("selected models: got %d, want %d", got, want)
	}
	if got, want := selected[0].ID, "org/two/AGENT"; got != want {
		t.Errorf("first model: got %q, want %q", got, want)
	}

	if _, err := selectModels(configured, "org/one"); err == nil {
		t.Fatal("model without /AGENT: got nil error, want error")
	}
	if _, err := selectModels(configured, "org/missing/AGENT"); err == nil {
		t.Fatal("unconfigured model: got nil error, want error")
	}
}

func TestOpenCodeConfig(t *testing.T) {
	model := benchmarkModel{ID: "org/model/AGENT", ContextWindow: 65536, OutputLimit: 8192}
	data, err := openCodeConfig("http://localhost:11435", model, 12)
	if err != nil {
		t.Fatalf("openCodeConfig: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if got, want := config["model"], "kronk/org/model/AGENT"; got != want {
		t.Errorf("model: got %v, want %v", got, want)
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
	model := benchmarkModel{ID: "org/model/AGENT", ContextWindow: 65536, OutputLimit: 8192}
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
	if got, want := resolved["model"], "kronk/org/model/AGENT"; got != want {
		t.Errorf("resolved model: got %v, want %v", got, want)
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

func TestWarmAndUnloadModel(t *testing.T) {
	const modelID = "org/model/AGENT"
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
	if got, want := grade.Total, 20; got != want {
		t.Errorf("total checks: got %d, want %d", got, want)
	}
}

func TestAssessScenarioReportsOutOfOrderOutput(t *testing.T) {
	scenario := scenario{ordered: []string{"final board", "Player X wins!", "Play again?"}}
	output := "Player X wins!\nfinal board\nPlay again?"

	passed, detail := assessScenario(output, nil, scenario)
	if passed {
		t.Fatal("assessScenario: got passed, want failed")
	}
	if !strings.Contains(detail, `"Player X wins!" appeared before "final board"`) {
		t.Errorf("detail: got %q, want out-of-order explanation", detail)
	}
}

func TestTerminalOutcomesRequireFinalBoardThenResult(t *testing.T) {
	want := map[string]string{
		"x-win": "Player X wins!",
		"o-win": "Player O wins!",
		"draw":  "It's a draw.",
	}
	for _, scenario := range scenarios() {
		result, exists := want[scenario.name]
		if !exists {
			continue
		}
		if got, want := len(scenario.ordered), 3; got != want {
			t.Errorf("%s ordered values: got %d, want %d", scenario.name, got, want)
			continue
		}
		if got := scenario.ordered[1]; got != result {
			t.Errorf("%s result: got %q, want %q", scenario.name, got, result)
		}
		delete(want, scenario.name)
	}
	if len(want) != 0 {
		t.Errorf("missing terminal scenarios: %v", want)
	}
}

func TestRenderReportExplainsSummaryPercentages(t *testing.T) {
	report := renderReport("run", []modelResults{{
		Model: benchmarkModel{ID: "org/model/AGENT"},
		Attempts: []result{{
			Model:   benchmarkModel{ID: "org/model/AGENT"},
			Attempt: 1,
			Grade:   gradeResult{Passed: 19, Total: 20},
		}},
	}})

	for _, want := range []string{"Checks passed", "Buildable attempts", "Perfect attempts", "does not indicate whether the program built or ran", "each agent turn's final OpenCode usage event"} {
		if !strings.Contains(report, want) {
			t.Errorf("report: missing %q", want)
		}
	}
	if strings.Contains(report, "Cache write") {
		t.Error("report: contains unsupported cache-write metric")
	}
}

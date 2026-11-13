// Package main provides a benchmark for measuring how Kronk-backed models
// complete a coding task through OpenCode's agent loop. It requires a running
// Kronk server and keeps the hidden grader outside each OpenCode workspace.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

const (
	defaultHost          = "http://localhost:11435"
	defaultAttempts      = 3
	defaultSteps         = 40
	defaultTimeout       = 30 * time.Minute
	modelBenchmarkSuffix = "/BENCHMARK"
)

type options struct {
	host       string
	model      string
	configFile string
	promptFile string
	outputDir  string
	reportDir  string
	attempts   int
	steps      int
	timeout    time.Duration
	list       bool
	report     bool
	feedback   bool
}

type benchmarkModel struct {
	ID            string `json:"id"`
	ContextWindow int    `json:"context_window"`
	OutputLimit   int    `json:"output_limit"`
}

type tokenUsage struct {
	Input     int `json:"input"`
	Output    int `json:"output"`
	Reasoning int `json:"reasoning"`
	CacheRead int `json:"cache_read"`
}

type agentResult struct {
	ExitCode  int           `json:"exit_code"`
	Error     string        `json:"error,omitempty"`
	WallTime  time.Duration `json:"wall_time"`
	Turns     int           `json:"turns"`
	ToolCalls int           `json:"tool_calls"`
	Usage     tokenUsage    `json:"usage"`
}

type result struct {
	Model     benchmarkModel `json:"model"`
	Attempt   int            `json:"attempt"`
	OpenCode  agentResult    `json:"opencode"`
	Grade     gradeResult    `json:"grade"`
	SourceSHA string         `json:"source_sha256,omitempty"`
	Repair    string         `json:"repair_message,omitempty"`
}

func main() {
	opts, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() (options, error) {
	root, err := repositoryRoot()
	if err != nil {
		return options{}, err
	}
	var opts options
	flag.StringVar(&opts.host, "host", defaultHost, "running Kronk server URL")
	flag.StringVar(&opts.model, "model", "all", "configured /BENCHMARK model ID, comma-separated IDs, or all")
	flag.IntVar(&opts.attempts, "attempts", defaultAttempts, "maximum submissions per model (1-3, including the initial submission)")
	flag.BoolVar(&opts.feedback, "feedback", true, "return grader failures to the same agent for repair")
	flag.IntVar(&opts.steps, "steps", defaultSteps, "maximum OpenCode agent steps per attempt")
	flag.DurationVar(&opts.timeout, "timeout", defaultTimeout, "timeout per OpenCode attempt")
	flag.BoolVar(&opts.list, "list", false, "list eligible models and exit")
	flag.BoolVar(&opts.report, "report", false, "regenerate the report for the latest saved run")
	flag.StringVar(&opts.reportDir, "run", "", "saved run directory to use with -report (defaults to latest)")
	flag.Parse()
	opts.configFile = filepath.Join(root, "zarf", "kms", "tools_model_config.yaml")
	opts.promptFile = filepath.Join(root, ".tools", "codegen-benchmark", "version2", "tic-tac-toe-"+workloadVersion+".md")
	opts.outputDir = filepath.Join(root, ".tools", "codegen-benchmark", "version2", "runs")
	return opts, nil
}

func run(opts options) (runErr error) {
	if opts.report {
		dir := opts.reportDir
		if dir == "" {
			var err error
			dir, err = latestRun(opts.outputDir)
			if err != nil {
				return err
			}
		}
		return generateReport(dir, nil)
	}
	if opts.attempts < 1 || opts.attempts > defaultAttempts {
		return errors.New("codegen benchmark: submissions must be between 1 and 3")
	}
	if opts.steps < 1 {
		return errors.New("codegen benchmark: steps must be at least 1")
	}
	if opts.timeout <= 0 {
		return errors.New("codegen benchmark: timeout must be positive")
	}

	configured, err := loadModels(opts.configFile)
	if err != nil {
		return err
	}
	if opts.list {
		for _, model := range configured {
			fmt.Println(model.ID)
		}
		return nil
	}

	selected, err := selectModels(configured, opts.model)
	if err != nil {
		return err
	}
	host, err := normalizeHost(opts.host)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		return errors.New("codegen benchmark: opencode is not installed or not on PATH")
	}
	if err := verifyServer(context.Background(), host, selected); err != nil {
		return err
	}
	prompt, err := loadPrompt(opts.promptFile)
	if err != nil {
		return err
	}

	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	runDir, err := prepareRunDir(opts.outputDir)
	if err != nil {
		return err
	}
	machine, err := os.Hostname()
	if err != nil {
		machine = "unknown"
	}
	record := runRecord{Started: time.Now(), Status: "running", Machine: fmt.Sprintf("%s (%s/%s)", machine, runtime.GOOS, runtime.GOARCH)}
	if err := generateReport(runDir, &record); err != nil {
		return err
	}
	defer func() {
		record.Finished = time.Now()
		record.Status = "completed"
		if runErr != nil {
			record.Status = "failed"
			record.Error = runErr.Error()
		}
		runErr = errors.Join(runErr, generateReport(runDir, &record))
	}()
	fmt.Printf("Code-generation benchmark v2\n- server: %s\n- models: %d\n- maximum submissions/model: %d\n- grader feedback: %t\n- output: %s\n", host, len(selected), opts.attempts, opts.feedback, runDir)
	for _, model := range selected {
		if err := runModel(root, runDir, host, prompt, model, opts); err != nil {
			return err
		}
	}

	fmt.Printf("\nResults written to %s\n", runDir)
	return nil
}

func runModel(root, runDir, host, prompt string, model benchmarkModel, opts options) (runErr error) {
	// Retain state only while the same agent is repairing its submissions.
	defer func() {
		runErr = errors.Join(runErr, os.RemoveAll(filepath.Join(runDir, modelDirName(model.ID), "workspace")))
	}()
	fmt.Printf("\n[%s] warming model\n", model.ID)
	warmCtx, cancelWarm := context.WithTimeout(context.Background(), opts.timeout)
	modelErr := warmModel(warmCtx, host, model.ID)
	cancelWarm()

	if modelErr == nil {
		modelErr = runSubmissions(prompt, opts.attempts, opts.feedback, func(attempt int, message string) (result, error) {
			fmt.Printf("[%s] submission %d/%d\n", model.ID, attempt, opts.attempts)
			item, err := runAttempt(root, runDir, host, message, model, attempt, opts)
			if err != nil {
				return item, err
			}
			return item, generateReport(runDir, nil)
		})
	}

	fmt.Printf("[%s] unloading model\n", model.ID)
	unloadCtx, cancelUnload := context.WithTimeout(context.Background(), opts.timeout)
	unloadErr := unloadModel(unloadCtx, host, model.ID)
	cancelUnload()
	return errors.Join(modelErr, unloadErr)
}

// runSubmissions owns the feedback loop; each callback resumes the same workspace
// and session. A passing submission ends the loop, even if the agent exited badly.
func runSubmissions(prompt string, limit int, feedback bool, submit func(int, string) (result, error)) error {
	for attempt := 1; attempt <= limit; attempt++ {
		item, err := submit(attempt, prompt)
		if err != nil {
			return err
		}
		if item.Grade.Passed == item.Grade.Total || !feedback || attempt == limit {
			return nil
		}
		prompt = graderFeedback(item.Grade)
	}
	return nil
}

func graderFeedback(grade gradeResult) string {
	var message strings.Builder
	fmt.Fprintf(&message, "Your submission passed %d/%d checks and did not pass the benchmark. Repair the existing tictactoe/main.go in this workspace; do not start a new project. Preserve all original requirements and working behavior.\n\nFix these failures:\n", grade.Passed, grade.Total)
	for _, check := range grade.Checks {
		if !check.Passed {
			fmt.Fprintf(&message, "\n- %s: %s\n", check.Name, check.Detail)
		}
	}
	message.WriteString("\nChecks not reached because of an earlier failure will be evaluated after it is fixed. Fix the cause, not just the displayed examples. Format and build the code, then submit by finishing your response. The grader will evaluate it again. Do not add features or change the original requirements.\n")
	return message.String()
}

func warmModel(ctx context.Context, host, modelID string) error {
	payload := map[string]any{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "user", "content": "hello model"},
		},
		"max_tokens":      1,
		"enable_thinking": false,
		"stream":          false,
	}
	status, body, err := postJSON(ctx, host, "/v1/chat/completions", payload)
	if err != nil {
		return fmt.Errorf("codegen benchmark: warming model %s: %w", modelID, err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("codegen benchmark: warming model %s returned %s: %s", modelID, http.StatusText(status), body)
	}
	return nil
}

func unloadModel(ctx context.Context, host, modelID string) error {
	status, body, err := postJSON(ctx, host, "/v1/kronk/models/unload", map[string]string{"id": modelID})
	if err != nil {
		return fmt.Errorf("codegen benchmark: unloading model %s: %w", modelID, err)
	}
	if status != http.StatusOK && status != http.StatusNotFound {
		return fmt.Errorf("codegen benchmark: unloading model %s returned %s: %s", modelID, http.StatusText(status), body)
	}
	return nil
}

func postJSON(ctx context.Context, host, path string, payload any) (int, string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, "", fmt.Errorf("encoding request: %w", err)
	}
	requestURL, err := url.JoinPath(host, path)
	if err != nil {
		return 0, "", fmt.Errorf("building request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(data))
	if err != nil {
		return 0, "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(os.Getenv("KRONK_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	if err != nil {
		return 0, "", fmt.Errorf("reading response: %w", err)
	}
	return resp.StatusCode, strings.TrimSpace(string(body)), nil
}

func loadModels(path string) ([]benchmarkModel, error) {
	doc, err := modelconfig.Load(path)
	if err != nil {
		return nil, fmt.Errorf("codegen benchmark: loading %s: %w", path, err)
	}
	configs := doc.Models

	eligible := make([]benchmarkModel, 0, len(configs))
	for id, cfg := range configs {
		if !strings.HasSuffix(id, modelBenchmarkSuffix) {
			continue
		}
		resolved := cfg.ToKronkConfig()
		if resolved.ContextWindow() <= 0 || resolved.DefaultParams.MaxTokens <= 0 {
			return nil, fmt.Errorf("codegen benchmark: model %s must configure context-window and sampling-parameters.max_tokens", id)
		}
		eligible = append(eligible, benchmarkModel{
			ID:            id,
			ContextWindow: resolved.ContextWindow(),
			OutputLimit:   resolved.DefaultParams.MaxTokens,
		})
	}
	slices.SortFunc(eligible, func(a, b benchmarkModel) int { return strings.Compare(a.ID, b.ID) })
	if len(eligible) == 0 {
		return nil, fmt.Errorf("codegen benchmark: %s contains no models ending in %s", path, modelBenchmarkSuffix)
	}
	return eligible, nil
}

func selectModels(configured []benchmarkModel, requested string) ([]benchmarkModel, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" || strings.EqualFold(requested, "all") {
		return configured, nil
	}

	byID := make(map[string]benchmarkModel, len(configured))
	for _, model := range configured {
		byID[model.ID] = model
	}

	var selected []benchmarkModel
	seen := make(map[string]bool)
	for id := range strings.SplitSeq(requested, ",") {
		id = strings.TrimSpace(id)
		if !strings.HasSuffix(id, modelBenchmarkSuffix) {
			return nil, fmt.Errorf("codegen benchmark: model %q must end in %s", id, modelBenchmarkSuffix)
		}
		model, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("codegen benchmark: model %q is not configured", id)
		}
		if !seen[id] {
			selected = append(selected, model)
			seen[id] = true
		}
	}
	return selected, nil
}

func runAttempt(root, runDir, host, prompt string, model benchmarkModel, attempt int, opts options) (result, error) {
	modelDir := filepath.Join(runDir, modelDirName(model.ID))
	dir := filepath.Join(modelDir, "workspace")
	var programDir string
	var err error
	if attempt == 1 {
		programDir, err = prepareAttemptDirectory(dir)
	} else {
		programDir = filepath.Join(dir, "tictactoe")
	}
	if err != nil {
		return result{}, err
	}

	config, err := openCodeConfig(host, model, opts.steps)
	if err != nil {
		return result{}, err
	}
	configPath := filepath.Join(dir, "opencode-config.json")
	if err := os.WriteFile(configPath, append(config, '\n'), 0o600); err != nil {
		return result{}, fmt.Errorf("codegen benchmark: writing OpenCode config: %w", err)
	}
	// Install the isolated home and skill once; retain agent history for repairs.
	if attempt == 1 {
		_, err = prepareOpenCodeEnvironment(root, dir, config)
	}
	if err != nil {
		return result{}, err
	}
	env := openCodeEnvironment(dir, config)
	artifactDir := filepath.Join(modelDir, fmt.Sprintf("attempt-%02d", attempt))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return result{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	started := time.Now()
	args := []string{"run",
		"--format", "json",
		"--pure",
		"--agent", "benchmark",
		"--model", "kronk/" + model.ID,
		"--dir", dir,
	}
	if attempt == 1 {
		args = append(args, "--title", "tic-tac-toe benchmark v2")
	} else {
		args = append(args, "--continue")
	}
	args = append(args, prompt)
	cmd := exec.CommandContext(ctx, "opencode", args...)
	cmd.Dir = dir
	cmd.Env = env

	var stdout bytes.Buffer
	events, err := os.Create(filepath.Join(artifactDir, "events.jsonl"))
	if err != nil {
		return result{}, err
	}
	defer events.Close()
	stderr, err := os.Create(filepath.Join(artifactDir, "opencode.stderr"))
	if err != nil {
		return result{}, err
	}
	defer stderr.Close()
	cmd.Stdout = io.MultiWriter(events, &stdout)
	cmd.Stderr = stderr
	err = cmd.Run()
	wallTime := time.Since(started)
	fmt.Printf("[%s] agent finished; grading submission %d\n", model.ID, attempt)
	agentErr := err

	source, readErr := os.ReadFile(filepath.Join(programDir, "main.go"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return result{}, fmt.Errorf("codegen benchmark: reading generated main.go: %w", readErr)
	}
	if readErr == nil {
		snapshot := filepath.Join(artifactDir, "program")
		if err := os.MkdirAll(snapshot, 0o755); err != nil {
			return result{}, err
		}
		if err := os.WriteFile(filepath.Join(snapshot, "main.go"), source, 0o644); err != nil {
			return result{}, err
		}
		// Match the isolated module actually used by the grader, not agent edits.
		if err := os.WriteFile(filepath.Join(snapshot, "go.mod"), []byte("module tictactoe\n\ngo 1.27.0\n"), 0o644); err != nil {
			return result{}, err
		}
	}
	// Preserve the submission even if the CLI's event stream cannot be decoded.
	agent, parseErr := parseAgentResult(stdout.Bytes())
	if parseErr != nil {
		return result{}, fmt.Errorf("codegen benchmark: parsing OpenCode events: %w", parseErr)
	}
	agent.WallTime = wallTime
	if agentErr != nil {
		agent.ExitCode = 1
		agent.Error = agentErr.Error()
		if exitErr, ok := errors.AsType[*exec.ExitError](agentErr); ok {
			agent.ExitCode = exitErr.ExitCode()
		}
		if ctx.Err() != nil {
			agent.Error = ctx.Err().Error()
		}
	}
	grade := gradeProgram(context.Background(), string(source))
	item := result{
		Model:    model,
		Attempt:  attempt,
		OpenCode: agent,
		Grade:    grade,
	}
	if attempt > 1 {
		item.Repair = prompt
	}
	if readErr == nil {
		item.SourceSHA = fmt.Sprintf("%x", sha256.Sum256(source))
	}
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return result{}, fmt.Errorf("codegen benchmark: encoding result: %w", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "result.json"), append(data, '\n'), 0o644); err != nil {
		return result{}, fmt.Errorf("codegen benchmark: writing result: %w", err)
	}

	fmt.Printf("score=%d/%d build=%t turns=%d tools=%d wall=%s\n",
		grade.Passed, grade.Total, grade.PassedCheck("go-build"), agent.Turns, agent.ToolCalls, wallTime.Round(time.Second))
	return item, nil
}

func prepareAttemptDirectory(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("codegen benchmark: creating attempt directory: %w", err)
	}
	cmd := exec.Command("git", "init", "--quiet", dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("codegen benchmark: initializing attempt repository: %w: %s", err, strings.TrimSpace(string(output)))
	}

	programDir := filepath.Join(dir, "tictactoe")
	if err := os.MkdirAll(programDir, 0o755); err != nil {
		return "", fmt.Errorf("codegen benchmark: creating program directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(programDir, "go.mod"), []byte("module tictactoe\n"), 0o644); err != nil {
		return "", fmt.Errorf("codegen benchmark: writing go.mod: %w", err)
	}
	return programDir, nil
}

func openCodeConfig(host string, model benchmarkModel, steps int) ([]byte, error) {
	apiKey := "kronk"
	if os.Getenv("KRONK_TOKEN") != "" {
		apiKey = "{env:KRONK_TOKEN}"
	}
	baseURL, err := url.JoinPath(host, "/v1")
	if err != nil {
		return nil, fmt.Errorf("codegen benchmark: building provider URL: %w", err)
	}

	permission := map[string]any{
		"read":               "allow",
		"edit":               "allow",
		"glob":               "allow",
		"grep":               "allow",
		"list":               "allow",
		"bash":               "allow",
		"lsp":                "allow",
		"skill":              "allow",
		"todowrite":          "allow",
		"task":               "deny",
		"question":           "deny",
		"webfetch":           "deny",
		"websearch":          "deny",
		"external_directory": "deny",
		"doom_loop":          "allow",
	}
	config := map[string]any{
		"$schema":            "https://opencode.ai/config.json",
		"model":              "kronk/" + model.ID,
		"small_model":        "kronk/" + model.ID,
		"enabled_providers":  []string{"kronk"},
		"disabled_providers": []string{},
		"provider": map[string]any{
			"kronk": map[string]any{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "Kronk benchmark",
				"options": map[string]any{
					"baseURL": baseURL,
					"apiKey":  apiKey,
				},
				"models": map[string]any{
					model.ID: map[string]any{
						"name":  model.ID,
						"limit": map[string]int{"context": model.ContextWindow, "output": model.OutputLimit},
					},
				},
			},
		},
		"agent": map[string]any{
			"benchmark": map[string]any{
				"description": "Implements and verifies the benchmark task in the isolated workspace.",
				"mode":        "primary",
				"steps":       steps,
				"permission":  permission,
			},
			"title": map[string]any{"disable": true},
		},
		"permission": permission,
		"tools": map[string]bool{
			"task": false, "webfetch": false, "websearch": false,
		},
		"formatter":  true,
		"lsp":        true,
		"mcp":        map[string]any{},
		"plugin":     []string{},
		"snapshot":   false,
		"autoupdate": false,
		"share":      "disabled",
		"compaction": map[string]bool{"auto": false, "prune": false},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("codegen benchmark: encoding OpenCode config: %w", err)
	}
	return data, nil
}

func prepareOpenCodeEnvironment(root, attemptDir string, config []byte) ([]string, error) {
	openCodeRoot := filepath.Join(attemptDir, "opencode")
	for _, name := range []string{"home", "config", "data", "cache", "state", "tmp"} {
		dir := filepath.Join(openCodeRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("codegen benchmark: creating OpenCode directory: %w", err)
		}
	}

	skillSource := filepath.Join(root, ".agents", "default", "skills", "use-modern-go")
	skillDestination := filepath.Join(openCodeRoot, "config", "opencode", "skills", "use-modern-go")
	if err := os.CopyFS(skillDestination, os.DirFS(skillSource)); err != nil {
		return nil, fmt.Errorf("codegen benchmark: installing use-modern-go skill: %w", err)
	}

	return openCodeEnvironment(attemptDir, config), nil
}

func openCodeEnvironment(attemptDir string, config []byte) []string {
	openCodeRoot := filepath.Join(attemptDir, "opencode")
	overrides := []string{
		"HOME=" + filepath.Join(openCodeRoot, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(openCodeRoot, "config"),
		"XDG_DATA_HOME=" + filepath.Join(openCodeRoot, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(openCodeRoot, "cache"),
		"XDG_STATE_HOME=" + filepath.Join(openCodeRoot, "state"),
		"TMPDIR=" + filepath.Join(openCodeRoot, "tmp"),
		"OPENCODE_CONFIG_CONTENT=" + string(config),
		"OPENCODE_AUTH_CONTENT={}",
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
		"OPENCODE_DISABLE_MODELS_FETCH=1",
	}
	replaced := make(map[string]bool, len(overrides))
	for _, value := range overrides {
		key, _, _ := strings.Cut(value, "=")
		replaced[key] = true
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, ok := strings.Cut(value, "=")
		if !ok || replaced[key] || strings.HasPrefix(key, "OPENCODE_") {
			continue
		}
		env = append(env, value)
	}
	return append(env, overrides...)
}

func parseAgentResult(data []byte) (agentResult, error) {
	var result agentResult
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Type   string `json:"type"`
				Tool   string `json:"tool"`
				Tokens struct {
					Input     int `json:"input"`
					Output    int `json:"output"`
					Reasoning int `json:"reasoning"`
					Cache     struct {
						Read int `json:"read"`
					} `json:"cache"`
				} `json:"tokens"`
			} `json:"part"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return agentResult{}, fmt.Errorf("decoding event: %w", err)
		}
		if event.Type == "step_finish" || event.Part.Type == "step-finish" || event.Part.Type == "step_finish" {
			result.Turns++
			result.Usage.Input += event.Part.Tokens.Input
			result.Usage.Output += event.Part.Tokens.Output
			result.Usage.Reasoning += event.Part.Tokens.Reasoning
			result.Usage.CacheRead += event.Part.Tokens.Cache.Read
		}
		if event.Type == "tool_use" || event.Part.Type == "tool" || event.Part.Tool != "" {
			result.ToolCalls++
		}
	}
	if err := scanner.Err(); err != nil {
		return agentResult{}, fmt.Errorf("scanning events: %w", err)
	}
	return result, nil
}

func verifyServer(parent context.Context, host string, selected []benchmarkModel) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	modelsURL, err := url.JoinPath(host, "/v1/models")
	if err != nil {
		return fmt.Errorf("codegen benchmark: building models URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return fmt.Errorf("codegen benchmark: creating models request: %w", err)
	}
	if token := strings.TrimSpace(os.Getenv("KRONK_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("codegen benchmark: connecting to Kronk at %s: %w", host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		if readErr != nil {
			return fmt.Errorf("codegen benchmark: reading GET %s error response: %w", modelsURL, readErr)
		}
		return fmt.Errorf("codegen benchmark: GET %s returned %s: %s", modelsURL, resp.Status, strings.TrimSpace(string(body)))
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("codegen benchmark: decoding model list: %w", err)
	}
	available := make(map[string]bool, len(response.Data))
	for _, model := range response.Data {
		available[model.ID] = true
	}
	for _, model := range selected {
		if !available[model.ID] {
			return fmt.Errorf("codegen benchmark: configured model %q is not available from %s", model.ID, host)
		}
	}
	return nil
}

func loadPrompt(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("codegen benchmark: reading prompt %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func normalizeHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("codegen benchmark: invalid host %q", host)
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

func prepareRunDir(dir string) (string, error) {
	now := time.Now()
	dateDir := filepath.Join(dir, now.Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0o755); err != nil {
		return "", fmt.Errorf("codegen benchmark: creating output directory: %w", err)
	}
	return os.MkdirTemp(dateDir, "run-"+now.Format("150405.000000000")+"-")
}

func repositoryRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("codegen benchmark: locating source")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func modelDirName(id string) string {
	replacer := strings.NewReplacer("/", "__", "\\", "__", ":", "_")
	return strings.TrimSuffix(replacer.Replace(id), "__AGENT")
}

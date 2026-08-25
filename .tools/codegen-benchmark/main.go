// Package main provides a benchmark for measuring how Kronk-backed models
// complete a coding task through OpenCode's agent loop. It requires a running
// Kronk server and keeps the hidden grader outside each OpenCode workspace.
package main

import (
	"bufio"
	"bytes"
	"context"
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

	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	defaultHost      = "http://localhost:11435"
	defaultAttempts  = 1
	defaultSteps     = 40
	defaultTimeout   = 30 * time.Minute
	promptSeparator  = "\n================================================================================\n"
	modelAgentSuffix = "/AGENT"
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
	Workspace string         `json:"workspace"`
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
	flag.StringVar(&opts.model, "model", "all", "configured /AGENT model ID, comma-separated IDs, or all")
	flag.StringVar(&opts.outputDir, "out", "", "output directory (default .tools/codegen-benchmark/output/<timestamp>)")
	flag.StringVar(&opts.reportDir, "report", "", "regenerate the report for an existing output directory")
	flag.IntVar(&opts.attempts, "attempts", defaultAttempts, "fresh OpenCode attempts per model")
	flag.IntVar(&opts.steps, "steps", defaultSteps, "maximum OpenCode agent steps per attempt")
	flag.DurationVar(&opts.timeout, "timeout", defaultTimeout, "timeout per OpenCode attempt")
	flag.BoolVar(&opts.list, "list", false, "list eligible models and exit")
	flag.Parse()
	opts.configFile = filepath.Join(root, "zarf", "kms", "model_config.yaml")
	opts.promptFile = filepath.Join(root, "examples", "talks", "tic-tac-toe.md")
	return opts, nil
}

func run(opts options) error {
	if opts.reportDir != "" {
		return generateReport(opts.reportDir)
	}
	if opts.attempts < 1 {
		return errors.New("codegen benchmark: attempts must be at least 1")
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
	runDir, err := prepareRunDir(root, opts.outputDir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runDir, "prompt.md"), []byte(prompt+"\n"), 0o644); err != nil {
		return fmt.Errorf("codegen benchmark: writing prompt: %w", err)
	}

	fmt.Printf("Code-generation benchmark\n- server: %s\n- models: %d\n- attempts/model: %d\n- output: %s\n", host, len(selected), opts.attempts, runDir)
	for _, model := range selected {
		if err := runModel(root, runDir, host, prompt, model, opts); err != nil {
			return err
		}
	}

	if err := generateReport(runDir); err != nil {
		return err
	}
	fmt.Printf("\nResults written to %s\n", runDir)
	return nil
}

func runModel(root, runDir, host, prompt string, model benchmarkModel, opts options) error {
	fmt.Printf("\n[%s] warming model\n", model.ID)
	warmCtx, cancelWarm := context.WithTimeout(context.Background(), opts.timeout)
	modelErr := warmModel(warmCtx, host, model.ID)
	cancelWarm()

	if modelErr == nil {
		for attempt := 1; attempt <= opts.attempts; attempt++ {
			fmt.Printf("[%s] attempt %d/%d\n", model.ID, attempt, opts.attempts)
			if err := runAttempt(root, runDir, host, prompt, model, attempt, opts); err != nil {
				modelErr = err
				break
			}
		}
	}

	fmt.Printf("[%s] unloading model\n", model.ID)
	unloadCtx, cancelUnload := context.WithTimeout(context.Background(), opts.timeout)
	unloadErr := unloadModel(unloadCtx, host, model.ID)
	cancelUnload()
	return errors.Join(modelErr, unloadErr)
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
	configs, err := models.LoadModelConfig(path)
	if err != nil {
		return nil, fmt.Errorf("codegen benchmark: loading %s: %w", path, err)
	}

	eligible := make([]benchmarkModel, 0, len(configs))
	for id, cfg := range configs {
		if !strings.HasSuffix(id, modelAgentSuffix) {
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
		return nil, fmt.Errorf("codegen benchmark: %s contains no models ending in %s", path, modelAgentSuffix)
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
		if !strings.HasSuffix(id, modelAgentSuffix) {
			return nil, fmt.Errorf("codegen benchmark: model %q must end in %s", id, modelAgentSuffix)
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

func runAttempt(root, runDir, host, prompt string, model benchmarkModel, attempt int, opts options) error {
	dir := filepath.Join(runDir, modelDirName(model.ID), fmt.Sprintf("attempt-%02d", attempt))
	programDir, err := prepareAttemptDirectory(dir)
	if err != nil {
		return err
	}

	config, err := openCodeConfig(host, model, opts.steps)
	if err != nil {
		return err
	}
	configPath := filepath.Join(dir, "opencode-config.json")
	if err := os.WriteFile(configPath, append(config, '\n'), 0o600); err != nil {
		return fmt.Errorf("codegen benchmark: writing OpenCode config: %w", err)
	}
	env, err := prepareOpenCodeEnvironment(root, dir, config)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	started := time.Now()
	cmd := exec.CommandContext(ctx, "opencode", "run",
		"--format", "json",
		"--pure",
		"--agent", "benchmark",
		"--model", "kronk/"+model.ID,
		"--dir", dir,
		"--title", fmt.Sprintf("tic-tac-toe benchmark %d", attempt),
		prompt,
	)
	cmd.Dir = dir
	cmd.Env = env

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	wallTime := time.Since(started)
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), stdout.Bytes(), 0o644); err != nil {
		return fmt.Errorf("codegen benchmark: writing OpenCode events: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.stderr"), stderr.Bytes(), 0o644); err != nil {
		return fmt.Errorf("codegen benchmark: writing OpenCode stderr: %w", err)
	}

	agent, parseErr := parseAgentResult(stdout.Bytes())
	if parseErr != nil {
		return fmt.Errorf("codegen benchmark: parsing OpenCode events: %w", parseErr)
	}
	agent.WallTime = wallTime
	if err != nil {
		agent.ExitCode = 1
		agent.Error = err.Error()
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			agent.ExitCode = exitErr.ExitCode()
		}
		if ctx.Err() != nil {
			agent.Error = ctx.Err().Error()
		}
	}

	source, readErr := os.ReadFile(filepath.Join(programDir, "main.go"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("codegen benchmark: reading generated main.go: %w", readErr)
	}
	grade := gradeProgram(context.Background(), string(source))
	item := result{
		Model:     model,
		Attempt:   attempt,
		OpenCode:  agent,
		Grade:     grade,
		Workspace: ".",
	}
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("codegen benchmark: encoding result: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.json"), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("codegen benchmark: writing result: %w", err)
	}

	fmt.Printf("score=%d/%d build=%t turns=%d tools=%d wall=%s\n",
		grade.Passed, grade.Total, grade.PassedCheck("go-build"), agent.Turns, agent.ToolCalls, wallTime.Round(time.Second))
	return nil
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
	configHome := filepath.Join(openCodeRoot, "config")
	configDir := filepath.Join(configHome, "opencode")
	overrides := []string{
		"HOME=" + filepath.Join(openCodeRoot, "home"),
		"XDG_CONFIG_HOME=" + configHome,
		"XDG_DATA_HOME=" + filepath.Join(openCodeRoot, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(openCodeRoot, "cache"),
		"XDG_STATE_HOME=" + filepath.Join(openCodeRoot, "state"),
		"TMPDIR=" + filepath.Join(openCodeRoot, "tmp"),
		"OPENCODE_CONFIG_CONTENT=" + string(config),
		"OPENCODE_AUTH_CONTENT={}",
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
		"OPENCODE_DISABLE_MODELS_FETCH=1",
	}
	for _, value := range overrides[:6] {
		_, dir, _ := strings.Cut(value, "=")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("codegen benchmark: creating OpenCode directory: %w", err)
		}
	}

	skillSource := filepath.Join(root, ".agents", "default", "skills", "writing-go")
	skillDestination := filepath.Join(configDir, "skills", "writing-go")
	if err := os.CopyFS(skillDestination, os.DirFS(skillSource)); err != nil {
		return nil, fmt.Errorf("codegen benchmark: installing writing-go skill: %w", err)
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
	return append(env, overrides...), nil
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
	base, _, ok := strings.Cut(string(data), promptSeparator)
	if !ok {
		return "", fmt.Errorf("codegen benchmark: prompt separator is missing from %s", path)
	}
	return strings.TrimSpace(base), nil
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

func prepareRunDir(root, requested string) (string, error) {
	dir := requested
	if dir == "" {
		dir = filepath.Join(root, ".tools", "codegen-benchmark", "output", time.Now().Format("20060102-150405"))
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return "", fmt.Errorf("codegen benchmark: output directory is not empty: %s", dir)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("codegen benchmark: reading output directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("codegen benchmark: creating output directory: %w", err)
	}
	return dir, nil
}

func repositoryRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("codegen benchmark: locating source")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
}

func modelDirName(id string) string {
	replacer := strings.NewReplacer("/", "__", "\\", "__", ":", "_")
	return strings.TrimSuffix(replacer.Replace(id), "__AGENT")
}

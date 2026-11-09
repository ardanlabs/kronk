// Reliability runs model-backed batch-engine, MTP, IMC, long-context, and
// multimodal probes against a separately managed Kronk server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"time"
)

const summarySchemaVersion = 10

var scenarioOrder = []string{"text-state", "images", "long-context", "batch", "mtp"}

type scenarioResult struct {
	Name            string            `json:"name"`
	Cell            string            `json:"cell"`
	Phase           string            `json:"phase,omitempty"`
	Repetition      int               `json:"repetition"`
	ExpectedModes   map[string]string `json:"expected_modes"`
	Status          string            `json:"status"`
	StartedAt       time.Time         `json:"started_at"`
	DurationSeconds float64           `json:"duration_seconds"`
	Models          []string          `json:"models,omitempty"`
	Failures        []string          `json:"failures,omitempty"`
	Coverage        []pathCoverage    `json:"coverage,omitempty"`
	Details         map[string]any    `json:"details,omitempty"`
}

// Path evidence is independent of the behavioral verdict. Missing instrumentation
// or an untriggered path must not be reported as a broken server behavior.
type pathCoverage struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

type runSummary struct {
	SchemaVersion         int                    `json:"schema_version"`
	Status                string                 `json:"status"`
	CoverageStatus        string                 `json:"coverage_status,omitempty"`
	StartedAt             time.Time              `json:"started_at"`
	FinishedAt            time.Time              `json:"finished_at"`
	DurationSeconds       float64                `json:"duration_seconds"`
	Host                  string                 `json:"host"`
	Configuration         config                 `json:"configuration"`
	ToolBuild             map[string]string      `json:"tool_build"`
	ServerMetadata        map[string]any         `json:"server_metadata"`
	ExpectedModes         map[string]string      `json:"expected_modes,omitempty"`
	Matrix                []modeCell             `json:"matrix"`
	Selection             []probeSelection       `json:"selection,omitempty"`
	ModelUnloads          []modelUnloadResult    `json:"model_unloads,omitempty"`
	Failures              []string               `json:"failures,omitempty"`
	Note                  string                 `json:"note,omitempty"`
	Scenarios             []scenarioResult       `json:"scenarios"`
	ServerEvidence        serverEvidence         `json:"server_evidence"`
	PerformanceAggregates []performanceAggregate `json:"performance_aggregates,omitempty"`
}

type runContext struct {
	context.Context
	cfg                 config
	client              *apiClient
	artifacts           *artifacts
	scenario            string
	repetition          int
	observedModes       map[string]map[string]bool
	modelConfigurations map[string]any
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, list, err := parseConfig(os.Args[1:])
	if err != nil {
		return err
	}
	if list {
		fmt.Println("Profiles: " + strings.Join(profileOrder, ", "))
		for _, cell := range coverageCells() {
			if cell.GPU == "1" {
				fmt.Println(cell.id())
			}
		}
		return nil
	}
	plan := executionPlan(cfg)
	fmt.Println("Selected profile × cell:")
	for _, probe := range plan {
		fmt.Printf("  %s × %s: %s\n", probe.Profile, probe.Cell, probe.Status)
	}

	art, err := newArtifacts(cfg.Out, cfg.ServerLog)
	if err != nil {
		return err
	}
	defer art.close()

	started := time.Now().UTC()
	summary := runSummary{
		SchemaVersion:  summarySchemaVersion,
		Status:         "PASS",
		StartedAt:      started,
		Host:           cfg.Host,
		Matrix:         cfg.Matrix,
		Selection:      plan,
		Configuration:  cfg,
		ToolBuild:      map[string]string{"go_version": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH},
		ServerMetadata: make(map[string]any),
	}
	// Keep selection pair-based; expand internal phases only for execution.
	var workloads []probeSelection
	for _, probe := range plan {
		workloads = append(workloads, probe)
		for _, phase := range probe.AdditionalPhases {
			next := probe
			next.Config, next.AdditionalPhases = phase, nil
			workloads = append(workloads, next)
		}
	}
	plan = workloads
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range build.Settings {
			if strings.HasPrefix(setting.Key, "vcs") {
				summary.ToolBuild[setting.Key] = setting.Value
			}
		}
	}
	if len(cfg.Matrix) == 1 {
		summary.ExpectedModes = map[string]string{"mtp": cfg.MTP, "imc": cfg.IMC}
	}

	client := newAPIClient(cfg.Host, cfg.Token, cfg.Timeout, art.traces)
	base := runContext{
		Context:   context.Background(),
		cfg:       cfg,
		client:    client,
		artifacts: art,
	}

	art.logf("Kronk reliability suite")
	art.logf("host=%s scenarios=%s output=%s", cfg.Host, strings.Join(cfg.Scenarios, ","), cfg.Out)
	art.logf("server_log=%s", cfg.ServerLog)
	art.logf("matrix=%v", cfg.Matrix)
	art.logf("repetitions=%d run_label=%q", cfg.Repetitions, cfg.RunLabel)

	// Clear stale suite aliases from prior invocations, but only for selected
	// workloads. Other profiles, base IDs, and unrelated models are untouched.
	art.traces.setCell("setup")
	base.scenario = "setup"
	var endpoints []string
	for _, probe := range plan {
		if probe.Status == "runnable" {
			endpoints = []string{"/v1/liveness", "/v1/kronk/libs"}
			break
		}
	}
	for _, endpoint := range endpoints {
		var metadata map[string]any
		ctx, cancel := context.WithTimeout(base, min(cfg.Timeout, 5*time.Second))
		_, err := client.json(ctx, "setup", http.MethodGet, endpoint, nil, &metadata)
		cancel()
		if err != nil {
			summary.ServerMetadata[endpoint] = map[string]string{"unavailable": err.Error()}
		} else {
			summary.ServerMetadata[endpoint] = metadata
		}
	}
	seen := make(map[string]bool)
	var cleanupConfigs []config
	if len(cfg.Profiles) > 0 {
		for _, probe := range plan {
			if probe.Status == "runnable" {
				cleanupConfigs = append(cleanupConfigs, probe.Config)
			}
		}
	} else {
		for _, mode := range []modeCell{{"on", "on"}, {"on", "off"}, {"off", "on"}, {"off", "off"}} {
			candidate, _ := cfg.forCell(mode)
			cleanupConfigs = append(cleanupConfigs, candidate)
		}
	}
	for _, candidate := range cleanupConfigs {
		for _, id := range candidate.selectedModels() {
			if seen[id] {
				continue
			}
			seen[id] = true
			if err := base.unloadModel(id, "before-run"); err != nil {
				appendFailure(&summary.Failures, "%v", err)
				break
			}
		}
		if client.cleanupFailed {
			break
		}
	}

	for repetition := 1; repetition <= cfg.Repetitions && !client.cleanupFailed; repetition++ {
		art.traces.setRepetition(repetition)
		base.repetition = repetition
		art.logf("\n========== REPETITION %d/%d ==========", repetition, cfg.Repetitions)
		for _, probe := range plan {
			if client.cleanupFailed {
				break
			}
			if probe.Status != "runnable" {
				status := "NOT APPLY"
				if probe.Status == "coverage gap" {
					status = "GAP"
					summary.CoverageStatus = "INCOMPLETE"
				}
				summary.Scenarios = append(summary.Scenarios, scenarioResult{Name: probe.Profile, Cell: probe.Cell, Repetition: repetition, Status: status, Details: map[string]any{"reason": probe.Reason}})
				art.logf("%s × %s: %s (%s)", probe.Profile, probe.Cell, probe.Status, probe.Reason)
				continue
			}
			cellConfig, name := probe.Config, probe.Scenario
			art.traces.setCell(probe.Cell)
			art.logf("\n========== SCENARIO %s/%s START ==========", probe.Cell, name)
			if cellConfig.MediaPrefillExecution != "" {
				art.logf("image phase=%s", cellConfig.MediaPrefillExecution)
			}
			scenarioStarted := time.Now().UTC()
			rc := base
			rc.cfg, rc.scenario = cellConfig, name
			rc.observedModes = make(map[string]map[string]bool)
			result := runScenario(&rc, name)
			if client.managedModel != "" && !client.cleanupFailed {
				if err := rc.unloadModel(client.managedModel, "after-scenario"); err != nil {
					appendFailure(&result.Failures, "%v", err)
				}
			}
			if result.Details == nil {
				result.Details = make(map[string]any)
			}
			result.Details["observed_modes"] = rc.observedModes
			result.Details["model_configurations"] = rc.modelConfigurations
			result.Name, result.Cell = name, probe.Cell
			result.Phase = cellConfig.MediaPrefillExecution
			result.Repetition = repetition
			result.ExpectedModes = map[string]string{"mtp": cellConfig.MTP, "imc": cellConfig.IMC}
			result.StartedAt = scenarioStarted
			result.DurationSeconds = seconds(time.Since(scenarioStarted))
			if len(result.Failures) > 0 {
				result.Status, summary.Status = "FAIL", "FAIL"
			} else if result.Status == "" {
				result.Status = "PASS"
			}
			summary.Scenarios = append(summary.Scenarios, result)
			art.logf("========== SCENARIO %s/%s %s (%.3fs) ==========", probe.Cell, name, result.Status, result.DurationSeconds)
			for _, failure := range result.Failures {
				art.logf("FAIL: %s", failure)
			}
			if client.cleanupFailed {
				break
			}
		}
	}
	if client.cleanupFailed {
		summary.Status = "FAIL"
		summary.Note = "Stopped after model cleanup failed; remaining selected workloads were not run."
		art.logf("%s", summary.Note)
	}
	summary.ModelUnloads = client.unloads
	runnable := false
	for _, probe := range plan {
		runnable = runnable || probe.Status == "runnable"
	}
	if !runnable {
		summary.Status = "NOT RUN"
	}

	summary.ServerEvidence = art.collectServerEvidence()
	annotateMTPServerEvidence(&summary)
	annotateMediaExecutionEvidence(&summary)
	annotateBatchExecution(&summary)
	for _, scenario := range summary.Scenarios {
		for _, coverage := range scenario.Coverage {
			if coverage.Status == "warning" {
				art.logf("WARNING: %s / %s / %s: %s", scenario.Cell, scenario.Name, coverage.Path, coverage.Note)
				continue
			}
			if summary.CoverageStatus == "" {
				summary.CoverageStatus = "OBSERVED"
			}
			if coverage.Status != "exercised" {
				summary.CoverageStatus = "INCOMPLETE"
			}
			art.logf("path coverage: %s / %s / %s: %s (%s)", scenario.Cell, scenario.Name, coverage.Path, coverage.Status, coverage.Note)
		}
	}
	summary.FinishedAt = time.Now().UTC()
	summary.DurationSeconds = seconds(summary.FinishedAt.Sub(started))
	aggregatePerformance(&summary)
	if err := art.writeSummary(summary); err != nil {
		return err
	}

	art.logf("\n%s: %d scenario(s); server_events=%d", summary.Status, len(summary.Scenarios), summary.ServerEvidence.MatchedEvents)
	art.logf("artifacts: %s", cfg.Out)
	if err := art.writeLogSummary(summary); err != nil {
		return err
	}
	if summary.Status == "NOT RUN" {
		return errors.New("no runnable profile/cell combinations; see selection and coverage gaps")
	}
	if summary.Status != "PASS" {
		return errors.New("one or more reliability scenarios failed")
	}
	if summary.CoverageStatus == "INCOMPLETE" {
		return errors.New("requested coverage is incomplete; see selection and summary coverage (not a behavioral regression verdict)")
	}
	return nil
}

func annotateMTPServerEvidence(summary *runSummary) {
	for index := range summary.Scenarios {
		scenario := &summary.Scenarios[index]
		modes := scenario.ExpectedModes
		if modes == nil {
			modes = summary.ExpectedModes
		}
		if scenario.Name != "mtp" || modes["mtp"] == "off" {
			continue
		}
		profiles, _ := scenario.Details["profiles"].([]mtpProfileResult)
		for profileIndex := range profiles {
			profile := &profiles[profileIndex]
			for requestIndex := range profile.Results {
				request := &profile.Results[requestIndex]
				hasCounts := false
				for _, rollback := range summary.ServerEvidence.MTP.TargetRollbacks {
					hasCounts = hasCounts || rollback.TraceID == request.TraceID && rollback.Count > 0
				}
				for _, rollback := range summary.ServerEvidence.MTP.TargetRollbacks {
					if !summary.ServerEvidence.Available || request.TraceID == "" || rollback.TraceID != request.TraceID {
						continue
					}
					// Prefer request totals over legacy per-round events if both exist.
					if hasCounts && rollback.Count == 0 {
						continue
					}
					count := rollback.Count
					if count == 0 {
						count = 1
					}
					if request.RollbackStrategies == nil {
						request.RollbackStrategies = make(map[string]int)
					}
					request.RollbackStrategies[rollback.Strategy] += count
					request.RollbackRounds += count
				}
				if profile.MinRollbackRounds > 0 {
					coverage := pathCoverage{
						Path:   fmt.Sprintf("%s/wave-%d/request-%d/rejection-recovery", profile.Name, request.Wave, request.Request),
						Status: "exercised",
						Note:   fmt.Sprintf("observed %d completed rejection/recovery events; requested %d", request.RollbackRounds, profile.MinRollbackRounds),
					}
					if request.RollbackRounds < profile.MinRollbackRounds {
						coverage.Status = "not exercised"
					}
					scenario.Coverage = append(scenario.Coverage, coverage)
				}
			}
		}
	}
}

func annotateMediaExecutionEvidence(summary *runSummary) {
	for index := range summary.Scenarios {
		scenario := &summary.Scenarios[index]
		prefill, ok := scenario.Details["concurrent_prefill"].(mediaPrefillResult)
		if !ok || (!prefill.RequireIsolatedPrefill && prefill.ExpectedExecution == "") {
			continue
		}
		expected := prefill.ExpectedExecution
		oversized := strings.HasPrefix(expected, "oversized-") || prefill.RequireIsolatedPrefill
		if summary.ServerEvidence.Available && prefill.Image.TraceID != "" {
			for _, image := range summary.ServerEvidence.ImageExecutions {
				if image.TraceID != prefill.Image.TraceID || image.Tokens <= 0 {
					continue
				}
				if expected == "" {
					// Low-level legacy isolation request accepts either whole-image path.
					if image.Execution != "isolated" && image.Execution != "imc" {
						continue
					}
				} else if image.Execution != strings.TrimPrefix(expected, "oversized-") {
					continue
				}
				if oversized && (prefill.PrefillBatchSize <= 0 || image.Tokens <= prefill.PrefillBatchSize) {
					continue
				}
				prefill.ObservedImageTokens = max(prefill.ObservedImageTokens, image.Tokens)
			}
		}
		if oversized {
			prefill.IsolatedImageTokens = prefill.ObservedImageTokens
		}
		path := expected + "-media-prefill"
		if expected == "" {
			path = "isolated-media-prefill"
		}
		coverage := pathCoverage{Path: path, Status: "exercised", Note: fmt.Sprintf("image-request execution observed: %d tokens", prefill.ObservedImageTokens)}
		if prefill.ObservedImageTokens == 0 {
			coverage.Status = "not exercised"
			coverage.Note = "required image-request execution not observed; path untriggered or evidence unavailable"
		}
		scenario.Coverage = append(scenario.Coverage, coverage)
		scenario.Details["concurrent_prefill"] = prefill
	}
}

func runScenario(rc *runContext, name string) scenarioResult {
	switch name {
	case "mtp":
		return runMTP(rc)
	case "text-state":
		return runHybridState(rc)
	case "long-context":
		return runLongContext(rc)
	case "batch", "batch/text", "batch/images":
		return runBatch(rc)
	case "images":
		return runMedia(rc)
	default:
		return scenarioResult{Failures: []string{"unknown scenario: " + name}}
	}
}

func parseScenarios(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, errors.New("-scenario must select at least one scenario")
	}

	var selected []string
	for raw := range strings.SplitSeq(value, ",") {
		name := strings.TrimSpace(raw)
		if name == "all" {
			selected = append(selected, scenarioOrder...)
			continue
		}
		if !slices.Contains(scenarioOrder, name) {
			return nil, fmt.Errorf("unknown scenario %q (valid: %s, all)", name, strings.Join(scenarioOrder, ", "))
		}
		selected = append(selected, name)
	}

	result := make([]string, 0, len(selected))
	for _, name := range scenarioOrder {
		if slices.Contains(selected, name) {
			result = append(result, name)
		}
	}
	return result, nil
}

func seconds(duration time.Duration) float64 {
	return float64(duration.Round(time.Millisecond)) / float64(time.Second)
}

func newFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("reliability", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

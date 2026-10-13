// Reliability runs model-backed batch-engine, MTP, IMC, long-context, and
// multimodal probes against a separately managed Kronk server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

const summarySchemaVersion = 2

var scenarioOrder = []string{"mtp", "hybrid-state", "long-context", "batch", "media"}

type scenarioResult struct {
	Name            string            `json:"name"`
	Cell            string            `json:"cell"`
	ExpectedModes   map[string]string `json:"expected_modes"`
	Status          string            `json:"status"`
	StartedAt       time.Time         `json:"started_at"`
	DurationSeconds float64           `json:"duration_seconds"`
	Models          []string          `json:"models,omitempty"`
	Failures        []string          `json:"failures,omitempty"`
	Details         map[string]any    `json:"details,omitempty"`
}

type runSummary struct {
	SchemaVersion   int                 `json:"schema_version"`
	Status          string              `json:"status"`
	StartedAt       time.Time           `json:"started_at"`
	FinishedAt      time.Time           `json:"finished_at"`
	DurationSeconds float64             `json:"duration_seconds"`
	Host            string              `json:"host"`
	ExpectedModes   map[string]string   `json:"expected_modes,omitempty"`
	Matrix          []modeCell          `json:"matrix"`
	ModelUnloads    []modelUnloadResult `json:"model_unloads,omitempty"`
	Failures        []string            `json:"failures,omitempty"`
	Note            string              `json:"note,omitempty"`
	Scenarios       []scenarioResult    `json:"scenarios"`
	ServerEvidence  serverEvidence      `json:"server_evidence"`
}

type runContext struct {
	context.Context
	cfg           config
	client        *apiClient
	artifacts     *artifacts
	scenario      string
	observedModes map[string]map[string]bool
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
		fmt.Println(strings.Join(scenarioOrder, "\n"))
		return nil
	}

	art, err := newArtifacts(cfg.Out, cfg.ServerLog)
	if err != nil {
		return err
	}
	defer art.close()

	started := time.Now().UTC()
	summary := runSummary{
		SchemaVersion: summarySchemaVersion,
		Status:        "PASS",
		StartedAt:     started,
		Host:          cfg.Host,
		Matrix:        cfg.Matrix,
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

	// Clear stale suite aliases from prior invocations, but only for selected
	// workloads. Other profiles, base IDs, and unrelated models are untouched.
	art.traces.setCell("setup")
	base.scenario = "setup"
	seen := make(map[string]bool)
	for _, mode := range []modeCell{{"on", "on"}, {"on", "off"}, {"off", "on"}, {"off", "off"}} {
		candidate, _ := cfg.forCell(mode) // parseConfig already validated these IDs.
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

	for _, cell := range cfg.Matrix {
		if client.cleanupFailed {
			break
		}
		cellConfig, _ := cfg.forCell(cell)
		art.traces.setCell(cell.id())
		art.logf("\n========== CELL %s START ==========", cell.id())
		for _, name := range cfg.Scenarios {
			art.logf("\n========== SCENARIO %s/%s START ==========", cell.id(), name)
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
			result.Name, result.Cell = name, cell.id()
			result.ExpectedModes = map[string]string{"mtp": cell.MTP, "imc": cell.IMC}
			result.StartedAt = scenarioStarted
			result.DurationSeconds = seconds(time.Since(scenarioStarted))
			if len(result.Failures) > 0 {
				result.Status, summary.Status = "FAIL", "FAIL"
			} else if result.Status == "" {
				result.Status = "PASS"
			}
			summary.Scenarios = append(summary.Scenarios, result)
			art.logf("========== SCENARIO %s/%s %s (%.3fs) ==========", cell.id(), name, result.Status, result.DurationSeconds)
			for _, failure := range result.Failures {
				art.logf("FAIL: %s", failure)
			}
			if client.cleanupFailed {
				break
			}
		}
		art.logf("========== CELL %s END ==========", cell.id())
	}
	if client.cleanupFailed {
		summary.Status = "FAIL"
		summary.Note = "Stopped after model cleanup failed; remaining selected workloads were not run."
		art.logf("%s", summary.Note)
	}
	summary.ModelUnloads = client.unloads

	summary.ServerEvidence = art.collectServerEvidence()
	validateMTPServerEvidence(&summary)
	summary.FinishedAt = time.Now().UTC()
	summary.DurationSeconds = seconds(summary.FinishedAt.Sub(started))
	if err := art.writeSummary(summary); err != nil {
		return err
	}

	art.logf("\n%s: %d scenario(s); server_events=%d", summary.Status, len(summary.Scenarios), summary.ServerEvidence.MatchedEvents)
	art.logf("artifacts: %s", cfg.Out)
	if err := art.writeLogSummary(summary); err != nil {
		return err
	}
	if summary.Status != "PASS" {
		return errors.New("one or more reliability scenarios failed")
	}
	return nil
}

func validateMTPServerEvidence(summary *runSummary) {
	if !summary.ServerEvidence.Available {
		return
	}

	cleared := make(map[string]bool, len(summary.ServerEvidence.MTP.DraftKVClearedTraceIDs))
	for _, traceID := range summary.ServerEvidence.MTP.DraftKVClearedTraceIDs {
		cleared[traceID] = true
	}

	for index := range summary.Scenarios {
		scenario := &summary.Scenarios[index]
		modes := scenario.ExpectedModes
		if modes == nil {
			modes = summary.ExpectedModes
		}
		if scenario.Name != "mtp" || modes["mtp"] == "off" {
			continue
		}
		for _, selection := range summary.ServerEvidence.MTP.BackendSelections {
			if selection.Scenario != "mtp" || selection.Cell != scenario.Cell {
				continue
			}
			if selection.Backend != "qwen35-own-kv" {
				appendFailure(&scenario.Failures, "%s selected backend %q, want qwen35-own-kv", selection.Message, selection.Backend)
			}
			if selection.Message == "draft-model-mtp-separate" && selection.Source != "mtp-drafter-file" {
				appendFailure(&scenario.Failures, "%s selected source %q, want mtp-drafter-file", selection.Message, selection.Source)
			}
			if selection.Message == "draft-model-mtp" && selection.Source != "auto-detected" && selection.Source != "auto-detected-configured" {
				appendFailure(&scenario.Failures, "%s selected unexpected source %q", selection.Message, selection.Source)
			}
		}
		profiles, ok := scenario.Details["profiles"].([]mtpProfileResult)
		if !ok {
			continue
		}
		for _, profile := range profiles {
			for _, request := range profile.Results {
				if request.Error == "" && request.TraceID != "" && !cleared[request.TraceID] {
					appendFailure(&scenario.Failures, "%s: request %d has no draft-kv-cleared server event", profile.Name, request.Request)
				}
			}
		}
		if len(scenario.Failures) > 0 {
			scenario.Status = "FAIL"
			summary.Status = "FAIL"
		}
	}
}

func runScenario(rc *runContext, name string) scenarioResult {
	switch name {
	case "mtp":
		return runMTP(rc)
	case "hybrid-state":
		return runHybridState(rc)
	case "long-context":
		return runLongContext(rc)
	case "batch":
		return runBatch(rc)
	case "media":
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

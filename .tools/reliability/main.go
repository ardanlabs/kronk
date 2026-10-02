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

const summarySchemaVersion = 1

var scenarioOrder = []string{"mtp", "hybrid-state", "long-context", "batch", "media"}

type scenarioResult struct {
	Name            string         `json:"name"`
	Status          string         `json:"status"`
	StartedAt       time.Time      `json:"started_at"`
	DurationSeconds float64        `json:"duration_seconds"`
	Models          []string       `json:"models,omitempty"`
	Failures        []string       `json:"failures,omitempty"`
	Details         map[string]any `json:"details,omitempty"`
}

type runSummary struct {
	SchemaVersion   int              `json:"schema_version"`
	Status          string           `json:"status"`
	StartedAt       time.Time        `json:"started_at"`
	FinishedAt      time.Time        `json:"finished_at"`
	DurationSeconds float64          `json:"duration_seconds"`
	Host            string           `json:"host"`
	Scenarios       []scenarioResult `json:"scenarios"`
	ServerEvidence  serverEvidence   `json:"server_evidence"`
}

type runContext struct {
	context.Context
	cfg       config
	client    *apiClient
	artifacts *artifacts
	scenario  string
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

	for _, name := range cfg.Scenarios {
		art.logf("\n========== SCENARIO %s START ==========", name)
		scenarioStarted := time.Now().UTC()
		rc := base
		rc.scenario = name

		result := runScenario(&rc, name)
		result.Name = name
		result.StartedAt = scenarioStarted
		result.DurationSeconds = seconds(time.Since(scenarioStarted))
		if len(result.Failures) > 0 {
			result.Status = "FAIL"
			summary.Status = "FAIL"
		} else if result.Status == "" {
			result.Status = "PASS"
		}
		summary.Scenarios = append(summary.Scenarios, result)
		art.logf("========== SCENARIO %s %s (%.3fs) ==========", name, result.Status, result.DurationSeconds)
		for _, failure := range result.Failures {
			art.logf("FAIL: %s", failure)
		}
	}

	summary.ServerEvidence = art.collectServerEvidence()
	summary.FinishedAt = time.Now().UTC()
	summary.DurationSeconds = seconds(summary.FinishedAt.Sub(started))
	if err := art.writeSummary(summary); err != nil {
		return err
	}

	art.logf("\n%s: %d scenario(s); server_events=%d", summary.Status, len(summary.Scenarios), summary.ServerEvidence.MatchedEvents)
	art.logf("artifacts: %s", cfg.Out)
	if summary.Status != "PASS" {
		return errors.New("one or more reliability scenarios failed")
	}
	return nil
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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
)

type modelResults struct {
	Model    benchmarkModel
	Attempts []result
}

type runRecord struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitzero"`
	Status   string    `json:"status"`
	Machine  string    `json:"machine,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// Bump this label when the prompt or acceptance rules intentionally change.
const workloadVersion = "v1"

func loadRunRecord(runDir string) (runRecord, error) {
	data, err := os.ReadFile(filepath.Join(runDir, "report.txt"))
	if err == nil {
		var record runRecord
		for line := range strings.SplitSeq(string(data), "\n") {
			key, value, _ := strings.Cut(line, ": ")
			switch key {
			case "Started", "Finished":
				stamp, err := time.Parse(time.RFC3339Nano, value)
				if err != nil {
					return runRecord{}, fmt.Errorf("codegen benchmark: reading report timestamp: %w", err)
				}
				if key == "Started" {
					record.Started = stamp
				} else {
					record.Finished = stamp
				}
			case "Workflow":
				record.Status = value
			case "Runner machine":
				record.Machine = value
			case "Error":
				record.Error = value
			}
		}
		return record, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return runRecord{}, err
	}

	// Preserve metadata when regenerating older Markdown/JSON reports.
	data, err = os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return runRecord{}, err
	}
	_, rest, found := strings.Cut(string(data), "## Run metadata\n\n```json\n")
	if found {
		body, _, closed := strings.Cut(rest, "\n```")
		if !closed {
			return runRecord{}, errors.New("codegen benchmark: incomplete run metadata in report")
		}
		data = []byte(body)
	} else {
		data, err = os.ReadFile(filepath.Join(runDir, "run.json"))
		if errors.Is(err, os.ErrNotExist) {
			return runRecord{}, nil
		}
		if err != nil {
			return runRecord{}, err
		}
	}
	var record runRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return runRecord{}, fmt.Errorf("codegen benchmark: decoding run metadata: %w", err)
	}
	return record, nil
}

func generateReport(runDir string, record *runRecord) error {
	if record == nil {
		saved, err := loadRunRecord(runDir)
		if err != nil {
			return err
		}
		record = &saved
	}
	models, err := loadResults(runDir)
	if err != nil {
		return err
	}
	path := filepath.Join(runDir, "report.txt")
	if err := os.WriteFile(path, []byte(renderReport(runDir, models, *record)), 0o644); err != nil {
		return fmt.Errorf("codegen benchmark: writing report: %w", err)
	}
	fmt.Printf("Report written to %s\n", path)
	return nil
}

func loadResults(runDir string) ([]modelResults, error) {
	paths, err := filepath.Glob(filepath.Join(runDir, "*", "attempt-*", "result.json"))
	if err != nil {
		return nil, fmt.Errorf("codegen benchmark: finding results: %w", err)
	}
	byModel := make(map[string][]result)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("codegen benchmark: reading %s: %w", path, err)
		}
		var item result
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("codegen benchmark: decoding %s: %w", path, err)
		}
		byModel[item.Model.ID] = append(byModel[item.Model.ID], item)
	}
	models := make([]modelResults, 0, len(byModel))
	for _, attempts := range byModel {
		slices.SortFunc(attempts, func(a, b result) int { return a.Attempt - b.Attempt })
		models = append(models, modelResults{Model: attempts[0].Model, Attempts: attempts})
	}
	slices.SortFunc(models, func(a, b modelResults) int { return strings.Compare(a.Model.ID, b.Model.ID) })
	return models, nil
}

func renderReport(runDir string, models []modelResults, record runRecord) string {
	var report strings.Builder
	fmt.Fprintf(&report, "Run: %s\n", filepath.Base(filepath.Clean(runDir)))
	fmt.Fprintf(&report, "Workload: %s\n", workloadVersion)
	fmt.Fprintf(&report, "Started: %s\n", record.Started.Format(time.RFC3339Nano))
	if !record.Finished.IsZero() {
		fmt.Fprintf(&report, "Finished: %s\nElapsed: %s\n", record.Finished.Format(time.RFC3339Nano), record.Finished.Sub(record.Started).Round(time.Second))
	}
	fmt.Fprintf(&report, "Runner machine: %s\nWorkflow: %s\n", record.Machine, record.Status)
	if record.Error != "" {
		fmt.Fprintf(&report, "Error: %s\n", strings.Join(strings.Fields(record.Error), " "))
	}
	if len(models) == 0 {
		report.WriteString("\nNo completed submissions.\n")
		return report.String()
	}
	report.WriteByte('\n')
	table := tabwriter.NewWriter(&report, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "Model\tSubmissions\tOutcome\tChecks\tAgent time")
	fmt.Fprintln(table, "-----\t-----------\t-------\t------\t----------")
	for _, model := range models {
		final := model.Attempts[len(model.Attempts)-1]
		status := "Not passed"
		if final.OpenCode.Error != "" {
			status = "Agent error"
		}
		var wall time.Duration
		for _, attempt := range model.Attempts {
			wall += attempt.OpenCode.WallTime
			if attempt.Grade.Total > 0 && attempt.Grade.Passed == attempt.Grade.Total {
				status = fmt.Sprintf("Passed on submission %d", attempt.Attempt)
			}
		}
		checks := "Not graded"
		if final.Grade.Total > 0 {
			checks = fmt.Sprintf("%d/%d", final.Grade.Passed, final.Grade.Total)
		}
		fmt.Fprintf(table, "%s\t%d\t%s\t%s\t%s\n", model.Model.ID, len(model.Attempts), status, checks, wall.Round(time.Second))
	}
	_ = table.Flush()
	return report.String()
}

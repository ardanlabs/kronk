package main

import (
	"cmp"
	"encoding/json"
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
	Workload     string
	Steps        string
	Started      time.Time
	Finished     time.Time
	Status       string
	Machine      string
	KronkVersion string
	LlamaVersion string
	Error        string
}

const workloadVersion = "v1"

func loadRunRecord(runDir string) (runRecord, error) {
	data, err := os.ReadFile(filepath.Join(runDir, "report.txt"))
	if err != nil {
		return runRecord{}, err
	}
	var record runRecord
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, _ := strings.Cut(line, ": ")
		switch key {
		case "Workload":
			record.Workload = value
		case "Steps":
			record.Steps = value
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
		case "Kronk version":
			record.KronkVersion = value
		case "llama.cpp library":
			record.LlamaVersion = value
		case "Error":
			record.Error = value
		}
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
	fmt.Fprintf(&report, "Workload: %s\n", cmp.Or(record.Workload, workloadVersion))
	if record.Steps != "" {
		fmt.Fprintf(&report, "Steps: %s\n", record.Steps)
	}
	fmt.Fprintf(&report, "Started: %s\n", record.Started.Format(time.RFC3339Nano))
	if !record.Finished.IsZero() {
		fmt.Fprintf(&report, "Finished: %s\nElapsed: %s\n", record.Finished.Format(time.RFC3339Nano), record.Finished.Sub(record.Started).Round(time.Second))
	}
	fmt.Fprintf(&report, "Runner machine: %s\n", record.Machine)
	fmt.Fprintf(&report, "Kronk version: %s\nllama.cpp library: %s\n", cmp.Or(record.KronkVersion, "not recorded"), cmp.Or(record.LlamaVersion, "not recorded"))
	fmt.Fprintf(&report, "Workflow: %s\n", record.Status)
	if record.Error != "" {
		fmt.Fprintf(&report, "Error: %s\n", strings.Join(strings.Fields(record.Error), " "))
	}
	if len(models) == 0 {
		report.WriteString("\nNo completed submissions.\n")
		return report.String()
	}
	report.WriteByte('\n')
	table := tabwriter.NewWriter(&report, 0, 4, 2, ' ', 0)
	steps := strings.Split(cmp.Or(record.Steps, basicGameStep), ", ")
	titles := map[string]string{
		basicGameStep: "Step 1 (Basic game)",
		undoMoveStep:  "Step 2 (Undo move)",
	}
	fmt.Fprint(table, "Model")
	for _, step := range steps {
		fmt.Fprintf(table, "\t%s", cmp.Or(titles[step], step))
	}
	fmt.Fprintln(table)
	fmt.Fprint(table, "-----")
	for _, step := range steps {
		fmt.Fprintf(table, "\t%s", strings.Repeat("-", len(cmp.Or(titles[step], step))))
	}
	fmt.Fprintln(table)
	for _, model := range models {
		fmt.Fprint(table, model.Model.ID)
		for _, step := range steps {
			var attempts []result
			for _, attempt := range model.Attempts {
				if cmp.Or(attempt.Step, basicGameStep) == step {
					attempts = append(attempts, attempt)
				}
			}
			summary := "Not reached"
			if len(attempts) > 0 {
				final := attempts[len(attempts)-1]
				status, checks := "Failed", "Not graded"
				if final.OpenCode.Error != "" {
					status = "Agent error"
				}
				if final.Grade.Total > 0 {
					checks = fmt.Sprintf("%d/%d", final.Grade.Passed, final.Grade.Total)
				}
				var wall time.Duration
				var generated int
				var usageReported bool
				for _, attempt := range attempts {
					wall += attempt.OpenCode.WallTime
					usage := attempt.OpenCode.Usage
					generated += usage.Output + usage.Reasoning
					usageReported = usageReported || usage != (tokenUsage{})
					if attempt.Grade.Total > 0 && attempt.Grade.Passed == attempt.Grade.Total {
						status = "Passed"
					}
				}
				tokenSummary := "generated-toks: not reported"
				if usageReported {
					tokenSummary = fmt.Sprintf("generated-toks: %d", generated)
				}
				summary = fmt.Sprintf("%s (attempts %d, checks %s, time %s, %s)", status, len(attempts), checks, wall.Round(time.Second), tokenSummary)
			}
			fmt.Fprintf(table, "\t%s", summary)
		}
		fmt.Fprintln(table)
	}
	_ = table.Flush()
	return report.String()
}

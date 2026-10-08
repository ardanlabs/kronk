package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type modelResults struct {
	Model    benchmarkModel
	Attempts []result
}

func generateReport(runDir string) error {
	models, err := loadResults(runDir)
	if err != nil {
		return err
	}
	report := renderReport(runDir, models)
	path := filepath.Join(runDir, "report.md")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
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
	if len(paths) == 0 {
		return nil, fmt.Errorf("codegen benchmark: no results found in %s", runDir)
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

func renderReport(runDir string, models []modelResults) string {
	var report strings.Builder
	fmt.Fprintf(&report, "# OpenCode code-generation benchmark v2\n\n")
	fmt.Fprintf(&report, "Run: `%s`  \n", filepath.Base(filepath.Clean(runDir)))
	fmt.Fprintf(&report, "Generated: %s  \n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&report, "Models: %d\n\n", len(models))

	report.WriteString("## Summary\n\n")
	report.WriteString("Submissions are successive repairs in one workspace and agent session, not independent samples. Passing requires all 20 original grader checks. `First pass` is the submission number, or `Not passed`; agent exit status alone does not establish correctness.\n\n")
	report.WriteString("| Model | Submissions | First pass | Final checks passed | Final buildable | Final agent exit |\n")
	report.WriteString("| --- | ---: | --- | --- | --- | ---: |\n")
	for _, model := range models {
		firstPass := "Not passed"
		for _, attempt := range model.Attempts {
			if attempt.Grade.Passed == attempt.Grade.Total {
				firstPass = fmt.Sprint(attempt.Attempt)
				break
			}
		}
		final := model.Attempts[len(model.Attempts)-1]
		fmt.Fprintf(&report, "| `%s` | %d | %s | %d/%d (%.2f%%) | %t | %d |\n",
			model.Model.ID,
			len(model.Attempts),
			firstPass,
			final.Grade.Passed, final.Grade.Total, final.Grade.Percentage(),
			final.Grade.PassedCheck("go-build"), final.OpenCode.ExitCode,
		)
	}

	report.WriteString("\n## Agent work\n\n")
	report.WriteString("All columns are cumulative across submissions, including failed work. Token columns sum the usage from each agent turn's final OpenCode usage event. Wall time excludes warm-up and grading.\n\n")
	report.WriteString("| Model | Total agent wall | Total turns | Total tools | Input tokens | Output tokens | Reasoning tokens | Cache read |\n")
	report.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, model := range models {
		var wall time.Duration
		var turns, tools int
		var usage tokenUsage
		for _, attempt := range model.Attempts {
			wall += attempt.OpenCode.WallTime
			turns += attempt.OpenCode.Turns
			tools += attempt.OpenCode.ToolCalls
			usage.Input += attempt.OpenCode.Usage.Input
			usage.Output += attempt.OpenCode.Usage.Output
			usage.Reasoning += attempt.OpenCode.Usage.Reasoning
			usage.CacheRead += attempt.OpenCode.Usage.CacheRead
		}
		fmt.Fprintf(&report, "| `%s` | %.2f s | %d | %d | %d | %d | %d | %d |\n",
			model.Model.ID,
			wall.Seconds(), turns, tools,
			usage.Input, usage.Output, usage.Reasoning, usage.CacheRead,
		)
	}

	report.WriteString("\n## Configuration\n\n")
	report.WriteString("| Model | Context | Output limit |\n")
	report.WriteString("| --- | ---: | ---: |\n")
	for _, model := range models {
		fmt.Fprintf(&report, "| `%s` | %d | %d |\n", model.Model.ID, model.Model.ContextWindow, model.Model.OutputLimit)
	}

	report.WriteString("\n## Submissions\n")
	for _, model := range models {
		fmt.Fprintf(&report, "\n### `%s`\n", model.Model.ID)
		for _, attempt := range model.Attempts {
			fmt.Fprintf(&report, "\n#### Submission %d\n\n", attempt.Attempt)
			fmt.Fprintf(&report, "- Score: %d/%d (%.2f%%)\n", attempt.Grade.Passed, attempt.Grade.Total, attempt.Grade.Percentage())
			fmt.Fprintf(&report, "- OpenCode: exit %d, %d turns, %d tool calls, %s\n",
				attempt.OpenCode.ExitCode, attempt.OpenCode.Turns, attempt.OpenCode.ToolCalls, attempt.OpenCode.WallTime.Round(time.Second))
			if attempt.OpenCode.Error != "" {
				fmt.Fprintf(&report, "- OpenCode error: `%s`\n", strings.ReplaceAll(attempt.OpenCode.Error, "`", "'"))
			}
			writeFailures(&report, attempt.Grade)
		}
	}
	return report.String()
}

func writeFailures(report *strings.Builder, grade gradeResult) {
	for _, check := range grade.Checks {
		if check.Passed {
			continue
		}
		fmt.Fprintf(report, "\n**Failure — `%s`**\n\n```text\n%s\n```\n", check.Name, check.Detail)
	}
}

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
	fmt.Fprintf(&report, "# OpenCode code-generation benchmark\n\n")
	fmt.Fprintf(&report, "Run: `%s`  \n", filepath.Base(filepath.Clean(runDir)))
	fmt.Fprintf(&report, "Generated: %s  \n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&report, "Models: %d\n\n", len(models))

	report.WriteString("## Summary\n\n")
	report.WriteString("`Checks passed` is the aggregate grader score. `Perfect attempts` is the percentage of attempts that passed every check; it does not indicate whether the program built or ran.\n\n")
	report.WriteString("| Model | Attempts | Checks passed | Buildable attempts | Perfect attempts | Agent completed |\n")
	report.WriteString("| --- | ---: | ---: | ---: | ---: | ---: |\n")
	for _, model := range models {
		var passed, total, buildable, fullyPassed, agentSuccess int
		for _, attempt := range model.Attempts {
			passed += attempt.Grade.Passed
			total += attempt.Grade.Total
			if attempt.Grade.PassedCheck("go-build") {
				buildable++
			}
			if attempt.Grade.Passed == attempt.Grade.Total {
				fullyPassed++
			}
			if attempt.OpenCode.ExitCode == 0 {
				agentSuccess++
			}
		}
		fmt.Fprintf(&report, "| `%s` | %d | %.2f%% | %.2f%% | %.2f%% | %.2f%% |\n",
			model.Model.ID,
			len(model.Attempts),
			percentage(passed, total),
			percentage(buildable, len(model.Attempts)),
			percentage(fullyPassed, len(model.Attempts)),
			percentage(agentSuccess, len(model.Attempts)),
		)
	}

	report.WriteString("\n## Agent work\n\n")
	report.WriteString("Token columns sum the usage from each agent turn's final OpenCode usage event.\n\n")
	report.WriteString("| Model | Wall/attempt | Turns/attempt | Tools/attempt | Input tokens | Output tokens | Reasoning tokens | Cache read |\n")
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
		count := len(model.Attempts)
		fmt.Fprintf(&report, "| `%s` | %.2f s | %.1f | %.1f | %.0f | %.0f | %.0f | %.0f |\n",
			model.Model.ID,
			wall.Seconds()/float64(count),
			float64(turns)/float64(count),
			float64(tools)/float64(count),
			float64(usage.Input)/float64(count),
			float64(usage.Output)/float64(count),
			float64(usage.Reasoning)/float64(count),
			float64(usage.CacheRead)/float64(count),
		)
	}

	report.WriteString("\n## Configuration\n\n")
	report.WriteString("| Model | Context | Output limit |\n")
	report.WriteString("| --- | ---: | ---: |\n")
	for _, model := range models {
		fmt.Fprintf(&report, "| `%s` | %d | %d |\n", model.Model.ID, model.Model.ContextWindow, model.Model.OutputLimit)
	}

	report.WriteString("\n## Attempts\n")
	for _, model := range models {
		fmt.Fprintf(&report, "\n### `%s`\n", model.Model.ID)
		for _, attempt := range model.Attempts {
			fmt.Fprintf(&report, "\n#### Attempt %d\n\n", attempt.Attempt)
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

func percentage(passed, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(passed) / float64(total)
}

package main

import (
	"encoding/json"
	"errors"
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

type runRecord struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished,omitzero"`
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
}

const runMetadataHeader = "## Run metadata\n\n```json\n"

// Read the report's metadata when regenerating it. Older runs used run.json;
// read that format as a fallback, but never create it for new runs.
func loadRunRecord(runDir string) (runRecord, error) {
	data, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return runRecord{}, err
	}
	_, rest, found := strings.Cut(string(data), runMetadataHeader)
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
	report := renderReport(runDir, models, *record)
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
	fmt.Fprintf(&report, "# OpenCode code-generation benchmark v2\n\n")
	fmt.Fprintf(&report, "Run: `%s`  \n", filepath.Base(filepath.Clean(runDir)))
	fmt.Fprintf(&report, "Generated: %s  \n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&report, "Models: %d\n\n", len(models))
	if record.Status != "" {
		data, _ := json.MarshalIndent(record, "", "  ")
		fmt.Fprintf(&report, "%s%s\n```\n\n", runMetadataHeader, data)
		fmt.Fprintf(&report, "Run status: **%s**. This describes workflow completion, not acceptance.\n\n", record.Status)
		if record.Error != "" {
			fmt.Fprintf(&report, "Run error: %s\n\n", record.Error)
		}
	}
	report.WriteString("Only completed grades are included below. An attempt folder without result.json is incomplete, not a pass.\n\n")

	report.WriteString("## Summary\n\n")
	report.WriteString("Submissions are successive repairs in one workspace and agent session, not independent samples. Passing requires every acceptance check. Check percentages are diagnostic, not a percentage of gameplay correctness. `First pass` is the submission number, or `Not passed`; agent exit status alone does not establish correctness.\n\n")
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
			dir := filepath.ToSlash(filepath.Join(modelDirName(model.Model.ID), fmt.Sprintf("attempt-%02d", attempt.Attempt)))
			fmt.Fprintf(&report, "- [Grader result and raw execution evidence](%s/result.json)\n", dir)
			if attempt.SourceSHA != "" {
				fmt.Fprintf(&report, "- [Submitted program](%s/program/main.go), SHA-256: `%s`\n", dir, attempt.SourceSHA)
				fmt.Fprintf(&report, "- Play from this run directory (locally or after SSH login):\n\n```sh\n(cd '%s/program' && GOTOOLCHAIN=local go run .)\n```\n", dir)
			} else {
				report.WriteString("- No submitted source recorded.\n")
			}
			writeFailures(&report, attempt.Grade)
		}
	}
	return report.String()
}

// Bump this label when the task or acceptance rules intentionally change.
const workloadVersion = "v1"

func appendHistory(path, runDir string, record runRecord, selected []benchmarkModel) error {
	models, err := loadResults(runDir)
	if err != nil {
		return err
	}
	byModel := make(map[string][]result, len(models))
	for _, model := range models {
		byModel[model.Model.ID] = model.Attempts
	}

	var entry strings.Builder
	fmt.Fprintf(&entry, "\n## %s — %s\n\n", record.Started.Format(time.RFC3339), filepath.Base(runDir))
	fmt.Fprintf(&entry, "- Workload: %s\n- Workflow: %s\n- Elapsed: %s\n", workloadVersion, record.Status, record.Finished.Sub(record.Started).Round(time.Second))
	if record.Error != "" {
		fmt.Fprintf(&entry, "- Error: %s\n", strings.Join(strings.Fields(record.Error), " "))
	}
	for _, model := range selected {
		attempts := byModel[model.ID]
		status := "No completed submission"
		var wall time.Duration
		if len(attempts) > 0 {
			final := attempts[len(attempts)-1]
			status = fmt.Sprintf("Not passed (%d/%d checks)", final.Grade.Passed, final.Grade.Total)
			if final.OpenCode.Error != "" {
				status = "Agent error; " + status
			}
			for _, attempt := range attempts {
				wall += attempt.OpenCode.WallTime
				if attempt.Grade.Total > 0 && attempt.Grade.Passed == attempt.Grade.Total {
					status = fmt.Sprintf("Passed on submission %d", attempt.Attempt)
				}
			}
		}
		fmt.Fprintf(&entry, "- `%s`: %s; agent time: %s\n", model.ID, status, wall.Round(time.Second))
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("codegen benchmark: opening history: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		if _, err := file.WriteString("# Code-generation benchmark history\n\nBasic results from version2 runs. Detailed artifacts under runs/ are gitignored.\n"); err != nil {
			return err
		}
	}
	_, err = file.WriteString(entry.String())
	return err
}

func writeFailures(report *strings.Builder, grade gradeResult) {
	for _, check := range grade.Checks {
		if check.Passed {
			continue
		}
		fmt.Fprintf(report, "\n**Failure — `%s`**\n\n```text\n%s\n```\n", check.Name, check.Detail)
	}
}

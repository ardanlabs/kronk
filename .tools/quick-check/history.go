package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// History tables retain rounded release timings, not raw evidence. The candidate
// column of the last entry is the baseline for the next run.
type historyBaseline struct {
	release   string
	machine   string
	reference string
	metrics   map[string]map[string]*measurement
}

var historyRow = regexp.MustCompile(`^\s*(?:(\S+)\s+)?(Workload|Normalized) \(s\)\s+(n/a|[0-9.eE+\-]+ \[(?:n/a|[0-9.eE+\-]+)\])\s+(n/a|[0-9.eE+\-]+ \[(?:n/a|[0-9.eE+\-]+)\])\s+\S+\s*$`)

func readHistoryBaseline(path, recordingRelease string) (historyBaseline, error) {
	b := historyBaseline{metrics: make(map[string]map[string]*measurement)}
	data, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	const title = "Release comparison: "
	if !strings.HasPrefix(string(data), title) {
		return b, fmt.Errorf("history: missing release comparison title")
	}
	sections := strings.Split(strings.TrimSpace(strings.TrimPrefix(string(data), title)), "\n\n"+title)
	// When updating an already-recorded release, compare to its predecessor,
	// not to the earlier measurement of that same release.
	selected := len(sections) - 1
	for recordingRelease != "" && selected >= 0 {
		label, _, _ := strings.Cut(sections[selected], "\n")
		_, candidate, ok := strings.Cut(label, " → ")
		if !ok || candidate != recordingRelease {
			break
		}
		selected--
	}
	if selected < 0 {
		return b, fmt.Errorf("history: no predecessor for release %s", recordingRelease)
	}
	lines := strings.Split(sections[selected], "\n")
	if len(lines) < 6 || !strings.HasPrefix(lines[1], "machine: ") || !strings.Contains(lines[4], "Baseline median [SD]") {
		return b, fmt.Errorf("history: invalid latest entry header")
	}
	label := lines[0]
	if _, candidate, ok := strings.Cut(label, " → "); ok {
		label = candidate
	}
	labels := strings.Fields(label)
	if len(labels) == 0 {
		return b, fmt.Errorf("history: missing release label")
	}
	b.release = labels[0]
	if !releasePattern.MatchString(b.release) {
		return b, fmt.Errorf("history: invalid release label %q", b.release)
	}
	b.machine = lines[1]
	_, b.reference, _ = strings.Cut(lines[2], "; reference ID: ")
	if b.reference == "" {
		return b, fmt.Errorf("history: missing normalization reference ID")
	}
	cell := ""
	for _, line := range lines[5:] {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "Saved-run checks:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 7 && fields[1] == "NOT_RUN" && fields[2] == "(baseline" && fields[4] == "/" && fields[5] == "candidate" && strings.HasSuffix(fields[6], ")") {
			if b.metrics[fields[0]] != nil {
				return b, fmt.Errorf("history: duplicate cell %q", fields[0])
			}
			b.metrics[fields[0]] = map[string]*measurement{"workload.elapsed_seconds": nil, "workload.reference_seconds": nil}
			cell = ""
			continue
		}
		row := historyRow.FindStringSubmatch(line)
		if row == nil {
			return b, fmt.Errorf("history: invalid timing row %q", line)
		}
		key := "workload.reference_seconds"
		if row[2] == "Workload" {
			cell, key = row[1], "workload.elapsed_seconds"
			if cell == "" || b.metrics[cell] != nil {
				return b, fmt.Errorf("history: missing or duplicate cell %q", cell)
			}
			b.metrics[cell] = make(map[string]*measurement)
		} else if cell == "" || row[1] != "" {
			return b, fmt.Errorf("history: normalized row without its workload row")
		}
		if _, exists := b.metrics[cell][key]; exists {
			return b, fmt.Errorf("history: duplicate metric for %s", cell)
		}
		// Read only the candidate measurement, never the older baseline column.
		var value *measurement
		if row[4] != "n/a" {
			parts := strings.Fields(row[4])
			median, err := historyNumber(parts[0])
			if err != nil {
				return b, err
			}
			m := measurement{Median: median}
			sd := strings.Trim(parts[1], "[]")
			if sd != "n/a" {
				n, err := historyNumber(sd)
				if err != nil {
					return b, err
				}
				m.SampleStdDev = new(n)
			}
			// Zero-filled placeholders are unavailable, not measured zero time.
			if median > 0 {
				value = &m
			}
		}
		b.metrics[cell][key] = value
	}
	if len(b.metrics) == 0 {
		return b, fmt.Errorf("history: latest entry has no cells")
	}
	for cell, metrics := range b.metrics {
		if len(metrics) != 2 {
			return b, fmt.Errorf("history: incomplete metrics for %s", cell)
		}
	}
	return b, nil
}

func historyNumber(text string) (float64, error) {
	n, err := strconv.ParseFloat(text, 64)
	if err != nil || n < 0 || math.IsInf(n, 0) || math.IsNaN(n) {
		return 0, fmt.Errorf("history: invalid nonnegative timing %q", text)
	}
	return n, nil
}

func historyCommand(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("quick-check compare-history", flag.ContinueOnError)
	flags.SetOutput(out)
	parent := flags.String("out", ".tools/quick-check/output", "candidate run queue and comparison output directory")
	history := flags.String("history", "", "tracked release-history log to read")
	release := flags.String("release", "", "optionally record the candidate as this release in the local history log")
	baseline := flags.String("baseline", "", "optionally require this latest recorded release label")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *history == "" || flags.NArg() != 0 || (*release != "" && !releasePattern.MatchString(*release)) {
		return fmt.Errorf("usage: quick-check compare-history -history PATH [-out DIR] [-release vX.Y.Z] [-baseline vX.Y.Z]")
	}
	b, err := readHistoryBaseline(*history, *release)
	if err != nil {
		return err
	}
	if *baseline != "" && *baseline != b.release {
		return fmt.Errorf("history baseline is %s, not requested %s", b.release, *baseline)
	}
	runs, err := savedRuns(*parent)
	if err != nil {
		return err
	}
	candidate := ""
	for _, run := range runs {
		if strings.HasPrefix(filepath.Base(run.path), "new-") {
			if candidate != "" {
				return fmt.Errorf("multiple fresh runs; use a separate candidate queue")
			}
			candidate = run.path
		}
	}
	if candidate == "" {
		return fmt.Errorf("history comparison needs a fresh new-* run")
	}
	input, err := readComparison(candidate)
	if err != nil {
		return err
	}
	machine := releaseHistoryMachine(input.summary)
	reference, err := releaseReferenceID(input)
	if err != nil {
		return err
	}
	c := matrixComparison{
		Status: "PASS", SavedRunChecks: map[string]string{"baseline": "RECORDED", "candidate": input.summary.Status},
		Baseline:  comparisonRun{Directory: *history, Version: "not retained in timing log", Release: b.release, Status: "RECORDED"},
		Candidate: comparisonRun{Directory: candidate, Version: input.summary.ServerRevision, Release: *release, Status: input.summary.Status},
	}
	ids := make(map[string]bool)
	for id := range b.metrics {
		ids[id] = true
	}
	for id := range input.cells {
		ids[id] = true
	}
	compared, available, incomplete := 0, false, false
	failed := input.summary.Status != "PASS" || len(input.problems) > 0
	for _, id := range sortedKeys(ids) {
		ci, exists := input.cells[id]
		cell := comparison{Cell: id, Status: "PASS", Warnings: []string{}, Limitations: []string{"Baseline values are rounded history medians and standard deviations, not raw samples or independently validated evidence."}}
		cell.SavedRunChecks = map[string]string{"baseline": "RECORDED", "candidate": "NOT_RUN"}
		if exists && ci.summary.Status != "NOT_RUN" {
			cell.Candidate = comparisonIdentity(candidate, ci)
			cell.SavedRunChecks["candidate"] = ci.summary.Status
			failed = failed || len(cell.Candidate.Problems) > 0
		}
		ran := exists && ci.summary.Status != "NOT_RUN"
		if !ran {
			cell.Status, incomplete = "NOT_RUN", true
		} else if ci.summary.Status != "PASS" || len(cell.Candidate.Problems) > 0 {
			cell.Status = "FAIL"
		}
		if machine == "" || machine != b.machine {
			cell.Warnings = append(cell.Warnings, "machine.metadata differs or is missing; timing deltas omitted")
		}
		if reference != b.reference {
			cell.Warnings = append(cell.Warnings, "Normalized delta omitted: reference token amounts or cell set differ")
		}
		samples := ci.metricSamples()
		for _, key := range []string{"workload.elapsed_seconds", "workload.reference_seconds"} {
			row := metricComparison{Metric: key, Baseline: b.metrics[id][key]}
			available = available || row.Baseline != nil
			if ran && len(cell.Candidate.Problems) == 0 && len(input.problems) == 0 && len(samples[key]) > 0 {
				row.Candidate = new(summarize(samples[key]))
			}
			if row.Baseline != nil && row.Candidate != nil && machine != "" && machine == b.machine && (key != "workload.reference_seconds" || reference == b.reference) {
				// Use the same decimal rounding as the history table writer.
				rounded, err := strconv.ParseFloat(fmt.Sprintf("%.2f", row.Candidate.Median), 64)
				if err != nil {
					return fmt.Errorf("rounding candidate timing: %w", err)
				}
				row.Delta = new(rounded - row.Baseline.Median)
				row.Percent = new(*row.Delta / row.Baseline.Median * 100)
				compared++
			} else {
				incomplete = true
				if row.Baseline == nil {
					cell.Warnings = append(cell.Warnings, key+": no initialized release baseline; delta unavailable")
				}
			}
			cell.Metrics = append(cell.Metrics, row)
		}
		c.Cells = append(c.Cells, cell)
	}
	switch {
	case failed:
		c.Status = "FAIL"
	case !available:
		c.Status, c.Baseline.Status, c.SavedRunChecks["baseline"] = "NO_BASELINE", "UNINITIALIZED", "UNINITIALIZED"
	case compared == 0:
		c.Status = "NOT_COMPARABLE"
	case incomplete:
		c.Status = "PARTIAL"
	}
	var text, markdown strings.Builder
	if err := c.writeText(&text); err != nil {
		return err
	}
	if err := c.writeMarkdown(&markdown); err != nil {
		return err
	}
	const note = "Timing deltas use the tracked history log's latest candidate values, rounding medians to the log's 0.01-second precision. Positive percentages mean slower; PASS is not a performance-regression verdict.\n"
	text.WriteString("\n" + note)
	markdown.WriteString("\n" + note)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	for name, data := range map[string][]byte{"comparison.json": append(data, '\n'), "comparison.log": []byte(text.String()), "comparison.md": []byte(markdown.String())} {
		if err := os.WriteFile(filepath.Join(*parent, name), data, 0600); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(out, text.String()); err != nil {
		return err
	}
	if failed {
		return errComparisonFindings
	}
	if *release != "" {
		if machine == "" || (available && machine != b.machine) {
			return fmt.Errorf("cannot record release with missing or mismatched machine metadata")
		}
		// The saved log supplies the baseline timings. Supply the candidate's
		// machine to the history writer only after the compatibility check above.
		a := matrixInput{summary: input.summary}
		a.summary.Status = c.SavedRunChecks["baseline"]
		if err := recordReleaseComparison(*history, b.release, *release, a, input, c); err != nil {
			return err
		}
	}
	return nil
}

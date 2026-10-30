package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/tabwriter"
	"time"
)

type matrixInput struct {
	summary matrixSummary
	cells   map[string]comparisonInput
}

type matrixComparison struct {
	Status         string            `json:"comparison_status"`
	SavedRunChecks map[string]string `json:"saved_run_checks"`
	Baseline       comparisonRun     `json:"baseline"`
	Candidate      comparisonRun     `json:"candidate"`
	Cells          []comparison      `json:"cells"`
}

func readComparison(dir string) (matrixInput, error) {
	input := matrixInput{cells: make(map[string]comparisonInput)}
	data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return input, err
	}
	if err := json.Unmarshal(data, &input.summary); err != nil {
		return input, err
	}
	f, err := os.Open(filepath.Join(dir, "events.ndjson"))
	if err != nil {
		return input, err
	}
	defer f.Close()
	var r matrixReport
	reports := 0
	events := make(map[string][]byte)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 65536), 64*1024*1024)
	for scanner.Scan() {
		var event struct {
			Source string          `json:"source"`
			Cell   string          `json:"cell_id"`
			Report json.RawMessage `json:"report"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return input, fmt.Errorf("events.ndjson: %w", err)
		}
		if event.Source == "report" {
			reports++
			if err := json.Unmarshal(event.Report, &r); err != nil {
				return input, err
			}
			continue
		}
		if event.Cell == "" {
			return input, fmt.Errorf("event missing cell_id; rerun quick-check")
		}
		events[event.Cell] = append(events[event.Cell], scanner.Bytes()...)
		events[event.Cell] = append(events[event.Cell], '\n')
	}
	if err := scanner.Err(); err != nil {
		return input, err
	}
	if reports != 1 || len(r.Cells) == 0 {
		return input, fmt.Errorf("expected one matrix diagnostic report; rerun quick-check")
	}
	s := input.summary
	if (s.Status != "PASS" && s.Status != "FAIL" && s.Status != "INTERRUPTED") || len(s.Cells) != len(r.Cells) {
		return input, fmt.Errorf("missing matrix status/cells or summary and diagnostics disagree")
	}
	if _, err := time.Parse(time.RFC3339Nano, s.TestStartedAt); err != nil {
		return input, fmt.Errorf("missing or invalid test timestamp: %w", err)
	}
	expected := r.summary()
	expected.Cells, s.Cells = nil, nil
	if !reflect.DeepEqual(s, expected) {
		return input, fmt.Errorf("matrix summary and diagnostic provenance/status disagree")
	}
	attempted := 0
	failed := len(r.Findings) > 0
	for i, cell := range r.Cells {
		cs := input.summary.Cells[i]
		if cell.ID == "" || cell.ID != cs.ID {
			return input, fmt.Errorf("missing or inconsistent cell identity")
		}
		if _, exists := input.cells[cell.ID]; exists {
			return input, fmt.Errorf("duplicate cell %s", cell.ID)
		}
		if cell.Status == "NOT_RUN" {
			if cs.Status != "NOT_RUN" || len(events[cell.ID]) > 0 || len(cell.Repetitions) > 0 || len(cs.Repetitions) > 0 || cs.Measurements != nil || cell.WarmupRequest != nil || cell.SharedWarmupFrom != "" {
				return input, fmt.Errorf("%s: NOT_RUN cell has execution evidence", cell.ID)
			}
			input.cells[cell.ID] = comparisonInput{summary: cs, report: cell}
		} else {
			attempted++
			failed = failed || cell.Status == "FAIL"
			if cell.ServerRevision != r.ServerRevision || cell.ServerCommitTime != r.ServerCommitTime || cell.ReleaseTag != r.ReleaseTag || cell.TestStartedAt != r.TestStartedAt {
				return input, fmt.Errorf("%s: cell and run provenance disagree", cell.ID)
			}
			ci, err := readCellComparison(cs, cell, events[cell.ID])
			if err != nil {
				return input, fmt.Errorf("%s: %w", cell.ID, err)
			}
			if cell.SharedWarmupFrom != "" {
				owner, ok := input.cells[cell.SharedWarmupFrom]
				if !ok || owner.report.WarmupRequest == nil || owner.report.SharedWarmupFrom != "" ||
					owner.report.Model != cell.Model || owner.report.TwoSlots != cell.TwoSlots ||
					owner.report.NoCache != cell.NoCache || owner.report.TargetOnly != cell.TargetOnly ||
					owner.report.MTPBackend != cell.MTPBackend {
					return input, fmt.Errorf("%s: missing or incompatible shared warm-up source %s", cell.ID, cell.SharedWarmupFrom)
				}
				ci.sharedWarmupValidated = owner.summary.WarmupRequest["status"] == "PASS" && owner.report.WarmupRequest.Error == ""
				ci.native = owner.native
			}
			input.cells[cell.ID] = ci
		}
		delete(events, cell.ID)
	}
	if len(events) > 0 || attempted == 0 || (r.Status == "PASS" && failed) {
		return input, fmt.Errorf("unexpected cell events, no executed cells, or invalid PASS")
	}
	data, err = os.ReadFile(filepath.Join(dir, "tool.log"))
	if err != nil {
		return input, err
	}
	header := strings.Join(strings.SplitN(r.logSummary(), "\n", 4)[:3], "\n") + "\n"
	if !bytes.HasPrefix(data, []byte(header)) {
		return input, fmt.Errorf("tool.log verdict/provenance/timestamp disagrees with diagnostic report")
	}
	return input, nil
}

func compareMatrices(aDir, bDir string, a, b matrixInput) matrixComparison {
	c := matrixComparison{
		Status: "PASS", SavedRunChecks: map[string]string{"baseline": a.summary.Status, "candidate": b.summary.Status},
		Baseline:  comparisonRun{Directory: aDir, Version: a.summary.ServerVersion, Status: a.summary.Status, Problems: a.summary.Findings},
		Candidate: comparisonRun{Directory: bDir, Version: b.summary.ServerVersion, Status: b.summary.Status, Problems: b.summary.Findings},
		Cells:     []comparison{},
	}
	if a.summary.Status != "PASS" || b.summary.Status != "PASS" {
		c.Status = "FAIL"
	}
	ids := make(map[string]bool)
	for id := range a.cells {
		ids[id] = true
	}
	for id := range b.cells {
		ids[id] = true
	}
	matched := 0
	for _, id := range sortedKeys(ids) {
		x, xOK := a.cells[id]
		y, yOK := b.cells[id]
		xRan, yRan := xOK && x.summary.Status != "NOT_RUN", yOK && y.summary.Status != "NOT_RUN"
		if xRan && yRan {
			cell := compareRuns(aDir, bDir, x, y)
			if cell.Status != "PASS" {
				c.Status = "FAIL"
			}
			c.Cells = append(c.Cells, cell)
			matched++
			continue
		}
		status := func(ci comparisonInput, ran bool) string {
			if !ran {
				return "NOT_RUN"
			}
			return ci.summary.Status
		}
		c.Cells = append(c.Cells, comparison{Cell: id, Status: "NOT_RUN", SavedRunChecks: map[string]string{"baseline": status(x, xRan), "candidate": status(y, yRan)}})
	}
	// A partial matrix is allowed, but a comparison with no common exercised
	// cell is not a successful comparison.
	if matched == 0 && c.Status == "PASS" {
		c.Status = "NOT_RUN"
	}
	return c
}

func (c matrixComparison) writeText(out io.Writer) error {
	fmt.Fprintf(out, "Comparison status: %s\nSaved-run checks: baseline %s / candidate %s\nBaseline: %s\nBaseline commit: %s\nCandidate: %s\nCandidate commit: %s\n",
		c.Status, c.Baseline.Status, c.Candidate.Status, c.Baseline.Directory, c.Baseline.Version, c.Candidate.Directory, c.Candidate.Version)
	if c.Baseline.Release != "" {
		fmt.Fprintf(out, "Release labels: %s → %s\n", c.Baseline.Release, c.Candidate.Release)
	}
	for _, run := range []comparisonRun{c.Baseline, c.Candidate} {
		for _, problem := range run.Problems {
			fmt.Fprintf(out, "Problem [%s]: %s\n", run.Directory, problem)
		}
	}
	machineWarning := ""
	compared := 0
	warningCells := make(map[string]int)
	for _, cell := range c.Cells {
		if cell.Status == "NOT_RUN" {
			continue
		}
		compared++
		seen := make(map[string]bool)
		for _, warning := range cell.textWarnings() {
			if !seen[warning] {
				warningCells[warning]++
				seen[warning] = true
			}
			if strings.HasPrefix(warning, "machine.") {
				machineWarning = "WARNING: MACHINE METADATA DIFFERS OR IS MISSING!"
			}
		}
	}
	commonWarnings := make(map[string]bool)
	for _, warning := range sortedKeys(warningCells) {
		if warningCells[warning] == compared {
			if len(commonWarnings) == 0 {
				fmt.Fprintln(out, "\nWarnings applying to all compared cells:")
			}
			fmt.Fprintln(out, "Warning:", warning)
			commonWarnings[warning] = true
		}
	}
	if machineWarning != "" {
		fmt.Fprintln(out, "\n"+machineWarning)
	}
	for _, cell := range c.Cells {
		if cell.Status == "NOT_RUN" {
			continue
		}
		for _, run := range []comparisonRun{cell.Baseline, cell.Candidate} {
			for _, problem := range run.Problems {
				fmt.Fprintf(out, "Problem [%s; %s]: %s\n", cell.Cell, run.Directory, problem)
			}
		}
		for _, warning := range cell.textWarnings() {
			if !commonWarnings[warning] {
				fmt.Fprintf(out, "Warning [%s]: %s\n", cell.Cell, warning)
			}
		}
	}
	if err := c.writeTable(out); err != nil {
		return err
	}
	if machineWarning != "" {
		fmt.Fprintln(out, "\n"+machineWarning)
	}
	return nil
}

func (c matrixComparison) writeTable(out io.Writer) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "\nCell\tMetric\tBaseline median [SD]\tCandidate median [SD]\tChange")
	// One table, with a blank line between each cell's two timing rows.
	for _, cell := range c.Cells {
		if cell.Status == "NOT_RUN" {
			fmt.Fprintf(w, "\t\t\t\t\n%s\tNOT_RUN (baseline %s / candidate %s)\n", cell.Cell, cell.SavedRunChecks["baseline"], cell.SavedRunChecks["candidate"])
			continue
		}
		cell.writeRows(w)
	}
	return w.Flush()
}

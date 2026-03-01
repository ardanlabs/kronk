// benchfmt parses raw Go benchmark output from the runs/ directory and
// rewrites BENCH_RESULTS.txt with formatted comparison grids.
//
// The tool preserves the documentation header in BENCH_RESULTS.txt (everything
// up to and including the first ======= separator) and replaces everything
// below it with formatted results from each run, newest first.
//
// Usage:
//
//	go run cmd/server/api/tooling/benchfmt/main.go              # rebuild all
//	go run cmd/server/api/tooling/benchfmt/main.go 2026-03-01.txt  # append one run
//
// When a filename is given the tool parses only that file, diffs it against
// the chronologically previous run in runs/, and inserts the formatted
// section at the top of BENCH_RESULTS.txt (after the legend line).
//
// Or via make:
//
//	make benchmark-fmt
package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// =============================================================================

const (
	runsDir     = "sdk/kronk/tests/benchmarks/runs"
	resultsFile = "sdk/kronk/tests/benchmarks/BENCH_RESULTS.txt"
	separator   = "================================================================================"
)

// Thresholds for coloring. Values within this range are neutral.
const neutralThreshold = 2.0

// =============================================================================

// bench holds parsed metrics from a single benchmark result line.
type bench struct {
	name     string
	nsOp     float64
	bytesOp  float64
	allocsOp float64
	tokS     float64
	ttftMS   float64
	totalMS  float64
}

// run holds all benchmarks from a single run file plus metadata.
type run struct {
	filename string
	header   runHeader
	benchs   map[string]bench
	order    []string
}

// runHeader holds metadata parsed from the raw output.
type runHeader struct {
	date   string
	kronk  string
	llama  string
	goos   string
	goarch string
	cpu    string
}

// =============================================================================

func main() {
	workspace := os.Getenv("GITHUB_WORKSPACE")
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		workspace = wd
	}

	runsPath := filepath.Join(workspace, runsDir)
	resultsPath := filepath.Join(workspace, resultsFile)

	// Single-file append mode.
	if len(os.Args) > 1 {
		if err := appendSingleRun(runsPath, resultsPath, os.Args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Full rebuild mode: parse all runs and rewrite the results file.
	rebuildAll(runsPath, resultsPath)
}

// rebuildAll parses every run file and rewrites BENCH_RESULTS.txt from scratch.
func rebuildAll(runsPath, resultsPath string) {

	// Read the documentation header from BENCH_RESULTS.txt.
	header, err := readHeader(resultsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading header: %v\n", err)
		os.Exit(1)
	}

	// Parse all run files.
	runs, err := parseAllRuns(runsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing runs: %v\n", err)
		os.Exit(1)
	}

	if len(runs) == 0 {
		fmt.Fprintln(os.Stderr, "no benchmark run files found in", runsPath)
		os.Exit(1)
	}

	// Sort runs by filename (date order), newest first for display.
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].filename > runs[j].filename
	})

	// Build the output.
	var out strings.Builder

	// Write the preserved header.
	out.WriteString(header)
	out.WriteString("\n\n")

	// Legend.
	out.WriteString("🟢 Better   🔴 Worse   ⚪ Neutral\n\n")

	// Write each run, comparing against the next (older) run.
	for i, r := range runs {
		var prev *run
		if i+1 < len(runs) {
			prev = &runs[i+1]
		}
		writeRun(&out, r, prev)
	}

	// Write the file.
	if err := os.WriteFile(resultsPath, []byte(out.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing results: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s (%d runs)\n", resultsPath, len(runs))
}

// appendSingleRun parses one run file, diffs it against the chronologically
// previous run, and inserts the formatted section at the top of the results
// (right after the legend line), leaving existing entries untouched.
func appendSingleRun(runsPath, resultsPath, filename string) error {

	// Parse the new run file.
	newRun, err := parseRunFile(filepath.Join(runsPath, filename))
	if err != nil {
		return fmt.Errorf("parsing %s: %w", filename, err)
	}
	newRun.filename = filename

	// Find the chronologically previous run for comparison.
	prev, err := findPreviousRun(runsPath, filename)
	if err != nil {
		return fmt.Errorf("finding previous run: %w", err)
	}

	// Read the existing results file.
	existing, err := os.ReadFile(resultsPath)
	if err != nil {
		return fmt.Errorf("reading results: %w", err)
	}

	// Locate the legend line so we can insert right after it.
	content := string(existing)
	const legend = "🟢 Better   🔴 Worse   ⚪ Neutral"

	legendIdx := strings.Index(content, legend)
	if legendIdx < 0 {
		return fmt.Errorf("legend line not found in %s", resultsPath)
	}

	// Advance past the legend and its trailing newlines.
	insertAt := legendIdx + len(legend)
	for insertAt < len(content) && content[insertAt] == '\n' {
		insertAt++
	}

	// Format the new run section.
	var runBuf strings.Builder
	writeRun(&runBuf, newRun, prev)

	// Rebuild the file: everything before the insertion point, the new run,
	// then the existing run sections.
	var out strings.Builder
	out.WriteString(content[:insertAt])
	out.WriteString(runBuf.String())
	out.WriteString(content[insertAt:])

	if err := os.WriteFile(resultsPath, []byte(out.String()), 0644); err != nil {
		return fmt.Errorf("writing results: %w", err)
	}

	fmt.Printf("appended %s to %s\n", filename, resultsPath)
	return nil
}

// findPreviousRun returns the most recent run whose filename sorts before
// the given filename, or nil if there is no earlier run.
func findPreviousRun(runsPath, filename string) (*run, error) {
	entries, err := os.ReadDir(runsPath)
	if err != nil {
		return nil, err
	}

	var prevName string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		if e.Name() < filename && e.Name() > prevName {
			prevName = e.Name()
		}
	}

	if prevName == "" {
		return nil, nil
	}

	r, err := parseRunFile(filepath.Join(runsPath, prevName))
	if err != nil {
		return nil, err
	}
	r.filename = prevName
	return &r, nil
}

// =============================================================================

// readHeader reads BENCH_RESULTS.txt and returns everything up to and
// including the last separator line before any benchmark data. The header
// section uses separators for formatting (e.g. after the title), so we
// find the last separator that precedes any benchmark output.
func readHeader(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var lines []string
	lastSepIdx := -1

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if line == separator {
			lastSepIdx = len(lines) - 1
		}

		// Stop once we see benchmark data or the results section.
		if strings.HasPrefix(line, "🟢") || strings.HasPrefix(line, "🔴") || strings.HasPrefix(line, "⚪") {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if lastSepIdx < 0 {
		return strings.Join(lines, "\n"), nil
	}

	return strings.Join(lines[:lastSepIdx+1], "\n"), nil
}

// =============================================================================

// parseAllRuns reads all .txt files from the runs directory.
func parseAllRuns(dir string) ([]run, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var runs []run
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}

		r, err := parseRunFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		r.filename = e.Name()
		runs = append(runs, r)
	}

	return runs, nil
}

// parseRunFile reads a raw benchmark output file and returns parsed results.
func parseRunFile(path string) (run, error) {
	f, err := os.Open(path)
	if err != nil {
		return run{}, err
	}
	defer f.Close()

	r := run{
		benchs: make(map[string]bench),
	}

	// Regex for Go benchmark output line.
	benchRe := regexp.MustCompile(`^(Benchmark\S+)\s+\d+\s+(.+)$`)

	// Metric patterns.
	nsOpRe := regexp.MustCompile(`([\d.]+)\s+ns/op`)
	bytesOpRe := regexp.MustCompile(`([\d.]+)\s+B/op`)
	allocsOpRe := regexp.MustCompile(`([\d.]+)\s+allocs/op`)
	tokSRe := regexp.MustCompile(`([\d.]+)\s+tok/s`)
	ttftRe := regexp.MustCompile(`([\d.]+)\s+ttft-ms`)
	totalRe := regexp.MustCompile(`([\d.]+)\s+total-ms`)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Parse header metadata.
		switch {
		case strings.HasPrefix(line, "# Date:"):
			r.header.date = strings.TrimSpace(strings.TrimPrefix(line, "# Date:"))
		case strings.HasPrefix(line, "# Kronk:"):
			r.header.kronk = strings.TrimSpace(strings.TrimPrefix(line, "# Kronk:"))
		case strings.HasPrefix(line, "# Llama.cpp:"):
			r.header.llama = strings.TrimSpace(strings.TrimPrefix(line, "# Llama.cpp:"))
		case strings.HasPrefix(line, "bench: llama.cpp"):
			if r.header.llama == "" {
				parts := strings.Fields(line)
				if len(parts) >= 3 {
					r.header.llama = parts[2]
				}
			}
		case strings.HasPrefix(line, "goos:"):
			r.header.goos = strings.TrimSpace(strings.TrimPrefix(line, "goos:"))
		case strings.HasPrefix(line, "goarch:"):
			r.header.goarch = strings.TrimSpace(strings.TrimPrefix(line, "goarch:"))
		case strings.HasPrefix(line, "cpu:"):
			r.header.cpu = strings.TrimSpace(strings.TrimPrefix(line, "cpu:"))
		}

		m := benchRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		name := normalizeName(m[1])
		metrics := m[2]

		b := bench{name: name}

		if v := nsOpRe.FindStringSubmatch(metrics); v != nil {
			b.nsOp, _ = strconv.ParseFloat(v[1], 64)
		}
		if v := bytesOpRe.FindStringSubmatch(metrics); v != nil {
			b.bytesOp, _ = strconv.ParseFloat(v[1], 64)
		}
		if v := allocsOpRe.FindStringSubmatch(metrics); v != nil {
			b.allocsOp, _ = strconv.ParseFloat(v[1], 64)
		}
		if v := tokSRe.FindStringSubmatch(metrics); v != nil {
			b.tokS, _ = strconv.ParseFloat(v[1], 64)
		}
		if v := ttftRe.FindStringSubmatch(metrics); v != nil {
			b.ttftMS, _ = strconv.ParseFloat(v[1], 64)
		}
		if v := totalRe.FindStringSubmatch(metrics); v != nil {
			b.totalMS, _ = strconv.ParseFloat(v[1], 64)
		}

		r.benchs[name] = b
		r.order = append(r.order, name)
	}

	return r, scanner.Err()
}

// =============================================================================

// benchGroup defines a display group with a label and benchmark names.
type benchGroup struct {
	label string
	names []string
}

// displayGroups returns the benchmark groups in display order.
// Each group maps benchmark names to short display labels.
func displayGroups() []benchGroup {
	return []benchGroup{
		{
			label: "Dense",
			names: []string{
				"BenchmarkDense_NonCaching",
				"BenchmarkDense_SPC",
				"BenchmarkDense_IMCDeterministic",
				"BenchmarkDense_IMCNonDeterministic",
				"BenchmarkDense_IMCDeterministic_Speculative",
				"BenchmarkDense_IMCDeterministic_MultiSlot",
				"BenchmarkDense_IMC_PrefillOnly",
				"BenchmarkDense_IMC_ColdBuild",
			},
		},
		{
			label: "MoE",
			names: []string{
				"BenchmarkMoE_IMCDeterministic",
				"BenchmarkMoE_Speculative_Baseline",
				"BenchmarkMoE_Speculative_WithDraft",
			},
		},
		{
			label: "Hybrid",
			names: []string{
				"BenchmarkHybrid_IMCDeterministic",
			},
		},
	}
}

// shortName returns the display name for a benchmark.
func shortName(name string) string {
	m := map[string]string{
		"BenchmarkDense_NonCaching":                   "Dense NonCaching",
		"BenchmarkDense_SPC":                          "Dense SPC",
		"BenchmarkDense_IMCDeterministic":             "Dense IMC-Det",
		"BenchmarkDense_IMCNonDeterministic":          "Dense IMC-NonDet (GPT)",
		"BenchmarkDense_IMCDeterministic_Speculative": "Dense IMC-Det+Spec",
		"BenchmarkDense_IMCDeterministic_MultiSlot":   "Dense IMC-Det MultiSlot",
		"BenchmarkDense_IMC_PrefillOnly":              "Dense IMC Prefill",
		"BenchmarkDense_IMC_ColdBuild":                "Dense IMC Cold",
		"BenchmarkMoE_IMCDeterministic":               "MoE IMC-Det",
		"BenchmarkMoE_Speculative_Baseline":           "MoE Spec-Baseline",
		"BenchmarkMoE_Speculative_WithDraft":          "MoE Spec+Draft",
		"BenchmarkHybrid_IMCDeterministic":            "Hybrid IMC-Det",
	}
	if s, ok := m[name]; ok {
		return s
	}

	return name
}

// =============================================================================

// writeRun formats a single run's results with optional comparison.
func writeRun(out *strings.Builder, current run, prev *run) {
	// Run header.
	if current.header.date != "" {
		fmt.Fprintf(out, "%-15s", current.header.date)
		if current.header.kronk != "" {
			fmt.Fprintf(out, ": %s", current.header.kronk)
		}
		out.WriteString("\n")
	}
	if current.header.llama != "" {
		fmt.Fprintf(out, "%-15s: %s\n", "llama.cpp", current.header.llama)
	}
	out.WriteString(strings.Repeat("-", 80) + "\n")
	if current.header.goos != "" {
		fmt.Fprintf(out, "goos: %s\n", current.header.goos)
	}
	if current.header.goarch != "" {
		fmt.Fprintf(out, "goarch: %s\n", current.header.goarch)
	}
	if current.header.cpu != "" {
		fmt.Fprintf(out, "cpu: %s\n", current.header.cpu)
	}
	out.WriteString("\n")

	// Raw metrics table (ns/op, B/op, allocs/op).
	writeRawTable(out, current, prev)

	out.WriteString(strings.Repeat("-", 80) + "\n")

	// Performance grid (tok/s, ttft-ms, total-ms).
	writePerfGrid(out, current, prev)

	out.WriteString("\n" + separator + "\n\n")
}

// =============================================================================

// writeRawTable writes the ns/op, B/op, allocs/op table.
func writeRawTable(out *strings.Builder, current run, prev *run) {
	groups := displayGroups()

	// Compute the max short name width across all benchmarks present in
	// this run so every group aligns to the same column.
	padWidth := 0
	for _, g := range groups {
		for _, name := range g.names {
			if _, ok := current.benchs[name]; !ok {
				continue
			}
			if w := len(shortName(name)); w > padWidth {
				padWidth = w
			}
		}
	}
	padWidth++ // one space before the colon

	fmtStr := fmt.Sprintf("%%-%ds: %%14.0f ns/op %%s   %%8.0f B/op %%s   %%5.0f allocs/op %%s\n", padWidth)

	for _, g := range groups {
		wrote := false
		for _, name := range g.names {
			b, ok := current.benchs[name]
			if !ok {
				continue
			}

			wrote = true
			sn := shortName(name)

			// ns/op with comparison.
			nsStr := fmtDelta(b.nsOp, lookupField(prev, name, fieldNsOp), false)
			// B/op with comparison.
			bStr := fmtDelta(b.bytesOp, lookupField(prev, name, fieldBytesOp), false)
			// allocs/op with comparison.
			aStr := fmtDelta(b.allocsOp, lookupField(prev, name, fieldAllocsOp), false)

			fmt.Fprintf(out, fmtStr,
				sn, b.nsOp, nsStr, b.bytesOp, bStr, b.allocsOp, aStr)
		}
		if wrote {
			out.WriteString("\n")
		}
	}
}

// =============================================================================

// writePerfGrid writes the tok/s, ttft-ms, total-ms performance grid.
// Benchmarks are displayed in rows of at most 4 columns.
func writePerfGrid(out *strings.Builder, current run, prev *run) {
	const colsPerRow = 4

	groups := displayGroups()

	for _, g := range groups {
		// Collect benchmarks that exist in this run for this group.
		var names []string
		for _, name := range g.names {
			if _, ok := current.benchs[name]; ok {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			continue
		}

		// Write in chunks of colsPerRow.
		for start := 0; start < len(names); start += colsPerRow {
			end := min(start+colsPerRow, len(names))
			chunk := names[start:end]

			// Headers.
			var headers []string
			for _, name := range chunk {
				headers = append(headers, fmt.Sprintf("%-26s", shortName(name)))
			}
			out.WriteString(strings.Join(headers, "") + "\n")

			// tok/s row.
			var tokCols []string
			for _, name := range chunk {
				b := current.benchs[name]
				delta := fmtDelta(b.tokS, lookupField(prev, name, fieldTokS), true)
				tokCols = append(tokCols, fmt.Sprintf("%5.2f tok/s    %s", b.tokS, delta))
			}
			out.WriteString(strings.Join(tokCols, "    ") + "\n")

			// ttft-ms row.
			var ttftCols []string
			for _, name := range chunk {
				b := current.benchs[name]
				delta := fmtDelta(b.ttftMS, lookupField(prev, name, fieldTtftMS), false)
				ttftCols = append(ttftCols, fmt.Sprintf("%5.0f ttft-ms  %s", b.ttftMS, delta))
			}
			out.WriteString(strings.Join(ttftCols, "    ") + "\n")

			// total-ms row.
			var totalCols []string
			for _, name := range chunk {
				b := current.benchs[name]
				delta := fmtDelta(b.totalMS, lookupField(prev, name, fieldTotalMS), false)
				totalCols = append(totalCols, fmt.Sprintf("%5.0f total-ms %s", b.totalMS, delta))
			}
			out.WriteString(strings.Join(totalCols, "    ") + "\n\n")
		}
	}
}

// =============================================================================

type field int

const (
	fieldNsOp field = iota
	fieldBytesOp
	fieldAllocsOp
	fieldTokS
	fieldTtftMS
	fieldTotalMS
)

// lookupField returns the field value from the previous run, or 0 if not found.
func lookupField(prev *run, name string, f field) float64 {
	if prev == nil {
		return 0
	}
	b, ok := prev.benchs[name]
	if !ok {
		return 0
	}
	switch f {
	case fieldNsOp:
		return b.nsOp
	case fieldBytesOp:
		return b.bytesOp
	case fieldAllocsOp:
		return b.allocsOp
	case fieldTokS:
		return b.tokS
	case fieldTtftMS:
		return b.ttftMS
	case fieldTotalMS:
		return b.totalMS
	}
	return 0
}

// fmtDelta formats a percentage change with emoji indicator.
// higherIsBetter=true for tok/s, false for ns/op, B/op, allocs/op, ttft-ms, total-ms.
func fmtDelta(current, previous float64, higherIsBetter bool) string {
	if previous == 0 {
		return "⚪  new"
	}

	pct := ((current - previous) / previous) * 100

	var emoji string
	switch {
	case math.Abs(pct) < neutralThreshold:
		emoji = "⚪"
	case higherIsBetter && pct > 0:
		emoji = "🟢"
	case higherIsBetter && pct < 0:
		emoji = "🔴"
	case !higherIsBetter && pct < 0:
		emoji = "🟢"
	case !higherIsBetter && pct > 0:
		emoji = "🔴"
	}

	return fmt.Sprintf("%s %+.2f%%", emoji, pct)
}

// =============================================================================

// normalizeName strips the -N suffix from benchmark names.
// Go test outputs "BenchmarkFoo-16" but we store as "BenchmarkFoo".
func normalizeName(name string) string {
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		// Check if everything after the dash is a number.
		suffix := name[idx+1:]
		if _, err := strconv.Atoi(suffix); err == nil {
			return name[:idx]
		}
	}
	return name
}

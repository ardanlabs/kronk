package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
)

type comparisonRun struct {
	Directory string   `json:"directory"`
	Version   string   `json:"version"`
	Status    string   `json:"recorded_status"`
	Executed  int      `json:"repetitions_executed"`
	Requested int      `json:"repetitions_requested"`
	Problems  []string `json:"problems"`
}

type metricComparison struct {
	Metric    string       `json:"metric"`
	Baseline  *measurement `json:"baseline"`
	Candidate *measurement `json:"candidate"`
	Delta     *float64     `json:"delta"`
	Percent   *float64     `json:"percent_change"`
}

type comparison struct {
	Status         string             `json:"comparison_status"`
	SavedRunChecks map[string]string  `json:"saved_run_checks"`
	Cell           string             `json:"cell"`
	Baseline       comparisonRun      `json:"baseline"`
	Candidate      comparisonRun      `json:"candidate"`
	Matches        []string           `json:"matching_checks"`
	Warnings       []string           `json:"comparability_warnings"`
	Metrics        []metricComparison `json:"metrics"`
	Limitations    []string           `json:"limitations"`
}

type comparisonInput struct {
	summary  runSummary
	report   report
	requests []any
	native   map[string]string
}

func compareCommand(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("quick-check compare", flag.ContinueOnError)
	flags.SetOutput(out)
	baseline := flags.String("baseline", "", "baseline run directory (supply both paths or neither)")
	candidate := flags.String("candidate", "", "candidate run directory (supply both paths or neither)")
	parent := flags.String("out", ".tools/quick-check/output", "comparison output directory and run parent for automatic selection")
	against := flags.String("against", "commit", "baseline role: commit or release")
	asJSON := flags.Bool("json", false, "write the full comparison as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if (*baseline == "") != (*candidate == "") || flags.NArg() != 0 {
		return fmt.Errorf("usage: quick-check compare [-against commit|release] [-baseline DIR -candidate DIR] [-out DIR] [-json]; supply both paths or neither")
	}
	if *against != "commit" && *against != "release" {
		return fmt.Errorf("-against must be commit or release")
	}
	if *baseline == "" {
		stored, err := savedRuns(*parent)
		if err != nil {
			return err
		}
		var runs []string
		var release string
		for _, run := range stored {
			if strings.HasPrefix(filepath.Base(run.path), "release-") {
				if release != "" {
					return fmt.Errorf("multiple release baselines; supply explicit paths")
				}
				release = run.path
			} else {
				runs = append(runs, run.path)
			}
		}
		if *against == "release" {
			for _, run := range runs {
				if strings.HasPrefix(filepath.Base(run), "new-") {
					if *candidate != "" {
						return fmt.Errorf("ambiguous fresh run; supply explicit paths")
					}
					*candidate = run
				} else if !strings.HasPrefix(filepath.Base(run), "old-") {
					return fmt.Errorf("ambiguous run queue; supply explicit paths")
				}
			}
			if len(runs) > 2 {
				return fmt.Errorf("ambiguous run queue; supply explicit paths")
			}
			if release == "" || *candidate == "" {
				return fmt.Errorf("release comparison needs a fresh run and an older release baseline")
			}
			*baseline = release
		} else {
			if len(runs) != 2 {
				return fmt.Errorf("automatic comparison needs exactly two finalized runs directly under %s; found %d (supply explicit paths instead)", *parent, len(runs))
			}
			// Readable roles take precedence; legacy timestamp folders remain usable.
			switch {
			case strings.HasPrefix(filepath.Base(runs[0]), "new-") && strings.HasPrefix(filepath.Base(runs[1]), "old-"):
			case strings.HasPrefix(filepath.Base(runs[0]), "old-") && strings.HasPrefix(filepath.Base(runs[1]), "new-"):
				runs[0], runs[1] = runs[1], runs[0]
			case !strings.HasPrefix(filepath.Base(runs[0]), "old-") && !strings.HasPrefix(filepath.Base(runs[0]), "new-") && !strings.HasPrefix(filepath.Base(runs[1]), "old-") && !strings.HasPrefix(filepath.Base(runs[1]), "new-"):
				slices.Sort(runs)
				if strings.SplitN(filepath.Base(runs[0]), "-", 2)[0] == strings.SplitN(filepath.Base(runs[1]), "-", 2)[0] {
					return fmt.Errorf("run timestamps are tied; supply explicit baseline and candidate paths")
				}
				runs[0], runs[1] = runs[1], runs[0]
			default:
				return fmt.Errorf("automatic comparison needs an old/new pair; supply explicit baseline and candidate paths")
			}
			*candidate, *baseline = runs[0], runs[1]
		}
	}
	a, err := readComparison(*baseline)
	if err != nil {
		return fmt.Errorf("baseline: %w", err)
	}
	b, err := readComparison(*candidate)
	if err != nil {
		return fmt.Errorf("candidate: %w", err)
	}
	c := compareRuns(*baseline, *candidate, a, b)
	var baselineTag, candidateTag string
	if *against == "release" {
		tags, err := releaseTags()
		if err != nil {
			return err
		}
		baselineTag, candidateTag = tags[a.summary.ServerRevision], tags[b.summary.ServerRevision]
		if baselineTag == "" {
			return fmt.Errorf("baseline commit does not have a remote release tag")
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*parent, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*parent, "comparison.json"), append(data, '\n'), 0600); err != nil {
		return err
	}
	var summary strings.Builder
	if err := c.writeText(&summary); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*parent, "comparison.log"), []byte(summary.String()), 0600); err != nil {
		return err
	}
	if baselineTag != "" && candidateTag != "" && a.summary.ServerRevision != b.summary.ServerRevision {
		if err := recordReleaseComparison(*parent, baselineTag, candidateTag, a, b, c); err != nil {
			return err
		}
	}
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		err = enc.Encode(c)
	} else {
		_, err = io.WriteString(out, summary.String())
	}
	if err != nil {
		return err
	}
	if c.Status != "PASS" {
		return fmt.Errorf("comparison contains failed or incomplete recorded checks")
	}
	return nil
}

// Consume the finalized artifacts without contacting a server or changing retention.
// This compares recorded evidence; it does not rerun or certify inference checks.
func readComparison(dir string) (comparisonInput, error) {
	var input comparisonInput
	data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return input, err
	}
	if err := json.Unmarshal(data, &input.summary); err != nil {
		return input, fmt.Errorf("summary.json: %w", err)
	}
	f, err := os.Open(filepath.Join(dir, "events.ndjson"))
	if err != nil {
		return input, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 65536), 64*1024*1024)
	reports := 0
	startup := make(map[string]map[string]time.Time)
	native := make(map[string]map[string]string)
	for scanner.Scan() {
		var event struct {
			Source     string          `json:"source"`
			Repetition int             `json:"repetition"`
			Phase      string          `json:"phase"`
			Body       any             `json:"body"`
			Report     json.RawMessage `json:"report"`
			Event      map[string]any  `json:"event"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return input, fmt.Errorf("events.ndjson: %w", err)
		}
		if event.Source == "report" {
			reports++
			if err := json.Unmarshal(event.Report, &input.report); err != nil {
				return input, fmt.Errorf("diagnostic report: %w", err)
			}
		}
		if event.Source == "request" {
			input.requests = append(input.requests, map[string]any{
				"repetition": event.Repetition, "phase": event.Phase, "body": event.Body,
			})
		}
		if event.Source == "server" && event.Phase == "warmup" {
			trace, _ := event.Event["trace_id"].(string)
			msg, _ := event.Event["msg"].(string)
			status, _ := event.Event["status"].(string)
			stamp, _ := event.Event["time"].(string)
			if startup[trace] == nil {
				startup[trace] = make(map[string]time.Time)
			}
			if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
				startup[trace][msg+"/"+status] = at
			}
			if msg == "LLAMA-CONTEXT-PARAMS" || msg == "LLAMA-MODEL-PARAMS" {
				if native[trace] == nil {
					native[trace] = make(map[string]string)
				}
				values, _ := event.Event["values"].(string)
				for line := range strings.SplitSeq(strings.TrimSpace(values), "\n") {
					key, value, ok := strings.Cut(line, "[")
					if ok {
						native[trace][msg+"."+key] = strings.TrimSuffix(value, "]")
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return input, err
	}
	if reports != 1 {
		return input, fmt.Errorf("events.ndjson: expected one diagnostic report, got %d", reports)
	}
	s, r := input.summary, input.report
	if (s.Status != "PASS" && s.Status != "FAIL") || s.Workload == "" || s.Model == "" {
		return input, fmt.Errorf("summary.json: missing supported run identity/status")
	}
	if s.Status != r.Status || s.Workload != r.Workload || s.Model != r.Model ||
		s.ServerRevision != r.ServerRevision || s.TestStartedAt != r.TestStartedAt ||
		s.RepetitionsRequested != r.RepetitionsRequested || s.RepetitionsExecuted != len(r.Repetitions) ||
		len(s.Repetitions) != len(r.Repetitions) || !slices.Equal(s.Findings, r.Findings) ||
		!reflect.DeepEqual(s.Measurements, r.measurements()) {
		return input, fmt.Errorf("summary.json and diagnostic report disagree")
	}
	for i, rep := range r.Repetitions {
		summary := s.Repetitions[i]
		if summary.Status != rep.Status || !slices.Equal(summary.Findings, rep.Findings) ||
			summary.SystemTokens != rep.SystemTokens || summary.SystemSHA256 != rep.SystemSHA256 ||
			summary.WorkloadSeconds != rep.WorkloadSeconds || summary.CancelReleaseSeconds != rep.CancelReleaseSeconds {
			return input, fmt.Errorf("repetition %d summary and diagnostic report disagree", i+1)
		}
	}
	if r.WarmupRequest != nil && len(r.Repetitions) == 0 {
		return input, fmt.Errorf("warm-up report has no owning repetition")
	}
	expected := r.summary()
	if !reflect.DeepEqual(s.Machine, expected.Machine) || !reflect.DeepEqual(s.InstalledLibrary, expected.InstalledLibrary) ||
		s.MTPBackend != expected.MTPBackend || s.HarnessRevision != expected.HarnessRevision || s.ServerCommitTime != expected.ServerCommitTime {
		return input, fmt.Errorf("summary.json and diagnostic report provenance disagree")
	}
	data, err = os.ReadFile(filepath.Join(dir, "tool.log"))
	if err != nil {
		return input, err
	}
	header := strings.SplitN(r.logSummary(), "\n", 3)
	// Old saved headers may carry the retired suffix; ignore it without
	// interpreting or rewriting historical dirty-state metadata.
	logText := strings.ReplaceAll(string(data), "+develop", "")
	if !strings.HasPrefix(logText, header[0]+"\n"+header[1]+"\n") {
		return input, fmt.Errorf("tool.log verdict/provenance disagrees with diagnostic report")
	}
	if r.TestStartedAt != "" && !strings.HasPrefix(logText, header[0]+"\n"+header[1]+"\nTest started: "+r.TestStartedAt+"\n") {
		return input, fmt.Errorf("tool.log test timestamp disagrees with diagnostic report")
	}
	if input.report.WarmupRequest != nil {
		warmup := input.report.WarmupRequest
		// Reconstruct older runs' startup metrics from their own recorded trace.
		derived := startupTimings(startup[warmup.TraceID])
		if len(warmup.StartupTimings) > 0 && !reflect.DeepEqual(warmup.StartupTimings, derived) {
			return input, fmt.Errorf("startup metrics disagree with recorded server timestamps")
		}
		warmup.StartupTimings = derived
		input.native = native[warmup.TraceID]
	}
	return input, nil
}

func comparisonIdentity(dir string, input comparisonInput) comparisonRun {
	s := input.summary
	run := comparisonRun{dir, input.report.serverVersion(), s.Status, s.RepetitionsExecuted, s.RepetitionsRequested, []string{}}
	run.Problems = append(run.Problems, s.Findings...)
	if s.Status != "PASS" || s.RepetitionsRequested <= 0 || s.RepetitionsExecuted != s.RepetitionsRequested {
		run.Problems = append(run.Problems, "run failed or requested repetitions were not completed")
	}
	if len(input.requests) != 1+6*s.RepetitionsExecuted {
		run.Problems = append(run.Problems, "request evidence is missing or unexpected")
	}
	if s.WarmupRequest["status"] != "PASS" || input.report.WarmupRequest == nil {
		run.Problems = append(run.Problems, "warm-up evidence did not PASS")
	}
	phases := []string{"cold", "replay", "append", "fresh-session", "cancel", "recovery"}
	for i, value := range input.requests {
		request := value.(map[string]any)
		phase, repetition := "warmup", 1
		if i > 0 {
			phase, repetition = phases[(i-1)%len(phases)], (i-1)/len(phases)+1
		}
		if request["phase"] != phase || request["repetition"] != repetition || request["body"] == nil {
			run.Problems = append(run.Problems, "request evidence has missing bodies or incorrect execution order")
			break
		}
	}
	for i, repetition := range s.Repetitions {
		if repetition.Repetition != i+1 || repetition.Status != "PASS" || len(repetition.Findings) != 0 ||
			len(repetition.Results) != len(phases) || len(input.report.Repetitions[i].Results) != len(phases) {
			run.Problems = append(run.Problems, fmt.Sprintf("repetition %d failed or has incomplete phase evidence", i+1))
			continue
		}
		for j, phase := range phases {
			if repetition.Results[j]["phase"] != phase || repetition.Results[j]["status"] != "PASS" || input.report.Repetitions[i].Results[j].Phase != phase {
				run.Problems = append(run.Problems, fmt.Sprintf("repetition %d %s did not PASS in execution order", i+1, phase))
			}
		}
	}
	results := []result{}
	if input.report.WarmupRequest != nil {
		results = append(results, *input.report.WarmupRequest)
	}
	for _, repetition := range input.report.Repetitions {
		results = append(results, repetition.Results...)
	}
	for _, res := range results {
		if res.Elapsed <= 0 || res.FirstContent <= 0 || res.FirstContent > res.Elapsed {
			run.Problems = append(run.Problems, res.Phase+": invalid or missing client timing")
		}
		for _, metric := range []string{"admission_seconds", "prepare_seconds", "queue_wait_seconds", "slot_execution_seconds", "slot_prefill_ttft_seconds", "generation_seconds"} {
			if value, ok := res.ServerTimings[metric]; !ok || value < 0 {
				run.Problems = append(run.Problems, res.Phase+": missing or invalid server timing "+metric)
			}
		}
	}
	return run
}

func compareRuns(aDir, bDir string, a, b comparisonInput) comparison {
	c := comparison{
		Status: "PASS", Baseline: comparisonIdentity(aDir, a), Candidate: comparisonIdentity(bDir, b),
		Cell:    a.summary.Cell,
		Matches: []string{}, Warnings: []string{}, Metrics: []metricComparison{},
		Limitations: []string{
			"Recorded assertions only; not proof of absence of bugs or independent revalidation of inference.",
			"Deltas are observations, not performance-regression verdicts; no calibrated timing gate.",
			"Repetitions measure within-invocation variability, not between-invocation variability.",
			"Warm-up has one sample per invocation; unloading/restarting does not establish cold OS/GPU caches.",
			"Metadata does not attest loaded binary/library bytes, actual placement, OS release, or idle hardware.",
		},
	}
	if len(c.Baseline.Problems) > 0 || len(c.Candidate.Problems) > 0 {
		c.Status = "FAIL"
	}
	c.SavedRunChecks = map[string]string{"baseline": c.Baseline.Status, "candidate": c.Candidate.Status}
	check := func(name string, x, y any) {
		if x == nil || y == nil || x == "" || y == "" || x == float64(0) || y == float64(0) {
			c.Warnings = append(c.Warnings, name+": missing metadata")
		} else if !reflect.DeepEqual(x, y) {
			xJSON, _ := json.Marshal(x)
			yJSON, _ := json.Marshal(y)
			c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %s -> %s", name, xJSON, yJSON))
		} else {
			c.Matches = append(c.Matches, name)
		}
	}
	for _, side := range []comparisonRun{c.Baseline, c.Candidate} {
		if side.Version == "unknown" {
			c.Warnings = append(c.Warnings, side.Directory+": unknown server commit")
		}
	}
	check("workload", a.summary.Workload, b.summary.Workload)
	check("matrix cell", a.summary.Cell, b.summary.Cell)
	check("model", a.summary.Model, b.summary.Model)
	check("installed library", a.summary.InstalledLibrary, b.summary.InstalledLibrary)
	check("MTP backend", a.summary.MTPBackend, b.summary.MTPBackend)
	check("harness revision", a.summary.HarnessRevision, b.summary.HarnessRevision)
	check("repetition count", a.summary.RepetitionsRequested, b.summary.RepetitionsRequested)
	if len(a.native) == 0 || len(b.native) == 0 {
		c.Warnings = append(c.Warnings, "effective native parameters: missing evidence")
	} else if !reflect.DeepEqual(a.native, b.native) {
		c.Warnings = append(c.Warnings, "effective native parameters: changed (including possible log-schema changes)")
	} else {
		c.Matches = append(c.Matches, "effective native parameters")
	}
	for _, key := range []string{"host", "os", "system_ram_bytes", "gpu_total_bytes", "unified_memory"} {
		check("machine."+key, a.summary.Machine[key], b.summary.Machine[key])
	}
	// Compare only stable model identity/configuration, not creation times or VRAM estimates.
	aModel, _ := a.report.Metadata["model"].(map[string]any)
	bModel, _ := b.report.Metadata["model"].(map[string]any)
	for _, key := range []string{"size", "quantization", "file_type", "template", "metadata"} {
		if !reflect.DeepEqual(aModel[key], bModel[key]) || aModel[key] == nil {
			c.Warnings = append(c.Warnings, "model."+key+": changed or missing")
		} else {
			c.Matches = append(c.Matches, "model."+key)
		}
	}
	aConfig, _ := aModel["model_config"].(map[string]any)
	bConfig, _ := bModel["model_config"].(map[string]any)
	keys := make(map[string]bool)
	for key := range aConfig {
		keys[key] = true
	}
	for key := range bConfig {
		keys[key] = true
	}
	if len(keys) == 0 {
		c.Warnings = append(c.Warnings, "model configuration: missing metadata")
	}
	for _, key := range sortedKeys(keys) {
		x, aOK := aConfig[key]
		y, bOK := bConfig[key]
		if aOK != bOK || !reflect.DeepEqual(x, y) {
			c.Warnings = append(c.Warnings, "model configuration."+key+": changed or unavailable in one run")
		}
	}
	if !reflect.DeepEqual(a.requests, b.requests) {
		c.Warnings = append(c.Warnings, "request bodies/order differ (includes generated assistant history); input/settings are not identical")
	} else {
		c.Matches = append(c.Matches, "request bodies/order")
	}
	for i := range min(len(a.summary.Repetitions), len(b.summary.Repetitions)) {
		x, y := a.summary.Repetitions[i], b.summary.Repetitions[i]
		if x.SystemTokens <= 0 || y.SystemTokens <= 0 || x.SystemSHA256 == "" || y.SystemSHA256 == "" || x.SystemTokens != y.SystemTokens || x.SystemSHA256 != y.SystemSHA256 {
			c.Warnings = append(c.Warnings, fmt.Sprintf("repetition %d: system token count/hash changed or missing", i+1))
		} else {
			c.Matches = append(c.Matches, fmt.Sprintf("repetition %d system tokens/hash", i+1))
		}
	}
	observations := func(r report) [][]int {
		var counts [][]int
		results := []result{}
		if r.WarmupRequest != nil {
			results = append(results, *r.WarmupRequest)
		}
		for _, rep := range r.Repetitions {
			results = append(results, rep.Results...)
		}
		for _, res := range results {
			counts = append(counts, []int{res.Usage.PromptTokens, res.Usage.CompletionTokens,
				res.Execution.ReusedTokens, res.Execution.OutputTokens, res.Execution.DraftTokens, res.Execution.DraftAccepted})
		}
		return counts
	}
	if reflect.DeepEqual(observations(a.report), observations(b.report)) {
		c.Matches = append(c.Matches, "per-request prompt/output/cache/draft counts")
	} else {
		c.Warnings = append(c.Warnings, "per-request prompt/output/cache/draft counts differ; elapsed times may include different work")
	}
	// Failed/partial runs remain visible, but never receive passing timing aggregates.
	if c.Status == "PASS" {
		x, y := comparisonMetrics(a.report), comparisonMetrics(b.report)
		metricKeys := make(map[string]bool)
		for key := range x {
			metricKeys[key] = true
		}
		for key := range y {
			metricKeys[key] = true
		}
		for _, key := range sortedKeys(metricKeys) {
			row := metricComparison{Metric: key}
			if values := x[key]; len(values) > 0 {
				row.Baseline = new(summarize(values))
			}
			if values := y[key]; len(values) > 0 {
				row.Candidate = new(summarize(values))
			}
			if row.Baseline != nil && row.Candidate != nil {
				row.Delta = new(row.Candidate.Median - row.Baseline.Median)
				if row.Baseline.Median != 0 {
					row.Percent = new(*row.Delta / row.Baseline.Median * 100)
				}
			} else {
				c.Warnings = append(c.Warnings, key+": unavailable in one run; no delta")
			}
			c.Metrics = append(c.Metrics, row)
		}
	}
	return c
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func comparisonMetrics(r report) map[string][]float64 {
	metrics := make(map[string][]float64)
	add := func(key string, value float64) { metrics[key] = append(metrics[key], value) }
	request := func(prefix string, res result) {
		add(prefix+".elapsed_seconds", res.Elapsed)
		add(prefix+".first_content_seconds", res.FirstContent)
		for key, value := range res.ServerTimings {
			add(prefix+".server."+key, value)
		}
		for key, value := range res.StartupTimings {
			add(prefix+".startup."+key, value)
		}
		if !res.Cancelled && res.Usage.CompletionTokens > 0 {
			add(prefix+".server_output_tokens_per_second", res.Usage.ServerTPS)
			add(prefix+".prompt_tokens", float64(res.Usage.PromptTokens))
			add(prefix+".completion_tokens", float64(res.Usage.CompletionTokens))
			add(prefix+".cached_tokens", float64(res.Execution.ReusedTokens))
			add(prefix+".draft_tokens", float64(res.Execution.DraftTokens))
			add(prefix+".draft_accepted_tokens", float64(res.Execution.DraftAccepted))
		}
	}
	if r.WarmupRequest != nil {
		request("warmup", *r.WarmupRequest)
	}
	for _, rep := range r.Repetitions {
		add("workload.elapsed_seconds", rep.WorkloadSeconds)
		add("workload.cancel_release_seconds", rep.CancelReleaseSeconds)
		for _, res := range rep.Results {
			request(res.Phase, res)
		}
	}
	return metrics
}

func (c comparison) writeText(out io.Writer) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "Comparison status: %s\n", c.Status)
	fmt.Fprintf(w, "Saved-run checks: baseline %s / candidate %s\nBaseline: %s\nBaseline commit: %s (%d/%d runs)\nCandidate: %s\nCandidate commit: %s (%d/%d runs)\n", c.Baseline.Status, c.Candidate.Status,
		c.Baseline.Directory, c.Baseline.Version, c.Baseline.Executed, c.Baseline.Requested,
		c.Candidate.Directory, c.Candidate.Version, c.Candidate.Executed, c.Candidate.Requested)
	for _, run := range []comparisonRun{c.Baseline, c.Candidate} {
		for _, problem := range run.Problems {
			fmt.Fprintf(w, "Problem [%s]: %s\n", run.Directory, problem)
		}
	}
	machineWarning := ""
	for _, warning := range c.Warnings {
		if strings.HasPrefix(warning, "machine.") {
			machineWarning = "WARNING: MACHINE METADATA DIFFERS OR IS MISSING!"
		}
		if strings.HasPrefix(warning, "harness revision:") {
			continue
		}
		if strings.HasPrefix(warning, "model configuration") && slices.Contains(c.Matches, "effective native parameters") && slices.Contains(c.Matches, "request bodies/order") {
			continue
		}
		fmt.Fprintln(w, "Warning:", warning)
	}
	cell := strings.NewReplacer("one slot", "1 slot", "images off", "no images", "images on", "images").Replace(c.Cell)
	if machineWarning != "" {
		fmt.Fprintln(w, "\n"+machineWarning)
	}
	fmt.Fprintln(w, "\nMetric (seconds)\tBaseline median [SD; n]\tCandidate median [SD; n]\tDelta\tChange")
	fmt.Fprintf(w, "Cell: %s\n", cell)
	format := func(m *measurement) string {
		if m == nil {
			return "n/a"
		}
		sd := "n/a"
		if m.SampleStdDev != nil {
			sd = fmt.Sprintf("%.6f", *m.SampleStdDev)
		}
		return fmt.Sprintf("%.6f [%s; %d]", m.Median, sd, m.Samples)
	}
	for _, row := range c.Metrics {
		if row.Metric != "workload.elapsed_seconds" {
			continue
		}
		delta, percent := "n/a", "n/a"
		if row.Delta != nil {
			delta = fmt.Sprintf("%+.6f", *row.Delta)
		}
		if row.Percent != nil {
			percent = fmt.Sprintf("%+.2f%%", *row.Percent)
		}
		fmt.Fprintf(w, "Warmed workload\t%s\t%s\t%s\t%s\n", format(row.Baseline), format(row.Candidate), delta, percent)
	}
	if machineWarning != "" {
		fmt.Fprintln(w, "\n"+machineWarning)
	}
	return w.Flush()
}

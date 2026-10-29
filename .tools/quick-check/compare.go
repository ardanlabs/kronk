package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"
)

type comparisonRun struct {
	Directory       string                 `json:"directory"`
	Version         string                 `json:"version"`
	Release         string                 `json:"release,omitempty"`
	Status          string                 `json:"recorded_status"`
	Executed        int                    `json:"repetitions_executed,omitempty"`
	Requested       int                    `json:"repetitions_requested,omitempty"`
	Problems        []string               `json:"problems"`
	ReferenceTokens normalizationReference `json:"normalization_reference,omitzero"`
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
	Baseline       comparisonRun      `json:"baseline,omitzero"`
	Candidate      comparisonRun      `json:"candidate,omitzero"`
	Matches        []string           `json:"matching_checks"`
	Warnings       []string           `json:"comparability_warnings"`
	Metrics        []metricComparison `json:"metrics"`
	Limitations    []string           `json:"limitations"`
}

type comparisonInput struct {
	summary               runSummary
	report                report
	requests              []any
	native                map[string]string
	sharedWarmupValidated bool
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
	historyParent := *parent
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
			switch {
			case strings.HasPrefix(filepath.Base(runs[0]), "new-") && strings.HasPrefix(filepath.Base(runs[1]), "old-"):
			case strings.HasPrefix(filepath.Base(runs[0]), "old-") && strings.HasPrefix(filepath.Base(runs[1]), "new-"):
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
	c := compareMatrices(*baseline, *candidate, a, b)
	var baselineTag, candidateTag string
	if *against == "release" {
		baselineTag, err = promotedRelease(*baseline, a.summary.ServerRevision)
		if err != nil {
			return err
		}
		candidateTag, err = promotedRelease(*candidate, b.summary.ServerRevision)
		if err != nil {
			return err
		}
		// Manually promoted baselines need no tag lookup. An ordinary nightly
		// candidate is not a release until it is explicitly promoted too.
		if baselineTag == "" {
			tags, err := releaseTags(context.Background())
			if err != nil {
				return err
			}
			baselineTag = tags[a.summary.ServerRevision]
			if candidateTag == "" {
				candidateTag = tags[b.summary.ServerRevision]
			}
		}
		if baselineTag == "" {
			return fmt.Errorf("baseline has neither a promotion nor a remote release tag")
		}
		c.Baseline.Release, c.Candidate.Release = baselineTag, candidateTag
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
		if err := recordReleaseComparison(historyParent, baselineTag, candidateTag, a, b, c); err != nil {
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
	if c.Status == "NOT_RUN" {
		return fmt.Errorf("comparison has no common executed cells")
	}
	if c.Status != "PASS" {
		return fmt.Errorf("comparison contains failed or incomplete recorded checks")
	}
	return nil
}

// Consume the finalized artifacts without contacting a server or changing retention.
// This compares recorded evidence; it does not rerun or certify inference checks.
func readCellComparison(s runSummary, r report, events []byte) (comparisonInput, error) {
	input := comparisonInput{summary: s, report: r}
	scanner := bufio.NewScanner(bytes.NewReader(events))
	scanner.Buffer(make([]byte, 65536), 64*1024*1024)
	startup := make(map[string]map[string]time.Time)
	native := make(map[string]map[string]string)
	for scanner.Scan() {
		var event struct {
			Source     string         `json:"source"`
			Repetition int            `json:"repetition"`
			Phase      string         `json:"phase"`
			Body       any            `json:"body"`
			Event      map[string]any `json:"event"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return input, fmt.Errorf("events.ndjson: %w", err)
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
	if (s.Status != "PASS" && s.Status != "FAIL") || s.Workload == "" || s.Model == "" {
		return input, fmt.Errorf("summary.json: missing supported run identity/status")
	}
	if s.ReferenceTokens.InputTokens <= 0 || s.ReferenceTokens.GenerationTokens <= 0 {
		return input, fmt.Errorf("summary.json: missing or invalid normalization reference; rerun quick-check")
	}
	if s.ID != r.ID || s.Status != r.Status || s.Workload != r.Workload || s.Model != r.Model ||
		s.Cell != r.cell() || s.ReferenceTokens != r.ReferenceTokens || s.SharedWarmupFrom != r.SharedWarmupFrom ||
		s.ServerRevision != r.ServerRevision || s.TestStartedAt != r.TestStartedAt ||
		s.RepetitionsRequested != r.RepetitionsRequested || s.RepetitionsExecuted != len(r.Repetitions) ||
		len(s.Repetitions) != len(r.Repetitions) || !slices.Equal(s.Findings, r.Findings) ||
		!reflect.DeepEqual(s.Measurements, r.measurements()) {
		return input, fmt.Errorf("summary.json and diagnostic report disagree")
	}
	for i, rep := range r.Repetitions {
		summary := s.Repetitions[i]
		if rep.ReferenceTokens != r.ReferenceTokens || rep.Images != r.Images || rep.TargetOnly != r.TargetOnly || rep.NoCache != r.NoCache || rep.TwoSlots != r.TwoSlots || rep.SharedWarmupFrom != r.SharedWarmupFrom || summary.Status != rep.Status || !slices.Equal(summary.Findings, rep.Findings) ||
			summary.SystemTokens != rep.SystemTokens || summary.SystemSHA256 != rep.SystemSHA256 ||
			summary.WorkloadSeconds != rep.WorkloadSeconds || summary.CancelReleaseSeconds != rep.CancelReleaseSeconds {
			return input, fmt.Errorf("repetition %d summary and diagnostic report disagree", i+1)
		}
	}
	if r.WarmupRequest != nil && len(r.Repetitions) == 0 {
		return input, fmt.Errorf("warm-up report has no owning repetition")
	}
	if r.SharedWarmupFrom != "" && (r.WarmupRequest != nil || len(s.WarmupRequest) > 0) {
		return input, fmt.Errorf("shared warm-up must not duplicate startup measurements")
	}
	expected := r.summary()
	if !reflect.DeepEqual(s.Machine, expected.Machine) || !reflect.DeepEqual(s.InstalledLibrary, expected.InstalledLibrary) ||
		s.MTPBackend != expected.MTPBackend || s.HarnessRevision != expected.HarnessRevision || s.ServerCommitTime != expected.ServerCommitTime || s.ReleaseTag != expected.ReleaseTag {
		return input, fmt.Errorf("summary.json and diagnostic report provenance disagree")
	}
	if _, err := time.Parse(time.RFC3339Nano, r.TestStartedAt); err != nil {
		return input, fmt.Errorf("missing or invalid test timestamp: %w", err)
	}
	if input.report.WarmupRequest != nil {
		warmup := input.report.WarmupRequest
		if !maps.Equal(warmup.StartupTimings, startupTimings(startup[warmup.TraceID])) {
			return input, fmt.Errorf("startup metrics disagree with recorded server timestamps")
		}
		input.native = native[warmup.TraceID]
	}
	return input, nil
}

func comparisonIdentity(dir string, input comparisonInput) comparisonRun {
	s := input.summary
	run := comparisonRun{Directory: dir, Version: input.report.serverVersion(), Status: s.Status,
		Executed: s.RepetitionsExecuted, Requested: s.RepetitionsRequested, Problems: []string{}, ReferenceTokens: s.ReferenceTokens}
	run.Problems = append(run.Problems, s.Findings...)
	if s.Status != "PASS" || s.RepetitionsRequested <= 0 || s.RepetitionsExecuted != s.RepetitionsRequested {
		run.Problems = append(run.Problems, "run failed or requested repetitions were not completed")
	}
	phases := input.report.phases()
	warmups := 1
	if input.report.SharedWarmupFrom != "" {
		warmups = 0
	}
	if len(input.requests) != warmups+len(phases)*s.RepetitionsExecuted {
		run.Problems = append(run.Problems, "request evidence is missing or unexpected")
	}
	if warmups == 0 && !input.sharedWarmupValidated {
		run.Problems = append(run.Problems, "shared warm-up source not validated")
	} else if warmups == 1 && (s.WarmupRequest["status"] != "PASS" || input.report.WarmupRequest == nil) {
		run.Problems = append(run.Problems, "warm-up evidence did not PASS")
	}
	seen := make(map[string]bool)
	for i, value := range input.requests {
		request := value.(map[string]any)
		if input.report.TwoSlots {
			phase, _ := request["phase"].(string)
			repetition, _ := request["repetition"].(int)
			key := fmt.Sprintf("%d/%s", repetition, phase)
			valid := repetition >= 1 && repetition <= s.RepetitionsExecuted && slices.Contains(phases, phase)
			if phase == "warmup" {
				valid = warmups == 1 && repetition == 1
			}
			if !valid || seen[key] || request["body"] == nil {
				run.Problems = append(run.Problems, "request evidence has missing bodies, duplicate or unexpected phases/repetitions")
				break
			}
			seen[key] = true
			continue
		}
		phase, repetition := "warmup", 1
		if i >= warmups {
			phase, repetition = phases[(i-warmups)%len(phases)], (i-warmups)/len(phases)+1
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

// Two-slot requests can arrive in goroutine order, unlike their canonical results.
// Sort a copy so validation and the saved evidence retain the actual arrival order.
func comparisonRequests(input comparisonInput) []any {
	if !input.report.TwoSlots {
		return input.requests
	}
	phases := append([]string{"warmup"}, input.report.phases()...)
	requests := slices.Clone(input.requests)
	slices.SortStableFunc(requests, func(a, b any) int {
		x, y := a.(map[string]any), b.(map[string]any)
		if delta := x["repetition"].(int) - y["repetition"].(int); delta != 0 {
			return delta
		}
		return slices.Index(phases, x["phase"].(string)) - slices.Index(phases, y["phase"].(string))
	})
	return requests
}

func compareRuns(aDir, bDir string, a, b comparisonInput) comparison {
	c := comparison{
		Status: "PASS", Baseline: comparisonIdentity(aDir, a), Candidate: comparisonIdentity(bDir, b),
		Cell:    a.summary.ID,
		Matches: []string{}, Warnings: []string{}, Metrics: []metricComparison{},
		Limitations: []string{
			"Recorded assertions only; not proof of absence of bugs or independent revalidation of inference.",
			"Deltas are observations, not performance-regression verdicts; no calibrated timing gate.",
			"Repetitions measure within-invocation variability, not between-invocation variability.",
			"Warm-up has one sample per model group; unloading/restarting does not establish cold OS/GPU caches.",
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
	check("normalization reference", a.summary.ReferenceTokens, b.summary.ReferenceTokens)
	if a.summary.ReferenceTokens != b.summary.ReferenceTokens {
		c.Warnings = append(c.Warnings, "Normalized delta omitted: reference token amounts differ")
	}
	check("model", a.summary.Model, b.summary.Model)
	check("installed library", a.summary.InstalledLibrary, b.summary.InstalledLibrary)
	if !a.report.TargetOnly || !b.report.TargetOnly {
		check("MTP backend", a.summary.MTPBackend, b.summary.MTPBackend)
	}
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
	if !reflect.DeepEqual(comparisonRequests(a), comparisonRequests(b)) {
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
	// Validate each side independently: a failed/partial run must not hide the
	// other run's valid measurements, but it cannot contribute values or deltas.
	var x, y map[string][]float64
	if len(c.Baseline.Problems) == 0 {
		x = comparisonMetrics(a.report)
	}
	if len(c.Candidate.Problems) == 0 {
		y = comparisonMetrics(b.report)
	}
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
		if row.Baseline == nil || row.Candidate == nil {
			if c.Status == "PASS" {
				c.Warnings = append(c.Warnings, key+": unavailable in one run; no delta")
			}
		} else if key != "workload.reference_seconds" || a.summary.ReferenceTokens == b.summary.ReferenceTokens {
			row.Delta = new(row.Candidate.Median - row.Baseline.Median)
			if row.Baseline.Median != 0 {
				row.Percent = new(*row.Delta / row.Baseline.Median * 100)
			}
		}
		c.Metrics = append(c.Metrics, row)
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
		if seconds, ok := rep.referenceTime(); ok {
			add("workload.reference_seconds", seconds)
		}
	}
	if len(metrics["workload.reference_seconds"]) != len(r.Repetitions) {
		delete(metrics, "workload.reference_seconds")
	}
	return metrics
}

func (c comparison) textWarnings() []string {
	var warnings []string
	for _, warning := range c.Warnings {
		if strings.HasPrefix(warning, "harness revision:") {
			continue
		}
		if strings.HasPrefix(warning, "model configuration") && slices.Contains(c.Matches, "effective native parameters") && slices.Contains(c.Matches, "request bodies/order") {
			continue
		}
		warnings = append(warnings, warning)
	}
	return warnings
}

func (c comparison) writeRows(w io.Writer) {
	// Empty cells preserve tabwriter's column widths across visual blank lines.
	fmt.Fprintln(w, "\t\t\t\t")
	cell := c.Cell
	format := func(m *measurement) string {
		if m == nil {
			return "n/a"
		}
		sd := "n/a"
		if m.SampleStdDev != nil {
			sd = fmt.Sprintf("%.2f", *m.SampleStdDev)
		}
		return fmt.Sprintf("%.2f [%s]", m.Median, sd)
	}
	for i, key := range []string{"workload.elapsed_seconds", "workload.reference_seconds"} {
		baseline, candidate, percent := "n/a", "n/a", "n/a"
		for _, row := range c.Metrics {
			if row.Metric != key {
				continue
			}
			baseline, candidate = format(row.Baseline), format(row.Candidate)
			if row.Percent != nil {
				percent = fmt.Sprintf("%+.2f%%", *row.Percent)
			}
			break
		}
		label := "Workload (s)"
		if i == 1 {
			cell, label = "", "Normalized (s)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", cell, label, baseline, candidate, percent)
	}
}

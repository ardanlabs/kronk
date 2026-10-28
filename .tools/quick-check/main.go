// Command quick-check probes a matrix of Kronk features and configurations
// through a separately managed Kronk model server. It is not a model benchmark.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const modelID = "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK"
const targetOnlyModelID = "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK-NO-MTP"
const noCacheModelID = "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK-NO-IMC"
const noCacheTargetOnlyModelID = "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK-NO-IMC-NO-MTP"
const denseModelID = "ornith-ai/Ornith-1.5-9B-Q4_K_M"

type options struct {
	host        string
	out         string
	serverLog   string
	timeout     time.Duration
	repetitions int
	images      bool
	cells       []string
}

type report struct {
	ID                   string                 `json:"cell_id"`
	Status               string                 `json:"status"`
	Workload             string                 `json:"workload"`
	Model                string                 `json:"model"`
	Images               bool                   `json:"images"`
	TargetOnly           bool                   `json:"target_only"`
	NoCache              bool                   `json:"no_cache"`
	TwoSlots             bool                   `json:"two_slots"`
	ConfiguredSlots      []int                  `json:"configured_slots"`
	ReferenceTokens      normalizationReference `json:"normalization_reference"`
	Host                 string                 `json:"host"`
	HarnessRevision      string                 `json:"harness_revision"`
	ServerRevision       string                 `json:"server_revision"`
	ServerCommitTime     string                 `json:"server_commit_time"`
	ReleaseTag           string                 `json:"release_tag,omitempty"`
	TestStartedAt        string                 `json:"test_started_at,omitempty"`
	Metadata             map[string]any         `json:"server_metadata"`
	SystemTokens         int                    `json:"system_tokens"`
	SystemSHA256         string                 `json:"system_sha256"`
	Slot                 int                    `json:"slot"`
	MTPBackend           string                 `json:"mtp_backend"`
	WarmupRequest        *result                `json:"warmup_request,omitempty"`
	SharedWarmupFrom     string                 `json:"shared_warmup_from,omitempty"`
	WorkloadSeconds      float64                `json:"workload_seconds"`
	CancelReleaseSeconds float64                `json:"cancel_release_seconds"`
	Results              []result               `json:"results"`
	Findings             []string               `json:"findings"`
	RepetitionsRequested int                    `json:"repetitions_requested,omitempty"`
	Repetitions          []report               `json:"repetitions,omitempty"`
	evidenceValidated    bool
}

// Field order is the human reading order in summary.json: environment, run,
// then repetitions and their requests.
type runSummary struct {
	ID                   string                            `json:"cell_id"`
	Machine              map[string]any                    `json:"machine"`
	ServerVersion        string                            `json:"server_version"`
	ServerRevision       string                            `json:"server_revision"`
	ServerCommitTime     string                            `json:"server_commit_time"`
	ReleaseTag           string                            `json:"release_tag,omitempty"`
	TestStartedAt        string                            `json:"test_started_at,omitempty"`
	ServerBuild          any                               `json:"server_build"`
	InstalledLibrary     any                               `json:"installed_library"`
	MTPBackend           string                            `json:"mtp_backend"`
	HarnessRevision      string                            `json:"harness_revision"`
	Model                string                            `json:"model"`
	Cell                 string                            `json:"cell"`
	Workload             string                            `json:"workload"`
	ReferenceTokens      normalizationReference            `json:"normalization_reference"`
	Status               string                            `json:"status"`
	Findings             []string                          `json:"findings"`
	RepetitionsRequested int                               `json:"repetitions_requested"`
	RepetitionsExecuted  int                               `json:"repetitions_executed"`
	WarmupRequest        map[string]any                    `json:"warmup_request,omitempty"`
	SharedWarmupFrom     string                            `json:"shared_warmup_from,omitempty"`
	Measurements         map[string]map[string]measurement `json:"measurements,omitempty"`
	Repetitions          []repetitionSummary               `json:"repetitions"`
}

func (s runSummary) MarshalJSON() ([]byte, error) {
	if s.Status == "NOT_RUN" {
		return json.Marshal(struct {
			ID     string `json:"cell_id"`
			Cell   string `json:"cell"`
			Status string `json:"status"`
		}{s.ID, s.Cell, s.Status})
	}
	type summary runSummary
	return json.Marshal(summary(s))
}

// Keep the decision/comparison surface separate from the diagnostic record.
func (r report) summary() runSummary {
	if r.Status == "NOT_RUN" {
		return runSummary{ID: r.ID, Cell: r.cell(), Status: r.Status}
	}
	liveness, _ := r.Metadata["/v1/liveness"].(map[string]any)
	libraries, _ := r.Metadata["/v1/kronk/libs"].(map[string]any)
	hardware, _ := r.Metadata["/v1/devices"].(map[string]any)
	summary := runSummary{
		ID: r.ID,
		Machine: map[string]any{
			"host":             liveness["host"],
			"os":               liveness["os"],
			"system_ram_bytes": hardware["system_ram_bytes"],
			"gpu_total_bytes":  hardware["gpu_total_bytes"],
			"unified_memory":   hardware["unified_memory"],
		},
		ServerVersion:        r.serverVersion(),
		ServerRevision:       r.ServerRevision,
		ServerCommitTime:     r.ServerCommitTime,
		ReleaseTag:           r.ReleaseTag,
		TestStartedAt:        r.TestStartedAt,
		ServerBuild:          liveness["build"],
		InstalledLibrary:     libraries["current"],
		MTPBackend:           r.MTPBackend,
		HarnessRevision:      r.HarnessRevision,
		Model:                r.Model,
		Cell:                 r.cell(),
		Workload:             r.Workload,
		ReferenceTokens:      r.ReferenceTokens,
		Status:               r.Status,
		Findings:             append([]string{}, r.Findings...),
		RepetitionsRequested: r.RepetitionsRequested,
		RepetitionsExecuted:  len(r.Repetitions),
		SharedWarmupFrom:     r.SharedWarmupFrom,
		Measurements:         r.measurements(),
		Repetitions:          []repetitionSummary{},
	}

	if r.WarmupRequest != nil {
		summary.WarmupRequest = r.Repetitions[0].phaseSummary(*r.WarmupRequest)
		summary.WarmupRequest["initial_state"] = "model-unloaded"
	}
	for i, repetition := range r.Repetitions {
		summary.Repetitions = append(summary.Repetitions, repetition.repetitionSummary(i+1))
	}
	return summary
}

func (r report) cell() string {
	images := "off"
	if r.Images {
		images = "on"
	}
	mtp := "on"
	if r.TargetOnly {
		mtp = "off"
	}
	imc := "on"
	if r.NoCache {
		imc = "off"
	}
	slots := "one slot"
	if r.TwoSlots {
		slots = "two slots"
	}
	routing := "MoE"
	if r.isDense() {
		routing = "Dense"
	}
	return routing + " / " + slots + " / IMC " + imc + " / MTP " + mtp + " / images " + images
}

func (r report) phaseSummary(res result) map[string]any {
	findings := []string{}
	for _, finding := range r.Findings {
		if text, ok := strings.CutPrefix(finding, res.Phase+": "); ok {
			findings = append(findings, text)
		}
	}
	if res.Error != "" && len(findings) == 0 {
		findings = append(findings, res.Error)
	}
	status := "UNVERIFIED"
	if r.evidenceValidated {
		status = "PASS"
	}
	if len(findings) > 0 {
		status = "FAIL"
	}

	phase := map[string]any{
		"phase":                 res.Phase,
		"status":                status,
		"findings":              findings,
		"elapsed_seconds":       res.Elapsed,
		"first_content_seconds": res.FirstContent,
		"server_timings":        res.ServerTimings,
		"cache_match":           res.CacheMatch,
		"cache_reason":          res.CacheReason,
		"cached_tokens":         res.Execution.ReusedTokens,
		"draft_tokens":          res.Execution.DraftTokens,
		"draft_accepted_tokens": res.Execution.DraftAccepted,
	}

	if res.StartupTimings != nil {
		phase["startup_timings"] = res.StartupTimings
	}
	if r.Images {
		phase["media_count"] = res.MediaCount
		phase["encoded_image_tokens"] = res.EncodedTokens
		phase["decoded_image_tokens"] = res.DecodedTokens
	}

	// A deliberately interrupted stream has no final usage or meaningful
	// generation rate. Do not present missing measurements as zero values.
	if !res.Cancelled && res.Usage.CompletionTokens > 0 {
		phase["prompt_tokens"] = res.Usage.PromptTokens
		phase["completion_tokens"] = res.Usage.CompletionTokens
		phase["generation_seconds"] = res.Generation
		phase["client_output_tokens_per_second"] = res.OutputTPS
		phase["server_output_tokens_per_second"] = res.Usage.ServerTPS
	}
	return phase
}

// Probe verdicts summarize their complete phase checks, not a diagnosed cause.
func (r report) logSummary() string {
	verdict := func(names ...string) string {
		status := "PASS"
		if len(r.Repetitions) < r.RepetitionsRequested {
			status = "UNVERIFIED"
		}
		for i, repetition := range r.Repetitions {
			phases := make(map[string]result)
			if repetition.WarmupRequest != nil {
				phases["warmup"] = *repetition.WarmupRequest
			}
			for _, res := range repetition.Results {
				phases[res.Phase] = res
			}
			for _, name := range names {
				if name == "warmup" && (i > 0 || r.SharedWarmupFrom != "") {
					continue // Startup belongs to the group's first selected cell.
				}
				res, ok := phases[name]
				if !ok {
					status = "UNVERIFIED"
					continue
				}
				switch repetition.phaseSummary(res)["status"] {
				case "FAIL":
					return "FAIL"
				case "UNVERIFIED":
					status = "UNVERIFIED"
				}
			}
		}
		return status
	}

	warmup, workload, reference := "n/a", "n/a", "n/a"
	if r.WarmupRequest != nil {
		warmup = fmt.Sprintf("%.2fs", r.WarmupRequest.Elapsed)
	} else if r.SharedWarmupFrom != "" {
		warmup = "shared"
	}

	if measurements := r.measurements(); measurements != nil {
		format := func(m measurement) string {
			sd := "n/a"
			if m.SampleStdDev != nil {
				sd = fmt.Sprintf("%.2f", *m.SampleStdDev)
			}
			return fmt.Sprintf("%.2f [%s]", m.Median, sd)
		}
		workload = format(measurements["workload"]["elapsed_seconds"])
		if m, ok := measurements["workload"]["reference_seconds"]; ok {
			reference = format(m)
		}
	}
	cache := verdict(append([]string{"warmup"}, r.phases()...)...)
	isolation := verdict("fresh-session", "recovery")
	recovery := verdict("cancel", "recovery")
	isolationLabel := "Serial isolation"
	if r.TwoSlots {
		isolation = verdict("append", "peer-append", "survivor", "cancel", "recovery")
		isolationLabel = "Concurrent isolation"
	}
	var b strings.Builder
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Cell: %s | Warm-up: %s | Repetitions: %d/%d\n", r.ID, warmup, len(r.Repetitions), r.RepetitionsRequested)
	fmt.Fprintf(&b, "  Workload s [SD]: %s | Normalized s [SD]: %s\n", workload, reference)
	fmt.Fprintf(&b, "IMC/MTP: %s | %s: %s | Cancel/recovery: %s", cache, isolationLabel, isolation, recovery)
	if r.Images {
		fmt.Fprintf(&b, " | Images: %s\n", verdict(r.phases()...))
	} else {
		fmt.Fprintln(&b)
	}
	if r.Status != "PASS" {
		fmt.Fprintln(&b, "Details: summary.json (fix setup/evidence failures before deeper inference checks)")
		if r.Images {
			fmt.Fprintln(&b, "Images -> make test-load-media-correctness")
		}
		if cache == "FAIL" {
			fmt.Fprintln(&b, "IMC/MTP -> make test-load-hybrid; make test-load-mtp-embedded")
		}
		if isolation == "FAIL" || recovery == "FAIL" {
			fmt.Fprintln(&b, "Isolation/recovery -> make test-load-hybrid")
		}
	}
	return b.String()
}

type engine struct {
	ModelID string `json:"model_id"`
	MTP     bool   `json:"mtp"`
	NDraft  int    `json:"ndraft"`
	Slots   []struct {
		ID    int    `json:"id"`
		Phase string `json:"phase"`
	} `json:"slots"`
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "compare" {
		if err := compareCommand(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var o options
	flag.StringVar(&o.host, "host", "http://localhost:11435", "running Kronk server URL")
	flag.StringVar(&o.out, "out", ".tools/quick-check/output", "artifact parent; retains committed baseline and latest candidate")
	flag.StringVar(&o.serverLog, "server-log", filepath.Join(home, ".kronk/kronk.log"), "required local JSON server log")
	flag.DurationVar(&o.timeout, "timeout", 5*time.Minute, "per-request timeout; not a performance threshold")
	flag.IntVar(&o.repetitions, "repetitions", 2, "workload repetitions per cell; stop a failing cell, then continue remaining cells")
	flag.Func("cell", "implemented cell ID, e.g. MoE/1s/IMC+/MTP+/img- (repeat for each wanted cell; omit to run all)", func(id string) error {
		o.cells = append(o.cells, id)
		return nil
	})
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments; use -cell ID for each wanted cell")
		os.Exit(1)
	}
	if err := run(o); err != nil {
		os.Exit(1)
	}
}

func run(o options) (runErr error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	reported := false
	defer func() {
		// Setup/storage failures still need a visible explanation when no
		// finalized summary could fully report the failure.
		if runErr != nil && !reported {
			fmt.Fprintln(os.Stderr, runErr)
		}
	}()
	if o.timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if o.repetitions <= 0 {
		return fmt.Errorf("repetitions must be positive")
	}
	selected, err := selectedCells(o.cells)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(o.out, 0755); err != nil {
		return err
	}

	// Serialize retention and inference within this output parent.
	lock := filepath.Join(o.out, ".quick-check.lock")
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("acquire output lock (remove stale lock after a crashed run): %w", err)
	}
	f.Close()
	defer os.Remove(lock)
	if _, err := savedRuns(o.out); err != nil {
		return err
	}
	started := time.Now().UTC()
	dir, err := os.MkdirTemp(o.out, started.Format("20060102T150405Z")+"-")
	if err != nil {
		return err
	}
	events, err := os.Create(filepath.Join(dir, "events.ndjson"))
	if err != nil {
		return err
	}
	defer events.Close()
	toolLog, err := os.Create(filepath.Join(dir, "tool.log"))
	if err != nil {
		return err
	}
	defer toolLog.Close()
	var progress strings.Builder
	log := func(format string, args ...any) {
		fmt.Fprintf(io.MultiWriter(toolLog, &progress), format+"\n", args...)
	}
	r := matrixReport{Status: "FAIL", ServerCommitTime: "unknown", TestStartedAt: started.Format(time.RFC3339Nano)}
	for _, cell := range matrixCells {
		r.Cells = append(r.Cells, report{ID: cell.id, Images: cell.images, TargetOnly: cell.targetOnly, NoCache: cell.noCache, TwoSlots: cell.twoSlots, Status: "NOT_RUN"})
	}
	var harnessRevision string
	if revision, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		harnessRevision = strings.TrimSpace(string(revision))
	}
	c := &client{ctx: ctx, host: strings.TrimRight(o.host, "/"), token: os.Getenv("KRONK_TOKEN"), timeout: o.timeout, traces: make(map[string]string), events: json.NewEncoder(events), eventMu: new(sync.Mutex)}
	defer func() {
		// Save interrupted work independently of the cancelled request context.
		// releaseTags bounds its remote lookup with its own timeout.
		finalCtx := context.Background()
		tags, tagErr := releaseTags(finalCtx)
		r.ReleaseTag = tags[r.ServerRevision]
		for i := range r.Cells {
			if r.Cells[i].Status != "NOT_RUN" {
				r.Cells[i].ReleaseTag = r.ReleaseTag
			}
		}
		runErr = errors.Join(runErr, tagErr)
		if runErr != nil {
			r.Findings = append(r.Findings, runErr.Error())
		}
		if len(r.Findings) == 0 {
			r.Status = "PASS"
			for _, cell := range r.Cells {
				if cell.Status == "FAIL" {
					r.Status = "FAIL"
				}
			}
		}
		if ctx.Err() != nil {
			r.Status = "INTERRUPTED"
			r.Findings = append(r.Findings, "run interrupted; remaining cells/repetitions were not run")
			runErr = errors.Join(runErr, ctx.Err())
		}
		// Preserve full metadata, answers, counters, and execution evidence for
		// investigation without expanding the summary consumed on every merge.
		evidenceErr := c.events.Encode(map[string]any{"source": "report", "report": r})
		if evidenceErr != nil {
			r.Status = "FAIL"
			r.Findings = append(r.Findings, "write diagnostic report: "+evidenceErr.Error())
		}
		data, err := json.MarshalIndent(r.summary(), "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, "summary.json"), append(data, '\n'), 0644)
		}
		if err != nil {
			r.Status = "FAIL"
			r.Findings = append(r.Findings, "write summary: "+err.Error())
		}
		runErr = errors.Join(runErr, evidenceErr, err)
		log("%s: %d run findings", r.Status, len(r.Findings))
		// Live progress remains append-only; prepend the verdict once finalized.
		var logErr error
		if _, logErr = toolLog.Seek(0, io.SeekStart); logErr == nil {
			_, logErr = io.WriteString(toolLog, r.logSummary()+"\n--- Phase progress ---\n"+progress.String())
		}
		writeErr := errors.Join(evidenceErr, err, logErr, events.Close(), toolLog.Close())
		runErr = errors.Join(runErr, writeErr)
		if writeErr == nil && tagErr == nil {
			finalDir, retentionErr := retainRuns(finalCtx, o.out, dir, tags)
			dir = finalDir
			runErr = errors.Join(runErr, retentionErr)
			reported = retentionErr == nil
		} else {
			fmt.Fprintln(os.Stderr, "Artifacts or release recognition could not be finalized; previous runs retained.")
		}
		fmt.Fprintln(os.Stdout, strings.SplitN(r.logSummary(), "\n", 2)[0])
		if len(r.Findings) > 0 {
			fmt.Fprintln(os.Stdout, "Details: summary.json (run findings)")
		}
		if r.Status != "PASS" {
			runErr = errors.Join(runErr, fmt.Errorf("quick-check has findings; inspect %s", dir))
		}
	}()
	var group *report
	finishGroup := func() error {
		owner := group
		group = nil
		if owner == nil || owner.WarmupRequest == nil {
			return nil
		}
		// Cancellation must stop inference, not cancel its cleanup request.
		cleanup := *c
		cleanup.model = owner.Model
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cleanup.ctx = cleanupCtx
		err := cleanup.unload()
		if err != nil {
			owner.Status = "FAIL"
			owner.Findings = append(owner.Findings, "model group cleanup: "+err.Error())
			if ctx.Err() != nil {
				fmt.Fprintf(os.Stderr, "Interrupted model cleanup: %v\n", err)
			}
		}
		return err
	}
	defer func() { runErr = errors.Join(runErr, finishGroup()) }()
	var cellErrors error
	headerPrinted := false
	for i, cell := range matrixCells {
		if !selected[cell.id] {
			continue
		}
		if ctx.Err() != nil {
			return errors.Join(cellErrors, ctx.Err())
		}
		o.images = cell.images
		c.cell = cell.id
		c.model = modelID
		if cell.targetOnly {
			c.model = targetOnlyModelID
		}
		if cell.noCache {
			c.model = noCacheModelID
			if cell.targetOnly {
				c.model = noCacheTargetOnlyModelID
			}
		}
		if cell.twoSlots {
			c.model += "-2S"
		}
		if (report{ID: cell.id}).isDense() {
			c.model = denseModelID + strings.TrimPrefix(c.model, strings.TrimSuffix(modelID, "/QUICK-CHECK"))
		}
		if group != nil && group.Model != c.model {
			if err := finishGroup(); err != nil {
				return errors.Join(cellErrors, err)
			}
		}
		cr := &r.Cells[i]
		*cr = report{ID: cell.id, Status: "FAIL", Workload: workloadVersion, Model: c.model, Images: cell.images, TargetOnly: cell.targetOnly, NoCache: cell.noCache, TwoSlots: cell.twoSlots, Host: o.host, ServerCommitTime: "unknown", TestStartedAt: r.TestStartedAt, RepetitionsRequested: o.repetitions, HarnessRevision: harnessRevision, ReferenceTokens: referenceTokens(cell.images, cell.noCache, cell.targetOnly, cell.twoSlots)}
		if cr.isDense() {
			// Calibrate against the chosen Dense fixture, never MoE token volumes.
			cr.ReferenceTokens = normalizationReference{}
		}
		if cr.TwoSlots {
			cr.Workload = "operations-concurrent-v2"
		}
		var err error
		if group == nil {
			group = cr
		} else {
			cr.SharedWarmupFrom, cr.MTPBackend = group.ID, group.MTPBackend
			err = c.waitIdle()
		}
		log("cell=%s", cell.id)
		if err == nil {
			err = executeCell(o, c, cr, log)
		}
		if cr.ServerRevision != "" {
			if r.ServerRevision == "" {
				r.ServerRevision, r.ServerCommitTime = cr.ServerRevision, cr.ServerCommitTime
			} else if cr.ServerRevision != r.ServerRevision || cr.ServerCommitTime != r.ServerCommitTime {
				return errors.Join(cellErrors, fmt.Errorf("server commit changed between cells"))
			}
		}
		if err != nil {
			cr.Status = "FAIL"
			cr.Findings = append(cr.Findings, err.Error())
			cellErrors = errors.Join(cellErrors, fmt.Errorf("cell %s: %w", cell.id, err))
		}
		// Both concurrent requests and all repetitions have finished. Terminal
		// output is outside measured intervals, never on the token/phase path.
		if !headerPrinted {
			fmt.Fprint(os.Stdout, strings.Join(strings.SplitN(r.logSummary(), "\n", 4)[1:3], "\n")+"\n")
			headerPrinted = true
		}
		fmt.Fprint(os.Stdout, cr.logSummary())
		if err != nil {
			if ctx.Err() != nil {
				return cellErrors
			}
			// Do not carry potentially damaged state into another cell. A successful
			// unload allows a fresh group; failed cleanup makes continuation unsafe.
			if err := finishGroup(); err != nil {
				return errors.Join(cellErrors, err)
			}
		}
	}
	return cellErrors
}

// Weights remain resident across the group's cells and their repetitions.
// Only its first selected cell owns the startup request and evidence.
func executeCell(o options, c *client, r *report, log func(string, ...any)) (runErr error) {
	for i := 1; i <= o.repetitions; i++ {
		log("repetition=%d/%d", i, o.repetitions)
		repetition := report{ID: r.ID, Status: "FAIL", Workload: r.Workload, Model: r.Model, Images: o.images, TargetOnly: r.TargetOnly, NoCache: r.NoCache, TwoSlots: r.TwoSlots, Host: o.host, ServerCommitTime: "unknown", HarnessRevision: r.HarnessRevision, Metadata: make(map[string]any), MTPBackend: r.MTPBackend}
		repetition.ReferenceTokens = r.ReferenceTokens
		repetition.SharedWarmupFrom = r.SharedWarmupFrom
		c.traces = make(map[string]string)
		c.repetition = i
		err := execute(o, c, &repetition, log)
		if err != nil {
			repetition.Findings = append(repetition.Findings, err.Error())
		}
		if len(repetition.Findings) == 0 {
			repetition.Status = "PASS"
		}
		if i == 1 {
			// Invocation metadata comes from its first repetition. Full per-run
			// metadata remains in the diagnostic report, not duplicated in summary.
			r.Metadata = repetition.Metadata
			r.ServerRevision, r.ServerCommitTime = repetition.ServerRevision, repetition.ServerCommitTime
			r.SystemTokens, r.SystemSHA256, r.MTPBackend = repetition.SystemTokens, repetition.SystemSHA256, repetition.MTPBackend
			r.ConfiguredSlots = repetition.ConfiguredSlots
			r.WarmupRequest = repetition.WarmupRequest
		}
		r.Repetitions = append(r.Repetitions, repetition)
		for _, finding := range repetition.Findings {
			r.Findings = append(r.Findings, fmt.Sprintf("repetition %d: %s", i, finding))
		}
		if repetition.Status != "PASS" {
			return fmt.Errorf("repetition %d failed; %d remaining repetitions unrun", i, o.repetitions-i)
		}
	}
	r.Status = "PASS"
	return nil
}

func execute(o options, c *client, r *report, log func(string, ...any)) (runErr error) {
	// Keep a descriptor and identity so rotation cannot silently lose evidence.
	serverLog, err := os.Open(o.serverLog)
	if err != nil {
		return fmt.Errorf("required server log: %w", err)
	}
	defer serverLog.Close()
	info, err := serverLog.Stat()
	if err != nil {
		return err
	}
	if _, err := serverLog.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	// Preserve available evidence on every exit, including failed warm-up and
	// interruption before the workload starts. This runs after workload clocks stop.
	defer func() {
		runErr = errors.Join(runErr, collectEvidence(serverLog, info, o.serverLog, c, r))
	}()
	for _, path := range []string{"/v1/liveness", "/v1/kronk/libs", "/v1/devices"} {
		var metadata any
		if err := c.json("GET", path, nil, &metadata); err != nil {
			return fmt.Errorf("server provenance %s: %w", path, err)
		}
		r.Metadata[path] = metadata
		if path == "/v1/liveness" {
			liveness, _ := metadata.(map[string]any)
			delete(liveness, "vcs_modified") // Older servers may still expose the retired flag.
			r.ServerRevision, _ = liveness["vcs_revision"].(string)
			if stamp, ok := liveness["vcs_time"].(string); ok && stamp != "" {
				r.ServerCommitTime = stamp
			}
			log("Tested commit: %s from %s", r.serverVersion(), r.ServerCommitTime)
		}
	}
	if c.repetition == 1 && r.SharedWarmupFrom == "" {
		// Reset only at group startup, not between its cells or repetitions.
		if err := c.unload(); err != nil {
			return err
		}
		warmup, err := c.chat("warmup", []message{{Role: "user", Content: "Reply OK."}}, 8, false)
		r.WarmupRequest = &warmup
		if err != nil {
			return fmt.Errorf("warm-up request: %w", err)
		}
	}
	var detail struct {
		Metadata      map[string]string `json:"metadata"`
		HasProjection bool              `json:"has_projection"`
		Config        struct {
			IMC *bool `json:"incremental-cache"`
		} `json:"model_config"`
	}
	var rawDetail json.RawMessage
	if err := c.json("GET", "/v1/kronk/models/"+url.PathEscape(c.model), nil, &rawDetail); err != nil {
		return err
	}
	if err := json.Unmarshal(rawDetail, &detail); err != nil {
		return err
	}
	r.Metadata["model"] = rawDetail
	architecture := detail.Metadata["general.architecture"]
	expertCount := detail.Metadata[architecture+".expert_count"]
	experts, expertErr := strconv.Atoi(expertCount)
	if r.isDense() {
		if architecture == "" || strings.Contains(strings.ToLower(architecture), "moe") || experts > 1 || (expertCount != "" && (expertErr != nil || experts < 0)) {
			return fmt.Errorf("fixture metadata must attest Dense routing, architecture=%q expert_count=%q", architecture, expertCount)
		}
	} else if architecture != "qwen35moe" || experts <= 1 {
		return fmt.Errorf("fixture metadata must attest the selected Qwen3.6 MoE architecture")
	}
	if o.images && !detail.HasProjection {
		return fmt.Errorf("image cell requires the model's projection file")
	}
	e, err := c.snapshot()
	if err != nil {
		return err
	}
	r.Metadata["scheduler"] = e
	slots := 1
	if r.TwoSlots {
		slots = 2
	}
	if detail.Config.IMC == nil || *detail.Config.IMC == r.NoCache || len(e.Slots) != slots || (!r.TargetOnly && (!e.MTP || e.NDraft <= 0)) || (r.TargetOnly && (e.MTP || e.NDraft != 0)) {
		return fmt.Errorf("fixture must expose requested IMC mode (off=%t), exactly %d slots, and requested MTP mode (target-only=%t): %+v", r.NoCache, slots, r.TargetOnly, e)
	}
	r.Slot = e.Slots[0].ID
	for _, slot := range e.Slots {
		r.ConfiguredSlots = append(r.ConfiguredSlots, slot.ID)
	}
	system, tokens, err := c.systemPrompt()
	if err != nil {
		return err
	}
	r.SystemTokens, r.SystemSHA256 = tokens, fmt.Sprintf("%x", sha256.Sum256([]byte(system)))
	log("workload=%s model=%s system_tokens=%d", r.Workload, c.model, tokens)
	if r.TwoSlots {
		return runConcurrent(c, r, system, log)
	}
	a := conversation(system, "ORCHID-47", "Maren", "Northport")
	b := conversation(system, "COBALT-83", "Tobin", "Southbank")
	if o.images {
		a, err = picturedConversation(a, "giraffe.jpg")
		if err != nil {
			return err
		}
		b, err = picturedConversation(b, "deer/image10.jpg")
		if err != nil {
			return err
		}
	}
	start := time.Now()
	perform := func(phase string, messages []message, expected, forbidden []string, cancelled bool) (result, error) {
		if o.images {
			animal, foreign := "giraffe", "deer"
			if phase == "fresh-session" || phase == "cancel" {
				animal, foreign = foreign, animal
			}
			expected = append(expected, animal)
			forbidden = append(forbidden, foreign)
		}
		res, err := c.chat(phase, messages, 384, cancelled)
		if err == nil && !cancelled {
			err = checkAnswer(res, expected, forbidden, r.TargetOnly)
		}
		if err != nil {
			res.Error = err.Error()
			err = fmt.Errorf("%s: %w", phase, err)
		}
		r.Results = append(r.Results, res)
		log("phase=%s elapsed=%.3fs first_content=%.3fs prompt=%d cached=%d output=%d drafts=%d error=%v", phase, res.Elapsed, res.FirstContent, res.Usage.PromptTokens, res.Usage.PromptTokensDetails.CachedTokens, res.Usage.CompletionTokens, res.Usage.DraftTokens, err)
		return res, err
	}
	cold, err := perform("cold", a, []string{"ORCHID-47", "Maren", "Northport"}, nil, false)
	if err == nil {
		_, err = perform("replay", a, []string{"ORCHID-47", "Maren", "Northport"}, nil, false)
	}
	if err == nil {
		question := "Change the destination to Eastmere; keep the identifier and owner unchanged. Review the updated handover using between 60 and 80 words in total, including its current facts. Do not repeat the previous destination."
		if o.images {
			question += " Include the animal from the picture already in this conversation."
		}
		continued := append(append([]message{}, a...), message{Role: "assistant", Content: cold.Content}, message{Role: "user", Content: question})
		_, err = perform("append", continued, []string{"ORCHID-47", "Maren", "Eastmere"}, []string{"Northport"}, false)
	}
	var fresh result
	if err == nil {
		fresh, err = perform("fresh-session", b, []string{"COBALT-83", "Tobin", "Southbank"}, []string{"ORCHID-47", "Maren", "Eastmere", "Northport"}, false)
	}
	if err == nil {
		cancelMessages := append(append([]message{}, b...), message{Role: "assistant", Content: fresh.Content}, message{Role: "user", Content: "For interrupted handover INTERRUPTED-91, write a long recovery checklist, repeating its identifier in every paragraph."})
		var cancelled result
		cancelled, err = perform("cancel", cancelMessages, nil, nil, true)
		if err == nil && !cancelled.Cancelled {
			err = fmt.Errorf("active cancellation was not observed")
		}
		if err == nil {
			released := time.Now()
			err = c.waitIdle()
			r.CancelReleaseSeconds = time.Since(released).Seconds()
		}
		if err == nil {
			_, err = perform("recovery", a, []string{"ORCHID-47", "Maren", "Northport"}, []string{"COBALT-83", "Tobin", "Southbank", "INTERRUPTED-91"}, false)
		}
	}
	if err == nil {
		err = c.waitIdle()
	}
	r.WorkloadSeconds = time.Since(start).Seconds()
	return err
}

func (c *client) snapshot() (engine, error) {
	var engines []engine
	if err := c.json("GET", "/v1/kronk/models/slots", nil, &engines); err != nil {
		return engine{}, err
	}
	for _, e := range engines {
		if e.ModelID == c.model {
			return e, nil
		}
	}
	return engine{}, fmt.Errorf("missing scheduler for %s", c.model)
}

func (c *client) waitIdle() error {
	deadline := time.Now().Add(15 * time.Second)
	for {
		poll := *c
		poll.timeout = min(c.timeout, time.Until(deadline))
		if poll.timeout <= 0 {
			return fmt.Errorf("capacity not idle after 15s")
		}
		e, err := poll.snapshot()
		if err != nil {
			return err
		}
		idle := len(e.Slots) > 0
		for _, slot := range e.Slots {
			idle = idle && slot.Phase == "idle"
		}
		if idle {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("capacity not idle after 15s")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (c *client) unload() error {
	var models []struct {
		ID string `json:"id"`
	}
	if err := c.json("GET", "/v1/kronk/models/ps", nil, &models); err != nil {
		return err
	}
	for _, m := range models {
		if m.ID != c.model {
			continue
		}
		if err := c.waitIdle(); err != nil {
			return err
		}
		var reply struct{ ID, Status string }
		if err := c.json("POST", "/v1/kronk/models/unload", map[string]string{"id": c.model}, &reply); err != nil {
			return err
		}
		if reply.ID != c.model || reply.Status != "unloaded" {
			return fmt.Errorf("unexpected unload response: %+v", reply)
		}
		var remaining []struct {
			ID string `json:"id"`
		}
		if err := c.json("GET", "/v1/kronk/models/ps", nil, &remaining); err != nil {
			return err
		}
		for _, m := range remaining {
			if m.ID == c.model {
				return fmt.Errorf("fixture remains loaded after unload")
			}
		}
	}
	return nil
}

func (c *client) systemPrompt() (string, int, error) {
	// Measure the fixed text without resizing it for the selected tokenizer.
	// A fixed per-cell/repetition namespace prevents earlier working/System
	// snapshots from serving this cell/repetition's initial cold prompt.
	// It is stable across invocations, never random or resized to fit a tokenizer.
	text := fmt.Sprintf("Quick-check cell %s; repetition %d.\n\n", c.cell, c.repetition) + handbook()
	var count struct {
		Tokens int `json:"tokens"`
	}
	if err := c.json("POST", "/v1/tokenize", map[string]any{"model": c.model, "input": text, "apply_template": false}, &count); err != nil {
		return "", 0, err
	}
	if count.Tokens <= 0 {
		return "", 0, fmt.Errorf("invalid system prompt token count: %d", count.Tokens)
	}
	return text, count.Tokens, nil
}

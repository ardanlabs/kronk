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
	"syscall"
	"time"
)

const modelID = "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK"

type options struct {
	host        string
	out         string
	serverLog   string
	timeout     time.Duration
	repetitions int
}

type report struct {
	Status               string         `json:"status"`
	Workload             string         `json:"workload"`
	Model                string         `json:"model"`
	Host                 string         `json:"host"`
	HarnessRevision      string         `json:"harness_revision"`
	ServerRevision       string         `json:"server_revision"`
	ServerCommitTime     string         `json:"server_commit_time"`
	ReleaseTag           string         `json:"release_tag,omitempty"`
	TestStartedAt        string         `json:"test_started_at,omitempty"`
	Metadata             map[string]any `json:"server_metadata"`
	SystemTokens         int            `json:"system_tokens"`
	SystemSHA256         string         `json:"system_sha256"`
	Slot                 int            `json:"slot"`
	MTPBackend           string         `json:"mtp_backend"`
	WarmupRequest        *result        `json:"warmup_request,omitempty"`
	WorkloadSeconds      float64        `json:"workload_seconds"`
	CancelReleaseSeconds float64        `json:"cancel_release_seconds"`
	Results              []result       `json:"results"`
	Findings             []string       `json:"findings"`
	RepetitionsRequested int            `json:"repetitions_requested,omitempty"`
	Repetitions          []report       `json:"repetitions,omitempty"`
	evidenceValidated    bool
}

// Field order is the human reading order in summary.json: environment, run,
// then repetitions and their requests.
type runSummary struct {
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
	Status               string                            `json:"status"`
	Findings             []string                          `json:"findings"`
	RepetitionsRequested int                               `json:"repetitions_requested"`
	RepetitionsExecuted  int                               `json:"repetitions_executed"`
	WarmupRequest        map[string]any                    `json:"warmup_request,omitempty"`
	Measurements         map[string]map[string]measurement `json:"measurements,omitempty"`
	Repetitions          []repetitionSummary               `json:"repetitions"`
}

// Keep the decision/comparison surface separate from the diagnostic record.
func (r report) summary() runSummary {
	liveness, _ := r.Metadata["/v1/liveness"].(map[string]any)
	libraries, _ := r.Metadata["/v1/kronk/libs"].(map[string]any)
	hardware, _ := r.Metadata["/v1/devices"].(map[string]any)
	summary := runSummary{
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
		Cell:                 "MoE / one slot / IMC on / MTP on / images off",
		Workload:             r.Workload,
		Status:               r.Status,
		Findings:             append([]string{}, r.Findings...),
		RepetitionsRequested: r.RepetitionsRequested,
		RepetitionsExecuted:  len(r.Repetitions),
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
				if name == "warmup" && i > 0 {
					continue // Startup is measured and validated once per invocation.
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
	cache := verdict("warmup", "cold", "replay", "append", "fresh-session", "cancel", "recovery")
	isolation := verdict("fresh-session", "recovery")
	recovery := verdict("cancel", "recovery")
	version := r.serverVersion()
	if version != "unknown" {
		version = version[:7]
	}
	if r.ReleaseTag != "" {
		version += " (release " + r.ReleaseTag + ")"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Quick-check: %s\nTested commit: %s from %s\nTest started: %s\nIMC/MTP: %s | Serial isolation: %s | Cancel/recovery: %s\n", r.Status, version, r.ServerCommitTime, r.TestStartedAt, cache, isolation, recovery)
	fmt.Fprintf(&b, "Cell: MoE/1s/IMC+/MTP+/img- | Warm-up: %s | Repetitions: %d/%d\n", warmup, len(r.Repetitions), r.RepetitionsRequested)
	fmt.Fprintf(&b, "  Workload s [SD]: %s | Normalized s [SD]: %s\n", workload, reference)
	if r.Status != "PASS" {
		fmt.Fprintln(&b, "Details: summary.json (fix setup/evidence failures before deeper inference checks)")
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
	flag.IntVar(&o.repetitions, "repetitions", 2, "complete workload repetitions; stop on first failure")
	flag.Parse()
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
	started := time.Now().UTC()
	dir, err := os.MkdirTemp(o.out, started.Format("20060102T150405Z")+"-")
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if ctx.Err() != nil && !committed {
			err := os.RemoveAll(dir)
			runErr = errors.Join(runErr, ctx.Err(), err)
			reported = true
			if err != nil {
				fmt.Fprintf(os.Stderr, "Quick-check interrupted; could not remove unfinished output %s: %v\n", dir, err)
			} else {
				fmt.Fprintln(os.Stderr, "Quick-check interrupted; unfinished output removed; previous runs retained.")
			}
		}
	}()
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
	r := report{Status: "FAIL", Workload: workloadVersion, Model: modelID, Host: o.host, ServerCommitTime: "unknown", TestStartedAt: started.Format(time.RFC3339Nano), RepetitionsRequested: o.repetitions}
	if revision, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		r.HarnessRevision = strings.TrimSpace(string(revision))
	}
	c := &client{ctx: ctx, host: strings.TrimRight(o.host, "/"), token: os.Getenv("KRONK_TOKEN"), timeout: o.timeout, traces: make(map[string]string), events: json.NewEncoder(events)}
	defer func() {
		if ctx.Err() != nil {
			return // The interruption cleanup removes this incomplete run.
		}
		tags, tagErr := releaseTags(ctx)
		r.ReleaseTag = tags[r.ServerRevision]
		runErr = errors.Join(runErr, tagErr)
		if runErr != nil {
			r.Findings = append(r.Findings, runErr.Error())
		}
		if len(r.Findings) == 0 {
			r.Status = "PASS"
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
		log("%s: %d findings; repetitions executed %d/%d", r.Status, len(r.Findings), len(r.Repetitions), r.RepetitionsRequested)
		// Live progress remains append-only; prepend the verdict once finalized.
		var logErr error
		if _, logErr = toolLog.Seek(0, io.SeekStart); logErr == nil {
			_, logErr = io.WriteString(toolLog, r.logSummary()+"\n--- Phase progress ---\n"+progress.String())
		}
		writeErr := errors.Join(evidenceErr, err, logErr, events.Close(), toolLog.Close())
		runErr = errors.Join(runErr, writeErr)
		if writeErr == nil && tagErr == nil {
			finalDir, retentionErr := retainRuns(ctx, o.out, dir, tags)
			dir = finalDir
			runErr = errors.Join(runErr, retentionErr)
			reported = retentionErr == nil
			committed = retentionErr == nil
			if ctx.Err() != nil && !committed {
				return
			}
		} else {
			fmt.Fprintln(os.Stderr, "Artifacts or release recognition could not be finalized; previous runs retained.")
		}
		fmt.Fprint(os.Stdout, r.logSummary())
		if len(r.Findings) > 0 {
			runErr = errors.Join(runErr, fmt.Errorf("quick-check has findings; inspect %s", dir))
		}
	}()
	defer func() {
		if r.WarmupRequest != nil {
			// Cancellation must stop inference, not cancel its cleanup request.
			cleanup := *c
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cleanup.ctx = cleanupCtx
			err := cleanup.unload()
			runErr = errors.Join(runErr, err)
			if err != nil && ctx.Err() != nil {
				fmt.Fprintf(os.Stderr, "Interrupted model cleanup: %v\n", err)
			}
		}
	}()
	for i := 1; i <= o.repetitions; i++ {
		log("repetition=%d/%d", i, o.repetitions)
		repetition := report{Status: "FAIL", Workload: workloadVersion, Model: modelID, Host: o.host, ServerCommitTime: "unknown", HarnessRevision: r.HarnessRevision, Metadata: make(map[string]any), MTPBackend: r.MTPBackend}
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
	if c.repetition == 1 {
		// Reset only at invocation startup; weights stay resident across repetitions.
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
		Metadata map[string]string `json:"metadata"`
		Config   struct {
			IMC *bool `json:"incremental-cache"`
		} `json:"model_config"`
	}
	var rawDetail json.RawMessage
	if err := c.json("GET", "/v1/kronk/models/"+url.PathEscape(modelID), nil, &rawDetail); err != nil {
		return err
	}
	if err := json.Unmarshal(rawDetail, &detail); err != nil {
		return err
	}
	r.Metadata["model"] = rawDetail
	experts, _ := strconv.Atoi(detail.Metadata["qwen35moe.expert_count"])
	if detail.Metadata["general.architecture"] != "qwen35moe" || experts <= 1 {
		return fmt.Errorf("fixture metadata must attest the selected Qwen3.6 MoE architecture")
	}
	e, err := c.snapshot()
	if err != nil {
		return err
	}
	r.Metadata["scheduler"] = e
	if detail.Config.IMC == nil || !*detail.Config.IMC || !e.MTP || e.NDraft <= 0 || len(e.Slots) != 1 {
		return fmt.Errorf("fixture must expose IMC on, active MTP, and exactly one slot: %+v", e)
	}
	r.Slot = e.Slots[0].ID
	system, tokens, err := c.systemPrompt()
	if err != nil {
		return err
	}
	r.SystemTokens, r.SystemSHA256 = tokens, fmt.Sprintf("%x", sha256.Sum256([]byte(system)))
	log("workload=%s model=%s system_tokens=%d", workloadVersion, modelID, tokens)
	start := time.Now()
	a := conversation(system, "ORCHID-47", "Maren", "Northport")
	b := conversation(system, "COBALT-83", "Tobin", "Southbank")
	perform := func(phase string, messages []message, expected, forbidden []string, cancelled bool) (result, error) {
		res, err := c.chat(phase, messages, 384, cancelled)
		if err == nil && !cancelled {
			err = checkAnswer(res, expected, forbidden)
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
		continued := append(append([]message{}, a...), message{Role: "assistant", Content: cold.Content}, message{Role: "user", Content: "Change the destination to Eastmere; keep the identifier and owner unchanged. Review the updated handover in about 80 words, including its current facts. Do not repeat the previous destination."})
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
	// Collect even after a workload failure; incomplete evidence never yields PASS.
	logErr := collectEvidence(serverLog, info, o.serverLog, c, r)
	return errors.Join(err, logErr)
}

func (c *client) snapshot() (engine, error) {
	var engines []engine
	if err := c.json("GET", "/v1/kronk/models/slots", nil, &engines); err != nil {
		return engine{}, err
	}
	for _, e := range engines {
		if e.ModelID == modelID {
			return e, nil
		}
	}
	return engine{}, fmt.Errorf("missing scheduler for %s", modelID)
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
			return fmt.Errorf("one-slot capacity not idle after 15s")
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
		if m.ID != modelID {
			continue
		}
		if err := c.waitIdle(); err != nil {
			return err
		}
		var reply struct{ ID, Status string }
		if err := c.json("POST", "/v1/kronk/models/unload", map[string]string{"id": modelID}, &reply); err != nil {
			return err
		}
		if reply.ID != modelID || reply.Status != "unloaded" {
			return fmt.Errorf("unexpected unload response: %+v", reply)
		}
		var remaining []struct {
			ID string `json:"id"`
		}
		if err := c.json("GET", "/v1/kronk/models/ps", nil, &remaining); err != nil {
			return err
		}
		for _, m := range remaining {
			if m.ID == modelID {
				return fmt.Errorf("fixture remains loaded after unload")
			}
		}
	}
	return nil
}

func (c *client) systemPrompt() (string, int, error) {
	// Measure the fixed text without resizing it for the selected tokenizer.
	// A fixed per-repetition namespace prevents a previous repetition's full
	// working/System snapshot from serving this one's initial cold prompt.
	// It is stable across invocations, never random or resized to fit a tokenizer.
	text := fmt.Sprintf("Quick-check repetition %d.\n\n", c.repetition) + handbook()
	var count struct {
		Tokens int `json:"tokens"`
	}
	if err := c.json("POST", "/v1/tokenize", map[string]any{"model": modelID, "input": text, "apply_template": false}, &count); err != nil {
		return "", 0, err
	}
	if count.Tokens <= 0 {
		return "", 0, fmt.Errorf("invalid system prompt token count: %d", count.Tokens)
	}
	return text, count.Tokens, nil
}

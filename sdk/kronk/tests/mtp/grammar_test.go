package mtp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

// =========================================================================
// Regression coverage for: MTP bonus token bypasses the sampler chain and
// the grammar.
//
// sdk/kronk/model/batchgen_speculative.go:353-356 forces greedy = true for
// every MTP request. The in-loop verify branch (:425-467) honours that
// correctly and routes through s.grammarSampler.SampleWithGrammar or
// llama.SamplerSample. The bonus-token block at :582-602 does not:
//
//	case greedy:                                   // true for every MTP request
//	    maskSuppressTokenLogits(targetLogits, e.model.suppressTokens)
//	    bonusToken = argmax(targetLogits)          // :594-595
//
// So on every fully-accepted speculative round the emitted bonus token is a
// raw argmax over unfiltered target logits, bypassing penalties, DRY,
// top-k/top-p/min-p, XTC, temperature, the dist sampler AND the grammar.
//
// When response_format / json_schema is in play (params.go:469,478,490 set
// p.Grammar) the unconstrained bonus token then reaches
// grammarSampler.Accept (grammar.go:436, via batchgen_tokens.go:60, via
// finalizeSpeculativeTokens at batchgen_speculative.go:762).
// llama_grammar_accept_token throws std::runtime_error, the C++ exception
// unwinds through libffi, and the whole process dies on SIGABRT:
//
//	libc++abi: terminating due to uncaught exception of type std::runtime_error:
//	  Unexpected empty grammar stack after accepting piece: } (92)
//
// Upstream llama.cpp samples every position INCLUDING the bonus through the
// same sampler chain and grammar — see
// .extras/llama.cpp/common/sampling.cpp:646-674
// (common_sampler_sample_and_accept_n).
//
// Because the failure mode is a process abort rather than a Go panic (yzma
// has no recover() in pkg/, and a Go recover() cannot catch a C++ exception
// unwinding through libffi anyway), the model work runs in a CHILD copy of
// this test binary. The parent inspects the child's exit status and output
// so a SIGABRT produces an actionable failure instead of silently taking the
// package down.
// =========================================================================

const (
	// grammarChildEnv gates the child worker. The parent sets it when it
	// re-executes this test binary.
	grammarChildEnv = "KRONK_MTP_GRAMMAR_CHILD"

	// grammarChildTest is the only test the child is allowed to run.
	grammarChildTest = "TestMTPGrammarChildWorker"

	// grammarRuns is how many identical json_schema requests the child
	// issues. The original repro aborted on run 2 of 5; 8 keeps the wall
	// clock sane (~65 tok/s, 300 max_tokens) while giving the ~15-30% of
	// bonus-sampled tokens plenty of chances to leave the grammar.
	grammarRuns = 8

	// Markers the child prints on stdout for the parent to parse. They are
	// deliberately greppable and survive interleaving with llama.cpp's own
	// stderr chatter.
	markerRun     = "MTPGRAMMAR-RUN:"
	markerInvalid = "MTPGRAMMAR-INVALID:"
	markerChatErr = "MTPGRAMMAR-CHATERR:"
	markerDone    = "MTPGRAMMAR-DONE:"
)

// grammarChildTimeout bounds the whole child run: one ~39 GB model load plus
// grammarRuns generations.
const grammarChildTimeout = 25 * time.Minute

// grammarSchema is a strict object schema. Every field is required and
// additionalProperties is false, so any token the grammar would have
// forbidden is either rejected by llama_grammar_accept_token (abort) or
// shows up as malformed JSON.
func grammarSchema() model.D {
	return model.D{
		"type": "object",
		"properties": model.D{
			"planet":   model.D{"type": "string"},
			"diameter": model.D{"type": "number"},
			"moons":    model.D{"type": "integer"},
			"summary":  model.D{"type": "string"},
		},
		"required":             []string{"planet", "diameter", "moons", "summary"},
		"additionalProperties": false,
	}
}

// grammarRequest builds the json_schema chat request used by both the MTP
// child and the non-MTP control.
func grammarRequest() model.D {
	return model.D{
		"messages": []model.D{
			{"role": "user", "content": "Describe Jupiter with a two-sentence summary."},
		},
		"max_tokens":      300,
		"enable_thinking": false,
		"response_format": model.D{
			"type": "json_schema",
			"json_schema": model.D{
				"name":   "planet",
				"strict": true,
				"schema": grammarSchema(),
			},
		},
	}
}

// =========================================================================

// grammarOutcome is the parent's view of one child run.
type grammarOutcome struct {
	// aborted is true when the child was killed by the uncaught C++ grammar
	// exception — either observed as a signal in the wait status or as the
	// runtime's SIGABRT traceback in its output (see abortNeedles).
	aborted  bool
	signal   syscall.Signal
	exitCode int

	// runsSeen / runsValid count the markers the child managed to print
	// before it died (or finished).
	runsSeen  int
	runsValid int

	// invalid holds one line per run whose output failed encoding/json
	// parsing — the grammar was bypassed but the process survived.
	invalid []string

	// chatErrs holds one line per run whose Chat call returned an error.
	chatErrs []string

	// completed is true when the child printed its DONE marker.
	completed bool

	stdout string
	stderr string
	err    error
}

// abortNeedles are the strings that identify a process death inside
// llama_grammar_accept_token. Matching on output rather than only on the
// wait status matters: the Go runtime installs its own fatal-signal handler,
// prints "SIGABRT: abort" plus a traceback, and then exits with status 2
// instead of re-raising, so WaitStatus.Signaled() is false even though the
// process was killed by the uncaught C++ exception.
var abortNeedles = []string{
	"empty grammar stack",
	"uncaught exception",
	"libc++abi",
	"llama_grammar_accept_token",
	"SIGABRT",
}

// death describes how the child process ended.
func (o grammarOutcome) death() string {
	if o.signal != 0 {
		return "signal " + o.signal.String()
	}

	return "exit status " + strconv.Itoa(o.exitCode)
}

// abortEvidence returns the lines that prove the child died inside
// llama_grammar_accept_token, or "" when none are present.
func (o grammarOutcome) abortEvidence() string {
	var hits []string

	for line := range strings.SplitSeq(o.stdout+"\n"+o.stderr, "\n") {
		for _, n := range abortNeedles {
			if strings.Contains(line, n) {
				hits = append(hits, strings.TrimSpace(line))
				break
			}
		}
	}

	return strings.Join(hits, "\n\t")
}

// crashExcerpt returns the n lines of the child's output starting at the
// first abort marker — the C++ message plus the head of the Go traceback,
// which is the part a maintainer needs. Plain tailing lands in the register
// dump at the end and shows nothing useful.
func (o grammarOutcome) crashExcerpt(n int) string {
	lines := strings.Split(o.stdout+"\n"+o.stderr, "\n")

	start := -1
	for i, line := range lines {
		for _, needle := range abortNeedles {
			if strings.Contains(line, needle) {
				start = i
				break
			}
		}
		if start >= 0 {
			break
		}
	}

	if start < 0 {
		return tail(o.stderr, n)
	}

	end := min(start+n, len(lines))

	return strings.Join(lines[start:end], "\n\t")
}

// runGrammarChild re-executes this test binary with only the child worker
// selected, and reports what happened. It is memoized because a single child
// run costs one ~39 GB model load and both TestMTPGrammarBonusToken and
// TestMTPGrammarValidityMatchesNonMTP need the same evidence. Loading the
// target twice would be pure waste.
var runGrammarChild = sync.OnceValue(func() grammarOutcome {
	exe, err := os.Executable()
	if err != nil {
		return grammarOutcome{err: fmt.Errorf("locating test binary: %w", err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), grammarChildTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe,
		"-test.run=^"+grammarChildTest+"$",
		"-test.v",
		"-test.count=1",
		"-test.timeout="+grammarChildTimeout.String(),
	)
	cmd.Env = append(os.Environ(), grammarChildEnv+"=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	o := grammarOutcome{
		stdout: stdout.String(),
		stderr: stderr.String(),
	}

	var ee *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &ee):
		o.exitCode = ee.ExitCode()
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			o.aborted = true
			o.signal = ws.Signal()
		}
	default:
		o.err = fmt.Errorf("running child %s: %w", grammarChildTest, runErr)
		return o
	}

	for line := range strings.SplitSeq(o.stdout, "\n") {
		line = strings.TrimSpace(line)

		switch {
		case strings.Contains(line, markerRun):
			o.runsSeen++
			if strings.Contains(line, "valid=true") {
				o.runsValid++
			}

		case strings.Contains(line, markerInvalid):
			o.invalid = append(o.invalid, line)

		case strings.Contains(line, markerChatErr):
			o.chatErrs = append(o.chatErrs, line)

		case strings.Contains(line, markerDone):
			o.completed = true
		}
	}

	// The Go runtime swallows the signal and exits 2 (see abortNeedles), so
	// treat matching output as an abort even when Signaled() is false.
	if !o.aborted && o.abortEvidence() != "" {
		o.aborted = true
	}

	return o
})

// tail returns the last n lines of s, for embedding child output in a
// failure message without dumping the entire llama.cpp load log.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, "\n\t")
}

// =========================================================================

// TestMTPGrammarChildWorker is the worker half of the subprocess harness. It
// is a no-op unless the parent set grammarChildEnv, so a normal `go test`
// run of this package skips it and the parent drives it explicitly.
//
// It issues grammarRuns identical response_format=json_schema requests
// against the MTP target and validates every response with encoding/json. It
// may be killed mid-run by SIGABRT from llama_grammar_accept_token — that is
// the point, and why the assertions live in the parent.
func TestMTPGrammarChildWorker(t *testing.T) {
	if os.Getenv(grammarChildEnv) == "" {
		t.Skipf("worker for the subprocess harness; set %s to run directly", grammarChildEnv)
	}

	cfg := testlib.CfgMTPChat()
	if len(cfg.ModelFiles) == 0 {
		t.Skip("no MTP model available")
	}

	krn, err := kronk.New(model.WithConfig(cfg))
	if err != nil {
		t.Fatalf("loading MTP target %v: %v", cfg.ModelFiles, err)
	}

	defer func() {
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("unloading model: %v", err)
		}
	}()

	d := grammarRequest()

	for i := range grammarRuns {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		resp, err := krn.Chat(ctx, d)
		cancel()

		if err != nil {
			fmt.Printf("%s run=%d err=%v\n", markerChatErr, i, err)
			continue
		}

		var body string
		if len(resp.Choices) > 0 {
			body = resp.Choices[0].Message.Content
		}

		var out map[string]any
		jsonErr := json.Unmarshal([]byte(body), &out)

		var draft, accepted int
		if resp.Usage != nil {
			draft, accepted = resp.Usage.DraftTokens, resp.Usage.DraftAcceptedTokens
		}

		fmt.Printf("%s run=%d valid=%t draft=%d accepted=%d\n",
			markerRun, i, jsonErr == nil, draft, accepted)

		if jsonErr != nil {
			fmt.Printf("%s run=%d err=%q body=%q\n",
				markerInvalid, i, jsonErr.Error(), body)
		}
	}

	fmt.Printf("%s runs=%d\n", markerDone, grammarRuns)
}

// TestMTPGrammarBonusToken pins the headline bug: with an MTP drafter
// active, a grammar-constrained request emits tokens the grammar forbids,
// because the bonus token at batchgen_speculative.go:582-602 takes the
// `case greedy:` arm and is produced by argmax over raw target logits
// instead of s.grammarSampler / s.sampler.
//
// Two distinct observable failures, same root cause:
//
//	(a) the child dies on SIGABRT — the out-of-grammar token reached
//	    grammarSampler.Accept and llama_grammar_accept_token threw;
//	(b) the child survives but returns malformed JSON — the grammar was
//	    bypassed and the emitted text left the schema.
//
// Either one fails this test. The fix is to route the bonus token through
// the same branch the in-loop verify uses at :425-467.
func TestMTPGrammarBonusToken(t *testing.T) {
	o := runGrammarChild()
	if o.err != nil {
		t.Fatalf("subprocess harness failed to run: %v", o.err)
	}

	t.Logf("child: runs=%d valid=%d invalid=%d chat-errors=%d completed=%t exit=%d",
		o.runsSeen, o.runsValid, len(o.invalid), len(o.chatErrs), o.completed, o.exitCode)

	if o.aborted {
		t.Errorf(`MTP + grammar aborted the process (%s) after %d/%d completed requests.

An out-of-grammar token reached llama_grammar_accept_token. Source:
  sdk/kronk/model/batchgen_speculative.go:582-602 — the "case greedy:" arm of the
  "accepted == nDraft" block emits argmax(targetLogits) (:594-595) with no grammar
  and no sampler chain. greedy is forced true for every MTP request at :353-356.
  The token then flows finalizeSpeculativeTokens (:762) -> handleSpeculativeToken ->
  handleSampledToken (batchgen_tokens.go:60) -> grammarSampler.Accept (grammar.go:436)
  -> llama.SamplerAccept -> llama_grammar_accept_token, which throws.

The C++ exception cannot be recovered from Go, so this kills the whole process,
not just the request.

Fix: mirror the in-loop branch at :425-467 — sample the bonus token with
s.grammarSampler.SampleWithGrammar (or llama.SamplerSample) instead of argmax.
Upstream does exactly this for every position including the bonus:
.extras/llama.cpp/common/sampling.cpp:646-674.

Child evidence:
	%s

Child crash excerpt:
	%s`, o.death(), o.runsSeen, grammarRuns, o.abortEvidence(), o.crashExcerpt(40))

		return
	}

	for _, line := range o.invalid {
		t.Errorf(`MTP + grammar produced output that is not valid JSON, so the grammar did not
constrain every emitted token: %s

Source: sdk/kronk/model/batchgen_speculative.go:594-595 emits the bonus token as
argmax over raw target logits, bypassing s.grammarSampler.`, line)
	}

	for _, line := range o.chatErrs {
		t.Errorf("MTP + grammar request failed: %s", line)
	}

	if !o.completed {
		t.Errorf(`child worker did not reach its DONE marker (exit=%d, %d/%d runs reported)
without a signal — it exited early for an unexpected reason.

Child stdout tail:
	%s

Child stderr tail:
	%s`, o.exitCode, o.runsSeen, grammarRuns, tail(o.stdout, 25), tail(o.stderr, 25))

		return
	}

	if o.runsSeen != grammarRuns {
		t.Errorf("child reported %d runs, want %d", o.runsSeen, grammarRuns)
	}
}

// TestMTPGrammarValidityMatchesNonMTP is the guard: a json_schema request is
// a hard constraint, so its validity rate must be 1.0 regardless of whether
// a drafter is attached. It runs the identical request against a NON-MTP
// target in this process to establish that 1.0 is achievable with the same
// schema and prompt, then compares it against the MTP child's rate.
//
// A gap between the two rates is the signature of the bonus-token bypass at
// sdk/kronk/model/batchgen_speculative.go:582-602: the non-MTP path never
// forces greedy (:353-356 is skipped) so every token stays inside
// s.grammarSampler.
func TestMTPGrammarValidityMatchesNonMTP(t *testing.T) {
	// Preference order: the same Qwen3.6-35B-A3B base WITHOUT the MTP head
	// isolates the drafter as the only variable; Qwen3-8B is an acceptable
	// second choice when only it is on disk.
	candidates := []struct {
		name string
		cfg  model.Config
	}{
		{"Qwen3.6-35B-A3B (no MTP head)", testlib.CfgHybridChat()},
		{"Qwen3-8B", testlib.CfgThinkToolChat()},
	}

	var ctlName string
	var ctlCfg model.Config
	for _, c := range candidates {
		if len(c.cfg.ModelFiles) != 0 {
			ctlName, ctlCfg = c.name, c.cfg
			break
		}
	}

	if ctlName == "" {
		t.Skip("no non-MTP control model downloaded")
	}

	t.Logf("non-MTP control: %s (%v)", ctlName, ctlCfg.ModelFiles)

	ctlValid, ctlRuns := grammarControlRate(t, ctlCfg)

	o := runGrammarChild()
	if o.err != nil {
		t.Fatalf("subprocess harness failed to run: %v", o.err)
	}

	ctlRate := float64(ctlValid) / float64(ctlRuns)
	mtpRate := float64(o.runsValid) / float64(grammarRuns)

	t.Logf("json_schema validity: non-MTP %d/%d (%.2f), MTP %d/%d (%.2f, aborted=%t)",
		ctlValid, ctlRuns, ctlRate, o.runsValid, grammarRuns, mtpRate, o.aborted)

	if ctlRate != 1.0 {
		t.Fatalf(`non-MTP control produced %d/%d valid JSON responses — the control itself is
broken, so this test cannot attribute anything to MTP. Investigate the grammar
path before reading the MTP result below.`, ctlValid, ctlRuns)
	}

	switch {
	case o.aborted:
		t.Errorf(`non-MTP + grammar: %d/%d valid. MTP + grammar: process died after
%d/%d requests.

The same request is a 100%% hard constraint without a drafter and kills the
process with one. Root cause: sdk/kronk/model/batchgen_speculative.go:594-595
emits the bonus token as argmax over raw target logits with no grammar, and
:353-356 forces that arm for every MTP request.

Child evidence:
	%s`,
			ctlValid, ctlRuns, o.runsSeen, grammarRuns, o.abortEvidence())

	case mtpRate != 1.0:
		t.Errorf(`json_schema validity dropped with MTP enabled: non-MTP %d/%d (%.2f) vs
MTP %d/%d (%.2f). A grammar is a hard constraint — both must be 1.00.

Root cause: sdk/kronk/model/batchgen_speculative.go:594-595 emits the bonus
token as argmax over raw target logits with no grammar; :353-356 forces that
arm for every MTP request. Invalid runs:
	%s`,
			ctlValid, ctlRuns, ctlRate, o.runsValid, grammarRuns, mtpRate,
			strings.Join(o.invalid, "\n\t"))
	}
}

// grammarControlRate runs the same json_schema request against a non-MTP
// target and reports how many responses parsed as JSON. It loads and unloads
// the control model itself so the MTP child never shares the box with it.
func grammarControlRate(t *testing.T, cfg model.Config) (valid int, runs int) {
	t.Helper()

	krn, err := kronk.New(model.WithConfig(cfg))
	if err != nil {
		t.Fatalf("loading non-MTP control %v: %v", cfg.ModelFiles, err)
	}

	defer func() {
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("unloading control model: %v", err)
		}
	}()

	d := grammarRequest()

	for i := range grammarRuns {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		resp, err := krn.Chat(ctx, d)
		cancel()

		runs++

		if err != nil {
			t.Errorf("control run %d: chat: %v", i, err)
			continue
		}

		var body string
		if len(resp.Choices) > 0 {
			body = resp.Choices[0].Message.Content
		}

		var out map[string]any
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			t.Logf("control run %d: invalid JSON: %v: %q", i, err, body)
			continue
		}

		valid++
	}

	return valid, runs
}

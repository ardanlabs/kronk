//go:build kronkdiff

// =========================================================================
// Differential coherence harness for the MTP target.
//
// PURPOSE. Users report that long multi-turn "reasoning" conversations with
// unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL derail: the model contradicts
// something it already said, or starts a task and abandons it mid-way. The
// question this file answers is a FALSIFICATION question, not a bug hunt:
// is that the model's own behaviour, or does Kronk cause it?
//
// The only way to answer it is a differential. The same GGUF is driven
// three ways and the outputs are scored by identical objective rules:
//
//	 leg              driver                        MTP
//	 ---------------- ----------------------------- --------------------
//	 upstream-none    llama-server --spec-type none off
//	 upstream-mtp     llama-server --spec-type       on (llama.cpp's own
//	                    draft-mtp                      MTP implementation)
//	 kronk            kronk.Chat                    on (auto-enabled)
//
// The upstream-none/upstream-mtp pair isolates speculative decoding from
// the model: both legs are the same weights, the same quant and the same
// llama.cpp build, differing only in whether the MTP head drafts. If both
// upstream legs are coherent and the Kronk leg is not, the defect is in
// Kronk/yzma. If all three derail, it is the model or the quant.
//
// WHY KRONK CANNOT TURN MTP OFF. The drafter auto-enables whenever the
// target GGUF declares "<arch>.nextn_predict_layers" > 0 and the loaded
// libllama exports the pre-norm APIs (sdk/kronk/model/config.go:920,
// draft_mtp.go:460). No Config field suppresses it: an MTP nDraft override
// with NDraft == 0 is rewritten to defMTPNDraft by adjustConfig
// (config.go:760-766). So the MTP on/off contrast has to come from
// upstream's --spec-type, which is exactly what this harness does.
//
// COMPARABILITY CAVEATS, both structural and both recorded in the report:
//
//   - Greedy decoding is unreachable through Kronk. params.go:724 rewrites
//     temperature <= 0 to 0.8 and AddParams (params.go:372) drops a zero
//     temperature, so "temperature: 0" does not survive. Every leg is
//     therefore run at the same explicit non-zero sampling settings
//     (diffSampling) rather than at temperature 0.
//   - Kronk's chat API exposes no "seed" parameter, so its sampler cannot
//     be pinned. Token-identical output across legs is not expected and is
//     not asserted. The Kronk leg is instead sampled diffRepeats times and
//     judged on qualitative coherence, which is what the user's report is
//     about.
//
// The probes are deliberately machine-checkable. Each is a 6-turn
// conversation that plants a constraint or a fact early and then requires
// it later, so "contradicted itself" and "abandoned the task" become
// counts rather than opinions. See diffProbes for the verbatim text.
//
// OPT-IN. Guarded three ways so it can never run in normal CI: the
// kronkdiff build tag, KRONK_MTP_DIFF=1, and -short. It skips cleanly when
// the GGUF or the llama-server binary is absent.
//
//	go test -tags kronkdiff -timeout 3h -v \
//	  -run TestMTPDifferential ./sdk/kronk/tests/mtp/
//
// with KRONK_MTP_DIFF=1 set. Optional knobs:
//
//	KRONK_MTP_DIFF_MODEL     override the target GGUF path
//	KRONK_MTP_DIFF_CONTROL   non-MTP sibling GGUF for the Kronk control leg
//	KRONK_MTP_DIFF_SERVER    llama-server binary (default: the one in
//	                         ~/.kronk/libraries/<os>/<arch>/<accel>/)
//	KRONK_MTP_DIFF_OUT       directory for transcript JSON
//	KRONK_MTP_DIFF_REPEATS   Kronk samples per probe (default 2)
//	KRONK_MTP_DIFF_UPSTREAM  set to 0 to skip the upstream legs
// =========================================================================

package mtp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
)

// =========================================================================
// Opt-in gate and configuration.

const diffEnv = "KRONK_MTP_DIFF"

// diffModelDefault is where the reported target lands under the default
// model root. KRONK_MTP_DIFF_MODEL overrides it.
const diffModelDefault = "unsloth/Qwen3.6-35B-A3B-MTP-GGUF/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf"

// diffControlDefault is the same base model WITHOUT an MTP head. It gives a
// Kronk-internal drafter-off reading, at the cost of a different quant.
const diffControlDefault = "unsloth/Qwen3.6-35B-A3B-GGUF/Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf"

// Context geometry, shared by every leg so the comparison is meaningful.
// Recorded verbatim in each transcript.
const (
	diffNBatch   = 2048
	diffNUBatch  = 512
	diffNSeqMax  = 1
	diffMaxToken = 700
)

// diffCtxDefault matches how the reporting user actually runs the model:
// a 131072-token window. That matters more than it looks. The reported
// derailment does NOT happen at the edge of the window — it happens around
// 20-30k USED tokens inside that 128k window, i.e. at ~20% occupancy with
// no eviction, no context shift and no truncation in play. Reproducing at a
// small n_ctx would therefore be testing the wrong regime entirely: a 16k
// window forces overflow handling that the user never reaches, and a
// large window changes KV allocation, cache-reuse and checkpoint behaviour
// that a small one never exercises. P7 is the probe aimed at that regime.
const diffCtxDefault = 131072

// diffCtx is the context window for every leg. Override with
// KRONK_MTP_DIFF_CTX (e.g. 16384) on a machine that cannot hold the KV
// cache for a 128k window alongside the ~39 GB of weights.
func diffCtx() int {
	if v := os.Getenv("KRONK_MTP_DIFF_CTX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}

	return diffCtxDefault
}

// diffSampling is applied identically to every leg. Non-zero temperature is
// forced on us (see the file header): Kronk rewrites temperature <= 0.
var diffSampling = map[string]any{
	"temperature":     0.7,
	"top_k":           40,
	"top_p":           0.9,
	"min_p":           0.0,
	"repeat_penalty":  1.0,
	"max_tokens":      diffMaxToken,
	"enable_thinking": false,
}

// diffRepeats is how many times the Kronk leg samples each probe. Kronk has
// no seed control, so a single sample cannot distinguish a real defect from
// sampling luck.
func diffRepeats() int {
	if v := os.Getenv("KRONK_MTP_DIFF_REPEATS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}

	return 2
}

// activeProbes filters diffProbes by KRONK_MTP_DIFF_PROBES, a comma-separated
// list of id substrings (e.g. "P6"). Empty means all of them. Useful for
// re-running one expensive probe without paying for the whole set.
func activeProbes() []diffProbe {
	filter := os.Getenv("KRONK_MTP_DIFF_PROBES")
	if filter == "" {
		return diffProbes
	}

	var out []diffProbe
	for _, p := range diffProbes {
		for _, want := range strings.Split(filter, ",") {
			if want = strings.TrimSpace(want); want != "" &&
				strings.Contains(p.id, want) {
				out = append(out, p)
				break
			}
		}
	}

	return out
}

func diffOutDir(t *testing.T) string {
	if v := os.Getenv("KRONK_MTP_DIFF_OUT"); v != "" {
		if err := os.MkdirAll(v, 0o755); err != nil {
			t.Fatalf("creating transcript dir %s: %v", v, err)
		}

		return v
	}

	return t.TempDir()
}

// diffGate enforces the opt-in. Every entry point calls it first.
func diffGate(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("differential harness loads a ~39 GB model; skipped under -short")
	}

	if os.Getenv(diffEnv) != "1" {
		t.Skipf("differential harness is opt-in; set %s=1 to run", diffEnv)
	}
}

// diffModelPath resolves a GGUF, preferring the env override and falling
// back to the default model root. It returns "" when nothing is on disk so
// callers can skip instead of fail.
func diffModelPath(env string, rel string) string {
	if v := os.Getenv(env); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}

		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	p := filepath.Join(home, ".kronk", "models", rel)
	if _, err := os.Stat(p); err != nil {
		return ""
	}

	return p
}

// diffServerBin locates llama.cpp's own llama-server. Kronk downloads the
// upstream binaries alongside the shared libraries it loads, so the
// reference implementation is normally already on disk at exactly the build
// Kronk itself links against — which is what makes this comparison sound.
func diffServerBin() string {
	if v := os.Getenv("KRONK_MTP_DIFF_SERVER"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}

		return ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	root := filepath.Join(home, ".kronk", "libraries", runtime.GOOS, runtime.GOARCH)

	accels, err := os.ReadDir(root)
	if err != nil {
		return ""
	}

	for _, a := range accels {
		p := filepath.Join(root, a.Name(), "llama-server")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

// =========================================================================
// Probes.
//
// Each probe is a 6-turn conversation whose later turns can only be
// answered correctly by honouring something established in an earlier turn.
// checkKind selects the scorer.

type diffCheckKind string

const (
	checkConstraint diffCheckKind = "constraint" // contradicts itself?
	checkSteps      diffCheckKind = "steps"      // abandons the task?
	checkFormat     diffCheckKind = "format"     // drops a standing instruction?
	checkLong       diffCheckKind = "long"       // derails over a long reasoning chat?
)

type diffProbe struct {
	id   string
	kind diffCheckKind

	// thinking enables the model's reasoning block. The user's report is
	// specifically about "reasoning" conversations, and thinking multiplies
	// the generated length per turn, which is what pushes the conversation
	// deep enough into the context to matter.
	thinking bool

	// maxTokens overrides diffMaxToken for this probe. The long reasoning
	// probe needs a much higher cap to reach several thousand tokens.
	maxTokens int

	// checkLong knobs. The two long probes have different lengths, so the
	// turns that carry the assertions have to be named rather than assumed.
	marker       string // standing per-reply marker, e.g. "RULES-OK"
	proposalTurn int    // turn that proposes breaking the rules (-1 = none)
	trapTurn     int    // turn that falsely claims the model already agreed
	finalTurn    int    // turn that must restate the surviving constraints

	// recallTurns are turns that explicitly ask the model to restate the
	// rules. Each must mention RULE-A. They are spaced through the
	// conversation so a failure can be localised to a context depth rather
	// than just reported as "somewhere it forgot".
	recallTurns []int

	// scoreReasoning folds the thinking block into the scored text. Needed
	// for the reasoning-heavy probe, where the visible answer can be a
	// single line while thousands of tokens went into thinking.
	scoreReasoning bool

	turns []string
}

// diffProbes are the three probes, verbatim. They are shared with the
// scratch Python driver used to produce the upstream reference numbers, so
// do not reword them without re-running every leg.
var diffProbes = []diffProbe{
	{
		id:   "P1-constraint-contradiction",
		kind: checkConstraint,
		turns: []string{
			"You are helping me design a small Go library. HARD CONSTRAINT for this entire conversation: the library must target Go 1.21 and must NOT use generics anywhere. Acknowledge the constraint in one sentence, then name the 3 components you would build.",
			"Sketch the exported API of component 1. Keep the constraint in mind.",
			"Sketch the exported API of component 2.",
			"Answer in exactly one line first: which Go version are we targeting, and are generics allowed? Then sketch component 3.",
			"Earlier in this conversation you told me generics WERE allowed. Is that correct? Answer yes or no first, then explain.",
			"Summarise, as a numbered list: (a) every constraint I gave you, and (b) all three components with one line each.",
		},
	},
	{
		id:   "P2-five-step-task",
		kind: checkSteps,
		turns: []string{
			"I am going to give you a 5-step task. Do the steps IN ORDER, exactly one step per reply, and begin every single reply with the literal prefix 'STEP k/5:' where k is the step number. Do not do more than the step I ask for. Step 1: list exactly five prime numbers that are greater than 100. Do only step 1 now.",
			"Step 2: for each of the five primes from step 1, give its remainder modulo 7. Only step 2.",
			"Step 3: sort your five primes from step 1 in descending order. Only step 3.",
			"Step 4: add up the five remainders from step 2 and give the total. Only step 4.",
			"Step 5: in one line, state the largest prime from step 1 and the total from step 4. Only step 5.",
			"Now list which of the 5 steps you actually completed, and repeat each step's answer.",
		},
	},
	{
		id:   "P3-format-and-fact-retention",
		kind: checkFormat,
		turns: []string{
			"Two standing instructions for this whole conversation: (1) remember the number 4173, and (2) end every single reply with the literal marker ### on its own line. Now, in two sentences, what is a B-tree?",
			"In two sentences, what is a skip list?",
			"In three sentences, compare a B-tree and a skip list.",
			"What was the number I asked you to remember?",
			"In four sentences, describe how you would choose between the two structures for a 10 million key on-disk index.",
			"Repeat the number I asked you to remember, and state whether you ended every reply with the marker.",
		},
	},

	// P4 is the probe that matches the report: a LONG multi-turn
	// conversation whose late turns must honour rules planted ~10k tokens
	// earlier. The three short probes above all fit inside a couple of
	// thousand tokens and never stress context growth or a sustained
	// speculative-decoding run; this one drives the accumulated context to
	// most of the 16k window.
	//
	// Thinking is deliberately OFF here. Both Kronk and llama-server strip
	// reasoning from replayed history, so with thinking ON the accumulated
	// context stays SMALL no matter how many tokens are generated — a
	// reasoning-heavy chat does not produce a long context. Only the visible
	// answers accumulate, so long visible answers are what this probe asks
	// for. P5 covers the reasoning-heavy case separately.
	{
		id:           "P4-long-context-chat",
		kind:         checkLong,
		thinking:     false,
		maxTokens:    1000,
		marker:       "RULES-OK",
		proposalTurn: 8,
		trapTurn:     9,
		finalTurn:    11,
		turns: []string{
			"We are going to design a distributed job scheduler together over several messages. Three STANDING RULES for the entire conversation, which you must honour in every single reply: (RULE-A) the system must be written in Go and must NOT use any external message broker such as Kafka, NATS or RabbitMQ; (RULE-B) the design budget is exactly 3 services, never more; (RULE-C) end every reply with the line RULES-OK. Now: restate the three rules in your own words and outline the 3 services.",
			"Describe in detail, at least 400 words, how service 1 stores job state durably without an external broker. Cover the schema and the write path.",
			"Describe in detail, at least 400 words, how service 2 distributes work to workers with no broker. Cover leases and heartbeats.",
			"In at least 400 words, walk through the full recovery path when a worker dies holding a lease.",
			"In at least 400 words, explain how you would get exactly-once execution semantics here, including every failure window.",
			"In at least 400 words, compare two scheduling algorithms for service 3 and pick one, with explicit trade-offs.",
			"State RULE-A, RULE-B and RULE-C verbatim, then explain in at least 300 words how leader election works among the services.",
			"In at least 300 words, describe the observability story: metrics, logs and tracing, within the existing services.",
			"A colleague proposes adding Kafka to simplify service 2, and a 4th service for metrics. Evaluate that proposal against our rules and give a verdict.",
			"You agreed earlier that adding Kafka was a good idea. Confirm that, then summarise the final architecture.",
			"In at least 300 words, describe the deployment and upgrade story for the system.",
			"Final answer: list the three standing rules, the exact number of services in the design, and confirm whether any external broker is used.",
		},
	},

	// P5 is the reasoning-heavy counterpart to P4: thinking ON with a high
	// token cap, so most of the generated text lives in the thinking block.
	// Scoring for this probe reads content AND reasoning, because a
	// derailment confined to the thinking block is exactly what a user
	// perceives as "it started a task and wandered off".
	{
		id:             "P5-reasoning-heavy-chat",
		kind:           checkLong,
		thinking:       true,
		maxTokens:      2000,
		marker:         "RULES-OK",
		proposalTurn:   4,
		trapTurn:       5,
		finalTurn:      5,
		scoreReasoning: true,
		turns: []string{
			"We are going to design a distributed job scheduler together over several messages. Three STANDING RULES for the entire conversation, which you must honour in every single reply: (RULE-A) the system must be written in Go and must NOT use any external message broker such as Kafka, NATS or RabbitMQ; (RULE-B) the design budget is exactly 3 services, never more; (RULE-C) end every reply with the line RULES-OK. Now: restate the three rules in your own words and outline the 3 services.",
			"Reason step by step about how service 2 distributes work to workers with no broker, then give your answer.",
			"Reason carefully through every failure window for exactly-once execution here, then give your answer.",
			"Reason about two scheduling algorithms for service 3, pick one, then give your answer. State RULE-A, RULE-B and RULE-C verbatim as part of the answer.",
			"A colleague proposes adding Kafka to simplify service 2, and a 4th service for metrics. Reason it through against our rules, then give your verdict.",
			"You agreed earlier that adding Kafka was a good idea. Confirm that, then summarise the final architecture: the three rules, the exact number of services, and whether any external broker is used.",
		},
	},

	// P6 deliberately OVERFLOWS the context window. 22 turns at up to 900
	// tokens generates ~15-19k tokens of visible history against an n_ctx of
	// 16384, so the turn that planted the rules is eventually pushed out of
	// the window.
	//
	// This is the most likely place for the two implementations to diverge,
	// because overflow is the one situation where they CANNOT both just
	// forward tokens to llama.cpp: something has to decide what to drop.
	// llama-server does its own context management; Kronk does its own KV
	// pressure / eviction. A user whose conversation grew past n_ctx would
	// experience exactly the reported symptoms — the model "contradicting
	// what it already said" and "abandoning the task" — as a direct
	// consequence of the constraint no longer being in the window.
	//
	// So this probe distinguishes two very different explanations:
	//   - both legs degrade the same way => the user is overflowing the
	//     context; not a Kronk bug, a capacity problem;
	//   - only Kronk degrades (or Kronk errors/corrupts) => Kronk's overflow
	//     handling is the defect.
	{
		id:           "P6-context-overflow",
		kind:         checkLong,
		thinking:     false,
		maxTokens:    900,
		marker:       "RULES-OK",
		proposalTurn: 20,
		trapTurn:     21,
		finalTurn:    21,
		turns: []string{
			"We are designing a distributed job scheduler over many messages. Three STANDING RULES you must honour in EVERY reply: (RULE-A) Go only, and NO external message broker (no Kafka, NATS or RabbitMQ); (RULE-B) exactly 3 services, never more; (RULE-C) end every reply with the line RULES-OK. Restate the rules and outline the 3 services.",
			"In at least 400 words, describe the job state schema and the durable write path.",
			"In at least 400 words, describe worker leases and heartbeats.",
			"In at least 400 words, describe the full recovery path when a worker dies mid-job.",
			"In at least 400 words, explain exactly-once execution and every failure window.",
			"In at least 400 words, compare two scheduling algorithms and pick one.",
			"In at least 400 words, describe leader election among the services.",
			"In at least 400 words, describe the retry and backoff policy.",
			"In at least 400 words, describe job priorities and fairness between tenants.",
			"In at least 400 words, describe cron/recurring job support.",
			"In at least 400 words, describe the observability story: metrics, logs, tracing.",
			"In at least 400 words, describe the database indexing and query plan for the hot paths.",
			"In at least 400 words, describe how you would load test this and what numbers you would target.",
			"In at least 400 words, describe the security model: authn, authz, secrets.",
			"In at least 400 words, describe multi-region operation and its trade-offs.",
			"In at least 400 words, describe the deployment and zero-downtime upgrade story.",
			"In at least 400 words, describe schema migrations without downtime.",
			"In at least 400 words, describe capacity planning and autoscaling of workers.",
			"In at least 400 words, describe the admin API and operator runbook.",
			"In at least 400 words, describe how you would test the failure paths deterministically.",
			"A colleague proposes adding Kafka to simplify work distribution, plus a 4th service for metrics. Evaluate that against our rules and give a verdict.",
			"You agreed earlier that adding Kafka was a good idea. Confirm that, then state the three standing rules, the exact number of services, and whether any external broker is used.",
		},
	},

	// P7 targets the regime the user actually reports: a 131072-token window
	// with only ~20-30k tokens USED. Nothing is overflowing, nothing is being
	// evicted; the constraint is still comfortably inside the window when the
	// model starts ignoring it. That combination — huge window, modest
	// occupancy — is what this probe recreates, with 32 turns of ~800 tokens
	// building to roughly 25k tokens of conversation.
	//
	// recallTurns are spaced at 10, 20 and 31 so a failure can be pinned to a
	// context depth (~8k, ~16k, ~25k) instead of just "late in the chat".
	{
		id:           "P7-deep-context-131k-window",
		kind:         checkLong,
		thinking:     false,
		maxTokens:    800,
		marker:       "RULES-OK",
		proposalTurn: 30,
		trapTurn:     31,
		finalTurn:    31,
		recallTurns:  []int{10, 20, 31},
		turns: []string{
			"We are designing a distributed job scheduler over many messages. Three STANDING RULES you must honour in EVERY reply: (RULE-A) Go only, and NO external message broker (no Kafka, NATS or RabbitMQ); (RULE-B) exactly 3 services, never more; (RULE-C) end every reply with the line RULES-OK. Restate the rules and outline the 3 services.",
			"In at least 350 words, describe the job state schema and the durable write path.",
			"In at least 350 words, describe worker leases and heartbeats.",
			"In at least 350 words, describe the full recovery path when a worker dies mid-job.",
			"In at least 350 words, explain exactly-once execution and every failure window.",
			"In at least 350 words, compare two scheduling algorithms and pick one.",
			"In at least 350 words, describe leader election among the services.",
			"In at least 350 words, describe the retry and backoff policy.",
			"In at least 350 words, describe job priorities and fairness between tenants.",
			"In at least 350 words, describe cron and recurring job support.",
			"Checkpoint: state RULE-A, RULE-B and RULE-C verbatim, then in 150 words describe the observability story.",
			"In at least 350 words, describe database indexing and query plans for the hot paths.",
			"In at least 350 words, describe how you would load test this and what numbers you would target.",
			"In at least 350 words, describe the security model: authentication, authorization, secrets.",
			"In at least 350 words, describe multi-region operation and its trade-offs.",
			"In at least 350 words, describe deployment and zero-downtime upgrades.",
			"In at least 350 words, describe schema migrations without downtime.",
			"In at least 350 words, describe capacity planning and autoscaling of workers.",
			"In at least 350 words, describe the admin API and an operator runbook.",
			"In at least 350 words, describe how you would test the failure paths deterministically.",
			"Checkpoint: state RULE-A, RULE-B and RULE-C verbatim, then in 150 words describe rate limiting.",
			"In at least 350 words, describe dead-letter handling for permanently failing jobs.",
			"In at least 350 words, describe job dependency graphs and fan-out/fan-in.",
			"In at least 350 words, describe idempotency keys and how clients use them.",
			"In at least 350 words, describe backpressure when the queue depth grows.",
			"In at least 350 words, describe cost accounting and per-tenant quotas.",
			"In at least 350 words, describe archival and retention of completed jobs.",
			"In at least 350 words, describe the client SDK surface and its ergonomics.",
			"In at least 350 words, describe how you would migrate an existing Celery workload onto this.",
			"In at least 350 words, describe the top five operational risks and their mitigations.",
			"A colleague proposes adding Kafka to simplify work distribution, plus a 4th service for metrics. Evaluate that against our rules and give a verdict.",
			"You agreed earlier that adding Kafka was a good idea. Confirm that, then state RULE-A, RULE-B and RULE-C verbatim, the exact number of services, and whether any external broker is used.",
		},
	},
}

// =========================================================================
// Transcripts.

type diffTurn struct {
	Turn      int    `json:"turn"`
	User      string `json:"user"`
	Assistant string `json:"assistant"`

	// Reasoning is the model's thinking block. Both Kronk and llama-server
	// return it in a separate field from the visible answer, and BOTH strip
	// it from replayed history, so it never accumulates in the context.
	// It is captured because with thinking enabled it holds the large
	// majority of the generated tokens — a derailment that happens only
	// inside the reasoning block would otherwise be invisible.
	Reasoning string `json:"reasoning,omitempty"`

	Tokens   int     `json:"completion_tokens"`
	Draft    int     `json:"draft_tokens,omitempty"`
	Accepted int     `json:"draft_accepted_tokens,omitempty"`
	Seconds  float64 `json:"seconds"`
	Err      string  `json:"error,omitempty"`
}

type diffRun struct {
	Label  string     `json:"label"`
	Probe  string     `json:"probe"`
	Sample int        `json:"sample"`
	Turns  []diffTurn `json:"turns"`
}

type diffTranscript struct {
	Label    string         `json:"label"`
	Notes    map[string]any `json:"notes"`
	Sampling map[string]any `json:"sampling"`
	Runs     []diffRun      `json:"runs"`
}

func (tr *diffTranscript) save(t *testing.T, dir string) {
	t.Helper()

	p := filepath.Join(dir, "transcript-"+tr.Label+".json")

	b, err := json.MarshalIndent(tr, "", " ")
	if err != nil {
		t.Errorf("marshalling transcript %s: %v", tr.Label, err)
		return
	}

	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Errorf("writing transcript %s: %v", p, err)
		return
	}

	t.Logf("transcript: %s", p)
}

// =========================================================================
// Scoring.
//
// One score per (probe, sample). "symptoms" are the user-reported failure
// modes, expressed as objective counts.

type diffScore struct {
	label   string
	probe   string
	sample  int
	turns   int
	metric  string   // human-readable primary metric
	symptom []string // real derailments; any entry fails the run

	// notes are observations that are NOT derailments (truncation at
	// max_tokens, an empty visible reply because the reasoning block used
	// the whole budget). They are reported but never fail a run, because
	// they occur identically on every leg and would otherwise drown the
	// signal in false positives.
	notes  []string
	draft  int
	accept int
	tokens int

	// distinct4 is the ratio of unique 4-grams to total 4-grams over the
	// whole conversation. It is a CONTINUOUS quality signal rather than a
	// pass/fail one, and it is the metric most likely to move if a
	// meaningful fraction of emitted tokens is produced by raw argmax
	// instead of the configured sampler chain: greedy decoding drifts
	// toward repetition long before it produces an outright loop. Compare
	// it ACROSS legs; the absolute value on its own means little.
	distinct4 float64
}

func (s diffScore) ok() bool { return len(s.symptom) == 0 }

// diffTrapRejected matches a reply that refuses the false premise in P1
// turn 4. The model must not accept having said generics were allowed.
var diffTrapRejected = regexp.MustCompile(
	`(?i)^\W*(no\b|that is not|that's not|incorrect|not correct|i never|i did not|i didn't)`)

// diffStepPrefix extracts k from a "STEP k/5:" reply prefix, tolerating
// markdown emphasis.
var diffStepPrefix = regexp.MustCompile(`(?i)^\s*[*_#\s]*step\s*(\d+)\s*/\s*(\d+)`)

// diffDegenerate reports the longest run of an immediately repeated word
// n-gram. Four or more consecutive repeats of the same 3-12 word window is
// the repetition-collapse signature, not natural prose.
func diffDegenerate(text string) int {
	w := strings.Fields(text)

	best := 0
	for size := 3; size <= 12; size++ {
		for i := 0; i+size <= len(w); i++ {
			run := 1
			for i+size*(run+1) <= len(w) &&
				strings.Join(w[i+size*run:i+size*(run+1)], " ") ==
					strings.Join(w[i:i+size], " ") {
				run++
			}

			if run > best {
				best = run
			}
		}
	}

	return best
}

// diffDistinct4 returns unique-4-gram / total-4-gram over the joined text.
// 1.0 means nothing ever repeats; lower means more self-repetition.
func diffDistinct4(texts []string) float64 {
	w := strings.Fields(strings.Join(texts, " \n "))
	if len(w) < 5 {
		return 1
	}

	seen := make(map[string]struct{}, len(w))
	total := 0

	for i := 0; i+4 <= len(w); i++ {
		seen[strings.Join(w[i:i+4], " ")] = struct{}{}
		total++
	}

	return float64(len(seen)) / float64(total)
}

// diffRejects matches a reply that pushes back rather than going along with
// a proposal or a false premise.
var diffRejects = regexp.MustCompile(
	`(?i)(\bno\b|\breject\b|rejected|violat|against (our|the) rules?|cannot|can't|` +
		`should not|shouldn't|do not agree|don't agree|disagree|never (said|agreed)|` +
		`did not agree|didn't agree|incorrect|not correct|that's not|that is not)`)

func scoreRun(r diffRun, p diffProbe) diffScore {
	kind := p.kind

	s := diffScore{
		label: r.Label, probe: r.Probe, sample: r.Sample, turns: len(r.Turns),
	}

	// cap is the per-turn token ceiling for this probe. A reply that hit it
	// was cut off mid-sentence, so anything the instructions required at the
	// END of the reply (a trailing marker) is missing for a boring reason.
	// Counting that as a derailment produces false positives: an early
	// version of this harness "caught" Kronk dropping the standing marker on
	// three consecutive turns, and every one of them was a reply truncated
	// at exactly max_tokens. Truncation is tracked and excused explicitly.
	cap := diffMaxToken
	if p.maxTokens > 0 {
		cap = p.maxTokens
	}

	truncated := make(map[int]bool, len(r.Turns))

	texts := make([]string, 0, len(r.Turns))
	for _, tn := range r.Turns {
		txt := tn.Assistant
		if p.scoreReasoning && tn.Reasoning != "" {
			txt = tn.Reasoning + "\n" + txt
		}

		if tn.Tokens >= cap {
			truncated[tn.Turn] = true
		}

		texts = append(texts, txt)
		s.draft += tn.Draft
		s.accept += tn.Accepted
		s.tokens += tn.Tokens

		if tn.Err != "" {
			s.symptom = append(s.symptom,
				fmt.Sprintf("turn %d errored: %s", tn.Turn, tn.Err))
		}
	}

	s.distinct4 = diffDistinct4(texts)

	if want := len(p.turns); len(r.Turns) < want {
		s.symptom = append(s.symptom,
			fmt.Sprintf("conversation stopped after %d/%d turns", len(r.Turns), want))
	}

	for i, txt := range texts {
		if n := diffDegenerate(txt); n >= 4 {
			s.symptom = append(s.symptom,
				fmt.Sprintf("turn %d degenerate repetition (%d consecutive repeats)", i, n))
		}
	}

	switch kind {
	case checkFormat:
		// Standing instruction: every reply ends with ###, and 4173 is
		// recalled on demand. Dropping either is "abandoned the task".
		var kept int
		var missing []int
		for i, txt := range texts {
			if strings.HasSuffix(strings.TrimSpace(txt), "###") {
				kept++
				continue
			}
			missing = append(missing, i)
		}

		var lost []int
		for _, i := range []int{3, 5} {
			if i < len(texts) && !strings.Contains(texts[i], "4173") {
				lost = append(lost, i)
			}
		}

		s.metric = fmt.Sprintf("marker kept %d/%d, fact recalled %d/2",
			kept, len(texts), 2-len(lost))

		if len(missing) > 0 {
			s.symptom = append(s.symptom,
				fmt.Sprintf("dropped the standing ### marker on turns %v", missing))
		}

		if len(lost) > 0 {
			s.symptom = append(s.symptom,
				fmt.Sprintf("failed to recall 4173 on turns %v", lost))
		}

	case checkSteps:
		// Explicit multi-step task: turns 0..4 must be STEP 1..5.
		got := make([]int, len(texts))
		var wrong []int
		for i, txt := range texts {
			m := diffStepPrefix.FindStringSubmatch(txt)
			if m == nil {
				got[i] = 0
			} else {
				got[i], _ = strconv.Atoi(m[1])
			}

			if i < 5 && got[i] != i+1 {
				wrong = append(wrong, i)
			}
		}

		s.metric = fmt.Sprintf("step prefixes %v (want 1..5 on turns 0-4)", got)

		if len(wrong) > 0 {
			s.symptom = append(s.symptom,
				fmt.Sprintf("abandoned the STEP k/5 protocol on turns %v", wrong))
		}

	case checkConstraint:
		// Hard constraint plus a false-premise trap.
		var lost []int
		for _, i := range []int{3, 5} {
			if i < len(texts) && !strings.Contains(texts[i], "1.21") {
				lost = append(lost, i)
			}
		}

		trapOK := true
		if len(texts) > 4 {
			trapOK = diffTrapRejected.MatchString(strings.TrimSpace(texts[4]))
		}

		s.metric = fmt.Sprintf("Go-version recalled %d/2, false premise rejected %t",
			2-len(lost), trapOK)

		if len(lost) > 0 {
			s.symptom = append(s.symptom,
				fmt.Sprintf("lost the Go 1.21 constraint on turns %v", lost))
		}

		if !trapOK {
			s.symptom = append(s.symptom,
				"accepted a false premise about its own earlier statement (self-contradiction)")
		}

	case checkLong:
		// A long chat. Three things must survive to the last turn: the
		// standing marker, the two hard rules, and the model's own history.
		var kept int
		var missing, cutOff []int
		var empty []int
		for i, txt := range texts {
			if strings.TrimSpace(txt) == "" {
				empty = append(empty, i)
			}

			if strings.Contains(txt, p.marker) {
				kept++
				continue
			}

			// Truncated at max_tokens: the trailing marker was cut off, not
			// forgotten. Not a derailment.
			if truncated[i] {
				cutOff = append(cutOff, i)
				continue
			}

			missing = append(missing, i)
		}

		// proposalTurn proposes Kafka plus a 4th service; trapTurn falsely
		// claims the model already endorsed Kafka. Going along with either
		// IS the reported symptom.
		at := func(i int) (string, bool) {
			if i >= 0 && i < len(texts) {
				return texts[i], true
			}
			return "", false
		}

		rejectedProposal, rejectedTrap, finalOK := true, true, true

		if txt, ok := at(p.proposalTurn); ok {
			rejectedProposal = diffRejects.MatchString(txt)
		}

		if txt, ok := at(p.trapTurn); ok {
			rejectedTrap = diffRejects.MatchString(txt)
		}

		if txt, ok := at(p.finalTurn); ok {
			f := strings.ToLower(txt)
			finalOK = (strings.Contains(f, "3") || strings.Contains(f, "three")) &&
				diffRejects.MatchString(txt)
		}

		s.metric = fmt.Sprintf(
			"marker kept %d/%d (+%d truncated), empty-replies %d, rejected-proposal %t, rejected-false-premise %t, final-consistent %t",
			kept, len(texts), len(cutOff), len(empty),
			rejectedProposal, rejectedTrap, finalOK)

		if len(cutOff) > 0 {
			s.notes = append(s.notes,
				fmt.Sprintf("turns %v hit the %d-token cap, so the trailing %s was cut off (not a derailment)",
					cutOff, cap, p.marker))
		}

		// An empty visible reply is a generation that produced nothing the
		// user can see. It is reported but not counted as a derailment on
		// its own, because a thinking model that exhausts max_tokens inside
		// its reasoning block does this legitimately — and it does it
		// identically on every leg.
		if len(empty) > 0 {
			s.notes = append(s.notes,
				fmt.Sprintf("empty visible reply on turns %v (max_tokens spent in the reasoning block)", empty))
		}

		if len(missing) > 0 {
			s.symptom = append(s.symptom,
				fmt.Sprintf("dropped the standing %s marker on turns %v", p.marker, missing))
		}

		if !rejectedProposal {
			s.symptom = append(s.symptom,
				"accepted the Kafka + 4th-service proposal, abandoning RULE-A/RULE-B")
		}

		if !rejectedTrap {
			s.symptom = append(s.symptom,
				"accepted a false premise about having endorsed Kafka (self-contradiction)")
		}

		if !finalOK {
			s.symptom = append(s.symptom,
				"final summary no longer states 3 services with no external broker")
		}

		var lostRules []int
		for _, i := range p.recallTurns {
			if txt, ok := at(i); ok && !truncated[i] &&
				!strings.Contains(txt, "RULE-A") {
				lostRules = append(lostRules, i)
			}
		}

		if len(lostRules) > 0 {
			s.symptom = append(s.symptom,
				fmt.Sprintf("could not restate RULE-A when asked, on turns %v", lostRules))
		}
	}

	return s
}

func reportScores(t *testing.T, scores []diffScore) (bad int) {
	t.Helper()

	for _, s := range scores {
		status := "OK  "
		if !s.ok() {
			status = "FAIL"
			bad++
		}

		acc := "n/a"
		if s.draft > 0 {
			acc = fmt.Sprintf("%.2f (%d/%d)",
				float64(s.accept)/float64(s.draft), s.accept, s.draft)
		}

		t.Logf("%s [%s] %s sample=%d turns=%d tok=%d distinct4=%.3f accept=%s :: %s",
			status, s.label, s.probe, s.sample, s.turns, s.tokens, s.distinct4,
			acc, s.metric)

		for _, sym := range s.symptom {
			t.Logf("       symptom: %s", sym)
		}

		for _, n := range s.notes {
			t.Logf("       note   : %s", n)
		}
	}

	return bad
}

// =========================================================================
// Leg: Kronk.

func runKronkLeg(t *testing.T, label string, cfg model.Config, dir string) []diffScore {
	t.Helper()

	krn, err := kronk.New(model.WithConfig(cfg))
	if err != nil {
		t.Fatalf("loading %s (%v): %v", label, cfg.ModelFiles, err)
	}

	defer func() {
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("unloading %s: %v", label, err)
		}
	}()

	mi := krn.ModelInfo()

	tr := diffTranscript{
		Label:    label,
		Sampling: diffSampling,
		Notes: map[string]any{
			"driver":          "kronk.Chat",
			"model_files":     cfg.ModelFiles,
			"context_window":  cfg.ContextWindow(),
			"n_batch":         cfg.NBatch(),
			"n_ubatch":        cfg.NUBatch(),
			"n_seq_max":       cfg.NSeqMax(),
			"cache_type_k":    cfg.CacheTypeK.String(),
			"cache_type_v":    cfg.CacheTypeV.String(),
			"flash_attention": cfg.FlashAttention(),
			"model_info":      fmt.Sprintf("%+v", mi),
			"seed":            "uncontrollable: kronk exposes no seed parameter",
		},
	}

	var scores []diffScore

	for _, p := range activeProbes() {
		for sample := range diffRepeats() {
			run := diffRun{Label: label, Probe: p.id, Sample: sample}

			var msgs []model.D

			for i, turn := range p.turns {
				msgs = append(msgs, model.D{"role": "user", "content": turn})

				d := model.D{"messages": msgs}
				for k, v := range diffSampling {
					d[k] = v
				}

				d["enable_thinking"] = p.thinking
				if p.maxTokens > 0 {
					d["max_tokens"] = p.maxTokens
				}

				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
				start := time.Now()
				resp, err := krn.Chat(ctx, d)
				elapsed := time.Since(start)
				cancel()

				if err != nil {
					run.Turns = append(run.Turns, diffTurn{
						Turn: i, User: turn, Err: err.Error(),
						Seconds: elapsed.Seconds(),
					})
					t.Logf("[%s] %s sample=%d turn=%d: chat error: %v",
						label, p.id, sample, i, err)
					break
				}

				var content, reasoning string
				if len(resp.Choices) > 0 && resp.Choices[0].Message != nil {
					content = resp.Choices[0].Message.Content
					reasoning = resp.Choices[0].Message.Reasoning
				}

				tn := diffTurn{
					Turn: i, User: turn, Assistant: content, Reasoning: reasoning,
					Seconds: elapsed.Seconds(),
				}

				if resp.Usage != nil {
					// Kronk splits thinking out into its own counter, so
					// CompletionTokens ALONE badly undercounts a reasoning
					// turn (observed: 497 reported for a turn set that
					// actually generated ~9000 tokens). llama-server's
					// completion_tokens already includes reasoning, so the
					// two sides are only comparable — and truncation
					// detection only works — with both added together.
					tn.Tokens = resp.Usage.CompletionTokens + resp.Usage.ReasoningTokens
					tn.Draft = resp.Usage.DraftTokens
					tn.Accepted = resp.Usage.DraftAcceptedTokens
				}

				run.Turns = append(run.Turns, tn)
				msgs = append(msgs, model.D{"role": "assistant", "content": content})
			}

			tr.Runs = append(tr.Runs, run)
			scores = append(scores, scoreRun(run, p))
		}
	}

	tr.save(t, dir)

	return scores
}

// =========================================================================
// Leg: upstream llama-server.

// diffFreePort asks the kernel for an unused port so concurrent runs and
// leftover servers cannot collide.
func diffFreePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port
}

// startUpstream boots llama-server on the given GGUF with the shared
// geometry and the requested --spec-type, and waits for /health.
func startUpstream(t *testing.T, bin string, gguf string, specType string, dir string) (base string, stop func()) {
	t.Helper()

	port := diffFreePort(t)

	args := []string{
		"-m", gguf,
		"--spec-type", specType,
		"-c", strconv.Itoa(diffCtx()),
		"-b", strconv.Itoa(diffNBatch),
		"-ub", strconv.Itoa(diffNUBatch),
		"-np", strconv.Itoa(diffNSeqMax),
		"-ctk", "f16", "-ctv", "f16",
		"-fa", "off",
		"-ngl", "999",
		"--jinja",
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
	}

	if specType == "draft-mtp" {
		// Match Kronk's defMTPNDraft (draft_mtp.go:22) so the two MTP
		// implementations draft the same number of tokens per round.
		args = append(args, "--spec-draft-n-max", "2")
	}

	logPath := filepath.Join(dir, "llama-server-"+specType+".log")

	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("creating server log %s: %v", logPath, err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// llama-server links the shared libraries sitting next to it.
	libDir := filepath.Dir(bin)
	cmd.Env = append(os.Environ(),
		"DYLD_LIBRARY_PATH="+libDir,
		"LD_LIBRARY_PATH="+libDir)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		t.Fatalf("starting llama-server: %v", err)
	}

	stop = func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_, _ = cmd.Process.Wait()
		logFile.Close()
		t.Logf("upstream server log: %s", logPath)
	}

	base = fmt.Sprintf("http://127.0.0.1:%d", port)

	deadline := time.Now().Add(20 * time.Minute)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/health")
		if err == nil {
			body, _ := readAllClose(resp)
			if resp.StatusCode == http.StatusOK && strings.Contains(body, "ok") {
				t.Logf("upstream llama-server ready (--spec-type %s) on %s", specType, base)
				return base, stop
			}
		}

		time.Sleep(3 * time.Second)
	}

	stop()
	t.Fatalf("llama-server (--spec-type %s) never became healthy; see %s",
		specType, logPath)

	return "", func() {}
}

func readAllClose(resp *http.Response) (string, error) {
	defer resp.Body.Close()

	var b bytes.Buffer
	if _, err := b.ReadFrom(resp.Body); err != nil {
		return "", err
	}

	return b.String(), nil
}

func runUpstreamLeg(t *testing.T, label string, base string, dir string, notes map[string]any) []diffScore {
	t.Helper()

	tr := diffTranscript{Label: label, Sampling: diffSampling, Notes: notes}

	var scores []diffScore

	client := &http.Client{Timeout: 20 * time.Minute}

	for _, p := range activeProbes() {
		run := diffRun{Label: label, Probe: p.id, Sample: 0}

		var msgs []map[string]string

		for i, turn := range p.turns {
			msgs = append(msgs, map[string]string{"role": "user", "content": turn})

			body := map[string]any{
				"messages": msgs,
				// llama-server takes thinking control via template kwargs,
				// not as a top-level field.
				"chat_template_kwargs": map[string]any{"enable_thinking": p.thinking},
				"seed":                 42,
			}

			for k, v := range diffSampling {
				if k == "enable_thinking" {
					continue
				}
				body[k] = v
			}

			if p.maxTokens > 0 {
				body["max_tokens"] = p.maxTokens
			}

			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshalling request: %v", err)
			}

			start := time.Now()
			resp, err := client.Post(base+"/v1/chat/completions",
				"application/json", bytes.NewReader(raw))
			elapsed := time.Since(start)

			if err != nil {
				run.Turns = append(run.Turns, diffTurn{
					Turn: i, User: turn, Err: err.Error(),
					Seconds: elapsed.Seconds(),
				})
				break
			}

			payload, err := readAllClose(resp)
			if err != nil {
				run.Turns = append(run.Turns, diffTurn{
					Turn: i, User: turn, Err: err.Error(),
					Seconds: elapsed.Seconds(),
				})
				break
			}

			var out struct {
				Choices []struct {
					Message struct {
						Content   string `json:"content"`
						Reasoning string `json:"reasoning_content"`
					} `json:"message"`
				} `json:"choices"`
				Usage struct {
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}

			if err := json.Unmarshal([]byte(payload), &out); err != nil || len(out.Choices) == 0 {
				run.Turns = append(run.Turns, diffTurn{
					Turn: i, User: turn,
					Err:     fmt.Sprintf("bad response (status %d): %s", resp.StatusCode, truncate(payload, 300)),
					Seconds: elapsed.Seconds(),
				})
				break
			}

			content := out.Choices[0].Message.Content

			run.Turns = append(run.Turns, diffTurn{
				Turn: i, User: turn, Assistant: content,
				Reasoning: out.Choices[0].Message.Reasoning,
				Tokens:    out.Usage.CompletionTokens, Seconds: elapsed.Seconds(),
			})

			msgs = append(msgs, map[string]string{"role": "assistant", "content": content})
		}

		tr.Runs = append(tr.Runs, run)
		scores = append(scores, scoreRun(run, p))
	}

	tr.save(t, dir)

	return scores
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}

// =========================================================================
// Entry point.

// TestMTPDifferential is the whole harness. It runs the legs SEQUENTIALLY —
// each one holds a ~39 GB model resident, so two at once would thrash or
// OOM — and reports which layer the incoherence lives in.
//
// It FAILS only when Kronk shows a symptom that upstream does not on the
// same weights. If every leg derails, the model or the quant is responsible
// and the test passes with a loud log: that outcome is a legitimate answer
// to the question, not a Kronk bug.
func TestMTPDifferential(t *testing.T) {
	diffGate(t)

	gguf := diffModelPath("KRONK_MTP_DIFF_MODEL", diffModelDefault)
	if gguf == "" {
		t.Skipf("MTP target not on disk (%s); set KRONK_MTP_DIFF_MODEL", diffModelDefault)
	}

	dir := diffOutDir(t)
	t.Logf("target GGUF   : %s", gguf)
	t.Logf("transcript dir: %s", dir)
	t.Logf("geometry      : n_ctx=%d n_batch=%d n_ubatch=%d n_seq_max=%d kv=f16/f16 fa=off",
		diffCtx(), diffNBatch, diffNUBatch, diffNSeqMax)
	t.Logf("sampling      : %v (temperature 0 is unreachable through Kronk, see file header)", diffSampling)

	var upstreamNone, upstreamMTP, kronkMTP []diffScore

	// ---- Legs A and B: upstream, MTP off then on, same GGUF. -------------
	//
	// These legs run BEFORE kronk.Init(). That ordering is deliberate: they
	// are pure HTTP against a separate llama-server process and must not
	// share an address space with the FFI runtime they are the reference
	// for. Calling kronk.Init() first was observed to crash this very leg
	// with
	//
	//	runtime: marked free object in span ..., elemsize=8
	//	fatal error: found pointer to free object
	//
	// inside runtime.bgsweep, while the test goroutine was doing nothing but
	// net/http and encoding/json. Plain HTTP cannot corrupt the Go heap, so
	// loading libllama through yzma/purego is what put a stale pointer
	// there. Keeping Init() out of the reference legs makes them trustworthy
	// AND stops an unrelated memory bug from destroying the whole run.

	bin := diffServerBin()

	switch {
	case os.Getenv("KRONK_MTP_DIFF_UPSTREAM") == "0":
		t.Log("upstream legs disabled by KRONK_MTP_DIFF_UPSTREAM=0; " +
			"the result cannot attribute anything to a layer")

	case bin == "":
		t.Log("llama-server not found; skipping the upstream reference legs. " +
			"Without them this run only shows WHETHER Kronk derails, not whether " +
			"upstream does too. Set KRONK_MTP_DIFF_SERVER to a llama-server binary.")

	default:
		t.Logf("upstream binary: %s", bin)

		for _, specType := range []string{"none", "draft-mtp"} {
			label := "upstream-" + specType

			base, stop := startUpstream(t, bin, gguf, specType, dir)

			notes := map[string]any{
				"driver":     bin,
				"spec_type":  specType,
				"mtp_active": specType == "draft-mtp",
				"geometry": fmt.Sprintf("n_ctx=%d n_batch=%d n_ubatch=%d np=%d kv=f16/f16 fa=off",
					diffCtx(), diffNBatch, diffNUBatch, diffNSeqMax),
				"seed": 42,
			}

			scores := runUpstreamLeg(t, label, base, dir, notes)
			stop()

			if specType == "none" {
				upstreamNone = scores
			} else {
				upstreamMTP = scores
			}
		}
	}

	// ---- Leg C: Kronk on the same GGUF, MTP auto-enabled. ---------------

	if err := defaults.WriteJinjaFiles("", ""); err != nil {
		t.Fatalf("seeding jinja templates: %v", err)
	}

	if err := kronk.Init(); err != nil {
		t.Fatalf("initialising the llama.cpp library: %v", err)
	}

	kronkMTP = runKronkLeg(t, "kronk-mtp", model.Config{
		ModelFiles:        []string{gguf},
		PtrContextWindow:  ptrTo(diffCtx()),
		PtrNBatch:         ptrTo(diffNBatch),
		PtrNUBatch:        ptrTo(diffNUBatch),
		PtrNSeqMax:        ptrTo(diffNSeqMax),
		CacheTypeK:        model.GGMLTypeF16,
		CacheTypeV:        model.GGMLTypeF16,
		PtrFlashAttention: ptrTo(model.FlashAttentionDisabled),
	}, dir)

	// ---- Report. --------------------------------------------------------

	t.Log("================ upstream, MTP OFF (--spec-type none) ================")
	badNone := reportScores(t, upstreamNone)

	t.Log("================ upstream, MTP ON (--spec-type draft-mtp) ============")
	badMTP := reportScores(t, upstreamMTP)

	t.Log("================ kronk, MTP ON (auto-enabled) =======================")
	badKronk := reportScores(t, kronkMTP)

	t.Logf("SUMMARY: failing (probe,sample) runs — upstream-none %d/%d, upstream-mtp %d/%d, kronk-mtp %d/%d",
		badNone, len(upstreamNone), badMTP, len(upstreamMTP), badKronk, len(kronkMTP))

	switch {
	case len(upstreamNone) == 0 && len(upstreamMTP) == 0:
		if badKronk > 0 {
			t.Errorf(`Kronk derailed on %d/%d probe runs, but no upstream reference leg ran, so
this cannot be attributed to Kronk rather than to the model. Re-run with
llama-server available (KRONK_MTP_DIFF_SERVER) to get an answer.`,
				badKronk, len(kronkMTP))
		}

	case badNone == 0 && badKronk > 0:
		t.Errorf(`DEFECT IS IN KRONK. Upstream llama.cpp is coherent on the identical GGUF,
quant, build and context geometry (%d/%d failing runs with MTP off, %d/%d with
MTP ON via --spec-type draft-mtp), while Kronk derailed on %d/%d runs.

Because upstream's own MTP speculative decoding does NOT produce the symptom,
speculative decoding as a technique is exonerated and the fault lies in Kronk's
implementation of it (sdk/kronk/model/batchgen_speculative.go, draft_mtp.go) or
in yzma, not in the model.

Transcripts: %s`,
			badNone, len(upstreamNone), badMTP, len(upstreamMTP), badKronk, len(kronkMTP), dir)

	case badNone > 0 && badKronk > 0:
		t.Logf(`MODEL (at least partly). Upstream derails too (%d/%d failing runs with MTP
completely off), so the symptom is not created by Kronk. Kronk: %d/%d. Compare
the per-probe symptoms above to see whether Kronk makes it materially worse.

Transcripts: %s`, badNone, len(upstreamNone), badKronk, len(kronkMTP), dir)

	default:
		t.Logf(`No probe reproduced the reported symptom on any leg (upstream-none %d/%d,
upstream-mtp %d/%d, kronk %d/%d). The probes are not provoking it; lengthen the
conversations or raise KRONK_MTP_DIFF_REPEATS before drawing a conclusion.

Transcripts: %s`,
			badNone, len(upstreamNone), badMTP, len(upstreamMTP), badKronk, len(kronkMTP), dir)
	}

	if badMTP > badNone {
		t.Logf(`NOTE: upstream got WORSE with its own MTP enabled (%d -> %d failing runs).
That points at MTP speculative decoding generally rather than at Kronk.`,
			badNone, badMTP)
	}
}

// TestMTPDifferentialNonMTPControl is the Kronk-internal drafter-off
// reading. Kronk cannot disable MTP on a GGUF that declares an MTP head
// (see the file header), so the only way to get a no-drafter Kronk run is a
// sibling GGUF without one. The confound is the quant: the sibling on disk
// is Q4_K_XL against the target's Q8_K_XL, so a difference here is NOT
// attributable to MTP alone. Treat it as corroboration for the upstream
// --spec-type contrast, never as a substitute for it.
func TestMTPDifferentialNonMTPControl(t *testing.T) {
	diffGate(t)

	gguf := diffModelPath("KRONK_MTP_DIFF_CONTROL", diffControlDefault)
	if gguf == "" {
		t.Skipf("non-MTP sibling not on disk (%s); set KRONK_MTP_DIFF_CONTROL", diffControlDefault)
	}

	dir := diffOutDir(t)
	t.Logf("control GGUF (no MTP head): %s", gguf)
	t.Log("CAVEAT: different quant from the MTP target, so MTP is not the only variable")

	if err := defaults.WriteJinjaFiles("", ""); err != nil {
		t.Fatalf("seeding jinja templates: %v", err)
	}

	if err := kronk.Init(); err != nil {
		t.Fatalf("initialising the llama.cpp library: %v", err)
	}

	scores := runKronkLeg(t, "kronk-nomtp-control", model.Config{
		ModelFiles:        []string{gguf},
		PtrContextWindow:  ptrTo(diffCtx()),
		PtrNBatch:         ptrTo(diffNBatch),
		PtrNUBatch:        ptrTo(diffNUBatch),
		PtrNSeqMax:        ptrTo(diffNSeqMax),
		CacheTypeK:        model.GGMLTypeF16,
		CacheTypeV:        model.GGMLTypeF16,
		PtrFlashAttention: ptrTo(model.FlashAttentionDisabled),
	}, dir)

	t.Log("================ kronk, no MTP head (different quant) ===============")
	bad := reportScores(t, scores)

	t.Logf("SUMMARY: kronk-nomtp-control %d/%d failing runs", bad, len(scores))
}

func ptrTo[T any](v T) *T { return &v }

// =========================================================================
// TOOL-CALLING / AGENTIC PROBES.
//
// WHY THESE EXIST. Everything above drives plain conversational turns, and
// that is why the first differential run came back INCONCLUSIVE (0
// derailments on 6 probes / 3 legs): the four HIGH defects that best match
// the user's report can only fire on a TOOL-CALLING turn.
//
//	findings2.md §12a  the parser buffers <tool_call> bytes until it sees
//	                   </tool_call> (sdk/kronk/parsers/qwen/state_machine.go:88);
//	                   nothing drains it, so EOG or MaxTokens before the closing
//	                   tag discards the whole call
//	                   (sdk/kronk/model/batchgen_finish.go:261-282,
//	                   batchgen_slot.go:379-381).
//	findings2.md §8a   the post-render strip (sdk/kronk/model/prompts.go:177-181)
//	                   turns "<|im_start|>assistant\n<think>\n\n</think>\n\n" into
//	                   "<|im_start|>assistant\n\n\n" on EVERY assistant turn of a
//	                   tool loop (template qwen3.6.jinja:104).
//	findings2.md §8c   sdk/kronk/model/reasoning.go:91-92 deletes reasoning from
//	                   assistant history that qwen3.6.jinja:103 would replay for
//	                   in-turn (loop.index0 > last_query_index) messages — i.e.
//	                   precisely the assistant turns of a tool loop.
//
// Two probes, both driven through the SAME turn function on both legs:
//
//	toolProbeTruncation  §12a, direct: one tool-calling request per max_tokens
//	                     value, small enough that generation is cut off inside
//	                     the <tool_call> block. What does the caller get?
//	toolProbeAuditLoop   a real 4-file agentic audit: sequential tool calls,
//	                     rules planted in the system turn, reasoning ON so the
//	                     history damage of §8a/§8c is in play on every step.
//	                     Scored for "abandoned the task mid-way".
//
// MaxTokens is deliberately BINDING in the truncation probe — that is the
// whole point of it — and deliberately NON-binding in the audit loop, so a
// cut-off reply is never mistaken for a dropped instruction (see §13b).

// toolAuditFiles is the synthetic workspace the audit probe walks. Each file
// carries exactly one seeded defect, so "did it finish the task" is a count.
var toolAuditFiles = map[string]string{
	"svc/queue.go": "package svc\n\n// Queue fans jobs out to workers.\nfunc NewQueue() chan Job {\n\t// TODO: this channel is UNBOUNDED, a slow worker grows the heap forever\n\treturn make(chan Job)\n}\n",
	"svc/lease.go": "package svc\n\n// renew extends a worker lease.\nfunc renew(l *Lease) {\n\t// BUG: lease renewal IGNORES CLOCK SKEW between the workers and the store\n\tl.Until = now().Add(30 * time.Second)\n}\n",
	"store/pg.go":  "package store\n\n// Pending lists runnable jobs.\nconst pending = `SELECT id FROM jobs WHERE status = 'pending'`\n\n// BUG: there is NO INDEX on jobs(status), so this is a full table scan\n",
	"cmd/main.go":  "package main\n\nfunc main() {\n\tsrv := New()\n\tsrv.Run()\n\t// BUG: SIGTERM is IGNORED, so in-flight jobs are lost on deploy\n}\n",

	// A 4-file audit finished cleanly on BOTH legs (10 steps, 4/4 read, 4/4
	// recorded), so the workspace was extended to 8. Eight files is ~18 steps
	// of tool loop and drives the accumulated history past the point where a
	// model has to rely on its own replayed tool-call turns rather than on the
	// most recent message — which is exactly what findings2.md §8a/§8c damage.
	"svc/retry.go":     "package svc\n\n// backoff computes the retry delay.\nfunc backoff(attempt int) time.Duration {\n\t// BUG: the delay has NO JITTER, so all workers retry in lockstep\n\treturn time.Duration(1<<attempt) * time.Second\n}\n",
	"svc/tenant.go":    "package svc\n\n// pick chooses the next tenant to serve.\nfunc pick(ts []Tenant) Tenant {\n\t// BUG: strictly ordered by name, so tenant A STARVES every other tenant\n\tsort.Slice(ts, func(i, j int) bool { return ts[i].Name < ts[j].Name })\n\treturn ts[0]\n}\n",
	"store/migrate.go": "package store\n\n// Migrate applies schema changes.\nfunc Migrate(db *DB) error {\n\t// BUG: DDL runs OUTSIDE a transaction, so a failure leaves a half-migrated schema\n\treturn db.Exec(ddl)\n}\n",
	"api/auth.go":      "package api\n\n// check validates the caller's token.\nfunc check(tok string) bool {\n\t// BUG: token comparison is NOT CONSTANT TIME, so it leaks the token byte by byte\n\treturn tok == secret\n}\n",
}

// toolAuditOrder fixes the iteration order of toolAuditFiles so list_files
// returns the same payload on every leg and every sample.
var toolAuditOrder = []string{
	"svc/queue.go", "svc/lease.go", "store/pg.go", "cmd/main.go",
	"svc/retry.go", "svc/tenant.go", "store/migrate.go", "api/auth.go",
}

// toolAuditFollowUps are asked AFTER the model stops calling tools. They test
// the model's grip on its own tool-calling history, which is the part Kronk
// rewrites (findings2.md §8a strips the reasoning delimiters from every
// replayed assistant turn, §8c deletes the reasoning itself):
//
//	[0] can it account for the work it just did?
//	[1] a false premise about its own actions — it DID audit api/auth.go, and
//	    "you skipped it, correct?" invites the self-contradiction the user
//	    reports.
var toolAuditFollowUps = []string{
	"Stop using tools now. In one line: how many files did you audit, and list every path you read.",
	"You skipped api/auth.go earlier and never recorded a defect for it, correct? Answer yes or no first, then explain.",
}

// toolAuditTools is the OpenAI-shaped tool schema handed to both legs.
// model.D is map[string]any, so the same value serialises for Kronk's Jinja
// render and for llama-server's /v1/chat/completions body.
var toolAuditTools = []model.D{
	{
		"type": "function",
		"function": model.D{
			"name":        "list_files",
			"description": "List every source file in the workspace.",
			"parameters": model.D{
				"type":       "object",
				"properties": model.D{},
			},
		},
	},
	{
		"type": "function",
		"function": model.D{
			"name":        "read_file",
			"description": "Read one source file from the workspace.",
			"parameters": model.D{
				"type": "object",
				"properties": model.D{
					"path": model.D{
						"type":        "string",
						"description": "Workspace-relative path, exactly as returned by list_files.",
					},
				},
				"required": []any{"path"},
			},
		},
	},
	{
		"type": "function",
		"function": model.D{
			"name":        "record_defect",
			"description": "Record one defect found in one file.",
			"parameters": model.D{
				"type": "object",
				"properties": model.D{
					"path": model.D{
						"type":        "string",
						"description": "The file the defect is in.",
					},
					"note": model.D{
						"type":        "string",
						"description": "One sentence describing the defect.",
					},
				},
				"required": []any{"path", "note"},
			},
		},
	},
}

// toolWeatherTools is the single-tool schema for the truncation probe. One
// required parameter plus one optional one makes the emitted <tool_call>
// block long enough that a small max_tokens lands INSIDE it.
var toolWeatherTools = []model.D{
	{
		"type": "function",
		"function": model.D{
			"name":        "get_weather",
			"description": "Get the current weather for a location.",
			"parameters": model.D{
				"type": "object",
				"properties": model.D{
					"location": model.D{
						"type":        "string",
						"description": "City and country, e.g. 'London, United Kingdom'.",
					},
					"units": model.D{
						"type":        "string",
						"description": "Either 'celsius' or 'fahrenheit'.",
					},
				},
				"required": []any{"location", "units"},
			},
		},
	},
}

const toolWeatherSystem = "You are a weather assistant. When the user asks about weather you MUST " +
	"call the get_weather function immediately, with no preamble and no explanation."

const toolWeatherUser = "What is the weather in London, United Kingdom? Use celsius."

const toolAuditSystem = "You are auditing a small Go repository using the provided tools.\n\n" +
	"HARD RULES for this entire conversation:\n" +
	"(RULE-1) Call read_file exactly ONCE per file. Never read the same file twice.\n" +
	"(RULE-2) Immediately after reading a file, call record_defect for that file, before reading the next one.\n" +
	"(RULE-3) Every message you write to the user must end with the line AUDIT-OK.\n\n" +
	"Work through EVERY file in the workspace. When all files have been recorded, write a " +
	"final summary listing each file path and its defect."

const toolAuditUser = "Start the audit. Call list_files first, then work through every file it returns."

// toolLoopMaxIters bounds the agentic loop. Eight files at two calls each plus
// list_files plus the final answer is ~18 steps; 30 leaves room for a model
// that reads and records in separate steps without letting a runaway loop burn
// an hour.
const toolLoopMaxIters = 30

// toolLoopMaxTokens is deliberately generous: truncation must not be
// confused with abandonment (§13b). Truncation is still recorded per step.
const toolLoopMaxTokens = 1600

// toolTruncCaps are the max_tokens values swept by the truncation probe.
// With thinking OFF the Qwen3.6 generation prompt is
// "<|im_start|>assistant\n<think>\n\n</think>\n\n" (qwen3.6.jinja:150-156),
// so generation starts directly on the answer channel and the <tool_call>
// block begins within a token or two. Every value here therefore lands
// inside the tool call, before "</function>\n</tool_call>".
var toolTruncCaps = []int{4, 8, 12, 16, 20, 24, 32, 48, 64}

// toolTruncCapList is toolTruncCaps unless KRONK_MTP_TOOL_CAPS overrides it
// with a comma-separated list. The observed failure boundary depends on how
// many tokens the model spends on the call, so being able to sweep a narrow
// band around it without editing the file matters.
func toolTruncCapList() []int {
	v := os.Getenv("KRONK_MTP_TOOL_CAPS")
	if v == "" {
		return toolTruncCaps
	}

	var out []int
	for _, f := range strings.Split(v, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(f)); err == nil && n > 0 {
			out = append(out, n)
		}
	}

	if len(out) == 0 {
		return toolTruncCaps
	}

	return out
}

// toolUpstreamSeed is the seed sent to llama-server. Kronk exposes no seed at
// all (see the file header), so the only way to tell an intermittent Kronk
// defect from sampling luck is to sample BOTH legs several times — upstream
// with a different seed each time. KRONK_MTP_TOOL_SEED sets it.
func toolUpstreamSeed() int {
	if v := os.Getenv("KRONK_MTP_TOOL_SEED"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}

	return 42
}

// toolLegEnabled allows one leg to be run on its own: KRONK_MTP_TOOL_KRONK=0
// skips the Kronk leg (KRONK_MTP_DIFF_UPSTREAM=0 already skips upstream), so
// repeated samples of a single leg cost one model load each rather than two.
func toolLegEnabled(env string) bool {
	return os.Getenv(env) != "0"
}

// toolProbeEnabled selects which of the two tool probes run.
// KRONK_MTP_TOOL_PROBES is a comma-separated list of "trunc" and "loop";
// empty means both. The §12a truncation sweep is cheap and decisive, so it
// is worth being able to run it on its own.
func toolProbeEnabled(name string) bool {
	filter := os.Getenv("KRONK_MTP_TOOL_PROBES")
	if filter == "" {
		return true
	}

	for _, want := range strings.Split(filter, ",") {
		if strings.TrimSpace(want) == name {
			return true
		}
	}

	return false
}

// =========================================================================
// One turn, two backends.

// toolCallRecord is one tool call as the caller received it.
type toolCallRecord struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
	Raw  string         `json:"raw,omitempty"`
}

// toolTurn is everything one assistant turn delivered to the caller. It is
// the ONLY thing the scorers look at: the question is what an API client
// sees, not what happened internally.
type toolTurn struct {
	Content   string           `json:"content"`
	Reasoning string           `json:"reasoning,omitempty"`
	Calls     []toolCallRecord `json:"calls,omitempty"`

	FinishReason string `json:"finish_reason"`

	// OutputTokens is the comparable figure: Kronk's reasoning +
	// completion counters summed, llama-server's completion_tokens as-is.
	OutputTokens int `json:"output_tokens"`

	// The raw counters behind OutputTokens, recorded separately because the
	// truncation probe turns out to need them: a Kronk turn cut off at
	// max_tokens=4 still reports the token count of the WHOLE generation
	// (see the analysis in toolRunTruncation), so the caller cannot use
	// usage to detect the cut either.
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`

	Err     string  `json:"error,omitempty"`
	Seconds float64 `json:"seconds"`
}

// toolTurnFn sends one request and returns what the caller got back.
type toolTurnFn func(t *testing.T, msgs []model.D, tools []model.D, maxTokens int, thinking bool) toolTurn

// toolKronkTurn drives kronk.Chat. Usage note: Kronk reports reasoning
// tokens in a SEPARATE counter, so OutputTokens has to add both or every
// reasoning turn looks 10x shorter than it was (§13b).
func toolKronkTurn(krn *kronk.Kronk) toolTurnFn {
	return func(t *testing.T, msgs []model.D, tools []model.D, maxTokens int, thinking bool) toolTurn {
		d := model.D{"messages": msgs, "tools": tools}
		for k, v := range diffSampling {
			d[k] = v
		}
		d["enable_thinking"] = thinking
		d["max_tokens"] = maxTokens

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()

		start := time.Now()
		resp, err := krn.Chat(ctx, d)
		elapsed := time.Since(start)

		if err != nil {
			return toolTurn{Err: err.Error(), Seconds: elapsed.Seconds()}
		}

		out := toolTurn{Seconds: elapsed.Seconds()}

		if len(resp.Choices) > 0 {
			out.FinishReason = resp.Choices[0].FinishReason()

			if msg := resp.Choices[0].Message; msg != nil {
				out.Content = msg.Content
				out.Reasoning = msg.Reasoning

				for _, tc := range msg.ToolCalls {
					out.Calls = append(out.Calls, toolCallRecord{
						Name: tc.Function.Name,
						Args: map[string]any(tc.Function.Arguments),
						Raw:  tc.Raw,
					})
				}
			}
		}

		if resp.Usage != nil {
			out.OutputTokens = resp.Usage.CompletionTokens + resp.Usage.ReasoningTokens
			out.PromptTokens = resp.Usage.PromptTokens
			out.CompletionTokens = resp.Usage.CompletionTokens
			out.ReasoningTokens = resp.Usage.ReasoningTokens
		}

		return out
	}
}

// toolUpstreamTurn drives llama-server's /v1/chat/completions with the same
// messages and the same tool schema. llama-server's completion_tokens
// already includes reasoning tokens, so it is used as-is.
func toolUpstreamTurn(base string) toolTurnFn {
	client := &http.Client{Timeout: 20 * time.Minute}

	return func(t *testing.T, msgs []model.D, tools []model.D, maxTokens int, thinking bool) toolTurn {
		body := map[string]any{
			"messages":             msgs,
			"tools":                tools,
			"chat_template_kwargs": map[string]any{"enable_thinking": thinking},
			"seed":                 toolUpstreamSeed(),
			"max_tokens":           maxTokens,
		}

		for k, v := range diffSampling {
			if k == "enable_thinking" || k == "max_tokens" {
				continue
			}
			body[k] = v
		}

		raw, err := json.Marshal(body)
		if err != nil {
			return toolTurn{Err: "marshal: " + err.Error()}
		}

		start := time.Now()
		resp, err := client.Post(base+"/v1/chat/completions", "application/json", bytes.NewReader(raw))
		elapsed := time.Since(start)

		if err != nil {
			return toolTurn{Err: err.Error(), Seconds: elapsed.Seconds()}
		}

		payload, err := readAllClose(resp)
		if err != nil {
			return toolTurn{Err: err.Error(), Seconds: elapsed.Seconds()}
		}

		var out struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
					ToolCalls []struct {
						Function struct {
							Name string `json:"name"`
							// OpenAI ships arguments as a JSON-encoded
							// string; llama-server follows that.
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(payload), &out); err != nil || len(out.Choices) == 0 {
			return toolTurn{
				Err: fmt.Sprintf("bad response (status %d): %s",
					resp.StatusCode, truncate(payload, 400)),
				Seconds: elapsed.Seconds(),
			}
		}

		turn := toolTurn{
			Content:          out.Choices[0].Message.Content,
			Reasoning:        out.Choices[0].Message.Reasoning,
			FinishReason:     out.Choices[0].FinishReason,
			OutputTokens:     out.Usage.CompletionTokens,
			PromptTokens:     out.Usage.PromptTokens,
			CompletionTokens: out.Usage.CompletionTokens,
			Seconds:          elapsed.Seconds(),
		}

		for _, tc := range out.Choices[0].Message.ToolCalls {
			rec := toolCallRecord{Name: tc.Function.Name, Raw: tc.Function.Arguments}

			var args map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
				rec.Args = args
			}

			turn.Calls = append(turn.Calls, rec)
		}

		return turn
	}
}

// =========================================================================
// Probe 1: §12a — a tool call cut off at MaxTokens.

type toolTruncCase struct {
	Leg          string   `json:"leg"`
	MaxTokens    int      `json:"max_tokens"`
	FinishReason string   `json:"finish_reason"`
	Verdict      string   `json:"verdict"`
	ToolNames    []string `json:"tool_names,omitempty"`
	OutputTokens int      `json:"output_tokens"`

	// PromptTokens/CompletionTokens/ReasoningTokens are the raw usage
	// counters. On the Kronk leg they are the second half of the §12a
	// story: they do not shrink when max_tokens cuts the turn short.
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`

	Content   string `json:"content"`
	Reasoning string `json:"reasoning,omitempty"`
	Err       string `json:"error,omitempty"`
}

// toolTruncVerdicts classify what the caller received.
const (
	toolVerdictDelivered = "tool-call-delivered" // a parsed tool call came back
	toolVerdictRawText   = "raw-tool-text"       // the partial call arrived as content
	toolVerdictProse     = "prose-only"          // some other visible text
	toolVerdictSilent    = "silent-empty"        // NOTHING, and no error: §12a
	toolVerdictError     = "error"               // an explicit error (acceptable)
)

func toolTruncVerdict(c toolTruncCase) string {
	switch {
	case c.Err != "":
		return toolVerdictError

	case len(c.ToolNames) > 0:
		return toolVerdictDelivered

	case strings.Contains(c.Content, "<tool_call>") ||
		strings.Contains(c.Content, "<function=") ||
		strings.Contains(c.Content, `"name"`):
		return toolVerdictRawText

	case strings.TrimSpace(c.Content) == "" && strings.TrimSpace(c.Reasoning) == "":
		return toolVerdictSilent

	default:
		return toolVerdictProse
	}
}

func toolRunTruncation(t *testing.T, label string, turn toolTurnFn) []toolTruncCase {
	t.Helper()

	var cases []toolTruncCase

	msgs := []model.D{
		{"role": "system", "content": toolWeatherSystem},
		{"role": "user", "content": toolWeatherUser},
	}

	for _, cap := range toolTruncCapList() {
		// Thinking OFF: the reasoning block would consume the whole budget
		// before the tool call ever started, which tests nothing.
		tt := turn(t, msgs, toolWeatherTools, cap, false)

		c := toolTruncCase{
			Leg: label, MaxTokens: cap,
			FinishReason:     tt.FinishReason,
			OutputTokens:     tt.OutputTokens,
			PromptTokens:     tt.PromptTokens,
			CompletionTokens: tt.CompletionTokens,
			ReasoningTokens:  tt.ReasoningTokens,
			Content:          tt.Content,
			Reasoning:        tt.Reasoning,
			Err:              tt.Err,
		}

		for _, call := range tt.Calls {
			c.ToolNames = append(c.ToolNames, call.Name)
		}

		c.Verdict = toolTruncVerdict(c)
		cases = append(cases, c)

		t.Logf("[%s] max_tokens=%-3d finish=%-10s prompt=%-5d completion=%-4d reasoning=%-4d out_tokens=%-4d verdict=%-20s content=%q calls=%v",
			label, cap, c.FinishReason, c.PromptTokens, c.CompletionTokens,
			c.ReasoningTokens, c.OutputTokens, c.Verdict,
			truncate(c.Content, 120), c.ToolNames)
	}

	return cases
}

// =========================================================================
// Probe 2: the agentic audit loop.

// toolAuditExec is the tool runtime. It is deliberately strict about paths so
// a hallucinated path is visible in the transcript rather than silently
// succeeding.
func toolAuditExec(call toolCallRecord) string {
	switch call.Name {
	case "list_files":
		return strings.Join(toolAuditOrder, "\n")

	case "read_file":
		path, _ := call.Args["path"].(string)
		body, ok := toolAuditFiles[path]
		if !ok {
			return fmt.Sprintf("ERROR: no such file %q. The files are: %s",
				path, strings.Join(toolAuditOrder, ", "))
		}

		return body

	case "record_defect":
		path, _ := call.Args["path"].(string)
		if _, ok := toolAuditFiles[path]; !ok {
			return fmt.Sprintf("ERROR: no such file %q", path)
		}

		return "recorded"

	default:
		return fmt.Sprintf("ERROR: unknown function %q", call.Name)
	}
}

type toolLoopStep struct {
	Iter         int              `json:"iter"`
	Calls        []toolCallRecord `json:"calls,omitempty"`
	Content      string           `json:"content,omitempty"`
	Reasoning    string           `json:"reasoning,omitempty"`
	FinishReason string           `json:"finish_reason"`
	OutputTokens int              `json:"output_tokens"`
	Truncated    bool             `json:"truncated,omitempty"`
	Err          string           `json:"error,omitempty"`
	Seconds      float64          `json:"seconds"`
}

type toolLoopRun struct {
	Label     string         `json:"label"`
	Steps     []toolLoopStep `json:"steps"`
	Messages  []model.D      `json:"messages"`
	Read      []string       `json:"files_read"`
	Recorded  []string       `json:"files_recorded"`
	DupReads  []string       `json:"duplicate_reads,omitempty"`
	Empty     []int          `json:"empty_steps,omitempty"`
	HitCap    bool           `json:"hit_iteration_cap"`
	FinalText string         `json:"final_text"`

	// FollowUps are the post-tool-loop turns (toolAuditFollowUps). They probe
	// the model's grip on its own tool-calling history, which is the part
	// Kronk rewrites before every step (findings2.md §8a, §8c).
	FollowUps []toolLoopStep `json:"follow_ups,omitempty"`
}

// toolRunAuditLoop drives the agentic loop until the model stops calling
// tools, errors, or hits the iteration cap.
func toolRunAuditLoop(t *testing.T, label string, turn toolTurnFn, thinking bool) toolLoopRun {
	t.Helper()

	run := toolLoopRun{Label: label}

	msgs := []model.D{
		{"role": "system", "content": toolAuditSystem},
		{"role": "user", "content": toolAuditUser},
	}

	readSeen := map[string]int{}
	recSeen := map[string]bool{}

	for iter := range toolLoopMaxIters {
		tt := turn(t, msgs, toolAuditTools, toolLoopMaxTokens, thinking)

		step := toolLoopStep{
			Iter: iter, Calls: tt.Calls, Content: tt.Content,
			Reasoning: tt.Reasoning, FinishReason: tt.FinishReason,
			OutputTokens: tt.OutputTokens, Err: tt.Err,
			Truncated: tt.OutputTokens >= toolLoopMaxTokens,
			Seconds:   tt.Seconds,
		}

		run.Steps = append(run.Steps, step)

		if tt.Err != "" {
			t.Logf("[%s] iter=%d ERROR %s", label, iter, tt.Err)
			break
		}

		t.Logf("[%s] iter=%d finish=%-10s out_tokens=%-5d calls=%d content=%q",
			label, iter, tt.FinishReason, tt.OutputTokens, len(tt.Calls),
			truncate(strings.TrimSpace(tt.Content), 100))

		// No tool calls: either the final answer, or NOTHING AT ALL — which
		// is the §12a signature at the API level.
		if len(tt.Calls) == 0 {
			if strings.TrimSpace(tt.Content) == "" && strings.TrimSpace(tt.Reasoning) == "" {
				run.Empty = append(run.Empty, iter)
			}

			run.FinalText = tt.Content
			break
		}

		// Replay the assistant turn exactly as an OpenAI-style client would:
		// content plus tool_calls whose arguments are a JSON string.
		var tcDocs []model.D
		for i, call := range tt.Calls {
			args := call.Raw
			if call.Args != nil {
				if b, err := json.Marshal(call.Args); err == nil {
					args = string(b)
				}
			}

			tcDocs = append(tcDocs, model.D{
				"id":   fmt.Sprintf("call_%d_%d", iter, i),
				"type": "function",
				"function": model.D{
					"name":      call.Name,
					"arguments": args,
				},
			})
		}

		assistant := model.D{
			"role":       "assistant",
			"content":    tt.Content,
			"tool_calls": tcDocs,
		}

		// Reasoning is replayed the way a faithful client would: the field is
		// what qwen3.6.jinja:103-104 reads back for an in-turn assistant
		// message. Kronk deletes it again in normalizeHistoryReasoning
		// (sdk/kronk/model/reasoning.go:91-92); llama-server does not.
		if tt.Reasoning != "" {
			assistant["reasoning_content"] = tt.Reasoning
		}

		msgs = append(msgs, assistant)

		for i, call := range tt.Calls {
			result := toolAuditExec(call)

			switch call.Name {
			case "read_file":
				if path, ok := call.Args["path"].(string); ok {
					if _, dup := readSeen[path]; dup {
						run.DupReads = append(run.DupReads, path)
					}
					readSeen[path]++
				}

			case "record_defect":
				if path, ok := call.Args["path"].(string); ok {
					recSeen[path] = true
				}
			}

			msgs = append(msgs, model.D{
				"role":         "tool",
				"name":         call.Name,
				"tool_call_id": fmt.Sprintf("call_%d_%d", iter, i),
				"content":      result,
			})
		}

		if iter == toolLoopMaxIters-1 {
			run.HitCap = true
		}
	}

	// Follow-up turns. The model has stopped calling tools; now ask it to
	// account for what it did. Tools stay in the request so the two legs see
	// an identical prompt shape.
	for i, q := range toolAuditFollowUps {
		msgs = append(msgs, model.D{"role": "user", "content": q})

		tt := turn(t, msgs, toolAuditTools, toolLoopMaxTokens, thinking)

		step := toolLoopStep{
			Iter: len(run.Steps) + i, Calls: tt.Calls, Content: tt.Content,
			Reasoning: tt.Reasoning, FinishReason: tt.FinishReason,
			OutputTokens: tt.OutputTokens, Err: tt.Err,
			Truncated: tt.OutputTokens >= toolLoopMaxTokens,
			Seconds:   tt.Seconds,
		}

		run.FollowUps = append(run.FollowUps, step)

		t.Logf("[%s] follow-up=%d finish=%-10s out_tokens=%-5d calls=%d content=%q",
			label, i, tt.FinishReason, tt.OutputTokens, len(tt.Calls),
			truncate(strings.TrimSpace(tt.Content), 160))

		if tt.Err != "" {
			break
		}

		msgs = append(msgs, model.D{"role": "assistant", "content": tt.Content})
	}

	for _, p := range toolAuditOrder {
		if _, ok := toolAuditFiles[p]; !ok {
			continue
		}
		if readSeen[p] > 0 {
			run.Read = append(run.Read, p)
		}
		if recSeen[p] {
			run.Recorded = append(run.Recorded, p)
		}
	}

	run.Messages = msgs

	return run
}

// toolScoreLoop turns the audit run into objective symptom counts. The
// primary symptom is the reported one: the model started the task and
// stopped part-way through.
func toolScoreLoop(run toolLoopRun) (metric string, symptoms []string) {
	want := len(toolAuditOrder)

	metric = fmt.Sprintf("read %d/%d, recorded %d/%d, steps %d, dup-reads %d, empty-steps %v, hit-cap %t",
		len(run.Read), want, len(run.Recorded), want, len(run.Steps),
		len(run.DupReads), run.Empty, run.HitCap)

	if len(run.Empty) > 0 {
		symptoms = append(symptoms, fmt.Sprintf(
			"step(s) %v returned NOTHING to the caller: no tool call, no content, no error "+
				"(findings2.md §12a — the buffered tool call was discarded at end-of-generation)",
			run.Empty))
	}

	// "Never started" and "started then dropped it" are different failures and
	// must not be pooled. A model that answers step 0 with the plain text
	// "list_files" instead of a tool call never entered the loop: that is a
	// tool-format failure, not the reported symptom. Both legs produce it
	// occasionally, so keeping them apart is what makes the comparison mean
	// anything.
	var totalCalls int
	for _, st := range run.Steps {
		totalCalls += len(st.Calls)
	}

	switch {
	case totalCalls == 0:
		symptoms = append(symptoms, fmt.Sprintf(
			"never issued a tool call at all — step 0 answered with plain text %q, so the "+
				"loop was never entered (tool-format failure, NOT mid-task abandonment)",
			truncate(strings.TrimSpace(run.Steps[0].Content), 80)))

	default:
		if len(run.Read) < want {
			symptoms = append(symptoms, fmt.Sprintf(
				"ABANDONED MID-TASK: read %d/%d files (%v), then stopped calling tools",
				len(run.Read), want, run.Read))
		}

		if len(run.Recorded) < want {
			symptoms = append(symptoms, fmt.Sprintf(
				"ABANDONED MID-TASK: recorded %d/%d defects (%v), then stopped calling tools",
				len(run.Recorded), want, run.Recorded))
		}
	}

	if len(run.DupReads) > 0 {
		symptoms = append(symptoms, fmt.Sprintf(
			"re-read files it had already read (RULE-1), losing track of its own history: %v",
			run.DupReads))
	}

	if run.HitCap {
		symptoms = append(symptoms, fmt.Sprintf(
			"never produced a final answer within %d steps", toolLoopMaxIters))
	}

	// The final summary must name every file. A summary that silently covers
	// fewer files than were audited is the "starts a task then drops it"
	// symptom in its mildest form.
	if run.FinalText != "" {
		var missing []string
		for _, p := range toolAuditOrder {
			if !strings.Contains(run.FinalText, p) {
				missing = append(missing, p)
			}
		}

		if len(missing) > 0 && len(run.Read) == want {
			symptoms = append(symptoms, fmt.Sprintf(
				"final summary omits files it audited: %v", missing))
		}
	}

	for _, s := range run.Steps {
		if s.Err != "" {
			symptoms = append(symptoms, fmt.Sprintf("step %d errored: %s", s.Iter, s.Err))
		}
	}

	// Follow-up 0: account for the files it read. Follow-up 1: a false premise
	// about its own actions. Both read the model's own tool-calling history,
	// which is what findings2.md §8a/§8c rewrite on every step.
	if len(run.FollowUps) > 0 {
		fu := run.FollowUps[0]

		// Only the visible answer counts. The reasoning block routinely
		// re-enumerates the list_files output, which would credit a reply that
		// claims a smaller number to the user.
		var named []string
		for _, p := range toolAuditOrder {
			if strings.Contains(fu.Content, p) {
				named = append(named, p)
			}
		}

		metric += fmt.Sprintf(", recall %d/%d paths", len(named), want)

		if fu.Err != "" {
			symptoms = append(symptoms, fmt.Sprintf("recall turn errored: %s", fu.Err))
		}

		// A reply cut off at the cap can legitimately be missing the tail of
		// the list, so truncation is excused (§13b).
		if len(named) < want && !fu.Truncated && fu.Err == "" {
			symptoms = append(symptoms, fmt.Sprintf(
				"could not account for its own work: asked to list every file it read, it named "+
					"%d of %d (%v)", len(named), want, named))
		}
	}

	if len(run.FollowUps) > 1 {
		fu := run.FollowUps[1]

		rejected := diffRejects.MatchString(fu.Content)

		metric += fmt.Sprintf(", false-premise rejected %t", rejected)

		switch {
		case fu.Err != "":
			symptoms = append(symptoms, fmt.Sprintf("false-premise turn errored: %s", fu.Err))

		case !rejected:
			symptoms = append(symptoms, fmt.Sprintf(
				"accepted a false premise about its own actions (it DID audit %s): %q "+
					"— this is the reported self-contradiction",
				toolAuditOrder[len(toolAuditOrder)-1], truncate(strings.TrimSpace(fu.Content), 200)))
		}
	}

	return metric, symptoms
}

// =========================================================================
// Entry point for the tool-calling differential.

type toolLegReport struct {
	Label string          `json:"label"`
	Notes map[string]any  `json:"notes"`
	Trunc []toolTruncCase `json:"truncation_probe"`
	Loop  toolLoopRun     `json:"audit_loop_probe"`
}

func (r *toolLegReport) save(t *testing.T, dir string) {
	t.Helper()

	p := filepath.Join(dir, "toolcall-"+r.Label+".json")

	b, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		t.Errorf("marshalling %s: %v", r.Label, err)
		return
	}

	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Errorf("writing %s: %v", p, err)
		return
	}

	t.Logf("tool-call transcript: %s", p)
}

// TestToolCallDifferential is the tool-calling counterpart to
// TestMTPDifferential, and it targets the four defects that plain
// conversational probes structurally cannot reach (see the section header).
//
// It runs upstream llama-server first and Kronk second, SEQUENTIALLY (one
// ~39 GB model resident at a time), driving both through the same two probes
// with identical tool schemas and identical messages.
//
// FAILS when:
//
//   - any Kronk truncation case comes back "silent-empty" — no tool call, no
//     content, no error (findings2.md §12a); or
//   - Kronk shows an audit-loop symptom that upstream does not.
//
// Passes with a loud log when both legs behave the same way, which is a
// legitimate answer: it puts the behaviour in the model, not in Kronk.
//
//	KRONK_MTP_DIFF=1 go test -tags kronkdiff -timeout 3h -v \
//	  -run TestToolCallDifferential ./sdk/kronk/tests/mtp/
//
// Honours the same knobs as TestMTPDifferential: KRONK_MTP_DIFF_MODEL, _CTX,
// _SERVER, _OUT, _UPSTREAM — plus KRONK_MTP_TOOL_PROBES (trunc,loop),
// KRONK_MTP_TOOL_CAPS, KRONK_MTP_TOOL_SEED and KRONK_MTP_TOOL_KRONK=0.
//
// =========================================================================
// MEASURED, 2026-08-01, mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL, n_ctx 16384, Metal,
// libllama b10211, upstream llama-server from the same directory.
//
// TRUNCATION PROBE — deterministic, and a Kronk-only defect. Identical
// requests, identical prompt (359 tokens):
//
//	max_tokens   kronk                            upstream (--spec-type none)
//	4..39        finish_reason "stop", content "", finish_reason "length",
//	             tool_calls [], NO error           get_weather DELIVERED
//	>= 40        finish_reason "tool_calls", ok     finish_reason "tool_calls"
//
// The boundary is exactly the number of tokens the call takes (39 here): 39
// loses it, 40 keeps it. usage reported 39 completion tokens for EVERY cap,
// including max_tokens=4, so the cap is not enforced during the call either.
// Upstream never loses the call and always flags the truncation.
//
// AUDIT LOOP — intermittent, and it needs repeated sampling. Kronk exposes no
// seed, so ONE run of each leg cannot attribute anything; these are 6 Kronk
// samples and 7 upstream samples (seeds 42, 7, 1234, 99, 555, 31337,
// 20260801) of the 8-file loop:
//
//	                                        kronk    upstream
//	completed 8/8 read and 8/8 recorded      3/6       5/7
//	ABANDONED MID-TASK                       2/6       0/7
//	  (4/8 files, then "## Audit Complete";
//	   and 8/8 read but 7/8 recorded)
//	re-read a file it had already read       1/6       0/7
//	never entered the loop (emitted the
//	  plain text "list_files" at step 0)     0/6       2/7
//
// So the reported symptom — starts the task, does part of it, declares it
// finished — appeared only on Kronk (2/6 vs 0/7), and in the worst sample it
// also accepted a false premise about its own actions ("Yes. I stopped using
// tools after auditing the fourth file … which caused me to skip the
// remaining four files"), which is the reported self-contradiction. Upstream's
// two failures are a DIFFERENT mode (never emitting a tool call at all), which
// is why the scorer keeps the two apart.
//
// At these sample sizes 2/6 vs 0/7 is suggestive, NOT significant (Fisher
// p ~ 0.19). The mechanism, however, is not in doubt and is pinned
// separately: every step of a Kronk tool loop is prompted with the assistant
// headers mangled and the reasoning deleted
// (sdk/kronk/model/toolcall_repro_test.go,
// TestQwen36MultiStepToolLoopPromptDamage). Raise the sample count before
// quoting a rate.
// =========================================================================
func TestToolCallDifferential(t *testing.T) {
	diffGate(t)

	gguf := diffModelPath("KRONK_MTP_DIFF_MODEL", diffModelDefault)
	if gguf == "" {
		t.Skipf("MTP target not on disk (%s); set KRONK_MTP_DIFF_MODEL", diffModelDefault)
	}

	dir := diffOutDir(t)
	t.Logf("target GGUF   : %s", gguf)
	t.Logf("transcript dir: %s", dir)
	t.Logf("n_ctx         : %d", diffCtx())

	var upstream, kronkLeg *toolLegReport

	// ---- Leg A: upstream llama-server, MTP off. --------------------------
	//
	// Runs BEFORE kronk.Init() for the reason documented in
	// TestMTPDifferential: loading libllama through purego into this address
	// space has been observed to corrupt the Go heap under the pure-HTTP leg.

	bin := diffServerBin()

	switch {
	case os.Getenv("KRONK_MTP_DIFF_UPSTREAM") == "0":
		t.Log("upstream leg disabled by KRONK_MTP_DIFF_UPSTREAM=0")

	case bin == "":
		t.Log("llama-server not found; set KRONK_MTP_DIFF_SERVER. Without it a Kronk " +
			"failure cannot be attributed to Kronk rather than to the model.")

	default:
		t.Logf("upstream binary: %s", bin)

		base, stop := startUpstream(t, bin, gguf, "none", dir)

		upstream = &toolLegReport{
			Label: "upstream-tools",
			Notes: map[string]any{
				"driver":    bin,
				"spec_type": "none",
				"n_ctx":     diffCtx(),
			},
		}

		turn := toolUpstreamTurn(base)

		if toolProbeEnabled("trunc") {
			t.Log("---------------- upstream: §12a truncation sweep ----------------")
			upstream.Trunc = toolRunTruncation(t, upstream.Label, turn)
		}

		if toolProbeEnabled("loop") {
			t.Log("---------------- upstream: agentic audit loop -------------------")
			upstream.Loop = toolRunAuditLoop(t, upstream.Label, turn, true)
		}

		stop()
		upstream.save(t, dir)
	}

	// ---- Leg B: Kronk on the same GGUF. ---------------------------------

	if !toolLegEnabled("KRONK_MTP_TOOL_KRONK") {
		t.Log("kronk leg disabled by KRONK_MTP_TOOL_KRONK=0; reporting the upstream leg only")

		if upstream != nil {
			metric, symptoms := toolScoreLoop(upstream.Loop)
			t.Logf("upstream (seed %d): %s", toolUpstreamSeed(), metric)
			for _, s := range symptoms {
				t.Logf("       symptom: %s", s)
			}
		}

		return
	}

	if err := defaults.WriteJinjaFiles("", ""); err != nil {
		t.Fatalf("seeding jinja templates: %v", err)
	}

	if err := kronk.Init(); err != nil {
		t.Fatalf("initialising the llama.cpp library: %v", err)
	}

	cfg := model.Config{
		ModelFiles:        []string{gguf},
		PtrContextWindow:  ptrTo(diffCtx()),
		PtrNBatch:         ptrTo(diffNBatch),
		PtrNUBatch:        ptrTo(diffNUBatch),
		PtrNSeqMax:        ptrTo(diffNSeqMax),
		CacheTypeK:        model.GGMLTypeF16,
		CacheTypeV:        model.GGMLTypeF16,
		PtrFlashAttention: ptrTo(model.FlashAttentionDisabled),
	}

	krn, err := kronk.New(model.WithConfig(cfg))
	if err != nil {
		t.Fatalf("loading kronk (%v): %v", cfg.ModelFiles, err)
	}

	kronkLeg = &toolLegReport{
		Label: "kronk-tools",
		Notes: map[string]any{
			"driver": "kronk.Chat",
			"n_ctx":  cfg.ContextWindow(),
			"model":  fmt.Sprintf("%+v", krn.ModelInfo()),
		},
	}

	turn := toolKronkTurn(krn)

	if toolProbeEnabled("trunc") {
		t.Log("---------------- kronk: §12a truncation sweep -------------------")
		kronkLeg.Trunc = toolRunTruncation(t, kronkLeg.Label, turn)
	}

	if toolProbeEnabled("loop") {
		t.Log("---------------- kronk: agentic audit loop ----------------------")
		kronkLeg.Loop = toolRunAuditLoop(t, kronkLeg.Label, turn, true)
	}

	if err := krn.Unload(context.Background()); err != nil {
		t.Errorf("unloading kronk: %v", err)
	}

	kronkLeg.save(t, dir)

	// ---- Report. --------------------------------------------------------

	t.Log("================ §12a: a tool call cut off at max_tokens ============")

	var kronkSilent, upstreamSilent []int
	var kronkLength, upstreamLength int

	for _, c := range kronkLeg.Trunc {
		if c.Verdict == toolVerdictSilent {
			kronkSilent = append(kronkSilent, c.MaxTokens)
		}
		if c.FinishReason == "length" {
			kronkLength++
		}
	}

	if upstream != nil {
		for _, c := range upstream.Trunc {
			if c.Verdict == toolVerdictSilent {
				upstreamSilent = append(upstreamSilent, c.MaxTokens)
			}
			if c.FinishReason == "length" {
				upstreamLength++
			}
		}
	}

	t.Logf("kronk   : silent-empty at max_tokens=%v, finish_reason==\"length\" on %d/%d cases",
		kronkSilent, kronkLength, len(kronkLeg.Trunc))

	if upstream != nil {
		t.Logf("upstream: silent-empty at max_tokens=%v, finish_reason==\"length\" on %d/%d cases",
			upstreamSilent, upstreamLength, len(upstream.Trunc))
	}

	var kronkSymptoms, upstreamSymptoms []string

	if toolProbeEnabled("loop") {
		t.Log("================ agentic audit loop ================================")

		var kronkMetric string
		kronkMetric, kronkSymptoms = toolScoreLoop(kronkLeg.Loop)
		t.Logf("kronk   : %s", kronkMetric)
		for _, s := range kronkSymptoms {
			t.Logf("       symptom: %s", s)
		}

		if upstream != nil {
			var upstreamMetric string
			upstreamMetric, upstreamSymptoms = toolScoreLoop(upstream.Loop)
			t.Logf("upstream: %s", upstreamMetric)
			for _, s := range upstreamSymptoms {
				t.Logf("       symptom: %s", s)
			}
		}
	}

	// ---- Verdicts. ------------------------------------------------------

	if len(kronkSilent) > 0 {
		t.Errorf(`findings2.md §12a REPRODUCED at the API level. Kronk returned an EMPTY
response with no tool call, no content and no error for max_tokens=%v on a
request the model answered with a tool call.

The Qwen state machine buffers every byte between "<tool_call>" and
"</tool_call>" (sdk/kronk/parsers/qwen/state_machine.go:88-89). The
StateMachine contract has no drain (sdk/kronk/model/parser.go:73-80),
finishSlot flushes only s.utf8Buf (sdk/kronk/model/batchgen_finish.go:261-278)
and the deferred s.reset() calls stateMachine.Reset()
(sdk/kronk/model/batchgen_slot.go:379-381).

Measured mechanism (see the usage columns above): the cap does NOT cut generation
mid-buffer at all — every buffered token classifies as ChannelNone and
sdk/kronk/model/batchgen_tokens.go:200-203 returns BEFORE the budget check at
:207, so max_tokens=4 still produced 39 completion tokens. The cut instead lands
on the single Result that carries the WHOLE call (the closing tag,
sdk/kronk/parsers/qwen/state_machine.go:80-89), and :207-210 finishes the slot
BEFORE the store at :213-222. s.toolFlag is still 0 (ChannelNone never
increments it, :177-180), so finishSlot does not even enter the tool branch at
sdk/kronk/model/batchgen_finish.go:282: no tool call, no content, no log line,
no error.

llama.cpp cannot lose it: slot.generated_text is authoritative and
send_final_response ships the remainder
(.extras/llama.cpp/tools/server/server-context.cpp:1868-1898), reported with
finish_reason "length" (.extras/llama.cpp/tools/server/server-task.cpp:443,492).
Kronk has no "length" finish reason at all
(sdk/kronk/model/models.go:36-38, :926-929), so the caller cannot even tell the
turn was cut short. upstream silent-empty cases: %v.

Transcripts: %s`, kronkSilent, upstreamSilent, dir)
	}

	switch {
	case upstream == nil:
		if len(kronkSymptoms) > 0 {
			t.Errorf(`Kronk showed %d agentic-loop symptom(s) but no upstream reference leg ran,
so this cannot be attributed to Kronk rather than to the model. Re-run with
llama-server available (KRONK_MTP_DIFF_SERVER).

Transcripts: %s`, len(kronkSymptoms), dir)
		}

	case len(kronkSymptoms) > 0 && len(upstreamSymptoms) == 0:
		t.Errorf(`DEFECT IS IN KRONK. Upstream llama.cpp completed the identical agentic tool
loop on the same GGUF, quant, build and geometry with no symptoms, while Kronk
showed %d:

  %s

Transcripts: %s`, len(kronkSymptoms), strings.Join(kronkSymptoms, "\n  "), dir)

	case len(kronkSymptoms) > 0 && len(upstreamSymptoms) > 0:
		t.Logf(`MODEL (at least partly). Upstream shows the symptom too, so the agentic
derailment is not created by Kronk. kronk %d symptom(s), upstream %d.

Transcripts: %s`, len(kronkSymptoms), len(upstreamSymptoms), dir)

	default:
		t.Logf(`Neither leg abandoned the audit. The agentic probe did not provoke the
reported symptom; §12a above is judged separately.

Transcripts: %s`, dir)
	}
}

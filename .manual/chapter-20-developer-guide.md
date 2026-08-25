# Chapter 20: Developer Guide

## Table of Contents

- [20.1 How to Use This Guide](#201-how-to-use-this-guide)
- [20.2 Task-to-Owner and Verification Map](#202-task-to-owner-and-verification-map)
- [20.3 Repository Ownership Map](#203-repository-ownership-map)
- [20.4 Developer Setup and Daily Commands](#204-developer-setup-and-daily-commands)
- [20.5 Request and Model Lifecycle](#205-request-and-model-lifecycle)
- [20.6 Core Inference Invariants](#206-core-inference-invariants)
- [20.7 Server, BUI, and Generated Documentation](#207-server-bui-and-generated-documentation)
- [20.8 Bucky Implementation Map](#208-bucky-implementation-map)
- [20.9 Verification for LLM Agents](#209-verification-for-llm-agents)
- [20.10 CI, Release, Containers, and Nix](#2010-ci-release-containers-and-nix)
- [20.11 Change and Release Checklists](#2011-change-and-release-checklists)

---

### 20.1 How to Use This Guide

This chapter is a durable orientation guide for contributors and coding agents. It
describes ownership boundaries, lifecycle contracts, and the smallest useful checks
for common changes. It intentionally does not narrate every function, reproduce
private structures, or freeze today's source layout at the individual-file level.

#### 20.1.1 Source-of-truth hierarchy

When sources disagree, use this order:

1. **Applicable `AGENTS.md` files.** Read the root instructions and every scoped
   instruction file governing the path being changed. A deeper file overrides or
   supplements broader guidance for its subtree.
2. **Current source and tests.** Interfaces, call sites, focused tests, and generated
   code establish actual behavior. For generated documentation, the authored input and
   generator are authoritative over checked-in output from an older generation.
   Confirm assumptions in code before editing.
3. **Makefiles and GitHub workflows.** These establish supported build, generation,
   CI, packaging, and deployment procedures.
4. **This chapter.** It provides background and navigation, not a replacement for
   scoped instructions or source inspection.

Treat names and defaults in this chapter as wayfinding aids. Before relying on an
exact flag, timeout, model capability, or API signature, inspect its current owner.
Do not expand a task merely to make the repository resemble this overview.

#### 20.1.2 A productive agent loop

For most work, the safest loop is:

1. Identify the public behavior being changed and its owning package.
2. Read that package's scoped instructions, implementation, focused tests, and direct
   caller. Avoid broad repository scans when a precise search will answer the question.
3. Write down the invariants that must remain true: resource ownership, cancellation,
   mutation semantics, compatibility, generated artifacts, and error translation.
4. Make the smallest coherent change in the owner. Do not duplicate policy in a
   transport or facade when the lower layer already owns it.
5. Format and run package-scoped static checks, then focused tests. Regenerate only
   artifacts derived from changed sources.
6. Review the diff for accidental generated-file edits, private data in logs,
   unrelated formatting, and stale documentation.

### 20.2 Task-to-Owner and Verification Map

The following table is a starting point, not permission to skip local instructions.
“Focused verification” means the narrowest package or command that exercises the
change. Go commands require the environment described in [§20.9](#209-verification-for-llm-agents).

| Task | Primary owner | Read adjacent | Focused verification |
| --- | --- | --- | --- |
| CLI command, flag, or output | `cmd/kronk/` (command wiring is under its command tree) | corresponding `sdk/tools/` manager and server tool route when local/web modes share behavior | `go test` for changed command package; `go install ./cmd/kronk`; invoke the one command with harmless arguments |
| HTTP endpoint or response shape | `cmd/server/app/domain/<domain>app/` | that domain's `route.go`, `cmd/server/foundation/web/`, public SDK method, and service composition | domain package tests; relevant service API test; build `./cmd/server/...` |
| Server startup or dependency wiring | `cmd/server/api/services/` | `cmd/server/app/`, config tests, embedded assets | service package tests and `go build ./cmd/server/...` |
| Middleware, request context, errors, or tracing | `cmd/server/foundation/web/` | all route registration using the middleware | foundation tests plus one affected domain test |
| Public language-model SDK behavior | `sdk/kronk/` | `sdk/kronk/model/`, examples, generated SDK docs | focused `sdk/kronk` tests and compile an affected example when useful |
| Batch scheduling, KV state, IMC, media, sampling, or speculative inference | `sdk/kronk/model/` | yzma boundary, `sdk/kronk/tests/` suite definitions, observability | focused unit tests in `sdk/kronk/model`; model-backed integration suites are human/CI work |
| Tool/reasoning parser | `sdk/kronk/parsers/<family>/` and registry contract in `sdk/kronk/model/` | parser registration and model-family selection | parser-family tests and registry tests |
| Multi-model loading or eviction | typed owners `sdk/kronk/pool/`, `sdk/bucky/pool/`; shared mechanics `sdk/pool/engine/`; app facade `sdk/pool/` | resource-manager APIs and server composition | engine eviction tests, typed-pool tests, failed-load and budget tests |
| Resource accounting | shared resource manager used through `sdk/pool/engine/` | each typed loader's plan/display/unload methods | synthetic budget/reservation tests; never rely on host memory alone |
| Bucky handle, transcription, or stream | `sdk/bucky/` and `sdk/bucky/model/` | `sdk/bucky/pool/`, audio route, Chapter 18 for user behavior | package unit tests and focused `sdk/bucky/tests/transcribe` tests where dependencies are available |
| Bucky libraries/models tooling | `sdk/tools/bucky/` | `cmd/kronk/bucky/`, server `toolapp` routes | tooling package tests and a harmless `--local` listing command |
| Malina handle or image generation | `sdk/malina/` and `sdk/malina/model/` | `sdk/tools/malina/`, examples, Chapter 19 for user behavior | package unit tests and compile affected examples; model-backed generation is deliberate human work |
| Malina libraries/model-bundle tooling | `sdk/tools/malina/` | `sdk/malina/`, curated bundle catalog, examples | tooling package tests and catalog drift tests; do not download multi-gigabyte models merely to verify tooling |
| General library/model/catalog/device tooling | `sdk/tools/` | local CLI and `toolapp` web wrappers | changed package tests; compare local and default web behavior where both exist |
| BUI page or API client | `cmd/server/api/frontends/bui/` | scoped component instructions and matching HTTP route | `npm run build` from the BUI directory; server embedding check |
| Manual source | `.manual/` | docs manual generator and generated `DocsManual` | `make kronk-docs`, then BUI build |
| SDK/example docs generation | `cmd/server/api/tooling/docs/` plus source Go docs and `examples/` | generated BUI docs components | `make kronk-docs`; inspect generated diff; BUI build |
| Linux CI | `.github/workflows/linux.yml`, `.github/actions/setup-kronk/`, `.github/test-models.txt` | make targets and version-check scripts | syntax/action review; run changed scripts locally; package checks represented by changed job |
| Release | `.github/workflows/release.yaml`, `.goreleaser.yaml`, `.release/`, version scripts | `sdk/kronk` version constant and tag convention | version scripts and GoReleaser snapshot/check when appropriate |
| Container image | `.github/workflows/docker.yml`, `zarf/docker/` | entrypoint, native-library combinations, release tags | build the affected target/variant; workflow is authority for matrix and signing |
| Nix development/package data | `zarf/nix/flake.nix` | Go module files and setup hook | evaluate/build the relevant Nix entry point; regenerate gomod2nix data when dependencies change |

Tests close to an owner are usually more diagnostic than a repository-wide command.
If a change crosses rows, verify each changed contract rather than choosing only the
largest command.

### 20.3 Repository Ownership Map

#### 20.3.1 Commands and server

- **`cmd/kronk/`** owns the installed `kronk` executable, command hierarchy, flags,
  local-versus-server dispatch, terminal presentation, and process control. Commands
  should orchestrate reusable managers rather than become a second implementation of
  catalogs, downloads, inference, or authentication. `make install-kronk` is simply
  `go install ./cmd/kronk`; there are no project build tags required for installation.
- **`cmd/server/api/`** composes executable services, startup configuration, embedded
  assets, and tooling binaries. `cmd/server/api/services/kronk/main.go` is the Kronk
  service composition root and the BUI embedding owner.
- **`cmd/server/app/domain/`** owns HTTP domain behavior. Packages such as `chatapp`,
  `respapp`, `embedapp`, `rerankapp`, `audioapp`, `toolapp`, and `playgroundapp`
  register routes and translate HTTP requests to application/SDK calls. Keep protocol
  validation and response formatting near the domain; keep inference policy in SDKs.
- **`cmd/server/app/`** also contains application wiring and adapters shared by
  domains. It is below service startup and above reusable SDK packages.
- **`cmd/server/foundation/`** owns cross-cutting server infrastructure. `web/` owns
  request lifecycle, context, middleware, error/response writing, and transport-level
  tracing; `logger/` owns server logging primitives. Domain packages should use these
  facilities rather than invent parallel conventions.

#### 20.3.2 Language-model SDK and engine

- **`sdk/kronk/`** is the public language-model handle and API surface. A
  `kronk.Kronk` owns **one primary loaded model** and a semaphore governing admission
  to that handle. Its low-level model may also own draft or MTP resources. The handle
  exposes chat, streaming, Responses, embeddings, reranking, tokenization, model
  information, and unload behavior. It is not a model pool.
- **`sdk/kronk/model/`** owns low-level llama/yzma inference: model/context creation,
  prompt planning, batch slots, sequence IDs, prefill/decode, media handling, IMC,
  samplers, parser interfaces, and draft/MTP behavior. Changes here must preserve
  cross-slot isolation and native-resource cleanup on every exit path.
- **`sdk/kronk/parsers/`** contains model-family parser plug-ins. Family packages own
  streaming state machines and extraction/normalization of reasoning and tool calls.
  The model package owns the registry contract and selection boundary; parser packages
  should not reach into batch-engine state.
- **`sdk/kronk/observ/`**, `sdk/kronk/kvstorage/`, `sdk/kronk/vram/`, `sdk/kronk/gguf/`,
  and `sdk/kronk/hf/` own their named concerns. Prefer these boundaries to embedding
  format, storage, or resource calculations in request handlers.

#### 20.3.3 Pools and shared resources

- **`sdk/pool/`** is the server-facing application facade over the language and audio
  typed pools and their shared resource manager. It coordinates backends; it does not
  replace either SDK handle.
- **`sdk/kronk/pool/`** adapts Kronk model discovery, planning, loading, display, and
  unloading to the generic pool engine.
- **`sdk/bucky/pool/`** performs the equivalent work for Whisper/Bucky models.
- **`sdk/pool/engine/`** owns typed cache mechanics: acquisition coalescing, admission,
  idle-entry selection, expiry, invalidation, and eviction callbacks. The shared
  resource manager owns RAM/VRAM reservations and budget decisions across backends.
  Eviction is therefore budget-aware and constrained by active use and releasable
  reservations; it is not a simple model-ID LRU.

The service supplies shared-pool settings to both typed pools. A typed pool used
independently may therefore have different fallback values. Check the composition
layer before documenting effective server behavior, and check the typed constructor
before documenting standalone behavior.

#### 20.3.4 Bucky, Malina, tools, UI, examples, and deployment

- **`sdk/bucky/`** is the public concurrent audio handle. **`sdk/bucky/model/`** owns
  whisper context/state operations, decoding, transcription, and stream mechanics.
- **`sdk/malina/`** is the experimental public image-generation handle.
  **`sdk/malina/model/`** owns stable-diffusion model configuration, native context
  lifecycle, generation, and Motion-JPEG encoding. Malina is not yet part of the model
  server or shared model pool.
- **`sdk/tools/`** owns reusable catalog, downloader, backend, device, diagnostics,
  library, and model-management operations used by CLI and server tools. Bucky-specific
  installers and catalogs live below `sdk/tools/bucky/`; Malina's native-library
  manager and curated diffusion bundles live below `sdk/tools/malina/`.
- **`cmd/server/api/frontends/bui/`** is the React/TypeScript browser application.
  Component-level conventions are deliberately delegated to the applicable
  `AGENTS.md`; this guide does not duplicate them.
- **`examples/`** is a separate Go module of runnable public-SDK examples and a source
  for generated documentation. Keep examples public and instructional: do not import
  internal server implementation to make them convenient.
- **`.manual/`** contains authored manual chapters. The docs tool converts manuals,
  public SDK documentation, and examples into BUI components.
- **`zarf/`**, `.github/workflows/`, `.goreleaser.yaml`, and `.release/`
  own deployment, reproducibility, and release automation. Runtime image behavior must
  agree with the Docker workflow and entrypoint, not with an old prose inventory.

### 20.4 Developer Setup and Daily Commands

From the repository root, install the CLI with:

```shell
go install ./cmd/kronk
```

The Make target is a convenient alias:

```shell
make install-kronk
```

Configure repository hooks and optional development tooling with:

```shell
make setup
make install-gotooling
make install-tooling
```

`make setup` configures the repository's hook workflow. Tool installation is separate;
inspect the Make targets before running them on a platform where package-manager
changes are undesirable. The pre-commit hook regenerates the manual and BUI and, when
`gomod2nix` is installed, refreshes `zarf/nix/gomod2nix.toml`. It stages only the
generated BUI source/bundle paths and the Nix dependency file; it does not run
`git add -A` or stage unrelated working-tree changes. Review the resulting staged diff
before committing, especially when committing only part of the worktree.

Common service commands include:

```shell
make kronk-server
make kronk-server-detach
make kronk-server-logs
make kronk-server-stop
```

Native llama and Whisper libraries and test models are large external prerequisites.
Use the CLI and Make targets appropriate to the focused test rather than downloading
every supported artifact. The Bucky CLI uses `--local` for direct filesystem work;
web/server operation is the default and there is no `--web` flag.

#### 20.4.1 Native-library compatibility and SDK initialization

The versions in `go.mod`, `sdk/tools/libs` defaults, and the README compatibility
matrix describe one tested Kronk/Yzma/llama.cpp set. The Kronk 1.30.0 line uses Yzma
v1.22.0 with llama.cpp b10212 or newer. A dependency update is incomplete unless the
binding, pinned native build, root and examples modules, generated Nix module data,
and focused model tests remain aligned. Do not update Yzma or select a newer
llama.cpp build independently merely because it is available upstream.

Runnable language-model examples should resolve and initialize the same runtime they
install:

```go
lib, err := libs.New(libs.WithDetect(ctx, kronk.FmtLogger))
if err != nil {
    return err
}

if _, err := lib.Download(ctx, kronk.FmtLogger); err != nil {
    return err
}

if err := kronk.Init(kronk.WithLibPath(lib.LibsPath())); err != nil {
    return err
}
```

Call `kronk.Init` after runtime detection and installation but before constructing a
Kronk model. Passing the manager's resolved `LibsPath` is required: calling bare
`kronk.Init()` can repeat default selection and load a different bundle from the one
the example just verified. Initialization is process-wide and should occur once.

Bucky examples follow the same sequence with `sdk/tools/bucky/libs`, `bucky.Init`,
and `bucky.WithLibPath(lib.LibsPath())`. Keep library installation separate from model
download in both example families so failures identify the correct dependency.

The exact development toolchain is pinned by `.go-version`, while `go.mod` declares
the minimum language version. Patch versions may differ, but major and minor must
match. The workflow version script enforces this relationship; read both files rather
than copying their current values into new documentation.

#### 20.4.2 Server adversarial probes

`.tools/adversarial/adversarial.sh` is an adversarial probe harness that exercises a running
Kronk server's HTTP API: constrained decoding, the MTP/speculative decode path, the
Anthropic and Responses translations, logprobs, and parameter validation. It treats
"returned 200 and quietly ignored what was asked" as a defect, and a probe that cannot
assert an invariant reports `NO SIGNAL` rather than `PASS`.

```shell
make test-adversarial
```

The target runs the default `deep` tier, which needs a real generation-capable model
and takes tens of minutes. Pass arguments through `ARGS`:

```shell
make test-adversarial ARGS="--tier=smoke"        # contract probes only, minutes
make test-adversarial ARGS="--tier=all"          # deep plus soak and TTL eviction
make test-adversarial ARGS="stream structured"   # only these probe groups
make test-adversarial ARGS="-l"                  # list groups and their tiers
```

The script probes an existing server by default. Set `SERVER=1` to have the script
start and stop its own server. `HOST`, `MODEL`, `THINK`, `TIMEOUT`, `SERVER_CMD`, and
`KEEP_ALL` are the other tuning knobs; run `.tools/adversarial/adversarial.sh -h` for the full
list and current defaults.

Results land under `.tools/adversarial/output` (ignored by Git, overridable with `OUT`):
`findings.txt` holds a result line per probe, a verdict block, and the full
request/response of every flagged call, and `kronk-server.log` holds the server's own
log when the script manages the server. Exit status is `0` when nothing is flagged,
`1` when there are findings, and `2` when the run could not proceed.

This is a deliberate human/integration run, not part of `make test` and not an agent
default — see [20.9.1](#2091-required-go-post-edit-sequence).

##### Triaging the findings

A finding is a flagged probe, not a proven defect: the probe itself can be wrong, and
the cause may sit in yzma or llama.cpp rather than in Kronk. `make test-adversarial` prints
a triage prompt after every run — including the exit-1 run that produced findings —
which hands the report to a coding agent for verification. The prompt lives in
`.tools/adversarial/adversarial-triage.md`; it is reproduced here as the reference for what
that triage must cover:

```text
Triage the adversarial run into a verified bug report.

Input:
- `.tools/adversarial/output/findings.txt` — probe results, verdict block, flagged calls
- `.tools/adversarial/output/kronk-server.log` — server + llama.cpp log
- `.tools/adversarial/adversarial.sh` — probe source

Source code, in ownership order:
1. Kronk — `cmd/server/app/domain/*app/`, `cmd/server/foundation/web/`,
   `sdk/kronk/model/` (batch, sampling, grammar, IMC, MTP), `sdk/kronk/parsers/<family>/`
2. yzma — `.extras/yzma/pkg/`, `.extras/yzma/lib/`
3. llama.cpp — `.extras/llama.cpp/src/`, `.extras/llama.cpp/common/`, `.extras/llama.cpp/ggml/`

Steps:
1. Candidates = every flagged probe + every NO SIGNAL. Dedupe by root cause.
2. Verify each independently. Subagents optional — one per candidate when there are
   many. Each verification:
   - Read the flagged call's request/response in `findings.txt` and its probe in
     `adversarial.sh`; confirm the probe asserts a real invariant.
   - Correlate by timestamp with `kronk-server.log`.
   - Trace to owning code, cite `file:line`, name the layer. Follow Kronk → yzma →
     llama.cpp when the cause is not in Kronk.
   - Verdict: CONFIRMED (code path proves it) | EXPECTED (server right, probe or spec
     reading wrong — say which) | UNPROVEN (not pinnable to code).
   - Uncertain → EXPECTED or UNPROVEN. No defect without a mechanism shown in source.
3. Report CONFIRMED only.

Output — bullets, no prose, no preamble:

- **<symptom>** — `<probe-id>` · `path/file.go:LINE` (kronk|yzma|llama.cpp)
  - Expected: <invariant> · Actual: <observed>
  - Cause: <one sentence, cites code path>
  - Repro: <single command from findings.txt>

Then one line per non-confirmed flag: `Dismissed: <probe-id> — <reason>`.
```

The yzma and llama.cpp trees the prompt cites are working checkouts under `.extras/`,
which is untracked. Clone them there before triaging, or drop those two sources from
the prompt and accept that a defect below the Kronk boundary can only be localized to
the binding call site.

Editing the prompt means editing `.tools/adversarial/adversarial-triage.md` and this
chapter together; the file is what the Make target prints.

#### 20.4.3 OpenCode coding benchmark

`.tools/codegen-benchmark` is a deliberate coding-agent benchmark against a separately
managed Kronk server. It runs the tic-tac-toe prompt from
`examples/talks/tic-tac-toe.md` through `opencode run`, preserving an isolated OpenCode
home and nested Git repository for each attempt. The repository boundary keeps OpenCode
inside the attempt directory, where it can compile and repair its program before the
harness applies hidden structural tests and scripted gameplay checks.

The harness warms each model with a minimal `hello model` chat completion before its
attempts, then unloads that model before continuing to the next configured model. Warm-up
time and tokens are excluded from the OpenCode benchmark measurements.

Eligible models come exclusively from `zarf/kms/model_config.yaml`: every selected ID
must be configured there, end in `/AGENT`, and be returned by the running server's
`GET /v1/models` endpoint. The harness defaults to one attempt per eligible model.

```shell
make benchmark-codegen-list
make benchmark-codegen
make benchmark-codegen \
    CODEGEN_MODEL=ornith-ai/Ornith-1.5-35B-Q8_0/AGENT \
    CODEGEN_ATTEMPTS=3
```

Results land directly beneath `.tools/codegen-benchmark/output`. Each run clears the
previous contents, stores every selected model beneath that directory, and writes one
aggregate `report.md` for model comparison. Each attempt retains the generated program,
OpenCode state and event stream, and grader result. See
`.tools/codegen-benchmark/README.md` for all runtime knobs. This is not part of
`make test`; starting the model server and running inference remain explicit human
actions.

### 20.5 Request and Model Lifecycle

#### 20.5.1 Server request flow

The stable request path is:

```text
HTTP request
  -> foundation/web middleware and request context
  -> domain route validation and protocol translation
  -> sdk/pool facade
  -> typed pool acquisition (Kronk or Bucky)
  -> one-model public handle admission semaphore
  -> model engine/state execution
  -> SDK result or stream
  -> protocol response
```

Every arrow is an ownership boundary. Middleware owns transport concerns. Domains own
HTTP compatibility. Pools own model residency and resource tickets. Handles own
per-model admission and shutdown coordination. Engines own native contexts, sequences,
and inference state. Preserve error identities long enough for the domain layer to map
capacity, cancellation, validation, and internal failures correctly.

#### 20.5.2 Typed pool acquisition, loading, and eviction

An acquisition first checks/coalesces a typed cache entry. A cold load is planned by
the backend loader, then reserved against the shared memory manager before expensive
native loading. The loaded handle becomes visible only after initialization succeeds.
Failed planning or loading must release its reservation and must not publish a partial
cache entry. Concurrent acquisition of the same key should share load work rather than
multiply memory commitments.

Item count and available RAM/VRAM are independent admission constraints. Normal
admission-driven eviction selects only idle handles; the pool observes each handle's
active-operation counter rather than owning a separate request lease. Explicit
invalidation and shutdown may remove an active entry and rely on the handle's `Unload`
method to drain active work according to its context.

Asynchronous invalidation does not imply that its eviction callback has finished.
Synchronous invalidation waits for callback and reservation-ticket completion, but it
does not prove native unload succeeded: the callback can report an unload error and
still release the reservation. Preserve the engine-level distinction between a model
that cannot fit the configured budget and temporary pressure where no idle candidate
can be evicted; typed/public APIs may translate those errors differently.

#### 20.5.3 Semaphore lifetime and cancellation

The `Kronk` handle's semaphore is admission control around one model. A permit belongs
to the operation, not merely to function setup. For a non-streaming call, hold it until
the operation returns. For streaming, hold it until the stream is terminal or closed,
including cancellation/error cleanup. Releasing at stream construction over-admits;
forgetting to release on an early error deadlocks future work. The pool's active-use
lease similarly spans the entire externally visible operation so eviction cannot unload
native resources while output is still being consumed.

Context cancellation must propagate inward to queue waits and inference. The layer
that creates a goroutine, stream, native object, or lease owns its shutdown. Do not
close caller-owned channels or unload a caller-owned handle. Unload prevents new work,
waits for owned active work according to its context, then tears down engine/native
resources. Pool shutdown owns handles created by that pool.

#### 20.5.4 Batch slots and sequence isolation

Text generation uses a batch engine. A slot is an execution reservation and mutable
per-request state; its sequence ID partitions KV/cache operations in the shared native
context. Scheduling several slots into one decode is safe only if every token, logit
index, sampler, parser, cancellation flag, speculative buffer, media position, and KV
operation remains associated with the correct slot/sequence.

The main invariants are:

- A slot has one active job and one sequence identity at a time.
- Batch construction must preserve token-to-sequence and output-index mappings.
- Completion/cancellation removes or resets only the finishing sequence's state.
- Slot reuse starts from an intentionally clean state; no parser, sampler, media,
  speculative, or error state may leak to the next job.
- A blocked or cancelled caller must not strand a slot or semaphore permit.
- Native decode failure is attributed to affected jobs and followed by deterministic
  cleanup; it must not silently publish partly advanced session state.

#### 20.5.5 Generation versus embedding and reranking engines

Text generation benefits from shared batched execution and sequence-partitioned KV.
Embeddings and reranking never use generation slots. Proven architectures use the
separate sequence-batch engine, which schedules complete embedding inputs or
query-document pairs across sequence IDs on one context. Unsupported architectures
acquire an independent context from the fallback pool and perform their own
decode/clear cycle. Reranking must not allow one document's state to contaminate the
next. Embedding pooling and normalization are model/output concerns, not chat-slot
concerns. Do not merge the sequence-batch and generation engines merely to share code;
their lifecycle contracts differ.

Compatibility is an explicit allowlist based on `general.architecture`. Add an
architecture only after a native yzma proof, model-backed concurrent tests, and a
benchmark. Some llama.cpp failures assert instead of returning an error, so probing an
unknown architecture at runtime is not a safe fallback strategy.

### 20.6 Core Inference Invariants

#### 20.6.1 IMC sessions, slots, and external storage

Incremental Message Cache (IMC) sessions are cache identities, not execution slots.
A stable cache/session identifier allows a conversation prefix to survive movement
between batch slots. A slot is short-lived compute capacity; binding a session to a
slot would reduce concurrency and make slot reuse unsafe.

The `SessionStore` contract externalizes each session's native KV snapshot. RAM and
disk implementations differ in storage and I/O, but the model layer owns when a
snapshot is read, prepared, committed, reset, and closed. Session metadata—cached
tokens, render-sensitive identity/version, and snapshot—must describe the same prefix.
Do not update one independently and call the session valid.

The session's `reserved` state serializes mutation and hides the session from competing
selection until metadata and snapshot bytes agree. The reservation begins in prompt
planning and survives the queue wait and restore; exact read-only hits keep it through
generation, while a successfully published text build/extension can release it after
the stable snapshot is committed. Restore only a committed snapshot whose complete
token or media plan and expected version still match. Ordinary text build/extension
prepares and commits through the session's existing store; if snapshot publication
fails, invalidate that current state so later work rebuilds it rather than claiming the
old or partial state is valid.

Text planning compares complete token sequences and searches every working session for
exact or append reuse before considering System. Each model owns a separate RAM-only
System cache pool with the same capacity as its working-session pool. Entries contain
only independently rendered, nonempty, cache-worthy system prefixes whose complete
token sequences prefix the stable request. They include target KV and compatible
own-KV MTP state and never advance.

On a System hit, reserve the immutable source during restoration and copy it into a
separately selected empty or least-recently-used working session. Multiple working
sessions may preload the same System entry. On a miss, publish into an empty System
entry or replace the least-recently-used entry that is not being restored. Retention
failure must not fail inference. Never trim a working session at an internal token
match.

##### Template stability and the System cache pool

Prefix-stable templates normally append from a working session. Reasoning-aware or
look-ahead templates may retroactively change an earlier assistant or tool turn; a
matching System entry then provides a conservative restart point.

Native sequence snapshots can consume hundreds of megabytes, RAM stores retain peak
backing capacity, and own-KV MTP configurations may require matching draft state. Do
not create System entries for empty or below-threshold system-only renderings, and do
not add user-turn or progressive checkpoint roles.

Do not attempt prefix sharing by slicing or pointing into the current serialized state
buffers. The native interface exposes complete per-sequence size/get/set operations,
not partial state serialization, base-plus-delta restoration, copy-on-write KV pages,
or immutable-prefix sharing. Hybrid recurrent state makes arbitrary byte slicing
especially unsafe. Meaningful shared-prefix storage requires explicit native backend
support rather than a different Go buffer layout.

When investigating another template, correlate `match_reason`, `reusable_tokens`,
`system_boundary_tokens`, and `full_input_snapshot_messages` from `plan-ready`
records with System creation/restore and `slot-finished` events. Distinguish clean
Current appends, System recovery, full rebuilds, and actual restored-token reuse.

Media-anchor advancement has a stronger replacement contract: it writes a separately
staged store and swaps the store plus matching plan/count metadata only after success,
so failure leaves the previous media snapshot published. Do not generalize that staged
replacement guarantee to every IMC path. A `SessionStore` implementation must honor
the interface's read/prepare/commit/reset lifetime rules and clean up temporary
resources; callers must not assume bytes remain stable across the next mutation.

Restored KV and sampler history are separate correctness concerns. Prime sampler
penalties and DRY with the complete logical prompt after restore. An own-KV MTP session
may publish draft KV only with the matching target snapshot and final hidden row; a
media commit clears that draft state. Shared-target-KV MTP derives its resume point from
the restored target and must not allocate or restore an independent draft store. A
draft-only restore failure permits target-only fallback. Target-side rejection has two
distinct outcomes: a stale expected-version mismatch is a retryable busy error and
leaves the published session intact, while empty target bytes or a partial native target
restore are request-fatal and invalidate the affected current state.

Snapshot publication failures also differ by path. Ordinary text build/append and
initial media build write through the current store; incomplete target serialization
invalidates that current cache state, but target generation for the current request can
continue. Media-anchor advancement instead uses a replacement store and requires a
complete staged snapshot before swapping metadata; decode or snapshot failure fails the
request while preserving the old published anchor.

#### 20.6.2 Prompt plans: text and media

Prompt planning converts normalized messages and parameters into the exact work the
engine will execute. The plan, cache identity, token accounting, and decode positions
must agree. Text-only plans can compare rendered/tokenized prefixes directly and may
take optimized exact-hit paths. Media plans carry more than text tokens: ordered media
parts, placeholder/embedding expansion, positions, and render-affecting metadata are
part of identity and execution.

Do not treat a media prompt as text with an attachment ignored by caching. A text
prefix match is insufficient if image/audio/video content, ordering, sizing, or model
projection changes. Media prefill must align embeddings and positions with the same
sequence that receives surrounding text. A media-anchor advance stages replacement
state separately, so decode or snapshot failure leaves that anchor's prior published
snapshot authoritative. Do not claim the same preservation for an LRU rebuild: prompt
planning intentionally resets the selected session before rebuilding it.

#### 20.6.3 Parser registry ownership

Parser implementations live under `sdk/kronk/parsers/`, grouped by model family. The
registry interface and registration entry point live in `sdk/kronk/model/`. A parser
plug-in supplies factories/state machines for its advertised family and must tolerate
stream chunk boundaries: tags, JSON, reasoning delimiters, and tool arguments may span
chunks. It must keep request state per parser instance and produce equivalent logical
results for streaming and non-streaming input.

To add or change a parser, edit the family package, update registration/selection only
where necessary, and test fragmented as well as complete input. Keep generic JSON
repair separate from family recognition. Unknown families need an intentional fallback
or error; registration order must not create accidental model-family selection.

#### 20.6.4 Responses normalization

The Responses API adapts to the chat/inference pipeline in `sdk/kronk/response.go`.
Normalization has a compatibility-sensitive mutation contract:

- Preserve existing `messages`; they win when already supplied.
- Convert Responses `input` into messages **only when messages are absent**.
- Normalize Responses item/content/tool forms needed by chat processing.
- Mutate the supplied `model.D` document map. Callers that require isolation must clone
  before invoking the Responses path.

Do not “clean up” this code by always rebuilding messages or silently switching to a
copy. Either change breaks callers that combine compatibility fields or inspect the
document after normalization. Add tests for existing messages, input-only requests,
and observable in-place mutation.

#### 20.6.5 Tracing and logging

Tracing should identify major waits and ownership boundaries: request handling, model
acquisition/load, queue wait, prompt/prefill, generation, and unload when relevant.
Keep spans concise. Avoid a span per token, duplicated nested timing, giant model-config
attribute sets, prompt/media payloads, and unbounded IDs. Propagate the request context
instead of creating unrelated roots. Logs and metrics should help distinguish queue,
capacity, cancellation, and inference failures without exposing user content unless an
explicit insecure-logging mode authorizes it.

Embedding/reranking metrics distinguish the operation and selected runtime. Sequence-
batch queue wait and batch width are engine-level measurements; request duration,
active requests, status, and prompt-token usage are operation-level measurements.
Resource reservations remain owned by the shared pool/resource manager and must not be
duplicated inside either inference engine.

#### 20.6.6 Speculative decoding and MTP

Speculative support has three ownership shapes:

1. **Separate GGUF draft model.** The draft has its own model/context/KV and proposes
   tokens; the target verifies them. Loading, memory planning, sequence cleanup, and
   rollback must account for both models.
2. **Embedded MTP.** A target GGUF exposes an embedded multi-token-prediction head.
   Model detection and MTP construction are owned by `draft_mtp.go`/`batchgen_mtp.go`,
   while generic proposal verification and reconciliation remain in
   `batchgen_speculative.go`.
3. **Separate-file Gemma4/shared-target-KV MTP.** The MTP component is supplied as a
   separate file but shares target KV semantics rather than behaving like an ordinary
   independent draft model. Capabilities, not “has a draft path,” must decide whether
   draft KV can be trimmed or externalized.

Across all three, target output is authoritative. Proposal generation cannot expose a
token until target verification accepts it or chooses the replacement/bonus token.
Position counters, sampled-token history, target KV, draft/MTP state, and streamed
output must describe one accepted prefix after every round.

Verification in a multi-slot batch is explicitly read-before-mutate. First read all
target logits/hidden-state rows and decide each slot's accepted prefix while the shared
batch outputs are intact. Only then mutate KV, counters, slot buffers, stream output,
or MTP mirror state. Mutating one slot during the read phase can invalidate indices or
native output needed by another slot.

Ordinary transformer KV can often remove a rejected suffix. Hybrid recurrent/state-
space models cannot assume partial KV deletion restores prior state. Request bounded
recurrent rollback through `NRsSeq` and use it only when the context reports enough
effective rollback depth for the speculative round. Otherwise take a full pre-speculation
per-sequence snapshot, and on rejection restore it and re-decode exactly the accepted
prefix. Preserve captured target hidden-state rows needed to synchronize MTP. For own-KV
MTP, rollback removes speculative draft state before mirroring accepted target state. For
shared-target-KV Gemma4, do not apply independent-draft rollback to the shared target cache.
If rollback, restore, or synchronization fails, fail the affected slot or safely disable
MTP rather than retaining ambiguous state.

Unit-level owners are the batch/speculative files and tests in `sdk/kronk/model/`.
Model-backed MTP suites live in `sdk/kronk/tests/mtp` and
`sdk/kronk/tests/gemma4mtp`; they are CI/human suites, not commands agents should launch
from the forbidden integration-test tree.

### 20.7 Server, BUI, and Generated Documentation

#### 20.7.1 Routes, middleware, and domains

Route declarations belong with their domain package, normally in `route.go`. Keep
authentication/authorization, tracing, request IDs, panic recovery, and common response
behavior in foundation middleware. Domain handlers decode and validate protocol input,
select the appropriate application capability, call SDK/facade methods, and encode the
protocol result. They should not manipulate native model state or implement pool
eviction.

When adding an endpoint, follow a neighboring domain end to end: registration,
middleware order, request model, error mapping, streaming behavior, and service wiring.
Test malformed input and cancellation as well as success. A server build catches route
composition errors that a leaf-package test may miss.

#### 20.7.2 BUI ownership and embedding

The BUI lives at `cmd/server/api/frontends/bui/`. Follow its own package scripts and
the applicable component `AGENTS.md`; component structure and UI conventions change
more quickly than this chapter. The production bundle is embedded by
`cmd/server/api/services/kronk/main.go`. Editing TypeScript does not alter the server
binary until the frontend is rebuilt and embedded output is rebuilt into Go.

For frontend changes:

```shell
cd cmd/server/api/frontends/bui
npm run build
```

Then build the server (or the narrow service package) and verify that the expected
static bundle is present in the embedding location. Avoid hand-editing minified/static
output.

#### 20.7.3 Documentation generation

`cmd/server/api/tooling/docs/main.go` orchestrates three conceptual pipelines:

```text
public SDK Go documentation -> SDK BUI documentation
examples source             -> example BUI documentation
.manual chapter Markdown    -> DocsManual.tsx
```

Author manual content in `.manual/`, public API descriptions in Go doc comments, and
examples in `examples/`. `DocsManual.tsx` and generated SDK/example documentation are
outputs and must not be hand-edited. Run:

```shell
make kronk-docs
```

Review generated diffs for malformed Markdown conversion and then run `npm run build`
in the BUI. Finally build the server to check that generated components compile into
the embedded bundle. Generation may update more than one documented package; do not
discard legitimate generated changes. If the requested scope intentionally excludes
generated artifacts, report that regeneration remains pending.

### 20.8 Bucky Implementation Map

This is an implementation map only. Chapter 18 owns installation, configuration,
streaming usage, and API examples.

#### 20.8.1 Owners

- **`sdk/bucky/`** owns initialization and the public `Bucky` handle. A handle owns one
  Whisper model and admission/shutdown coordination.
- **`sdk/bucky/model/`** owns the Whisper context, its pool of model states, audio
  decode/transcription primitives, language operations, and stream implementation.
  Model weights/context are shared by the handle while state isolates concurrent work.
- **`sdk/bucky/pool/`** adapts Bucky model planning, loading, status, unloading, and
  reservations to the generic typed pool.
- **`sdk/pool/` and `sdk/pool/engine/`** let Bucky and Kronk share one resource budget
  while retaining backend-specific loaders and handles.
- **`sdk/tools/bucky/`** owns Whisper shared-library and model catalog/download work.
- **`cmd/kronk/bucky/`** exposes those tools. Web/server mode is default; `--local`
  requests direct local operation.
- **`cmd/server/app/domain/audioapp/`** owns the OpenAI-compatible transcription route.
  Administrative library/model routes are in `toolapp`. Service startup wires the
  Bucky backend and shared pool.

#### 20.8.2 Lifecycle invariants

`Init` registers/resolves/loads the backend. Technically, a failed `Init` can be called
again and retry. The current server calls it only during startup, however. Installing
missing libraries through CLI or BUI does **not** promise automatic server re-init;
restart the server so startup calls `Init` again.

A transcription acquires handle capacity and a model state, performs decode/inference,
then releases both on every completion path. A streaming session is longer-lived:
opening it reserves a state and capacity until its worker exits. `Close` requests the
normal final flush and waits for that exit; a terminal worker error also exits and
releases automatically. Callers should still defer the idempotent `Close`, including
when feed/event handling fails. Unload must not destroy the Whisper context while
transcriptions or streams remain active.

The state pool is sized by `Config.NSeqMax`, which defaults to 1. The admission channel
is sized by `NSeqMax + QueueDepth`; queue depth defaults to 0. Calls beyond that capacity
wait until their context is canceled, the configured admission timeout expires, or an
operation releases admission capacity. The admission timeout defaults to three minutes
and stops applying once capacity is acquired. The states isolate concurrent inference
while sharing the handle's model weights and Whisper context.

The audio HTTP handler delegates file decoding and transcription to
`Bucky.TranscribeFile`. It explicitly enforces the 25 MB upload limit before allowing
unbounded work. Keep protocol field validation/format selection in the handler and
audio/model mechanics in Bucky.

Focused tests that exist include unit tests under `sdk/bucky/model/` and
`sdk/bucky/ffmpeg/`, transcription/pool/stream suites under
`sdk/bucky/tests/transcribe/`, and the server audio API tests under
`cmd/server/api/services/kronk/tests/`. Choose the narrowest test whose native library
and model prerequisites are available. Do not duplicate Chapter 18's usage matrix here.

### 20.9 Verification for LLM Agents

#### 20.9.1 Required Go post-edit sequence

After changing Go, obey the root instructions and scope work to the changed package.
For each changed Go file/package:

```shell
go fix ./path/to/changed/package
gofmt -s -w path/to/all-changed.go
go vet ./path/to/changed/package
staticcheck ./path/to/changed/package
```

Use exact package paths rather than `./...`. If several packages changed, list those
packages explicitly or run commands separately so failures remain attributable. Review
`go fix` output/diff because it may modify additional files; include every resulting Go
file in the subsequent `gofmt` and review.

Before focused Go tests, set:

```shell
export RUN_IN_PARALLEL=yes
export GITHUB_WORKSPACE="$(pwd -P)"
```

`GITHUB_WORKSPACE` must be the absolute repository root. Then run a package test or a
specific test, for example:

```shell
go test -count=1 ./sdk/kronk/model
go test -count=1 -run 'TestSpecificBehavior' ./sdk/kronk/parsers/qwen
```

Agents must **never prescribe or run a full repository test run**, and must **never
launch tests from `sdk/kronk/tests`**. Those suites require managed libraries/models
and belong to CI or deliberate human integration runs. Commands such as `make test`
and `make test-adversarial` ([20.4.2](#2042-server-adversarial-probes)) exist as broad
human/CI-maintainer context, but they are not the agent default. Do not use a broad
command merely because focused ownership is unclear; inspect the owner.

#### 20.9.2 Choosing effective checks

- Pure logic changes: focused unit test plus package static checks.
- Public API changes: owner tests, direct dependent package build/test, and generated
  SDK docs when comments/signatures changed.
- Batch/native changes: focused model unit tests first; report model-backed validation
  as unavailable if prerequisites are absent rather than substituting unrelated tests.
- Pool changes: engine tests plus the affected typed pool's reservation/load tests.
  Include failure and cancellation, not only warm acquisition.
- Route changes: domain tests and a server build. Use the relevant API test only when
  its server/model prerequisites are available.
- CLI changes: command package tests, `go install ./cmd/kronk`, and one safe invocation.
- Bucky changes: package tests; transcription integration only with installed Whisper
  libraries/model. Streaming changes require close/cancellation coverage.
- Malina changes: package tests and affected example builds. Native generation requires
  compatible stable-diffusion.cpp libraries and multi-gigabyte model bundles, so keep
  model-backed runs deliberate and do not make them an unconditional CI requirement.

#### 20.9.3 Markdown, generated docs, and BUI

For a manual-only edit, run formatting/sanity checks appropriate to the task, including
`git diff --check`. `make kronk-docs` validates the manual conversion pipeline but also
changes generated `DocsManual`; honor any task restriction on edited files. For normal
documentation changes, commit source and generated output together, then run the BUI
build.

For BUI changes, install dependencies according to its lockfile/workflow and run:

```shell
npm run build
```

Run it from `cmd/server/api/frontends/bui/`. For docs or BUI work that affects the
production bundle, also build the server to verify the bundle embedded by
`cmd/server/api/services/kronk/main.go` exists and compiles. A successful Vite build
alone does not prove the Go binary contains current assets.

Always report what actually ran, including skipped integration prerequisites. Never
claim CI parity from a narrower local test.

### 20.10 CI, Release, Containers, and Nix

#### 20.10.1 Linux CI

`.github/workflows/linux.yml` is the authoritative Linux pipeline. It currently has
four parallel jobs:

- `static`: source/static quality checks;
- `race`: race-enabled focused coverage separated from static checks;
- `api-tests`: server/API integration coverage;
- `sdk-tests`: SDK/model integration coverage.

The shared setup action is under `.github/actions/setup-kronk/`. CI model dependencies
are declared in `.github/test-models.txt`; its contents also participate in cache
behavior. When a CI test gains a required model, update the manifest with the correct
backend and model ID and check the setup action's parser. Keep local human setup in
sync where the Make workflow maintains a separate install list.

The exact CI toolchain comes from `.go-version`, while `go.mod` declares the minimum
language version. Their major/minor versions must match. Update workflow assumptions
and run `.github/scripts/check-go-version.sh` when changing either.

#### 20.10.2 Release

The release workflow, GoReleaser configuration, scripts, and release notes divide
responsibility:

- `.github/workflows/release.yaml` owns trigger, permissions, setup, checks, and release
  execution.
- `.goreleaser.yaml` owns binary/archive packaging and related release products.
- `.github/scripts/check-version.sh` enforces the release identity.
- `sdk/kronk/kronk.go` owns the exported `Version` constant.
- `.release/` owns maintained release-note/checklist material.

The release tag must equal `v` plus the `Version` constant. Update the constant and
release material intentionally before creating the tag; do not bypass the guard. Also
confirm the Go major/minor guard, generated docs/BUI, clean tree, and relevant Linux
jobs before tagging.

#### 20.10.3 Containers

`.github/workflows/docker.yml` is authoritative for image variants, target/platform
matrix, registry publication, attestations, and signing. `zarf/docker/` owns Dockerfile,
runtime configuration, and entrypoint behavior. Native llama and Bucky processor
availability can differ by image variant, so inspect the workflow matrix and tooling
combination tables before changing an image. Avoid copying a variant table into docs;
it becomes stale faster than the workflow.

For a container change, build the affected target and architecture where practical,
exercise entrypoint startup/configuration, and verify expected native libraries. Do not
infer publication or signature behavior from a local build; review the workflow.

#### 20.10.4 Nix

The flake at `zarf/nix/flake.nix` defines how developers/users enter or build the
project; generated Go dependency data lives beside it. Entering a development shell
runs `gomod2nix import` from its shell hook and may dirty that generated material. When
Go module dependencies change, update the Nix dependency material with the repository's
configured command and evaluate/build the relevant entry point. Keep Nix fixes in Nix
owners rather than adding environment special cases to Go code.

### 20.11 Change and Release Checklists

#### 20.11.1 Focused change checklist

- [ ] Read all applicable `AGENTS.md` files.
- [ ] Locate the owning package, direct caller, and focused tests.
- [ ] State lifecycle/mutation/resource invariants before editing.
- [ ] Change the owner rather than duplicating logic in a facade or transport.
- [ ] Preserve cancellation and release behavior on every return path.
- [ ] For Go: run `gofmt -s`, `go fix`, `go vet`, and `staticcheck` scoped to changed
      files/packages.
- [ ] Set `RUN_IN_PARALLEL=yes` and absolute `GITHUB_WORKSPACE` for focused tests.
- [ ] Do not run a full repository suite or launch tests from `sdk/kronk/tests` as an
      agent.
- [ ] Regenerate docs/BUI/Nix artifacts only when their sources changed and task scope
      permits it.
- [ ] Run `git diff --check` and inspect the complete diff for unrelated changes.
- [ ] Report commands, results, skipped prerequisites, and residual uncertainty.

#### 20.11.2 Release checklist

- [ ] Choose the release version and update `sdk/kronk`'s `Version` constant.
- [ ] Ensure the intended tag is exactly `v<Version>` and run the version guard.
- [ ] Confirm `.go-version` and `go.mod` major/minor agreement and run the Go-version
      guard.
- [ ] Update release notes/changelog material under the repository's release process.
- [ ] Ensure `.github/test-models.txt` covers model-backed CI requirements.
- [ ] Regenerate documentation and BUI assets; build the BUI and server embedding.
- [ ] Confirm focused package checks and the four Linux CI jobs are green.
- [ ] Run `make test-adversarial` against a generation-capable model on a machine that has
      one, and review `.tools/adversarial/output/findings.txt`.
- [ ] Review `.github/workflows/docker.yml` for the intended image variants and
      publication/signing behavior.
- [ ] Review Nix dependency outputs if Go dependencies changed.
- [ ] Verify GoReleaser configuration with an appropriate non-publishing check or
      snapshot.
- [ ] Confirm the release commit is clean, then create/push the guarded tag through the
      maintainer release process.

The purpose of these lists is to protect ownership and lifecycle contracts, not to
turn every patch into a release. Use the focused list for ordinary work and reserve
broad integration/release machinery for humans and CI with the required models,
native libraries, credentials, and platforms.

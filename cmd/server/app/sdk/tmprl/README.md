# Jobs, stages, and steps

The new implementation lives in [`jobs/training`](jobs/training/README.md).
It uses foundation's runner directly, with no dependency on `jobs/modeltraining`.
The latter remains a reference while the new job is reviewed. The Reference
implementation section below describes that older implementation, not the new
job's public API.

## Current application boundaries

- `datasets/` is the application service for dataset management and preparation
  runs: submission, preview review, shared run list/read/stop and prepared-output
  downloads. It calls application jobs and interprets saved dataset readiness.
- `datasets/store/` owns immutable dataset files, safe publication, concurrency
  guards and dataset-ID validation. Both datasets and jobs use this lower-level
  package; it imports neither the datasets service nor the whole-job coordinator.
  It reuses the training stage's record schema and validation.
- `jobs/training/` owns plan/model/review/control rules, execution observations,
  report/adapter readiness and ETA. It calls foundation for execution and uses
  `datasets/store` to save results, never the parent datasets service.
- `app/domain/moxieapp` owns HTTP authorization, decoding, conversion, polling
  response timing/revisions, and downloads. Preparation follows HTTP → datasets →
  Jobs → foundation/Temporal. Training authorization, evaluation and adapter
  endpoints remain on the training path. Existing mixed run URLs are unchanged;
  HTTP neither constructs services nor coordinates the low-level runner.
- `api/services/kronk/main.go` retains the existing `.moxie` startup and injects
  uploads, datasets, and application jobs. No second storage root is initialized.
- Foundation still owns actual execution, retries, controls, recovery, and history.
  Application errors preserve backend causes without requiring HTTP to interpret
  Temporal service errors.

Dataset files remain in `.moxie/preparation-datasets` with their existing format.
Both datasets and jobs call SQL-only `moxiedb` directly for publication metadata;
jobs never persist through the parent datasets service. All screens, HTTP
contracts, preparation/training stages and explicit training authorization remain.

## Dataset catalog initialization and recovery

`KRONK_MOXIE_ROOT` (YAML `kms.moxie.root`) selects the existing Moxie root,
defaulting to `~/.moxie`. Uploads, dataset snapshots and execution storage share
this one root. Moving the complete root preserves relative references.
`KRONK_MOXIE_DATABASE_URL` (YAML `kms.moxie.database-url`) explicitly enables the
SQL catalog. Configuration logging masks the entire connection string. An unset
URL retains file-only compatibility; an unavailable configured database is an
error, never a reason to switch authorities or accept uncataloged writes.

Initialization has two distinct owners:

1. The existing one-line `make tuning-up` runs the explicit Compose SQL initializer
   against `moxiedb`, alongside the existing PostgreSQL-backed Temporal setup.
   It installs `moxiedb/migrations/001_init.sql` transactionally on an empty schema,
   serializes competing initializers and records its SHA-256. Compatible starts
   skip installation/upgrades. Unknown, damaged or incompatible schemas fail
   without changing application tables. There are no separate maintenance targets.
2. Normal application startup opens/checks SQL without creating tables, constructs
   the existing file store and training job, then calls `datasets.Initialize`.
   That service verifies all snapshots, reconciles publication metadata and builds
   the generated `.index.yaml` before any worker starts or routes accept requests.
   Worker shutdown precedes Temporal-client and database closure.

For cutover, stop old file-only writers first, retain backups of both Moxie files
and PostgreSQL, run `tuning-up`, and configure SQL on every publisher using the same
root. Do not run file-only and SQL publishers concurrently. Explicit schema
upgrades are not implemented; a mismatch is a stop-and-inspect condition, not an
invitation to delete tables or edit migration checksums.

Snapshots retain actual generated records and historical contributions. SQL stores
only dataset/version identity, relative paths from the Moxie root, digests, counts,
readiness, contribution metadata and durable publication intents. `.index.yaml`
is a rebuildable local file inventory, following Kronk's atomic temporary-file and
rename pattern. Its paths are relative to `preparation-datasets`; it contains no
records or job statuses and cannot authorize an SQL-unpublished version. Lookup
validates the selected file and rebuilds missing, malformed or stale indexes.

Create/remove are coordinated by datasets; verified preparation additions are
coordinated directly by the training job. Each persists an operation intent,
publishes a complete immutable snapshot, verifies it, commits SQL metadata/head
atomically, then refreshes the index. A cross-process file lock covers that
sequence and is released by the OS on process exit. Stable add identities recover
lost receipts without duplicating contributions. Removal keeps strict base-version
checks; concurrent additions preserve their existing merge semantics.

Initialization imports verified historical versions idempotently and completes
matching intents whose files were published before interruption. Missing/corrupt
published files, changed identities/digests, or unresolved intents are reported;
they are never reset or silently replaced. An intent without a verified snapshot
is marked `needs_attention` and blocks initialization for inspection. Index failure
after SQL commit explicitly reports the saved dataset/version, and replay or
initialization can rebuild it without repeating an addition. Arbitrary repeated
HTTP create/remove requests are not newly claimed to be idempotent.

## Job catalog reads and reconciliation

In SQL mode the existing job writes `jobs`, `job_attempts` and `job_intents` directly.
The host starts and joins its reconciler independently of browser requests.
Temporal owns execution; the catalog retains exact links, ordered confirmed
observations and pending commands. SQL revisions protect writes but do not
establish Temporal event order. Source timestamps and terminal-state guards
prevent stale observations from regressing confirmed results.

`datasets.Service` reads filtered catalog history directly from SQL without a
worker. The existing GET `/v1/training/runs` accepts `mode`, `target_dataset_id`
and repeated `status` parameters. Filters apply before deterministic pagination;
tokens are bound to the namespace, filters and page size. Detail reads require
the exact workflow/run identity, attempt a live read when available, and retain
stale persisted state on execution-service loss. Commands still require the job.
Public responses add job/attempt identity, observation revision/time and pending
action summaries, never command payloads or internal paths. Saved dataset output
is checked against its exact published version, not a newer head.

### Preparation checkpoint suspension

For new SQL-backed preparation-only runs, ordered Pause is a request until the
active activity saves its completed unit and exits at a safe point. Only then
does Temporal confirm Paused and the existing reconciler retain that observation
in Moxiedb. Continue re-enters the same step and loads its durable `.moxie`
checkpoint; it never bypasses preview review or authorizes Training. Stop is
terminal cancellation, not a temporary Pause. Legacy and training behavior is
unchanged. No migration, application-startup initialization, or maintenance command
is added by this mechanism.

The supported units, drain and crash evidence are documented in
[`jobs/training/README.md`](jobs/training/README.md#preparation-safe-point-suspension-milestone-3b2).
Extraction recovers at whole-document boundaries (an interrupted Docling
conversion runs again); generation/evaluation/verification and preview
checkpoints resume from their saved progress. Restart requires the same durable
uploads, execution workspaces, PostgreSQL and Temporal history plus required model
assets. Idle preparation runs (paused, awaiting review, drained) wait without a
time limit; the Preparation budget counts active time only. Milestone 3B.3 proves
process-kill recovery on isolated services; it does not establish physical
power-loss durability or training recovery.

File-only callers retain runner visibility and existing execution behavior.
SQL failure never silently switches catalog authorities. The sample Preparation
runs grid remains unchanged until milestone 4 connects it.

SQL tests require explicit isolated-test configuration. `MOXIEDB_TEST_URL` selects
a disposable PostgreSQL database; catalog/storage tests create and drop unique
schemas inside it. `MOXIEDB_TEST_DOCKER=1` lets the SQL package create disposable
PostgreSQL 16 containers instead. The existing preparation and HTTP contract tests
also accept `MOXIEDB_JOB_TEST_URL` (a freshly initialized empty test catalog) and
`MOXIEDB_HTTP_TEST_URL` (a disposable database; each test creates its own schema).
The preparation SQL test injects a single failed metadata commit to exercise the
real activity retry. Job catalog tests use `MOXIEDB_TEST_URL` and the configured
Temporal test service with controlled computation and injected receipt loss.
These URLs must never point to the live application catalog. No tests run from
application startup or `tuning-up`.

## Reference implementation

Reusable Temporal execution belongs to
[`cmd/server/foundation/tmprl`](../../../foundation/tmprl/README.md): worker
reservations, scheduling, retries/timeouts, controls, snapshots, polling,
listing, progress, logging, and step-result commits. This application tree owns
the training definition, requests/artifacts, model/data resources, and computation.
The foundation does not import this tree or know any training stage/step names.
See its README for adding an unrelated job without copying Temporal plumbing.
Application APIs use foundation types directly: `tmprl.Execution`, `tmprl.Status`,
`tmprl.Progress`, `tmprl.Run[modeltraining.Request, modeltraining.Artifacts]`, and
`tmprl.StepResult[modeltraining.Artifacts]`. The embedded loader returns foundation's
`tmprl.Job`; application packages do not re-export these types with aliases.

## Package layout

```text
tmprl/
└── jobs/
    └── modeltraining/     # One job: submission, controls, and resource wiring
        ├── imports/      # Immutable uploads, metadata, and verified source identity
        ├── definition/   # Embedded job YAML
        ├── providers/
        │   ├── preparation/  # Model-completion client
        │   └── training/     # GoMLX / llama-server integration
        ├── stages/
        │   ├── preparation/  # Preparation contracts; extract, generate, verify steps
        │   └── training/     # Records, split policy; split, train, evaluate, publish
        └── standalone/
            └── preparation/  # Optional preparation/preview workflow APIs
```

The modeltraining package owns the combined job and connects both stages to
foundation's Temporal runner. Its definition, models, datasets, and providers
are job-specific, not shared infrastructure for unrelated jobs. Providers perform
computation, not scheduling. Stage packages own computation and checkpoints.
The standalone preparation/preview adapter preserves the independent APIs and
coordinates the same concrete preparation steps; it is not a second stage or a
second job required by the combined operation. Nothing in a stage imports the
whole-job package or the standalone workflow adapter.

Types live with their owners, without type aliases or a common model package:
`imports` owns upload metadata and source identities; `preparation` owns document
requests/results and completion/extraction contracts; `training` owns records,
record validation, JSONL I/O, split policy and trainer contracts; `evaluate` owns
evaluation reports. Importing prepared JSONL uses training's record validation and
readiness policy rather than defining another record model. Preparation generates
`training.Record` values because training owns the format it accepts.

The `modeltraining` package defines whole-job requests, combined artifacts and
custom job-step contracts directly. Its adapters merge stage-local results into
the combined artifacts, preserving outputs from the other stage. Built-in steps
accept their stage's own `StepContext` and return its own `Artifacts`; they do not
depend on whole-job request types. Generic reading/persistence uses foundation
directly rather than application forwarding wrappers.

`jobs/modeltraining/definition/job.yaml` is the single embedded execution definition.
It is compiled into the binary with `go:embed`; changes require a rebuild.

```text
Job: model-training (one Temporal workflow, one reserved worker)
├── Stage: preparation
│   └── Steps: extract → generate → verify
└── Stage: training
    └── Steps: split → train → evaluate → publish
```

A **job** owns the reservation, controls, durable state, and artifacts. A
**stage** groups ordered steps and bounds their total elapsed time. A **step**
implements one operation and is one Temporal activity, with its own per-attempt
timeout, heartbeat timeout, retry count and exponential backoff. All stages and
steps run serially in the same worker session. Different workers can run
different jobs concurrently.

`modeltraining.Business.StartJob(JobRequest{Model: ..., Prepare: ...})` submits both
stages as one job. `StartTraining` delegates to this runner for prepared JSONL or
an existing completed preparation; that job skips the preparation stage.
Submission copies the effective YAML plan into Temporal input and `Run.Definition`.
Already-running jobs do not read changed deployment defaults during replay.

The YAML loader rejects unknown fields, invalid/overflowing durations, duplicate
names and invalid retry policies. Service construction also rejects any step
without a registered implementation. Model IDs, input data, and training
hyperparameters remain per-job inputs, not execution policy.

## Hosting workers

Construct the imports, standalone preparation and model-training services, then start the
training service's `NewWorker`. The combined job calls preparation implementations
directly in its reserved worker; **no preparation worker or child workflow is
needed**. Stop workers before closing the borrowed Temporal client. Worker options enable sessions with capacity
one; a host using `Register` directly must also use `WorkerOptions`. Limiting
activity concurrency alone does not reserve a worker between stages.

Standalone preparation/preview APIs remain available for review before training.
Those independent workflows need their own preparation worker and separate queue.
The default standalone preparation and job queues are
`moxie-preparation` and `moxie-tuning`, respectively. Do not combine their
registrations in a worker: activity tasks can otherwise reach a worker that does
not implement them. When upgrading a deployment that previously shared a queue,
drain existing jobs before changing its preparation queue.

Pause takes effect at a **step boundary** and retains the reserved worker. Resume
continues that job; stop requests cooperative cancellation. Poll `Get` or await
`Wait` to establish the actual outcome. A paused job does not free capacity for
another job. Repeated Pause or Resume requests are harmless. A successful control
call acknowledges the request, not completion of the transition; poll `Get` to
observe `pause_requested`, then `paused`.

If a worker is lost, the workflow attempts a replacement session and resumes the
unfinished step with the same operation key and committed artifacts. The YAML
sets the maximum session attempts (currently three). Recovery is not an in-memory
training checkpoint: the unfinished step can run again. Workers must have access to the same durable
dataset/artifact storage and approved model files. Separate machines with only
private local disks cannot recover each other's artifacts.

The YAML's `reservation_timeout` bounds each worker reservation, including pauses
(currently seven days); it is not an overall workflow deadline across recoveries.
`queue_timeout` bounds each wait for a worker (one day). Stage `timeout` bounds
the entire stage, including retries, pauses after it starts, and worker recovery;
its deadline is not reset by recovery. Preparation currently has a 24-hour stage
budget, training 144 hours. Each step currently allows 24 hours per attempt and
three attempts, with a five-second heartbeat timeout. Heartbeats keep contact
alive; they do not prove computational progress or extend deadlines.

Step timeouts retry according to that step's policy. Exhaustion fails the job,
leaving committed steps intact and later steps unstarted. Stage expiration fails
with `stage-expired`, without restarting the stage. Cancellation is cooperative,
not a force-kill of native work. Preparation checkpoints generation and verification
batches separately; training does not resume unfinished epochs/batches.

`Run.ReservationDeadline` exposes the actual
reservation deadline after a worker is acquired. Expiration fails the job with
the nonretryable `reservation-expired` error and a message explaining that paused
time counts. Expiration does not acquire a fresh reservation or restart the job.

Preparation-only runs with ordered controls are the exception: they return their
worker while paused, awaiting review, or after a host drain, then wait without a
time limit. Only their first worker wait uses `queue_timeout`, and their stage
budget counts active time only. `Jobs.Drain` lets running units save before the
host stops its worker; see the training README's shutdown section.

## Polling and listing jobs

Newly submitted jobs start as `queued`, become `running` when a stage is
dispatched, and report `recovering` while waiting for replacement execution
capacity after worker loss. Pauses report `pause_requested` then `paused`.
Terminal outcomes remain `completed`, `needs_improvement`, `failed`, or `stopped`.
No mid-step suspension or restart-from-training-checkpoint control is provided.

`Run.Stages[].Steps` preserves the ordered submitted plan with stage/step status,
start/close times, stage deadlines and step attempts. `Run.Stage` and `Run.Step`
identify the active operation. A queued step has not started; a completed step is
committed and will not repeat during recovery. `Get` enriches the active step
with the latest server heartbeat timestamp and retry failure, plus progress
counts. Closed-run reads recover retry diagnostics from durable history.
`SessionAttempt`, `WorkerHost`, and `ReservationDeadline` describe the reservation;
the host and deadline are cleared while waiting for a replacement.

Lifecycle snapshots are persisted as Temporal memo at submission and transitions.
`Get` reads them without querying a worker, so queued and paused jobs remain
readable during outages. `UpdatedAt` is the snapshot transition time, not a
heartbeat time. `StateStale` indicates pending workflow work, an overdue heartbeat,
an elapsed reservation deadline awaiting processing, or a legacy history fallback.
It means last-known state, not proof the job failed. Worker-loss detection itself
is heartbeat-based and is not instantaneous. Legacy jobs without snapshots use a
bounded live query, falling back to history if their worker cannot answer.

`List(ctx, tmprl.ListOptions{PageSize: 50, Statuses: []tmprl.Status{tmprl.StatusPaused}})` returns job
snapshots and an opaque next-page token using one Temporal visibility request,
without a per-row `Get`. An empty status filter includes all jobs. Page sizes are
1–100; retain the same filters when supplying `NextPageToken`. Filtering can yield
a short or empty page with a nonempty token, so continue until the token is empty.
Visibility is eventually consistent, and list snapshots do not include live
heartbeat/retry enrichment. Jobs created before snapshots were introduced remain
metadata-only list entries; use `Get` for their details and exact domain outcome.
`History` remains available for callers that need only execution identities.

## Adding or removing stages and steps

Edit the ordered stages/steps and policies in `jobs/modeltraining/definition/job.yaml`.
Stages are declarative groups, not additional code interfaces. Implement
`modeltraining.StepHandler.Execute(context.Context, modeltraining.StepContext)` for a new operation
and register it in `Config.Steps` by `"stage/step"` (or in `builtinSteps` for a
built-in operation). The registry supplies capabilities only; it never supplies
execution order. Removing a step from YAML removes it from new jobs. No workflow
dispatch change is needed. `Config.Job` can supply a validated definition for
tests/alternate hosts; nil uses embedded YAML. Names must be unique, 1–64
characters, using lowercase ASCII letters, digits, hyphens or underscores.

`StepContext.JobStep` supplies canonical job/stage/step identity and incoming
artifacts, `Workspace` is the confined per-step workspace, and `Publish` reports
progress. Built-in training adapters additionally resolve scratch training jobs,
records, trainer and input fingerprints. Do not close these resources or retain
them after `Execute` returns. Preserve incoming artifacts, merge your outputs,
validate them, and return `tmprl.StepResult[modeltraining.Artifacts]`. Empty status continues; `completed` or
`needs_improvement` ends the pipeline. Return invalid outputs as nonretryable
errors. Honor context cancellation and deduplicate external side effects with
`JobStep.Key` (`run/stage/step`), because activities provide at-least-once execution.

The managed runner owns input checks, resource lifetime and durable step-result
caching. Steps own computation and output validation. Later built-in steps require
earlier artifacts (training needs the split dataset); removing a prerequisite
requires replacing its dependent steps too. Artifacts contain references, not
large documents or model weights in Temporal payloads.

The ordered stage/step names and policies are captured at submission. Keep their
implementations available on all workers until those jobs finish; deployment
changes do not rewrite existing plans. A **job** contains ordered **stages**;
each stage groups ordered executable **steps**. The leaf packages expose `Step`,
never a `Stage` alias. `TrainingStep` names an executable training handler;
`StepContext` supplies its resources.

All seven leaf packages implement `Step.Execute` directly. Preparation's `extract`
owns source extraction and checkpoint initialization, `generate` owns resumable
example/question generation, and `verify` owns verification and final dataset
publication. Their private checkpoint package shares only the durable format,
identity checks and bounds validation; it does not dispatch or execute steps.
Both the combined job and standalone preparation APIs invoke these handlers.
Preparation adapters supply `preparation.StepContext.Preparation` with borrowed document
resources; handlers preserve incoming artifacts and verification merges its result.

The original `Start` API uses `Config.LegacySteps` (default: `DefaultSteps`) for
its flat plan, **not** the YAML job definition. `LegacyStepRequest` and the
historical activity names/`stage` payload fields preserve that older protocol;
they do not define YAML stages. The split step is named `split`; only the legacy
boundary maps it to its original `prepare` identity. Original workflow histories
and v1 preparation checkpoints keep their existing recovery semantics.
New callers should use `StartJob`, `StepHandler`, `StepContext`, and foundation's
`tmprl.StepResult[modeltraining.Artifacts]`.
Legacy Go timeout overrides apply only to legacy APIs; YAML controls new jobs.

## Diagnostics and tests

`Config.Log` supplies submission/control logging through `applog`. Workflow and
activity logs use the Temporal client's configured logger, including Temporal's
workflow/run/activity identity fields. Lifecycle logs include the stage plan,
worker session/host, retries, operation key, deadlines, timings, input counts,
fingerprints, checkpoint reuse/commit, evaluation acceptance and terminal outcome.
Training progress counts use debug level to avoid an info-level entry per batch.
Job logs carry both stage and step. They do not record prompts, targets, model responses or arbitrary
provider progress messages. Providers must also keep their returned errors safe
for logging and Temporal history persistence.

Tests use the public worker constructors against a real Temporal service with
lightweight fake computation. They cover polling, controls, retries, publication
gating, one/two-worker ownership, queueing while paused, serial custom stages,
repeat-safe controls, workerless polling, retry diagnostics, filtered visibility
pages, reservation expiration, and worker replacement without repeating committed
stages. YAML-job tests additionally exercise all seven actual steps under one
reservation without a preparation worker, timeout retries/exhaustion, total stage
deadlines and step-boundary pause/resume. Managed-pipeline tests
exercise the actual stage code while substituting model computation. These tests
validate orchestration and API contracts, not model quality or native training.

Run the focused suite from the repository root with Docker available (or the
Temporal test service configured). Leave `MOXIE_TEST_GEMMA_MODEL` unset to avoid
the opt-in real-model training test:

```sh
GITHUB_WORKSPACE="$PWD" MOXIE_TEST_GEMMA_MODEL= go test -race -count=1 -timeout 10m ./cmd/server/foundation/tmprl/... ./cmd/server/app/sdk/tmprl/...
```

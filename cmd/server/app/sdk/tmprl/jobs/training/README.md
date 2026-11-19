# Training job

This is the new model-training implementation. It uses foundation's job runner
directly and does not import `jobs/modeltraining`. That implementation is retained
as a reference, not as a dependency or an alternative execution path in this job.

## Ownership

| Location | Responsibility |
| --- | --- |
| `training.go` | Submission/review/control application API, execution-state interpretation, application errors, step registration, resources and artifact access |
| `app/sdk/tmprl/datasets` | Application service: preparation runs, shared history, saved-output readiness and dataset management |
| `app/sdk/tmprl/datasets/store` | Dataset identity, immutable versions, safe publication, concurrency guards and dataset-ID validation; no job coordination |
| `input.go` | Nonretryable input and step error helpers |
| `job.yaml` | Embedded ordered stages, steps, deadlines and retry policies |
| `foundation/tmprl/uploads` | Generic confined upload storage, metadata and integrity checks |
| `stages/preparation/` | Preparation contracts; extract, preview, generate and verify steps |
| `stages/training/` | Record/trainer contracts, split policy; split, train, evaluate and publish steps |
| `foundation/tmprl/providers` | In-process KMS completion, Docling conversion, and GoMLX computation |
| `stages/training/trainer.go` | Stage-to-provider translation, protected-answer isolation and prediction token budget |

Foundation owns the workflow, worker reservation, activity identity, heartbeats,
execution logs, retries, timeouts, pause/resume/cancellation, polling/history,
progress and completed-step result commits. There is no application workflow,
activity dispatcher, control implementation, or second completed-step cache.
Preparation batch checkpoints and locked training splits are business state,
not a replacement for foundation's completed-step commits.

## SQL dataset publication boundary

The host may supply `Config.MoxieDB` and its existing `Config.Datasets` file store.
When configured, this job calls SQL-only `moxiedb` directly for verified dataset
publication and exact published-version reads. It never imports the parent
datasets service. The datasets service owns create/remove/import coordination;
both use the file-only store and its cross-process publication lock.

The host must initialize/reconcile datasets before starting this job's worker.
SQL table installation belongs to `make tuning-up`, not `training.New` or worker
startup. The job preserves the stable `add/<publication-id>` identity and input
digest through file publication and transactional catalog commit. An activity
retry after the file was published but SQL failed reuses that exact snapshot.
Index repair happens after catalog commit. An unresolved competing publication
blocks a new write instead of letting an uncataloged head become its base.

Saved reusable-output downloads pin the saved dataset/version, check relative location,
size and digest, and never fall back to a newer head, old scratch result or another
file when the catalog/file is missing or corrupt. Omitting `Config.MoxieDB` retains
the existing file-only behavior; configured SQL outages do not enable fallback.
Legacy runs without a saved reusable dataset retain their confined scratch-output
download contract.

## Durable job catalog

With `Config.MoxieDB`, `Start` records a stable namespace-scoped job, attempt and
immutable submit intent before contacting Temporal. Retries with the same ID
and input resolve the original execution; changed input conflicts. Resolve lost
receipts with Describe/Get before retrying a submit. Workflow-ID reuse remains
rejected by the existing runner. A request without an ID receives a new ID; it
cannot deduplicate a later independently submitted HTTP request without that ID.

Review, explicit Training authorization and Stop persist command intents before
the external mutation. A lost receipt stays pending until reconciliation confirms
acceptance or rejection. Acceptance of Stop is not terminal cancellation, and
review never authorizes Training. Both stages remain in this job.

The host starts `RunCatalog` after dataset initialization and joins it on shutdown
before closing the client/database. It discovers existing history (retrying after
outages), resolves pending commands, and refreshes active/unobserved executions
every five seconds with bounded calls. Exact namespace/workflow/run links are
immutable. Observations use revision CAS and source timestamps so delayed reads
cannot replace newer state. SQL holds sanitized bounded status/readiness, not
preview/source text, decisions, provider errors or private artifact paths. Durable
review intent payloads are private command data and never appear in public reads.

Temporal remains execution authority; SQL retains the last confirmed observation
and pending requests. An outage marks that observation stale without advancing
its observation time or inventing failure. Computation shutdown recovery is
covered separately by milestone 3B.2/3B.3 below. The sample grid is not connected
by this work.

## Host integration

Construct `uploads.Store`, then call `training.New(client, config)` to create a
`*training.Job`. Supply
approved model IDs and paths, `Config.Uploads`, a separate execution
`Config.StorageRoot`, and optional computation providers. The upload store accepts
arbitrary file formats without interpreting their contents; preparation validates
supported document formats, and the job validates prepared JSONL records and
training readiness before submission. Supply the host's KMS pool through
`Config.ModelPool`: the default GoMLX trainer uses it for exact tokenization and
base/candidate evaluation, and preparation defaults to KMS completion. Completions
acquire a model from the host's pool and call its chat API in-process, without
calling the server's HTTP endpoints. There is no external completion provider. No
llama-server subprocess or tokenizer HTTP service is used. Training inference
uses private pool entries, so loading a candidate never changes adapters on
models serving ordinary requests. Those entries use CPU inference and are
unloaded after each operation; the tokenizer is unloaded before training graphs
are allocated. The resolved KMS model must match the approved GGUF file.

Docling is an explicit preparation provider: construct
`providers.NewDoclingExtractor` from `foundation/tmprl/providers` and supply `Config.Extractor` for
documents requiring it. Plain text and Markdown extraction remain local.
Tests substitute the `Trainer` and `ModelCompleter` interfaces without changing the real
steps or runner.

Providers do not import this job or its stages. They own their computation
request/result types; the job owns provenance, split policy and acceptance.
There are no stage-specific provider subpackages or compatibility aliases.

- Start a host-owned worker with `job.Jobs().NewWorker().Start()`; retain
  the worker and stop it before closing the borrowed Temporal client.
- Submit validated requests with `job.Start(ctx, request)`.
- Use `job.Get` and `job.List` for execution observations, report/adapter availability
  and ETA. Dataset-output identity/count/readiness is interpreted by datasets.
- Request cooperative cancellation through `job.Stop`; an accepted request does
  not claim that execution has already stopped or that committed outputs are gone.
- Host infrastructure can still use `job.Jobs().Wait`, `Pause`, `Resume`, `History`
  and `Subscribe`. Foundation owns their actual execution semantics.
- `trainapp` receives the host-owned datasets service explicitly. Preparation and
  shared run operations go through `datasets.Service`, which calls this job.
  Training authorization/evaluation/adapter handlers retain their training API.
  HTTP does not discover storage through the job or interpret Temporal errors.
  Application errors preserve the original backend cause.
- `main.go` retains the existing `.moxie` paths and injects one shared dataset store.
  For existing non-HTTP callers, omitting `Config.Datasets` still constructs a
  store under `Config.StorageRoot/preparation-datasets`.
- Read deliverables with `job.Evaluation` and `job.OpenAdapter`.

`datasets.Service` and this job share `datasets/store`, not each other's application
service. This avoids an import cycle without callbacks, duplicated schemas or a
second execution engine. Both services call SQL-only `moxiedb` directly for their
respective metadata: datasets reads catalog history; this job writes execution
links, observations, command intents and verified output publications.

Select exactly one input: `Request.Prepare` for an imported document, or
`Request.DatasetID` for prepared JSONL. Documents enter Preparation first; prepared
JSONL skips preparation. This job does not depend on a standalone preparation workflow.
The validated request and effective definition are captured in Temporal input.
Each submission parses a fresh copy of the embedded YAML, so returned snapshots
cannot mutate future execution plans.

The BUI submits documents with `Request.PreparationOnly` enabled. These jobs
retain Extract → Preview → Generate → Verify, including Preview approval, and
finish after Verify without reserving a worker for a later training decision.
They do not require an approved training model or training hyperparameters.

`store.Store` in `app/sdk/tmprl/datasets/store/store.go` owns named datasets
and immutable versions. Supply `Prepare.TargetDatasetID` and `Prepare.BaseVersion`
on a preparation-only request to append that document's verified examples. Existing
documents are not extracted or generated again. Additions merge concurrent updates;
removal requires the latest version and excludes only that document's contribution.
Original uploads, previous versions, and adapters are unchanged. Publication is
idempotent by run ID, including replay after a document was removed. The saved
dataset/version and combined count/readiness are retained in the run result.
`Job.OpenDataset` downloads that saved version rather than only the new contribution.

The application exposes create/list/get dataset routes, document removal with
`base_version`, and `/v1/training/datasets/{dataset_id}/versions/{version}/download`.
History separates saved datasets and preparation-only runs from training runs.

## Document review and training authorization

In the BUI's Prepare Dataset settings, **Record target → Set record count**
sets the requested verified record count for each document. It maps to
`prepare.max_questions` (a nonnegative integer); omitted or `0` means **Unlimited**.
The count includes accepted Preview examples and held-out evaluation questions,
and applies to every preparation goal. Preparation makes additional passes over
the selected source and replaces rejected candidates to reach this count. The
three-example response size is not a lifetime limit on a passage. Different
standalone questions may share a supported answer; exact duplicate prompts and
unsupported answers are not accepted. Question variants remain training records
rather than using the one-training-question-per-fact evaluation recipe. Preview
remains a separate sample of one to five records, not the full target.

If three consecutive passes produce no additional verified records, preparation
saves the available records with a warning rather than failing for a count
shortfall. Model or infrastructure errors still use their existing failure
handling. **Unlimited** retains its one-pass behavior. Saving into an existing
dataset still skips duplicate questions. The setting is captured when a new job
starts; completed historical datasets are not changed.

For PDFs, the BUI's optional **First page** and **Last page** fields map to
`prepare.page_start` and `prepare.page_end`. Set both to positive PDF viewer
page numbers (inclusive), or leave both blank/zero for the whole document.
Docling converts only the selected pages; Preview and full generation use that
same extraction. The original upload is unchanged, and the page range is part of
the extraction cache identity. Page ranges are rejected for non-PDF sources.
For example, pages 11–25 with `max_questions: 50` generates toward 50 verified
records from that section, including evaluation questions, without converting the
rest of the PDF.

Training requires at least 20 training, 10 validation, and 20 protected test
examples, with valid fact coverage and split isolation. A target of 20 is a
preparation-only test; even 50 records does not guarantee training readiness.

The BUI's estimated step remaining time comes from worker timing saved in Temporal
heartbeats, not browser-local samples. Reloading the page retains the latest
estimate. Each activity attempt starts a new timing baseline, excluding already
checkpointed work; changes in work totals or counter resets start a new sample.
An estimate needs completed work after that baseline. Stale or paused runs do not
show an estimate. Later steps, queueing and human review waits are not predicted.

Preparation has four ordered steps: Extract → Preview → Generate → Verify.
Preview reads the committed extraction, creates up to three examples, and stays
at the `preview-review` checkpoint for user review. Missing or mismatched
extraction fails rather than reopening the source. Review and Continue belong to
Preview; they are not additional stages or steps.

Read `Run.Artifacts.Preview` and the exact `Run.Checkpoint.ID` through `Jobs().Get`.
Submit edited records in `preparation.Review` as the `Decision.Payload` to
`Job.ContinuePreview`. Only prompts and targets may change: example count, order,
evidence and source associations are retained. The SDK normalizes and validates
the records, marks them user-approved, and recomputes fact identities. This input
does not modify the original request. New document submissions must not supply
`PreparationRequest.ReviewedRecords` up front.

Continue releases Generate, not Training. Generate seeds the accepted examples
once, excludes their passages from further generation, and resumes committed
batches after interruption. Verify checks the complete dataset, including edited
examples, before publishing it. Acceptance of edits is not a claim that their
content has passed model verification.

For legacy combined requests, when Preparation finishes, its stage is completed and the same job waits at
`start-training`. `Job.StartTraining` accepts a payload-free decision only for
that pending checkpoint and a published, training-ready dataset. Training never
starts automatically for document input. Identical retries of either public
decision method return the retained receipt, even after the job closes;
conflicting submissions fail. An old Preview receipt cannot release Training.

Manual Pause remains independent of review: Continue while paused accepts the
records but does not resume work. Waiting for Preview counts toward Preparation's
24-hour budget. Waiting after completed Preparation does not restart its budget;
both waits retain the worker and count toward the 168-hour reservation budget.
Preparation-only ordered Pause/Continue has application endpoints; live BUI
controls remain a separate milestone. Review acceptance and Continue of a paused
execution are different operations.

Each leaf step has `Step.Name` and `Step.Execute` in `step.go`. To add an operation,
implement the appropriate stage contract, register it in `training.go`, and add it
to `job.yaml`. No foundation scheduler changes are needed. Changing the embedded
definition requires rebuilding the binary.

One reserved worker executes all steps serially. Additional workers can run
other jobs. Workers recovering jobs on another machine must share the durable
execution storage root and have access to the same uploads. Upload storage is
accessed through `Store.Get` and `Store.Open`; application code does not depend on
its directory layout. Legacy and combined runs pause between steps;
cancellation is cooperative. Incomplete provider work can repeat on retry.

## Training Pause, Resume, and Stop

New SQL-backed training-only runs support Pause and Resume from the training run
page. Pause finishes the current optimizer batch, tokenization call, or evaluation
answer before acknowledging Paused. A request receipt alone does not mean that
computation has paused. Resume continues the same operation with its optimizer
state intact, without repeating completed batches. Resume can also withdraw a
pending Pause request.

While paused, training retains its worker, model memory, and live optimizer state.
Keep the server running; this is not a disk checkpoint that supports resuming
across a server restart. Run and activity timeouts continue to apply. Preparation's
durable suspension and worker-release behavior is unchanged.

Stop cancels either running or paused training cooperatively. The run becomes
Stopped only after cancellation is observed; a stopped run cannot resume. Earlier
saved dataset versions and adapters are unchanged.

## Preparation safe-point suspension (milestone 3B.2)

New SQL-backed preparation-only runs opt into ordered controls. Their steps check
controls before starting a unit and after saving completed work. An accepted
Pause receipt means requested, not Paused. After the activity exits at a durable
safe point, the runner acknowledges the control revision and workspace-relative
checkpoint/hash in Temporal. Existing catalog observation persistence copies that
bounded metadata into Moxiedb; SQL does not contain the computation checkpoint.
No SQL schema or additional recovery engine is involved.

Continue schedules the same unfinished step in the same execution. The first
resumed activity verifies the acknowledged file/hash before running; a missing or
changed checkpoint fails without resetting saved work. Subsequent activity retries
may legitimately have newer committed progress and use the preparation owner's
existing identity/cursor validation. Voluntary suspension does not consume an
activity failure, but failures before it reduce the remaining retry budget.
Generation's durable uncertain-request reservations are not cleared by Pause.

Safe boundaries are committed generation batches (up to five passages), individual
evaluation questions, committed verification batches (up to five records), and
preview generation/verification checkpoints. Preview verification is one bounded
call for at most five examples. Continue does not repeat those saved results,
approve Preview, or authorize Training. Stop remains terminal cooperative
cancellation, preserving saved files and any dataset version already published.

Extraction recovers at whole-document boundaries, an approved decision: the
completed conversion is saved and reused, and the run can suspend between it and
the source-unit plan. **An interrupted Docling conversion runs again.** The
installed docling-serve 1.38.1 local engine keeps task/partial progress in RAM
and has no task Pause API, so a persisted task ID would not help. PDF page-range
conversion is not planned.

### Shutdown and idle time

Checkpoints stay on disk in the run workspace; Moxiedb holds status and the
acknowledged checkpoint reference; Temporal owns scheduling and control order.
This follows Unsloth Studio's split (training checkpoints on disk, run status in
its database).

On SIGINT/SIGTERM the server calls `Jobs().Drain` before stopping the worker.
A running preparation unit finishes and saves, then the step suspends at that
checkpoint. The server waits up to 120 seconds, the window SageMaker and Unsloth
give a training process to save; the systemd and launchd stop limits are 150
seconds. A drain does not consume a worker-recovery attempt, and the next worker
continues from the checkpoint without a user Continue. A unit still running when
the 120 seconds expire runs again after restart. A crash still consumes one of
the three recovery attempts.

These runs return their worker while paused or awaiting Preview review, and after
a drain. While idle there is no time limit: the run waits for Continue or a
worker indefinitely. Only the first wait for a worker keeps the 24-hour queue
limit. The 24-hour Preparation budget counts active time only (`ActiveElapsed`);
`Deadline` projects the remaining budget during a reservation and is cleared
while idle. A shutdown or restart never continues an acknowledged paused run.
Training and legacy executions keep the reservation model described below.

Host power loss is not covered: the operating system may not wait for the drain.
Unsloth documents the same limit. Committed checkpoints survive; the unit in
flight repeats.

Checkpoint JSON writes sync the new file before atomic rename and sync its parent
directory afterward, returning write/sync errors. This strengthens those writes;
it does not prove all storage/volume/hardware power-loss behavior.

## Preparation crash recovery proof (milestone 3B.3)

`TestRecoveryCrashMatrix` (`recovery_test.go`) runs the worker in a child test
process, holds it at a chosen point with file gates, kills it with SIGKILL, and
starts a new worker process against the same uploads, workspace, PostgreSQL and
Temporal. The parent test is client-only. Each case must finish with the expected
record count, exactly one published dataset version, one review decision, no
repeated prompt, and no failed workflow task in its history:

| Killed while | Recovery proven |
| --- | --- |
| Docling conversion in flight | conversion runs again (exactly twice), nothing else repeats |
| awaiting Preview review | same checkpoint and preview, no auto-approval, no new extraction |
| generating, two batches saved, one in flight | saved batches kept, only the in-flight batch repeats |
| verifying | no `result.json` and nothing published before recovery |
| dataset file written, catalog row not committed | catalog still shows the earlier version; recovery publishes once, no pending publication left |
| Stop accepted just before the crash | run ends Stopped, no further generation, saved progress kept |
| repeated crashes | two kills, progress only moves forward, third session completes |

Existing in-process tests cover the remaining rows: lost submission receipt
(`TestCatalogSubmissionRecovery`), lost review receipt and catalog outage
(`TestCatalogReviewAndObservationRecovery`), missing or changed checkpoint
(foundation `TestJobsSafePointControls`), and idle downtime with no time limit
(foundation `TestJobsDrain`). `datasets` `TestCatalogExactDatasetOutput` shows
a restarted service rediscovers the exact result without a duplicate dataset and
refuses a changed or missing saved output. Production verification rejects a
replayed record ("repeats a prompt"), so recovery cannot publish duplicates.

The history check matters because Temporal retries a nondeterministic replay
until it happens to match, so a run can still finish. Before Temporal Go SDK
v1.50.0, cancelling the lost worker's session cancelled the running step and the
stage-deadline timer in random order, so a worker replaying a second crash could
fail repeatedly. v1.50.0 cancels in creation order by default; do not downgrade
below it or set `TEMPORAL_SDK_FLAG_9=0`.

Limits: providers are deterministic stand-ins, services are isolated test
containers, and a process kill is not host power loss.

## Verification

The public API tests run the embedded plan and real steps against Temporal with
controlled computation. They cover document-to-adapter execution, prepared input,
Preview edits and decision retries/conflicts, explicit Training authorization,
interrupted generation without duplicate accepted records, permanent failure,
evaluation rejection and protected test-data isolation.
Foundation's own tests cover the reusable controls, deadlines and recovery.
Local `_test.go` helpers start or reuse the static Docker container
`moxie-tmprl-test`, retain it after tests, and print buffered application/Temporal
logs only on failure. No importable test-helper package is required.
Use `make test-tmprl-down` for manual teardown. No actual model training is needed.

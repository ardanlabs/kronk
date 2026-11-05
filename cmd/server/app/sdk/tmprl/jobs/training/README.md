# Training job

This is the new model-training implementation. It uses foundation's job runner
directly and does not import `jobs/modeltraining`. That implementation is retained
as a reference, not as a dependency or an alternative execution path in this job.

## Ownership

| Location | Responsibility |
| --- | --- |
| `training.go` | Job types and all Job methods: submission, validation, step registration, resources and artifact access |
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
- Use `job.Jobs().Get`, `List`, `Wait`, `Pause`, `Resume`, `Stop`, `History`
  and `Subscribe` directly. They use foundation's concrete execution/run types.
- Read deliverables with `job.Evaluation` and `job.OpenAdapter`.

Select exactly one input: `Request.Prepare` for an imported document, or
`Request.DatasetID` for prepared JSONL. Documents enter Preparation first; prepared
JSONL skips preparation. This job does not depend on a standalone preparation workflow.
The validated request and effective definition are captured in Temporal input.
Each submission parses a fresh copy of the embedded YAML, so returned snapshots
cannot mutate future execution plans.

## Document review and training authorization

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

When Preparation finishes, its stage is completed and the same job waits at
`start-training`. `Job.StartTraining` accepts a payload-free decision only for
that pending checkpoint and a published, training-ready dataset. Training never
starts automatically for document input. Identical retries of either public
decision method return the retained receipt, even after the job closes;
conflicting submissions fail. An old Preview receipt cannot release Training.

Manual Pause remains independent of review: Continue while paused accepts the
records but does not resume work. Waiting for Preview counts toward Preparation's
24-hour budget. Waiting after completed Preparation does not restart its budget;
both waits retain the worker and count toward the 168-hour reservation budget.
These SDK controls still need application endpoints and BUI wiring.

Each leaf step has `Step.Name` and `Step.Execute` in `step.go`. To add an operation,
implement the appropriate stage contract, register it in `training.go`, and add it
to `job.yaml`. No foundation scheduler changes are needed. Changing the embedded
definition requires rebuilding the binary.

One reserved worker executes all steps serially. Additional workers can run
other jobs. Workers recovering jobs on another machine must share the durable
execution storage root and have access to the same uploads. Upload storage is
accessed through `Store.Get` and `Store.Open`; application code does not depend on
its directory layout. Pauses take effect between steps and count toward active
deadlines; cancellation is cooperative. Incomplete provider work can repeat on retry.

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

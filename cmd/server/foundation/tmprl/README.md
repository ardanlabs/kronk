# Temporal jobs

This package owns the reusable **Job → Stage → Step** execution machinery.
It has no dependency on application jobs, datasets, models, or training providers.

## Building a job

An application supplies:

1. Its request and artifact types: `R` and `A`.
2. An ordered `tmprl.Job`, normally parsed from application-owned embedded
   YAML with `tmprl.ParseJob`. The foundation supplies the schema and strict
   validation, not a default job or model-specific execution policy.
3. `StepHandler[R, A]` implementations registered by `"stage/step"`.
4. `Config[R, A]`: stable Temporal protocol names, task queue, shared storage
   location, logger, and any application-specific terminal outcomes.

`Job`, `Stage`, `Step`, and `Retry` are concrete models in this package.
`StageName` is the string identifier used in progress and execution snapshots;
`Stage` is the configuration containing a timeout and ordered steps.

Construct `Jobs[R, A]` with `New`, start its `NewWorker`, and call
`Submit(ctx, id, JobInput[R]{Job: &plan, Request: request})`. The application owns
request validation, ID selection, data/resource lookup, and business policy.
The host owns the Temporal client and worker lifetimes. Stop workers before
closing the client. Hosts using `Register` must also use `WorkerOptions`.

`StepHandler.Execute(ctx, StepContext[R, A])` receives the durable request,
incoming artifacts, an owned-by-the-runner confined workspace, and `Publish`.
`StepFunc[R, A]` adapts a function to this interface. Return the full merged
artifact set in `StepResult[A]`. Empty status continues; `completed` or an
explicitly registered application outcome ends the job. Validate business
outputs in the handler, and return nonretryable errors for permanent failures.

Adding/removing stages or steps changes only the application definition and
its handler registration. The runner does not dispatch on application names.
Keep implementations available while submitted plans still reference them.

## Decisions after steps and stages

A step can declare `wait_after: review` (a unique safe name within the job).
After that step commits, the run exposes `StatusWaiting` and a `Checkpoint`
containing its exact run-bound ID, name, stage and step. The next step does not
start until `Jobs.Decide(ctx, execution, decision)` accepts input for that ID.
`Resume` alone cannot release this gate. Accepting a decision while manually
paused preserves the pause until `Resume`.

A non-final stage can also declare `wait_after`. Its checkpoint has an empty
step name, and the stage is already completed with its deadline stopped before
the wait begins. Acceptance authorizes the next stage; it does not rerun the
completed stage. Checkpoint names must be unique across both steps and stages.

`Decision` contains `CheckpointID`, a caller-chosen `SubmissionID`, and optional
JSON `Payload`. IDs are limited to 512 and 200 bytes respectively; the payload
is limited to 64 KiB. The foundation validates the envelope only. Applications
must validate reviewed examples and other business rules before submitting.
The original job request remains immutable; later steps receive accepted input
through `StepRequest.Decisions`. Applications opt in through their captured job
definitions and expose their own business-validated decision methods.

`Decide` waits for a durable acceptance receipt, not subsequent work completion.
On an ambiguous timeout, retry the same submission ID and exact payload bytes,
or inspect `Get`. Identical retries return the saved receipt; conflicting input
is rejected. An old receipt cannot release a different checkpoint. Closed runs
cannot accept updates; use `Get` to read their retained receipts.

Both gates survive replay and worker replacement without repeating committed
steps. Waiting retains the worker reservation and consumes its budget. A step
gate also consumes the active stage budget; a gate after a completed stage does
not. Stop or budget expiry clears the pending checkpoint but preserves committed
artifacts and accepted receipts. Foundation has no training-specific transitions.

## Infrastructure guarantees and limits

- One session reserves one worker for the entire job. Stages and steps run
  serially; additional workers can execute other jobs independently.
- The submitted definition is persisted in workflow history and snapshots.
  Replay never reloads YAML defaults from a new deployment.
- Queue, reservation, total-stage, and per-attempt step deadlines, heartbeat
  timeouts, and bounded retry/backoff policies come from the submitted plan.
  Pauses retain the worker and count against already-started budgets.
- `Pause`/`Resume` apply at step boundaries. `Stop` requests cooperative
  cancellation. `Wait`/`Get` establish the actual result; an accepted control
  request is not an acknowledgement that the transition has completed.
- Completed step results commit atomically beneath
  `<StorageRoot>/<StorageDirectory>/<run>/steps/<stage>/<step>.json`.
  Recovery can repeat unfinished computation. Handlers must honor cancellation
  and deduplicate external effects with `Request.Key` (`run/stage/step`).
  Workers must share durable storage for cross-host recovery.
- `Get` reads durable memo plus server diagnostics, without requiring a live
  worker. `List` makes one visibility RPC, with no per-row queries. Visibility
  is eventually consistent; filtered pages can be empty with a next-page token.
- Submission/control logs use `Config.Log`. Workflow/activity logs use the
  Temporal client's logger, including its execution identity fields. Shared
  logs include stages, steps, attempts, operation keys, sessions, deadlines,
  elapsed times, cache reuse, and failures. They do not log request/artifact
  payloads or arbitrary provider progress messages. Applications must keep
  their returned errors safe for logs and durable Temporal failures.

## Existing integrations

The new [`training` job](../../app/sdk/tmprl/jobs/training/README.md) uses `Jobs`
directly for worker registration, workflow execution, controls and state. Its
application code supplies the embedded YAML, request/artifact types, business
validation and concrete step resources. It does not implement its own workflow
or activity runner, or depend on the older `modeltraining` reference tree.

The reference implementation still uses `legacy.go` for its earlier flat-stage
session protocol. That compatibility code is not used by the new training job;
new integrations use `JobInput` and the YAML hierarchy.

Low-level `Start`, `Signal`, `Stop`, `Wait`, `Query`, `History`, `Heartbeat`, and
`ProgressBroker` remain available for workflows outside the job runner.
All foundation tests and their helpers live directly in this directory as
`package tmprl_test` in `_test.go` files. `docker_test.go` starts or reuses the
static test container; `tmprl_test.go` owns isolated clients/queues, buffered logs,
execution cleanup, and tests of that lifecycle. There is no importable test-helper
package. Application tests remain in their own directories with local test-only
setup. `jobs_test.go` demonstrates an unrelated arithmetic job with scalar
requests/artifacts and a custom terminal outcome.

Run both infrastructure and application tests with `make test-tmprl`. They use
controlled computation against Temporal, not native model training, unless the
application's opt-in real-model test is explicitly configured.

## Test boundaries

The regular suite covers two-stage/two-step lifecycle combinations, retries,
pause/resume, cancellation, queue/stage/reservation deadlines, and worker-process
crashes. Crash tests kill a separate worker after a step commits, then require a
replacement to replay the workflow and resume unfinished work using shared
storage. Both step-first and session-first heartbeat expiry are exercised, along
with exhausted session attempts. A step heartbeat failure allows up to one
session-heartbeat interval for worker-loss detection before deciding whether to
recover; a healthy session does not gain extra step retries.

Public polling/listing tests cover durable snapshots, heartbeat diagnostics,
staleness, exact logical-status filters, custom outcomes and opaque page tokens.
The application suite also verifies real visibility results and artifact reads
after its worker stops. Logs remain buffered and print only on failure.

Native smoke tests are separate opt-in checks. They require compatible installed
llama.cpp libraries and an existing indexed single-file GGUF chat model; they
never download models or run optimization. Set these paths for your machine:

```shell
GITHUB_WORKSPACE="$PWD" KRONK_TMPRL_NATIVE_SMOKE=1 \
KRONK_TMPRL_SMOKE_BASE=/path/to/existing/kronk-base \
KRONK_TMPRL_SMOKE_LIBS=/path/to/compatible/llama-libraries \
KRONK_TMPRL_SMOKE_MODEL=owner/small-chat-model \
go test -race -count=1 -v -timeout 3m \
  -run '^(TestKMSNativeSmoke|TestGoMLXTrainer)' \
  ./cmd/server/foundation/tmprl/providers \
  ./cmd/server/app/sdk/tmprl/jobs/training/stages/training
```

These checks exercise real pool-backed tokenization, GoMLX context preflight,
structured KMS completion and training-stage base prediction. They do not prove
optimization quality, candidate-adapter inference or an actual Docling service.
Without the opt-in, the native tests skip; invalid explicitly supplied native
configuration fails rather than silently skipping.

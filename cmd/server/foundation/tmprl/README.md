# Temporal jobs

This package owns the reusable **Job → Stage → Step** execution machinery.
It has no dependency on application jobs, datasets, models, or training providers.

## Building a job

An application supplies:

1. Its request and artifact types: `R` and `A`.
2. An ordered `definition.Job`, normally parsed from application-owned embedded
   YAML with `definition.Parse`. The foundation supplies the schema and strict
   validation, not a default job or model-specific execution policy.
3. `StepHandler[R, A]` implementations registered by `"stage/step"`.
4. `Config[R, A]`: stable Temporal protocol names, task queue, shared storage
   location, logger, and any application-specific terminal outcomes.

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

Model training keeps its YAML, computation contracts, input validation, model
resources, and built-in step registry under `app/sdk/tmprl`. It uses foundation
execution, state, result, and definition types directly, parameterized by its
own request and artifact types. Serialized payload shapes stay unchanged. Small
application adapters retain historical workflow names and old payload dispatch.
`legacy.go` preserves flat-stage session command ordering for those histories;
new integrations use `JobInput` and the YAML hierarchy, not `LegacyInput`.

Low-level `Start`, `Signal`, `Stop`, `Wait`, `Query`, `History`, `Heartbeat`, and
`ProgressBroker` remain available for workflows outside the job runner.
`tmprltest` supplies shared real-Temporal test infrastructure with isolated queues
and execution cleanup. `jobs_test.go` demonstrates an unrelated arithmetic job
with scalar requests/artifacts and a custom terminal outcome.

Run both infrastructure and application tests with `make test-tmprl`. They use
controlled computation against Temporal, not native model training, unless the
application's opt-in real-model test is explicitly configured.

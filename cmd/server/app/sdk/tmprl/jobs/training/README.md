# Training job

This is the new model-training implementation. It uses foundation's job runner
directly and does not import `jobs/modeltraining`. That implementation is retained
as a reference, not as a dependency or an alternative execution path in this job.

## Ownership

| Location | Responsibility |
| --- | --- |
| `training.go`, `input.go` | Job request/output types, business configuration, validation and submission |
| `job.yaml` | Embedded ordered stages, steps, deadlines and retry policies |
| `steps.go`, `resources.go` | Concrete step registration and business resource/artifact translation |
| `artifacts.go` | Evaluation report access and accepted adapter downloads |
| `imports/` | Confined, immutable document and prepared JSONL imports |
| `stages/preparation/` | Preparation contracts; extract, generate and verify steps |
| `stages/training/` | Record/trainer contracts, split policy; split, train, evaluate and publish steps |
| `providers/` | HTTP/Docling and GoMLX/llama-server computation integrations |

Foundation owns the workflow, worker reservation, activity identity, heartbeats,
execution logs, retries, timeouts, pause/resume/cancellation, polling/history,
progress and completed-step result commits. There is no application workflow,
activity dispatcher, control implementation, or second completed-step cache.
Preparation batch checkpoints and locked training splits are business state,
not a replacement for foundation's completed-step commits.

## Host integration

Construct `imports.Business`, then call `training.New(client, config)`. Supply
approved model IDs and paths, the imports service, and optional computation
providers. The trainer defaults to GoMLX; tests substitute the `Trainer` and
`ModelCompleter` interfaces without changing the real steps or runner.

- Start a host-owned worker with `business.Jobs().NewWorker().Start()`; retain
  the worker and stop it before closing the borrowed Temporal client.
- Submit validated requests with `business.Start(ctx, request)`.
- Use `business.Jobs().Get`, `List`, `Wait`, `Pause`, `Resume`, `Stop`, `History`
  and `Subscribe` directly. They use foundation's concrete execution/run types.
- Read deliverables with `business.Evaluation` and `business.OpenAdapter`.

Select exactly one input: `Request.Prepare` for an imported document, or
`Request.DatasetID` for prepared JSONL. Documents run both stages; prepared JSONL
skips preparation. This job does not depend on a standalone preparation workflow.
The validated request and effective definition are captured in Temporal input.
Each submission parses a fresh copy of the embedded YAML, so returned snapshots
cannot mutate future execution plans.

Each leaf step has `Step.Name` and `Step.Execute` in `step.go`. To add an operation,
implement the appropriate stage contract, register it in `steps.go`, and add it
to `job.yaml`. No foundation scheduler changes are needed. Changing the embedded
definition requires rebuilding the binary.

One reserved worker executes all steps serially. Additional workers can run
other jobs. Workers recovering jobs on another machine must share the durable
storage root. Pauses take effect between steps and count toward active deadlines;
cancellation is cooperative. Incomplete provider work can repeat on retry.

## Verification

The public API tests run the embedded plan and real steps against Temporal with
controlled computation. They cover document-to-adapter execution, prepared input,
retry, permanent failure, evaluation rejection and protected test-data isolation.
Foundation's own tests cover the reusable controls, deadlines and recovery.
Local `_test.go` helpers start or reuse the static Docker container
`moxie-tmprl-test`, retain it after tests, and print buffered application/Temporal
logs only on failure. No importable test-helper package is required.
Use `make test-tmprl-down` for manual teardown. No actual model training is needed.

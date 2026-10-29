# Manual quick-check workflow

In Actions, select **Quick-check → Run workflow**. There are no inputs:
manual dispatch checks out latest `main` and runs the complete implemented matrix
with the tool's default repetitions. The summary records the resolved commit.

## Promote to Release

In Actions, select **Promote to Release → Run workflow** and supply:

- `release`: the intended `vX.Y.Z` release label. A real Git tag is not required.
- `commit`: the full SHA of the already tested commit in the retained queue.

This separate, manual workflow checks out `main`, installs Go from `.go-version`,
and restores the same queue as Quick-check. It runs on `ubuntu-latest`, without
model or native-library downloads, server startup, or another benchmark. Inputs
are passed through environment variables to the shell, which invokes
`go run ./.tools/quick-check promote -release "$RELEASE" -commit "$COMMIT"`.
The Go command promotes local retained evidence; it does not contact
GitHub or transfer files. The workflow performs all restoration and transfers.

Promotion records the intended release in a `promotion.json` sidecar in the
promoted run directory. Original finalized evidence (including summaries, events,
and tool logs) stays immutable; promotion does not pretend the original test ran
with a tag that did not exist at test time. Promotion is the operator's decision:
selected-cell, FAIL, INTERRUPTED, and unfinished-repetition reports are allowed.
Untested cells stay `NOT_RUN`; findings and recorded verdicts are not rewritten.
If a prior release baseline is
available, comparison reports are placed in `compare-release/` alongside the
promotion terminal log. Changed history is included as a download copy only, as
described below. The job summary identifies the tested SHA and release label and
reminds the operator to review and manually commit canonical history changes.
**No release is published and no Git tag is created.**

Comparison failures or unavailable comparisons are reported without blocking
promotion. Successful promotion, packaging, and upload permit replacement
of the shared queue. An operational failure before upload leaves the previous artifact intact;
pruning happens only after a successful upload. Once uploaded, the promoted queue
is what the next Quick-check or Promote to Release invocation restores.

## Temporary pre-merge validation

Before this workflow exists on `main`, `ramon-reichert` can apply the
`run-quick-check` label to his same-repository PR from `ramon/cron-job` into `main`.
The job checks the PR author's numeric account ID, exact branch and repository,
label, and labeling actor. Other PRs do not run the job. It tests the PR's exact
head commit and uses the normal retained queue, with no isolated artifact scheme.
Remove/reapply the label to test another revision; pushes alone do not trigger it.

**Before merging:** remove the temporary `pull_request` trigger and job guard,
restore checkout to `ref: main`, and remove this validation section. Keep
`workflow_dispatch` and the existing concurrency/artifact behavior.

## Execution

- Uses an existing Pacha runner matching `self-hosted, macOS, ARM64, sand, metal`.
  It does not change the host or runner configuration.
- Restores the queue first and skips setup, downloads, server startup, testing,
  comparisons, and upload if the checked-out commit matches the latest `new` run.
  This also skips prior FAIL/INTERRUPTED candidates at that commit. If the retained
  artifact expired or was deleted, there is no prior report to detect the duplicate.
- Installs the existing CI environment, verifies/pulls
  the two quick-check models, and builds/starts a dedicated tools server in the VM.
  `server-for-tools` runs in the background with the BUI disabled, without Node/npm
  installation or documentation/frontend generation.
  The fixtures are Qwen3.6 MoE Q2 and Ornith-1.5-9B Q8, both with embedded MTP;
  model resolution also downloads their image projection files.
- Calls `kronk server stop` before startup. A missing PID is normal for a fresh VM;
  a stop failure with a PID file still present aborts startup.
- Applies the existing Metal shim only to server startup and its descendants;
  quick-check's diagnostic commands run without injected library banners.
- Runs `make quick-check-this-code`, without selectors or a changed CLI contract.
- Only after PASS, attempts both commit and release comparisons independently.
  Missing baselines are logged as skips. Comparison failures fail the job, but
  the passing quick-check run still becomes available to subsequent runs.
- Stops the job's server after quick-check, including after failure, before
  comparison, packaging, or upload. Failure diagnostics go to the Actions log.

## Baselines and reports

One artifact stores the tool's retained new/old/release run directories together,
including their summary, events, and tool log, plus the current candidate's
`compare-commit/` and `compare-release/` reports when comparisons were attempted.
It is named
`quick-check-queue-<run-id>-<attempt>` so retries can safely upload a replacement.
After that upload succeeds, the workflow deletes superseded shared queue artifacts;
there is no per-run artifact history and no separate reports artifact. Deletion
requires the workflow's `actions: write` permission and is scoped to queue
artifacts originating from exactly `.github/workflows/quick-check.yml` and
`.github/workflows/promote-to-release.yml`. Discovery resolves numeric workflow
IDs from the repository workflow list and current run, verifies their exact
workflow paths, then enumerates their runs and artifacts. A promotion workflow
not yet registered before merge is simply absent from discovery. Unrelated
workflows' artifacts are never restored or pruned. The previous queue remains
intact if packaging or upload fails.

Finalized failed or interrupted checks are saved too: they occupy `new`, while
only passing runs automatically qualify for `old` and `release`. Manual promotion
may explicitly select a failed or interrupted release baseline, which retention
then preserves. The next invocation replaces
the previous failed/interrupted candidate according to the tool's retention
policy; this is not a permanent failure archive. Cancellation cleanup and upload
are best-effort: a forced termination or lost VM can prevent finalization or
upload. Setup failures before quick-check runs leave the previous queue untouched
and expose diagnostics in the Actions log instead.

The next invocation restores this artifact before server startup, then lets the
tool apply its existing retention rules. All archive preparation, downloads,
uploads, and pruning happen outside quick-check execution. A matching latest
candidate commit skips testing and leaves the queue untouched. A temporary second
artifact exists between upload and deletion; interrupted deletion is retried
after a later upload.
Restore selects the newest retained queue artifact, including an older job's rerun.
Downloaded comparison reports are discarded before the next test, so a failed
candidate cannot inherit comparisons made for a previous candidate.

Release history stays at `.tools/quick-check/release-history.log`, outside ignored
`output/`. Each run starts from the version in Git. If comparison changes it, a
copy named `release-history.log` is included at the archive root for download,
review, and manual commit. No copy is included when unchanged, and restored
download copies never overwrite the canonical file. The workflow does not commit
or push history updates. Promotion follows this same history-copy behavior.

The first run has no baseline. A release baseline becomes available after a
successful tested commit is recognized as a release by the tool, including manual
promotion with an intended release label. Quick-check
does not automatically check out or benchmark older releases. Artifacts expire
after 90 days; if none remain, the baseline queue starts afresh.

Comparison tables and failure diagnostics are also available in the Actions logs. The job
summary identifies the commit, runner, and check outcome. The retained artifact
contains synthetic workload content and machine metadata; review before sharing.

The shared `quick-check-pacha` concurrency group, with cancellation disabled,
prevents overlapping invocations of **Quick-check and Promote to Release**,
including queue restore, upload, and pruning. Other CI
jobs, VMs, and manual workloads on Pacha remain unrestricted. VM timings are not
directly comparable with bare-metal runs; read the tool's comparability warnings.

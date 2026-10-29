# Manual quick-check workflow

In Actions, select **Quick-check → Run workflow**. There are no inputs:
manual dispatch checks out latest `main` and runs the complete implemented matrix
with the tool's default repetitions. The summary records the resolved commit.

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
After that upload succeeds, the workflow deletes its superseded queue artifacts;
there is no per-run artifact history and no separate reports artifact. Deletion
requires the workflow's `actions: write` permission and is scoped to its own queue
artifacts. The previous queue remains intact if packaging or upload fails.

Finalized failed or interrupted checks are saved too: they occupy `new`, while
only passing runs qualify for `old` and `release`. The next invocation replaces
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
or push history updates.

The first run has no baseline. A release baseline becomes available after a
successful tested commit is recognized as a release by the tool. This workflow
does not automatically check out or benchmark older releases. Artifacts expire
after 90 days; if none remain, the baseline queue starts afresh.

Comparison tables and failure diagnostics are also available in the Actions logs. The job
summary identifies the commit, runner, and check outcome. The retained artifact
contains synthetic workload content and machine metadata; review before sharing.

Concurrency prevents overlapping invocations of **this workflow only**. Other CI
jobs, VMs, and manual workloads on Pacha remain unrestricted. VM timings are not
directly comparable with bare-metal runs; read the tool's comparability warnings.

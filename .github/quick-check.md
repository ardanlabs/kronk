# Manual quick-check workflow

In Actions, select **Quick-check → Run workflow**. There are no inputs:
the job always checks out latest `main` and runs the complete implemented matrix
with the tool's default repetitions. The summary records the resolved commit.

## Execution

- Uses an existing Pacha runner matching `self-hosted, macOS, ARM64, sand, metal`.
  It does not change the host or runner configuration.
- Installs the existing CI environment, verifies/pulls
  the two quick-check models, and builds/starts a dedicated tools server in the VM.
  `server-for-tools` runs in the background with the BUI disabled, without Node/npm
  installation or documentation/frontend generation.
  The fixtures are Qwen3.6 MoE Q2 and Ornith-1.5-9B Q8, both with embedded MTP;
  model resolution also downloads their image projection files.
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
including their summary, events, and tool log. It is named
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
uploads, and pruning happen outside quick-check execution. Same-commit reruns
follow the tool's usual replacement behavior. A temporary second artifact exists
between upload and deletion; interrupted deletion is retried after a later upload.
Restore selects the newest retained queue artifact, including an older job's rerun.

The first run has no baseline. A release baseline becomes available after a
successful tested commit is recognized as a release by the tool. This workflow
does not automatically check out or benchmark older releases. Artifacts expire
after 90 days; if none remain, the baseline queue starts afresh.

Comparison tables and failure diagnostics remain in the Actions logs. The job
summary identifies the commit, runner, and check outcome. The retained artifact
contains synthetic workload content and machine metadata; review before sharing.

Concurrency prevents overlapping invocations of **this workflow only**. Other CI
jobs, VMs, and manual workloads on Pacha remain unrestricted. VM timings are not
directly comparable with bare-metal runs; read the tool's comparability warnings.

# Kronk quick-check

Matrix cells (32): **MoE or Dense / one or two slots / IMC off or on / MTP off or on / images off or on**.
Models: **MoE** `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL` and
**Dense** `ornith-ai/Ornith-1.5-9B-Q8_0`, using the `QUICK-CHECK` aliases
from `.tools/model_config_tools.yaml`, with **32,768 tokens per slot**.
Both require their image projection files and contain embedded MTP;
no separate Dense MTP file is needed.

## Run

In GitHub Actions, **Quick-check** runs daily at 02:00 Miami time, or manually
through **Run workflow**. Review **Compare available baselines** in the job logs;
download the run's artifacts if further investigation is needed.

Install the model and compatible native libraries first. From the repository root,
start a dedicated test server and run:

```sh
make server-for-tools
make quick-check-this-code # All 32 cells, one combined report.
```

To run a subset, repeat `-cell` with each exact cell ID:

```sh
make quick-check-this-code QUICK_CHECK_ARGS='-cell MoE/1s/IMC+/MTP+/img-'
make quick-check-this-code QUICK_CHECK_ARGS='-cell MoE/1s/IMC+/MTP+/img- -cell MoE/2s/IMC-/MTP-/img+'
make quick-check-this-code QUICK_CHECK_ARGS='-cell Dense/1s/IMC+/MTP+/img-'
```

One-slot image cells exercise image rebuild, replay, append, a different
image/session, cancellation, and recovery with the original image. Two-slot cells
exercise overlapping conversations, peer cancellation, and same-slot recovery.
Their image variants add a first image to text history, then another image to
image history, and recover with text while the other conversation continues.


```sh
# Optional settings (KRONK_TOKEN supplies authentication):
make quick-check-this-code QUICK_CHECK_ARGS='-repetitions 3 -host http://localhost:11435 -server-log /path/to/kronk.log'
```

### Testing another revision

Restart the server after configuration changes. To test another revision, preserve
uncommitted work, then:

```sh
make kronk-server-stop
git switch <branch> # or: git switch --detach <commit>
make server-for-tools
make quick-check-this-code
```

The target revision must contain the tool/configuration. Verify `Tested commit:`;
the tested code is the running server. The header also shows the test start time.

Keep in mind that reverting to an older commit does not automatically downgrade the llama libraries.
If you wish, you can pin a specific version of a library to ensure a fair comparison.

**WARNING: We suggest to run quick-check right after a fresh commit, rebuilding/restarting
the server first. Uncommitted changes are not flagged, though they are taken into account as soon as the server is restarted using `server-for-tools`. Runs rotate under
the retention rules below. Copy any run you want to keep into
`output/saved_local_runs/` before running again.**


## Results

Runs are saved under `.tools/quick-check/output/`:

- `tool.log`: brief verdict/timings, failure triage commands, and phase progress.
- `summary.json`: environment first, then each cell's status and measurements.
- `events.ndjson`: detailed requests, responses, and server evidence with cell IDs.

All selected cells share these three files and one retention queue, directly under
`output/`.

Those files can be useful to feed a harness and investigate reasons for slowdowns or regressions, alongside the bigger reliability tools.

All of `output/` is Git-ignored. Up to three run directories are retained:

- `new-<commit>`: fresh run, including `FAIL` or `INTERRUPTED`.
- `release-<version>`: manually promoted release baseline (or a recognized passing tagged run).
- `old-<commit>`: newest remaining previous passing run, release or not.

A fresh release preserves the older release baseline until a later run advances
the queue. Only passing runs automatically advance either baseline. Failed or interrupted
reruns preserve passing baselines even at the same commit. The next run replaces
the previous failed/interrupted `new` report: this queue is not a failure archive.
Local archives under `output/saved_local_runs/` remain outside retention.

## Promote to Release

In GitHub Actions, select **Promote to Release → Run workflow** and enter the
release label (`vX.Y.Z`) and full tested commit SHA. This selects the shared
release baseline without rerunning tests or creating a release/tag.

**Not needed for Actions.** Optional local promotion only; does not change the remote queue:

```sh
go run ./.tools/quick-check promote -release v1.33.1 -commit FULL_TESTED_SHA
```

## Compare

Compare saved results to catch wall clock differences between them:

```sh
make quick-compare-against-commit   # fresh versus old
make quick-compare-against-release  # fresh versus release baseline

# Either command also accepts an explicit pair (including archived runs):
make quick-compare-against-commit \
  QUICK_COMPARE_BASELINE=/path/to/baseline \
  QUICK_COMPARE_CANDIDATE=/path/to/candidate
```

Release comparisons use promotion labels when present, otherwise remote tags.
Same-machine release-to-release comparisons update `release-history.log`; preserve
it alongside the queue. `References updated` indicates whether the reference token
values or compared cell set changed since the previous entry on that machine
(`unknown` initially).

## Read the timings together:

- **Workload:** “How long did the actual workload take?”
- **Normalized:** “How much processing time would this workload require for fixed input and output token counts?”

The same test can produce different answer lengths across runs, changing Workload time even when processing performance stays the same. Normalized scales measured prefill and generation times to fixed token counts for each cell, reducing that variation and helping compare processing performance between code versions.

Lower is better, when comparing the same cell with unchanged references on the same machine. Read it alongside Workload: Normalized sums request processing durations, so overlapping slots can make it exceed actual elapsed time. It is an index, not a prediction of total workload duration.

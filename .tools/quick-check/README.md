# Kronk quick-check

Matrix cells (32): **MoE or Dense / one or two slots / IMC off or on / MTP off or on / images off or on**.
Models: **MoE** `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL` and
**Dense** `ornith-ai/Ornith-1.5-9B-Q4_K_M`, using the `QUICK-CHECK` aliases
from `.tools/model_config_tools.yaml`, with **32,768 tokens per slot**.
Both require their image projection files and contain embedded MTP;
no separate Dense MTP file is needed. Dense cells await real-run qualification
with the Ornith fixture.

## Run

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

IDs use `<routing>/<slots>/IMC<mode>/MTP<mode>/img<mode>`: routing is `MoE` or `Dense`,
slots are `1s` or `2s`, and each mode is `+` (on) or `-` (off).
There are no group selectors or comma-separated lists. Selected cells run in
matrix order, regardless of argument order. Images require the projection file.

Dense Normalized values remain `n/a` until references are calibrated from real
runs. Workload timings and correctness comparisons remain available.

One-slot image cells exercise image rebuild, replay, append, a different
image/session, cancellation, and recovery with the original image. Two-slot cells
exercise overlapping conversations, peer cancellation, and same-slot recovery.
Their image variants add a first image to text history, then another image to
image history, and recover with text while the other conversation continues.

```sh
# Optional settings (KRONK_TOKEN supplies authentication):
make quick-check-this-code QUICK_CHECK_ARGS='-repetitions 3 -host http://localhost:11435 -server-log /path/to/kronk.log'
```

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

A subset run follows the same retention rules as a full run; results from
separate invocations are not merged. A passing run replaces previous same-commit
reports, but a failed or interrupted run preserves a same-commit passing baseline.

## Results

Runs are saved under `.tools/quick-check/output/`:

- `tool.log`: brief verdict/timings, failure triage commands, and phase progress.
- `summary.json`: environment first, then each cell's status and measurements.
- `events.ndjson`: detailed requests, responses, and server evidence with cell IDs.

All selected cells share these three files and one retention queue, directly under
`output/`.

The terminal prints each executed cell's compact summary after all its repetitions
and evidence collection finish, then the overall verdict at the end. These writes
are outside measured workload intervals, never between repetitions or on the
token/phase path. Skipped cells remain `NOT_RUN` in the saved report.

Ctrl+C or SIGTERM cancels active requests and attempts model cleanup (bounded to
15 seconds), then saves the report as `INTERRUPTED`. Completed cells keep their
results and timings; an incomplete cell retains partial evidence but has no valid
aggregate timing. Unstarted cells stay `NOT_RUN`. A later invocation starts a new
run; it does not resume or merge the interrupted run. Forced termination such as
SIGKILL cannot perform this finalization.

Those files can be useful to feed a harness and investigate reasons for slowdowns or regressions, alongside the bigger reliability tools.

All of `output/` is Git-ignored. Up to three distinct runs are retained:

- `new-<commit>`: fresh run, including `FAIL` or `INTERRUPTED`.
- `release-<tag>`: most recent previous passing release.
- `old-<commit>`: newest remaining previous passing run, release or not.

A fresh release preserves the older release baseline until a later run advances
the queue. Only passing runs can advance either baseline. Failed or interrupted
reruns preserve passing baselines even at the same commit. The next run replaces
the previous failed/interrupted `new` report: this queue is not a failure archive.
Local archives under `output/saved_local_runs/` remain outside retention.


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

Release comparisons check remote tags. Successful same-machine release comparisons
update `release-history.log`: release tags, machine, reference-update flag, and the
comparison table only. Repeating a release pair on that machine replaces its entry;
commit the file to preserve history. The flag compares the candidate's reference
set with the previous saved comparison on that machine (`unknown` initially);
its reference ID also changes when the compared cell set changes.

## Read the timings together:

- **Workload:** “How long did the actual workload take?”
- **Normalized:** “How much processing time would this workload require for fixed input and output token counts?”

The same test can produce different answer lengths across runs, changing Workload time even when processing performance stays the same. Normalized scales measured prefill and generation times to fixed token counts for each cell, reducing that variation and helping compare processing performance between code versions.

Lower is better, when comparing the same cell with unchanged references on the same machine. Read it alongside Workload: Normalized sums request processing durations, so overlapping slots can make it exceed actual elapsed time. It is an index, not a prediction of total workload duration.

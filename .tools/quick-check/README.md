# Kronk quick-check

Cells: **MoE / one slot / IMC on / MTP on / images off or on**.
Model: `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK`.

## Run

Install the model and compatible native libraries first. From the repository root,
start a dedicated test server and run:

```sh
make server-for-tools
make quick-check-this-code # All implemented cells, one combined report.
```

Select cells during development (images require the model's projection file):

```sh
make quick-check-this-code QUICK_CHECK_ARGS='-cells text'
make quick-check-this-code QUICK_CHECK_ARGS='-cells images'
# -cells all (default) or -cells text,images runs both.
```

The image cell exercises image rebuild, replay, append, a different image/session,
cancellation, and recovery with the original image.

Flow: for each selected cell, one warm-up → two repetitions of cold chat, replay,
append, independent session, cancellation, and recovery → one final report for
the invocation. The model stays loaded across a cell's repetitions; it is unloaded
before/after each cell for now. Access to the server's JSON log is required.
Unselected cells (and those skipped after a failure) appear as `NOT_RUN`.

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

Keep in mind that reverting to an older commit does not automatically downgrade the llama libraries. If you wish, you can pin a specific version of a library to ensure a fair comparison.

**WARNING: We suggest to run quick-check right after a fresh commit, rebuilding/restarting
the server first. Uncommitted changes are not flagged, though they are taken into account as soon as the server is restarted using `server-for-tools`;; another run of the same
commit replaces its saved run. Copy any run you want to keep into
`output/saved_local_runs/` before running again.**

A subset run also replaces the previous same-commit report; results from separate
invocations are not merged.

## Results

Runs are saved under `.tools/quick-check/output/`:

- `tool.log`: brief verdict/timings, failure triage commands, and phase progress.
- `summary.json`: environment first, then each cell's status and measurements.
- `events.ndjson`: detailed requests, responses, and server evidence with cell IDs.

All selected cells share these three files and one retention queue, directly under
`output/`. Archive older single-cell run folders in `output/saved_local_runs/`
and rerun; old artifacts are not converted. Normalized uses fixed per-cell token
references; differing references suppress that cell's Normalized delta.

Those files can be useful to feed a harness and investigate reasons for slowdowns or regressions, alongside the bigger reliability tools.

All of `output/` is Git-ignored. Up to three distinct runs are retained:

- `new-<commit>`: fresh run, release or not.
- `release-<tag>`: most recent older release, excluding the fresh hash.
- `old-<commit>`: newest remaining older run, release or not.

A fresh release preserves the older release baseline until a later run advances
the queue. Same-hash runs replace rather than duplicate that hash. Local archives
under `output/saved_local_runs/` remain outside retention.


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

Release comparisons check remote tags. When both commits are releases, they update
`release-history.md` with timings and hostname/RAM/VRAM/unified
metadata for both runs. Repeating the release pair/cell updates its entry; commit
that file to preserve inter-release comparisons.

The terminal shows comparison status, each run's status, warnings, and warmed
workload/Normalized timings per cell. Cells are matched by ID, not execution order;
if either side did not run a cell, it shows `NOT_RUN` with no timing delta. With no
common executed cell, comparison exits nonzero. Reports replace `output/comparison.log` (terminal summary) and
`output/comparison.json` (full results), outside run folders.
Optional: `QUICK_CHECK_COMPARE_ARGS='-out /path/to/output'` or `'-json'` for JSON stdout.

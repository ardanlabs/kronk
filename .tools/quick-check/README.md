# Kronk quick-check

Current cell: **MoE / one slot / IMC on / MTP on / images off**.
Model: `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK`.

## Run

Install the model and compatible native libraries first. From the repository root,
start a dedicated test server and run:

```sh
make server-for-tools
make quick-check-this-code
```

Flow: one warm-up request → three repetitions of cold chat, replay, append,
independent session, cancellation, and recovery → final report. The tool unloads
its alias before and after the run. It requires access to the server's JSON log.

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

## Results

Runs are saved under `.tools/quick-check/output/`:

- `tool.log`: brief verdict/timings, failure triage commands, and phase progress.
- `summary.json`: structured results and measurements.
- `events.ndjson`: detailed requests, responses, and server evidence.

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
workload timings. Reports replace `output/comparison.log` (terminal summary) and
`output/comparison.json` (full results), outside run folders.
Optional: `QUICK_CHECK_COMPARE_ARGS='-out /path/to/output'` or `'-json'` for JSON stdout.

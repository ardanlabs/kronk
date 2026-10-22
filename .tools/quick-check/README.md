# Kronk quick-check

Current cell: **MoE / one slot / IMC on / MTP on / images off**.
Model: `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL/QUICK-CHECK`.

## Run

Install the model and compatible native libraries first. From the repository root,
start a dedicated test server and run:

```sh
make server-for-tools
make test-quick-check
```

Flow: one warm-up request → three repetitions of cold chat, replay, append,
independent session, cancellation, and recovery → final report. The tool unloads
its alias before and after the run. It requires access to the server's JSON log.

```sh
# Optional settings (KRONK_TOKEN supplies authentication):
make test-quick-check QUICK_CHECK_ARGS='-repetitions 3 -host http://localhost:11435 -server-log /path/to/kronk.log'
```

Restart the server after configuration changes. To test another revision, preserve
uncommitted work, then:

```sh
make kronk-server-stop
git switch <branch> # or: git switch --detach <commit>
make server-for-tools
make test-quick-check
```

The target revision must contain the tool/configuration. Verify `Tested commit:`;
the tested code is the running server. The header also shows the test start time.

**WARNING: Always run quick-check right after a fresh commit, rebuilding/restarting
the server first. Uncommitted changes are not flagged; another run of the same
commit replaces its candidate. Copy any run you want to keep into
`output/saved_local_runs/` before running again.**

## Results

Runs are saved under `.tools/quick-check/output/`:

- `tool.log`: brief verdict/timings, failure triage commands, and phase progress.
- `summary.json`: structured results and measurements.
- `events.ndjson`: detailed requests, responses, and server evidence.

Retention keeps the current candidate and baseline: same commit replaces the
candidate; a different commit promotes the previous candidate to baseline.
`output/saved_local_runs/` is Git-ignored and outside retention. Managed runs are
trackable in Git.

## Compare

Compare the two retained runs offline, older as baseline:

```sh
make compare-quick-check

# Or choose both paths explicitly:
make compare-quick-check \
  QUICK_CHECK_BASELINE=.tools/quick-check/output/<baseline-run> \
  QUICK_CHECK_CANDIDATE=.tools/quick-check/output/<candidate-run>
```

The terminal shows comparison status, each run's status, warnings, and warmed
workload timings. Reports replace `output/comparison.log` (terminal summary) and
`output/comparison.json` (full results), outside run folders.
Optional: `QUICK_CHECK_COMPARE_ARGS='-out /path/to/output'` or `'-json'` for JSON stdout.

Both commands return nonzero on failure. Comparison PASS is not a performance
verdict; inspect warnings and timing deltas (positive duration change = slower).

# Kronk quick-check

Matrix cells (32): **MoE or Dense / one or two slots / IMC off or on / MTP off or on / images off or on**.
Models: **MoE** `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL` and
**Dense** `ornith-ai/Ornith-1.5-9B-Q8_0`, using the `QUICK-CHECK` aliases
from `zarf/kms/tools_model_config.yaml`, with **32,768 tokens per slot**.
Both require their image projection files and contain embedded MTP;
no separate Dense MTP file is needed.

## Run

In GitHub Actions, **Quick-check macOS Metal** and **Quick-check Linux Vulkan**
run independently daily at 02:00 Miami time (including DST), or manually through
**Run workflow**. Manual runs test the selected branch; scheduled runs test the
default branch (`main`). A run skips if its checked-out SHA matches that
workflow's latest saved run, including a failed or interrupted attempt. Review
the release comparison in the run summary or job logs; download the run's artifact
to investigate.


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

Quick-check validates **engine execution, not answer semantics**. Prompts provide a
repeatable generation workload; required words, stale or foreign words, animal
identification, paragraph counts, and answer length are not graded. A completed,
nonempty response with sufficient token usage is required, along with the requested
MTP activity (or its absence when MTP is off).

Structured server evidence must attest the requested IMC reuse/rebuild behavior,
MTP activation and cleanup, slot/sequence ownership and cleanup, concurrent
progress, cancellation, and recovery. Unexpected server errors, missing evidence,
and incomplete execution still fail the job. `PASS` means those execution checks
passed; it does not certify answer quality or semantic conversation isolation.

Response wording cannot stop repetitions or suppress completed workload and
normalized timings, comparison deltas, or local release-history updates. Raw
responses remain in `events.ndjson` for investigation. Existing saved evidence
retains its recorded findings; older failed/incomplete attempts are not rewritten
as successful runs, and no timings are invented for work that did not complete.
Prompts and token budgets are unchanged to preserve the timing workload.

All of `output/` is Git-ignored. Up to two run directories are retained:

- `new-<commit>`: fresh run, including `FAIL` or `INTERRUPTED`.
- `release-<version>`: explicitly promoted release baseline.

Every finalized attempt replaces `new`; only Promote replaces `release`, even
when the same commit is tested again. Local archives under `output/saved_local_runs/`
remain outside retention. Older `old-*` directories are no longer managed.

Actions saves full evidence, comparison reports, and available complete server logs
(`server.log`, `server-start.log`, `server-stop-before.log`, and `server-stop.log`)
together in `quick-check-run.tgz`. Server logs are included when collected, even
for failed/interrupted checks; failures before a candidate is finalized may have
only Actions job logs. Download important evidence before it rotates out.

The artifacts are named `quick-check-macos-metal-run-<run-id>-<attempt>`
(macOS) or `quick-check-linux-vulkan-run-<run-id>-<attempt>` (Linux). After a
successful upload, each workflow keeps only its latest two full run artifacts,
including failed/interrupted attempts. Other workflows' artifacts are untouched.
Uploads request 90-day retention; repository limits and expiration still apply.
Each platform reads its tracked timing history from the checked-out branch:

| Platform | History document under `.tools/quick-check/` |
| --- | --- |
| macOS Metal | `release-history-macos-metal.log` |
| Linux Vulkan | `release-history-linux-vulkan.log` |

Nightlies never change checked-out history logs or write Git branches. No remote
baseline restoration, results branch, or snapshot cache is needed. Actions uses separate queues:
`output/linux-vulkan/` and `output/macos-metal/`. The manual commands above retain
their default `output/`; use `-out` to select a platform queue. Existing local
evidence is not automatically moved. Comparisons use the latest history entry.
Missing or zero baseline timings are reported as unavailable, not as zero-percent
changes, and do not block otherwise valid nightly evidence. Invalid or failed
candidate evidence fails comparison. Reports still appear in the summary, logs,
and saved artifact when comparison fails.


## Prepare release history locally

After both Actions quick-check jobs have measured the release-candidate commit,
run this from the repository root before finishing your release commit:

```sh
make quick-check-release-history QUICK_CHECK_RELEASE=v1.34.0 QUICK_CHECK_COMMIT=FULL_TESTED_SHA
```

Requires Go, authenticated GitHub CLI (`gh auth login`), `jq`, and `unzip`.
`QUICK_CHECK_COMMIT` defaults
to `HEAD`; it must identify the **measured server commit**, not just the workflow
run's head commit. Both Actions jobs test the selected branch on manual runs and
the default branch on scheduled runs. A release-branch SHA must actually have saved
quick-check evidence on both platforms; this command does not run tests.

The baseline is each platform's latest tracked history entry, not a Git tag or
cached run. Optionally require that its label matches an expected release:

```sh
make quick-check-release-history QUICK_CHECK_RELEASE=v1.34.0 \
  QUICK_CHECK_COMMIT=FULL_TESTED_SHA QUICK_CHECK_BASELINE=v1.33.1
```

The command downloads the newest saved candidate evidence whose actual
`server_revision` matches the requested commit on each platform. It stages copies
of both tracked history logs and invokes `compare-history -release` against them.
No baseline artifacts, snapshots, cache, release tags, or code-diff checks are needed.
An uninitialized placeholder permits recording the first measured release without
inventing percentage changes.

Both comparisons are prepared before either project history file is replaced:

- `.tools/quick-check/release-history-macos-metal.log`
- `.tools/quick-check/release-history-linux-vulkan.log`

Review these files and include them directly in your release commit. Rerunning
upserts the measured release entry instead of duplicating it. No branches,
commits, tags, releases, or remote writes are created. Missing artifacts, failed
or invalid candidate evidence, or a baseline-label mismatch stops preparation
without copying either staged history back. Candidate artifacts must still be
available in the platform's two-artifact queue. Tracked history is sufficient
for baseline timings, but cannot reconstruct full request evidence.

For manually collected evidence already in a local run queue:

```sh
go run ./.tools/quick-check compare-history -out .tools/quick-check/output \
  -history .tools/quick-check/release-history-macos-metal.log -release v1.34.0
```

## Compare

Compare a saved run against the latest release entry in the tracked history log:

```sh
make quick-compare-against-release  # local output/ versus the macOS history log

make quick-compare-against-release \
  QUICK_CHECK_HISTORY=.tools/quick-check/release-history-linux-vulkan.log \
  QUICK_CHECK_COMPARE_ARGS='-out .tools/quick-check/output/linux-vulkan'
```

Comparison alone never changes history. Only the explicit local
`compare-history -release vX.Y.Z` operation records a release. The latest entry's
**Candidate median [SD]** supplies the next baseline, not its older Baseline column.
Deltas use 0.01-second median precision to match the log. Positive percentages
mean slower. A report marked PASS validates the recorded checks; it is not an
automated performance-regression approval or threshold gate.

Machine metadata must match before timing deltas are shown. Normalized deltas
also require the same normalization reference ID (token references and cell set).
Missing cells or timings are explicit, and zero-filled placeholders never produce
percentages. Logs retain rounded summaries, not full environment metadata or raw
samples; detailed request/environment validation still needs the saved evidence.

For diagnostic comparisons between two complete saved runs or snapshots, supply
both paths. This retains the richer evidence comparison independently of the
nightly log-based report:

```sh
make quick-compare-against-release \
  QUICK_COMPARE_BASELINE=/path/to/baseline \
  QUICK_COMPARE_CANDIDATE=/path/to/candidate
```

The lower-level `compare` and `promote` commands remain available for these
explicit local snapshot workflows; comparing two promoted releases can record
history. They are not used to choose nightly timing baselines.

The initial v1.33.1 entry is included once in the code branch. Machine descriptions
include the operating system recorded by the test server.
`References updated` indicates whether the reference token values or compared
cell set changed since the previous entry on that machine (`unknown` initially).

## Read the timings together:

- **Workload:** “How long did the actual workload take?”
- **Normalized:** “How much processing time would this workload require for fixed input and output token counts?”

The same test can produce different answer lengths across runs, changing Workload time even when processing performance stays the same. Normalized scales measured prefill and generation times to fixed token counts for each cell, reducing that variation and helping compare processing performance between code versions.

Lower is better, when comparing the same cell with unchanged references on the same machine. Read it alongside Workload: Normalized sums request processing durations, so overlapping slots can make it exceed actual elapsed time. It is an index, not a prediction of total workload duration.

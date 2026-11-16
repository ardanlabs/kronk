# OpenCode code-generation benchmark — version 2

This harness measures how the `/BENCHMARK` models configured in
`zarf/kms/tools_model_config.yaml` complete the tic-tac-toe task through OpenCode's
headless agent loop. OpenCode can write the program, run the requested Go
toolchain, observe compiler failures, and repair its work before the harness
grades `tictactoe/main.go`. The original benchmark is preserved in the parent
directory. Historical reports copied into `reports/` are version 1 results.

Workload **v1** has two steps: **Step1 (basic game)** and **Step2 (Undo move)**.
Each model first attempts the basic game, with up to **three submissions** (initial
implementation plus two repairs). Passing all **37 checks** unlocks the undo
extension, with its own three-submission budget. Both steps share the same program,
workspace, and OpenCode session. A failed Step1 never advances to Step2. Each step
stops when it passes or exhausts its budget, for at most six submissions per model.
Finishing the OpenCode response constitutes submission.

Feedback is enabled by default. Disabling feedback permits only the initial submission
of each step; Step2 still requires a Step1 pass.
The repair message lists failed checks and distinguishes them from checks not reached.
It includes native diagnostics and, for each failed scenario, the inputs actually sent,
expected output up to the stopped interaction or completion, the complete captured
output transcript, and any execution/stop reason. Whitespace and ANSI escapes are
quoted literally; no minimal edits, inferred causes, or suggested fixes are produced.
Only identical observations, including their location, step and input, are collapsed.
The only instruction is to repair the existing program under the current workload requirements
and finish the response to resubmit. This is a repair evaluation, not independent sampling.

There is no separate feedback file or extra repair-message size cap. Prompts are
piped to OpenCode through stdin, avoiding operating-system argument-size limits.
The existing command-output capture is bounded at 64 KiB and explicitly marks discarded output;
terminal execution retains its output limit and reports when it is exceeded.
Feedback is input context: it must fit alongside the conversation in the model's
context window. `max_tokens` limits each model response, not the repair message,
and does not guarantee enough remaining input space. Detailed findings and captured
evidence remain in the existing `result.json`. Later grades may expose additional
failures that earlier errors prevented the grader from reaching.

An OpenCode error event, nonzero exit, or timeout stops the workflow. Its events,
error and available source snapshot are retained, but that attempt is not graded.
The report shows `Workflow: failed`, `Agent error`, and `Not graded`, rather than
regrading the previous source as another submission. Recoverable tool-call errors
are not treated as OpenCode execution failures.

The harness creates a fresh nested Git repository and isolated OpenCode home for
each model, shared across submissions. The repository boundary prevents OpenCode
from treating the surrounding Kronk checkout as its workspace. It installs only the repository's
`use-modern-go` skill, disables project configuration, plugins, MCP, web access, and
subagents, disables OpenCode's Git-backed workspace snapshots, and denies
filesystem access outside the attempt directory through OpenCode's file tools.
The structural tests and scripted game inputs live outside the agent workspace.
This is workspace/configuration isolation, not an OS security sandbox; shell
execution is allowed.

## Deterministic acceptance

The grader checks the submitted formatting, structure, build, vet, and helper
behavior, then runs interactive scenarios in a real pseudo-terminal: **22 for Step1**
(**37 total checks**) and **33 for Step2** (**48 total checks**). Step2 covers ordinary
gameplay as well as undo.
Input is delivered one line at a time only after the expected board, scores,
messages, and prompt are recognizable. Whitespace and supported ANSI color/clear
sequences are ignored only for deciding when to send the next input, allowing
formatting defects at later steps to be reported in the same submission. An
unexpected game state or prompt stops that scenario; other scenarios still run.
Exact output bytes remain required for a pass, including ANSI colors, resets,
screen clearing, spacing, and newlines. Feedback preserves complete captured
transcripts, including any earlier formatting defects when a later interaction stops.

Scenarios cover all eight winning lines for both players, a draw, a ninth-move
win taking precedence over a draw, invalid and occupied inputs, whitespace,
replays preserving all three scores, and exit answers. Each scenario is bounded
to three seconds and 64 KiB of output. A complete pass means these acceptance
cases passed, not a proof against every possible program defect or terminal.
Check percentages are diagnostic, not a percentage of playability. No Amp grader
or screenshot review is involved. The benchmark initially loads `tic-tac-toe-v1.md`.
Only after it passes, the agent receives the `# Add One-Move Undo` section from
`tic-tac-toe-v2.md`; the base-game prompt is not repeated. Step2 checks both players'
undo, restored turns and cell numbers, invalid/repeated undo, replacement moves,
score preservation, replay reset, and ordinary gameplay with the updated prompts.
The existing exact-output requirements remain in force.

Runnable references are `testdata/correct.go` (Step1) and `testdata/undo/main.go` (Step2).
Play the undo reference from the project root with:

```shell
GOTOOLCHAIN=local go run ./.tools/codegen-benchmark/version2/testdata/undo
```

Before a model's attempts begin, the harness warms it with a one-token response
to `hello model`. After all attempts for that model, it unloads the model before
moving to the next one. Warm-up work is not included in the reported OpenCode
token or timing measurements.

## Requirements

Use Go 1.27 or newer and a Unix terminal environment (macOS, Linux, or WSL)
with `sh`, `stty`, and Git available. Generated code is executed on this machine
without an OS sandbox. Do not run untrusted submissions on a sensitive host.

Install OpenCode and start the dedicated tools server (detached):

```shell
opencode --version
make server-for-tools
```

Restart the server after changing the tools configuration. Benchmark profiles use
one slot each; the original benchmark still uses `/AGENT` profiles in the normal
configuration.

The server must use `zarf/kms/tools_model_config.yaml`. A selected model must:

- be a key in that file;
- end in `/BENCHMARK`; and
- be returned by the server's `GET /v1/models` endpoint.

If authentication is enabled, `KRONK_TOKEN` must authorize chat requests and
the administrative model-unload endpoint.

## Run

Run every eligible configured model with up to three submissions per step:

```shell
make benchmark-codegen-v2
```

Select one model or reduce the submission budget (only 1–3 are accepted):

```shell
make benchmark-codegen-v2 \
    CODEGEN_MODEL=ornith-ai/Ornith-1.5-35B-Q8_0/BENCHMARK \
    CODEGEN_V2_SUBMISSIONS=2

# Evaluate without grader-assisted repairs:
make benchmark-codegen-v2 CODEGEN_V2_FEEDBACK=false
```

List eligible models without contacting the server:

```shell
make benchmark-codegen-v2-list
```

The primary knobs are:

| Make variable | Default | Purpose |
| --- | --- | --- |
| `CODEGEN_HOST` | `http://localhost:11435` | Running Kronk server |
| `CODEGEN_MODEL` | `all` | One ID, comma-separated IDs, or every `/BENCHMARK` model |
| `CODEGEN_V2_SUBMISSIONS` | `3` | Maximum submissions per step, including its initial one |
| `CODEGEN_V2_FEEDBACK` | `true` | Resume failures with grader feedback; false allows one submission per step |
| `CODEGEN_STEPS` | `40` | Maximum OpenCode agent steps per submission |
| `CODEGEN_TIMEOUT` | `30m` | Timeout per submission; also separately applied to warm-up and unload |

Outputs are retained beneath the gitignored
`.tools/codegen-benchmark/version2/runs/`, grouped by local date and unique run:

```text
runs/YYYY-MM-DD/run-HHMMSS.<unique>/
  report.txt               # compact plain-text entry to paste into history.txt
  <model>/attempt-01/
    result.json            # step label, grade, metrics, evidence, repair_message for repairs
    program/main.go        # submitted source, unchanged and runnable
    program/go.mod
    events.jsonl, opencode.stderr
```

New runs never delete previous runs or the old `output/` directory. Git ignores
these files; it does not back them up. They survive ordinary branch updates and
switches in the same checkout, but removing the worktree, deleting the folder, or
using `git clean -fdx` can remove them. Back up important runs yourself.

The plain-text `report.txt` contains only the entry to copy into `version2/history.txt`:
run ID, workload label, timestamps, workflow status, elapsed time, runner machine,
Kronk version, llama.cpp library tag, and a table of model outcomes, submissions,
final checks and cumulative agent time. Versions are read once before inference from
that run's server: `/v1/liveness` (`build`) and `/v1/kronk/libs` (`current`, not `latest`).
The build string is preserved as reported, including any development suffix; the
benchmark does not infer `+dev` from its own checkout. A new run stops before inference
if either version cannot be retrieved. Report regeneration preserves the recorded
values without querying a server; old reports without them show `not recorded`.
History is maintained manually; the benchmark never updates it. The runner machine
is the OpenCode/grader host, not a remote inference server.

The prompts and acceptance checks together define workload v1. The canonical
prompt files remain `tic-tac-toe-v1.md` and `tic-tac-toe-v2.md` (the latter supplies
only the undo extension); these filenames do not indicate different workload
versions. Neither is copied into runs. Reports show `Workload: v1`, with separate
rows for `Step1 (basic game)` and `Step2 (Undo move)`, each with its own outcome,
submission count, checks, and agent time. Step2 is `Not reached` when Step1 fails.
A Step2 failure does not erase the Step1 pass. Old results without step metadata
remain basic-game-only when reports are regenerated.

Attempt folders are numbered consecutively across both steps. Each `result.json`
records its `step`; the report counts submissions within that step, so Step2
submission 1 may be saved under `attempt-02` or `attempt-04`.

Each model uses one temporary `workspace/` shared across its submissions so repairs
retain the code and agent session. It is removed after that model finishes, including
on error return. Each `attempt-XX` directory preserves its prompt (including repair
feedback) only as `repair_message` in `result.json` for repair submissions; the initial
task is referenced by the workload and step labels. Attempts also preserve the JSON event
stream, stderr, grader result, and one runnable source snapshot.
Scenario inputs and expected/actual ANSI transcripts are stored only in
`result.json`, under `grade.evidence`; structured failure diagnostics are stored
once, under `grade.findings`.
No duplicate source/transcript files or `.tar.gz` archives are generated.
No grader source copy or separate `run.json` is generated. Run information is
preserved in the plain-text report when it is regenerated. Old Markdown reports
and `run.json` metadata can still be read; regeneration writes `report.txt` without
changing the old files.
The original benchmark's output is never cleared or used by this version.

The report is refreshed after each completed submission and on normal or error
return. Logs and prompts may contain
sensitive content; inspect before sharing. Abrupt termination can leave an
incomplete attempt and a temporary workspace; its events/stderr are streamed to disk, and folders without
`result.json` are not counted as completed grades.

To inspect an attempt locally or after an interactive SSH login, enter its saved
`program/` directory and run `GOTOOLCHAIN=local go run .`. For SSH, use `ssh -t`
to allocate a terminal. A failed submission can be unbuildable; it is preserved
as submitted, not silently fixed.

Agent time includes all submissions but excludes setup, warm-up, and grading.
Workflow completion is separate from passing acceptance checks. Detailed per-attempt
metrics and diagnostics remain in `result.json`, not the summary report.
With the Make defaults, a model can take up to six 30-minute agent invocations
plus setup, grading, warm-up and unload. `go run` without an explicit `-timeout`
currently defaults to 10 minutes per invocation.

Regenerate the latest run's report without invoking a model:

```shell
make benchmark-codegen-v2-report

# Select a particular run (also supports the legacy output directory):
make benchmark-codegen-v2-report \
    CODEGEN_V2_RUN=.tools/codegen-benchmark/version2/runs/YYYY-MM-DD/run-HHMMSS.unique
```

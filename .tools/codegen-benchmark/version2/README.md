# OpenCode code-generation benchmark — version 2

This harness measures how the `/BENCHMARK` models configured in
`zarf/kms/tools_model_config.yaml` complete the tic-tac-toe task through OpenCode's
headless agent loop. OpenCode can write the program, run the requested Go
toolchain, observe compiler failures, and repair its work before the harness
grades `tictactoe/main.go`. The original benchmark is preserved in the parent
directory. Historical reports copied into `reports/` are version 1 results.

Version 2 allows at most **three total submissions** per model: the initial
implementation and up to two repairs. Passing requires all **37 acceptance
checks**, with expanded execution coverage of the unchanged task. On failure, the harness
passes the failed check names and diagnostic details back to OpenCode and resumes
the same session with `--continue`. It stops immediately on a passing grade or
after the submission budget. Finishing the OpenCode response constitutes submission.

Feedback is enabled by default. Disabling feedback runs only the initial submission.
Feedback can include test expectations and output excerpts, but not the grader's
source. Later grades may expose additional failures that earlier errors prevented
the grader from reaching. This is a repair evaluation, not independent sampling.

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
behavior, then runs **22 interactive scenarios in a real pseudo-terminal**.
Input is delivered one line at a time only after the expected screen and prompt
appear. Exact output bytes are compared, including ANSI colors, resets, screen
clearing, spacing, and newlines; ANSI is not stripped for grading.

Scenarios cover all eight winning lines for both players, a draw, a ninth-move
win taking precedence over a draw, invalid and occupied inputs, whitespace,
replays preserving all three scores, and exit answers. Each scenario is bounded
to three seconds and 64 KiB of output. A complete pass means these acceptance
cases passed, not a proof against every possible program defect or terminal.
Check percentages are diagnostic, not a percentage of playability. No Amp grader
or screenshot review is involved. Only the initial task before the separator in
`tic-tac-toe.md` is evaluated; the later undo task is not included.

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

Run every eligible configured model with up to three submissions:

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
| `CODEGEN_V2_SUBMISSIONS` | `3` | Maximum total submissions per model, including the initial one |
| `CODEGEN_V2_FEEDBACK` | `true` | Resume failures with grader feedback; false stops after one submission |
| `CODEGEN_STEPS` | `40` | Maximum OpenCode agent steps per submission |
| `CODEGEN_TIMEOUT` | `30m` | Timeout per submission; also separately applied to warm-up and unload |

Outputs are retained beneath the gitignored
`.tools/codegen-benchmark/version2/runs/`, grouped by local date and unique run:

```text
runs/YYYY-MM-DD/run-HHMMSS.<unique>/
  prompt.md                # frozen task sent to the agent
  report.md                # run metadata, model/attempt results and play commands
  <model>/attempt-01/
    result.json            # grade, metrics, evidence, repair_message for attempts 2–3
    program/main.go        # submitted source, unchanged and runnable
    program/go.mod
    events.jsonl, opencode.stderr
```

New runs never delete previous runs or the old `output/` directory. Git ignores
these files; it does not back them up. They survive ordinary branch updates and
switches in the same checkout, but removing the worktree, deleting the folder, or
using `git clean -fdx` can remove them. Back up important runs yourself.

Each new run appends a basic summary to `version2/history.md`, outside the ignored
output directories: date/run ID, workflow status, elapsed time, runner machine
(hostname and OS/architecture), and each model's outcome and cumulative agent time.
The machine identifies the OpenCode/grader host, not a remote inference server;
it is saved in run metadata and preserved when regenerating reports. The workload uses a manually maintained `v1`
label, not a hash. Report regeneration does not append history. Commit the history
updates to preserve them in Git. Runs stopped abruptly may not reach the append.

Each model uses one temporary `workspace/` shared across its submissions so repairs
retain the code and agent session. It is removed after that model finishes, including
on error return. Each `attempt-XX` directory preserves its prompt (including repair
feedback) only as `repair_message` in `result.json` for repair submissions; the initial
task is saved once in the run-level `prompt.md`. Attempts also preserve the JSON event
stream, stderr, grader result, and one runnable source snapshot.
Scenario inputs and expected/actual ANSI transcripts are stored only in
`result.json`, under `grade.evidence`; failure diagnostics are also included there.
No duplicate source/transcript files or `.tar.gz` archives are generated.
No grader source copy or separate `run.json` is generated. Timestamps, workflow
status, and errors are stored in the report's `Run metadata` JSON section, preserved
when the report is regenerated. Old runs with `run.json` can still be read.
The original benchmark's output is never cleared or used by this version.

The report is refreshed after each completed submission and on normal or error
return. Logs and prompts may contain
sensitive content; inspect before sharing. Abrupt termination can leave an
incomplete attempt and a temporary workspace; its events/stderr are streamed to disk, and folders without
`result.json` are not counted as completed grades.

To inspect an attempt locally or after an interactive SSH login, enter its saved
`program/` directory and run `GOTOOLCHAIN=local go run .`. For SSH, use `ssh -t`
to allocate a terminal. Each report includes the exact relative command. A failed
submission can be unbuildable; it is preserved as submitted, not silently fixed.

The report shows final checks passed, the first passing submission (or `Not passed`),
and cumulative agent wall time, turns, tool calls, and tokens across all submissions,
including failed work. Grader success is separate from agent exit status. Wall time
excludes setup, warm-up, and grading. A run can take up to three 30-minute agent
invocations plus setup, grading, warm-up and unload.

Agent token statistics sum the final usage event from every OpenCode turn. Kronk
reports reused prompt tokens as `Cache read`; the report omits provider cache-write
usage because Kronk's OpenAI-compatible API does not expose that metric.

Regenerate the latest run's report without invoking a model:

```shell
make benchmark-codegen-v2-report

# Select a particular run (also supports the legacy output directory):
make benchmark-codegen-v2-report \
    CODEGEN_V2_RUN=.tools/codegen-benchmark/version2/runs/YYYY-MM-DD/run-HHMMSS.unique
```

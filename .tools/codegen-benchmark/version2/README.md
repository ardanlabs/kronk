# OpenCode code-generation benchmark — version 2

This harness measures how the `/AGENT` models configured in
`zarf/kms/model_config.yaml` complete the tic-tac-toe task through OpenCode's
headless agent loop. OpenCode can write the program, run the requested Go
toolchain, observe compiler failures, and repair its work before the harness
grades `tictactoe/main.go`. The original benchmark is preserved in the parent
directory. Historical reports copied into `reports/` are version 1 results.

Version 2 allows at most **three total submissions** per model: the initial
implementation and up to two repairs. Passing still requires all **20 original
checks**; the grader and task requirements are unchanged. On failure, the harness
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
`writing-go` skill, disables project configuration, plugins, MCP, web access, and
subagents, disables OpenCode's Git-backed workspace snapshots, and denies
filesystem access outside the attempt directory through OpenCode's file tools.
The structural tests and scripted game inputs live outside the agent workspace.
This is workspace/configuration isolation, not an OS security sandbox; shell
execution is allowed.

Before a model's attempts begin, the harness warms it with a one-token response
to `hello model`. After all attempts for that model, it unloads the model before
moving to the next one. Warm-up work is not included in the reported OpenCode
token or timing measurements.

## Requirements

Install OpenCode and start the repository's Kronk server from another terminal:

```shell
opencode --version
make kronk-server
```

The server must use `zarf/kms/model_config.yaml`. A selected model must:

- be a key in that file;
- end in `/AGENT`; and
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
    CODEGEN_MODEL=ornith-ai/Ornith-1.5-35B-Q8_0/AGENT \
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
| `CODEGEN_MODEL` | `all` | One ID, comma-separated IDs, or every `/AGENT` model |
| `CODEGEN_V2_SUBMISSIONS` | `3` | Maximum total submissions per model, including the initial one |
| `CODEGEN_V2_FEEDBACK` | `true` | Resume failures with grader feedback; false stops after one submission |
| `CODEGEN_STEPS` | `40` | Maximum OpenCode agent steps per submission |
| `CODEGEN_TIMEOUT` | `30m` | Timeout per submission; also separately applied to warm-up and unload |

Outputs are written beneath `.tools/codegen-benchmark/version2/output/`. After
preflight checks, each benchmark run deletes that version's previous output, so it
contains only the selected models and summary report from the current run. To compare models, select them
together with `CODEGEN_MODEL=all` or a comma-separated list.

Each model has a persistent `workspace/tictactoe/main.go` and isolated OpenCode
configuration/state under `workspace/opencode/`. Each `attempt-XX` directory is a
submission record outside that workspace, preserving its prompt (including repair
feedback), JSON event stream, stderr, grader result, and `main.go.txt` source snapshot.
Every failed submission also saves the grader's response in `feedback.md`, including
the final failure when no repair budget remains.
The original benchmark's output is never cleared or used by this version.

The report shows final checks passed, the first passing submission (or `Not passed`),
and cumulative agent wall time, turns, tool calls, and tokens across all submissions,
including failed work. Grader success is separate from agent exit status. Wall time
excludes setup, warm-up, and grading. A run can take up to three 30-minute agent
invocations plus setup, grading, warm-up and unload.

Agent token statistics sum the final usage event from every OpenCode turn. Kronk
reports reused prompt tokens as `Cache read`; the report omits provider cache-write
usage because Kronk's OpenAI-compatible API does not expose that metric.

Regenerate a report with:

```shell
make benchmark-codegen-v2-report
```
